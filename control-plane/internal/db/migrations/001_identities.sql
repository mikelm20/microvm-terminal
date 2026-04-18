-- +goose Up
-- +goose StatementBegin
create table identities (
  uuid uuid primary key,
  email text unique,
  email_verified_at timestamptz,
  lang text not null default 'es',
  department text,
  name text,
  haptics_enabled boolean not null default false,
  push_enabled boolean not null default false,
  created_at timestamptz not null default now(),
  claimed_at timestamptz,
  last_seen_at timestamptz not null default now()
);
create index identities_email_idx on identities (email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists identities;
-- +goose StatementEnd
