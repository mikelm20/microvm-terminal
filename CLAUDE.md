# learn-platform agent guide

Interactive Claude Code learning environment. A Go control plane on a bare metal host spawns Firecracker microVMs on demand; a Next.js web app (`apps/web/`) gives learners a terminal + wizard sidebar; the wizard advances as real events inside the VM match lesson predicates.

Product shape (ICP v2 + product brief v2, 2026-04-21): three-phase arc. Fundamentos (concepts, no terminal) to Terminal guiada (real embedded Claude Code, scaffolded commands on day 1, free composition on day N) to Autonomia (learner downloads Claude Code locally, company pays Anthropic license). Web surface, desktop plus mobile responsive. Mobile native is deferred. Never use "graduation" in copy or contracts.

## Host

- **Machine**: `learn-01`, `<host-ip>`
- **CPU/RAM/Disk**: redacted (second NVMe mounted at `/var/lib/firecracker`)
- **OS**: Ubuntu 24.04.3 LTS, kernel 6.8
- **SSH**: `ssh <user>@<host-ip>`
- **Public DNS**: `learn.example.com` -> <host-ip> (cloudflare, not proxied)

## Components (summary)

| Path | What it is |
|---|---|
| `control-plane/` | Go binary on the host. `POST/DELETE /sessions`, WS for wizard + PTY, Postgres state, warm pool, magic-link auth, Platform proxy surface. Running on `127.0.0.1:8080` behind Caddy; as of 2026-04-21 still on `mock launcher` (no real Firecracker boot yet). |
| `guest-agent/` | Go binary inside each VM (PID-1 child). vsock server: pty, events, inject, claude-wrap. HMAC-signed handshake with the control plane. |
| `vm-image/` | Scripts that build the Firecracker rootfs (ext4): Ubuntu 24.04 + Claude Code + Node + git + python + guest-agent. Includes `claude-wrap/` Go binary that parses `claude --output-format stream-json` and emits typed events over vsock. |
| `apps/web/` | Next.js 15 App Router frontend served behind Caddy under `learn.example.com`. xterm.js terminal + wizard sidebar, anon identity, PostHog telemetry. Not yet scaffolded as of 2026-04-21; Fase 1 kickoff. |
| `proxy/` | Platform-owned Anthropic proxy (Go) with per-session quota and audit log. Path for post-MVP auth cutover. |
| `shared/` | TS packages: `api` (Zod schemas + event types), `tokens` (design tokens), `voice` (copy tables es/en), `lessons` (YAML loader + schema). Consumed by `apps/web/` and by the landing repo. |
| `infra/` | `bootstrap.sh` (idempotent host setup), systemd units, Caddyfile, nftables rules for VM egress lockdown, Jailer wrappers, `docker-compose.dev.yml` for local dev stack. |
| `lessons/` | YAML lesson definitions, 8 modules bilingual es/en (`m2`..`m8` + `hello-claude`). Predicates: `file_exists`, `file_contents_regex`, `process_started`, `port_listening`, `claude_prompt_sent`, `claude_tool_call`, `command_exited`. |
| `tests/` | `learner-bot/` harness + `fixtures/claude/` corpus (one captured `stream-json` per lesson step). |

## Claude Code auth inside VMs (MVP)

Single `CLAUDE_CODE_OAUTH_TOKEN` minted via `claude setup-token` on operator machine. Stored on host in `/etc/learn-platform/claude-oauth-token` (mode 0600, owner `learn` user). Injected into each VM via guest-agent handshake at boot. Not baked into the rootfs.

**Cutover path for real users**: Anthropic Console API keys + per-session proxy with token/dollar quotas. The control plane config already reserves a `claude_proxy_url` field so MVP can be swapped without refactor.

## Ops rules for this repo

1. Every version is pinned (Firecracker, Go, Node, Caddy, kernel). Version bumps are commits, not drift.
2. Nothing mutates host state outside `infra/bootstrap.sh` and a future `infra/reconcile.sh`. No "I SSHed in and ran apt install".
3. The Claude OAuth token, any API keys, and any HMAC seeds live in `/etc/learn-platform/` (0600) on the host, **never** in the repo.
4. Gitleaks runs on every push (see `.github/workflows/gitleaks.yml`). Per team DevOps convention.
5. Structured JSON logs to journald for control plane and guest agent. No text logs.
6. `/metrics` (Prometheus) and `/healthz` on control plane from day 1.
7. `em dash` prohibited in code, docs, and commits (team-wide rule).

## Related

- ICP v2: `<drive-link>`
- Product brief v2: `<drive-link>`
- Pre-pivote assets (content, mocks, 8-department templates): `<drive-link>`. Useful as lesson material; the technical spec in that folder no longer applies.
- Landing repo (separate product surface): `<private-repo>` and `learn-landing` on GitHub. 15-level scripted demo at the root of `learn.example.com`.
- Sandbox repo (raw PTY, not this product): `<private-repo>` and `<private-repo>`. Browser Claude Code at `<internal-host>` for internal use, not the guided arc.
- DNS managed in `<private-repo>` (the `custom_records` map includes `learn`).
- DevOps IDP (separate product): see `<private-repo>` and the DevOps `03-tech/CLAUDE.md`. This repo is **not** part of the IDP.

## Monorepo conventions

Source of truth for every shared boundary: `CONTRACTS.md` at the repo root. Read it before writing anything that crosses package lines. If a contract is ambiguous or missing, open a `contracts-question` issue and wait.

- **Workspace**: pnpm + Turborepo. Root scripts: `pnpm typecheck`, `pnpm lint`, `pnpm build`, `pnpm codegen`, `pnpm format`.
- **Local dev stack**: `make dev-up` (Postgres 16 via `infra/docker-compose.dev.yml`), `make dev-down`, `make dev-logs`, `make psql`. Credentials in `.env.local` (gitignored). Template at `.env.local.example`.
- **Go modules**: `control-plane/`, `guest-agent/`, and the future `vm-image/claude-wrap/` and `proxy/` are independent Go modules. `make go-build` compiles every module that is present.
- **Codegen**: TS is the single source of truth. Regenerate Go types and voice key types with `pnpm codegen` whenever `shared/api/*.ts` or `shared/voice/es.json` change. CI fails if committed generated files are stale.
- **File ownership**: each agent writes only inside its own scope (see CONTRACTS.md section 8). Shared mutation targets (root `package.json`, `turbo.json`, `CONTRACTS.md`, this file) are owned by Agent-Tooling post-scaffold.
- **No em dashes.** Use comma, period, or colon. Enforced by review, not tooling.
