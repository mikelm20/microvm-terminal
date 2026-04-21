package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

// SessionTTL is how long a post-claim session cookie is valid.
const SessionTTL = 30 * 24 * time.Hour

// LearnSessionCookie is the cookie name for the authenticated session.
const LearnSessionCookie = "learn_session"

// GenerateSessionToken returns a 32-byte random URL-safe token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// MintSession creates a server-side session row and returns the raw token.
// The raw token is returned to the client as a cookie and in the response body.
// Only its hash is stored.
func MintSession(ctx context.Context, store *db.Store, identityUUID uuid.UUID) (string, error) {
	tok, err := GenerateSessionToken()
	if err != nil {
		return "", err
	}
	if err := store.InsertAuthSession(ctx, tok, identityUUID, SessionTTL); err != nil {
		return "", err
	}
	return tok, nil
}

// ValidateSession looks up an auth session by its raw token. Returns the
// bound identity UUID on success.
func ValidateSession(ctx context.Context, store *db.Store, rawToken string) (uuid.UUID, error) {
	s, err := store.GetAuthSession(ctx, rawToken)
	if err != nil {
		return uuid.Nil, err
	}
	return s.IdentityUUID, nil
}

// SessionCookie returns an http.Cookie for the given raw session token.
// secure controls whether the Secure flag is set; set to false in local dev.
// domain, when non-empty, sets the Domain attribute so the cookie is shared
// across subdomains (e.g. ".example.com").
func SessionCookie(rawToken string, secure bool, domain string) *http.Cookie {
	c := &http.Cookie{
		Name:     LearnSessionCookie,
		Value:    rawToken,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	}
	if domain != "" {
		c.Domain = domain
	}
	return c
}

// ClearSessionCookie returns an expired session cookie (for logout). domain
// must match the Domain used when the cookie was minted, otherwise the
// browser keeps the live cookie alongside the expired one.
func ClearSessionCookie(secure bool, domain string) *http.Cookie {
	c := &http.Cookie{
		Name:     LearnSessionCookie,
		Value:    "",
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	if domain != "" {
		c.Domain = domain
	}
	return c
}
