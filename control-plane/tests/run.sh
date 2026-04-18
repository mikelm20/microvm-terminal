#!/usr/bin/env bash
# Starts a fresh control-plane against the local Postgres, redirects logs
# to /tmp/learn-server.out so the curl suite can harvest the magic-link
# token, runs tests/api.sh, and tears the server down.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

DB_URL="${DATABASE_URL:-postgres://learn:learn@localhost:5432/learn?sslmode=disable}"
PORT="${PORT:-18080}"
BASE="http://127.0.0.1:$PORT"
LOG="/tmp/learn-server.out"

echo "wiping dev DB..."
PSQL="docker exec -i learn-postgres psql -U learn -d learn"
$PSQL -c "DROP TABLE IF EXISTS schema_migrations, identities, sessions, session_events, progress, magic_links, auth_sessions, certificates, prompt_idempotency CASCADE;" >/dev/null

echo "building control plane..."
( cd "$repo/control-plane" && go build -o /tmp/learn-cp ./cmd/learn-control-plane )

echo "starting server on :$PORT..."
: > "$LOG"
DATABASE_URL="$DB_URL" \
LESSONS_DIR="$repo/lessons" \
VOICE_DIR="$repo/shared/voice" \
LEARN_LISTEN_ADDR="127.0.0.1:$PORT" \
PUBLIC_ORIGIN="$BASE" \
USE_MOCK_LAUNCHER=true \
/tmp/learn-cp -config /dev/null >"$LOG" 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT

# Wait for /healthz
for _ in $(seq 1 30); do
  if curl -sf "$BASE/healthz" >/dev/null; then break; fi
  sleep 0.2
done

BASE="$BASE" LOG="$LOG" "$here/api.sh"
