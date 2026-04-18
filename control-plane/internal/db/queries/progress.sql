-- name: UpsertProgress :one
insert into progress (identity_uuid, state, updated_at)
values ($1, $2, now())
on conflict (identity_uuid) do update set state = excluded.state, updated_at = now()
returning state, updated_at;

-- name: GetProgress :one
select identity_uuid, state, updated_at from progress where identity_uuid = $1;

-- name: GetProgressStats :one
-- Totals derived from progress.state jsonb, for MeResponse.
select coalesce((state->>'streak_days')::int, 0) as streak_days,
       coalesce((state->>'grace_tokens')::int, 3) as grace_tokens,
       coalesce((
         select sum((m.value->>'xp_earned')::int)
         from jsonb_each(state->'modules') as m
       ), 0) as total_xp
from progress where identity_uuid = $1;
