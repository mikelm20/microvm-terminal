-- +goose Up
-- +goose StatementBegin
create table sessions (
  id uuid primary key,
  identity_uuid uuid not null references identities(uuid),
  lesson_id text not null,
  lang text not null,
  department text,
  vm_ip inet,
  warm boolean not null default false,
  created_at timestamptz not null default now(),
  ready_at timestamptz,
  reaped_at timestamptz,
  grace_started_at timestamptz,
  last_event_at timestamptz
);
create index sessions_identity_idx on sessions (identity_uuid);
create index sessions_alive_idx on sessions (reaped_at) where reaped_at is null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists sessions;
-- +goose StatementEnd
