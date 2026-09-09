// The control plane boots one Firecracker microVM per session and bridges its
// serial console to a browser terminal.
//
//   - password login with a signed cookie
//   - POST /sessions boots a VM (jailer, cgroups, seccomp, nftables egress)
//   - GET /sessions/{id}/pty is the WebSocket to the VM serial console
//   - idle VMs are reaped, capacity is capped, state lands in Postgres
//   - /healthz and /metrics
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/api"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/config"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// Version is stamped at build time via -ldflags.
var Version = "dev"

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/microvm-terminal/config.yaml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logger.Info("starting", "version", Version, "config", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		logger.Error("migrate db", "err", err)
		os.Exit(1)
	}
	logger.Info("db ready")

	gate, err := auth.NewGate(cfg.PasswordFile, cfg.CookieSecretFile, auth.Options{
		Secure: cfg.SecureCookies,
		Domain: cfg.CookieDomain,
	})
	if err != nil {
		logger.Error("login gate", "err", err, "hint", "write the shared password to password_file (8+ characters)")
		os.Exit(1)
	}

	launcher, err := pickLauncher(cfg, logger)
	if err != nil {
		logger.Error("pick launcher", "err", err)
		os.Exit(1)
	}
	var pool *session.WarmPool
	if cfg.WarmPoolTarget > 0 {
		pool = session.NewWarmPool(launcher, cfg.WarmPoolTarget, logger)
		pool.Start()
		defer pool.Stop()
	}
	host := session.NewHost(launcher, pool, cfg.MaxConcurrent, logger)
	host.OnGone(func(s *session.Session) {
		id, err := uuid.Parse(s.ID)
		if err != nil {
			return
		}
		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.MarkSessionReaped(dbCtx, id); err != nil {
			logger.Warn("mark session reaped", "id", s.ID, "err", err)
		}
	})
	go host.RunIdleReaper(ctx, time.Duration(cfg.IdleTimeoutSeconds)*time.Second, 15*time.Second)
	api.RegisterMetrics(host)

	handler := api.NewRouter(api.Deps{
		Logger:       logger,
		Store:        store,
		Gate:         gate,
		Host:         host,
		PublicOrigin: cfg.PublicOrigin,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddr, "max_concurrent", cfg.MaxConcurrent,
			"idle_timeout_s", cfg.IdleTimeoutSeconds, "vcpu", cfg.VM.Vcpu, "mem_mib", cfg.VM.MemMiB)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown", "err", err)
	}
	// Tear down every VM so no Firecracker process outlives the daemon.
	for _, s := range host.List() {
		_ = host.Destroy(s.ID)
	}
	fmt.Fprintln(os.Stderr, "bye")
}

// pickLauncher returns the VM launcher the daemon should use.
//
// Default is the real Firecracker-backed launcher (via session.NewManager).
// The mock launcher is only selected when cfg.UseMockLauncher is true. It
// serves in-memory sessions with no console and exists for the API tests and
// for development on machines without KVM.
//
// A failure to initialise the real manager when mock is NOT requested is
// fatal: silently falling back to the mock used to hide a broken host.
func pickLauncher(cfg config.Config, logger *slog.Logger) (session.Launcher, error) {
	if cfg.UseMockLauncher {
		logger.Warn("launcher: mock (opt-in via config); sessions have no console")
		return session.LauncherFunc(func(ctx context.Context) (*session.Session, error) {
			return session.NewInMemorySession(""), nil
		}), nil
	}
	logger.Info("launcher: real firecracker",
		"kernel", cfg.KernelPath,
		"rootfs", cfg.RootfsPath,
		"jailer", cfg.Jailer.Enabled,
		"chroot_base", cfg.Jailer.ChrootBase,
		"cpu_quota_us", cfg.JailerCPUQuotaMicros(),
		"mem_bytes", cfg.JailerMemBytes(),
	)
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init firecracker manager: %w (set use_mock_launcher: true to run without a hypervisor)", err)
	}
	boot := time.Duration(cfg.BootTimeoutSeconds) * time.Second
	return &managerLauncher{mgr: mgr, boot: boot, logger: logger}, nil
}

// managerLauncher adapts *session.Manager to Launcher and waits for the
// guest-agent handshake before handing the session out, so a client that
// gets a session id can attach to a console that is already alive.
type managerLauncher struct {
	mgr    *session.Manager
	boot   time.Duration
	logger *slog.Logger
}

func (a *managerLauncher) Launch(ctx context.Context) (*session.Session, error) {
	s, err := a.mgr.Create(ctx)
	if err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, a.boot)
	defer cancel()
	if err := s.WaitReady(waitCtx); err != nil {
		a.logger.Error("vm did not become ready within boot timeout",
			"session", s.ID, "timeout", a.boot, "err", err)
		_ = a.mgr.Destroy(s.ID)
		return nil, session.ErrBootTimeout
	}
	return s, nil
}
