package session

import (
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/firecracker"
)

// Session is one running Firecracker VM plus its serial pipes.
type Session struct {
	ID           string
	IP           netip.Addr
	MAC          string
	Tap          string
	Hostname     string
	SessionToken string // injected via kernel cmdline; guest-agent hello must match

	// IdentityUUID is the learner the session belongs to. Set by the Manager
	// after allocation. WS/HTTP handlers gate access on this.
	IdentityUUID uuid.UUID

	// LessonID the session is serving. Informational; predicate evaluation
	// lives in internal/lesson (Agent-Spine).
	LessonID string

	// Lang is the session locale (mirrors lessons/<id>.<lang>.yml).
	Lang string

	Process *firecracker.Process
	VmDir   string

	// Serial drains Process.Stdout() into a ring buffer and broadcasts new bytes
	// to the currently-attached WebSocket client (if any).
	Serial *SerialTee

	// Events is a fan-out bus for guest-agent events over vsock. Wizard WS
	// subscribers receive from here.
	Events *EventBus

	// claudeBusy tracks the last claude_busy event. Reads via GetClaudeBusy().
	claudeBusy atomic.Bool

	// Warm reports whether this session was served from the warm pool.
	Warm bool

	createdAt time.Time
	mgr       *Manager
	done      chan struct{}
}

// CreatedAt returns the session's launch time.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// NewInMemorySession constructs a bare Session for tests and warm-pool entries
// that aren't backed by a real Firecracker Process.
func NewInMemorySession(id string) *Session {
	return &Session{
		ID:        id,
		Events:    NewEventBus(),
		createdAt: time.Now(),
		done:      make(chan struct{}),
	}
}

// Done returns a channel closed when the Session is destroyed.
func (s *Session) Done() <-chan struct{} { return s.done }

// CloseDone closes the done channel. Idempotent.
func (s *Session) CloseDone() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}
