package parser

import "testing"

func TestParser_AssistantTextTurn(t *testing.T) {
	p := New()
	got := p.Feed([]byte(`{"type":"assistant","message":{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hola"}]}}`))
	if len(got) < 2 {
		t.Fatalf("want busy+token events, got %d", len(got))
	}
	if got[0].Type != "claude_busy" || got[0].Busy == nil || !*got[0].Busy {
		t.Fatalf("first event should be claude_busy=true, got %+v", got[0])
	}
	found := false
	for _, e := range got {
		if e.Type == "claude_token_streamed" && e.Delta == "hola" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected token_streamed with delta=hola")
	}

	finals := p.Feed([]byte(`{"type":"result","subtype":"success","duration_ms":123,"result":"ok"}`))
	if len(finals) != 2 {
		t.Fatalf("expected message+busy_false, got %d", len(finals))
	}
	if finals[0].Type != "claude_message" || finals[0].Role != "assistant" || finals[0].Text != "hola" {
		t.Fatalf("bad final message: %+v", finals[0])
	}
	if finals[1].Type != "claude_busy" || finals[1].Busy == nil || *finals[1].Busy {
		t.Fatalf("expected claude_busy=false, got %+v", finals[1])
	}
}

func TestParser_ToolUseRead(t *testing.T) {
	p := New()
	_ = p.Feed([]byte(`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"looking"}]}}`))
	got := p.Feed([]byte(`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"Read","input":{"file_path":"/home/learner/empresa-prueba/01-ventas/CLAUDE.md"}}]}}`))
	var tc *Event
	for i := range got {
		if got[i].Type == "claude_tool_call" {
			tc = &got[i]
			break
		}
	}
	if tc == nil {
		t.Fatalf("expected claude_tool_call")
	}
	if tc.Tool != "Read" {
		t.Fatalf("tool=%q want Read", tc.Tool)
	}
	if tc.Path != "/home/learner/empresa-prueba/01-ventas/CLAUDE.md" {
		t.Fatalf("path=%q unexpected", tc.Path)
	}
	if tc.CallID != "tool_1" {
		t.Fatalf("call_id=%q want tool_1", tc.CallID)
	}
}

func TestParser_ToolResult(t *testing.T) {
	p := New()
	// First, emit tool_use so the parser remembers the tool name for the call.
	_ = p.Feed([]byte(`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"Bash","input":{"command":"mkdir mi-trabajo"}}]}}`))
	got := p.Feed([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"ok","is_error":false}]}}`))
	if len(got) != 1 || got[0].Type != "claude_tool_result" {
		t.Fatalf("expected single claude_tool_result, got %+v", got)
	}
	if got[0].OK == nil || !*got[0].OK {
		t.Fatalf("expected ok=true")
	}
	if got[0].CallID != "tool_1" {
		t.Fatalf("call_id wrong")
	}
}

func TestParser_UserText(t *testing.T) {
	p := New()
	got := p.Feed([]byte(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"lee la carpeta 01-ventas"}]}}`))
	if len(got) != 2 {
		t.Fatalf("want prompt_sent + claude_message, got %d", len(got))
	}
	if got[0].Type != "claude_prompt_sent" {
		t.Fatalf("first event %q want claude_prompt_sent", got[0].Type)
	}
	if got[0].Text != "lee la carpeta 01-ventas" {
		t.Fatalf("prompt text mismatch: %q", got[0].Text)
	}
	if got[1].Type != "claude_message" || got[1].Role != "user" {
		t.Fatalf("second event bad: %+v", got[1])
	}
}
