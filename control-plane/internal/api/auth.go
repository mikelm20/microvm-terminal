package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/mail"
)

type authHandler struct {
	deps Deps
}

// RequestMagicLink handles POST /auth/magic-link. Rate-limited per email
// and per source IP. Always responds with `ok: true` to avoid leaking which
// emails exist. On success (or when the user exists) an email is delivered
// with a signed link pointing to the app's claim flow.
func (h *authHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	var req apitypes.RequestMagicLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !strings.Contains(email, "@") {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid email")
		return
	}
	if !apitypes.ValidLang(string(req.Lang)) {
		req.Lang = apitypes.LangES
	}

	if h.deps.MagicLimiter != nil && !h.deps.MagicLimiter.Allow(email) {
		apiErrorRetry(w, r, http.StatusTooManyRequests, apitypes.ErrRateLimited, "rate limited", 60)
		return
	}
	if h.deps.IPLimiter != nil && !h.deps.IPLimiter.Allow(r.RemoteAddr) {
		apiErrorRetry(w, r, http.StatusTooManyRequests, apitypes.ErrRateLimited, "rate limited", 60)
		return
	}

	var anon *uuid.UUID
	if req.AnonymousUUID != nil && *req.AnonymousUUID != "" {
		parsed, err := uuid.Parse(*req.AnonymousUUID)
		if err == nil {
			anon = &parsed
		}
	}

	token, ttl, err := auth.CreateMagicLink(r.Context(), h.deps.Store, email, string(req.Lang), anon)
	if err != nil {
		h.deps.Logger.Error("create magic link", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	link := h.deps.PublicOrigin + "/auth/callback?token=" + token
	subject, text, html := mail.MagicLinkEmail(string(req.Lang), link)
	if err := h.deps.Mail.Send(r.Context(), email, subject, text, html); err != nil {
		h.deps.Logger.Error("send magic link", "err", err)
		// Don't leak: still return ok, but log. In dev the StdoutSender never errors.
	}

	writeJSON(w, http.StatusOK, apitypes.RequestMagicLinkResponse{
		OK:               true,
		ExpiresInSeconds: int(ttl.Seconds()),
	})
}

// Claim handles POST /auth/claim. Consumes a magic-link token, creates or
// finds the matching identity, migrates anonymous progress if supplied,
// and mints a post-claim session cookie.
func (h *authHandler) Claim(w http.ResponseWriter, r *http.Request) {
	var req apitypes.ClaimAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	if req.MagicLinkToken == "" {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "missing token")
		return
	}

	email, anonFromLink, lang, err := auth.ConsumeMagicLink(r.Context(), h.deps.Store, req.MagicLinkToken)
	if err != nil {
		if errors.Is(err, db.ErrExpired) || errors.Is(err, db.ErrNotFound) {
			apiError(w, r, http.StatusUnauthorized, apitypes.ErrMagicLinkExpired, "magic link expired or invalid")
			return
		}
		h.deps.Logger.Error("consume magic link", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	// Prefer the anonymous_uuid from the body (fresh intent) over the one
	// baked into the magic link when they differ.
	var anon *uuid.UUID
	if req.AnonymousUUID != nil && *req.AnonymousUUID != "" {
		parsed, err := uuid.Parse(*req.AnonymousUUID)
		if err == nil {
			anon = &parsed
		}
	}
	if anon == nil && anonFromLink != nil {
		anon = anonFromLink
	}

	identityID, err := h.deps.Store.ClaimIdentity(r.Context(), anon, email, lang)
	if err != nil {
		h.deps.Logger.Error("claim identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	sessionToken, err := auth.MintSession(r.Context(), h.deps.Store, identityID)
	if err != nil {
		h.deps.Logger.Error("mint session", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	// Set both cookies: the authenticated session and a refreshed identity
	// cookie pointing to the claimed UUID.
	http.SetCookie(w, auth.SessionCookie(sessionToken, h.deps.SecureCookies))
	http.SetCookie(w, h.deps.Signer.IdentityHTTPCookie(identityID, h.deps.SecureCookies))

	writeJSON(w, http.StatusOK, apitypes.ClaimAccountResponse{
		UUID:         identityID.String(),
		Email:        email,
		SessionToken: sessionToken,
	})
}

// Logout clears the session cookie and revokes the server-side token.
func (h *authHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.LearnSessionCookie); err == nil && c.Value != "" {
		_ = h.deps.Store.DeleteAuthSession(r.Context(), c.Value)
	}
	http.SetCookie(w, auth.ClearSessionCookie(h.deps.SecureCookies))
	writeJSON(w, http.StatusOK, apitypes.OKResponse{OK: true})
}
