// learner-bot verifies the deterministic predicate spine end-to-end.
//
// Mode `fixture` (default): replays a JSONL corpus from tests/fixtures/claude
// through the claude-wrap parser, pipes the resulting events into the lesson
// Evaluator, and asserts that every expected claude_tool_call appears and
// every step_satisfied fires in order. Exits 0 on pass, non-zero on fail.
//
// Mode `live` (unimplemented in this branch): would dial the control plane WS
// at -api and drive a real session. The hook is left in place so Agent-API's
// docker-compose.dev stack plugs in without a rewrite.
//
// Usage:
//
//	go run ./tests/learner-bot -lesson m2 -lang es
//	go run ./tests/learner-bot -lesson m5 -lang en -mode fixture
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mikelm20/learn-platform/control-plane/internal/lesson"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/parser"
)

var (
	lessonFlag = flag.String("lesson", "m2", "lesson id prefix, e.g. m2 or m2-primera-conversacion")
	langFlag   = flag.String("lang", "es", "language, es|en")
	modeFlag   = flag.String("mode", "fixture", "fixture|live")
	repoRoot   = flag.String("root", autoRoot(), "repository root")
	apiFlag    = flag.String("api", "http://localhost:8080", "control plane base URL (live mode)")
)

// fixtureMap tells the bot which step IDs to satisfy in what order and which
// fixture(s) to replay for each. The order here matches the lesson YAML.
var fixtureMap = map[string][]struct {
	Step    string
	Fixture string
}{
	"m2-primera-conversacion": {
		{"leer-departamento", "m2-leer-departamento-1.jsonl"},
		{"corregir-sobre-la-marcha", "m2-corregir-1.jsonl"},
		{"pedir-algo-ambicioso", "m2-pedir-ambicioso-1.jsonl"},
		{"pregunta-sin-datos", "m2-pregunta-sin-datos-1.jsonl"},
	},
	"m4-dia-a-dia": {
		{"parte-matinal", "m4-parte-matinal-1.jsonl"},
		{"brief-pre-reunion", "m4-brief-reunion-1.jsonl"},
	},
	"m5-subagentes": {
		{"ver-subagentes-vacios", "m5-ver-subagentes-1.jsonl"},
		{"crear-jefe-de-agenda", "m5-crear-jefe-agenda-1.jsonl"},
		{"invocar-jefe-de-agenda", "m5-invocar-1.jsonl"},
	},
}

func main() {
	flag.Parse()

	if *modeFlag != "fixture" {
		log.Fatalf("mode %q not implemented in this branch; use -mode fixture", *modeFlag)
	}

	lessonID := resolveLessonID(*lessonFlag)
	lessonPath := filepath.Join(*repoRoot, "lessons", lessonID+"."+*langFlag+".yml")
	ls, err := loadLesson(lessonPath)
	if err != nil {
		log.Fatalf("load lesson %s: %v", lessonPath, err)
	}

	plan, ok := fixtureMap[lessonID]
	if !ok {
		log.Fatalf("no fixture plan registered for %s", lessonID)
	}

	// Build a minimal Lesson for the evaluator out of the YAML-loaded steps.
	subset := make([]lesson.Step, 0, len(plan))
	for _, entry := range plan {
		ydef := findStep(ls, entry.Step)
		if ydef == nil {
			log.Fatalf("lesson %s has no step %s", lessonID, entry.Step)
		}
		subset = append(subset, ydef.toEvalStep())
	}
	evalLesson := &lesson.Lesson{ID: lessonID, Steps: subset}

	// Subscribe BEFORE the evaluator attaches so we see step_satisfied and
	// can count tool_calls passing through.
	bus := session.NewEventBus()
	_, observer, unsub := bus.Subscribe(512)
	defer unsub()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Results accumulator for the final report.
	toolCalls := []string{}
	satisfied := []string{}

	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-observer:
				if !ok {
					return
				}
				switch ev.Type {
				case "claude_tool_call":
					tool, _ := ev.Payload["tool"].(string)
					path, _ := ev.Payload["path"].(string)
					cmd, _ := ev.Payload["command"].(string)
					label := tool
					if path != "" {
						label = fmt.Sprintf("%s %s", tool, path)
					} else if cmd != "" {
						if len(cmd) > 50 {
							cmd = cmd[:50] + "..."
						}
						label = fmt.Sprintf("%s %s", tool, cmd)
					}
					toolCalls = append(toolCalls, label)
				case "step_satisfied":
					sid, _ := ev.Payload["step_id"].(string)
					satisfied = append(satisfied, sid)
					if len(satisfied) == len(plan) {
						close(doneCh)
						return
					}
				}
			}
		}
	}()

	// Replay each fixture. We feed events through the same shape the guest-
	// agent would emit via vsock: type + ts + flat payload.
	for _, entry := range plan {
		fixPath := filepath.Join(*repoRoot, "tests", "fixtures", "claude", entry.Fixture)
		if err := replayFixture(fixPath, bus); err != nil {
			log.Fatalf("replay %s: %v", fixPath, err)
		}
	}

	// Start the evaluator AFTER publishing so it reads history and advances
	// through every step deterministically.
	ev := lesson.NewEvaluator(nil, bus, evalLesson)
	go ev.Run(ctx)

	select {
	case <-doneCh:
	case <-ctx.Done():
	}

	fmt.Printf("lesson:    %s (%s)\n", lessonID, *langFlag)
	fmt.Printf("expected:  %d step_satisfied\n", len(plan))
	fmt.Printf("observed:  %d step_satisfied\n", len(satisfied))
	fmt.Printf("tool calls:\n")
	for _, tc := range toolCalls {
		fmt.Printf("  - %s\n", tc)
	}
	fmt.Printf("order:     %s\n", strings.Join(satisfied, " -> "))

	if len(satisfied) != len(plan) {
		log.Fatalf("FAIL: only %d/%d steps satisfied", len(satisfied), len(plan))
	}
	for i, entry := range plan {
		if satisfied[i] != entry.Step {
			log.Fatalf("FAIL: step %d got %s, want %s", i, satisfied[i], entry.Step)
		}
	}
	fmt.Println("PASS")
}

// replayFixture parses a JSONL claude stream file through the claude-wrap
// parser and publishes each resulting event to the bus, wrapped as a
// GuestEvent. Also asserts the fixture yields at least one event.
func replayFixture(path string, bus *session.EventBus) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	p := parser.New()
	lines := strings.Split(string(data), "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, ev := range p.Feed([]byte(line)) {
			bus.Publish(toGuestEvent(ev))
			count++
			// Simulate the guest-agent's watchFiles: a successful Write tool
			// would cause the file to appear on disk, which in production
			// triggers a file_exists event. In fixture replay we don't have
			// a live filesystem, so we synthesize the follow-up event.
			if ev.Type == "claude_tool_call" && (ev.Tool == "Write" || ev.Tool == "Edit") && ev.Path != "" {
				bus.Publish(session.GuestEvent{
					Type: "file_exists",
					TS:   ev.TS,
					Payload: map[string]any{"path": ev.Path},
				})
			}
		}
	}
	if count == 0 {
		return fmt.Errorf("no events parsed from fixture")
	}
	return nil
}

// toGuestEvent mirrors the shape the guest-agent produces when it tails
// claude-wrap: flat Payload with tool/path/command/etc.
func toGuestEvent(ev parser.Event) session.GuestEvent {
	out := session.GuestEvent{
		Type:    ev.Type,
		TS:      ev.TS,
		Payload: map[string]any{},
	}
	if ev.Tool != "" {
		out.Payload["tool"] = ev.Tool
	}
	if ev.Path != "" {
		out.Payload["path"] = ev.Path
	}
	if ev.Command != "" {
		out.Payload["command"] = ev.Command
	}
	if ev.SubagentType != "" {
		out.Payload["subagent_type"] = ev.SubagentType
	}
	if ev.CallID != "" {
		out.Payload["call_id"] = ev.CallID
	}
	if ev.CommandName != "" {
		out.Payload["command_name"] = ev.CommandName
	}
	if ev.PromptText != "" {
		out.Payload["text"] = ev.PromptText
		out.Payload["prompt_text"] = ev.PromptText
	}
	if ev.Text != "" && out.Payload["text"] == nil {
		out.Payload["text"] = ev.Text
	}
	if ev.Role != "" {
		out.Payload["role"] = ev.Role
	}
	if ev.OK != nil {
		out.Payload["ok"] = *ev.OK
	}
	if ev.Summary != "" {
		out.Payload["summary"] = ev.Summary
	}
	return out
}

// yamlLesson is the minimal shape we need; we accept unknown fields.
type yamlLesson struct {
	ID    string     `yaml:"id"`
	Steps []yamlStep `yaml:"steps"`
}
type yamlStep struct {
	ID      string `yaml:"id"`
	Success struct {
		Type       string `yaml:"type"`
		MatchRegex string `yaml:"match_regex"`
		Match      struct {
			Name         string `yaml:"name"`
			Port         int    `yaml:"port"`
			Tool         string `yaml:"tool"`
			PathRegex    string `yaml:"path_regex"`
			CommandRegex string `yaml:"command_regex"`
			SubagentType string `yaml:"subagent_type"`
			Command      string `yaml:"command"`
			Path         string `yaml:"path"`
			Regex        string `yaml:"regex"`
		} `yaml:"match"`
	} `yaml:"success"`
}

func (s yamlStep) toEvalStep() lesson.Step {
	out := lesson.Step{ID: s.ID}
	out.Success.Type = s.Success.Type
	out.Success.MatchRegex = s.Success.MatchRegex
	if hasMatch(s) {
		out.Success.Match = &lesson.Match{
			Name:         s.Success.Match.Name,
			Port:         s.Success.Match.Port,
			Tool:         s.Success.Match.Tool,
			PathRegex:    s.Success.Match.PathRegex,
			CommandRegex: s.Success.Match.CommandRegex,
			SubagentType: s.Success.Match.SubagentType,
			Command:      s.Success.Match.Command,
			Path:         s.Success.Match.Path,
			Regex:        s.Success.Match.Regex,
		}
	}
	return out
}

func hasMatch(s yamlStep) bool {
	m := s.Success.Match
	return m.Name != "" || m.Port != 0 || m.Tool != "" || m.PathRegex != "" ||
		m.CommandRegex != "" || m.SubagentType != "" || m.Command != "" ||
		m.Path != "" || m.Regex != ""
}

func loadLesson(path string) (*yamlLesson, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ls yamlLesson
	if err := yaml.Unmarshal(b, &ls); err != nil {
		return nil, err
	}
	return &ls, nil
}

func findStep(ls *yamlLesson, id string) *yamlStep {
	for i := range ls.Steps {
		if ls.Steps[i].ID == id {
			return &ls.Steps[i]
		}
	}
	return nil
}

// resolveLessonID maps short flags like "m2" to the canonical id.
func resolveLessonID(flag string) string {
	if strings.Contains(flag, "-") {
		return flag
	}
	switch flag {
	case "m2":
		return "m2-primera-conversacion"
	case "m3":
		return "m3-organizar-cabeza"
	case "m4":
		return "m4-dia-a-dia"
	case "m5":
		return "m5-subagentes"
	case "m6":
		return "m6-skills"
	case "m7":
		return "m7-mcps"
	case "m8":
		return "m8-vida-ai-native"
	}
	return flag
}

// autoRoot walks up from the running binary's cwd until it finds CONTRACTS.md.
// Allows `go run` without a -root flag from anywhere in the tree.
func autoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for cur := wd; cur != "/"; cur = filepath.Dir(cur) {
		if _, err := os.Stat(filepath.Join(cur, "CONTRACTS.md")); err == nil {
			return cur
		}
	}
	return wd
}
