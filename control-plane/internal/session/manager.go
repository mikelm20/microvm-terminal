// Package session owns the life cycle of VM sessions. One Manager per daemon;
// at most N concurrent sessions (MVP cap = 3).
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/config"
	"github.com/mikelm20/learn-platform/control-plane/internal/firecracker"
	"github.com/mikelm20/learn-platform/control-plane/internal/netalloc"
)

type Manager struct {
	cfg       config.Config
	logger    *slog.Logger
	allocator *netalloc.Allocator
	upstream  string // detected default iface, e.g. eno1

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(cfg config.Config, logger *slog.Logger) (*Manager, error) {
	alloc, err := netalloc.New(cfg.VmCIDRBase)
	if err != nil {
		return nil, fmt.Errorf("netalloc: %w", err)
	}
	subnet, err := netip.ParsePrefix(cfg.VmCIDRBase)
	if err != nil {
		return nil, err
	}
	upstream, err := detectUpstreamIface()
	if err != nil {
		return nil, fmt.Errorf("detect upstream: %w", err)
	}

	if err := firecracker.EnsureBridge(cfg.BridgeName, alloc.HostAddr(), alloc.PrefixLen(), subnet, upstream); err != nil {
		return nil, fmt.Errorf("ensure bridge: %w", err)
	}

	return &Manager{
		cfg:       cfg,
		logger:    logger,
		allocator: alloc,
		upstream:  upstream,
		sessions:  make(map[string]*Session),
	}, nil
}

// Create allocates resources, prepares a per-VM rootfs, launches Firecracker,
// and returns a Session ready for PTY attachment. The ctx parameter bounds the
// creation sequence (rootfs prep, process start) but NOT the VM lifetime.
func (m *Manager) Create(ctx context.Context) (*Session, error) {
	_ = ctx // used for cancelling the prep steps via their own timeouts; VM lifetime is manager-owned
	m.mu.Lock()
	if len(m.sessions) >= m.cfg.MaxConcurrent {
		m.mu.Unlock()
		return nil, ErrAtCapacity
	}
	m.mu.Unlock()

	ip, mac, tap, cid, err := m.allocator.Alloc()
	if err != nil {
		return nil, fmt.Errorf("alloc net: %w", err)
	}

	if err := firecracker.CreateTAP(tap, m.cfg.BridgeName); err != nil {
		m.allocator.Release(ip)
		return nil, fmt.Errorf("create tap: %w", err)
	}

	// Random 32-byte token proves the guest-agent is ours.
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		_ = firecracker.DeleteTAP(tap)
		m.allocator.Release(ip)
		return nil, fmt.Errorf("rand: %w", err)
	}
	sessionToken := hex.EncodeToString(tokBytes)

	sid := uuid.NewString()
	vmDir := filepath.Join(m.cfg.VmDataDir, sid)
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		_ = firecracker.DeleteTAP(tap)
		m.allocator.Release(ip)
		return nil, fmt.Errorf("mkdir vmdir: %w", err)
	}
	rootfs := filepath.Join(vmDir, "rootfs.ext4")

	token, err := readTokenFile(m.cfg.ClaudeOAuthTokenFile)
	if err != nil {
		cleanup(vmDir, tap, m.allocator, ip)
		return nil, fmt.Errorf("read token file: %w", err)
	}

	hostname := fmt.Sprintf("learn-vm-%s", shortID(sid))
	if err := firecracker.PrepareRootfs(m.cfg.RootfsPath, rootfs, ip, m.allocator.PrefixLen(), m.allocator.HostAddr(), hostname, token); err != nil {
		cleanup(vmDir, tap, m.allocator, ip)
		return nil, fmt.Errorf("prepare rootfs: %w", err)
	}

	// The VM lives past the creating HTTP request; use a manager-owned context
	// rather than the request context (which would kill it on handler return).
	vsockUDS := filepath.Join(vmDir, "fc.vsock")
	bootArgs := fmt.Sprintf("console=ttyS0 reboot=k panic=1 pci=off rw learn.session_token=%s learn.vsock_port=5555", sessionToken)
	proc, err := firecracker.Launch(context.Background(), firecracker.LaunchSpec{
		BinaryPath: m.cfg.FirecrackerBin,
		VMDir:      vmDir,
		KernelPath: m.cfg.KernelPath,
		RootfsPath: rootfs,
		VcpuCount:  2,
		MemMiB:     2048,
		TapName:    tap,
		GuestMAC:   mac,
		BootArgs:   bootArgs,
		VsockCID:   cid,
		VsockUDS:   vsockUDS,
	})
	if err != nil {
		cleanup(vmDir, tap, m.allocator, ip)
		return nil, fmt.Errorf("launch: %w", err)
	}

	s := &Session{
		ID:           sid,
		IP:           ip,
		MAC:          mac,
		Tap:          tap,
		Hostname:     hostname,
		SessionToken: sessionToken,
		Process:      proc,
		VmDir:        vmDir,
		createdAt:    time.Now(),
		mgr:          m,
		done:         make(chan struct{}),
		// Ring buffer of serial output so late-attaching WebSockets get boot
		// output, not a blank screen. Plus it keeps the pipe drained so FC
		// never blocks on write when no one is attached.
		Serial: NewSerialTee(proc.Stdout(), 256*1024),
		// Event bus for guest-agent events relayed via vsock.
		Events: NewEventBus(),
	}
	m.mu.Lock()
	m.sessions[sid] = s
	m.mu.Unlock()

	// Start accepting vsock connections from the guest-agent inside the VM.
	m.startVsockListener(s, vsockUDS)

	// Reap the FC process if it exits on its own (guest shutdown, crash).
	go func() {
		_ = proc.Wait()
		m.logger.Info("vm exited", "id", sid)
		_ = m.Destroy(sid)
	}()

	m.logger.Info("session created", "id", sid, "ip", ip.String(), "tap", tap, "hostname", hostname)
	return s, nil
}

// Get looks up a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Destroy stops a session's VM and releases its resources.
func (m *Manager) Destroy(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	// Signal listeners (vsock, wizard WS) to stop.
	close(s.done)

	_ = s.Process.Stop()
	_ = firecracker.DeleteTAP(s.Tap)
	m.allocator.Release(s.IP)
	_ = os.RemoveAll(s.VmDir)
	m.logger.Info("session destroyed", "id", id)
	return nil
}

// Shutdown is called on daemon exit; best-effort teardown of all sessions.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Destroy(id)
	}
	return nil
}

// Count returns the current number of active sessions (for metrics / cap check).
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

var (
	ErrAtCapacity = errors.New("session pool at capacity")
	ErrNotFound   = errors.New("session not found")
)

func cleanup(vmDir, tap string, alloc *netalloc.Allocator, ip netip.Addr) {
	_ = os.RemoveAll(vmDir)
	_ = firecracker.DeleteTAP(tap)
	alloc.Release(ip)
}

func readTokenFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// detectUpstreamIface reads the kernel's default route and returns its egress
// interface name (e.g., "eno1"). We only call this at startup.
func detectUpstreamIface() (string, error) {
	out, err := exec.Command("ip", "-o", "route", "show", "default").Output()
	if err != nil {
		return "", err
	}
	// Line looks like: "default via <upstream-gw> dev eno1 proto static ..."
	fields := strings.Fields(string(out))
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == "dev" {
			return fields[i+1], nil
		}
	}
	return "", errors.New("no default route dev found")
}

func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}
