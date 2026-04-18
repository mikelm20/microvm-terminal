// Package api wires HTTP / WS handlers.
package api

import (
	"log/slog"
	"net/http"

	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(mgr *session.Manager, gate *auth.Gate, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.Handle("GET /metrics", promhttp.Handler())

	// Auth endpoints (public)
	mux.HandleFunc("POST /login", gate.LoginHandler())
	mux.HandleFunc("POST /logout", gate.LogoutHandler())
	mux.HandleFunc("GET /auth/check", gate.CheckHandler())

	// Session endpoints (auth-required)
	h := &sessionHandler{mgr: mgr, logger: logger}
	authed := http.NewServeMux()
	authed.HandleFunc("POST /sessions", h.Create)
	authed.HandleFunc("DELETE /sessions/{id}", h.Destroy)
	authed.HandleFunc("GET /sessions/{id}/pty", h.PTY)
	authed.HandleFunc("GET /sessions/{id}/wizard", h.Wizard)
	authed.HandleFunc("/sessions/{id}/preview/{port}/", h.Preview)
	authed.HandleFunc("/sessions/{id}/preview/{port}", h.Preview)
	mux.Handle("/sessions", gate.Middleware(authed))
	mux.Handle("/sessions/", gate.Middleware(authed))

	return logging(logger, mux)
}

func logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("req", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
