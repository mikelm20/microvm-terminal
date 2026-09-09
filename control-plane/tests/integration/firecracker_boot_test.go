//go:build integration_firecracker

// Package integration exercises the real Firecracker launcher end to end.
//
// Gated behind `-tags=integration_firecracker` because it needs:
//
//   - Linux host with /dev/kvm accessible to the running user
//   - Firecracker + jailer binaries at the configured paths
//   - A prebuilt vmlinux + rootfs.ext4 (see vm-image/ for the build scripts)
//   - CAP_NET_ADMIN and root-ish privileges to create tap devices and mount
//     the per-VM rootfs copy
//
// Run via `make integration-vm` from control-plane/ or the repo root.
//
// Environment knobs:
//
//	IT_KERNEL       absolute path to the vmlinux file                    (required)
//	IT_ROOTFS       absolute path to the golden rootfs.ext4              (required)
//	IT_TOKEN_FILE   absolute path to the claude OAuth token file         (required)
//	IT_FC_BIN       firecracker binary                                   (default /usr/local/bin/firecracker)
//	IT_JAILER       jailer wrapper script                                (default /usr/local/libexec/microvm-terminal/jailer.sh)
//	IT_VM_DATA_DIR  per-VM scratch root                                  (default /var/lib/firecracker/vms)
//	IT_CHROOT_BASE  jailer chroot base                                   (default /srv/jailer)
//	IT_USE_JAILER   "0" disables the jailer path (bring-up only)         (default "1")
//	IT_BRIDGE       host bridge name                                     (default fc-br0)
//	IT_CIDR         bridge CIDR                                          (default 172.20.0.0/24)
//	IT_BOOT_TIMEOUT seconds to wait for guest-agent handshake            (default 90)
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
// same code path the daemon uses, waits for the guest-agent vsock handshake,
// pushes a resize frame, and tears the VM down. Failure modes it catches:
//
//   - Jailer misconfiguration (chroot, cgroups, seccomp) stopping the kernel.
//   - Guest-agent not baked into the rootfs or not dialling vsock.
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

	cfg := config.Defaults()
	cfg.MaxConcurrent = 1
	cfg.FirecrackerBin = envOr("IT_FC_BIN", cfg.FirecrackerBin)
	cfg.KernelPath = requireEnv("IT_KERNEL")
	cfg.RootfsPath = requireEnv("IT_ROOTFS")
	cfg.VMDataDir = envOr("IT_VM_DATA_DIR", cfg.VMDataDir)
	cfg.BridgeName = envOr("IT_BRIDGE", cfg.BridgeName)
	cfg.VMCIDR = envOr("IT_CIDR", cfg.VMCIDR)
	cfg.ClaudeOAuthTokenFile = requireEnv("IT_TOKEN_FILE")
	cfg.Jailer.Enabled = envBool("IT_USE_JAILER", true)
	cfg.Jailer.Script = envOr("IT_JAILER", cfg.Jailer.Script)
	cfg.Jailer.ChrootBase = envOr("IT_CHROOT_BASE", cfg.Jailer.ChrootBase)
	cfg.BootTimeoutSeconds = envInt("IT_BOOT_TIMEOUT", 90)

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

	// A geometry sent before the guest is up must be replayed on handshake.
	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("Resize before ready: %v", err)
	}

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

	// With the guest attached, a resize must reach it without error.
	if err := s.Resize(100, 30); err != nil {
		t.Fatalf("Resize after ready: %v", err)
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
