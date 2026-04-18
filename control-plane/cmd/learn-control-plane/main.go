// learn-control-plane is the host-side daemon that launches Firecracker VMs on
// demand and bridges their serial consoles to browsers over WebSocket.
//
// MVP Phase 1 scope:
//   - POST   /sessions          -> allocate + boot a VM, return session id
//   - DELETE /sessions/{id}     -> tear down a VM
//   - GET    /sessions/{id}/pty -> WebSocket that bridges bytes to/from the VM serial
//   - GET    /healthz           -> liveness probe (no state, always 200)
//   - GET    /metrics           -> Prometheus metrics
//
// The daemon listens on 127.0.0.1:8080 by default; Caddy terminates TLS at
// learn.example.com and reverse-proxies here.
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

	"github.com/mikelm20/learn-platform/control-plane/internal/api"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/config"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// Version is stamped at build time via -ldflags.
var Version = "dev"

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/etc/learn-platform/config.toml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	logger.Info("starting", "version", Version, "config", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		logger.Error("init session manager", "err", err)
		os.Exit(1)
	}

	gate, err := auth.NewGate(cfg.AuthPasswordFile, cfg.AuthCookieSecretFile)
	if err != nil {
		logger.Error("init auth gate", "err", err)
		os.Exit(1)
	}

	handler := api.NewRouter(mgr, gate, logger)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout is intentionally unset: WS upgrades need long-lived writes.
	}

	// Graceful shutdown: drain VMs on SIGTERM/SIGINT.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddr)
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
	if err := mgr.Shutdown(shutdownCtx); err != nil {
		logger.Warn("session shutdown", "err", err)
	}
	fmt.Fprintln(os.Stderr, "bye")
}
