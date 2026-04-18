package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
)

type progressHandler struct {
	deps Deps
}

func (h *progressHandler) Sync(w http.ResponseWriter, r *http.Request) {
	ctxID, haveID := identity.FromContext(r.Context())
	var req apitypes.ProgressSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	target, err := uuid.Parse(req.UUID)
	if err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid uuid")
		return
	}

	// Either the request UUID must match the identity carried on the request,
	// or the request has no identity (first-time-anon progress push; we
	// trust the body and proceed, relying on the anonymous UUID as the key).
	if haveID && ctxID != target {
		apiError(w, r, http.StatusForbidden, apitypes.ErrAuthInvalid, "uuid mismatch")
		return
	}

	// Make sure there's an identities row (anonymous UUIDs may post progress
	// before identity minting happened in this DB). Upsert defensively.
	if err := h.deps.Store.InsertOrTouchIdentity(r.Context(), target, "es"); err != nil {
		h.deps.Logger.Error("touch identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	body, err := json.Marshal(req.State)
	if err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "bad state")
		return
	}
	updated, err := h.deps.Store.UpsertProgress(r.Context(), target, body)
	if err != nil {
		h.deps.Logger.Error("upsert progress", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	writeJSON(w, http.StatusOK, apitypes.ProgressSyncResponse{
		State:           req.State,
		ServerUpdatedAt: updated.UTC().Format(time.RFC3339),
	})
}

func (h *progressHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	raw, updated, err := h.deps.Store.GetProgress(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Empty default.
			writeJSON(w, http.StatusOK, apitypes.ProgressSyncResponse{
				State: apitypes.ProgressState{
					Modules:        map[string]apitypes.ModuleProgress{},
					StreakDays:     0,
					GraceTokens:    3,
					LastVisited:    "",
					SeenCoachmarks: []string{},
					UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
				},
				ServerUpdatedAt: time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		h.deps.Logger.Error("get progress", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	var state apitypes.ProgressState
	if err := json.Unmarshal(raw, &state); err != nil {
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "corrupt state")
		return
	}
	writeJSON(w, http.StatusOK, apitypes.ProgressSyncResponse{
		State:           state,
		ServerUpdatedAt: updated.UTC().Format(time.RFC3339),
	})
}
