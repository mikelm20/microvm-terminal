-- name: InsertSession :exec
insert into sessions (id, identity_uuid, lesson_id, lang, department, vm_ip, warm, ready_at, last_event_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, now());

-- name: GetSession :one
select id, identity_uuid, lesson_id, lang, department, vm_ip, warm,
       created_at, ready_at, reaped_at, grace_started_at, last_event_at
from sessions where id = $1;

-- name: MarkSessionReaped :exec
update sessions set reaped_at = now() where id = $1 and reaped_at is null;

-- name: TouchSession :exec
update sessions set last_event_at = now() where id = $1;

-- name: InsertSessionEvent :exec
insert into session_events (session_id, ts, type, payload)
values ($1, $2, $3, $4);

-- name: GetSessionEvents :many
select session_id, seq, ts, type, payload
from session_events
where session_id = $1
order by seq asc;

-- name: UpsertPromptIdempotency :one
insert into prompt_idempotency (session_id, idempotency_key, turn_id)
values ($1, $2, $3)
on conflict (session_id, idempotency_key) do update set idempotency_key = excluded.idempotency_key
returning turn_id;
