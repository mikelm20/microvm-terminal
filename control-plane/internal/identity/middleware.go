package identity

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/mikelm20/learn-platform/control-plane/internal/auth"
	"github.com/mikelm20/learn-platform/control-plane/internal/db"
)

type ctxKey int

const (
	ctxKeyIdentity ctxKey = iota
	ctxKeyAuthed
)

// Middleware resolves the caller's identity. It looks at (in order):
//  1. `learn_session` cookie, validated against `auth_sessions` table.
//     If valid, the bound identity UUID is attached as authenticated.
//  2. `learn_identity` signed cookie. If the signature is valid the embedded
//     UUID is attached as anonymous.
//
// If neither is present, the handler proceeds with no identity; endpoints
// that require one must check Require*() helpers.
func Middleware(signer *Signer, store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if sc, err := r.Cookie(auth.LearnSessionCookie); err == nil && sc.Value != "" {
				if id, err := auth.ValidateSession(ctx, store, sc.Value); err == nil {
					ctx = context.WithValue(ctx, ctxKeyIdentity, id)
					ctx = context.WithValue(ctx, ctxKeyAuthed, true)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if ic, err := r.Cookie(IdentityCookie); err == nil && ic.Value != "" {
				if id, err := signer.Verify(ic.Value); err == nil {
					ctx = context.WithValue(ctx, ctxKeyIdentity, id)
					ctx = context.WithValue(ctx, ctxKeyAuthed, false)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext returns the identity UUID attached by Middleware, if any.
func FromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxKeyIdentity).(uuid.UUID)
	return v, ok
}

// IsAuthenticated reports whether the request carried a valid session token
// (not just an anonymous identity cookie).
func IsAuthenticated(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyAuthed).(bool)
	return v
}
