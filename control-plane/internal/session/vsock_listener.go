package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"time"
)

// A GuestEvent is what the in-VM guest-agent sends over vsock. The control
// plane re-broadcasts these to wizard WebSocket subscribers.
type GuestEvent struct {
	Type         string         `json:"type"`
	TS           string         `json:"ts"`
	SessionToken string         `json:"session_token,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

// startVsockListener accepts exactly one connection on the per-VM vsock UDS,
// verifies the guest's handshake (session_token must match), and forwards
// subsequent events to s.Events. The first message MUST be type="hello" with
// the correct session_token; otherwise the connection is closed.
//
// This is intentionally simple for MVP: one connection at a time. Guest-agent
// reconnects on drop.
func (m *Manager) startVsockListener(s *Session, udsPath string) {
	// Firecracker creates `<udsPath>_<port>` on the host when the guest dials
	// vsock port <port>. We bind a Unix listener there and accept.
	port := uint32(5555)
	addr := udsPathForPort(udsPath, port)
	// Remove stale socket from prior runs
	_ = os.Remove(addr)

	ln, err := net.Listen("unix", addr)
	if err != nil {
		m.logger.Error("vsock listen", "addr", addr, "err", err)
		return
	}
	// The uds file must be accessible to the firecracker process (running as root here).
	_ = os.Chmod(addr, 0o660)
	m.logger.Info("vm boot: vsock listener up",
		"session", s.ID, "addr", addr)
	s.Events.Publish(GuestEvent{Type: "vm_booting", TS: nowRFC3339(), Payload: map[string]any{"stage": "guest_agent"}})

	go func() {
		defer ln.Close()
		defer os.Remove(addr)
		for {
			// Stop the loop when the session is destroyed.
			select {
			case <-s.done:
				return
			default:
			}
			// Use a short deadline so Accept wakes up and can notice session
			// shutdown, without a goroutine leak.
			if tcp, ok := ln.(*net.UnixListener); ok {
				_ = tcp.SetDeadline(time.Now().Add(2 * time.Second))
			}
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				m.logger.Warn("vsock accept", "err", err, "session", s.ID)
				continue
			}
			go m.handleGuestConn(s, conn)
		}
	}()
}

func (m *Manager) handleGuestConn(s *Session, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	dec := json.NewDecoder(bufio.NewReader(conn))

	var hello GuestEvent
	if err := dec.Decode(&hello); err != nil {
		m.logger.Warn("vsock hello decode", "err", err, "session", s.ID)
		return
	}
	if hello.Type != "hello" || hello.SessionToken != s.SessionToken {
		m.logger.Warn("vsock hello rejected", "session", s.ID, "type", hello.Type, "token_match", hello.SessionToken == s.SessionToken)
		return
	}
	m.logger.Info("vm boot: guest-agent handshake complete",
		"session", s.ID, "hostname", hello.Payload["hostname"],
		"boot_elapsed_ms", time.Since(s.CreatedAt()).Milliseconds())
	s.SetGuestConn(conn)
	defer s.SetGuestConn(nil)

	// Session is now fully ready: publish, signal waiters, timestamp the db.
	s.Events.Publish(GuestEvent{Type: "agent_online", TS: nowRFC3339(), Payload: map[string]any{"hostname": hello.Payload["hostname"]}})
	s.Events.Publish(GuestEvent{Type: "vm_ready", TS: nowRFC3339()})
	s.MarkReady()
	m.logger.Info("session ready", "session", s.ID,
		"boot_ms", time.Since(s.CreatedAt()).Milliseconds())

	// Clear deadline and relay events
	_ = conn.SetDeadline(time.Time{})
	for {
		var ev GuestEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				m.logger.Info("guest-agent disconnected", "session", s.ID)
			} else {
				m.logger.Warn("guest event decode", "err", err, "session", s.ID)
			}
			s.Events.Publish(GuestEvent{Type: "agent_offline", TS: nowRFC3339()})
			return
		}
		// Strip the session token from events before broadcasting
		ev.SessionToken = ""
		s.Events.Publish(ev)
	}
}

// udsPathForPort appends `_<port>` to the Firecracker vsock socket path; this
// is the convention Firecracker uses to expose a specific guest port to the host.
func udsPathForPort(base string, port uint32) string {
	return base + "_" + u32ToString(port)
}

func u32ToString(n uint32) string {
	// Avoid strconv import clutter; 5555 is short anyway.
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }
