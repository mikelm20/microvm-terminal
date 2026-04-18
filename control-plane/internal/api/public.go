package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

type publicHandler struct {
	deps Deps
}

// Profile handles GET /p/:uuid/public. No auth. Returns a curated subset of
// the identity + completed modules for the public web proof page.
func (h *publicHandler) Profile(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "uuid")
	id, err := uuid.Parse(raw)
	if err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid uuid")
		return
	}
	ident, err := h.deps.Store.GetIdentity(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			apiError(w, r, http.StatusNotFound, apitypes.ErrNotFound, "profile not found")
			return
		}
		h.deps.Logger.Error("get identity", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	streak, xp, _ := progressStats(r.Context(), h.deps.Store, id)

	var name, dept *string
	if ident.Name.Valid {
		s := ident.Name.String
		name = &s
	}
	if ident.Department.Valid {
		s := ident.Department.String
		dept = &s
	}

	// Populate completed modules from certificates (authoritative public record).
	certs, err := h.deps.Store.ListCertificatesByIdentity(r.Context(), id)
	if err != nil {
		h.deps.Logger.Error("list certs", "err", err)
	}
	out := apitypes.PublicProfileResponse{
		UUID:       ident.UUID.String(),
		Name:       name,
		Department: dept,
		Lang:       apitypes.Lang(ident.Lang),
		StreakDays: streak,
		TotalXP:    xp,
	}
	for _, c := range certs {
		out.ModulesCompleted = append(out.ModulesCompleted, struct {
			LessonID    string   `json:"lesson_id"`
			Title       string   `json:"title"`
			CompletedAt string   `json:"completed_at"`
			Takeaways   []string `json:"takeaways"`
		}{
			LessonID:    c.LessonID,
			Title:       c.LessonID, // public page resolves title from voice table
			CompletedAt: c.CompletedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Takeaways:   []string{},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// Certificate handles GET /certificate/:id/public.
func (h *publicHandler) Certificate(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "id")
	cert, err := h.deps.Store.GetCertificate(r.Context(), cid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			apiError(w, r, http.StatusNotFound, apitypes.ErrNotFound, "certificate not found")
			return
		}
		h.deps.Logger.Error("get cert", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	ident, err := h.deps.Store.GetIdentity(r.Context(), cert.IdentityUUID)
	if err != nil {
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	var name, dept *string
	if ident.Name.Valid {
		s := ident.Name.String
		name = &s
	}
	if ident.Department.Valid {
		s := ident.Department.String
		dept = &s
	}
	writeJSON(w, http.StatusOK, apitypes.PublicCertificateResponse{
		ID:          cert.ID,
		UUID:        cert.IdentityUUID.String(),
		Name:        name,
		Department:  dept,
		Lang:        apitypes.Lang(ident.Lang),
		CompletedAt: cert.CompletedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Signature:   cert.Signature,
		Modules:     []string{cert.LessonID},
	})
}
