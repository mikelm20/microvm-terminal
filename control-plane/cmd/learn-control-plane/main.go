// learn-control-plane serves the full CONTRACTS.md API surface:
//   - identity + magic-link auth + Postgres-backed accounts
//   - lesson catalog from lessons/*.yml
//   - progress sync
//   - session lifecycle (cold + warm pool) + wizard WS + transcript
//   - public read-only proof + certificate endpoints
//   - /healthz + /metrics
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
	"github.com/mikelm20/learn-platform/control-plane/internal/mail"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open db", "err", err, "url", cfg.DatabaseURL)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		logger.Error("migrate db", "err", err)
		os.Exit(1)
	}
	logger.Info("db ready")

	secret, err := loadOrMintSecret(cfg.IdentityCookieSecret, logger)
	if err != nil {
		logger.Error("cookie secret", "err", err)
		os.Exit(1)
	}
	signer := identity.NewSigner(secret)

	var sender mail.Sender
	if cfg.ResendAPIKey != "" && cfg.ResendFromAddress != "" {
		sender = mail.NewResendSender(cfg.ResendAPIKey, cfg.ResendFromAddress, logger)
		logger.Info("mail via resend", "from", cfg.ResendFromAddress)
	} else {
		sender = mail.NewStdoutSender(logger)
		logger.Info("mail via stdout (no RESEND_API_KEY)")
	}
	if cfg.VoiceDir != "" {
		mail.SetVoiceDir(cfg.VoiceDir)
	}

	launcher := pickLauncher(cfg, logger)
	var pool *session.WarmPool
	if cfg.WarmPoolTarget > 0 {
		pool = session.NewWarmPool(launcher, cfg.WarmPoolTarget, logger)
		pool.Start()
		defer pool.Stop()
	}
	h := session.NewHost(launcher, pool, cfg.MaxConcurrent, logger)

	transcript := session.NewTranscript(store, logger)

	var legacyGate *auth.Gate
	if _, err := os.Stat(cfg.AuthPasswordFile); err == nil {
		if g, err := auth.NewGate(cfg.AuthPasswordFile, cfg.AuthCookieSecretFile); err == nil {
			legacyGate = g
		}
	}

	handler := api.NewRouter(api.Deps{
		Logger:        logger,
		Store:         store,
		Signer:        signer,
		Host:          h,
		Transcript:    transcript,
		Mail:          sender,
		MagicLimiter:  auth.MagicLinkLimiter(),
		IPLimiter:     auth.NewTokenBucket(20, 5*time.Minute),
		LessonsDir:    cfg.LessonsDir,
		PublicOrigin:  cfg.PublicOrigin,
		SecureCookies: cfg.SecureCookies,
		LegacyGate:    legacyGate,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
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
	fmt.Fprintln(os.Stderr, "bye")
}

// pickLauncher returns a VM launcher based on config. In UseMockLauncher mode
// (or when the Firecracker manager cannot be built, e.g. on macOS dev boxes)
// it returns a mock that yields empty Sessions instantly.
func pickLauncher(cfg config.Config, logger *slog.Logger) session.Launcher {
	if cfg.UseMockLauncher {
		logger.Info("using mock launcher")
		return session.LauncherFunc(func(ctx context.Context) (*session.Session, error) {
			return session.NewInMemorySession(""), nil
		})
	}
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		logger.Warn("firecracker manager init failed, falling back to mock launcher", "err", err)
		return session.LauncherFunc(func(ctx context.Context) (*session.Session, error) {
			return session.NewInMemorySession(""), nil
		})
	}
	return session.LauncherFunc(func(ctx context.Context) (*session.Session, error) {
		return mgr.Create(ctx)
	})
}

func loadOrMintSecret(hex32 string, logger *slog.Logger) ([]byte, error) {
	if hex32 != "" {
		b, err := hex.DecodeString(hex32)
		if err != nil {
			return nil, fmt.Errorf("decode secret: %w", err)
		}
		if len(b) < 32 {
			return nil, fmt.Errorf("secret too short: %d bytes", len(b))
		}
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	logger.Warn("IDENTITY_COOKIE_SECRET not set; generated ephemeral secret (restart invalidates identity cookies)")
	return b, nil
}
