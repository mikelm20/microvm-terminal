package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
)

type identityHandler struct {
	deps Deps
}

// Mint handles POST /identity. If the body includes a UUID, the server
// touches `last_seen_at` and returns the same UUID with a refreshed cookie.
// Otherwise a new UUID is minted.
func (h *identityHandler) Mint(w http.ResponseWriter, r *http.Request) {
	var req apitypes.MintIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	if !apitypes.ValidLang(string(req.Lang)) {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid lang")
		return
	}

	var supplied *uuid.UUID
	if req.UUID != nil && *req.UUID != "" {
		parsed, err := uuid.Parse(*req.UUID)
		if err != nil {
			apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid uuid")
			return
		}
		supplied = &parsed
	}

	id, err := identity.MintOrReattest(r.Context(), h.deps.Store, supplied, string(req.Lang))
	if err != nil {
		h.deps.Logger.Error("mint identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	cookie := h.deps.Signer.IdentityHTTPCookie(id, h.deps.SecureCookies, h.deps.CookieDomain)
	http.SetCookie(w, cookie)

	writeJSON(w, http.StatusOK, apitypes.MintIdentityResponse{
		UUID:   id.String(),
		Cookie: cookie.Value,
	})
}
