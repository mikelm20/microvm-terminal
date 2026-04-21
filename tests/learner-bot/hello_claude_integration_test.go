//go:build integration

package learnerbot

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/parser"
)

// TestHelloClaude_Integration drives a real Claude Code CLI with the exact
// flags claude-wrap uses (`--output-format stream-json --input-format
// stream-json --verbose --dangerously-skip-permissions`), pipes its stdout
// through the claude-wrap parser, and asserts the same hello-claude.yml
// predicates fire. It replaces fixture lines 4 and 5 with live output and
// replays synthesized guest-agent events for steps 1 to 3 (agent_online,
// process_started, port_listening cannot be produced outside a real VM).
//
// Guarded by build tag `integration`. Without CLAUDE_CODE_OAUTH_TOKEN or a
// `claude` binary on PATH, the test skips cleanly so `go test -tags=
// integration ./...` is always runnable.
func TestHelloClaude_Integration(t *testing.T) {
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && os.Getenv("LEARN_BOT_USE_AMBIENT_AUTH") != "1" {
		// claude-wrap itself relies on CLAUDE_CODE_OAUTH_TOKEN to pick up
		// the long-lived OAuth token inside the VM. We refuse to exercise
		// Claude under any other auth path from CI to avoid leaking
		// interactive sessions. Local dev can set LEARN_BOT_USE_AMBIENT_AUTH=1
		// to opt into the machine's existing `claude` login instead.
		t.Skip("CLAUDE_CODE_OAUTH_TOKEN not set; skipping integration")
	}
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		t.Skipf("claude binary not found on PATH: %v", err)
	}

	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	lessonPath := filepath.Join(root, "lessons", "hello-claude.yml")
	l, err := LoadLesson(lessonPath)
	if err != nil {
		t.Fatalf("load lesson: %v", err)
	}

	// Sandbox with a minimal README the Read-tool step can land on.
	sandbox := t.TempDir()
	readme := filepath.Join(sandbox, "README.md")
	if err := os.WriteFile(readme, []byte("Hello, Claude Code learner. This is a small README inside your sandbox.\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	bus, ch, cancel, unsub := newEvaluatorRig(t, l)
	defer cancel()
	defer unsub()
	bp := &BusPublisher{Bus: bus, Parser: parser.New()}

	// Steps 1 to 3 are VM-only events. Synthesize them so the evaluator can
	// advance to the stream-json driven steps.
	bp.Feed(FixtureLine{Wire: "guest", Type: "agent_online", Payload: map[string]interface{}{"hostname": "integration-local"}})
	WaitForStep(t, ch, "agent-online", 2*time.Second)

	bp.Feed(FixtureLine{Wire: "guest", Type: "process_started", Payload: map[string]interface{}{"name": "claude", "pid": 1}})
	WaitForStep(t, ch, "start-claude", 2*time.Second)

	bp.Feed(FixtureLine{Wire: "guest", Type: "port_listening", Payload: map[string]interface{}{"port": 3000}})
	WaitForStep(t, ch, "run-server", 2*time.Second)

	// Spawn Claude with stream-json i/o, driven directly through its pipes.
	// This is the wire claude-wrap carries: claude-wrap wraps both the pipe
	// handling and the prompt echo (claude_prompt_sent is emitted locally).
	// Here we send the user envelope on stdin and synthesize the same
	// claude_prompt_sent event ourselves, exactly like claude-wrap would.
	ctx, ccancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer ccancel()

	cmd := exec.CommandContext(ctx, claudeBin,
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
		"--print",
	)
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "CI=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start claude: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	// Pump stdout through the parser into the bus. Runs until Claude exits.
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
		for sc.Scan() {
			line := make([]byte, len(sc.Bytes()))
			copy(line, sc.Bytes())
			if testing.Verbose() {
				preview := string(line)
				if len(preview) > 180 {
					preview = preview[:180] + "..."
				}
				t.Logf("claude stdout: %s", preview)
			}
			evts := bp.Parser.Feed(line)
			for _, ev := range evts {
				if testing.Verbose() {
					t.Logf("  -> event type=%s tool=%s path=%s", ev.Type, ev.Tool, ev.Path)
				}
				bp.PublishParserEvent(ev)
			}
		}
		if err := sc.Err(); err != nil && err != io.EOF {
			t.Logf("stdout scan: %v", err)
		}
	}()

	send := func(text string) {
		// Mirror claude-wrap submit(): emit claude_prompt_sent ourselves,
		// then write the stream-json user envelope to claude's stdin.
		bp.Feed(FixtureLine{
			Wire:    "guest",
			Type:    "claude_prompt_sent",
			Payload: map[string]interface{}{"text": text, "prompt_text": text, "length": len(text)},
		})
		enc := json.NewEncoder(stdin)
		_ = enc.Encode(map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": text},
				},
			},
		})
	}

	send("say hi in one sentence")
	WaitForStep(t, ch, "first-prompt", 60*time.Second)

	send("Read the file README.md and tell me its first line.")
	WaitForStep(t, ch, "first-tool-call", 90*time.Second)

	// Close claude's stdin and let it drain.
	_ = stdin.Close()
	_ = cmd.Wait()
	wg.Wait()
}
