package session

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// Host is a SessionHost implementation that serves cold via Manager.Create
// and warm via a WarmPool. When the pool misses, it falls back to cold.
//
// Used in production (Manager backed by real Firecracker) and in the mock
// launcher flavor used by tests.
type Host struct {
	launcher Launcher
	pool     *WarmPool
	logger   *slog.Logger
	maxLive  int

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewHost(launcher Launcher, pool *WarmPool, maxLive int, logger *slog.Logger) *Host {
	if maxLive <= 0 {
		maxLive = 3
	}
	return &Host{
		launcher: launcher,
		pool:     pool,
		logger:   logger,
		maxLive:  maxLive,
		sessions: make(map[string]*Session),
	}
}

func (h *Host) Create(ctx context.Context) (*Session, error) {
	h.mu.Lock()
	if len(h.sessions) >= h.maxLive {
		h.mu.Unlock()
		return nil, ErrAtCapacity
	}
	h.mu.Unlock()

	s, err := h.launcher.Launch(ctx)
	if err != nil {
		return nil, err
	}
	if s.Events == nil {
		s.Events = NewEventBus()
	}
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	h.mu.Lock()
	h.sessions[s.ID] = s
	h.mu.Unlock()
	return s, nil
}

func (h *Host) CreateWarm(ctx context.Context) (*Session, error) {
	if h.pool != nil {
		if s, ok := h.pool.AcquireWarm(ctx); ok {
			h.mu.Lock()
			if len(h.sessions) >= h.maxLive {
				h.mu.Unlock()
				return nil, ErrAtCapacity
			}
			if s.ID == "" {
				s.ID = uuid.NewString()
			}
			if s.Events == nil {
				s.Events = NewEventBus()
			}
			s.Warm = true
			h.sessions[s.ID] = s
			h.mu.Unlock()
			return s, nil
		}
	}
	return h.Create(ctx)
}

func (h *Host) Get(id string) (*Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	return s, ok
}

func (h *Host) Destroy(id string) error {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	h.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	s.CloseDone()
	h.logger.Info("session destroyed", "id", id)
	return nil
}

func (h *Host) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sessions)
}

func (h *Host) WarmPoolSize() int {
	if h.pool == nil {
		return 0
	}
	return h.pool.Size()
}
