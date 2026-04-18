-- name: InsertMagicLink :exec
insert into magic_links (token, email, anonymous_uuid, lang, expires_at)
values ($1, $2, $3, $4, $5);

-- name: GetMagicLink :one
select token, email, anonymous_uuid, lang, created_at, expires_at, consumed_at
from magic_links
where token = $1;

-- name: ConsumeMagicLink :exec
update magic_links set consumed_at = now() where token = $1 and consumed_at is null;

-- name: InsertAuthSession :exec
insert into auth_sessions (token, identity_uuid, expires_at)
values ($1, $2, $3);

-- name: GetAuthSession :one
select token, identity_uuid, created_at, expires_at, last_used_at
from auth_sessions where token = $1;

-- name: TouchAuthSession :exec
update auth_sessions set last_used_at = now() where token = $1;

-- name: DeleteAuthSession :exec
delete from auth_sessions where token = $1;

-- name: CountRecentMagicLinksByEmail :one
select count(*) from magic_links
where email = $1 and created_at > now() - interval '10 minutes';
