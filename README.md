# learn-platform

Web app that teaches Claude Code by embedding it. A learner opens the page, enters a three-phase arc (Fundamentos concepts, Terminal guiada with a real VM-backed Claude Code, Autonomia handoff), and a wizard advances as real events inside the VM match lesson predicates.

Product: `learn.example.com` (landing at root from the separate `learn-landing` repo; this repo serves the app under a path prefix).
Host: `learn-01` (bare metal, provider and SKU redacted).
Owner: Platform Engineering LLC, linea IA. Product direction in `<drive-link>` and `<drive-link>`.

## Layout

```
apps/web/        Next.js 15 App Router: three-phase arc, xterm.js terminal, wizard sidebar. Fase 1 kickoff.
control-plane/   Go binary on the host: VM lifecycle, wizard event bus, HTTP/WS API, Postgres, auth, warm pool.
guest-agent/     Go binary inside each VM: PTY bridge, event emitter, vsock handshake.
vm-image/        Firecracker rootfs build (Ubuntu 24.04 + Claude Code). Includes claude-wrap that parses stream-json.
proxy/           Platform-owned Anthropic proxy (per-session quota, audit log) for post-MVP auth cutover.
shared/          TS packages: api (Zod + events), tokens, voice (es/en), lessons (loader + schema).
lessons/         YAML lesson definitions. 8 modules (m2..m8 + hello-claude), bilingual.
infra/           bootstrap.sh, systemd units, Caddy config, nftables rules, Jailer wrappers, docker-compose.dev.yml.
tests/           learner-bot harness + fixtures/claude/ captured stream-json per lesson step.
```

## Auth (MVP)

VMs are pre-authenticated with a `CLAUDE_CODE_OAUTH_TOKEN` minted via `claude setup-token`, stored on host at `/etc/learn-platform/claude-oauth-token` (0640), injected into each VM via guest-agent handshake at boot. Not baked into the rootfs.

Cutover path for real learners: per-session Anthropic Console API keys routed through `proxy/` with token and dollar quotas. The control plane already reserves a `claude_proxy_url` config field so the swap is a config change.

## Status (2026-04-21)

- `control-plane` deployed and running on `learn-01` behind Caddy, but on `mock launcher`: no real Firecracker boot yet.
- Contracts frozen in `CONTRACTS.md`. Surface pivoted to web-first on 2026-04-21.
- `apps/web/` not scaffolded; next step.
- Fase 1 (per product brief v2): 10 days to convert one `learn-landing` lesson into a real terminal-guided flow, telemetry minimal, pilot with 5 to 10 employees of Pilot Client via <contact>. Measure D3/D7/D14 retention.
