// learn-guest-agent runs inside every Firecracker VM. It watches the guest for
// events a lesson's wizard cares about (processes starting, ports opening,
// files appearing) and emits them over a vsock connection to the host control
// plane.
//
// Kernel cmdline params consumed:
//
//	learn.session_token=<hex>   -> proves our identity to the host
//	learn.vsock_port=<int>      -> host-side vsock port we dial (default 5555)
//
// Events are one per line, JSON.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/vsock"
)

var Version = "dev"

const (
	hostCID         uint32 = 2
	defaultVsockPort uint32 = 5555
)

type event struct {
	Type    string            `json:"type"`
	TS      string            `json:"ts"`
	Session string            `json:"session_token,omitempty"` // only on hello
	Payload map[string]any    `json:"payload,omitempty"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("learn-guest-agent %s starting", Version)

	sessionToken, vsockPort := readCmdlineParams()
	if sessionToken == "" {
		log.Printf("no learn.session_token on cmdline; agent is a no-op (demo mode)")
	}
	log.Printf("session_token=%s... vsock_port=%d", safePrefix(sessionToken), vsockPort)

	conn, err := dialHost(vsockPort)
	if err != nil {
		log.Fatalf("dial host vsock: %v", err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(event{
		Type:    "hello",
		TS:      nowRFC3339(),
		Session: sessionToken,
		Payload: map[string]any{
			"hostname": mustHostname(),
			"version":  Version,
		},
	}); err != nil {
		log.Fatalf("send hello: %v", err)
	}
	log.Printf("handshake sent")

	// Fan-in channel; watchers push events, writer serializes.
	events := make(chan event, 256)

	go watchProcesses(events)
	go watchPorts(events)
	go watchClaudeWrap(events, "/run/learn/claude-wrap.sock")

	filePaths := make(chan []string, 4)
	regexSpecs := make(chan []fileWatchSpec, 4)
	go watchFiles(events, filePaths)
	go watchFileContentsRegex(events, regexSpecs)

	// Config reader: the control plane can push lesson predicate sets to us
	// over the same vsock connection as JSON frames with type "config",
	// and snapshot_files requests that publish a synthetic event back out.
	go readConfig(conn, filePaths, regexSpecs, events)

	for e := range events {
		if err := enc.Encode(e); err != nil {
			log.Printf("encode event: %v; reconnecting...", err)
			conn.Close()
			for {
				time.Sleep(2 * time.Second)
				var reErr error
				conn, reErr = dialHost(vsockPort)
				if reErr == nil {
					enc = json.NewEncoder(conn)
					_ = enc.Encode(event{Type: "hello", TS: nowRFC3339(), Session: sessionToken, Payload: map[string]any{"hostname": mustHostname(), "reconnect": true}})
					go readConfig(conn, filePaths, regexSpecs, events)
					break
				}
				log.Printf("redial: %v", reErr)
			}
		}
	}
}

// readConfig decodes messages the control plane pushes down the vsock
// connection. Recognized types:
//   - config: `file_watches` + `regex_watches` update the active lesson
//     predicate set.
//   - snapshot_files: asks the agent to walk a directory and publish a
//     `files_snapshot` event on the outgoing channel. Used by the Capstone
//     flow after port_listening:3000 so the frontend can show the code
//     Claude just wrote.
//
// Unknown types are silently ignored for forward compatibility.
func readConfig(conn net.Conn, filePaths chan<- []string, regexSpecs chan<- []fileWatchSpec, events chan<- event) {
	type snapshotReq struct {
		Root     string `json:"root"`
		MaxBytes int    `json:"max_bytes"`
		MaxFiles int    `json:"max_files"`
		MaxDepth int    `json:"max_depth"`
	}
	type msg struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	dec := json.NewDecoder(conn)
	for {
		var m msg
		if err := dec.Decode(&m); err != nil {
			return
		}
		switch m.Type {
		case "config":
			var p struct {
				FileWatches  []string        `json:"file_watches"`
				RegexWatches []fileWatchSpec `json:"regex_watches"`
			}
			_ = json.Unmarshal(m.Payload, &p)
			filePaths <- p.FileWatches
			regexSpecs <- p.RegexWatches
		case "snapshot_files":
			var p snapshotReq
			_ = json.Unmarshal(m.Payload, &p)
			if p.Root == "" {
				continue
			}
			if p.MaxBytes <= 0 {
				p.MaxBytes = 64 * 1024
			}
			if p.MaxFiles <= 0 {
				p.MaxFiles = 32
			}
			if p.MaxDepth <= 0 {
				p.MaxDepth = 4
			}
			files := snapshotFiles(p.Root, p.MaxBytes, p.MaxFiles, p.MaxDepth)
			events <- event{
				Type: "files_snapshot",
				TS:   nowRFC3339(),
				Payload: map[string]any{
					"root":  p.Root,
					"files": files,
				},
			}
		}
	}
}

func dialHost(port uint32) (net.Conn, error) {
	return vsock.Dial(hostCID, port, nil)
}

func readCmdlineParams() (sessionToken string, vsockPort uint32) {
	vsockPort = defaultVsockPort
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return "", vsockPort
	}
	for _, tok := range strings.Fields(string(data)) {
		switch {
		case strings.HasPrefix(tok, "learn.session_token="):
			sessionToken = strings.TrimPrefix(tok, "learn.session_token=")
		case strings.HasPrefix(tok, "learn.vsock_port="):
			var n uint32
			fmt.Sscanf(strings.TrimPrefix(tok, "learn.vsock_port="), "%d", &n)
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

// watchProcesses polls /proc for new PIDs, compares names against a watchlist,
// and emits `process_started` events the first time each name appears.
func watchProcesses(out chan<- event) {
	seen := make(map[int]struct{})
	emitted := make(map[string]struct{})
	watchlist := map[string]struct{}{
		"claude": {},
		"node":   {},
		"npm":    {},
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		procs, _ := os.ReadDir("/proc")
		for _, d := range procs {
			if !d.IsDir() {
				continue
			}
			pid := 0
			if _, err := fmt.Sscanf(d.Name(), "%d", &pid); err != nil || pid == 0 {
				continue
			}
			if _, ok := seen[pid]; ok {
				continue
			}
			seen[pid] = struct{}{}
			comm, err := os.ReadFile("/proc/" + d.Name() + "/comm")
			if err != nil {
				continue
			}
			name := strings.TrimSpace(string(comm))
			if _, interesting := watchlist[name]; !interesting {
				continue
			}
			if _, already := emitted[name]; already {
				continue
			}
			emitted[name] = struct{}{}
			out <- event{
				Type: "process_started",
				TS:   nowRFC3339(),
				Payload: map[string]any{
					"name": name,
					"pid":  pid,
				},
			}
		}
	}
}

// watchPorts reads /proc/net/tcp and /proc/net/tcp6 every second and emits
// `port_listening` events the first time a port enters LISTEN state.
func watchPorts(out chan<- event) {
	emitted := make(map[int]struct{})

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			// skip header
			scanner.Scan()
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				// fields[1] = "local_address", fields[3] = state (hex, "0A" == LISTEN)
				if len(fields) < 4 || fields[3] != "0A" {
					continue
				}
				parts := strings.Split(fields[1], ":")
				if len(parts) != 2 {
					continue
				}
				var port int
				if _, err := fmt.Sscanf(parts[1], "%X", &port); err != nil || port == 0 {
					continue
				}
				if _, already := emitted[port]; already {
					continue
				}
				emitted[port] = struct{}{}
				out <- event{
					Type: "port_listening",
					TS:   nowRFC3339(),
					Payload: map[string]any{
						"port": port,
					},
				}
			}
			f.Close()
		}
	}
}
