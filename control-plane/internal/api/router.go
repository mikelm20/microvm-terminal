// Package api wires HTTP + WS handlers to the domain packages.
//
// Routes are mounted in NewRouter; see CONTRACTS.md section 5 for the full
// table. Every handler:
//   - validates input against apitypes,
//   - resolves identity via identity.Middleware,
//   - emits shape-correct JSON responses + apitypes.ApiError on failure,
//   - attaches a request_id to every response.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
	"github.com/mikelm20/learn-platform/control-plane/internal/mail"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Deps is the full set of collaborators a Server needs.
type Deps struct {
	Logger       *slog.Logger
	Store        *db.Store
	Signer       *identity.Signer
	Host         SessionHost
	Transcript   *session.Transcript
	Mail         mail.Sender
	MagicLimiter *auth.TokenBucket
	IPLimiter    *auth.TokenBucket
	LessonsDir   string
	// PublicOrigin is prefixed to generated links (magic-link URLs + WS URLs).
	// e.g. "http://localhost:8080" in dev.
	PublicOrigin string
	// SecureCookies controls the Secure flag on cookies. false for local http.
	SecureCookies bool
	// CookieDomain, when non-empty, is set as the Domain attribute on every
	// cookie issued by the control plane. Used to share auth across
	// subdomains (e.g. ".example.com"). Leave empty for local http dev.
	CookieDomain string
	// Legacy gate for /login + /sessions. Optional; when nil those routes
	// are not mounted and the client uses the new identity + magic-link flow.
	LegacyGate *auth.Gate
}

// NewRouter builds the chi mux and returns an http.Handler.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(requestIDMiddleware)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(identity.Middleware(deps.Signer, deps.Store))

	r.Get("/healthz", healthz)
	r.Method(http.MethodGet, "/metrics", promhttp.Handler())

	idH := &identityHandler{deps: deps}
	authH := &authHandler{deps: deps}
	meH := &meHandler{deps: deps}
	lessonsH := &lessonsHandler{deps: deps}
	progressH := &progressHandler{deps: deps}
	sessH := &sessionsHandler{deps: deps}
	publicH := &publicHandler{deps: deps}
	capstoneH := &capstoneHandler{deps: deps}
	previewH := newPreviewHandler(deps)

	r.Post("/identity", idH.Mint)
	r.Post("/auth/magic-link", authH.RequestMagicLink)
	r.Post("/auth/claim", authH.Claim)
	r.Post("/auth/logout", authH.Logout)

	r.Get("/me", meH.Get)
	r.Patch("/me", meH.Patch)

	r.Get("/lessons", lessonsH.List)
	r.Get("/lessons/{id}", lessonsH.Get)

	// Legacy auth gate for the Claude Code Lab. Intro stays open;
	// any endpoint that boots a VM or talks to one is wrapped below. The
	// routes here let the landing mint + check the learn_auth cookie.
	if deps.LegacyGate != nil {
		r.Post("/lab/login", deps.LegacyGate.LoginHandler())
		r.Post("/lab/logout", deps.LegacyGate.LogoutHandler())
		r.Get("/lab/auth/check", deps.LegacyGate.CheckHandler())
	}

	r.Group(func(r chi.Router) {
		if deps.LegacyGate != nil {
			r.Use(deps.LegacyGate.Middleware)
		}

		r.Post("/progress/sync", progressH.Sync)
		r.Get("/progress", progressH.Get)

		r.Post("/sessions", sessH.Create)
		r.Get("/sessions/{id}", sessH.Describe)
		r.Get("/sessions/{id}/heartbeat", sessH.Heartbeat)
		r.Get("/sessions/{id}/transcript", sessH.Transcript)
		r.Post("/sessions/{id}/prompt", sessH.SubmitPrompt)
		r.Post("/sessions/{id}/attach", sessH.Attach)
		r.Delete("/sessions/{id}", sessH.Delete)
		r.Get("/sessions/{id}/ws", sessH.WS)
		r.Get("/sessions/{id}/pty", sessH.PTY)

		// Capstone (F2 Lab closing flow). Validator + Builder VMs, plus a
		// reverse-proxy preview path so the mini-app renders in an iframe on
		// learn.example.com. See internal/api/capstone.go and preview.go.
		r.Post("/capstone/validate", capstoneH.Validate)
		r.Post("/capstone/build", capstoneH.Build)
		r.Handle("/sessions/{id}/preview/{port}/*", previewH)
		r.Handle("/sessions/{id}/preview/{port}", previewH)
	})

	r.Get("/p/{uuid}/public", publicH.Profile)
	r.Get("/certificate/{id}/public", publicH.Certificate)

	return r
}

// keep the session + auth import alive for the Deps struct fields above.
var _ = session.ErrNotFound
var _ = auth.CookieName

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
