package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
)

type meHandler struct {
	deps Deps
}

func (h *meHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	ident, err := h.deps.Store.GetIdentity(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			apiError(w, r, http.StatusNotFound, apitypes.ErrNotFound, "identity not found")
			return
		}
		h.deps.Logger.Error("get identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	streak, xp, grace := progressStats(r.Context(), h.deps.Store, id)
	writeJSON(w, http.StatusOK, toMeResponse(ident, streak, xp, grace))
}

func (h *meHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	var req apitypes.PatchMeRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	var langPtr *string
	if req.Lang != nil {
		if !apitypes.ValidLang(string(*req.Lang)) {
			apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid lang")
			return
		}
		s := string(*req.Lang)
		langPtr = &s
	}
	if req.Department != nil && !apitypes.ValidDepartment(*req.Department) {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid department")
		return
	}

	ident, err := h.deps.Store.PatchIdentity(r.Context(), id, langPtr, req.Department, req.Name, req.HapticsEnabled, req.PushEnabled)
	if err != nil {
		h.deps.Logger.Error("patch identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	streak, xp, grace := progressStats(r.Context(), h.deps.Store, id)
	writeJSON(w, http.StatusOK, toMeResponse(ident, streak, xp, grace))
}

// progressStats pulls denormalized totals from the progress jsonb blob.
// Missing row returns zeroes + default grace tokens (3).
func progressStats(ctx context.Context, store *db.Store, id uuid.UUID) (streak, xp, grace int) {
	state, _, err := store.GetProgress(ctx, id)
	if err != nil || state == nil {
		return 0, 0, 3
	}
	return extractStats(state)
}
