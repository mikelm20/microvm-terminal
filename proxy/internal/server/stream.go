package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Usage pairs the token counts reported in an Anthropic response.
type Usage struct {
	RequestTokens  int64
	ResponseTokens int64
}

// streamBody copies the upstream body to the client while sniffing token
// usage from the payload. For SSE streams it scans each event line; for
// application/json it parses the whole body once. Returns what the proxy
// observed. Errors are non-fatal; the client still receives bytes.
func streamBody(w io.Writer, body io.Reader, flusher http.Flusher, isSSE bool) (Usage, []byte, error) {
	if isSSE {
		return streamSSE(w, body, flusher)
	}
	return streamJSON(w, body, flusher)
}

// streamSSE forwards text/event-stream bytes as they arrive. Anthropic emits
// `event: message_start` and `event: message_delta` frames that carry a
// `usage` object in their `data:` JSON. We parse only those without
// buffering the full stream.
func streamSSE(w io.Writer, body io.Reader, flusher http.Flusher) (Usage, []byte, error) {
	var usage Usage
	r := bufio.NewReaderSize(body, 64*1024)
	var captured bytes.Buffer
	const captureCap = 32 * 1024

	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if captured.Len() < captureCap {
				remain := captureCap - captured.Len()
				if len(line) < remain {
					captured.Write(line)
				} else {
					captured.Write(line[:remain])
				}
			}
			if _, werr := w.Write(line); werr != nil {
				return usage, captured.Bytes(), werr
			}
			// Flush after every complete SSE frame (blank line separates
			// events). Flushing per-chunk is cheap at this scale and keeps
			// first-token latency aligned with what the upstream emits.
			if flusher != nil && (len(line) == 1 && line[0] == '\n') {
				flusher.Flush()
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				data := bytes.TrimSpace(line[len("data:"):])
				usage = mergeUsage(usage, parseUsage(data))
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if flusher != nil {
					flusher.Flush()
				}
				return usage, captured.Bytes(), nil
			}
			return usage, captured.Bytes(), err
		}
	}
}

// streamJSON handles non-streaming /v1/messages (and anything else). We copy
// bytes through while also tee'ing into a capped buffer for usage parsing.
func streamJSON(w io.Writer, body io.Reader, flusher http.Flusher) (Usage, []byte, error) {
	var buf bytes.Buffer
	const cap = 1 << 20 // 1 MiB
	tee := io.TeeReader(body, &limitedWriter{W: &buf, N: cap})
	if _, err := io.Copy(w, tee); err != nil {
		if flusher != nil {
			flusher.Flush()
		}
		return Usage{}, buf.Bytes(), err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return parseUsage(buf.Bytes()), buf.Bytes(), nil
}

// parseUsage extracts input_tokens + output_tokens from a JSON blob. It is
// forgiving: malformed or unrelated payloads yield zeroes rather than an
// error. Anthropic shape:
//
//	{"usage":{"input_tokens":42,"output_tokens":73,...}}
func parseUsage(b []byte) Usage {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' {
		return Usage{}
	}
	var outer struct {
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
		// message_start frames wrap the usage inside a `message` object.
		Message struct {
			Usage struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		// message_delta frames may also include usage at the top level.
		Delta struct {
			Usage struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(b, &outer); err != nil {
		return Usage{}
	}
	return Usage{
		RequestTokens:  max64(outer.Usage.InputTokens, outer.Message.Usage.InputTokens, outer.Delta.Usage.InputTokens),
		ResponseTokens: max64(outer.Usage.OutputTokens, outer.Message.Usage.OutputTokens, outer.Delta.Usage.OutputTokens),
	}
}

// mergeUsage is additive for output tokens (SSE emits incremental deltas)
// and max for input tokens (constant across a turn). This matches what the
// Anthropic API reports.
func mergeUsage(a, b Usage) Usage {
	out := Usage{
		RequestTokens:  a.RequestTokens,
		ResponseTokens: a.ResponseTokens,
	}
	if b.RequestTokens > out.RequestTokens {
		out.RequestTokens = b.RequestTokens
	}
	if b.ResponseTokens > out.ResponseTokens {
		out.ResponseTokens = b.ResponseTokens
	}
	return out
}

func max64(xs ...int64) int64 {
	var m int64
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

// limitedWriter writes up to N bytes then drops the rest silently.
type limitedWriter struct {
	W io.Writer
	N int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.N <= 0 {
		return len(p), nil
	}
	if len(p) > lw.N {
		_, _ = lw.W.Write(p[:lw.N])
		lw.N = 0
		return len(p), nil
	}
	n, err := lw.W.Write(p)
	lw.N -= n
	return n, err
}

// estimateCostMicros uses Anthropic's published Sonnet 4.7 list price:
// $3 per 1M input tokens, $15 per 1M output tokens. 1 USD = 1_000_000 micros.
// This is a planning estimate. Canonical cost comes from Anthropic's billing
// export; this column lets Finance spot outliers without waiting for a month.
func estimateCostMicros(u Usage) int64 {
	in := u.RequestTokens * 3
	out := u.ResponseTokens * 15
	return in + out
}

