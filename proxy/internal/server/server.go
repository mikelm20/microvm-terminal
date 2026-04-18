// Package server wires the proxy HTTP handlers together. It accepts requests
// from VMs on :8443, authenticates the session token, checks quota, forwards
// to api.anthropic.com with streaming preserved, and writes audit rows.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mikelm20/learn-platform/proxy/internal/audit"
	"github.com/mikelm20/learn-platform/proxy/internal/keys"
	"github.com/mikelm20/learn-platform/proxy/internal/quota"
)

// Upstream defaults to Anthropic; overridden in tests.
const defaultUpstream = "https://api.anthropic.com"

// Config wires all dependencies the Server needs.
type Config struct {
	// Upstream is the base URL of the Anthropic API. Defaults to
	// https://api.anthropic.com when empty.
	Upstream string
	// Pool holds the Anthropic API keys and the session token mapping.
	Pool *keys.Pool
	// Quota enforces per-session caps. Safe for concurrent use.
	Quota *quota.Manager
	// Audit receives one Record per forwarded request. Writes happen on a
	// best-effort async goroutine so slow database writes never stall a
	// learner's stream. A failure is logged, never fatal.
	Audit audit.Sink
	// AdminToken gates POST /admin/rotate. Loaded from Doppler
	// (PROXY_ADMIN_TOKEN). Empty string disables the endpoint (tests).
	AdminToken string
	// HTTPClient is used to talk to the upstream. Tests inject a fake that
	// points at an httptest.Server.
	HTTPClient *http.Client
	// Logger is a structured logger; defaults to slog.Default().
	Logger *slog.Logger
}

// Server is the HTTP handler assembly.
type Server struct {
	cfg      Config
	upstream *url.URL
	mux      *http.ServeMux
	// inflight tracks forward handler count. Gated by mu so we can wait for
	// a stable zero during admin rotate without racing the WaitGroup
	// semantics (Add concurrent with Wait is formally unsafe).
	mu       sync.Mutex
	inflight int
	drained  *sync.Cond
}

// New constructs a ready-to-serve Server.
func New(cfg Config) (*Server, error) {
	if cfg.Upstream == "" {
		cfg.Upstream = defaultUpstream
	}
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("parse upstream: %w", err)
	}
	if cfg.Pool == nil {
		return nil, errors.New("server: nil Pool")
	}
	if cfg.Quota == nil {
		return nil, errors.New("server: nil Quota")
	}
	if cfg.Audit == nil {
		cfg.Audit = audit.NewInMemorySink()
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			// Long enough for slow streaming responses but not infinite. A
			// learner who idles for 10 minutes mid-turn is dead.
			Timeout: 10 * time.Minute,
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{cfg: cfg, upstream: u, mux: http.NewServeMux()}
	s.drained = sync.NewCond(&s.mu)
	s.routes()
	return s, nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/v1/", s.handleForward)
	s.mux.HandleFunc("/admin/rotate", s.handleAdminRotate)
	s.mux.HandleFunc("/admin/mint", s.handleAdminMint)
	s.mux.HandleFunc("/healthz", s.handleHealth)
}

// handleHealth is an unauthenticated liveness probe.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleForward proxies a single /v1/* request to Anthropic.
func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.inflight++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inflight--
		if s.inflight == 0 {
			s.drained.Broadcast()
		}
		s.mu.Unlock()
	}()

	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = uuid.New().String()
	}
	w.Header().Set("X-Request-Id", reqID)

	sessionToken, ok := bearerToken(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "auth_required", "missing bearer token", 0, reqID)
		return
	}
	secret, keyID, err := s.cfg.Pool.SecretForSession(sessionToken)
	if err != nil {
		switch {
		case errors.Is(err, keys.ErrUnknownSession):
			s.writeError(w, http.StatusUnauthorized, "auth_invalid", "unknown session token", 0, reqID)
		case errors.Is(err, keys.ErrKeyRevoked):
			s.writeError(w, http.StatusServiceUnavailable, "internal", "session key revoked", 5, reqID)
		default:
			s.writeError(w, http.StatusInternalServerError, "internal", "key resolve failed", 0, reqID)
		}
		return
	}
	// Operator-visible routing hint. Never the secret itself; just the ID
	// so rotations can be traced in access logs.
	w.Header().Set("X-Proxy-Key-Id", keyID)

	// We need to know the session ID for quota + audit. The control plane
	// issues tokens of the form sess_<hex>; sessionID is the token.
	sessionID := sessionToken

	if err := s.cfg.Quota.Reserve(sessionID); err != nil {
		var qerr *quota.Error
		if errors.As(err, &qerr) {
			retry := int(qerr.RetryAfter.Seconds())
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retry))
			}
			s.writeError(w, http.StatusTooManyRequests, "rate_limited", string(qerr.Code), retry, reqID)
			s.emitAudit(audit.Record{
				TS:        time.Now(),
				SessionID: sessionID,
				Endpoint:  r.URL.Path,
				Status:    http.StatusTooManyRequests,
			})
			return
		}
		s.writeError(w, http.StatusInternalServerError, "internal", "quota reserve failed", 0, reqID)
		return
	}

	// Snapshot the request body so we can size it for audit even when the
	// upstream response indicates a hard failure. Keep memory bounded: 1 MiB
	// is far more than any learner prompt will ever be, and the Composer
	// caps input at 4096 chars anyway.
	var reqBody bytes.Buffer
	if r.Body != nil {
		defer r.Body.Close()
		if _, err := io.Copy(&reqBody, io.LimitReader(r.Body, 1<<20)); err != nil {
			s.cfg.Quota.Refund(sessionID)
			s.writeError(w, http.StatusBadRequest, "bad_request", "read body: "+err.Error(), 0, reqID)
			return
		}
	}

	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, s.upstream.String()+r.URL.Path, bytes.NewReader(reqBody.Bytes()))
	if err != nil {
		s.cfg.Quota.Refund(sessionID)
		s.writeError(w, http.StatusInternalServerError, "internal", "build upstream request", 0, reqID)
		return
	}
	copyForwardHeaders(upReq.Header, r.Header)
	upReq.Header.Set("x-api-key", secret)
	upReq.Header.Set("anthropic-version", firstNonEmpty(r.Header.Get("anthropic-version"), "2023-06-01"))

	upResp, err := s.cfg.HTTPClient.Do(upReq)
	if err != nil {
		s.cfg.Quota.Refund(sessionID)
		s.writeError(w, http.StatusBadGateway, "internal", "upstream error: "+err.Error(), 0, reqID)
		s.emitAudit(audit.Record{
			TS:        time.Now(),
			SessionID: sessionID,
			Endpoint:  r.URL.Path,
			Status:    http.StatusBadGateway,
		})
		return
	}
	defer upResp.Body.Close()

	// Copy upstream headers to the client before we start streaming the
	// body. Content-Type will be text/event-stream for /v1/messages when
	// stream=true, application/json otherwise. Either way the wire shape is
	// preserved.
	for k, vv := range upResp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(upResp.StatusCode)

	flusher, _ := w.(http.Flusher)
	isSSE := strings.HasPrefix(upResp.Header.Get("Content-Type"), "text/event-stream")

	usage, bodyBytes, copyErr := streamBody(w, upResp.Body, flusher, isSSE)

	// Commit quota with observed usage (zeroes on error are fine; Anthropic
	// will not charge us for errors, so cost is zero).
	s.cfg.Quota.Commit(sessionID, usage.RequestTokens, usage.ResponseTokens)

	cost := estimateCostMicros(usage)
	s.emitAudit(audit.Record{
		TS:             time.Now(),
		SessionID:      sessionID,
		Endpoint:       r.URL.Path,
		RequestTokens:  usage.RequestTokens,
		ResponseTokens: usage.ResponseTokens,
		CostUSDMicros:  cost,
		Status:         upResp.StatusCode,
	})

	if copyErr != nil {
		s.cfg.Logger.Warn("stream copy error", "err", copyErr, "request_id", reqID)
	}
	_ = bodyBytes // reserved for debug; not logged to avoid PII blast radius.
}

// handleAdminRotate drains in-flight requests then swaps keys. Body is a
// JSON array of keys.Key, same shape as the Doppler secret.
func (s *Server) handleAdminRotate(w http.ResponseWriter, r *http.Request) {
	if !s.checkAdmin(w, r) {
		return
	}
	var next []keys.Key
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(next) == 0 {
		http.Error(w, "cannot rotate to empty pool", http.StatusBadRequest)
		return
	}
	// Drain: wait for in-flight forwards to complete before the swap so the
	// new pool is live only for new requests. 30s cap so a hung upstream
	// cannot wedge rotation forever.
	done := make(chan struct{})
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for s.inflight > 0 {
			s.drained.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	s.cfg.Pool.Replace(next)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleAdminMint lets the control plane obtain a fresh session token for a
// new VM. Body: {"session_id": "..."}. Returns {"token": "sess_..."}.
func (s *Server) handleAdminMint(w http.ResponseWriter, r *http.Request) {
	if !s.checkAdmin(w, r) {
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	tok, err := s.cfg.Pool.MintSessionToken(body.SessionID)
	if err != nil {
		http.Error(w, "mint: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

func (s *Server) checkAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AdminToken == "" {
		http.Error(w, "admin disabled", http.StatusNotFound)
		return false
	}
	tok, _ := bearerToken(r)
	if tok != s.cfg.AdminToken {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// writeError emits an ApiError-shaped JSON body matching
// shared/api/errors.ts. No stack traces, no internal details leak.
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string, retryAfter int, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{
		"code":       code,
		"message":    message,
		"request_id": reqID,
	}
	if retryAfter > 0 {
		body["retry_after_seconds"] = retryAfter
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) emitAudit(r audit.Record) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	go func() {
		defer cancel()
		if err := s.cfg.Audit.Write(ctx, r); err != nil {
			s.cfg.Logger.Warn("audit write failed", "err", err, "session_id", r.SessionID)
		}
	}()
}

// bearerToken extracts the `Authorization: Bearer <token>` header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), true
}

// hop-by-hop headers and the auth pair we replace are stripped before
// forwarding.
var strippedHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Authorization":       {},
	"X-Api-Key":           {},
}

func copyForwardHeaders(dst, src http.Header) {
	for k, vv := range src {
		if _, skip := strippedHeaders[http.CanonicalHeaderKey(k)]; skip {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
