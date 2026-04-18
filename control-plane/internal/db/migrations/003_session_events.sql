-- +goose Up
-- +goose StatementBegin
create table session_events (
  session_id uuid not null references sessions(id) on delete cascade,
  seq bigint generated always as identity,
  ts timestamptz not null default now(),
  type text not null,
  payload jsonb not null,
  primary key (session_id, seq)
);
create index session_events_ts_idx on session_events (session_id, ts);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists session_events;
-- +goose StatementEnd
