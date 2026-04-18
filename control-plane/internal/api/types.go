package api

import (
	"context"

	"github.com/mikelm20/learn-platform/control-plane/internal/session"
)

// SessionHost abstracts the thing that creates, fetches, and destroys
// sessions. In production it is wired to *session.Manager. In tests it's a
// stub that returns in-memory Session objects so the HTTP + WS layers can
// be exercised without Firecracker.
type SessionHost interface {
	Create(ctx context.Context) (*session.Session, error)
	CreateWarm(ctx context.Context) (*session.Session, error)
	Get(id string) (*session.Session, bool)
	Destroy(id string) error
	Count() int
	WarmPoolSize() int
}
