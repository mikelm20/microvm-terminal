package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
)

type apiCtxKey int

const (
	ctxKeyRequestID apiCtxKey = iota
)

// RequestIDHeader is the header both accepted (incoming override for tracing)
// and emitted on every response.
const RequestIDHeader = "X-Request-Id"

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// requestLogger is a minimal JSON-friendly request log.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info("req",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"request_id", r.Header.Get(RequestIDHeader),
			)
			next.ServeHTTP(w, r)
		})
	}
}
