// Package auth is the shared-secret gate in front of every VM-creating
// endpoint. It is NOT an identity system: the same password unlocks sessions
// for anyone who has it. Before the platform is opened to external learners,
// replace this with per-account login backed by a real user store.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CookieName   = "learn_auth"
	CookieMaxAge = 24 * time.Hour
)

// Gate validates passwords and signs cookies.
type Gate struct {
	password     []byte
	cookieSecret []byte

	// Rate-limit /login per IP to stop casual brute-force.
	rlMu     sync.Mutex
	rlTokens map[string]*rlState
}

type rlState struct {
	tokens float64
	last   time.Time
}

// NewGate loads secrets from files. passwordFile must exist. If
// cookieSecretFile is missing, a random 32-byte secret is generated and written.
func NewGate(passwordFile, cookieSecretFile string) (*Gate, error) {
	pw, err := os.ReadFile(passwordFile)
	if err != nil {
		return nil, fmt.Errorf("read password file %s: %w", passwordFile, err)
	}
	pw = []byte(strings.TrimSpace(string(pw)))
	if len(pw) < 4 {
		return nil, fmt.Errorf("password in %s is too short", passwordFile)
	}

	var secret []byte
	if b, err := os.ReadFile(cookieSecretFile); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		decoded, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr != nil {
			return nil, fmt.Errorf("decode cookie secret: %w", derr)
		}
		secret = decoded
	} else {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("generate cookie secret: %w", err)
		}
		if err := os.WriteFile(cookieSecretFile, []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("write cookie secret: %w", err)
		}
	}

	return &Gate{
		password:     pw,
		cookieSecret: secret,
		rlTokens:     make(map[string]*rlState),
	}, nil
}

// LoginHandler accepts POST /login with JSON body {"password":"..."}. On match
// it sets the auth cookie and returns 204. On miss it returns 401. Rate-limited.
func (g *Gate) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ip := clientIP(r)
		if !g.takeToken(ip) {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(body.Password), g.password) != 1 {
			// slight artificial delay on miss to equalize with the happy path
			time.Sleep(150 * time.Millisecond)
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, g.buildCookie())
		w.WriteHeader(http.StatusNoContent)
	}
}

// LogoutHandler clears the cookie. Idempotent.
func (g *Gate) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := g.buildCookie()
		c.MaxAge = -1
		c.Value = ""
		http.SetCookie(w, c)
		w.WriteHeader(http.StatusNoContent)
	}
}

// CheckHandler returns 204 if the caller has a valid cookie, 401 otherwise.
// Used by the frontend to decide which page to render.
func (g *Gate) CheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if g.cookieOK(r) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
	}
}

// Middleware wraps handlers that must be authenticated. On miss: 401.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.cookieOK(r) {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// buildCookie creates a signed cookie: hex(expiry_unix) + "." + hex(hmac).
func (g *Gate) buildCookie() *http.Cookie {
	exp := time.Now().Add(CookieMaxAge).Unix()
	value := fmt.Sprintf("%d", exp)
	mac := hmac.New(sha256.New, g.cookieSecret)
	mac.Write([]byte(value))
	sig := mac.Sum(nil)
	return &http.Cookie{
		Name:     CookieName,
		Value:    value + "." + hex.EncodeToString(sig),
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(CookieMaxAge.Seconds()),
	}
}

func (g *Gate) cookieOK(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	sig, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, g.cookieSecret)
	mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	if !hmac.Equal(sig, want) {
		return false
	}
	if time.Now().Unix() > expUnix {
		return false
	}
	return true
}

// token-bucket rate limit: 5 attempts per IP per 60s.
func (g *Gate) takeToken(ip string) bool {
	g.rlMu.Lock()
	defer g.rlMu.Unlock()
	st, ok := g.rlTokens[ip]
	now := time.Now()
	if !ok {
		st = &rlState{tokens: 5, last: now}
		g.rlTokens[ip] = st
	}
	// refill: 5 tokens per 60 seconds = 1/12 per second
	elapsed := now.Sub(st.last).Seconds()
	st.tokens = minFloat(5, st.tokens+elapsed*(5.0/60.0))
	st.last = now
	if st.tokens < 1 {
		return false
	}
	st.tokens--
	return true
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	return r.RemoteAddr
}

// ErrCookieInvalid is returned by helpers that might be exported later.
var ErrCookieInvalid = errors.New("cookie invalid or expired")
