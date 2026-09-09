package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/db"
	"github.com/mikelm20/microvm-terminal/control-plane/internal/session"
)

// SessionHost abstracts the thing that creates, fetches, and destroys
// sessions. In production it is session.Host wired to the Firecracker
// Manager; tests use the same Host with a mock launcher.
type SessionHost interface {
	Create(ctx context.Context) (*session.Session, error)
	CreateWarm(ctx context.Context) (*session.Session, error)
	Get(id string) (*session.Session, bool)
	List() []*session.Session
	Destroy(id string) error
	Count() int
	WarmPoolSize() int
}

// SessionStore is the persistence surface the handlers need. db.Store
// implements it; tests use an in-memory recorder.
type SessionStore interface {
	InsertSession(ctx context.Context, sess db.Session) error
	GetSession(ctx context.Context, id uuid.UUID) (*db.Session, error)
	MarkSessionReaped(ctx context.Context, id uuid.UUID) error
	TouchSessionAttached(ctx context.Context, id uuid.UUID) error
}

// Wire types. Handlers only ever emit these shapes.

type CreateSessionRequest struct {
	// Warm asks for a pre-booted VM from the pool when one is available.
	Warm bool `json:"warm,omitempty"`
}

type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
	VMIP      string `json:"vm_ip,omitempty"`
	PTYWSURL  string `json:"pty_ws_url"`
	Warm      bool   `json:"warm"`
}

type SessionInfo struct {
	SessionID string `json:"session_id"`
	Owner     string `json:"owner"`
	VMIP      string `json:"vm_ip,omitempty"`
	Alive     bool   `json:"alive"`
	Attached  int    `json:"attached"`
	CreatedAt string `json:"created_at"`
	ReadyAt   string `json:"ready_at,omitempty"`
	ReapedAt  string `json:"reaped_at,omitempty"`
}

type OKResponse struct {
	Ok bool `json:"ok"`
}

type APIError struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	RetryAfterSeconds *int   `json:"retry_after_seconds,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
}

// Error codes.
const (
	ErrCapacityFull   = "capacity_full"
	ErrAuthRequired   = "auth_required"
	ErrAuthInvalid    = "auth_invalid"
	ErrVMLaunchFailed = "vm_launch_failed"
	ErrVMNotFound     = "vm_not_found"
	ErrSessionReaped  = "session_reaped"
	ErrBadRequest     = "bad_request"
	ErrRateLimited    = "rate_limited"
	ErrInternal       = "internal"
)

// ControlMessage is the JSON shape a terminal client sends as a text frame
// on the PTY WebSocket. Binary frames are raw terminal input.
type ControlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}
