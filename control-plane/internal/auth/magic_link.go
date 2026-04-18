package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

// MagicLinkTTL is how long a magic link is valid for consumption.
const MagicLinkTTL = 10 * time.Minute

// GenerateMagicLinkToken returns a 32-byte random token, URL-safe base64.
// The raw token is sent to the user by email. Only its SHA-256 digest is
// stored in the database.
func GenerateMagicLinkToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateMagicLink generates + persists a magic link. Returns the raw token
// (for inclusion in the email URL) and the TTL.
func CreateMagicLink(ctx context.Context, store *db.Store, email, lang string, anonymous *uuid.UUID) (string, time.Duration, error) {
	token, err := GenerateMagicLinkToken()
	if err != nil {
		return "", 0, err
	}
	if err := store.InsertMagicLink(ctx, token, email, lang, anonymous, MagicLinkTTL); err != nil {
		return "", 0, err
	}
	return token, MagicLinkTTL, nil
}

// ConsumeMagicLink validates and marks a magic link consumed. Returns the
// recorded email, anonymous UUID (if any), and the lang preference.
func ConsumeMagicLink(ctx context.Context, store *db.Store, rawToken string) (email string, anon *uuid.UUID, lang string, err error) {
	return store.ConsumeMagicLink(ctx, rawToken)
}
