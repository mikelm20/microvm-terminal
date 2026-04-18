package session

import (
	"time"
)

// HeartbeatStatus is what GET /sessions/:id/heartbeat returns. Derived from
// the session's in-memory state plus `last_event_at` recorded by the
// Transcript.
type HeartbeatStatus struct {
	Alive       bool
	ClaudeBusy  bool
	LastEventAt time.Time
	AgeSeconds  int
}

// ClaudeBusy is flipped via the claude_busy event emitted by claude-wrap.
// The field is stored in the Session for cheap reads by the HTTP handler.
// Accessors are goroutine-safe via the EventBus serialization.
func (s *Session) SetClaudeBusy(v bool) {
	s.claudeBusy.Store(v)
}

func (s *Session) GetClaudeBusy() bool {
	return s.claudeBusy.Load()
}
