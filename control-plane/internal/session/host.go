package session

import (
	"context"
	"errors"
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
	return h.CreateWith(ctx, CreateOptions{})
}

// CreateWith launches a session using the given options. If the underlying
// Launcher is an OptionsLauncher (the production Manager via managerLauncher)
// the options are forwarded; otherwise they are ignored (tests / mocks).
func (h *Host) CreateWith(ctx context.Context, opts CreateOptions) (*Session, error) {
	h.mu.Lock()
	if len(h.sessions) >= h.maxLive {
		h.mu.Unlock()
		return nil, ErrAtCapacity
	}
	h.mu.Unlock()

	var (
		s   *Session
		err error
	)
	if ol, ok := h.launcher.(OptionsLauncher); ok {
		s, err = ol.LaunchWith(ctx, opts)
	} else {
		s, err = h.launcher.Launch(ctx)
	}
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
	h.watch(s)
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
			h.watch(s)
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

// watch drops s from the live map once its done channel closes, so a VM
// that exits on its own (guest shutdown, crash) frees Host capacity without
// waiting for a DELETE. Manager.Destroy is what closes the channel on that
// path, via the reaper goroutine in Manager.CreateWith.
func (h *Host) watch(s *Session) {
	done := s.Done()
	if done == nil {
		return
	}
	go func() {
		<-done
		h.mu.Lock()
		if cur, ok := h.sessions[s.ID]; ok && cur == s {
			delete(h.sessions, s.ID)
		}
		h.mu.Unlock()
	}()
}

// Destroy removes the session from the host and tears its VM down.
//
// Sessions created by the production Manager carry a back-pointer to it, and
// teardown goes through Manager.Destroy: stop Firecracker, delete the TAP,
// release the IP, remove the VM directory. Before this, Host.Destroy only
// dropped the map entry and closed the done channel, so every DELETE leaked a
// running VM and the host wedged at MaxConcurrent.
//
// Manager.Destroy returning ErrNotFound means the reaper already tore the VM
// down after it exited on its own; the caller still gets nil because the
// session is gone either way. Sessions without a manager (mock launcher,
// tests) only get their done channel closed.
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
	if s.mgr != nil {
		if err := s.mgr.Destroy(s.ID); err != nil && !errors.Is(err, ErrNotFound) {
			s.CloseDone()
			h.logger.Error("session teardown failed", "id", id, "err", err)
			return err
		}
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
