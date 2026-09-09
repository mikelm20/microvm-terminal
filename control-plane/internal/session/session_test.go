package session

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// TestCloseDoneConcurrent guards the double-close panic between
// Manager.Destroy, Host.Destroy and the process reaper goroutine. Before the
// sync.Once guard, two goroutines could both pass the select's default branch
// and close the channel twice, taking the daemon down.
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
	// A late caller must still be a no-op.
	s.CloseDone()
}

// TestHostDestroyDropsSessionOnNaturalExit covers the Host side of teardown:
// once a session's done channel closes (Manager.Destroy via the reaper), the
// Host must stop counting it toward MaxConcurrent.
func TestHostDropsSessionOnNaturalExit(t *testing.T) {
	launcher := LauncherFunc(func(ctx context.Context) (*Session, error) {
		return NewInMemorySession(""), nil
	})
	h := NewHost(launcher, nil, 1, slog.Default())
	s, err := h.Create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(context.Background()); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("expected ErrAtCapacity, got %v", err)
	}
	// Simulate the VM exiting on its own.
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
}

// TestHostDestroyClosesDone covers the manager-less path (mock launcher).
func TestHostDestroyClosesDone(t *testing.T) {
	h := NewHost(LauncherFunc(func(ctx context.Context) (*Session, error) {
		return NewInMemorySession(""), nil
	}), nil, 3, slog.Default())
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
