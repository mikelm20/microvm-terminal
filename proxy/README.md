# proxy

Platform-owned Anthropic proxy. Every microVM's Claude Code talks to this
service on `:8443` with an opaque session token. The proxy looks up the
backing Anthropic API key, enforces per-session quota, streams the response
back unchanged, and writes an audit row to Postgres.

See `../CONTRACTS.md` section 8 (Agent-Gate) for the contract. See
`../infra/doppler-setup.md` for the secrets this binary reads from the
environment.

## Endpoints

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/v1/*` | `Authorization: Bearer sess_...` | Forward any Anthropic API call |
| POST | `/admin/mint` | `Authorization: Bearer <admin-token>` | Issue a fresh session token |
| POST | `/admin/rotate` | `Authorization: Bearer <admin-token>` | Drain + swap the key pool |
| GET  | `/healthz` | none | Liveness probe |

## Environment variables

Consumed via `doppler run -- claude-proxy`:

- `ANTHROPIC_API_KEYS` (required) - JSON array. See
  `../infra/doppler-setup.md` for shape.
- `POSTGRES_URL` (optional) - if unset, audit uses an in-memory sink and
  records are dropped on restart. Prod must set this.
- `PROXY_ADMIN_TOKEN` (optional) - if unset, the admin endpoints 404 for
  everyone. Prod must set this.
- `PROXY_TLS_CERT_FILE`, `PROXY_TLS_KEY_FILE` (required unless `--plain`).

## Run locally (dev)

```
cd proxy
doppler run --config dev -- go run ./cmd/claude-proxy --plain --listen :8443
# or, without Doppler:
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
