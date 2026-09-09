// Package session owns the life cycle of VM sessions. One Manager per daemon;
// at most MaxConcurrent sessions at a time.
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
	"github.com/mikelm20/learn-platform/control-plane/internal/vm"
)

// Kernel command line parameters consumed by the guest agent.
const (
	cmdlineTokenKey = "mvt.session_token"
	cmdlineVsockKey = "mvt.vsock_port"
	guestVsockPort  = 5555
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
	alloc, err := netalloc.New(cfg.VMCIDR)
	if err != nil {
		return nil, fmt.Errorf("netalloc: %w", err)
	}
	subnet, err := netip.ParsePrefix(cfg.VMCIDR)
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
	_ = ctx // prep steps carry their own timeouts; VM lifetime is manager-owned
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
	vmDir := filepath.Join(m.cfg.VMDataDir, sid)
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

	hostname := fmt.Sprintf("vm-%s", shortID(sid))
	if err := firecracker.PrepareRootfs(m.cfg.RootfsPath, rootfs, ip, m.allocator.PrefixLen(), m.allocator.HostAddr(), hostname, token); err != nil {
		cleanup(vmDir, tap, m.allocator, ip)
		return nil, fmt.Errorf("prepare rootfs: %w", err)
	}
	m.logger.Info("vm boot: rootfs ready", "session", sid, "path", rootfs)

	vsockUDS := filepath.Join(vmDir, "fc.vsock")
	bootArgs := fmt.Sprintf("console=ttyS0 reboot=k panic=1 pci=off rw %s=%s %s=%d",
		cmdlineTokenKey, sessionToken, cmdlineVsockKey, guestVsockPort)
	fcSpec := firecracker.LaunchSpec{
		BinaryPath: m.cfg.FirecrackerBin,
		VMDir:      vmDir,
		KernelPath: m.cfg.KernelPath,
		RootfsPath: rootfs,
		VcpuCount:  m.cfg.VM.Vcpu,
		MemMiB:     m.cfg.VM.MemMiB,
		TapName:    tap,
		GuestMAC:   mac,
		BootArgs:   bootArgs,
		VsockCID:   cid,
		VsockUDS:   vsockUDS,
	}
	m.logger.Info("vm boot: kernel start requested",
		"session", sid, "kernel", fcSpec.KernelPath, "rootfs", fcSpec.RootfsPath,
		"vcpu", fcSpec.VcpuCount, "mem_mib", fcSpec.MemMiB,
		"jailer", m.cfg.Jailer.Enabled)

	proc, err := m.launchVM(sid, fcSpec)
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
		ready:        make(chan struct{}),
		// Ring buffer of serial output so late-attaching WebSockets get boot
		// output, not a blank screen. Plus it keeps the pipe drained so FC
		// never blocks on write when no one is attached.
		Serial: NewSerialTee(proc.Stdout(), 256*1024),
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

// launchVM dispatches to either vm.LaunchJailed (default on the real path)
// or firecracker.Launch (when jailer is disabled, e.g. bring-up on a clean
// box). Both return a VMProcess-compatible handle.
func (m *Manager) launchVM(sid string, spec firecracker.LaunchSpec) (VMProcess, error) {
	if m.cfg.Jailer.Enabled {
		js := vm.JailedSpec{
			LaunchSpec:     spec,
			VMID:           sid,
			JailerBin:      m.cfg.Jailer.Script,
			ChrootBase:     m.cfg.Jailer.ChrootBase,
			CPUQuotaMicros: m.cfg.JailerCPUQuotaMicros(),
			MemBytes:       m.cfg.JailerMemBytes(),
			SeccompProfile: m.cfg.Jailer.SeccompProfile,
			JailerUser:     m.cfg.Jailer.User,
			JailerUID:      m.cfg.Jailer.UID,
			JailerGID:      m.cfg.Jailer.GID,
		}
		return vm.LaunchJailed(context.Background(), js)
	}
	return firecracker.Launch(context.Background(), spec)
}

// ensure both process shapes satisfy VMProcess. Compile-time guards.
var (
	_ VMProcess = (*firecracker.Process)(nil)
	_ VMProcess = (*vm.Process)(nil)
)

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

	// Signal listeners (vsock, PTY) to stop. CloseDone is guarded by a
	// sync.Once because Host.Destroy and the reaper goroutine in Create can
	// both reach here for the same session.
	s.CloseDone()

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
	ErrAtCapacity    = errors.New("session pool at capacity")
	ErrNotFound      = errors.New("session not found")
	ErrNoGuest       = errors.New("no guest-agent attached")
	ErrSessionClosed = errors.New("session closed")
	ErrBootTimeout   = errors.New("vm boot timed out waiting for guest-agent handshake")
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
