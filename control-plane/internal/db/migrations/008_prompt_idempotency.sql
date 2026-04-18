-- +goose Up
-- +goose StatementBegin
-- Dedupe table for POST /sessions/:id/prompt Idempotency-Key header.
-- Same key on a session returns the same turn_id.
create table prompt_idempotency (
  session_id uuid not null references sessions(id) on delete cascade,
  idempotency_key text not null,
  turn_id text not null,
  created_at timestamptz not null default now(),
  primary key (session_id, idempotency_key)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists prompt_idempotency;
-- +goose StatementEnd
