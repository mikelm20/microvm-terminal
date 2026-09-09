-- +goose Up
-- +goose StatementBegin
create table sessions (
  id uuid primary key,
  owner text not null,
  vm_ip inet,
  warm boolean not null default false,
  created_at timestamptz not null default now(),
  ready_at timestamptz,
  reaped_at timestamptz,
  last_attached_at timestamptz
);
create index sessions_owner_idx on sessions (owner, created_at desc);
create index sessions_alive_idx on sessions (reaped_at) where reaped_at is null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists sessions;
-- +goose StatementEnd
