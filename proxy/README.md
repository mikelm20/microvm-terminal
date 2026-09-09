# proxy

Optional Anthropic proxy. When deployed, a microVM's Claude Code talks to
this service on `:8443` (the bridge address, `172.20.0.1`) with an opaque
session token. The proxy looks up the backing Anthropic API key, enforces
per-session quota, streams the response back unchanged, and writes an audit
row to Postgres.

The default deployment does not use it: every VM carries the shared OAuth
token written at rootfs prep and talks to `api.anthropic.com` directly. The
control plane has no proxy wiring yet; deploying the proxy means minting a
session token per VM and pointing Claude Code at it inside the guest. The
code and tests are here because the quota and audit path is real and
rehearsed by `infra/gate-smoke.sh`.

## Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/v1/*` | `Authorization: Bearer sess_...` | Forward any Anthropic API call |
| POST | `/admin/mint` | `Authorization: Bearer <admin-token>` | Issue a fresh session token |
| POST | `/admin/rotate` | `Authorization: Bearer <admin-token>` | Drain + swap the key pool |
| GET  | `/healthz` | none | Liveness probe |

## Environment variables

Read from the process environment (`/etc/microvm-terminal/proxy.env` under
systemd):

- `ANTHROPIC_API_KEYS` (required) - JSON array of
  `{"id":"k1","workspace":"prod","secret":"sk-ant-..."}`. Only `id` is ever
  logged.
- `POSTGRES_URL` (optional) - if unset, audit uses an in-memory sink and
  records are dropped on restart. Prod must set this.
- `PROXY_ADMIN_TOKEN` (optional) - if unset, the admin endpoints 404 for
  everyone. Prod must set this.
- `PROXY_TLS_CERT_FILE`, `PROXY_TLS_KEY_FILE` (required unless `--plain`).

## Run locally (dev)

```
cd proxy
ANTHROPIC_API_KEYS='[{"id":"k1","workspace":"dev","secret":"sk-ant-FAKE"}]' \
  go run ./cmd/claude-proxy --plain --listen :8443
```

## Tests

```
cd proxy && go test ./...
```

Integration tests live under `tests/`. A fake Anthropic upstream runs via
`httptest.Server`; no real API calls.

## Stack surface

```
cmd/claude-proxy/main.go      wiring + TLS + graceful shutdown
internal/server/              HTTP handlers, request forwarding, streaming
internal/quota/               per-session caps and per-minute rate limit
internal/audit/               append-only Postgres sink (or InMemory for tests)
internal/keys/                pool of Anthropic keys + session token mapping
tests/                        integration tests against a fake upstream
```
