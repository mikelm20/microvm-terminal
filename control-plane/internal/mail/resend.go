// Package mail owns the Resend HTTP client and the magic-link email template.
//
// For local dev without a Resend API key, Send() prints the recipient + body
// to stdout so the developer can copy the magic link manually.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Sender sends a single email. Implementations: ResendSender (real),
// StdoutSender (dev fallback).
type Sender interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

// --- Resend ---

type ResendSender struct {
	apiKey string
	from   string
	client *http.Client
	logger *slog.Logger
}

func NewResendSender(apiKey, from string, logger *slog.Logger) *ResendSender {
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger,
	}
}

type resendReq struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html,omitempty"`
}

func (r *ResendSender) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	body, err := json.Marshal(resendReq{
		From: r.from, To: []string{to}, Subject: subject, Text: textBody, HTML: htmlBody,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend http %d: %s", resp.StatusCode, string(b))
	}
	r.logger.Info("email sent", "to", to, "subject", subject)
	return nil
}

// --- Stdout fallback ---

type StdoutSender struct {
	logger *slog.Logger
}

func NewStdoutSender(logger *slog.Logger) *StdoutSender { return &StdoutSender{logger: logger} }

func (s *StdoutSender) Send(_ context.Context, to, subject, textBody, _ string) error {
	fmt.Fprintln(os.Stdout, "==== EMAIL (stdout fallback) ====")
	fmt.Fprintf(os.Stdout, "To:      %s\n", to)
	fmt.Fprintf(os.Stdout, "Subject: %s\n", subject)
	fmt.Fprintln(os.Stdout, strings.Repeat("-", 40))
	fmt.Fprintln(os.Stdout, textBody)
	fmt.Fprintln(os.Stdout, "================================")
	s.logger.Info("email printed to stdout", "to", to, "subject", subject)
	return nil
}

// --- Voice-table backed copy ---

// Voice is the subset of shared/voice/<lang>.json we need for email text.
// We read the whole JSON lazily so the email copy can be edited without
// restarting the server.
var (
	voiceOnce sync.Mutex
	voiceDir  string
)

// SetVoiceDir configures where Voice() looks for es.json / en.json.
// Pass the absolute path to shared/voice.
func SetVoiceDir(dir string) {
	voiceOnce.Lock()
	defer voiceOnce.Unlock()
	voiceDir = dir
}

// readVoice returns the nested JSON object for the lang.
func readVoice(lang string) (map[string]any, error) {
	voiceOnce.Lock()
	dir := voiceDir
	voiceOnce.Unlock()
	if dir == "" {
		return nil, errors.New("voice dir not configured")
	}
	file := filepath.Join(dir, lang+".json")
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func get(root map[string]any, path ...string) string {
	var cur any = root
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}

// MagicLinkEmail renders the subject + body strings for a magic link email.
// Falls back to English text if voice entries are missing.
func MagicLinkEmail(lang, link string) (subject, text, html string) {
	root, err := readVoice(lang)
	if err != nil {
		root = map[string]any{}
	}
	// Fallback hard-coded copy if the voice table doesn't have an `email.*`
	// entry yet. Keeps dev unblocked while shared/voice is still being filled.
	subject = get(root, "email", "magic_link", "subject")
	if subject == "" {
		if lang == "en" {
			subject = "Your learn link"
		} else {
			subject = "Tu enlace de acceso"
		}
	}
	intro := get(root, "email", "magic_link", "body")
	if intro == "" {
		if lang == "en" {
			intro = "Tap the link below to sign in. It expires in 10 minutes."
		} else {
			intro = "Pulsa el enlace de abajo para entrar. Expira en diez minutos."
		}
	}
	text = intro + "\n\n" + link + "\n"
	html = fmt.Sprintf("<p>%s</p><p><a href=\"%s\">%s</a></p>", intro, link, link)
	return
}
