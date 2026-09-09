// Package db owns the Postgres connection pool, the migration runner and the
// sessions table. The table is a history: every VM that ever booted, who
// owned it, when it became ready and when it was reaped.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// Migrate applies any migrations not yet in the schema_migrations table.
// Goose-style up/down markers are recognised; only "Up" blocks are applied.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, `
		create table if not exists schema_migrations (
			id text primary key,
			applied_at timestamptz not null default now()
		);
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := s.Pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where id = $1)`, name).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if exists {
			continue
		}
		raw, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return err
		}
		up := extractGooseUp(string(raw))
		if strings.TrimSpace(up) == "" {
			return fmt.Errorf("migration %s has no Up block", name)
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations (id) values ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// extractGooseUp pulls out everything between "-- +goose Up" and the next
// "-- +goose Down", stripping StatementBegin/StatementEnd markers.
func extractGooseUp(src string) string {
	lines := strings.Split(src, "\n")
	var out []string
	inUp := false
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "-- +goose Up") {
			inUp = true
			continue
		}
		if strings.HasPrefix(trim, "-- +goose Down") {
			inUp = false
			continue
		}
		if !inUp {
			continue
		}
		if strings.HasPrefix(trim, "-- +goose StatementBegin") || strings.HasPrefix(trim, "-- +goose StatementEnd") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// Session is one row of the sessions table.
type Session struct {
	ID             uuid.UUID
	Owner          string
	VMIP           sql.NullString
	Warm           bool
	CreatedAt      time.Time
	ReadyAt        sql.NullTime
	ReapedAt       sql.NullTime
	LastAttachedAt sql.NullTime
}

// InsertSession stores a freshly booted session. vmIP may be empty for
// mocked VMs. ready_at is stamped now because the launcher only returns
// after the guest handshake.
func (s *Store) InsertSession(ctx context.Context, sess Session) error {
	var vmIP any
	if sess.VMIP.Valid && sess.VMIP.String != "" {
		if ip := net.ParseIP(sess.VMIP.String); ip != nil {
			vmIP = sess.VMIP.String
		}
	}
	_, err := s.Pool.Exec(ctx, `
		insert into sessions (id, owner, vm_ip, warm, ready_at)
		values ($1, $2, $3, $4, now())
	`, sess.ID, sess.Owner, vmIP, sess.Warm)
	return err
}

func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	var out Session
	var vmIP sql.NullString
	err := s.Pool.QueryRow(ctx, `
		select id, owner, host(vm_ip), warm, created_at, ready_at, reaped_at, last_attached_at
		from sessions where id = $1
	`, id).Scan(&out.ID, &out.Owner, &vmIP, &out.Warm, &out.CreatedAt, &out.ReadyAt, &out.ReapedAt, &out.LastAttachedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out.VMIP = vmIP
	return &out, nil
}

// MarkSessionReaped stamps reaped_at once; later calls are no-ops.
func (s *Store) MarkSessionReaped(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `update sessions set reaped_at = now() where id = $1 and reaped_at is null`, id)
	return err
}

// TouchSessionAttached records the last time a terminal client connected.
func (s *Store) TouchSessionAttached(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `update sessions set last_attached_at = now() where id = $1`, id)
	return err
}

var ErrNotFound = errors.New("not found")
