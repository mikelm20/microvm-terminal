package learnerbot

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/parser"
)

// TestHelloClaude_Offline replays the captured fixtures for hello-claude.yml
// through the real claude-wrap parser + the real control-plane predicate
// evaluator, and asserts every step emits step_satisfied in order.
//
// This is the load-bearing CI contract for Agent-Spine: if the stream-json
// shape, the parser, the predicate evaluator, the lesson YAML schema, or the
// fixture corpus drift, this test fails before the drift reaches the host.
func TestHelloClaude_Offline(t *testing.T) {
	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}

	lessonPath := filepath.Join(root, "lessons", "hello-claude.yml")
	l, err := LoadLesson(lessonPath)
	if err != nil {
		t.Fatalf("load lesson: %v", err)
	}
	if len(l.Steps) != 5 {
		t.Fatalf("hello-claude should have 5 steps, got %d", len(l.Steps))
	}

	fixtures, err := StepFixtures(root, "hello-claude")
	if err != nil {
		t.Fatalf("list fixtures: %v", err)
	}
	if len(fixtures) != len(l.Steps) {
		t.Fatalf("want %d fixture files, got %d: %v", len(l.Steps), len(fixtures), fixtures)
	}

	bus, ch, cancel, unsub := newEvaluatorRig(t, l)
	defer cancel()
	defer unsub()
	bp := &BusPublisher{Bus: bus, Parser: parser.New()}

	for i, step := range l.Steps {
		lines, err := LoadFixture(fixtures[i])
		if err != nil {
			t.Fatalf("load fixture %s: %v", fixtures[i], err)
		}
		if len(lines) == 0 {
			t.Fatalf("fixture %s is empty", fixtures[i])
		}
		for _, fl := range lines {
			bp.Feed(fl)
		}
		got := WaitForStep(t, ch, step.ID, 2*time.Second)
		gotLesson, _ := got.Payload["lesson_id"].(string)
		if gotLesson != l.ID {
			t.Fatalf("step %q: lesson_id=%q want %q", step.ID, gotLesson, l.ID)
		}
	}
}
