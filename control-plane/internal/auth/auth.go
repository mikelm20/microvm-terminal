// Package auth is the shared-password gate in front of every VM-creating
// endpoint. A caller presents the password plus a display name; the name is
// slugified and carried in an HMAC-signed cookie, and becomes the owner of
// every session the caller boots.
//
// It is NOT an identity system: the same password unlocks the service for
// anyone who has it, and two people using the same name share ownership.
// Replace it with per-account login before opening the host to strangers.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CookieName   = "mvt_auth"
	CookieMaxAge = 30 * 24 * time.Hour
	MaxNameLen   = 32
)

// Options tune cookie attributes.
type Options struct {
	// Secure sets the Secure flag on the cookie. Disable only for plain
	// http development.
	Secure bool
	// Domain, when non-empty, is set as the cookie Domain attribute.
	Domain string
}

// Gate validates passwords and signs cookies.
type Gate struct {
	password     []byte
	cookieSecret []byte
	opts         Options

	rlMu     sync.Mutex
	rlTokens map[string]*rlState
}

type rlState struct {
	tokens float64
	last   time.Time
}

// NewGate loads secrets from files. passwordFile must exist and hold at
// least 8 characters. If cookieSecretFile is missing, a random 32-byte
// secret is generated and written there with mode 0600.
func NewGate(passwordFile, cookieSecretFile string, opts Options) (*Gate, error) {
	pw, err := os.ReadFile(passwordFile)
	if err != nil {
		return nil, fmt.Errorf("read password file %s: %w", passwordFile, err)
	}
	pw = []byte(strings.TrimSpace(string(pw)))
	if len(pw) < 8 {
		return nil, fmt.Errorf("password in %s is shorter than 8 characters", passwordFile)
	}

	secret, err := loadOrCreateSecret(cookieSecretFile)
	if err != nil {
		return nil, err
	}
	return &Gate{
		password:     pw,
		cookieSecret: secret,
		opts:         opts,
		rlTokens:     make(map[string]*rlState),
	}, nil
}

// NewGateFromSecrets builds a Gate without touching the filesystem. Tests.
func NewGateFromSecrets(password string, cookieSecret []byte, opts Options) *Gate {
	return &Gate{
		password:     []byte(password),
		cookieSecret: cookieSecret,
		opts:         opts,
		rlTokens:     make(map[string]*rlState),
	}
}

func loadOrCreateSecret(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 64 {
		decoded, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr != nil {
			return nil, fmt.Errorf("decode cookie secret %s: %w", path, derr)
		}
		return decoded, nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate cookie secret: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write cookie secret %s: %w", path, err)
	}
	return secret, nil
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify normalises a display name into the owner id carried by the
// cookie: lowercase, [a-z0-9-], at most MaxNameLen characters. Returns ""
// when nothing usable is left.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxNameLen {
		s = strings.Trim(s[:MaxNameLen], "-")
	}
	return s
}

// CheckPassword compares in constant time.
func (g *Gate) CheckPassword(candidate string) bool {
	return subtle.ConstantTimeCompare([]byte(candidate), g.password) == 1
}

// Allow applies the per-IP login rate limit: 5 attempts per 60 s.
func (g *Gate) Allow(ip string) bool {
	g.rlMu.Lock()
	defer g.rlMu.Unlock()
	st, ok := g.rlTokens[ip]
	now := time.Now()
	if !ok {
		st = &rlState{tokens: 5, last: now}
		g.rlTokens[ip] = st
	}
	elapsed := now.Sub(st.last).Seconds()
	st.tokens = min(5, st.tokens+elapsed*(5.0/60.0))
	st.last = now
	if st.tokens < 1 {
		return false
	}
	st.tokens--
	return true
}

// Cookie returns a signed cookie for owner. Value is
// "<owner>|<expiry unix>|<hex hmac>".
func (g *Gate) Cookie(owner string) *http.Cookie {
	exp := time.Now().Add(CookieMaxAge).Unix()
	payload := owner + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, g.cookieSecret)
	mac.Write([]byte(payload))
	c := &http.Cookie{
		Name:     CookieName,
		Value:    payload + "|" + hex.EncodeToString(mac.Sum(nil)),
		Path:     "/",
		Secure:   g.opts.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(CookieMaxAge.Seconds()),
	}
	if g.opts.Domain != "" {
		c.Domain = g.opts.Domain
	}
	return c
}

// ClearCookie returns an expired cookie that removes the session.
func (g *Gate) ClearCookie() *http.Cookie {
	c := g.Cookie("")
	c.Value = ""
	c.MaxAge = -1
	return c
}

// Verify parses a cookie value and returns the owner it was issued to.
func (g *Gate) Verify(value string) (string, error) {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 {
		return "", ErrCookieInvalid
	}
	owner, expStr, sigHex := parts[0], parts[1], parts[2]
	mac := hmac.New(sha256.New, g.cookieSecret)
	mac.Write([]byte(owner + "|" + expStr))
	sig, err := hex.DecodeString(sigHex)
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", ErrCookieInvalid
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", ErrCookieInvalid
	}
	if owner == "" || Slugify(owner) != owner {
		return "", ErrCookieInvalid
	}
	return owner, nil
}

// OwnerFromRequest returns the owner named by a valid cookie, if any.
func (g *Gate) OwnerFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	owner, err := g.Verify(c.Value)
	if err != nil {
		return "", false
	}
	return owner, true
}

type ctxKey int

const ctxKeyOwner ctxKey = iota

// Middleware rejects requests without a valid cookie with 401 and stores
// the owner in the request context for handlers behind it.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner, ok := g.OwnerFromRequest(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"auth_required","message":"log in first"}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyOwner, owner)))
	})
}

// OwnerFromContext returns the owner stored by Middleware.
func OwnerFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyOwner).(string)
	return v, ok && v != ""
}

// ClientIP prefers the first X-Forwarded-For hop (Caddy sets it) and falls
// back to RemoteAddr.
func ClientIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	return r.RemoteAddr
}

// ErrCookieInvalid is returned by Verify for any malformed, forged or
// expired cookie.
var ErrCookieInvalid = errors.New("cookie invalid or expired")
