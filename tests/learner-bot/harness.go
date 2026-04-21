// Package learnerbot is the Agent-Spine smoke-test harness. It drives the
// predicate evaluator in two modes:
//
//  1. Offline replay: JSONL fixtures per lesson step are fed through either
//     the claude-wrap stream-json parser or directly into the session
//     EventBus (for guest-agent watcher events). This mode runs in CI
//     without a live Claude Code CLI and is the default.
//
//  2. Integration: the real Claude Code CLI is spawned with stream-json
//     output, its stdout is piped through the exact same parser, and the
//     resulting events drive the evaluator. Guarded by build tag
//     `integration` and skipped when CLAUDE_CODE_OAUTH_TOKEN is unset.
//
// The harness deliberately does NOT spin up Firecracker or the guest-agent.
// Events that only a real VM can produce (agent_online, process_started,
// port_listening) are injected synthetically so that the full lesson arc can
// still be exercised in both modes. This keeps the Spine smoke test focused
// on parser + predicate correctness, which is what this package owns.
package learnerbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mikelm20/learn-platform/control-plane/internal/lesson"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/parser"
)

// lessonYAML is the minimal shape we need from hello-claude.yml. The full
// schema lives in shared/lessons/schema.ts and shared/lessons/schema.go (TS
// side); the Go control plane keeps its own lesson.Predicate mirror. We only
// decode the pieces the evaluator consumes.
type lessonYAML struct {
	ID    string `yaml:"id"`
	Steps []struct {
		ID      string                 `yaml:"id"`
		Success map[string]interface{} `yaml:"success"`
	} `yaml:"steps"`
}

// LoadLesson parses a lesson YAML from disk into a lesson.Lesson shaped for
// the evaluator. Matches the subset of shared/lessons/schema.ts the server
// side cares about; extra frontend-only fields (title, hint, xp, etc.) are
// ignored.
func LoadLesson(path string) (*lesson.Lesson, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc lessonYAML
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := &lesson.Lesson{ID: doc.ID}
	for _, s := range doc.Steps {
		p, err := decodePredicate(s.Success)
		if err != nil {
			return nil, fmt.Errorf("step %q: %w", s.ID, err)
		}
		out.Steps = append(out.Steps, lesson.Step{ID: s.ID, Success: p})
	}
	return out, nil
}

// decodePredicate converts a YAML-decoded success block (map[string]any) into
// the typed lesson.Predicate. Only the predicate types the schema declares
// are accepted.
func decodePredicate(m map[string]interface{}) (lesson.Predicate, error) {
	if m == nil {
		return lesson.Predicate{}, fmt.Errorf("missing success block")
	}
	t, _ := m["type"].(string)
	p := lesson.Predicate{Type: t}
	if mr, ok := m["match_regex"].(string); ok {
		p.MatchRegex = mr
	}
	if matchAny, ok := m["match"]; ok {
		match, _ := matchAny.(map[string]interface{})
		mm := &lesson.Match{}
		if v, ok := match["name"].(string); ok {
			mm.Name = v
		}
		switch v := match["port"].(type) {
		case int:
			mm.Port = v
		case int64:
			mm.Port = int(v)
		case float64:
			mm.Port = int(v)
		}
		if v, ok := match["tool"].(string); ok {
			mm.Tool = v
		}
		if v, ok := match["path_regex"].(string); ok {
			mm.PathRegex = v
		}
		if v, ok := match["command_regex"].(string); ok {
			mm.CommandRegex = v
		}
		if v, ok := match["subagent_type"].(string); ok {
			mm.SubagentType = v
		}
		if v, ok := match["command"].(string); ok {
			mm.Command = v
		}
		if v, ok := match["path"].(string); ok {
			mm.Path = v
		}
		if v, ok := match["regex"].(string); ok {
			mm.Regex = v
		}
		p.Match = mm
	}
	return p, nil
}

// FixtureLine is one entry in a hello-claude fixture file. `Wire` is either
// "guest" for a synthesized guest-agent event (already in the GuestEvent
// shape) or "streamjson" for a raw Claude Code stream-json envelope that the
// claude-wrap parser should transform before publishing.
type FixtureLine struct {
	Wire string          `json:"wire"`
	Line json.RawMessage `json:"line,omitempty"`

	// When Wire=="guest" the same object carries the event payload directly,
	// mirroring the guest-agent vsock wire. These fields are populated by
	// json decoding the full object against the shape below.
	Type    string                 `json:"type,omitempty"`
	TS      string                 `json:"ts,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// LoadFixture parses a JSONL fixture file into an ordered list of lines.
func LoadFixture(path string) ([]FixtureLine, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []FixtureLine
	for i, l := range splitLines(raw) {
		if len(l) == 0 {
			continue
		}
		var fl FixtureLine
		if err := json.Unmarshal(l, &fl); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		out = append(out, fl)
	}
	return out, nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

// StepFixtures returns the sorted fixture file paths for a given lesson_id
// under tests/fixtures/claude/<lesson_id>/. Names are expected to follow the
// "NN-step-id.jsonl" convention so that lexical order matches step order.
func StepFixtures(root, lessonID string) ([]string, error) {
	dir := filepath.Join(root, "tests", "fixtures", "claude", lessonID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

// BusPublisher turns parser.Event plus fixture lines into session.GuestEvent
// publishes on the shared bus. Mirrors guest-agent/watchers.go: a claude-wrap
// event arriving on the vsock wire is republished verbatim (keys lifted,
// `type` and `ts` copied to the envelope, the rest becomes `payload`).
type BusPublisher struct {
	Bus    *session.EventBus
	Parser *parser.Parser
}

// Feed applies one fixture line to the bus. Guest lines are published
// directly; streamjson lines are parsed first and every resulting parser
// event is republished.
func (bp *BusPublisher) Feed(fl FixtureLine) {
	switch fl.Wire {
	case "guest":
		ts := fl.TS
		if ts == "" {
			ts = nowRFC3339()
		}
		bp.Bus.Publish(session.GuestEvent{
			Type:    fl.Type,
			TS:      ts,
			Payload: fl.Payload,
		})
	case "streamjson":
		events := bp.Parser.Feed(fl.Line)
		for _, ev := range events {
			bp.PublishParserEvent(ev)
		}
	}
}

// PublishParserEvent flattens a parser.Event into a session.GuestEvent and
// publishes it on the bus. This mirrors the guest-agent watchClaudeWrap
// behavior: the control plane only sees {type, ts, payload}.
func (bp *BusPublisher) PublishParserEvent(ev parser.Event) {
	// json roundtrip is fine for a test-only helper: it gives us a clean
	// generic map without hand-mapping every parser.Event field. If this
	// ever shows up in a hot path we would switch to explicit copying.
	b, _ := json.Marshal(ev)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	t, _ := m["type"].(string)
	ts, _ := m["ts"].(string)
	delete(m, "type")
	delete(m, "ts")
	bp.Bus.Publish(session.GuestEvent{
		Type:    t,
		TS:      ts,
		Payload: m,
	})
}

// WaitForStep blocks on the provided channel until a step_satisfied event
// whose step_id equals stepID arrives, or deadline elapses. Non-matching
// events are discarded (the evaluator guarantees at-most-once per step).
func WaitForStep(t *testing.T, ch <-chan session.GuestEvent, stepID string, deadline time.Duration) session.GuestEvent {
	t.Helper()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("bus closed waiting for step_satisfied %q", stepID)
			}
			if ev.Type != "step_satisfied" {
				continue
			}
			id, _ := ev.Payload["step_id"].(string)
			if id == stepID {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timeout waiting for step_satisfied %q", stepID)
		}
	}
}

// RunEvaluator spins a lesson.Evaluator inside a goroutine and returns a
// cancel function plus a ready subscriber for step_satisfied events. The
// caller is responsible for invoking cancel() at test end.
func RunEvaluator(bus *session.EventBus, l *lesson.Lesson) (context.CancelFunc, <-chan session.GuestEvent, func()) {
	// Subscribe BEFORE starting the evaluator so its own step_satisfied
	// publishes are observable.
	_, ch, unsub := bus.Subscribe(64)
	ev := lesson.NewEvaluator(nil, bus, l)
	ctx, cancel := context.WithCancel(context.Background())
	go ev.Run(ctx)
	return cancel, ch, unsub
}

// RepoRoot returns the absolute path to the learn-platform repo root (the
// worktree we run inside). The learner-bot always lives at
// tests/learner-bot/ so the root is two levels up.
func RepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..")), nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
