package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

// GuestEvent is the frame shape the in-VM guest agent sends over vsock. The
// first frame must be a hello carrying the session token from the kernel
// command line; after that the agent only speaks when it has something to
// report (today: nothing, the channel is host-to-guest for resize).
type GuestEvent struct {
	Type         string         `json:"type"`
	TS           string         `json:"ts"`
	SessionToken string         `json:"session_token,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

// startVsockListener accepts connections on the per-VM vsock UDS, verifies
// the guest's handshake (session_token must match), marks the session ready
// and keeps the connection so the control plane can push resize frames.
func (m *Manager) startVsockListener(s *Session, udsPath string) {
	// Firecracker creates `<udsPath>_<port>` on the host when the guest dials
	// vsock port <port>. We bind a Unix listener there and accept.
	addr := udsPath + "_" + strconv.Itoa(guestVsockPort)
	_ = os.Remove(addr)

	ln, err := net.Listen("unix", addr)
	if err != nil {
		m.logger.Error("vsock listen", "addr", addr, "err", err)
		return
	}
	// The uds file must be accessible to the firecracker process.
	_ = os.Chmod(addr, 0o660)
	m.logger.Info("vm boot: vsock listener up", "session", s.ID, "addr", addr)

	go func() {
		defer ln.Close()
		defer os.Remove(addr)
		for {
			select {
			case <-s.done:
				return
			default:
			}
			// Short deadline so Accept wakes up and can notice session
			// shutdown, without a goroutine leak.
			if ul, ok := ln.(*net.UnixListener); ok {
				_ = ul.SetDeadline(time.Now().Add(2 * time.Second))
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
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
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
	s.MarkReady()
	m.logger.Info("session ready", "session", s.ID, "boot_ms", time.Since(s.CreatedAt()).Milliseconds())

	// A browser may have connected during boot and sent its geometry before
	// the guest was there to receive it. Replay it now so the tty is sized
	// before the login shell starts anything interactive.
	if cols, rows := s.WindowSize(); cols > 0 && rows > 0 {
		if err := s.SendToGuest(resizeMessage(cols, rows)); err != nil {
			m.logger.Warn("resize replay", "err", err, "session", s.ID)
		}
	}

	_ = conn.SetDeadline(time.Time{})
	for {
		var ev GuestEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				m.logger.Info("guest-agent disconnected", "session", s.ID)
			} else {
				m.logger.Warn("guest event decode", "err", err, "session", s.ID)
			}
			return
		}
		m.logger.Debug("guest event", "session", s.ID, "type", ev.Type)
	}
}
