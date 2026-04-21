// Package parser converts stream-json envelopes from Claude Code into the
// canonical wizard events consumed downstream. It is pure: given a line of
// JSON, it returns 0..N WsEvent-shaped payloads. No I/O here.
package parser

import (
	"encoding/json"
	"time"

	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/streamjson"
)

// Event is the on-the-wire shape we emit to the guest-agent. Keys mirror
// shared/api/events.ts exactly, since the guest-agent forwards as-is via
// vsock and the control plane expects the same names.
type Event struct {
	Type           string            `json:"type"`
	TS             string            `json:"ts"`
	Tool           string            `json:"tool,omitempty"`
	Args           map[string]string `json:"args,omitempty"`
	Path           string            `json:"path,omitempty"`
	Command        string            `json:"command,omitempty"`
	SubagentType   string            `json:"subagent_type,omitempty"`
	CallID         string            `json:"call_id,omitempty"`
	OK             *bool             `json:"ok,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	Role           string            `json:"role,omitempty"`
	Text           string            `json:"text,omitempty"`
	DurationMS     *int64            `json:"duration_ms,omitempty"`
	TurnID         string            `json:"turn_id,omitempty"`
	Busy           *bool             `json:"busy,omitempty"`
	CharsSoFar     *int              `json:"chars_so_far,omitempty"`
	Delta          string            `json:"delta,omitempty"`
	TotalChars     *int              `json:"total_chars,omitempty"`
	CommandName    string            `json:"command_name,omitempty"`
	CommandArgs    string            `json:"command_args,omitempty"`
	Length         *int              `json:"length,omitempty"`
	PromptText     string            `json:"prompt_text,omitempty"`
	LocaleDetected string            `json:"locale_detected,omitempty"`
}

// Parser holds the running state needed to fold stream-json envelopes into
// canonical events. The most important piece of state is the current
// assistant turn's accumulator and its turn_id.
type Parser struct {
	turnID     string
	turnText   string
	turnStart  time.Time
	inAssist   bool
	// inputMap lets us attach the tool name when tool_result arrives later.
	toolByCall map[string]string
}

func New() *Parser {
	return &Parser{toolByCall: make(map[string]string)}
}

// Feed parses a single stream-json line. It returns the events to emit, in
// order. Call with one decoded line at a time.
func (p *Parser) Feed(line []byte) []Event {
	var env streamjson.Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil
	}

	switch env.Type {
	case "system":
		return p.onSystem(env)
	case "user":
		return p.onUser(env)
	case "assistant":
		return p.onAssistant(env)
	case "result":
		return p.onResult(env)
	case "rate_limit_event", "stream_event", "usage_event":
		// Informational envelopes emitted by recent Claude Code CLI
		// releases. They carry no content the wizard spine cares about.
		// Listed explicitly so future drift stays visible, and so the
		// silent no-op is intentional rather than coincidental.
		return nil
	}
	return nil
}

func (p *Parser) onSystem(env streamjson.Envelope) []Event {
	// system/init carries session id and model. We do not emit anything here
	// today. Left as a hook for future `claude_system` events if needed.
	_ = env
	return nil
}

// onUser handles two subshapes: (1) pure text content, a real user prompt,
// and (2) tool_result blocks echoed back by Claude after a tool_use.
func (p *Parser) onUser(env streamjson.Envelope) []Event {
	if env.Message == nil {
		return nil
	}
	var out []Event
	ts := now()
	for _, b := range env.Message.Content {
		switch b.Type {
		case "tool_result":
			tool := p.toolByCall[b.ToolUseID]
			ok := !b.IsError
			summary := truncate(streamjson.DecodeContent(b.Content), 120)
			out = append(out, Event{
				Type:    "claude_tool_result",
				TS:      ts,
				CallID:  b.ToolUseID,
				OK:      &ok,
				Summary: summary,
				Tool:    tool, // informational; not in schema but tolerated
			})
		case "text", "":
			// Pure text user message. Emit prompt_sent + claude_message.
			// If the text is a slash command, also emit claude_slash_command
			// so lesson predicates can match on it even when the parser is
			// fed fixture data (no live forwardLoop to intercept).
			if b.Text == "" {
				continue
			}
			l := len(b.Text)
			out = append(out,
				Event{
					Type:       "claude_prompt_sent",
					TS:         ts,
					Text:       b.Text,
					Length:     &l,
					PromptText: b.Text,
				},
				Event{
					Type:   "claude_message",
					TS:     ts,
					Role:   "user",
					Text:   b.Text,
					TurnID: newTurnID(env.Message.ID),
				},
			)
			if len(b.Text) > 0 && b.Text[0] == '/' {
				parts := splitCmd(b.Text[1:])
				if parts[0] != "" {
					out = append(out, Event{
						Type:        "claude_slash_command",
						TS:          ts,
						CommandName: parts[0],
						CommandArgs: parts[1],
					})
				}
			}
		}
	}
	return out
}

// onAssistant folds assistant content blocks into a running turn: text
// accumulates, tool_use blocks emit individual claude_tool_call events.
// A claude_busy=true event is emitted the first time we see assistant output
// for a new turn. The turn is finalized in onResult.
func (p *Parser) onAssistant(env streamjson.Envelope) []Event {
	if env.Message == nil {
		return nil
	}
	var out []Event
	ts := now()

	if !p.inAssist {
		p.inAssist = true
		p.turnID = env.Message.ID
		if p.turnID == "" {
			p.turnID = newTurnID("")
		}
		p.turnStart = time.Now()
		p.turnText = ""
		busy := true
		out = append(out, Event{Type: "claude_busy", TS: ts, Busy: &busy})
	}

	for _, b := range env.Message.Content {
		switch b.Type {
		case "text":
			if b.Text != "" {
				p.turnText += b.Text
				total := len(p.turnText)
				out = append(out, Event{
					Type:       "claude_token_streamed",
					TS:         ts,
					TurnID:     p.turnID,
					Delta:      b.Text,
					TotalChars: &total,
				})
			}
		case "thinking":
			// Claude Code 2.1+ emits `thinking` content blocks before text
			// and tool_use inside the same assistant turn. We do not surface
			// them to learners, but listing the case here prevents drift
			// from sneaking through as a silent default.
			continue
		case "tool_use":
			call := classifyTool(b.Name, b.Input)
			call.Type = "claude_tool_call"
			call.TS = ts
			call.CallID = b.ID
			if b.ID != "" {
				p.toolByCall[b.ID] = call.Tool
			}
			out = append(out, call)
			// Mirror SlashCommand tool_use as a canonical claude_slash_command
			// so lesson predicates land on it whether the slash arrived via
			// the user's stdin (forwardLoop) or via Claude's own tool.
			if call.Tool == "SlashCommand" && call.CommandName != "" {
				out = append(out, Event{
					Type:        "claude_slash_command",
					TS:          ts,
					CommandName: call.CommandName,
				})
			}
		}
	}
	return out
}

// onResult finalizes the turn: emit claude_message{assistant,text,duration}
// then claude_busy=false.
func (p *Parser) onResult(env streamjson.Envelope) []Event {
	ts := now()
	var out []Event
	if p.inAssist {
		var dur *int64
		if env.Duration != nil {
			dur = env.Duration
		} else {
			d := time.Since(p.turnStart).Milliseconds()
			dur = &d
		}
		out = append(out, Event{
			Type:       "claude_message",
			TS:         ts,
			Role:       "assistant",
			Text:       p.turnText,
			DurationMS: dur,
			TurnID:     p.turnID,
		})
	}
	busy := false
	out = append(out, Event{Type: "claude_busy", TS: ts, Busy: &busy})
	p.inAssist = false
	p.turnID = ""
	p.turnText = ""
	return out
}

// classifyTool maps the Claude-reported tool name onto the closed set the
// wizard understands, and extracts the most useful args (path, command,
// subagent_type). Unknown names bucket as "Other".
func classifyTool(name string, input json.RawMessage) Event {
	known := map[string]string{
		"Read":          "Read",
		"Write":         "Write",
		"Edit":          "Edit",
		"MultiEdit":     "Edit",
		"Bash":          "Bash",
		"Glob":          "Glob",
		"Grep":          "Grep",
		"Task":          "Task",
		"WebFetch":      "WebFetch",
		"WebSearch":     "WebSearch",
		"SlashCommand":  "SlashCommand",
	}
	tool := known[name]
	if tool == "" {
		tool = "Other"
	}

	var raw map[string]any
	_ = json.Unmarshal(input, &raw)

	ev := Event{Tool: tool, Args: map[string]string{}}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			if len(s) > 512 {
				s = s[:512]
			}
			ev.Args[k] = s
		}
	}

	if p, ok := raw["file_path"].(string); ok && p != "" {
		ev.Path = p
	} else if p, ok := raw["path"].(string); ok && p != "" {
		ev.Path = p
	} else if p, ok := raw["notebook_path"].(string); ok && p != "" {
		ev.Path = p
	}
	if c, ok := raw["command"].(string); ok && c != "" {
		ev.Command = c
	}
	if st, ok := raw["subagent_type"].(string); ok && st != "" {
		ev.SubagentType = st
	}

	// SlashCommand has a `command` field like "/agents". Name is typically
	// "SlashCommand"; surface the slash name for downstream matching.
	if tool == "SlashCommand" {
		if c, ok := raw["command"].(string); ok {
			ev.CommandName = trimSlash(c)
		}
	}
	return ev
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

// splitCmd splits "agents" or "parte-matinal foo bar" into (name, argstr).
func splitCmd(s string) [2]string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func now() string { return Now() }

// Now returns an RFC3339Nano UTC timestamp suitable for event TS fields.
func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func newTurnID(seed string) string {
	if seed != "" {
		return seed
	}
	return time.Now().UTC().Format("20060102T150405.000000000")
}
