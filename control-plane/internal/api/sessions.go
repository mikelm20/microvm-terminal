package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

type sessionsHandler struct {
	deps Deps
}

// Create handles POST /sessions. When `warm: true` the server first tries
// AcquireWarm() on the pool; on miss it falls through to a cold Create().
func (h *sessionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	id, ok := identity.FromContext(r.Context())
	if !ok {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	var req apitypes.CreateSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	if req.LessonID == "" {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "missing lesson_id")
		return
	}
	if !apitypes.ValidLang(string(req.Lang)) {
		req.Lang = apitypes.LangES
	}

	wantWarm := req.Warm != nil && *req.Warm

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	var s *session.Session
	var err error
	if wantWarm {
		s, err = h.deps.Host.CreateWarm(ctx)
	} else {
		s, err = h.deps.Host.Create(ctx)
	}
	if err != nil {
		if errors.Is(err, session.ErrAtCapacity) {
			apiErrorRetry(w, r, http.StatusServiceUnavailable, apitypes.ErrCapacityFull, "capacity full", 30)
			return
		}
		h.deps.Logger.Error("create session", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrVMLaunchFailed, "vm launch failed")
		return
	}

	s.IdentityUUID = id
	s.LessonID = req.LessonID
	s.Lang = string(req.Lang)

	ipStr := ""
	if s.IP.IsValid() {
		ipStr = s.IP.String()
	}
	sid, perr := uuid.Parse(s.ID)
	if perr != nil {
		sid = uuid.New()
		s.ID = sid.String()
	}
	row := db.Session{
		ID:           sid,
		IdentityUUID: id,
		LessonID:     req.LessonID,
		Lang:         string(req.Lang),
		Warm:         s.Warm,
	}
	if ipStr != "" {
		row.VMIP = sql.NullString{String: ipStr, Valid: true}
	}
	if err := h.deps.Store.InsertSession(r.Context(), row); err != nil {
		h.deps.Logger.Error("insert session row", "err", err)
		_ = h.deps.Host.Destroy(s.ID)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	if h.deps.Transcript != nil {
		h.deps.Transcript.Start(context.Background(), sid, s.Events)
	}

	s.Events.Publish(session.GuestEvent{
		Type: "session_started",
		TS:   time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"session_id": s.ID,
			"lesson_id":  req.LessonID,
		},
	})

	origin := strings.TrimRight(h.deps.PublicOrigin, "/")
	wsOrigin := strings.Replace(origin, "http://", "ws://", 1)
	wsOrigin = strings.Replace(wsOrigin, "https://", "wss://", 1)

	writeJSON(w, http.StatusOK, apitypes.CreateSessionResponse{
		SessionID:          s.ID,
		VMIP:               ipStr,
		PTYWSURL:           wsOrigin + "/sessions/" + s.ID + "/pty",
		WizardWSURL:        wsOrigin + "/sessions/" + s.ID + "/ws",
		PreviewURLTemplate: origin + "/sessions/" + s.ID + "/preview/{port}",
	})
}

// Describe handles GET /sessions/:id.
func (h *sessionsHandler) Describe(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":    row.ID.String(),
		"lesson_id":     row.LessonID,
		"lang":          row.Lang,
		"vm_ip":         nullStrVal(row.VMIP),
		"warm":          row.Warm,
		"created_at":    row.CreatedAt.UTC().Format(time.RFC3339),
		"ready_at":      nullTime(row.ReadyAt),
		"reaped_at":     nullTime(row.ReapedAt),
		"last_event_at": nullTime(row.LastEventAt),
	})
}

// Heartbeat handles GET /sessions/:id/heartbeat.
func (h *sessionsHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	live, _ := h.deps.Host.Get(sid)
	alive := live != nil && !row.ReapedAt.Valid
	busy := false
	if live != nil {
		busy = live.GetClaudeBusy()
	}
	last := row.CreatedAt
	if row.LastEventAt.Valid {
		last = row.LastEventAt.Time
	}
	age := int(time.Since(last).Seconds())
	writeJSON(w, http.StatusOK, apitypes.HeartbeatResponse{
		SessionID:   row.ID.String(),
		VMIP:        nullStrVal(row.VMIP),
		Alive:       alive,
		ClaudeBusy:  busy,
		LastEventAt: last.UTC().Format(time.RFC3339),
		AgeSeconds:  age,
	})
}

// Transcript handles GET /sessions/:id/transcript.
func (h *sessionsHandler) Transcript(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	events, err := h.deps.Store.GetSessionEvents(r.Context(), row.ID)
	if err != nil {
		h.deps.Logger.Error("get events", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	payloads := make([]json.RawMessage, 0, len(events))
	for _, e := range events {
		payloads = append(payloads, json.RawMessage(e.Payload))
	}
	writeJSON(w, http.StatusOK, apitypes.TranscriptResponse{
		SessionID: row.ID.String(),
		Events:    payloads,
	})
}

// SubmitPrompt handles POST /sessions/:id/prompt.
// Requires Idempotency-Key header (UUID). Repeated calls with the same key
// return the same turn_id.
func (h *sessionsHandler) SubmitPrompt(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	idempotency := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotency == "" {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "missing Idempotency-Key")
		return
	}
	var req apitypes.SubmitPromptRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	if req.Text == "" || len(req.Text) > 4096 {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid text")
		return
	}

	newTurn := uuid.NewString()
	got, err := h.deps.Store.UpsertPromptIdempotency(r.Context(), row.ID, idempotency, newTurn)
	if err != nil {
		h.deps.Logger.Error("upsert idempotency", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	turn := got

	if turn == newTurn {
		// First time we've seen this idempotency key; publish the event.
		live, _ := h.deps.Host.Get(row.ID.String())
		if live != nil {
			live.Events.Publish(session.GuestEvent{
				Type: "claude_prompt_sent",
				TS:   time.Now().UTC().Format(time.RFC3339Nano),
				Payload: map[string]any{
					"text":    req.Text,
					"length":  len(req.Text),
					"turn_id": turn,
				},
			})
		}
		h.deps.Logger.Info("prompt submitted", "session", row.ID, "turn", turn, "bytes", len(req.Text))
	}

	writeJSON(w, http.StatusOK, apitypes.SubmitPromptResponse{Ok: true, TurnID: turn})
}

// Attach handles POST /sessions/:id/attach (multipart).
func (h *sessionsHandler) Attach(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "bad multipart")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "missing image")
		return
	}
	defer file.Close()

	base := filepath.Join(os.TempDir(), "learn-inbox", row.ID.String())
	if err := os.MkdirAll(base, 0o755); err != nil {
		h.deps.Logger.Error("mkdir inbox", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	safe := safeName(header.Filename)
	out := filepath.Join(base, safe)
	dst, err := os.Create(out)
	if err != nil {
		h.deps.Logger.Error("create inbox file", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		h.deps.Logger.Error("copy inbox file", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}
	writeJSON(w, http.StatusOK, apitypes.AttachImageResponse{InboxPath: "/inbox/" + safe})
}

// Delete handles DELETE /sessions/:id.
func (h *sessionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	if err := h.deps.Host.Destroy(row.ID.String()); err != nil && !errors.Is(err, session.ErrNotFound) {
		h.deps.Logger.Error("destroy session", "err", err)
	}
	if err := h.deps.Store.MarkSessionReaped(r.Context(), row.ID); err != nil {
		h.deps.Logger.Error("mark reaped", "err", err)
	}
	writeJSON(w, http.StatusOK, apitypes.OKResponse{Ok: true})
}

// WS handles GET /sessions/:id/ws (wizard events). Replays persisted history
// from session_events, then tails new events from the in-memory bus.
func (h *sessionsHandler) WS(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	live, ok := h.deps.Host.Get(row.ID.String())

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.deps.Logger.Warn("ws accept", "err", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	history, err := h.deps.Store.GetSessionEvents(r.Context(), row.ID)
	if err != nil {
		h.deps.Logger.Warn("replay events", "err", err)
	}
	for _, e := range history {
		if werr := c.Write(ctx, websocket.MessageText, e.Payload); werr != nil {
			return
		}
	}

	if !ok || live == nil {
		return
	}

	_, updates, detach := live.Events.Subscribe(64)
	defer detach()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-updates:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if werr := c.Write(ctx, websocket.MessageText, b); werr != nil {
				return
			}
		}
	}
}

// PTY handles GET /sessions/:id/pty (raw bytes to/from VM serial).
func (h *sessionsHandler) PTY(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	sid := chi.URLParam(r, "id")
	row, err := h.sessionForOwner(r.Context(), sid, id)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	live, ok := h.deps.Host.Get(row.ID.String())
	if !ok || live == nil || live.Serial == nil {
		apiError(w, r, http.StatusGone, apitypes.ErrSessionReaped, "session not live")
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	replay, updates, detach := live.Serial.Attach()
	defer detach()

	if len(replay) > 0 {
		_ = c.Write(ctx, websocket.MessageBinary, replay)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-updates:
				if !ok {
					cancel()
					return
				}
				if werr := c.Write(ctx, websocket.MessageBinary, data); werr != nil {
					cancel()
					return
				}
			}
		}
	}()
	for {
		_, data, rerr := c.Read(ctx)
		if rerr != nil {
			return
		}
		if live.Process != nil {
			_, _ = live.Process.Stdin().Write(data)
		}
	}
}

// --- helpers ---

func (h *sessionsHandler) sessionForOwner(ctx context.Context, sid string, ownerID uuid.UUID) (*db.Session, error) {
	parsed, err := uuid.Parse(sid)
	if err != nil {
		return nil, err
	}
	row, err := h.deps.Store.GetSession(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if row.IdentityUUID != ownerID {
		return nil, db.ErrNotFound
	}
	return row, nil
}

func (h *sessionsHandler) writeSessionErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apiError(w, r, http.StatusNotFound, apitypes.ErrVMNotFound, "session not found")
	default:
		h.deps.Logger.Error("session lookup", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
	}
}

func nullStrVal(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullTime(v sql.NullTime) string {
	if v.Valid {
		return v.Time.UTC().Format(time.RFC3339)
	}
	return ""
}

func safeName(n string) string {
	n = filepath.Base(n)
	if n == "" || n == "." || n == "/" {
		return "image"
	}
	return n
}
