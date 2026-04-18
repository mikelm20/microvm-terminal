package session

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// WarmPool keeps a small number of pre-booted VMs ready to be handed out
// when a POST /sessions call arrives with `warm=true`. When the pool is
// below capacity it launches a replacement in the background.
//
// The pool is best-effort; AcquireWarm returns (nil, false) if no ready VM
// is available and the caller falls back to a cold launch.
type WarmPool struct {
	launcher Launcher
	logger   *slog.Logger
	target   int

	mu    sync.Mutex
	ready []*Session

	replenish chan struct{}
	stop      chan struct{}
}

// NewWarmPool creates a pool targeting `target` ready VMs.
func NewWarmPool(launcher Launcher, target int, logger *slog.Logger) *WarmPool {
	if target <= 0 {
		target = 2
	}
	p := &WarmPool{
		launcher:  launcher,
		logger:    logger,
		target:    target,
		replenish: make(chan struct{}, target*2),
		stop:      make(chan struct{}),
	}
	return p
}

// Start begins the replenishment loop.
func (p *WarmPool) Start() {
	go p.loop()
	// Seed the pool.
	for i := 0; i < p.target; i++ {
		select {
		case p.replenish <- struct{}{}:
		default:
		}
	}
}

// Stop signals the replenish loop to exit. Does not tear down in-flight VMs.
func (p *WarmPool) Stop() { close(p.stop) }

// AcquireWarm pops a ready VM from the pool, if any. Triggers a background
// refill so the pool heals toward target.
func (p *WarmPool) AcquireWarm(_ context.Context) (*Session, bool) {
	p.mu.Lock()
	if len(p.ready) == 0 {
		p.mu.Unlock()
		// Still poke replenish in case we were below target for unrelated reasons.
		select {
		case p.replenish <- struct{}{}:
		default:
		}
		return nil, false
	}
	s := p.ready[0]
	p.ready = p.ready[1:]
	p.mu.Unlock()
	select {
	case p.replenish <- struct{}{}:
	default:
	}
	return s, true
}

// Size returns the number of ready VMs currently in the pool.
func (p *WarmPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.ready)
}

func (p *WarmPool) loop() {
	for {
		select {
		case <-p.stop:
			return
		case <-p.replenish:
			p.mu.Lock()
			have := len(p.ready)
			p.mu.Unlock()
			if have >= p.target {
				continue
			}
			// Launch a fresh VM to grow the pool.
			go p.buildOne()
		}
	}
}

func (p *WarmPool) buildOne() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s, err := p.launcher.Launch(ctx)
	if err != nil {
		p.logger.Warn("warm pool launch failed", "err", err)
		return
	}
	p.mu.Lock()
	p.ready = append(p.ready, s)
	p.mu.Unlock()
	p.logger.Info("warm pool grew", "size", len(p.ready))
}
