package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Host is a SessionHost implementation that serves cold via the Launcher
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

	// onGone is called once for every session that leaves the live map,
	// whatever the reason (DELETE, idle reap, guest shutdown, crash).
	onGone func(*Session)
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

// OnGone registers a callback invoked after a session has left the live
// map. The HTTP layer uses it to stamp reaped_at in Postgres.
func (h *Host) OnGone(fn func(*Session)) {
	h.mu.Lock()
	h.onGone = fn
	h.mu.Unlock()
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

// List returns a snapshot of the live sessions.
func (h *Host) List() []*Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*Session, 0, len(h.sessions))
	for _, s := range h.sessions {
		out = append(out, s)
	}
	return out
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
		removed := false
		if cur, ok := h.sessions[s.ID]; ok && cur == s {
			delete(h.sessions, s.ID)
			removed = true
		}
		fn := h.onGone
		h.mu.Unlock()
		if removed && fn != nil {
			fn(s)
		}
	}()
}

// Destroy removes the session from the host and tears its VM down.
//
// Sessions created by the production Manager carry a back-pointer to it, and
// teardown goes through Manager.Destroy: stop Firecracker, delete the TAP,
// release the IP, remove the VM directory. Manager.Destroy returning
// ErrNotFound means the reaper already tore the VM down after it exited on
// its own; the caller still gets nil because the session is gone either way.
// Sessions without a manager (mock launcher, tests) only get their done
// channel closed.
func (h *Host) Destroy(id string) error {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	fn := h.onGone
	h.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	if fn != nil {
		defer fn(s)
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

// ReapIdle destroys every session that has had no attached terminal client
// for longer than timeout, measured at now. Returns the ids it destroyed.
// A timeout of zero or less disables reaping.
func (h *Host) ReapIdle(now time.Time, timeout time.Duration) []string {
	if timeout <= 0 {
		return nil
	}
	var reaped []string
	for _, s := range h.List() {
		since, idle := s.IdleSince()
		if !idle || now.Sub(since) < timeout {
			continue
		}
		h.logger.Info("reaping idle session", "id", s.ID, "owner", s.Owner, "idle_for", now.Sub(since).Round(time.Second))
		if err := h.Destroy(s.ID); err != nil && !errors.Is(err, ErrNotFound) {
			h.logger.Error("idle reap failed", "id", s.ID, "err", err)
			continue
		}
		reaped = append(reaped, s.ID)
	}
	return reaped
}

// RunIdleReaper calls ReapIdle every interval until ctx is cancelled.
func (h *Host) RunIdleReaper(ctx context.Context, timeout, interval time.Duration) {
	if timeout <= 0 {
		h.logger.Info("idle reaper disabled (idle_timeout_seconds <= 0)")
		return
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	h.logger.Info("idle reaper started", "timeout", timeout, "interval", interval)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			h.ReapIdle(now, timeout)
		}
	}
}
