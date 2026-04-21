// Package identity owns anonymous UUID minting and the signed identity cookie.
//
// Flow:
//   - On first launch the client calls POST /identity without a uuid. The
//     server mints a fresh v4 UUID, returns it plus a signed cookie value,
//     and persists it in `identities`.
//   - On subsequent launches the client re-attests by supplying its stored
//     UUID. The server verifies the signature if a cookie is present, bumps
//     `last_seen_at`, and returns the same UUID.
package identity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

// IdentityCookie is the cookie name for the long-lived anonymous identity.
const IdentityCookie = "learn_identity"

// IdentityTTL is how long the anonymous identity cookie is valid.
const IdentityTTL = 365 * 24 * time.Hour

// Signer signs and validates cookie values.
type Signer struct {
	secret []byte
}

func NewSigner(secret []byte) *Signer { return &Signer{secret: secret} }

// Sign returns "<uuid>.<unix-exp>.<hex-hmac>".
func (s *Signer) Sign(id uuid.UUID, exp time.Time) string {
	body := fmt.Sprintf("%s.%d", id.String(), exp.Unix())
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(body))
	return body + "." + hex.EncodeToString(mac.Sum(nil))
}

// Verify parses a signed cookie and returns the UUID if the signature is
// valid and the cookie has not expired.
func (s *Signer) Verify(raw string) (uuid.UUID, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return uuid.Nil, errors.New("malformed cookie")
	}
	id, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, err
	}
	expUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return uuid.Nil, err
	}
	sig, err := hex.DecodeString(parts[2])
	if err != nil {
		return uuid.Nil, err
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return uuid.Nil, errors.New("bad signature")
	}
	if time.Now().Unix() > expUnix {
		return uuid.Nil, errors.New("expired")
	}
	return id, nil
}

// IdentityCookieFor returns a signed cookie value for the given UUID.
func (s *Signer) IdentityCookieFor(id uuid.UUID) string {
	return s.Sign(id, time.Now().Add(IdentityTTL))
}

// IdentityHTTPCookie returns an http.Cookie with the signed value. secure
// controls the Secure flag (disable for local http dev). domain, when
// non-empty, scopes the cookie across subdomains (e.g. ".example.com").
func (s *Signer) IdentityHTTPCookie(id uuid.UUID, secure bool, domain string) *http.Cookie {
	c := &http.Cookie{
		Name:     IdentityCookie,
		Value:    s.IdentityCookieFor(id),
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(IdentityTTL.Seconds()),
	}
	if domain != "" {
		c.Domain = domain
	}
	return c
}

// MintOrReattest is the POST /identity business logic. If supplied is nil
// (no uuid in body), mint a fresh one. Otherwise keep the UUID but touch
// last_seen_at.
func MintOrReattest(ctx context.Context, store *db.Store, supplied *uuid.UUID, lang string) (uuid.UUID, error) {
	if supplied != nil {
		if err := store.InsertOrTouchIdentity(ctx, *supplied, lang); err != nil {
			return uuid.Nil, err
		}
		return *supplied, nil
	}
	id := uuid.New()
	if err := store.InsertOrTouchIdentity(ctx, id, lang); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
