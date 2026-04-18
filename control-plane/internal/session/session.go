package session

import (
	"net/netip"
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
}

// CreatedAt returns the session's launch time.
func (s *Session) CreatedAt() time.Time { return s.createdAt }
