-- +goose Up
-- +goose StatementBegin
create table certificates (
  id text primary key,
  identity_uuid uuid not null references identities(uuid),
  lesson_id text not null,
  completed_at timestamptz not null,
  signature text not null,
  created_at timestamptz not null default now()
);
create index certificates_identity_idx on certificates (identity_uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists certificates;
-- +goose StatementEnd
