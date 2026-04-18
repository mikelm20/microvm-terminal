-- +goose Up
-- +goose StatementBegin
create table auth_sessions (
  token text primary key,
  identity_uuid uuid not null references identities(uuid) on delete cascade,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_used_at timestamptz not null default now()
);
create index auth_sessions_identity_idx on auth_sessions (identity_uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists auth_sessions;
-- +goose StatementEnd
