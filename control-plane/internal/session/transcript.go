package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

// Transcript persists every event from a Session's EventBus to the
// `session_events` table and updates `sessions.last_event_at` on each.
//
// The consumer side of the transcript (history replay + live tail on WS)
// is served by the HTTP handler via store.GetSessionEvents and a fresh
// EventBus subscription.
type Transcript struct {
	store  *db.Store
	logger *slog.Logger
}

func NewTranscript(store *db.Store, logger *slog.Logger) *Transcript {
	return &Transcript{store: store, logger: logger}
}

// Start spawns a goroutine that subscribes to the session's EventBus,
// appends each event to Postgres, and exits when ctx is cancelled.
// Safe to call once per Session.
func (t *Transcript) Start(ctx context.Context, sessionID uuid.UUID, bus *EventBus) {
	// Subscribe first so we catch every post-history event.
	_, ch, detach := bus.Subscribe(128)
	go func() {
		defer detach()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				t.persist(ctx, sessionID, ev)
			}
		}
	}()
}

func (t *Transcript) persist(ctx context.Context, sessionID uuid.UUID, ev GuestEvent) {
	ts := parseTS(ev.TS)
	payload, err := json.Marshal(ev)
	if err != nil {
		t.logger.Warn("marshal event", "err", err, "session", sessionID, "type", ev.Type)
		return
	}
	if err := t.store.InsertSessionEvent(ctx, sessionID, ts, ev.Type, payload); err != nil {
		t.logger.Warn("insert session_event", "err", err, "session", sessionID, "type", ev.Type)
	}
	if err := t.store.TouchSession(ctx, sessionID); err != nil {
		t.logger.Debug("touch session", "err", err, "session", sessionID)
	}
}

func parseTS(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}
