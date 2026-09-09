// microvm-guest-agent runs inside every Firecracker VM. It has two jobs:
//
//  1. Readiness. It dials the host over vsock and sends a hello frame that
//     carries the session token from the kernel command line. The control
//     plane marks the session ready when the token matches.
//  2. Terminal geometry. The browser terminal is bridged over the serial
//     console, which has no window size of its own. The control plane sends
//     {"type":"resize","payload":{"cols":N,"rows":M}} frames on the same
//     vsock connection and the agent applies them to /dev/ttyS0 with
//     TIOCSWINSZ, which also delivers SIGWINCH to the foreground process.
//
// Kernel cmdline params consumed:
//
//	mvt.session_token=<hex>   proves our identity to the host
//	mvt.vsock_port=<int>      host-side vsock port we dial (default 5555)
//
// Frames are one JSON object per line in both directions.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/vsock"
	"golang.org/x/sys/unix"
)

var Version = "dev"

const (
	hostCID          uint32 = 2
	defaultVsockPort uint32 = 5555
	consoleDevice           = "/dev/ttyS0"
)

type frame struct {
	Type    string         `json:"type"`
	TS      string         `json:"ts,omitempty"`
	Session string         `json:"session_token,omitempty"` // only on hello
	Payload map[string]any `json:"payload,omitempty"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("microvm-guest-agent %s starting", Version)

	sessionToken, vsockPort := readCmdlineParams()
	if sessionToken == "" {
		log.Printf("no mvt.session_token on cmdline; the host will reject the handshake")
	}
	log.Printf("session_token=%s... vsock_port=%d", safePrefix(sessionToken), vsockPort)

	backoff := time.Second
	for {
		err := serve(sessionToken, vsockPort)
		log.Printf("vsock session ended: %v; redialing in %s", err, backoff)
		time.Sleep(backoff)
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// serve dials the host, sends hello, then applies control frames until the
// connection drops.
func serve(sessionToken string, port uint32) error {
	conn, err := vsock.Dial(hostCID, port, nil)
	if err != nil {
		return fmt.Errorf("dial host: %w", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(frame{
		Type:    "hello",
		TS:      nowRFC3339(),
		Session: sessionToken,
		Payload: map[string]any{"hostname": mustHostname(), "version": Version},
	}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	log.Printf("handshake sent")

	dec := json.NewDecoder(bufio.NewReader(conn))
	for {
		var f frame
		if err := dec.Decode(&f); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		switch f.Type {
		case "resize":
			cols, rows := uint16(numField(f.Payload, "cols")), uint16(numField(f.Payload, "rows"))
			if err := applyWinsize(cols, rows); err != nil {
				log.Printf("resize %dx%d: %v", cols, rows, err)
				continue
			}
			log.Printf("resize applied: %dx%d", cols, rows)
		default:
			log.Printf("ignoring frame type %q", f.Type)
		}
	}
}

// applyWinsize sets the window size on the serial console tty. The kernel
// stores it on the tty and signals SIGWINCH to the foreground process group,
// so bash, the menu and Claude Code all pick it up the same way they would
// on a real pty.
func applyWinsize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("cols and rows must be positive")
	}
	fd, err := unix.Open(consoleDevice, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", consoleDevice, err)
	}
	defer unix.Close(fd)
	return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: cols, Row: rows})
}

func numField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func readCmdlineParams() (sessionToken string, vsockPort uint32) {
	vsockPort = defaultVsockPort
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return "", vsockPort
	}
	for _, tok := range strings.Fields(string(data)) {
		switch {
		case strings.HasPrefix(tok, "mvt.session_token="):
			sessionToken = strings.TrimPrefix(tok, "mvt.session_token=")
		case strings.HasPrefix(tok, "mvt.vsock_port="):
			var n uint32
			fmt.Sscanf(strings.TrimPrefix(tok, "mvt.vsock_port="), "%d", &n)
			if n > 0 {
				vsockPort = n
			}
		}
	}
	return sessionToken, vsockPort
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func mustHostname() string {
	h, _ := os.Hostname()
	return h
}

func safePrefix(s string) string {
	if len(s) < 4 {
		return "---"
	}
	return s[:4]
}
