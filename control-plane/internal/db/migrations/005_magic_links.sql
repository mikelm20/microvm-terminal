-- +goose Up
-- +goose StatementBegin
create table magic_links (
  token text primary key,
  email text not null,
  anonymous_uuid uuid,
  lang text not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  consumed_at timestamptz
);
create index magic_links_email_idx on magic_links (email);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists magic_links;
-- +goose StatementEnd
