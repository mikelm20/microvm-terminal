package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
)

// Wizard handles GET /sessions/{id}/wizard. It's a WebSocket that streams
// guest-agent events (process_started, port_listening, etc.) to the browser
// so the sidebar UI can advance.
func (h *sessionHandler) Wizard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s, ok := h.mgr.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		h.logger.Warn("wizard ws accept", "err", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Subscribe: get the history replay first, then stream updates.
	snap, updates, detach := s.Events.Subscribe(32)
	defer detach()

	// Replay
	for _, e := range snap {
		b, _ := json.Marshal(e)
		if werr := c.Write(ctx, websocket.MessageText, b); werr != nil {
			return
		}
	}

	// Stream
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-updates:
			if !ok {
				return
			}
			b, _ := json.Marshal(e)
			if werr := c.Write(ctx, websocket.MessageText, b); werr != nil {
				return
			}
		}
	}
}
