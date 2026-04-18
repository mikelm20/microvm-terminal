package session

import (
	"bytes"
	"io"
	"sync"
)

// SerialTee drains a reader continuously into a bounded ring buffer AND
// broadcasts new bytes to any currently-attached subscriber. This serves two
// purposes: (1) the VM's serial pipe never blocks on write, so a detached VM
// keeps booting; (2) a late-attaching WebSocket client can replay the last
// ~256 KiB of serial output so it sees the boot log + current state.
type SerialTee struct {
	mu         sync.Mutex
	ring       *bytes.Buffer
	ringLimit  int
	subscriber chan<- []byte
}

func NewSerialTee(src io.Reader, limit int) *SerialTee {
	t := &SerialTee{
		ring:      new(bytes.Buffer),
		ringLimit: limit,
	}
	go t.drain(src)
	return t
}

func (t *SerialTee) drain(src io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			t.mu.Lock()
			// keep at most ringLimit bytes in the buffer
			t.ring.Write(buf[:n])
			if over := t.ring.Len() - t.ringLimit; over > 0 {
				b := t.ring.Bytes()
				t.ring.Reset()
				t.ring.Write(b[over:])
			}
			sub := t.subscriber
			t.mu.Unlock()
			if sub != nil {
				// Copy out so the recipient can't mutate our ring
				cp := make([]byte, n)
				copy(cp, buf[:n])
				select {
				case sub <- cp:
				default:
					// subscriber too slow, drop
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// Attach returns a replay of the ring buffer and a channel of future bytes.
// Only one subscriber is allowed at a time; a second Attach replaces the first.
// Call the returned cleanup to detach.
func (t *SerialTee) Attach() ([]byte, <-chan []byte, func()) {
	t.mu.Lock()
	replay := make([]byte, t.ring.Len())
	copy(replay, t.ring.Bytes())
	ch := make(chan []byte, 64)
	t.subscriber = ch
	t.mu.Unlock()
	return replay, ch, func() {
		t.mu.Lock()
		if t.subscriber == ch {
			t.subscriber = nil
		}
		t.mu.Unlock()
		close(ch)
	}
}
