package auth

import (
	"sync"
	"time"
)

// TokenBucket is a simple in-memory rate limiter keyed by arbitrary string
// (IP, email, etc.). Refill rate and burst are configurable.
type TokenBucket struct {
	mu       sync.Mutex
	state    map[string]*bucketState
	capacity float64
	refill   float64 // tokens per second
}

type bucketState struct {
	tokens float64
	last   time.Time
}

// NewTokenBucket creates a limiter that permits at most `burst` events within
// a window of `perWindow`. After reaching capacity, tokens refill at
// `burst / perWindow` per second.
func NewTokenBucket(burst int, perWindow time.Duration) *TokenBucket {
	return &TokenBucket{
		state:    make(map[string]*bucketState),
		capacity: float64(burst),
		refill:   float64(burst) / perWindow.Seconds(),
	}
}

// Allow tries to consume one token for the given key. Returns true if allowed.
func (b *TokenBucket) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	st, ok := b.state[key]
	if !ok {
		st = &bucketState{tokens: b.capacity, last: now}
		b.state[key] = st
	}
	elapsed := now.Sub(st.last).Seconds()
	st.tokens += elapsed * b.refill
	if st.tokens > b.capacity {
		st.tokens = b.capacity
	}
	st.last = now
	if st.tokens < 1 {
		return false
	}
	st.tokens--
	return true
}

// MagicLinkLimiter is the recommended default: 3 magic-link requests per
// 5 minutes per key.
func MagicLinkLimiter() *TokenBucket {
	return NewTokenBucket(3, 5*time.Minute)
}
