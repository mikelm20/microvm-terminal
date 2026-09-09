// Package api wires HTTP + WebSocket handlers to the session layer and
// serves the two static pages (login, terminal).
//
// Routes:
//
//	GET  /                      login page, or redirect to /terminal when the cookie is valid
//	POST /login                 form or JSON {name, password}; sets the cookie
//	GET  /logout, POST /logout  clears the cookie
//	GET  /terminal              xterm.js page (cookie required)
//	POST /sessions              boot a VM for the caller
//	GET  /sessions/current      newest live VM owned by the caller
//	GET  /sessions/{id}         describe one VM (live or historical)
//	DELETE /sessions/{id}       destroy a VM
//	GET  /sessions/{id}/pty     WebSocket to the VM serial console
//	GET  /healthz, /metrics
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/auth"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Deps is the full set of collaborators the handlers need.
type Deps struct {
	Logger *slog.Logger
	Store  SessionStore
	Gate   *auth.Gate
	Host   SessionHost
	// PublicOrigin is prefixed to the WebSocket URL returned by POST
	// /sessions, e.g. "https://terminal.example.com".
	PublicOrigin string
}

// NewRouter builds the chi mux and returns an http.Handler.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(requestIDMiddleware)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", healthz)
	r.Method(http.MethodGet, "/metrics", promhttp.Handler())

	pages := newPagesHandler(deps)
	sessH := &sessionsHandler{deps: deps}

	r.Get("/", pages.Login)
	r.Post("/login", pages.SubmitLogin)
	r.Get("/logout", pages.Logout)
	r.Post("/logout", pages.Logout)
	r.Get("/terminal", pages.Terminal)

	r.Group(func(r chi.Router) {
		r.Use(deps.Gate.Middleware)
		r.Post("/sessions", sessH.Create)
		r.Get("/sessions/current", sessH.Current)
		r.Get("/sessions/{id}", sessH.Describe)
		r.Delete("/sessions/{id}", sessH.Delete)
		r.Get("/sessions/{id}/pty", sessH.PTY)
	})

	return r
}

// RegisterMetrics publishes the live-session gauge. Call once per process.
func RegisterMetrics(host SessionHost) {
	prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "microvm_sessions_live",
		Help: "Number of VMs currently running.",
	}, func() float64 { return float64(host.Count()) }))
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
