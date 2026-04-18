package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

type sessionHandler struct {
	mgr    *session.Manager
	logger *slog.Logger
}

type createResponse struct {
	SessionID  string `json:"session_id"`
	PTYPath    string `json:"pty_path"`
	WizardPath string `json:"wizard_path"`
}

func (h *sessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	s, err := h.mgr.Create(ctx)
	if err != nil {
		if errors.Is(err, session.ErrAtCapacity) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		h.logger.Error("create session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp := createResponse{
		SessionID:  s.ID,
		PTYPath:    "/sessions/" + s.ID + "/pty",
		WizardPath: "/sessions/" + s.ID + "/wizard",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *sessionHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.mgr.Destroy(id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PTY upgrades to WebSocket and bridges the browser to the VM's serial console.
// Binary frames carry raw bytes in both directions.
func (h *sessionHandler) PTY(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s, ok := h.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// MVP: no origin check. Tighten when learn.example.com is in production.
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Warn("ws accept", "err", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	replay, updates, detach := s.Serial.Attach()
	defer detach()

	// Send the boot-time replay first so clients see the state of the VM.
	if len(replay) > 0 {
		if werr := c.Write(ctx, websocket.MessageBinary, replay); werr != nil {
			return
		}
	}

	// VM -> browser (via tee broadcast)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-updates:
				if !ok {
					cancel()
					return
				}
				if werr := c.Write(ctx, websocket.MessageBinary, data); werr != nil {
					cancel()
					return
				}
			}
		}
	}()

	// browser -> VM
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		if _, werr := s.Process.Stdin().Write(data); werr != nil {
			h.logger.Debug("ws->vm write", "err", werr)
			return
		}
	}
}
