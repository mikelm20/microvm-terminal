package lesson

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// Evaluator walks a session through a lesson's predicates. It subscribes to
// the session EventBus, matches every incoming event against the active
// step's Predicate, and on first match publishes a step_satisfied event
// back on the bus. Evidence is the matching event so the UI can explain why.
//
// One Evaluator per (session, lesson) pair. Finishing the last step does not
// tear down the session; the evaluator simply idles.
type Evaluator struct {
	logger *slog.Logger
	lesson *Lesson
	bus    *session.EventBus

	mu       sync.Mutex
	stepIdx  int
	finished bool
}

func NewEvaluator(logger *slog.Logger, bus *session.EventBus, lesson *Lesson) *Evaluator {
	return &Evaluator{logger: logger, lesson: lesson, bus: bus}
}

// Run blocks until ctx is done or the bus closes. Call once per evaluator.
// History events published before Subscribe are replayed first so an
// evaluator started mid-session does not miss a predicate.
func (e *Evaluator) Run(ctx context.Context) {
	history, ch, unsub := e.bus.Subscribe(256)
	defer unsub()
	for _, ev := range history {
		e.consume(ev)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			e.consume(ev)
		}
	}
}

// ActiveStepID returns the id of the step currently being evaluated, or "" if
// the lesson is already complete.
func (e *Evaluator) ActiveStepID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.finished || e.stepIdx >= len(e.lesson.Steps) {
		return ""
	}
	return e.lesson.Steps[e.stepIdx].ID
}

func (e *Evaluator) consume(ev session.GuestEvent) {
	e.mu.Lock()
	if e.finished || e.stepIdx >= len(e.lesson.Steps) {
		e.mu.Unlock()
		return
	}
	step := e.lesson.Steps[e.stepIdx]
	e.mu.Unlock()

	if !matchEvent(step.Success, ev) {
		return
	}

	e.mu.Lock()
	// Guard against duplicate matches on the same step (events arrive in
	// bursts). Only advance if we're still on the same step.
	if step.ID != e.lesson.Steps[e.stepIdx].ID {
		e.mu.Unlock()
		return
	}
	evidence := ev
	e.stepIdx++
	if e.stepIdx >= len(e.lesson.Steps) {
		e.finished = true
	}
	e.mu.Unlock()

	out := session.GuestEvent{
		Type: "step_satisfied",
		TS:   time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"step_id":   step.ID,
			"lesson_id": e.lesson.ID,
			"evidence":  flatten(evidence),
		},
	}
	e.bus.Publish(out)
	if e.logger != nil {
		e.logger.Info("step_satisfied",
			"lesson", e.lesson.ID,
			"step", step.ID,
			"trigger", ev.Type,
		)
	}
}

// flatten produces a JSON-safe snapshot of the event that satisfied the step.
func flatten(ev session.GuestEvent) map[string]any {
	m := map[string]any{
		"type": ev.Type,
		"ts":   ev.TS,
	}
	for k, v := range ev.Payload {
		m[k] = v
	}
	return m
}

// matchEvent is the single place where predicate semantics live. Mirrors
// shared/lessons/schema.ts. Unknown predicate types never match.
func matchEvent(p Predicate, ev session.GuestEvent) bool {
	switch p.Type {
	case "agent_online":
		return ev.Type == "agent_online"
	case "process_started":
		if ev.Type != "process_started" {
			return false
		}
		if p.Match == nil {
			return true
		}
		return stringField(ev.Payload, "name") == p.Match.Name
	case "port_listening":
		if ev.Type != "port_listening" {
			return false
		}
		if p.Match == nil || p.Match.Port == 0 {
			return true
		}
		return intField(ev.Payload, "port") == p.Match.Port
	case "claude_prompt_sent":
		if ev.Type != "claude_prompt_sent" {
			return false
		}
		if p.MatchRegex == "" {
			return true
		}
		re, err := regexp.Compile(p.MatchRegex)
		if err != nil {
			return false
		}
		return re.MatchString(stringField(ev.Payload, "text")) ||
			re.MatchString(stringField(ev.Payload, "prompt_text"))
	case "claude_tool_call":
		if ev.Type != "claude_tool_call" || p.Match == nil {
			return ev.Type == "claude_tool_call" && p.Match == nil
		}
		if p.Match.Tool != "" && !strings.EqualFold(stringField(ev.Payload, "tool"), p.Match.Tool) {
			return false
		}
		if p.Match.PathRegex != "" {
			re, err := regexp.Compile(p.Match.PathRegex)
			if err != nil || !re.MatchString(stringField(ev.Payload, "path")) {
				return false
			}
		}
		if p.Match.CommandRegex != "" {
			re, err := regexp.Compile(p.Match.CommandRegex)
			if err != nil || !re.MatchString(stringField(ev.Payload, "command")) {
				return false
			}
		}
		if p.Match.SubagentType != "" && stringField(ev.Payload, "subagent_type") != p.Match.SubagentType {
			return false
		}
		return true
	case "claude_slash_command":
		if ev.Type != "claude_slash_command" {
			return false
		}
		if p.Match == nil || p.Match.Command == "" {
			return true
		}
		// Accept either `command` (schema) or `command_name` (wire from
		// claude-wrap) to keep evolution cheap.
		name := stringField(ev.Payload, "command_name")
		if name == "" {
			name = strings.TrimPrefix(stringField(ev.Payload, "command"), "/")
		}
		return name == p.Match.Command
	case "file_exists":
		if ev.Type != "file_exists" {
			return false
		}
		if p.Match == nil || p.Match.Path == "" {
			return true
		}
		return stringField(ev.Payload, "path") == p.Match.Path
	case "file_contents_regex":
		if ev.Type != "file_contents_regex" {
			return false
		}
		if p.Match == nil {
			return true
		}
		if p.Match.Path != "" && stringField(ev.Payload, "path") != p.Match.Path {
			return false
		}
		if p.Match.Regex != "" {
			re, err := regexp.Compile(p.Match.Regex)
			if err != nil {
				return false
			}
			if !re.MatchString(stringField(ev.Payload, "snippet")) {
				return false
			}
		}
		return true
	}
	return false
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	// Handle numeric JSON decoding path (json.Number) gracefully.
	if n, ok := m[key].(json.Number); ok {
		return n.String()
	}
	return ""
}

func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}
