package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// PTY WebSocket keepalive. Browsers cannot send pings themselves, and NAT
// boxes drop idle streams after a couple of minutes, so the server pings.
// coder/websocket answers the pong for us and Ping returns when it lands.
const (
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second
)

type sessionsHandler struct {
	deps Deps
}

// Create handles POST /sessions: boot a VM for the caller.
func (h *sessionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.OwnerFromContext(r.Context())

	var req CreateSessionRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &req); err != nil {
			apiError(w, r, http.StatusBadRequest, ErrBadRequest, "invalid json")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	var s *session.Session
	var err error
	if req.Warm {
		s, err = h.deps.Host.CreateWarm(ctx)
	} else {
		s, err = h.deps.Host.Create(ctx)
	}
	if err != nil {
		if errors.Is(err, session.ErrAtCapacity) {
			apiErrorRetry(w, r, http.StatusServiceUnavailable, ErrCapacityFull, "all VM slots are busy", 30)
			return
		}
		h.deps.Logger.Error("create session", "err", err, "owner", owner)
		apiError(w, r, http.StatusInternalServerError, ErrVMLaunchFailed, "vm launch failed")
		return
	}
	s.Owner = owner

	sid, perr := uuid.Parse(s.ID)
	if perr != nil {
		sid = uuid.New()
		s.ID = sid.String()
	}
	ipStr := ""
	if s.IP.IsValid() {
		ipStr = s.IP.String()
	}
	row := db.Session{ID: sid, Owner: owner, Warm: s.Warm}
	if ipStr != "" {
		row.VMIP = sql.NullString{String: ipStr, Valid: true}
	}
	if err := h.deps.Store.InsertSession(r.Context(), row); err != nil {
		h.deps.Logger.Error("insert session row", "err", err)
		_ = h.deps.Host.Destroy(s.ID)
		apiError(w, r, http.StatusInternalServerError, ErrInternal, "internal")
		return
	}
	h.deps.Logger.Info("session booted", "id", s.ID, "owner", owner, "ip", ipStr, "warm", s.Warm)

	writeJSON(w, http.StatusOK, CreateSessionResponse{
		SessionID: s.ID,
		VMIP:      ipStr,
		PTYWSURL:  h.wsOrigin() + "/sessions/" + s.ID + "/pty",
		Warm:      s.Warm,
	})
}

// Current handles GET /sessions/current: the newest live VM the caller owns.
func (h *sessionsHandler) Current(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.OwnerFromContext(r.Context())
	var newest *session.Session
	for _, s := range h.deps.Host.List() {
		if s.Owner != owner {
			continue
		}
		if newest == nil || s.CreatedAt().After(newest.CreatedAt()) {
			newest = s
		}
	}
	if newest == nil {
		apiError(w, r, http.StatusNotFound, ErrVMNotFound, "no live session")
		return
	}
	writeJSON(w, http.StatusOK, liveInfo(newest))
}

// Describe handles GET /sessions/{id}. Live sessions come from memory;
// finished ones from the sessions table.
func (h *sessionsHandler) Describe(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.OwnerFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if live, ok := h.deps.Host.Get(id); ok && live.Owner == owner {
		writeJSON(w, http.StatusOK, liveInfo(live))
		return
	}
	row, err := h.rowForOwner(r.Context(), id, owner)
	if err != nil {
		h.writeSessionErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SessionInfo{
		SessionID: row.ID.String(),
		Owner:     row.Owner,
		VMIP:      nullStrVal(row.VMIP),
		Alive:     false,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		ReadyAt:   nullTime(row.ReadyAt),
		ReapedAt:  nullTime(row.ReapedAt),
	})
}

// Delete handles DELETE /sessions/{id}.
func (h *sessionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.OwnerFromContext(r.Context())
	id := chi.URLParam(r, "id")
	live, ok := h.deps.Host.Get(id)
	if !ok || live.Owner != owner {
		// Not live: still answer ok when the row belongs to the caller so a
		// stale client can finish its teardown flow.
		if _, err := h.rowForOwner(r.Context(), id, owner); err != nil {
			h.writeSessionErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, OKResponse{Ok: true})
		return
	}
	if err := h.deps.Host.Destroy(id); err != nil && !errors.Is(err, session.ErrNotFound) {
		h.deps.Logger.Error("destroy session", "err", err, "id", id)
	}
	writeJSON(w, http.StatusOK, OKResponse{Ok: true})
}

// PTY handles GET /sessions/{id}/pty. Binary frames carry terminal bytes in
// both directions; text frames from the client are JSON control messages
// (resize). On connect the server replays the serial ring buffer as one
// binary frame, then streams.
func (h *sessionsHandler) PTY(w http.ResponseWriter, r *http.Request) {
	owner, _ := auth.OwnerFromContext(r.Context())
	id := chi.URLParam(r, "id")
	live, ok := h.deps.Host.Get(id)
	if !ok || live.Owner != owner {
		apiError(w, r, http.StatusNotFound, ErrVMNotFound, "session not found")
		return
	}
	if live.Serial == nil || live.Process == nil {
		apiError(w, r, http.StatusGone, ErrSessionReaped, "session has no console")
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")
	// Terminal streams can be large (replay is up to 256 KiB) and a client
	// pasting a file is not an attack.
	c.SetReadLimit(1 << 20)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	detach := live.Attach()
	defer detach()
	if err := h.deps.Store.TouchSessionAttached(r.Context(), mustUUID(id)); err != nil {
		h.deps.Logger.Warn("touch session", "err", err, "id", id)
	}

	replay, updates, unsubscribe := live.Serial.Attach()
	defer unsubscribe()
	if len(replay) > 0 {
		if err := c.Write(ctx, websocket.MessageBinary, replay); err != nil {
			return
		}
	}

	// Serial -> client, plus VM exit and keepalive.
	go func() {
		ping := time.NewTicker(pingInterval)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-live.Done():
				_ = c.Close(websocket.StatusGoingAway, "vm exited")
				cancel()
				return
			case data, ok := <-updates:
				if !ok {
					cancel()
					return
				}
				if err := c.Write(ctx, websocket.MessageBinary, data); err != nil {
					cancel()
					return
				}
			case <-ping.C:
				pctx, pcancel := context.WithTimeout(ctx, pingTimeout)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					h.deps.Logger.Info("pty keepalive failed, dropping client", "id", id, "err", err)
					cancel()
					return
				}
			}
		}
	}()

	// Client -> serial.
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if _, err := live.Process.Stdin().Write(data); err != nil {
				return
			}
		case websocket.MessageText:
			var m ControlMessage
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			if m.Type == "resize" {
				if err := live.Resize(m.Cols, m.Rows); err != nil {
					h.deps.Logger.Debug("resize", "id", id, "err", err)
				}
			}
		}
	}
}

// --- helpers ---

func liveInfo(s *session.Session) SessionInfo {
	ip := ""
	if s.IP.IsValid() {
		ip = s.IP.String()
	}
	info := SessionInfo{
		SessionID: s.ID,
		Owner:     s.Owner,
		VMIP:      ip,
		Alive:     true,
		Attached:  s.Attached(),
		CreatedAt: s.CreatedAt().UTC().Format(time.RFC3339),
	}
	if t := s.ReadyAt(); !t.IsZero() {
		info.ReadyAt = t.UTC().Format(time.RFC3339)
	}
	return info
}

func (h *sessionsHandler) wsOrigin() string {
	origin := strings.TrimRight(h.deps.PublicOrigin, "/")
	origin = strings.Replace(origin, "http://", "ws://", 1)
	return strings.Replace(origin, "https://", "wss://", 1)
}

func (h *sessionsHandler) rowForOwner(ctx context.Context, id, owner string) (*db.Session, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, db.ErrNotFound
	}
	row, err := h.deps.Store.GetSession(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if row.Owner != owner {
		return nil, db.ErrNotFound
	}
	return row, nil
}

func (h *sessionsHandler) writeSessionErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		apiError(w, r, http.StatusNotFound, ErrVMNotFound, "session not found")
	default:
		h.deps.Logger.Error("session lookup", "err", err)
		apiError(w, r, http.StatusInternalServerError, ErrInternal, "internal")
	}
}

func mustUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
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
