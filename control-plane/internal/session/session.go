package session

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// VMProcess abstracts both firecracker.Process (direct launch) and
// vm.Process (jailed launch). Both shapes expose the same surface so the
// session layer can stay launcher-agnostic.
type VMProcess interface {
	Stdin() io.Writer
	Stdout() io.Reader
	Stop() error
	Wait() error
	VmDir() string
}

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

	Process VMProcess
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
	readyAt   atomic.Int64 // unix nanos; set when the guest-agent handshake completes
	ready     chan struct{}
	readyOnce sync.Once
	mgr       *Manager
	done      chan struct{}
	doneOnce  sync.Once

	// guestConn is the currently-attached vsock connection to the guest-agent,
	// if any. Use SendToGuest to write to it safely.
	guestMu   sync.Mutex
	guestConn net.Conn
}

// MarkReady records the moment the guest-agent handshake completed and
// unblocks any WaitReady waiters. Idempotent. Called by the vsock listener
// after the hello frame validates.
//
// Callers must have initialised s.ready (all the constructors in this
// package do). Panics if s.ready is nil to surface misuse loudly.
func (s *Session) MarkReady() {
	if s.ready == nil {
		// Defensive: make it non-nil so the panic path stays noisy but
		// the daemon does not crash on a single session init bug.
		s.ready = make(chan struct{})
	}
	s.readyOnce.Do(func() {
		s.readyAt.Store(time.Now().UnixNano())
		close(s.ready)
	})
}

// ReadyAt returns the time the session became ready, or the zero value if
// the handshake has not completed yet.
func (s *Session) ReadyAt() time.Time {
	n := s.readyAt.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// WaitReady blocks until MarkReady has been called or ctx is cancelled.
// Returns nil on ready, or ctx.Err() otherwise.
func (s *Session) WaitReady(ctx context.Context) error {
	if s.ready == nil {
		// NewInMemorySession path: no boot, no wait.
		return nil
	}
	select {
	case <-s.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return ErrSessionClosed
	}
}

// SetGuestConn stores the live guest vsock connection. Passing nil clears it.
// Called by the vsock listener on attach and on disconnect.
func (s *Session) SetGuestConn(c net.Conn) {
	s.guestMu.Lock()
	s.guestConn = c
	s.guestMu.Unlock()
}

// SendToGuest marshals msg as JSON and writes it on the guest connection.
// Returns an error if no guest is attached or the write fails. The guest-agent
// knows one type today, "config".
func (s *Session) SendToGuest(msg any) error {
	s.guestMu.Lock()
	c := s.guestConn
	s.guestMu.Unlock()
	if c == nil {
		return ErrNoGuest
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = c.Write(b)
	return err
}

// CreatedAt returns the session's launch time.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// NewInMemorySession constructs a bare Session for tests and warm-pool entries
// that aren't backed by a real Firecracker Process.
func NewInMemorySession(id string) *Session {
	s := &Session{
		ID:        id,
		Events:    NewEventBus(),
		createdAt: time.Now(),
		done:      make(chan struct{}),
		ready:     make(chan struct{}),
	}
	// In-memory sessions are ready the moment they exist.
	s.MarkReady()
	return s
}

// Done returns a channel closed when the Session is destroyed.
func (s *Session) Done() <-chan struct{} { return s.done }

// CloseDone closes the done channel. Idempotent and safe to call from
// several goroutines at once: Manager.Destroy, Host.Destroy and the reaper
// goroutine that watches the Firecracker process can all race to close it.
// A sync.Once (rather than a select on the channel) is what makes the
// concurrent case safe; the select form still double-closes when two callers
// pass the default branch before either closes.
func (s *Session) CloseDone() {
	if s.done == nil {
		return
	}
	s.doneOnce.Do(func() { close(s.done) })
}
