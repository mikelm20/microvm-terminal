package api

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
)

// previewHandler implements the Capstone (and future) live preview path.
// Route: /sessions/{id}/preview/{port}/*
//
// Responsibilities:
//   - authenticate the caller via the standard identity middleware,
//   - verify the session is owned by them and still live,
//   - rate-limit per identity so a single learner can't accidentally DoS
//     us (hot reload bugs, tight polling),
//   - reverse-proxy to http://<vm_ip>:<port>/<rest> on a loopback-free
//     transport (no host.docker.internal shenanigans),
//   - strip hop-by-hop and sensitive headers from both directions,
//   - allow the iframe embed by emitting a CSP frame-ancestors header so
//     learn.example.com can host the mini-app.
//
// We deliberately do NOT implement subdomain wildcard preview in v1; the
// Capstone hardening risk is documented in CAPSTONE_DESIGN.md section 5.
type previewHandler struct {
	deps Deps

	// identity-scoped rate limiter. 60 req / 10s bucket is plenty for a
	// mini-app static serve + occasional ping; aggressive clients hit 429.
	mu     sync.Mutex
	bucket map[string]*rateBucket
}

func newPreviewHandler(deps Deps) *previewHandler {
	return &previewHandler{
		deps:   deps,
		bucket: make(map[string]*rateBucket),
	}
}

type rateBucket struct {
	count     int
	windowEnd time.Time
}

const (
	previewRateWindow = 10 * time.Second
	previewRateLimit  = 120 // requests per window per identity
)

// allow records a hit for the given identity key and returns true if the
// request fits within the window budget.
func (p *previewHandler) allow(key string) bool {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.bucket[key]
	if !ok || now.After(b.windowEnd) {
		p.bucket[key] = &rateBucket{count: 1, windowEnd: now.Add(previewRateWindow)}
		return true
	}
	b.count++
	return b.count <= previewRateLimit
}

// ServeHTTP dispatches the reverse proxy. Chi's URL params surface the
// session id and port; the catch-all ("*") captures the subpath after
// /preview/<port>/.
func (p *previewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	if !p.allow(id.String()) {
		apiErrorRetry(w, r, http.StatusTooManyRequests, apitypes.ErrRateLimited, "preview rate limit", 5)
		return
	}

	sid := chi.URLParam(r, "id")
	portStr := chi.URLParam(r, "port")
	subpath := chi.URLParam(r, "*")

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid port")
		return
	}
	// Narrow the allowed preview ports. We only expose the Capstone server
	// today; any other listening port in the VM could belong to something
	// the learner should not be able to reach via the public API.
	if port != 3000 {
		apiError(w, r, http.StatusForbidden, apitypes.ErrBadRequest, "preview port not allowed")
		return
	}

	row, err := p.sessionForOwner(r, sid, id)
	if err != nil {
		p.writeSessionErr(w, r, err)
		return
	}
	vmIP := ""
	if row.VMIP.Valid {
		vmIP = row.VMIP.String
	}
	if vmIP == "" {
		apiError(w, r, http.StatusGone, apitypes.ErrSessionReaped, "vm has no ip")
		return
	}
	live, ok := p.deps.Host.Get(row.ID.String())
	if !ok || live == nil {
		apiError(w, r, http.StatusGone, apitypes.ErrSessionReaped, "session not live")
		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(vmIP, portStr),
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Rewrite the path: strip the /sessions/:id/preview/:port prefix
			// so the VM sees the subpath only. Catch-all is empty for "/".
			rewritten := "/" + strings.TrimPrefix(subpath, "/")
			if rewritten == "//" {
				rewritten = "/"
			}
			req.URL.Path = rewritten
			req.URL.RawPath = rewritten
			req.Host = target.Host

			// Strip identity cookies before forwarding. The VM must not see
			// our session cookies (they carry identity across example.com
			// subdomains) and python http.server has no use for them anyway.
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			// Drop a couple of headers a nosy mini-app could try to probe.
			req.Header.Del("X-Forwarded-For")
			req.Header.Del("X-Real-IP")
		},
		ModifyResponse: func(resp *http.Response) error {
			// Let the mini-app be framed by learn.example.com only. Override
			// anything a smart app might have set (python http.server does
			// not emit CSP headers, but a future framework might).
			resp.Header.Set("Content-Security-Policy",
				"frame-ancestors 'self' https://learn.example.com http://localhost:3000")
			resp.Header.Del("X-Frame-Options")
			return nil
		},
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   3 * time.Second,
				KeepAlive: 15 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			DisableCompression:    true,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			p.deps.Logger.Warn("preview proxy err",
				"session", sid, "port", port, "path", r.URL.Path, "err", err)
			// Map connection refusal to a specific status so the frontend
			// can distinguish "app not ready yet" from "real failure".
			if isConnRefused(err) {
				http.Error(w, "preview not ready", http.StatusBadGateway)
				return
			}
			http.Error(w, "preview unavailable", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

func (p *previewHandler) sessionForOwner(r *http.Request, sid string, ownerID uuid.UUID) (*db.Session, error) {
	parsed, err := uuid.Parse(sid)
	if err != nil {
		return nil, err
	}
	row, err := p.deps.Store.GetSession(r.Context(), parsed)
	if err != nil {
		return nil, err
	}
	if row.IdentityUUID != ownerID {
		return nil, db.ErrNotFound
	}
	return row, nil
}

func (p *previewHandler) writeSessionErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apiError(w, r, http.StatusNotFound, apitypes.ErrVMNotFound, "session not found")
	default:
		p.deps.Logger.Error("preview session lookup", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
	}
}

func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return false
	}
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connect: no route") ||
		errors.Is(err, io.EOF)
}
