// Package keys manages the Anthropic API key pool and the session-token to
// key mapping. Keys are loaded from Doppler (ANTHROPIC_API_KEYS JSON array)
// via the process environment; rotation is triggered by the admin endpoint.
package keys

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Key is one entry in the pool.
type Key struct {
	// ID is a stable opaque identifier (not the secret itself). Used in audit
	// logs so operators can revoke a specific key on the Anthropic console
	// when they see suspicious use.
	ID string `json:"id"`
	// Workspace is the Anthropic workspace name this key belongs to. The
	// proxy forwards the `x-api-key` header verbatim.
	Workspace string `json:"workspace"`
	// Secret is the raw `sk-ant-...` token.
	Secret string `json:"secret"`
}

// Pool holds the active keys and issues session tokens.
type Pool struct {
	mu           sync.RWMutex
	keys         []Key
	sessionToKey map[string]string // sessionToken -> Key.ID
	keyBySecret  map[string]Key    // keyed by Key.ID
	rr           int               // round-robin index for assignment
}

// ErrEmptyPool is returned when no keys are loaded.
var ErrEmptyPool = errors.New("keys: pool is empty")

// LoadFromEnv reads ANTHROPIC_API_KEYS (JSON array of Key). Returns
// ErrEmptyPool if the env is unset or decodes to zero keys.
func LoadFromEnv() ([]Key, error) {
	raw := os.Getenv("ANTHROPIC_API_KEYS")
	if raw == "" {
		return nil, ErrEmptyPool
	}
	var ks []Key
	if err := json.Unmarshal([]byte(raw), &ks); err != nil {
		return nil, fmt.Errorf("keys: parse ANTHROPIC_API_KEYS: %w", err)
	}
	if len(ks) == 0 {
		return nil, ErrEmptyPool
	}
	for i, k := range ks {
		if k.ID == "" || k.Secret == "" {
			return nil, fmt.Errorf("keys: entry %d missing id or secret", i)
		}
	}
	return ks, nil
}

// New builds a Pool from an initial slice.
func New(initial []Key) *Pool {
	p := &Pool{
		sessionToKey: make(map[string]string),
		keyBySecret:  make(map[string]Key),
	}
	p.Replace(initial)
	return p
}

// Replace atomically swaps the pool contents. In-flight requests that already
// resolved a secret via SecretForSession continue to use the old secret; new
// calls to MintSessionToken route to the new pool.
func (p *Pool) Replace(next []Key) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append([]Key(nil), next...)
	p.keyBySecret = make(map[string]Key, len(next))
	for _, k := range next {
		p.keyBySecret[k.ID] = k
	}
	// Do not clear sessionToKey: sessions in progress keep their mapping. If
	// the key they depend on has been removed we fall back to ErrKeyRevoked
	// at SecretForSession.
}

// ErrKeyRevoked is returned when a session's assigned key no longer exists.
var ErrKeyRevoked = errors.New("keys: session key has been rotated out")

// ErrUnknownSession is returned when the session token has not been minted.
var ErrUnknownSession = errors.New("keys: unknown session token")

// MintSessionToken allocates a fresh token and pins it to a pool key via
// round-robin. Returned token goes back to the control plane, which hands it
// to the VM on launch. VMs only see this token, never the raw Anthropic key.
func (p *Pool) MintSessionToken(sessionID string) (token string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return "", ErrEmptyPool
	}
	token, err = randomToken()
	if err != nil {
		return "", err
	}
	key := p.keys[p.rr%len(p.keys)]
	p.rr++
	p.sessionToKey[token] = key.ID
	return token, nil
}

// RegisterSessionToken lets the control plane provide its own token (e.g. for
// deterministic tests or cross-system coordination) and pin it to a key.
func (p *Pool) RegisterSessionToken(token string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) == 0 {
		return ErrEmptyPool
	}
	key := p.keys[p.rr%len(p.keys)]
	p.rr++
	p.sessionToKey[token] = key.ID
	return nil
}

// RevokeSessionToken removes a session's mapping so further requests fail.
func (p *Pool) RevokeSessionToken(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessionToKey, token)
}

// SecretForSession returns the raw Anthropic key for an active session token.
// Does not expose the key ID externally; callers pass the return value as the
// `x-api-key` header to Anthropic and forget it. The proxy never logs this.
func (p *Pool) SecretForSession(token string) (secret string, keyID string, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	keyID, ok := p.sessionToKey[token]
	if !ok {
		return "", "", ErrUnknownSession
	}
	k, ok := p.keyBySecret[keyID]
	if !ok {
		return "", "", ErrKeyRevoked
	}
	return k.Secret, keyID, nil
}

// Stats for /metrics: number of keys, number of active session tokens.
type Stats struct {
	Keys           int
	ActiveSessions int
	LastRotation   time.Time
}

// Stats returns a snapshot.
func (p *Pool) Stats() Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return Stats{
		Keys:           len(p.keys),
		ActiveSessions: len(p.sessionToKey),
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "sess_" + hex.EncodeToString(b), nil
}
