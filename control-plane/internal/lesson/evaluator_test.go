package lesson

import (
	"context"
	"testing"
	"time"

	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// TestEvaluator_m2SyntheticStream replays a synthetic stream of events that
// mirrors what Agent-Spine expects from m2 and asserts the four step ids
// advance in order.
func TestEvaluator_m2SyntheticStream(t *testing.T) {
	bus := session.NewEventBus()
	l := &Lesson{
		ID: "m2-primera-conversacion",
		Steps: []Step{
			{
				ID: "leer-departamento",
				Success: Predicate{
					Type:  "claude_tool_call",
					Match: &Match{Tool: "Read", PathRegex: "01-ventas"},
				},
			},
			{
				ID: "corregir-sobre-la-marcha",
				Success: Predicate{
					Type:       "claude_prompt_sent",
					MatchRegex: "(?i)(dos\\s+frases|mas\\s+corto|informal)",
				},
			},
			{
				ID: "pedir-algo-ambicioso",
				Success: Predicate{
					Type:       "claude_prompt_sent",
					MatchRegex: "(?i)(resumen|estado).{0,40}(ventas|departamento)",
				},
			},
			{
				ID: "pregunta-sin-datos",
				Success: Predicate{
					Type:       "claude_prompt_sent",
					MatchRegex: "(?i)(cuantos|clientes).{0,40}madrid",
				},
			},
		},
	}

	// Capture step_satisfied events. Subscribe BEFORE the evaluator so we
	// don't miss its emissions. The evaluator's Run replays history, so we
	// can publish the synthetic stream before starting it.
	_, ch, unsub := bus.Subscribe(64)
	defer unsub()

	// Feed an event stream that satisfies each step in order, with some
	// noise events in between (distractors).
	stream := []session.GuestEvent{
		{Type: "agent_online", TS: "t0"},
		{Type: "claude_busy", TS: "t1", Payload: map[string]any{"busy": true}},
		{Type: "claude_tool_call", TS: "t2", Payload: map[string]any{
			"tool": "Read", "path": "/home/learner/empresa-prueba/01-ventas/CLAUDE.md", "call_id": "c1",
		}},
		{Type: "claude_message", TS: "t3", Payload: map[string]any{"role": "assistant", "text": "hola"}},
		{Type: "claude_prompt_sent", TS: "t4", Payload: map[string]any{"text": "dos frases y informal"}},
		{Type: "claude_prompt_sent", TS: "t5", Payload: map[string]any{"text": "preparame un resumen del estado del departamento de ventas"}},
		{Type: "claude_prompt_sent", TS: "t6", Payload: map[string]any{"text": "cuantos clientes tenemos en Madrid"}},
	}
	for _, ev := range stream {
		bus.Publish(ev)
	}

	e := NewEvaluator(nil, bus, l)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	want := []string{
		"leer-departamento",
		"corregir-sobre-la-marcha",
		"pedir-algo-ambicioso",
		"pregunta-sin-datos",
	}
	got := []string{}
	deadline := time.After(2 * time.Second)
	for len(got) < len(want) {
		select {
		case out := <-ch:
			if out.Type != "step_satisfied" {
				continue
			}
			stepID, _ := out.Payload["step_id"].(string)
			got = append(got, stepID)
		case <-deadline:
			t.Fatalf("timeout: got %v, want %v", got, want)
		}
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("step %d: got %q want %q (full %v)", i, got[i], w, got)
		}
	}
}

func TestMatchEvent_ToolCall(t *testing.T) {
	p := Predicate{Type: "claude_tool_call", Match: &Match{Tool: "Bash", CommandRegex: "mkdir.*mi-trabajo"}}
	ev := session.GuestEvent{Type: "claude_tool_call", Payload: map[string]any{
		"tool": "Bash", "command": "mkdir -p mi-trabajo",
	}}
	if !matchEvent(p, ev) {
		t.Fatalf("expected match for Bash mkdir mi-trabajo")
	}
	// Wrong tool
	ev.Payload["tool"] = "Read"
	if matchEvent(p, ev) {
		t.Fatalf("expected no match for Read with Bash predicate")
	}
}

func TestMatchEvent_FileContentsRegex(t *testing.T) {
	p := Predicate{Type: "file_contents_regex", Match: &Match{
		Path:  "/home/learner/mi-trabajo/CLAUDE.md",
		Regex: "(?i)empresa\\s+prueba",
	}}
	ev := session.GuestEvent{Type: "file_contents_regex", Payload: map[string]any{
		"path":    "/home/learner/mi-trabajo/CLAUDE.md",
		"snippet": "Trabajo en Empresa Prueba",
	}}
	if !matchEvent(p, ev) {
		t.Fatalf("expected file_contents_regex match")
	}
}

func TestMatchEvent_SlashCommand(t *testing.T) {
	p := Predicate{Type: "claude_slash_command", Match: &Match{Command: "agents"}}
	// Wire shape from claude-wrap uses command_name; accept either key.
	ev := session.GuestEvent{Type: "claude_slash_command", Payload: map[string]any{"command_name": "agents"}}
	if !matchEvent(p, ev) {
		t.Fatalf("expected /agents match via command_name")
	}
	ev2 := session.GuestEvent{Type: "claude_slash_command", Payload: map[string]any{"command": "/agents"}}
	if !matchEvent(p, ev2) {
		t.Fatalf("expected /agents match via command=/agents")
	}
}
