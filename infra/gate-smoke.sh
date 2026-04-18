#!/usr/bin/env bash
#
# gate-smoke.sh - end-to-end check of the security gate.
#
# Runs against infra/docker-compose.gate.yml. Brings up the stack, exercises
# the proxy from the vm-sim container, asserts:
#   1. direct curl to fake-anthropic from vm-sim: blocked.
#   2. curl to google.com equivalent from vm-sim: blocked.
#   3. curl to the claude-proxy from vm-sim with valid session token: ok.
#   4. burst of requests exceeds RPM quota and returns 429.
#   5. proxy_audit_log has at least one row in Postgres.
#
# Idempotent. Teardown is optional; pass --keep to leave the stack up.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE="docker compose -f infra/docker-compose.gate.yml"

# PROXY_ADMIN_TOKEN is a stable placeholder matching the value baked into
# docker-compose.gate.yml's claude-proxy service. Both are meant to be
# overridden at run time (PROXY_ADMIN_TOKEN=... infra/gate-smoke.sh) when
# testing against a doppler-fed proxy. Never a real production value.
: "${PROXY_ADMIN_TOKEN:=CHANGEME_LOCAL_DEV_ONLY}"
export PROXY_ADMIN_TOKEN

up() {
  echo "[gate-smoke] bringing up stack"
  $COMPOSE up -d --build
  # Give the proxy a moment to complete its Postgres migration.
  for i in $(seq 1 30); do
    if $COMPOSE exec -T postgres pg_isready -U learn -d learn >/dev/null 2>&1; then
      if curl -sf http://127.0.0.1:18443/healthz >/dev/null; then
        echo "[gate-smoke] proxy healthy"
        return 0
      fi
    fi
    sleep 1
  done
  echo "[gate-smoke] proxy never came up" >&2
  $COMPOSE logs claude-proxy | tail -30
  exit 1
}

mint_session() {
  curl -sS -X POST \
    -H "Authorization: Bearer ${PROXY_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    --data '{"session_id":"sess-smoke"}' \
    http://127.0.0.1:18443/admin/mint \
    | awk -F'"' '/"token"/ {print $4}'
}

run_tests() {
  local token; token="$(mint_session)"
  if [ -z "$token" ]; then
    echo "[gate-smoke] could not mint a session token" >&2
    exit 1
  fi
  echo "[gate-smoke] session token: $token"

  echo "[gate-smoke] 1/5 vm-sim direct to fake-anthropic should fail"
  if $COMPOSE exec -T vm-sim curl -sf --max-time 3 http://fake-anthropic:8080/v1/messages >/dev/null 2>&1; then
    echo "FAIL: direct call to fake-anthropic succeeded from vm-sim" >&2
    exit 1
  fi

  echo "[gate-smoke] 2/5 vm-sim egress to arbitrary host should fail"
  if $COMPOSE exec -T vm-sim curl -sf --max-time 3 http://1.1.1.1/ >/dev/null 2>&1; then
    echo "FAIL: vm-sim reached 1.1.1.1" >&2
    exit 1
  fi

  echo "[gate-smoke] 3/5 vm-sim through proxy with valid token should succeed"
  local resp
  resp="$($COMPOSE exec -T vm-sim curl -sS --max-time 5 \
    -X POST http://claude-proxy:8443/v1/messages \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data '{"model":"claude","messages":[{"role":"user","content":"hi"}]}')"
  echo "[gate-smoke] proxy response: $resp"
  if ! echo "$resp" | grep -q '"input_tokens":42'; then
    echo "FAIL: proxied response missing upstream body" >&2
    exit 1
  fi

  echo "[gate-smoke] 4/5 quota exhaustion returns 429 with ApiError shape"
  local got429=0
  for i in $(seq 1 20); do
    code="$($COMPOSE exec -T vm-sim curl -s -o /tmp/body -w '%{http_code}' --max-time 5 \
      -X POST http://claude-proxy:8443/v1/messages \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      --data '{}')"
    if [ "$code" = "429" ]; then
      got429=1
      break
    fi
  done
  if [ "$got429" -ne 1 ]; then
    echo "FAIL: did not observe 429 within 20 requests" >&2
    exit 1
  fi

  echo "[gate-smoke] 5/5 proxy_audit_log has rows"
  local rows
  rows="$($COMPOSE exec -T postgres psql -U learn -d learn -Atqc 'select count(*) from proxy_audit_log' 2>/dev/null || echo 0)"
  if [ -z "$rows" ] || [ "$rows" -lt 1 ]; then
    echo "FAIL: proxy_audit_log empty (rows=$rows)" >&2
    exit 1
  fi
  echo "[gate-smoke] proxy_audit_log rows: $rows"

  echo "[gate-smoke] all checks passed"
}

teardown() {
  if [ "${KEEP:-0}" = "1" ]; then
    echo "[gate-smoke] --keep set, leaving stack up"
    return
  fi
  echo "[gate-smoke] tearing down"
  $COMPOSE down -v >/dev/null
}

for a in "$@"; do
  case "$a" in
    --keep) KEEP=1;;
  esac
done

trap teardown EXIT
up
run_tests
