// claude-proxy is the Platform-owned Anthropic gateway. Every VM forwards its
// Claude API calls through :8443 with a bearer session token. The proxy
// looks up the backing Anthropic API key, enforces per-session quota, writes
// an audit row, and streams the response back. The shared Max-plan token is
// sunset; no VM holds Anthropic credentials directly.
package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mikelm20/learn-platform/proxy/internal/audit"
	"github.com/mikelm20/learn-platform/proxy/internal/keys"
	"github.com/mikelm20/learn-platform/proxy/internal/quota"
	"github.com/mikelm20/learn-platform/proxy/internal/server"
)

func main() {
	var (
		listen   = flag.String("listen", ":8443", "TLS listen address")
		tlsCert  = flag.String("tls-cert", os.Getenv("PROXY_TLS_CERT_FILE"), "path to TLS certificate PEM")
		tlsKey   = flag.String("tls-key", os.Getenv("PROXY_TLS_KEY_FILE"), "path to TLS private key PEM")
		plain    = flag.Bool("plain", false, "serve plaintext HTTP (dev only; never prod)")
		upstream = flag.String("upstream", envOr("PROXY_UPSTREAM", "https://api.anthropic.com"), "Anthropic base URL")
		pgURL    = flag.String("postgres", os.Getenv("POSTGRES_URL"), "postgres connection string for audit; empty = in-memory")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	poolKeys, err := keys.LoadFromEnv()
	if err != nil {
		logger.Error("keys: load from env failed", "err", err, "hint", "set ANTHROPIC_API_KEYS (JSON array) via doppler run")
		os.Exit(2)
	}
	pool := keys.New(poolKeys)

	var sink audit.Sink = audit.NewInMemorySink()
	var db *sql.DB
	if *pgURL != "" {
		db, err = sql.Open("pgx", *pgURL)
		if err != nil {
			logger.Error("postgres open failed", "err", err)
			os.Exit(2)
		}
		db.SetMaxOpenConns(8)
		db.SetConnMaxLifetime(30 * time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := db.PingContext(ctx); err != nil {
			cancel()
			logger.Error("postgres ping failed", "err", err)
			os.Exit(2)
		}
		cancel()
		// Ensure the schema exists. Idempotent; safe to run on every boot.
		if _, err := db.Exec(audit.Schema); err != nil {
			logger.Error("postgres migrate failed", "err", err)
			os.Exit(2)
		}
		sink = audit.NewSQLSink(db)
		logger.Info("audit: postgres sink ready")
	} else {
		logger.Warn("audit: using in-memory sink, records dropped on restart")
	}

	srv, err := server.New(server.Config{
		Upstream:   *upstream,
		Pool:       pool,
		Quota:      quota.New(quota.DefaultLimits),
		Audit:      sink,
		AdminToken: os.Getenv("PROXY_ADMIN_TOKEN"),
		Logger:     logger,
	})
	if err != nil {
		logger.Error("server init failed", "err", err)
		os.Exit(2)
	}

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	if !*plain {
		if *tlsCert == "" || *tlsKey == "" {
			logger.Error("TLS cert/key required (--tls-cert, --tls-key) unless --plain")
			os.Exit(2)
		}
		httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("claude-proxy listening", "addr", *listen, "tls", !*plain, "keys", len(poolKeys))
		var err error
		if *plain {
			err = httpSrv.ListenAndServe()
		} else {
			err = httpSrv.ListenAndServeTLS(*tlsCert, *tlsKey)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("shutdown", "err", err)
	}
	if db != nil {
		_ = db.Close()
	}
}

func envOr(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

