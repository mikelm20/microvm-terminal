// Package streamjson decodes the stream-json output emitted by
// `claude --output-format stream-json`. One JSON object per line, with a
// discriminator field `type` of system | user | assistant | result.
//
// See Claude Code CLI docs. The shape below is deliberately permissive: we
// only pin the fields we route into downstream events. Unknown fields are
// ignored, unknown content block types fall through as "Other" so we never
// drop a turn on shape drift.
package streamjson

import "encoding/json"

// Envelope is the outer shape of every JSONL line emitted by stream-json.
type Envelope struct {
	Type     string          `json:"type"`
	Subtype  string          `json:"subtype,omitempty"`
	Message  *Message        `json:"message,omitempty"`
	Session  string          `json:"session_id,omitempty"`
	Duration *int64          `json:"duration_ms,omitempty"`
	Result   string          `json:"result,omitempty"`
	IsError  bool            `json:"is_error,omitempty"`
	NumTurns *int            `json:"num_turns,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// Message holds the assistant or user message body.
type Message struct {
	ID      string         `json:"id,omitempty"`
	Role    string         `json:"role,omitempty"`
	Model   string         `json:"model,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
}

// ContentBlock is a discriminated union. Fields not matching the type are
// zero-valued.
type ContentBlock struct {
	Type string `json:"type"`

	// text blocks
	Text string `json:"text,omitempty"`

	// tool_use blocks (assistant)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result blocks (user)
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

// DecodeContent tolerates content being either a string, an array of
// strings/text blocks, or a single object. It returns the best-effort plain
// text form for display in summaries.
func DecodeContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of blocks
	var arr []ContentBlock
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := ""
		for _, b := range arr {
			if b.Text != "" {
				if out != "" {
					out += "\n"
				}
				out += b.Text
			}
		}
		return out
	}
	// Fall back to raw
	return string(raw)
}
