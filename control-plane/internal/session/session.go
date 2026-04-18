package session

import (
	"encoding/json"
	"net"
	"net/netip"
	"sync"
	"time"

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

	Process *firecracker.Process
	VmDir   string

	// Serial drains Process.Stdout() into a ring buffer and broadcasts new bytes
	// to the currently-attached WebSocket client (if any).
	Serial *SerialTee

	// Events is a fan-out bus for guest-agent events over vsock. Wizard WS
	// subscribers receive from here.
	Events *EventBus

	createdAt time.Time
	mgr       *Manager
	done      chan struct{}

	// guestConn is the currently-attached vsock connection to the guest-agent,
	// if any. Use SendToGuest to write to it safely.
	guestMu   sync.Mutex
	guestConn net.Conn
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
