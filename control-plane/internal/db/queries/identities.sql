-- name: InsertIdentity :exec
insert into identities (uuid, lang) values ($1, $2)
on conflict (uuid) do update set last_seen_at = now();

-- name: GetIdentityByUUID :one
select uuid, email, email_verified_at, lang, department, name,
       haptics_enabled, push_enabled, created_at, claimed_at, last_seen_at
from identities where uuid = $1;

-- name: GetIdentityByEmail :one
select uuid, email, email_verified_at, lang, department, name,
       haptics_enabled, push_enabled, created_at, claimed_at, last_seen_at
from identities where email = $1;

-- name: UpdateIdentityLastSeen :exec
update identities set last_seen_at = now() where uuid = $1;

-- name: PatchIdentity :one
update identities set
  lang = coalesce($2, lang),
  department = coalesce($3, department),
  name = coalesce($4, name),
  haptics_enabled = coalesce($5, haptics_enabled),
  push_enabled = coalesce($6, push_enabled)
where uuid = $1
returning uuid, email, email_verified_at, lang, department, name,
          haptics_enabled, push_enabled, created_at, claimed_at, last_seen_at;

-- name: ClaimIdentity :exec
-- Bind an anonymous UUID to an email. If another identity already has that
-- email, we instead migrate progress onto that row via a separate query.
update identities set email = $2, email_verified_at = now(), claimed_at = now()
where uuid = $1;

-- name: MigrateAnonymousProgress :exec
-- Copy progress row from anonymous UUID into the target identity, then drop the anon row.
with moved as (
  update progress set identity_uuid = $2 where identity_uuid = $1 returning 1
)
delete from identities where uuid = $1 and not exists (select 1 from moved);
