// Additional watchers for the guest-agent. These were introduced to carry
// claude-wrap events over vsock and to satisfy file_exists /
// file_contents_regex lesson predicates without a round-trip to the host.
//
// The existing process + port watchers remain in main.go.
package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// fileWatchSpec is one entry in the active lesson's predicate set for file
// watching. Match is exact path for file_exists, regex match for contents.
type fileWatchSpec struct {
	Path    string `json:"path"`
	RegexID string `json:"regex_id,omitempty"`
	Regex   string `json:"regex,omitempty"`
}

// snapshotFile is one entry in the files_snapshot event payload.
type snapshotFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SizeBytes int    `json:"size_bytes"`
}

// snapshotFiles walks root up to maxDepth, reads up to maxFiles files each
// capped at maxBytes bytes, and returns the collected slice. Returns whatever
// it could read on partial failure; the caller does not block the wider
// event flow over a directory not existing.
//
// Skips hidden entries and .git-like noise. Paths are returned relative to
// root so the frontend can group them cleanly ("index.html" vs full path).
func snapshotFiles(root string, maxBytes, maxFiles, maxDepth int) []snapshotFile {
	out := make([]snapshotFile, 0, maxFiles)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return out
	}
	type queued struct {
		path  string
		depth int
	}
	queue := []queued{{path: root, depth: 0}}
	for len(queue) > 0 && len(out) < maxFiles {
		head := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(head.path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "__pycache__" {
				continue
			}
			full := filepath.Join(head.path, name)
			if e.IsDir() {
				if head.depth+1 < maxDepth {
					queue = append(queue, queued{path: full, depth: head.depth + 1})
				}
				continue
			}
			if len(out) >= maxFiles {
				break
			}
			b, err := os.ReadFile(full)
			if err != nil {
				continue
			}
			orig := len(b)
			if orig > maxBytes {
				b = b[:maxBytes]
			}
			rel, rerr := filepath.Rel(root, full)
			if rerr != nil {
				rel = full
			}
			out = append(out, snapshotFile{
				Path:      rel,
				Content:   string(b),
				SizeBytes: orig,
			})
		}
	}
	return out
}

// watchClaudeWrap dials the Unix socket that claude-wrap publishes events on,
// decodes one JSON object per line, and republishes them over the outgoing
// vsock channel unchanged. Reconnects with backoff on drop: claude may
// restart inside learn-shell.sh's relaunch loop, so the socket can vanish and
// reappear.
func watchClaudeWrap(out chan<- event, socketPath string) {
	backoff := 500 * time.Millisecond
	for {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			time.Sleep(backoff)
			if backoff < 5*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 500 * time.Millisecond

		dec := json.NewDecoder(bufio.NewReader(conn))
		for {
			// Decode into a generic map; claude-wrap events already match the
			// wire shape. The control plane EventBus subscribers rely on the
			// `type` field and a flat payload.
			var raw map[string]any
			if err := dec.Decode(&raw); err != nil {
				_ = conn.Close()
				break
			}
			t, _ := raw["type"].(string)
			if t == "" {
				continue
			}
			ts, _ := raw["ts"].(string)
			if ts == "" {
				ts = nowRFC3339()
			}
			delete(raw, "type")
			delete(raw, "ts")
			out <- event{
				Type:    t,
				TS:      ts,
				Payload: raw,
			}
		}
	}
}

// watchFiles polls each path for existence. It emits a single file_exists
// event the first time a path is observed; it resumes polling if the file is
// later removed so that re-creation still triggers a fresh event in a later
// lesson.
func watchFiles(out chan<- event, paths <-chan []string) {
	active := map[string]bool{}
	emitted := map[string]bool{}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case list, ok := <-paths:
			if !ok {
				return
			}
			// Replace the active watchlist.
			active = map[string]bool{}
			emitted = map[string]bool{}
			for _, p := range list {
				active[p] = true
			}
		case <-ticker.C:
			for p := range active {
				if emitted[p] {
					continue
				}
				if _, err := os.Stat(p); err != nil {
					continue
				}
				emitted[p] = true
				out <- event{
					Type: "file_exists",
					TS:   nowRFC3339(),
					Payload: map[string]any{
						"path": p,
					},
				}
			}
		}
	}
}

// watchFileContentsRegex polls each configured file for a regex match in its
// contents. Emits file_contents_regex once per (path, regex_id) pair. If the
// active specs change (new lesson), previously-emitted matches are cleared.
func watchFileContentsRegex(out chan<- event, specsCh <-chan []fileWatchSpec) {
	type compiled struct {
		spec *fileWatchSpec
		re   *regexp.Regexp
	}
	active := []compiled{}
	emitted := map[string]bool{}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case specs, ok := <-specsCh:
			if !ok {
				return
			}
			active = active[:0]
			emitted = map[string]bool{}
			for i := range specs {
				s := specs[i]
				re, err := regexp.Compile(s.Regex)
				if err != nil {
					continue
				}
				active = append(active, compiled{spec: &s, re: re})
			}
		case <-ticker.C:
			for _, c := range active {
				key := c.spec.Path + "|" + c.spec.RegexID
				if emitted[key] {
					continue
				}
				b, err := os.ReadFile(c.spec.Path)
				if err != nil {
					continue
				}
				m := c.re.Find(b)
				if m == nil {
					continue
				}
				emitted[key] = true
				snippet := string(m)
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				out <- event{
					Type: "file_contents_regex",
					TS:   nowRFC3339(),
					Payload: map[string]any{
						"path":     c.spec.Path,
						"regex_id": c.spec.RegexID,
						"snippet":  snippet,
					},
				}
			}
		}
	}
}
