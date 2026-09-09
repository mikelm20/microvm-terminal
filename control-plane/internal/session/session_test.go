package session

import (
	"sync"
	"testing"
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
