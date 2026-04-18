// Package db owns the Postgres connection pool and migration runner.
//
// sqlc generation is wired via sqlc.yaml and queries/*.sql. For the v1 sprint
// the hand-written Store in this package implements the same query surface
// directly against pgx so the binary builds without a code-gen step. When
// sqlc is re-run (or wired into CI), Store can forward to the generated
// package; the public API of Store is what callers depend on.
package db

import (
	"context"
	"crypto/sha256"
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
// Goose-style up/down markers are recognized; we only apply "Up" blocks.
// When the goose CLI is available, `goose -dir internal/db/migrations postgres $DSN up`
// produces the same result.
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

// --- Domain types. sqlc would emit equivalents under dbq package. ---

type Identity struct {
	UUID            uuid.UUID
	Email           sql.NullString
	EmailVerifiedAt sql.NullTime
	Lang            string
	Department      sql.NullString
	Name            sql.NullString
	HapticsEnabled  bool
	PushEnabled     bool
	CreatedAt       time.Time
	ClaimedAt       sql.NullTime
	LastSeenAt      time.Time
}

type Session struct {
	ID             uuid.UUID
	IdentityUUID   uuid.UUID
	LessonID       string
	Lang           string
	Department     sql.NullString
	VMIP           sql.NullString
	Warm           bool
	CreatedAt      time.Time
	ReadyAt        sql.NullTime
	ReapedAt       sql.NullTime
	GraceStartedAt sql.NullTime
	LastEventAt    sql.NullTime
}

type SessionEvent struct {
	SessionID uuid.UUID
	Seq       int64
	TS        time.Time
	Type      string
	Payload   []byte // raw JSON
}

type MagicLink struct {
	Token         string
	Email         string
	AnonymousUUID *uuid.UUID
	Lang          string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	ConsumedAt    sql.NullTime
}

type AuthSession struct {
	Token        string
	IdentityUUID uuid.UUID
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastUsedAt   time.Time
}

type Certificate struct {
	ID           string
	IdentityUUID uuid.UUID
	LessonID     string
	CompletedAt  time.Time
	Signature    string
	CreatedAt    time.Time
}

// --- Identity queries ---

func (s *Store) InsertOrTouchIdentity(ctx context.Context, id uuid.UUID, lang string) error {
	_, err := s.Pool.Exec(ctx, `
		insert into identities (uuid, lang) values ($1, $2)
		on conflict (uuid) do update set last_seen_at = now()
	`, id, lang)
	return err
}

func (s *Store) GetIdentity(ctx context.Context, id uuid.UUID) (*Identity, error) {
	var out Identity
	err := s.Pool.QueryRow(ctx, `
		select uuid, email, email_verified_at, lang, department, name,
		       haptics_enabled, push_enabled, created_at, claimed_at, last_seen_at
		from identities where uuid = $1
	`, id).Scan(&out.UUID, &out.Email, &out.EmailVerifiedAt, &out.Lang, &out.Department, &out.Name,
		&out.HapticsEnabled, &out.PushEnabled, &out.CreatedAt, &out.ClaimedAt, &out.LastSeenAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

func (s *Store) GetIdentityByEmail(ctx context.Context, email string) (*Identity, error) {
	var out Identity
	err := s.Pool.QueryRow(ctx, `
		select uuid, email, email_verified_at, lang, department, name,
		       haptics_enabled, push_enabled, created_at, claimed_at, last_seen_at
		from identities where email = $1
	`, email).Scan(&out.UUID, &out.Email, &out.EmailVerifiedAt, &out.Lang, &out.Department, &out.Name,
		&out.HapticsEnabled, &out.PushEnabled, &out.CreatedAt, &out.ClaimedAt, &out.LastSeenAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &out, nil
}

// PatchIdentity applies optional updates. Nil values leave existing data.
func (s *Store) PatchIdentity(ctx context.Context, id uuid.UUID, lang, department, name *string, haptics, push *bool) (*Identity, error) {
	_, err := s.Pool.Exec(ctx, `
		update identities set
			lang = coalesce($2, lang),
			department = coalesce($3, department),
			name = coalesce($4, name),
			haptics_enabled = coalesce($5, haptics_enabled),
			push_enabled = coalesce($6, push_enabled)
		where uuid = $1
	`, id, lang, department, name, haptics, push)
	if err != nil {
		return nil, err
	}
	return s.GetIdentity(ctx, id)
}

// ClaimIdentity binds email to identity, migrating anonymous progress if a
// distinct anonymous UUID is supplied. Returns the identity UUID of the
// resulting account (the email's canonical row).
func (s *Store) ClaimIdentity(ctx context.Context, anonymousUUID *uuid.UUID, email, lang string) (uuid.UUID, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	// Is there already an identity with that email?
	var existingID uuid.UUID
	err = tx.QueryRow(ctx, `select uuid from identities where email = $1`, email).Scan(&existingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	var finalID uuid.UUID

	if errors.Is(err, pgx.ErrNoRows) {
		// No account for this email yet.
		if anonymousUUID != nil {
			// Promote the anonymous identity into an account.
			if _, err := tx.Exec(ctx, `
				insert into identities (uuid, lang) values ($1, $2)
				on conflict (uuid) do update set last_seen_at = now()
			`, *anonymousUUID, lang); err != nil {
				return uuid.Nil, err
			}
			if _, err := tx.Exec(ctx, `
				update identities
				set email = $2, email_verified_at = now(), claimed_at = now(), lang = $3
				where uuid = $1
			`, *anonymousUUID, email, lang); err != nil {
				return uuid.Nil, err
			}
			finalID = *anonymousUUID
		} else {
			// Mint a fresh account identity.
			finalID = uuid.New()
			if _, err := tx.Exec(ctx, `
				insert into identities (uuid, email, email_verified_at, lang, claimed_at)
				values ($1, $2, now(), $3, now())
			`, finalID, email, lang); err != nil {
				return uuid.Nil, err
			}
		}
	} else {
		// Existing account. Migrate anon progress then drop the anon row.
		finalID = existingID
		if anonymousUUID != nil && *anonymousUUID != existingID {
			// Move progress if the anon row has any and the target doesn't.
			if _, err := tx.Exec(ctx, `
				insert into progress (identity_uuid, state, updated_at)
				select $2, state, updated_at from progress where identity_uuid = $1
				on conflict (identity_uuid) do nothing
			`, *anonymousUUID, existingID); err != nil {
				return uuid.Nil, err
			}
			if _, err := tx.Exec(ctx, `delete from progress where identity_uuid = $1`, *anonymousUUID); err != nil {
				return uuid.Nil, err
			}
			// Clean sessions and identity row (cascade session_events).
			if _, err := tx.Exec(ctx, `delete from sessions where identity_uuid = $1`, *anonymousUUID); err != nil {
				return uuid.Nil, err
			}
			if _, err := tx.Exec(ctx, `delete from identities where uuid = $1`, *anonymousUUID); err != nil {
				return uuid.Nil, err
			}
		}
		if _, err := tx.Exec(ctx, `
			update identities set email_verified_at = coalesce(email_verified_at, now()),
			                      claimed_at = coalesce(claimed_at, now()),
			                      last_seen_at = now()
			where uuid = $1
		`, finalID); err != nil {
			return uuid.Nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return finalID, nil
}

// --- Session queries ---

// InsertSession stores the session metadata. vmIP may be empty for mocked VMs.
func (s *Store) InsertSession(ctx context.Context, sess Session) error {
	var vmIP any
	if sess.VMIP.Valid && sess.VMIP.String != "" {
		if ip := net.ParseIP(sess.VMIP.String); ip != nil {
			vmIP = sess.VMIP.String
		}
	}
	var dept any
	if sess.Department.Valid {
		dept = sess.Department.String
	}
	_, err := s.Pool.Exec(ctx, `
		insert into sessions (id, identity_uuid, lesson_id, lang, department, vm_ip, warm, ready_at, last_event_at)
		values ($1, $2, $3, $4, $5, $6, $7, now(), now())
	`, sess.ID, sess.IdentityUUID, sess.LessonID, sess.Lang, dept, vmIP, sess.Warm)
	return err
}

func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	var out Session
	var vmIP sql.NullString
	err := s.Pool.QueryRow(ctx, `
		select id, identity_uuid, lesson_id, lang, department, host(vm_ip), warm,
		       created_at, ready_at, reaped_at, grace_started_at, last_event_at
		from sessions where id = $1
	`, id).Scan(&out.ID, &out.IdentityUUID, &out.LessonID, &out.Lang, &out.Department, &vmIP, &out.Warm,
		&out.CreatedAt, &out.ReadyAt, &out.ReapedAt, &out.GraceStartedAt, &out.LastEventAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out.VMIP = vmIP
	return &out, nil
}

func (s *Store) MarkSessionReaped(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `update sessions set reaped_at = now() where id = $1 and reaped_at is null`, id)
	return err
}

func (s *Store) TouchSession(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `update sessions set last_event_at = now() where id = $1`, id)
	return err
}

func (s *Store) InsertSessionEvent(ctx context.Context, sessionID uuid.UUID, ts time.Time, etype string, payload []byte) error {
	_, err := s.Pool.Exec(ctx, `
		insert into session_events (session_id, ts, type, payload) values ($1, $2, $3, $4)
	`, sessionID, ts, etype, payload)
	return err
}

func (s *Store) GetSessionEvents(ctx context.Context, sessionID uuid.UUID) ([]SessionEvent, error) {
	rows, err := s.Pool.Query(ctx, `
		select session_id, seq, ts, type, payload
		from session_events where session_id = $1 order by seq asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionEvent
	for rows.Next() {
		var e SessionEvent
		if err := rows.Scan(&e.SessionID, &e.Seq, &e.TS, &e.Type, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertPromptIdempotency returns the turn_id for this (session, key). If the
// key is new, turnID is inserted. If it already existed, the existing turn_id
// is returned instead.
func (s *Store) UpsertPromptIdempotency(ctx context.Context, sessionID uuid.UUID, key, turnID string) (string, error) {
	var got string
	err := s.Pool.QueryRow(ctx, `
		insert into prompt_idempotency (session_id, idempotency_key, turn_id)
		values ($1, $2, $3)
		on conflict (session_id, idempotency_key) do update set idempotency_key = excluded.idempotency_key
		returning turn_id
	`, sessionID, key, turnID).Scan(&got)
	return got, err
}

// --- Magic link queries ---

// HashToken returns the SHA-256 hex digest used as the primary key in
// magic_links. The raw token value is never stored.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum[:])
}

func (s *Store) InsertMagicLink(ctx context.Context, rawToken, email, lang string, anonymous *uuid.UUID, ttl time.Duration) error {
	hashed := HashToken(rawToken)
	expires := time.Now().Add(ttl)
	var anon any
	if anonymous != nil {
		anon = *anonymous
	}
	_, err := s.Pool.Exec(ctx, `
		insert into magic_links (token, email, anonymous_uuid, lang, expires_at)
		values ($1, $2, $3, $4, $5)
	`, hashed, email, anon, lang, expires)
	return err
}

// ConsumeMagicLink validates and marks consumed. Returns (email, anonUUID, lang)
// on success. ErrExpired if past expiry or already consumed. ErrNotFound if token
// is unknown.
func (s *Store) ConsumeMagicLink(ctx context.Context, rawToken string) (string, *uuid.UUID, string, error) {
	hashed := HashToken(rawToken)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", nil, "", err
	}
	defer tx.Rollback(ctx)
	var email, lang string
	var anon *uuid.UUID
	var expires time.Time
	var consumed sql.NullTime
	err = tx.QueryRow(ctx, `
		select email, anonymous_uuid, lang, expires_at, consumed_at
		from magic_links where token = $1
	`, hashed).Scan(&email, &anon, &lang, &expires, &consumed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, "", ErrNotFound
		}
		return "", nil, "", err
	}
	if consumed.Valid {
		return "", nil, "", ErrExpired
	}
	if time.Now().After(expires) {
		return "", nil, "", ErrExpired
	}
	if _, err := tx.Exec(ctx, `update magic_links set consumed_at = now() where token = $1`, hashed); err != nil {
		return "", nil, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, "", err
	}
	return email, anon, lang, nil
}

func (s *Store) CountRecentMagicLinksByEmail(ctx context.Context, email string, window time.Duration) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		select count(*) from magic_links where email = $1 and created_at > now() - $2::interval
	`, email, fmt.Sprintf("%d seconds", int(window.Seconds()))).Scan(&n)
	return n, err
}

// --- Auth session queries ---

func (s *Store) InsertAuthSession(ctx context.Context, token string, identityUUID uuid.UUID, ttl time.Duration) error {
	_, err := s.Pool.Exec(ctx, `
		insert into auth_sessions (token, identity_uuid, expires_at)
		values ($1, $2, $3)
	`, HashToken(token), identityUUID, time.Now().Add(ttl))
	return err
}

func (s *Store) GetAuthSession(ctx context.Context, token string) (*AuthSession, error) {
	var out AuthSession
	err := s.Pool.QueryRow(ctx, `
		select token, identity_uuid, created_at, expires_at, last_used_at
		from auth_sessions where token = $1
	`, HashToken(token)).Scan(&out.Token, &out.IdentityUUID, &out.CreatedAt, &out.ExpiresAt, &out.LastUsedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if time.Now().After(out.ExpiresAt) {
		return nil, ErrExpired
	}
	return &out, nil
}

func (s *Store) DeleteAuthSession(ctx context.Context, token string) error {
	_, err := s.Pool.Exec(ctx, `delete from auth_sessions where token = $1`, HashToken(token))
	return err
}

// --- Progress queries ---

func (s *Store) UpsertProgress(ctx context.Context, identityUUID uuid.UUID, state []byte) (time.Time, error) {
	var updatedAt time.Time
	err := s.Pool.QueryRow(ctx, `
		insert into progress (identity_uuid, state, updated_at)
		values ($1, $2, now())
		on conflict (identity_uuid) do update set state = excluded.state, updated_at = now()
		returning updated_at
	`, identityUUID, state).Scan(&updatedAt)
	return updatedAt, err
}

func (s *Store) GetProgress(ctx context.Context, identityUUID uuid.UUID) ([]byte, time.Time, error) {
	var state []byte
	var updatedAt time.Time
	err := s.Pool.QueryRow(ctx, `
		select state, updated_at from progress where identity_uuid = $1
	`, identityUUID).Scan(&state, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, ErrNotFound
		}
		return nil, time.Time{}, err
	}
	return state, updatedAt, nil
}

// --- Certificate queries ---

func (s *Store) GetCertificate(ctx context.Context, id string) (*Certificate, error) {
	var c Certificate
	err := s.Pool.QueryRow(ctx, `
		select id, identity_uuid, lesson_id, completed_at, signature, created_at
		from certificates where id = $1
	`, id).Scan(&c.ID, &c.IdentityUUID, &c.LessonID, &c.CompletedAt, &c.Signature, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListCertificatesByIdentity(ctx context.Context, identityUUID uuid.UUID) ([]Certificate, error) {
	rows, err := s.Pool.Query(ctx, `
		select id, identity_uuid, lesson_id, completed_at, signature, created_at
		from certificates where identity_uuid = $1 order by completed_at asc
	`, identityUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Certificate
	for rows.Next() {
		var c Certificate
		if err := rows.Scan(&c.ID, &c.IdentityUUID, &c.LessonID, &c.CompletedAt, &c.Signature, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Errors ---

var (
	ErrNotFound = errors.New("not found")
	ErrExpired  = errors.New("expired")
)
