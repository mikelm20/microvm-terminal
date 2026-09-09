package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func memLauncher() Launcher {
	return LauncherFunc(func(ctx context.Context) (*Session, error) {
		return NewInMemorySession(""), nil
	})
}

// TestCloseDoneConcurrent guards the double-close panic between
// Manager.Destroy, Host.Destroy and the process reaper goroutine.
func TestCloseDoneConcurrent(t *testing.T) {
	s := NewInMemorySession("t")
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.CloseDone()
		}()
	}
	wg.Wait()
	select {
	case <-s.Done():
	default:
		t.Fatal("done channel not closed")
	}
	s.CloseDone()
}

// TestHostDropsSessionOnNaturalExit covers the Host side of teardown: once a
// session's done channel closes, the Host must stop counting it toward
// MaxConcurrent and must fire the OnGone hook exactly once.
func TestHostDropsSessionOnNaturalExit(t *testing.T) {
	h := NewHost(memLauncher(), nil, 1, slog.Default())
	var mu sync.Mutex
	gone := 0
	h.OnGone(func(*Session) { mu.Lock(); gone++; mu.Unlock() })

	s, err := h.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(context.Background()); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("expected ErrAtCapacity, got %v", err)
	}
	s.CloseDone()
	deadline := time.Now().Add(2 * time.Second)
	for h.Count() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("session not dropped from host after done closed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := h.Destroy(s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after natural exit, got %v", err)
	}
	if _, err := h.Create(context.Background()); err != nil {
		t.Fatalf("capacity not freed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gone != 1 {
		t.Fatalf("OnGone fired %d times, want 1", gone)
	}
}

// TestHostDestroyClosesDone covers the manager-less path (mock launcher).
func TestHostDestroyClosesDone(t *testing.T) {
	h := NewHost(memLauncher(), nil, 3, slog.Default())
	s, err := h.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Destroy(s.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("done not closed by Host.Destroy")
	}
	if err := h.Destroy(s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Destroy: expected ErrNotFound, got %v", err)
	}
}

// TestAttachIdleSince checks the bookkeeping the idle reaper relies on.
func TestAttachIdleSince(t *testing.T) {
	s := NewInMemorySession("a")
	since, idle := s.IdleSince()
	if !idle || since.IsZero() {
		t.Fatal("fresh session should be idle since its ready time")
	}
	detach := s.Attach()
	if _, idle := s.IdleSince(); idle {
		t.Fatal("attached session reported idle")
	}
	if s.Attached() != 1 {
		t.Fatalf("attached = %d", s.Attached())
	}
	detach()
	detach() // idempotent
	if s.Attached() != 0 {
		t.Fatalf("attached after detach = %d", s.Attached())
	}
	since2, idle := s.IdleSince()
	if !idle || !since2.After(since) && !since2.Equal(since) {
		t.Fatal("detach should stamp a new idle start")
	}
}

// TestReapIdle verifies only sessions past the timeout with no client are
// destroyed, and attached sessions are left alone however old they are.
func TestReapIdle(t *testing.T) {
	h := NewHost(memLauncher(), nil, 5, slog.Default())
	never, _ := h.Create(context.Background())    // never had a client
	busy, _ := h.Create(context.Background())     // has a client
	detached, _ := h.Create(context.Background()) // had a client, lost it

	detach := busy.Attach()
	defer detach()
	release := detached.Attach()
	release()

	// Simulate age by moving the clock instead of sleeping: at `now` both
	// never and detached have been idle for ten minutes.
	now := time.Now().Add(10 * time.Minute)
	reaped := h.ReapIdle(now, 5*time.Minute)
	if len(reaped) != 2 {
		t.Fatalf("reaped %v, want the two idle sessions only", reaped)
	}
	if _, ok := h.Get(never.ID); ok {
		t.Fatal("never-attached idle session still live")
	}
	if _, ok := h.Get(detached.ID); ok {
		t.Fatal("detached idle session still live")
	}
	if _, ok := h.Get(busy.ID); !ok {
		t.Fatal("attached session was reaped")
	}
	if got := h.ReapIdle(now, 0); got != nil {
		t.Fatalf("timeout 0 must disable reaping, got %v", got)
	}
}

// TestResizeWithoutGuest stores geometry for later replay and does not fail.
func TestResizeWithoutGuest(t *testing.T) {
	s := NewInMemorySession("r")
	if err := s.Resize(0, 10); err == nil {
		t.Fatal("zero cols must be rejected")
	}
	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("resize without guest: %v", err)
	}
	c, r := s.WindowSize()
	if c != 120 || r != 40 {
		t.Fatalf("window size = %dx%d", c, r)
	}
}
