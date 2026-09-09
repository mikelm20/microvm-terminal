// Package quota enforces per-session caps: tokens in, tokens out, requests per
// minute, requests per session. The proxy calls Reserve before each forwarded
// request; on success it calls Commit with the actual usage once the upstream
// response is fully observed. Reserve returns a QuotaError when exceeded.
package quota

import (
	"errors"
	"sync"
	"time"
)

// Limits caps a single session.
type Limits struct {
	// MaxTokensIn is the total cumulative input tokens per session.
	MaxTokensIn int64
	// MaxTokensOut is the total cumulative output tokens per session.
	MaxTokensOut int64
	// MaxRequests is the total request count per session.
	MaxRequests int64
	// MaxRequestsPerMinute caps burst rate across the session.
	MaxRequestsPerMinute int64
}

// DefaultLimits is the baseline enforcement for a single sandbox session.
// Generous enough for a working session, tight enough that a looped bash
// `while true; do claude --print ...` cannot torch the budget. Tuned against
// sessions that needed ~14k input tokens + ~22k output tokens across 9
// messages.
var DefaultLimits = Limits{
	MaxTokensIn:          60000,
	MaxTokensOut:         120000,
	MaxRequests:          60,
	MaxRequestsPerMinute: 12,
}

// Usage is the cumulative consumption of one session.
type Usage struct {
	TokensIn  int64
	TokensOut int64
	Requests  int64
}

// Code identifies why a quota check failed. Maps cleanly onto the ApiError
// contract in shared/api/errors.ts; all quota failures surface as
// code="rate_limited" with retry_after_seconds.
type Code string

const (
	CodeTokensIn   Code = "tokens_in_exhausted"
	CodeTokensOut  Code = "tokens_out_exhausted"
	CodeRequests   Code = "requests_exhausted"
	CodeRateLimit  Code = "rate_limit_per_minute"
)

// Error is returned by Reserve when the session cannot proceed.
type Error struct {
	Code         Code
	RetryAfter   time.Duration
	SessionID    string
}

// Error satisfies the error interface.
func (e *Error) Error() string { return string(e.Code) }

// Manager tracks quotas in memory. It is safe for concurrent use. State is not
// persisted; restarting the proxy resets every session, which is intentional:
// if the proxy restarts the session is dead anyway (VM reaped).
type Manager struct {
	mu      sync.Mutex
	limits  Limits
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	usage       Usage
	windowStart time.Time
	windowCount int64
}

// New returns a Manager enforcing the given limits.
func New(l Limits) *Manager {
	if l.MaxTokensIn <= 0 {
		l = DefaultLimits
	}
	return &Manager{
		limits:  l,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// SetNow swaps the clock for tests.
func (m *Manager) SetNow(f func() time.Time) { m.now = f }

// Reserve checks whether the session may issue another request right now. It
// does not decrement any counter; it only verifies that the previously
// committed usage plus the next request would stay inside the caps. Upstream
// callers should Commit immediately after the Anthropic response is observed.
// If exceeded, returns *Error; the proxy translates to 429 + ApiError.
func (m *Manager) Reserve(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.get(sessionID)
	now := m.now()

	if now.Sub(b.windowStart) >= time.Minute {
		b.windowStart = now
		b.windowCount = 0
	}
	if b.windowCount >= m.limits.MaxRequestsPerMinute {
		retry := time.Minute - now.Sub(b.windowStart)
		if retry < time.Second {
			retry = time.Second
		}
		return &Error{Code: CodeRateLimit, RetryAfter: retry, SessionID: sessionID}
	}
	if b.usage.Requests >= m.limits.MaxRequests {
		return &Error{Code: CodeRequests, RetryAfter: 0, SessionID: sessionID}
	}
	if b.usage.TokensIn >= m.limits.MaxTokensIn {
		return &Error{Code: CodeTokensIn, RetryAfter: 0, SessionID: sessionID}
	}
	if b.usage.TokensOut >= m.limits.MaxTokensOut {
		return &Error{Code: CodeTokensOut, RetryAfter: 0, SessionID: sessionID}
	}
	// Bump the per-minute counter on Reserve so a flood of concurrent requests
	// cannot each pass the check and only fail at Commit.
	b.windowCount++
	return nil
}

// Commit records actual usage after the upstream response is parsed.
// requestTokens and responseTokens are the exact counts reported by Anthropic.
func (m *Manager) Commit(sessionID string, requestTokens, responseTokens int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.get(sessionID)
	b.usage.TokensIn += requestTokens
	b.usage.TokensOut += responseTokens
	b.usage.Requests++
}

// Refund rolls back a rate-limit slot when the reserved request was aborted
// before reaching Anthropic. Does NOT roll back token usage (there was none).
func (m *Manager) Refund(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.get(sessionID)
	if b.windowCount > 0 {
		b.windowCount--
	}
}

// Usage returns a snapshot.
func (m *Manager) Usage(sessionID string) Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.get(sessionID)
	return b.usage
}

// Reset clears state for a session. Called on session teardown.
func (m *Manager) Reset(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.buckets, sessionID)
}

// Limits returns the active caps.
func (m *Manager) Limits() Limits { return m.limits }

func (m *Manager) get(sessionID string) *bucket {
	b, ok := m.buckets[sessionID]
	if !ok {
		b = &bucket{windowStart: m.now()}
		m.buckets[sessionID] = b
	}
	return b
}

// IsQuotaError is a typed helper for callers that catch.
func IsQuotaError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
