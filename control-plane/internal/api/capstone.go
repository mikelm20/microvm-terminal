package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/apitypes"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
	"github.com/mikelm20/learn-platform/control-plane/internal/firecracker"
	"github.com/mikelm20/learn-platform/control-plane/internal/identity"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
	"gopkg.in/yaml.v3"
)

// capstoneHandler serves the two endpoints the F2 Lab Capstone frontend
// calls after the learner types a brief. Routes:
//
//   POST /capstone/validate  ->  short-lived validator VM, returns {ok, reason?}
//   POST /capstone/build     ->  long-lived builder VM, returns session metadata
//
// Both endpoints rely on the per-VM RootfsOverrides plumbed through
// session.CreateOptions. The validator uses the learn-validator lesson's
// system_prompt; the builder uses lab-capstone's. Neither endpoint touches
// the warm pool: validator VMs are too short-lived to warm up for, and
// builder VMs need per-brief overrides that a pre-booted VM can't carry.
//
// The validator runs with the same isolation posture as Pilar 3 regla-oro
// today. That is a documented v1 risk; see the capstone lesson notes.
type capstoneHandler struct {
	deps Deps
}

// Shared prompt-size guards, mirrored (loosely) from the frontend. The
// schema enforces <=500 client-side; server-side we double-check and trim
// surrounding whitespace so Claude doesn't see a trailing \r.
const (
	minBriefChars = 10
	maxBriefChars = 500
	maxBriefLines = 6
)

// unsafeTokens is the server-side keyword blacklist applied BEFORE we
// spend a validator VM. Kept conservative; the real defense is the VM
// validator responding with JSON. Catches the obvious "ignore all previous
// instructions" / fetch-the-world variants.
var unsafeTokens = regexp.MustCompile(
	`(?i)\b(fetch\(|xmlhttprequest|axios|subprocess|exec\(|eval\(|import\s+os|child_process|fs\.write|chmod\b|\bsudo\b|curl\s+https?|npm\s+install|pip\s+install)\b`,
)

// anyURL is RE2-friendly (no negative lookahead). We accept any URL
// match and then manually filter out learn.example.com / api.learn.example.com
// hosts in sanitizeBrief.
var anyURL = regexp.MustCompile(`(?i)https?://([a-z0-9.\-]+)`)

// sanitizeBrief applies the synchronous filters documented in
// CAPSTONE_DESIGN.md section 1.2 step 4. Returns a trimmed prompt + nil
// error on success, or a caller-facing reason string on rejection.
func sanitizeBrief(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if len(s) < minBriefChars {
		return "", "Tu brief es muy corto. Describe en una o dos frases lo que quieres ver."
	}
	if len(s) > maxBriefChars {
		return "", "Tu brief es demasiado largo. Resume en 2 a 3 lineas."
	}
	if strings.Count(s, "\n") >= maxBriefLines {
		return "", "Demasiadas lineas. Usa maximo 3 o 4."
	}
	// Printable UTF-8 only. The standard library treats control chars as
	// non-printable; we allow \n but nothing else below 0x20.
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return "", "Tu brief contiene caracteres no permitidos."
		}
	}
	if unsafeTokens.MatchString(s) {
		return "", "Tu brief contiene instrucciones fuera de scope (red, shell, dependencias). Pide una mini-app visual de una pagina."
	}
	// URLs: allow learn.example.com / api.learn.example.com, reject anything else.
	// (Go regex engine is RE2; no negative lookahead, so we filter manually.)
	for _, m := range anyURL.FindAllStringSubmatch(s, -1) {
		host := strings.ToLower(m[1])
		if host != "learn.example.com" && host != "api.learn.example.com" {
			return "", "No se permiten URLs externas en el brief."
		}
	}
	return s, ""
}

// Validate handles POST /capstone/validate. Spins up a learn-validator VM,
// sends the learner's brief, parses the first assistant message as JSON,
// then tears the VM down. Cost is ~1 boot + 1 Claude turn per call.
func (h *capstoneHandler) Validate(w http.ResponseWriter, r *http.Request) {
	_, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	var req apitypes.CapstoneValidateRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	brief, reason := sanitizeBrief(req.Prompt)
	if reason != "" {
		writeJSON(w, http.StatusOK, apitypes.CapstoneValidateResponse{
			Ok:     false,
			Reason: strPtr(reason),
		})
		return
	}
	lang := "es"
	if req.Lang != nil && *req.Lang != "" {
		lang = *req.Lang
	}

	sysPrompt, err := h.loadSystemPrompt("learn-validator", lang)
	if err != nil {
		h.deps.Logger.Error("capstone validate: load system prompt", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "lesson load failed")
		return
	}

	// We cap the whole validate flow at 75s: cold boot (~30s worst case) +
	// Claude turn (~10s single line JSON) + teardown. If this slips, the
	// UI shows "retry" rather than hanging.
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()

	s, err := h.deps.Host.CreateWith(ctx, session.CreateOptions{
		RootfsOverrides: firecracker.RootfsOverrides{
			SystemPrompt: sysPrompt,
			LearnCwd:     "/home/learner",
		},
	})
	if err != nil {
		if errors.Is(err, session.ErrAtCapacity) {
			apiErrorRetry(w, r, http.StatusServiceUnavailable, apitypes.ErrCapacityFull, "capacity full", 15)
			return
		}
		h.deps.Logger.Error("capstone validate: create vm", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrVMLaunchFailed, "vm launch failed")
		return
	}
	defer func() {
		if dErr := h.deps.Host.Destroy(s.ID); dErr != nil && !errors.Is(dErr, session.ErrNotFound) {
			h.deps.Logger.Warn("capstone validate: destroy vm", "err", dErr, "session", s.ID)
		}
	}()

	resp, cerr := callValidator(ctx, s, brief)
	if cerr != nil {
		h.deps.Logger.Warn("capstone validate: call validator", "err", cerr)
		writeJSON(w, http.StatusOK, apitypes.CapstoneValidateResponse{
			Ok:     false,
			Reason: strPtr("No pude interpretar la respuesta. Reescribe tu brief en una frase clara."),
		})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Build handles POST /capstone/build. Spins up a lab-capstone VM primed
// with the builder system prompt + cwd /home/learner/app, submits the
// injection-wrapped learner brief, and returns session metadata. The
// frontend then listens on the wizard WS for port_listening, then mounts
// the preview iframe.
func (h *capstoneHandler) Build(w http.ResponseWriter, r *http.Request) {
	id, idOK := identity.FromContext(r.Context())
	if !idOK {
		apiError(w, r, http.StatusUnauthorized, apitypes.ErrAuthRequired, "identity required")
		return
	}
	var req apitypes.CapstoneBuildRequest
	if err := decodeJSON(r, &req); err != nil {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, "invalid json")
		return
	}
	brief, reason := sanitizeBrief(req.Prompt)
	if reason != "" {
		apiError(w, r, http.StatusBadRequest, apitypes.ErrBadRequest, reason)
		return
	}
	lang := "es"
	if req.Lang != nil && *req.Lang != "" {
		lang = *req.Lang
	}

	sysPrompt, err := h.loadSystemPrompt("lab-capstone", lang)
	if err != nil {
		h.deps.Logger.Error("capstone build: load system prompt", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "lesson load failed")
		return
	}

	// Builder VM boot deadline only. Once the VM is up and the prompt has
	// been written to serial, the frontend owns the wait (tailing the
	// wizard WS for port_listening).
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	s, err := h.deps.Host.CreateWith(ctx, session.CreateOptions{
		RootfsOverrides: firecracker.RootfsOverrides{
			SystemPrompt: sysPrompt,
			LearnCwd:     "/home/learner/app",
		},
	})
	if err != nil {
		if errors.Is(err, session.ErrAtCapacity) {
			apiErrorRetry(w, r, http.StatusServiceUnavailable, apitypes.ErrCapacityFull, "capacity full", 30)
			return
		}
		h.deps.Logger.Error("capstone build: create vm", "err", err)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrVMLaunchFailed, "vm launch failed")
		return
	}

	// Persist the session row so /sessions/{id}/... auth works for the
	// frontend (preview + wizard WS gate on sessionForOwner).
	s.IdentityUUID = id
	s.LessonID = "lab-capstone"
	s.Lang = lang
	ipStr := ""
	if s.IP.IsValid() {
		ipStr = s.IP.String()
	}
	sid, perr := uuid.Parse(s.ID)
	if perr != nil {
		sid = uuid.New()
		s.ID = sid.String()
	}
	row := db.Session{
		ID:           sid,
		IdentityUUID: id,
		LessonID:     "lab-capstone",
		Lang:         lang,
		Warm:         false,
	}
	if ipStr != "" {
		row.VMIP = sqlNullStringFrom(ipStr)
	}
	if err := h.deps.Store.InsertSession(r.Context(), row); err != nil {
		h.deps.Logger.Error("capstone build: insert session", "err", err)
		_ = h.deps.Host.Destroy(s.ID)
		apiError(w, r, http.StatusInternalServerError, apitypes.ErrInternal, "internal")
		return
	}

	if h.deps.Transcript != nil {
		h.deps.Transcript.Start(context.Background(), sid, s.Events)
	}

	// Wire up the files_snapshot trigger BEFORE submitting the prompt so
	// we don't race with port_listening arriving first.
	go watchPortAndSnapshot(s, h.deps.Logger, 3000)

	s.Events.Publish(session.GuestEvent{
		Type: "session_started",
		TS:   time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"session_id": s.ID,
			"lesson_id":  "lab-capstone",
		},
	})

	// Build the injection-wrapped prompt and write it to the VM serial.
	// claude-wrap reads stdin one line at a time, so we collapse newlines
	// in the brief itself (the template already expects a single line).
	oneLine := strings.TrimSpace(briefOneLine(brief))
	full := fmt.Sprintf("Construye ahora la mini-app descrita en este brief (no respondas nada mas, ponte a trabajar): %q", oneLine)
	// Feed the line to the learner's shell. claude-wrap trims and forwards.
	if s.Process != nil {
		_, werr := s.Process.Stdin().Write([]byte(full + "\n"))
		if werr != nil {
			h.deps.Logger.Error("capstone build: write prompt", "err", werr)
		}
	}
	turnID := uuid.NewString()
	s.Events.Publish(session.GuestEvent{
		Type: "claude_prompt_sent",
		TS:   time.Now().UTC().Format(time.RFC3339Nano),
		Payload: map[string]any{
			"text":    full,
			"length":  len(full),
			"turn_id": turnID,
		},
	})

	origin := strings.TrimRight(h.deps.PublicOrigin, "/")
	wsOrigin := strings.Replace(origin, "http://", "ws://", 1)
	wsOrigin = strings.Replace(wsOrigin, "https://", "wss://", 1)

	writeJSON(w, http.StatusOK, apitypes.CapstoneBuildResponse{
		SessionID:          s.ID,
		VMIP:               ipStr,
		WizardWSURL:        wsOrigin + "/sessions/" + s.ID + "/ws",
		PreviewURLTemplate: origin + "/sessions/" + s.ID + "/preview/{port}",
		TurnID:             turnID,
	})
}

// briefOneLine replaces newlines with spaces so the injection template
// reaches claude-wrap as a single stdin line. Whitespace is collapsed too.
func briefOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	// Collapse runs of whitespace.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// callValidator writes the brief to the validator VM's serial and waits for
// the first assistant claude_message event. The text is expected to be a
// single JSON line like {"ok":true} or {"ok":false,"reason":"..."}.
// Anything else is treated as a soft failure (caller returns a default
// "no pude interpretar" reason).
func callValidator(ctx context.Context, s *session.Session, brief string) (apitypes.CapstoneValidateResponse, error) {
	// The wizard bus is where claude-wrap publishes claude_message; that's
	// the canonical place to read the validator's reply from.
	_, updates, detach := s.Events.Subscribe(32)
	defer detach()

	oneLine := briefOneLine(brief)
	if s.Process == nil {
		return apitypes.CapstoneValidateResponse{}, errors.New("vm has no stdin")
	}
	if _, err := s.Process.Stdin().Write([]byte(oneLine + "\n")); err != nil {
		return apitypes.CapstoneValidateResponse{}, fmt.Errorf("write prompt: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		select {
		case <-waitCtx.Done():
			return apitypes.CapstoneValidateResponse{}, errors.New("validator timeout")
		case ev, ok := <-updates:
			if !ok {
				return apitypes.CapstoneValidateResponse{}, errors.New("event bus closed")
			}
			if ev.Type != "claude_message" {
				continue
			}
			role, _ := ev.Payload["role"].(string)
			if role != "assistant" {
				continue
			}
			text, _ := ev.Payload["text"].(string)
			text = extractFirstJSONLine(text)
			if text == "" {
				continue
			}
			var parsed struct {
				Ok     bool    `json:"ok"`
				Reason *string `json:"reason,omitempty"`
			}
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				return apitypes.CapstoneValidateResponse{}, fmt.Errorf("unmarshal validator json: %w", err)
			}
			return apitypes.CapstoneValidateResponse{Ok: parsed.Ok, Reason: parsed.Reason}, nil
		}
	}
}

// extractFirstJSONLine returns the first line of s that looks like a JSON
// object. Claude occasionally wraps a reply with leading whitespace or a
// trailing newline; this keeps validators forgiving without loosening the
// strict-JSON rule on the system prompt side.
func extractFirstJSONLine(s string) string {
	s = strings.TrimSpace(s)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			return line
		}
	}
	// Fall through: maybe the whole blob is valid JSON.
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return s
	}
	return ""
}

// loadSystemPrompt reads lessons/<id>.<lang>.yml, walks steps, and returns
// the first step.system_prompt found. Falls back to the es.yml if the
// requested lang is missing. Small and cached via the existing lessonCache
// protections; we piggyback on parseLessonFile.
func (h *capstoneHandler) loadSystemPrompt(lessonID, lang string) (string, error) {
	capstoneSystemPromptMu.Lock()
	defer capstoneSystemPromptMu.Unlock()
	key := lessonID + "." + lang
	if v, ok := capstoneSystemPromptCache[key]; ok {
		return v, nil
	}
	candidates := []string{
		filepath.Join(h.deps.LessonsDir, lessonID+"."+lang+".yml"),
		filepath.Join(h.deps.LessonsDir, lessonID+".yml"),
	}
	var raw map[string]any
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if uerr := yaml.Unmarshal(b, &raw); uerr == nil {
			break
		}
	}
	if raw == nil {
		return "", fmt.Errorf("lesson %s.%s not found", lessonID, lang)
	}
	steps, _ := raw["steps"].([]any)
	for _, st := range steps {
		m, ok := st.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := m["system_prompt"].(string); ok && strings.TrimSpace(v) != "" {
			capstoneSystemPromptCache[key] = v
			return v, nil
		}
	}
	return "", fmt.Errorf("lesson %s.%s has no system_prompt", lessonID, lang)
}

var (
	capstoneSystemPromptMu    sync.Mutex
	capstoneSystemPromptCache = make(map[string]string)
)

func strPtr(s string) *string { return &s }

func sqlNullStringFrom(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// watchPortAndSnapshot subscribes to the session event bus and, on the
// first port_listening event matching `targetPort`, asks the guest-agent
// for a files snapshot of /home/learner/app and publishes the
// `files_snapshot` event on the same bus. Runs in its own goroutine with
// a hard deadline.
func watchPortAndSnapshot(s *session.Session, logger interface{ Warn(string, ...any) }, targetPort int) {
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	_, updates, detach := s.Events.Subscribe(32)
	defer detach()
	for {
		select {
		case <-deadline.C:
			return
		case <-s.Done():
			return
		case ev, ok := <-updates:
			if !ok {
				return
			}
			if ev.Type != "port_listening" {
				continue
			}
			portF, _ := ev.Payload["port"].(float64)
			if int(portF) != targetPort {
				continue
			}
			req := map[string]any{
				"type": "snapshot_files",
				"payload": map[string]any{
					"root":        "/home/learner/app",
					"max_bytes":   64 * 1024,
					"max_files":   32,
					"max_depth":   4,
				},
			}
			if err := s.SendToGuest(req); err != nil {
				if logger != nil {
					logger.Warn("capstone: snapshot request failed", "err", err)
				}
			}
			return
		}
	}
}
