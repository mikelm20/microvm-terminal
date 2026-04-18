// Package audit persists a per-request log of every call the proxy made to
// Anthropic. Append-only; never mutated after insert. The table drives cost
// attribution (finance) and misuse investigations (security) in equal measure.
package audit

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

// Record is one row in the proxy_audit_log table.
type Record struct {
	TS             time.Time
	SessionID      string
	Endpoint       string
	RequestTokens  int64
	ResponseTokens int64
	CostUSDMicros  int64
	Status         int
}

// Sink persists audit records. Implementations must be safe for concurrent use.
type Sink interface {
	Write(ctx context.Context, r Record) error
	Close() error
}

// InMemorySink keeps records in RAM. Used by tests. Not for production.
type InMemorySink struct {
	mu      sync.Mutex
	records []Record
}

// NewInMemorySink returns an empty sink.
func NewInMemorySink() *InMemorySink { return &InMemorySink{} }

// Write appends the record.
func (s *InMemorySink) Write(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return nil
}

// Close is a no-op.
func (s *InMemorySink) Close() error { return nil }

// All returns a copy of every record seen.
func (s *InMemorySink) All() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}

// Len returns the record count.
func (s *InMemorySink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

// SQLSink writes to a Postgres database. Callers supply the *sql.DB; the sink
// does not own the pool.
type SQLSink struct {
	db *sql.DB
}

// NewSQLSink wraps db. The caller is responsible for db.Close().
func NewSQLSink(db *sql.DB) *SQLSink { return &SQLSink{db: db} }

// Write inserts a row into proxy_audit_log.
func (s *SQLSink) Write(ctx context.Context, r Record) error {
	if s.db == nil {
		return errors.New("audit: nil db")
	}
	const q = `
insert into proxy_audit_log
  (ts, session_id, endpoint, request_tokens, response_tokens, cost_usd_micros, status)
values ($1, $2, $3, $4, $5, $6, $7)`
	_, err := s.db.ExecContext(ctx, q,
		r.TS, r.SessionID, r.Endpoint, r.RequestTokens, r.ResponseTokens, r.CostUSDMicros, r.Status,
	)
	return err
}

// Close is a no-op because the caller owns the db handle.
func (s *SQLSink) Close() error { return nil }

// Schema is the DDL that must be applied before the SQLSink can accept writes.
// Kept close to the code to make the contract obvious to reviewers.
const Schema = `
create table if not exists proxy_audit_log (
  id              bigserial primary key,
  ts              timestamptz not null default now(),
  session_id      text not null,
  endpoint        text not null,
  request_tokens  bigint not null default 0,
  response_tokens bigint not null default 0,
  cost_usd_micros bigint not null default 0,
  status          integer not null
);
create index if not exists proxy_audit_log_session_idx on proxy_audit_log (session_id, ts);
create index if not exists proxy_audit_log_ts_idx on proxy_audit_log (ts);
`
