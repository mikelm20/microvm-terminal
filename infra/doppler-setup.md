# Doppler setup for learn-platform

Doppler replaces the plaintext files under `/etc/learn-platform/` (OAuth
token, cookie secret) with managed secrets injected at runtime via
`doppler run`. This document covers the first-time project bootstrap, the
inventory of every secret, and how each service consumes them.

## Project layout

Single project: `learn-platform`. Three environments:

| Config | Purpose                                                     | Access                              |
| ------ | ----------------------------------------------------------- | ----------------------------------- |
| `dev`  | Local development (laptops, CI preview). Fake keys.         | everyone                            |
| `stg`  | Shared staging host (future). Separate Anthropic workspace. | engineers                           |
| `prd`  | `learn-01.example.com`. The real workspace keys.            | two named operators only for writes |

Rotation is scoped per-config so rotating a dev credential never touches prod.

## One-time bootstrap

On Mikel's machine, signed in to Doppler as owner:

```
doppler login                                # browser auth
doppler projects create learn-platform
doppler configs create dev --project learn-platform
doppler configs create stg --project learn-platform
doppler configs create prd --project learn-platform

# Lock down prd so only the two owners can write.
doppler config-tokens create "platform-read-only" \
  --project learn-platform --config prd --access read
```

Check in `doppler.yaml` at the repo root so every clone runs
`doppler setup` against the right project + default config.

## Secret inventory

All seven configs hold the **same key names**; values differ per config.
The code never branches on config; it only reads environment variables.

| Key                            | Type                                                                      | Consumed by                  | Rotation                                                         |
| ------------------------------ | ------------------------------------------------------------------------- | ---------------------------- | ---------------------------------------------------------------- |
| `POSTGRES_URL`                 | URI `postgres://user:pass@host:5432/db`                                   | control-plane, proxy         | when DB user password rotates                                    |
| `RESEND_API_KEY`               | string                                                                    | control-plane (mail)         | when Resend dashboard rotates                                    |
| `ANTHROPIC_API_KEYS`           | JSON array: `[{"id":"k1","workspace":"learn-prd","secret":"sk-ant-..."}]` | proxy                        | quarterly or on compromise; triggers `POST /admin/rotate`        |
| `PROXY_TLS_CERT_FILE`          | filesystem path (rendered at boot)                                        | proxy                        | follows Caddy cert renewal                                       |
| `PROXY_TLS_KEY_FILE`           | filesystem path (rendered at boot)                                        | proxy                        | follows cert                                                     |
| `PROXY_TLS_CERT`               | PEM (raw, for dev)                                                        | proxy (dev config only)      | n/a                                                              |
| `PROXY_TLS_KEY`                | PEM (raw, for dev)                                                        | proxy (dev config only)      | n/a                                                              |
| `PROXY_ADMIN_TOKEN`            | random 32-byte hex                                                        | proxy admin endpoints        | any time; no downtime                                            |
| `SESSION_COOKIE_SECRET`        | 32-byte hex                                                               | control-plane (identity)     | rotating invalidates all logins (intentional)                    |
| `MAGIC_LINK_HMAC_KEY`          | 32-byte hex                                                               | control-plane (auth)         | rotating invalidates pending magic links                         |
| `CERT_SIGNING_ED25519_PRIVATE` | PEM                                                                       | control-plane (certificates) | breaks past-issued certificates; rotate never unless compromised |

Service token approach: each host runs `doppler` with a dedicated service
token scoped to one config. Tokens are stored in
`/etc/doppler/learn-platform.yaml` (mode 0640, owner `learn:learn`).

```
# On learn-01 (prd host), one-time:
sudo -u learn mkdir -p /var/lib/learn-platform
sudo -u learn doppler configure set token <service-token-prd> \
  --scope /var/lib/learn-platform
```

## Service integration

### control-plane

Systemd unit:

```
ExecStart=/usr/bin/doppler run --config prd --project learn-platform -- \
  /usr/local/bin/learn-control-plane
```

Inside the process, environment variables are the only way to read secrets;
there is no more `ClaudeOAuthTokenFile` or `AuthCookieSecretFile`. Any code
that still reads those paths is a bug; grep for them and cut them out during
the Gate merge.

`AuthPasswordFile` is the exception and stays on disk: the legacy MVP gate (`internal/auth/auth.go`) reads
`/etc/learn-platform/auth-password`. Magic-link auth lives in
`portal.example.com`, a separate product, not here.

### proxy

Systemd unit: see `infra/systemd/claude-proxy.service`. Secrets consumed:

- `ANTHROPIC_API_KEYS` -> keys pool on boot.
- `PROXY_ADMIN_TOKEN` -> gates `/admin/rotate` and `/admin/mint`.
- `PROXY_TLS_CERT_FILE` and `PROXY_TLS_KEY_FILE` -> TLS listener.
- `POSTGRES_URL` -> audit sink.

### web (Next.js)

`.env.local` is git-ignored. For dev, `doppler run -- pnpm dev` works. For
the self-hosted landing, the Caddy sidecar runs `doppler run -- next start`.

## Sunset of `/etc/learn-platform`

The following files are removed **after** Doppler migration succeeds on prd.
Bootstrap.sh no longer creates them. If any one of them still exists at
service start, log a warning and refuse to start:

- `/etc/learn-platform/claude-oauth-token` (replaced by `ANTHROPIC_API_KEYS`)
- `/etc/learn-platform/auth-cookie-secret` (replaced by `SESSION_COOKIE_SECRET`)

`/etc/learn-platform/auth-password` is intentionally retained: see the
control-plane section above.

The control plane no longer accepts these TOML keys:

```
claude_oauth_token_file
auth_cookie_secret_file
```

`auth_password_file` is still accepted.

Config.go reads from `os.Getenv` instead; `cmd/learn-cp/main.go` wires the
Resend client, session HMAC, and API key pool off the environment.

## Rotating `ANTHROPIC_API_KEYS`

1. Mint the new key in the Anthropic console (workspace `learn-prd`).
2. In Doppler, edit the `ANTHROPIC_API_KEYS` JSON array: add the new entry
   alongside the old. Commit.
3. Call the proxy rotate endpoint:

   ```sh
   curl -X POST \
     -H "Authorization: Bearer $(doppler secrets get PROXY_ADMIN_TOKEN --plain)" \
     --data "$(doppler secrets get ANTHROPIC_API_KEYS --plain)" \
     https://learn-01.example.com/proxy/admin/rotate
   ```

   The proxy drains in-flight calls, then swaps. In-flight sessions keep
   their old key until their VMs reap.

4. After 24 hours, remove the old key from the JSON array in Doppler and
   rotate again. Also revoke on the Anthropic dashboard.

## Emergency revocation

If a key leaks:

1. Revoke on Anthropic dashboard (kills every in-flight call immediately).
2. Remove from Doppler JSON array.
3. Trigger rotate endpoint as above; empty pool is rejected so add at least
   one healthy key first.
4. Audit `proxy_audit_log` for the 48 hours before the leak; filter by the
   key ID exposed via `X-Proxy-Key-Id` header on responses.
