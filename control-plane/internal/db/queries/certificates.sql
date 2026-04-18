-- name: GetCertificate :one
select id, identity_uuid, lesson_id, completed_at, signature, created_at
from certificates where id = $1;

-- name: ListCertificatesByIdentity :many
select id, identity_uuid, lesson_id, completed_at, signature, created_at
from certificates where identity_uuid = $1 order by completed_at asc;
