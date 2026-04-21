//go:build integration_firecracker

// Package integration exercises the real Firecracker launcher end to end.
//
// Gated behind `-tags=integration_firecracker` because it needs:
//
//   - Linux host with /dev/kvm accessible to the running user
//   - Firecracker + jailer binaries on PATH or at the configured paths
//   - A prebuilt vmlinux + rootfs.ext4 (see vm-image/ for the build scripts)
//   - CAP_NET_ADMIN and root-ish privileges to create tap devices and mount
//     the per-VM rootfs copy
//
// Run via `make integration-vm` from control-plane/ or the repo root.
//
// Environment knobs:
//
//	LEARN_IT_KERNEL       absolute path to the vmlinux file                    (required)
//	LEARN_IT_ROOTFS       absolute path to the golden rootfs.ext4              (required)
//	LEARN_IT_TOKEN_FILE   absolute path to the claude OAuth token file         (required)
//	LEARN_IT_FC_BIN       firecracker binary                                   (default /usr/local/bin/firecracker)
//	LEARN_IT_JAILER       jailer wrapper script                                (default /usr/local/libexec/learn-platform/jailer.sh)
//	LEARN_IT_VM_DATA_DIR  per-VM scratch root                                  (default /var/lib/firecracker/vms)
//	LEARN_IT_CHROOT_BASE  jailer chroot base                                   (default /srv/jailer)
//	LEARN_IT_USE_JAILER   "0" disables the jailer path (bring-up only)         (default "1")
//	LEARN_IT_BRIDGE       host bridge name                                     (default fc-br0)
//	LEARN_IT_CIDR         bridge CIDR                                          (default 172.20.0.0/24)
//	LEARN_IT_BOOT_TIMEOUT seconds to wait for guest-agent handshake            (default 90)
package integration

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/mikelm20/learn-platform/control-plane/internal/config"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// TestFirecrackerBootAndHandshake boots one real Firecracker VM through the
// same code path the control plane uses for learners, waits for the
// guest-agent vsock handshake to complete, asserts session_ready fires, and
// tears the VM down. Failure modes it catches:
//
//   - Mock launcher silently selected on a host that should boot real VMs.
//   - Jailer misconfiguration (chroot, cgroups, seccomp) stopping the kernel.
//   - Guest-agent not being baked into the rootfs or not dialling vsock.
//   - session_token mismatch between kernel cmdline and hello frame.
//   - Ready signal (MarkReady / WaitReady) not being wired.
func TestFirecrackerBootAndHandshake(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("firecracker integration test requires linux/kvm, got %s", runtime.GOOS)
	}
	requireEnv := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			t.Skipf("skip: %s not set; see test docstring for required env", key)
		}
		return v
	}

	cfg := config.Config{
		MaxConcurrent:        1,
		FirecrackerBin:       envOr("LEARN_IT_FC_BIN", "/usr/local/bin/firecracker"),
		JailerBin:            envOr("LEARN_IT_JAILER_BIN", "/usr/local/bin/jailer"),
		KernelPath:           requireEnv("LEARN_IT_KERNEL"),
		RootfsPath:           requireEnv("LEARN_IT_ROOTFS"),
		VmDataDir:            envOr("LEARN_IT_VM_DATA_DIR", "/var/lib/firecracker/vms"),
		BridgeName:           envOr("LEARN_IT_BRIDGE", "fc-br0"),
		VmCIDRBase:           envOr("LEARN_IT_CIDR", "172.20.0.0/24"),
		ClaudeOAuthTokenFile: requireEnv("LEARN_IT_TOKEN_FILE"),
		UseJailer:            envBool("LEARN_IT_USE_JAILER", true),
		JailerScript:         envOr("LEARN_IT_JAILER", "/usr/local/libexec/learn-platform/jailer.sh"),
		JailerChrootBase:     envOr("LEARN_IT_CHROOT_BASE", "/srv/jailer"),
		JailerCPUQuotaMicros: 100_000,
		JailerMemBytes:       1 << 30,
		BootTimeoutSeconds:   envInt("LEARN_IT_BOOT_TIMEOUT", 90),
		UseMockLauncher:      false,
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	s, err := mgr.Create(ctx)
	if err != nil {
		t.Fatalf("Create VM: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Destroy(s.ID) })

	// Assert the session-token handshake: the token is set in the kernel
	// cmdline and the guest-agent echoes it back on vsock hello. A
	// mismatch causes the listener to reject the hello, so WaitReady
	// would time out. WaitReady returning nil therefore proves the
	// handshake used a matching token.
	bootTimeout := time.Duration(cfg.BootTimeoutSeconds) * time.Second
	waitCtx, waitCancel := context.WithTimeout(ctx, bootTimeout)
	defer waitCancel()
	if err := s.WaitReady(waitCtx); err != nil {
		t.Fatalf("WaitReady: %v (boot took %s)", err, time.Since(start))
	}

	readyAt := s.ReadyAt()
	if readyAt.IsZero() {
		t.Fatal("ReadyAt zero after WaitReady returned nil")
	}
	bootElapsed := readyAt.Sub(s.CreatedAt())
	t.Logf("session %s ready after %s (token prefix %s...)", s.ID, bootElapsed, safePrefix(s.SessionToken))

	if bootElapsed <= 0 {
		t.Fatalf("non-positive boot elapsed: %s", bootElapsed)
	}
	if s.SessionToken == "" {
		t.Fatal("session token is empty; cmdline never carried it")
	}

	// vm_ready / agent_online must have been published.
	snap, _, unsub := s.Events.Subscribe(16)
	defer unsub()
	gotReady := false
	gotAgent := false
	for _, e := range snap {
		switch e.Type {
		case "vm_ready":
			gotReady = true
		case "agent_online":
			gotAgent = true
		}
	}
	if !gotReady {
		t.Error("missing vm_ready event in history")
	}
	if !gotAgent {
		t.Error("missing agent_online event in history")
	}

	if err := mgr.Destroy(s.ID); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, ok := mgr.Get(s.ID); ok {
		t.Fatal("session still present after Destroy")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || v == "true"
}

func safePrefix(s string) string {
	if len(s) < 6 {
		return s
	}
	return s[:6]
}
