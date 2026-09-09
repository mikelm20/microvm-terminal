package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
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

	// Owner is the login name the session belongs to. Set by the HTTP layer
	// right after creation; the PTY and delete handlers gate access on it.
	Owner string

	Process VMProcess
	VmDir   string

	// Serial drains Process.Stdout() into a ring buffer and broadcasts new bytes
	// to the currently-attached WebSocket client (if any).
	Serial *SerialTee

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

	// Terminal geometry last requested by a browser. The serial console has
	// no window size of its own, so the value is pushed to the guest agent,
	// which applies it to /dev/ttyS0 with TIOCSWINSZ.
	winMu sync.Mutex
	cols  uint16
	rows  uint16

	// Attach bookkeeping for the idle reaper.
	attachMu   sync.Mutex
	attached   int
	lastDetach time.Time
}

// MarkReady records the moment the guest-agent handshake completed and
// unblocks any WaitReady waiters. Idempotent. Called by the vsock listener
// after the hello frame validates.
func (s *Session) MarkReady() {
	if s.ready == nil {
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

// GuestMessage is the frame shape the control plane writes to the guest
// agent over vsock. Today the only type is "resize".
type GuestMessage struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

// SendToGuest marshals msg as JSON and writes it on the guest connection.
// Returns ErrNoGuest if no guest is attached.
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

// Resize records the terminal geometry and forwards it to the guest agent
// when one is attached. When the guest is not attached yet the value is kept
// and pushed by the vsock listener as soon as the handshake completes, so a
// browser that connects during boot still gets a correctly sized tty.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return errors.New("resize: cols and rows must be positive")
	}
	s.winMu.Lock()
	s.cols, s.rows = cols, rows
	s.winMu.Unlock()
	err := s.SendToGuest(resizeMessage(cols, rows))
	if errors.Is(err, ErrNoGuest) {
		return nil
	}
	return err
}

// WindowSize returns the last geometry requested by a client. Zero values
// mean no client has sent one yet.
func (s *Session) WindowSize() (cols, rows uint16) {
	s.winMu.Lock()
	defer s.winMu.Unlock()
	return s.cols, s.rows
}

func resizeMessage(cols, rows uint16) GuestMessage {
	return GuestMessage{Type: "resize", Payload: map[string]any{"cols": cols, "rows": rows}}
}

// Attach registers a terminal client. The returned function detaches it and
// stamps the moment the session became idle. The idle reaper only considers
// sessions with zero attached clients.
func (s *Session) Attach() func() {
	s.attachMu.Lock()
	s.attached++
	s.attachMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.attachMu.Lock()
			s.attached--
			if s.attached == 0 {
				s.lastDetach = time.Now()
			}
			s.attachMu.Unlock()
		})
	}
}

// Attached returns the number of terminal clients currently connected.
func (s *Session) Attached() int {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	return s.attached
}

// IdleSince reports when the session last lost its final client. For a
// session that never had a client it is the ready time, or the creation
// time if the guest never handshaked. ok is false while a client is attached.
func (s *Session) IdleSince() (t time.Time, ok bool) {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	if s.attached > 0 {
		return time.Time{}, false
	}
	if !s.lastDetach.IsZero() {
		return s.lastDetach, true
	}
	if r := s.ReadyAt(); !r.IsZero() {
		return r, true
	}
	return s.createdAt, true
}

// CreatedAt returns the session's launch time.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// NewInMemorySession constructs a bare Session for tests and warm-pool entries
// that aren't backed by a real Firecracker Process.
func NewInMemorySession(id string) *Session {
	s := &Session{
		ID:        id,
		createdAt: time.Now(),
		done:      make(chan struct{}),
		ready:     make(chan struct{}),
	}
	s.MarkReady()
	return s
}

// Done returns a channel closed when the Session is destroyed.
func (s *Session) Done() <-chan struct{} { return s.done }

// CloseDone closes the done channel. Idempotent and safe to call from
// several goroutines at once: Manager.Destroy, Host.Destroy and the reaper
// goroutine that watches the Firecracker process can all race to close it.
func (s *Session) CloseDone() {
	if s.done == nil {
		return
	}
	s.doneOnce.Do(func() { close(s.done) })
}
