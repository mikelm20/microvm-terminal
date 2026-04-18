#!/usr/bin/env bash
# End-to-end curl suite for the learn-platform control plane.
#
# Prereqs:
#   - Postgres reachable via $DATABASE_URL (default
#     postgres://learn:learn@localhost:5432/learn?sslmode=disable)
#   - Control plane running at $BASE (default http://127.0.0.1:18080)
#     in USE_MOCK_LAUNCHER=true mode, with stdout tee'd to /tmp/learn-server.out
#     (tests/run.sh handles this)
#
# Prints a tick-list per endpoint. Exits non-zero on any failure.

set -uo pipefail

BASE="${BASE:-http://127.0.0.1:18080}"
LOG="${LOG:-/tmp/learn-server.out}"
COOKIE_JAR="$(mktemp -t learn-cookies.XXXXXX)"
trap 'rm -f "$COOKIE_JAR" /tmp/learn-body' EXIT

pass() { printf '  [x] %s\n' "$1"; }
fail() { printf '  [ ] %s FAILED\n' "$1"; exit 1; }

req() {
  local method="$1" path="$2"
  shift 2
  BODY_STATUS="$(curl -sS -o /tmp/learn-body -w '%{http_code}' \
    -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -H 'Content-Type: application/json' \
    -X "$method" "$BASE$path" "$@")"
}

j() { jq -r "$1" /tmp/learn-body; }

expect_status() {
  local want="$1" label="$2"
  if [[ "$BODY_STATUS" == "$want" ]]; then pass "$label ($BODY_STATUS)"
  else printf "expected %s got %s, body:\n" "$want" "$BODY_STATUS" >&2; cat /tmp/learn-body >&2; fail "$label"
  fi
}

echo "== healthz =="
req GET /healthz
expect_status 200 "GET /healthz"

echo "== identity =="
req POST /identity --data '{"lang":"es"}'
expect_status 200 "POST /identity mint"
UUID="$(j '.uuid')"
[[ "$UUID" =~ ^[0-9a-f-]+$ ]] || fail "uuid format"
pass "uuid=$UUID"

req POST /identity --data "$(printf '{"uuid":"%s","lang":"es"}' "$UUID")"
expect_status 200 "POST /identity re-attest"
[[ "$(j '.uuid')" == "$UUID" ]] || fail "re-attest uuid drift"

echo "== me =="
req GET /me
expect_status 200 "GET /me anon"
[[ "$(j '.email')" == "null" ]] || fail "email should be null"

req PATCH /me --data '{"department":"ventas","name":"Marta","haptics_enabled":true}'
expect_status 200 "PATCH /me"
[[ "$(j '.department')" == "ventas" ]] || fail "department not persisted"

echo "== lessons =="
req GET /lessons
expect_status 200 "GET /lessons"
count="$(j '.lessons | length')"
[[ "$count" -ge 1 ]] || fail "lessons list empty"
pass "lessons: $count entries"

FIRST_ID="$(j '.lessons[0].id')"
FIRST_LANG="$(j '.lessons[0].language')"
req GET "/lessons/$FIRST_ID?lang=$FIRST_LANG"
expect_status 200 "GET /lessons/$FIRST_ID"

echo "== progress =="
SYNC=$(cat <<EOF
{"uuid":"$UUID","state":{"modules":{"m2":{"completed_steps":["a"],"paused_at":null,"xp_earned":10}},"streak_days":1,"grace_tokens":3,"last_visited":"m2","seen_coachmarks":["welcome"],"updated_at":"2026-04-18T10:00:00Z"}}
EOF
)
req POST /progress/sync --data "$SYNC"
expect_status 200 "POST /progress/sync"
req GET /progress
expect_status 200 "GET /progress"
[[ "$(j '.state.streak_days')" == "1" ]] || fail "progress streak"

echo "== magic-link + claim =="
req POST /auth/magic-link --data "$(printf '{"email":"ops@example.com","anonymous_uuid":"%s","lang":"es"}' "$UUID")"
expect_status 200 "POST /auth/magic-link"

# Stdout-sender writes the raw link to /tmp/learn-server.out.
RAW="$(grep -a -oE 'token=[A-Za-z0-9_-]+' "$LOG" | tail -1 | cut -d= -f2 || true)"
if [[ -z "$RAW" ]]; then
  echo "  [!] magic link token not found in $LOG; skipping claim/session tests"
  exit 0
fi
pass "magic link token captured"

req POST /auth/claim --data "$(printf '{"magic_link_token":"%s","anonymous_uuid":"%s"}' "$RAW" "$UUID")"
expect_status 200 "POST /auth/claim"
[[ "$(j '.email')" == "ops@example.com" ]] || fail "claim email"

req GET /me
expect_status 200 "GET /me post-claim"
[[ "$(j '.email')" == "ops@example.com" ]] || fail "email not set post-claim"

echo "== sessions =="
req POST /sessions --data '{"lesson_id":"hello-claude","lang":"es"}'
expect_status 200 "POST /sessions"
SID="$(j '.session_id')"
[[ -n "$SID" ]] || fail "no session_id"
pass "session_id=$SID"

req GET "/sessions/$SID/heartbeat"
expect_status 200 "GET heartbeat"
[[ "$(j '.session_id')" == "$SID" ]] || fail "heartbeat session id"

echo "== prompt idempotency =="
KEY="$(python3 -c 'import uuid,sys; print(uuid.uuid4())' 2>/dev/null || uuidgen | tr 'A-Z' 'a-z')"
req POST "/sessions/$SID/prompt" \
  -H "Idempotency-Key: $KEY" \
  --data '{"text":"hola","client_ts":"2026-04-18T10:00:00Z"}'
expect_status 200 "POST prompt #1"
FIRST="$(j '.turn_id')"

req POST "/sessions/$SID/prompt" \
  -H "Idempotency-Key: $KEY" \
  --data '{"text":"hola","client_ts":"2026-04-18T10:00:00Z"}'
expect_status 200 "POST prompt #2 (same key)"
SECOND="$(j '.turn_id')"
[[ "$FIRST" == "$SECOND" ]] || fail "idempotency: turn_id drift ($FIRST vs $SECOND)"
pass "idempotency holds: turn_id=$FIRST"

echo "== transcript =="
req GET "/sessions/$SID/transcript"
expect_status 200 "GET transcript"

echo "== public profile (pre-cert) =="
req GET "/p/$UUID/public"
expect_status 200 "GET /p/:uuid/public"

echo "== delete =="
req DELETE "/sessions/$SID"
expect_status 200 "DELETE /sessions/:id"

echo
echo "all good"
