// claude-wrap is a thin wrapper around Claude Code that runs inside each
// Firecracker VM. It replaces `claude` in learn-shell.sh. Responsibilities:
//
//  1. Spawn `claude --dangerously-skip-permissions --output-format stream-json
//     --input-format stream-json --verbose` and pipe its stdout through the
//     parser.
//  2. Read user input line by line from our stdin. On each submission, emit a
//     claude_prompt_sent event and forward the text to claude's stdin wrapped
//     as a stream-json user message.
//  3. Render assistant text and tool-call hints back to the user's terminal
//     so the interactive experience stays useful even though the underlying
//     protocol is stream-json.
//  4. Publish every canonical event to a Unix socket the guest-agent tails.
//
// Design tradeoff (documented per CONTRACTS.md): we do NOT run the full
// interactive Claude UI. The rich box-drawing interactive TTY assumes a real
// terminal; stream-json gives us deterministic events. Learners still see a
// readable chat; they do not get the box-drawing picker. The predicate spine
// is deterministic, which is the load-bearing property.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/internal/emitter"
	"github.com/mikelm20/learn-platform/vm-image/claude-wrap/parser"
)

var (
	socketPath       = flag.String("socket", "/run/learn/claude-wrap.sock", "unix socket path for guest-agent to tail")
	claudeBin        = flag.String("claude", "claude", "path to claude binary")
	cwd              = flag.String("cwd", "", "working directory for claude process")
	echoText         = flag.Bool("echo-text", true, "echo assistant text + tool notices back to user stdout")
	systemPromptFile = flag.String("system-prompt-file", "", "path to a file whose contents become --append-system-prompt to claude; layered on top of any baked CLAUDE.md")
	cmdlineExtra     string
)

func main() {
	flag.StringVar(&cmdlineExtra, "extra", "", "extra flags passed verbatim to claude")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stderr)

	em := emitter.New(*socketPath)
	go func() {
		if err := em.Run(); err != nil {
			log.Printf("emitter run: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// NB: Claude Code expects --verbose alongside stream-json for turn boundaries.
	args := []string{
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
	}
	if *systemPromptFile != "" {
		buf, err := os.ReadFile(*systemPromptFile)
		if err != nil {
			fatal("read system prompt file %s: %v", *systemPromptFile, err)
		}
		text := strings.TrimSpace(string(buf))
		if text != "" {
			args = append(args, "--append-system-prompt", text)
		}
	}
	if cmdlineExtra != "" {
		args = append(args, strings.Fields(cmdlineExtra)...)
	}

	cmd := exec.CommandContext(ctx, *claudeBin, args...)
	if *cwd != "" {
		cmd.Dir = *cwd
	}
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fatal("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fatal("stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fatal("start claude: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// stdout parse loop
	go func() {
		defer wg.Done()
		parseLoop(stdout, em)
	}()

	// stdin forward loop
	go func() {
		defer wg.Done()
		forwardLoop(os.Stdin, stdin, em)
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("claude exited: %v", err)
	}
	stop()
	em.Stop()
	wg.Wait()
}

// parseLoop reads one JSON line at a time from claude's stdout, feeds it to
// the parser, emits each resulting event to the unix socket, and echoes a
// compact human line to the user's terminal.
func parseLoop(r io.Reader, em *emitter.Emitter) {
	p := parser.New()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		events := p.Feed(line)
		for _, ev := range events {
			em.Send(ev)
			if *echoText {
				echoForUser(ev)
			}
		}
	}
	if err := sc.Err(); err != nil {
		log.Printf("stdout scan: %v", err)
	}
}

// forwardLoop reads user input line by line from our stdin, wraps it as a
// stream-json user message, writes to claude's stdin, and mirrors the prompt
// as a claude_prompt_sent event so downstream wizard logic can see it even
// before Claude's own echo arrives.
func forwardLoop(userIn io.Reader, claudeIn io.WriteCloser, em *emitter.Emitter) {
	sc := bufio.NewScanner(userIn)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	enc := json.NewEncoder(claudeIn)

	var pending strings.Builder
	for sc.Scan() {
		line := sc.Text()
		// Submit on empty line (double enter) or when line ends with a
		// trailing backslash-stripped continuation heuristic. Simplest
		// behavior: every non-empty line is one submission.
		if line == "" {
			continue
		}
		// Multi-line support: user may end with the literal token "<<<" to
		// force submit of a paragraph; otherwise each line is a prompt.
		if line == "<<<" {
			submit(pending.String(), enc, em)
			pending.Reset()
			continue
		}
		if isSubmitToken(line) {
			text := pending.String()
			pending.Reset()
			submit(text, enc, em)
			continue
		}
		if pending.Len() > 0 {
			pending.WriteString("\n")
		}
		pending.WriteString(line)
		// Single-line default: submit immediately.
		submit(pending.String(), enc, em)
		pending.Reset()
	}
	_ = claudeIn.Close()
}

// isSubmitToken detects the optional marker a learner types to indicate a
// multi-line paragraph is complete. Default ui is single-line-per-submit, but
// we keep this hook in case a client wants an explicit flush.
func isSubmitToken(line string) bool {
	switch strings.TrimSpace(line) {
	case ".send", "/send", "<<<":
		return true
	}
	return false
}

func submit(text string, enc *json.Encoder, em *emitter.Emitter) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	l := len(text)
	em.Send(parser.Event{
		Type:       "claude_prompt_sent",
		TS:         nowStr(),
		Text:       text,
		Length:     &l,
		PromptText: text,
	})
	// stream-json input format: one JSON object per line, type=user.
	msg := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	}
	if err := enc.Encode(msg); err != nil {
		log.Printf("encode user msg: %v", err)
	}
	// Slash-command detection fires an extra event even before the assistant
	// echoes it, because the client wizard may match /agents against this
	// predicate immediately.
	if strings.HasPrefix(text, "/") {
		fields := strings.Fields(text)
		name := strings.TrimPrefix(fields[0], "/")
		var argstr string
		if len(fields) > 1 {
			argstr = strings.Join(fields[1:], " ")
		}
		em.Send(parser.Event{
			Type:        "claude_slash_command",
			TS:          nowStr(),
			CommandName: name,
			CommandArgs: argstr,
		})
	}
}

// echoForUser renders a compact human-readable line for each event so the
// learner still sees progress, not just JSON. A structured client renders a
// richer experience from the structured events; this is for the raw PTY path.
func echoForUser(ev parser.Event) {
	switch ev.Type {
	case "claude_busy":
		if ev.Busy != nil && *ev.Busy {
			fmt.Fprintln(os.Stdout, "[claude is working...]")
		}
	case "claude_tool_call":
		label := ev.Tool
		if ev.Path != "" {
			label = fmt.Sprintf("%s %s", ev.Tool, ev.Path)
		} else if ev.Command != "" {
			cmd := ev.Command
			if len(cmd) > 60 {
				cmd = cmd[:60] + "..."
			}
			label = fmt.Sprintf("%s: %s", ev.Tool, cmd)
		}
		fmt.Fprintf(os.Stdout, "  * %s\n", label)
	case "claude_message":
		if ev.Role == "assistant" && ev.Text != "" {
			fmt.Fprintln(os.Stdout, ev.Text)
		}
	}
}

func nowStr() string { return parser.Now() }

func fatal(format string, args ...any) {
	log.Printf(format, args...)
	os.Exit(1)
}
