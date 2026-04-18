-- +goose Up
-- +goose StatementBegin
create table progress (
  identity_uuid uuid primary key references identities(uuid),
  state jsonb not null,
  updated_at timestamptz not null default now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists progress;
-- +goose StatementEnd
