# learn-platform agent guide

Interactive Claude Code learning environment. A Go control plane on a bare metal host spawns Firecracker microVMs on demand; a Next.js frontend gives learners a terminal + wizard sidebar; the wizard advances as real events inside the VM match lesson predicates.

## Host

- **Machine**: `learn-01`, `<host-ip>`
- **CPU/RAM/Disk**: redacted (second NVMe mounted at `/var/lib/firecracker`)
- **OS**: Ubuntu 24.04.3 LTS, kernel 6.8
- **SSH**: `ssh <user>@<host-ip>`
- **Public DNS**: `learn.example.com` -> <host-ip> (cloudflare, not proxied)

## Components (summary)

| Path | What it is |
|---|---|
| `control-plane/` | Go binary on the host. `POST/DELETE /sessions`, WS for wizard + PTY. VM pool cap = 3 for MVP. State in sqlite. |
| `guest-agent/` | Go binary inside each VM (PID-1 child). vsock server: pty, events, inject, claude-wrap. HMAC-signed handshake with the control plane. |
| `vm-image/` | Scripts that build the Firecracker rootfs (ext4): Ubuntu 24.04 + Claude Code + Node + git + python + guest-agent. |
| `web/` | Next.js frontend served behind Caddy at `learn.example.com`. xterm.js + wizard sidebar. |
| `infra/` | `bootstrap.sh` (idempotent host setup), systemd units, Caddyfile, iptables rules for VM egress lockdown. |
| `lessons/` | YAML lesson definitions. Predicates: `file_exists`, `file_contents_regex`, `process_started`, `port_listening`, `claude_prompt_sent`, `claude_tool_call`, `command_exited`. |

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

- Architecture source doc: `<drive-link>`
- DNS managed in `<private-repo>` (the `custom_records` map includes `learn`).
- DevOps IDP (separate product): see `<private-repo>` and the DevOps `03-tech/CLAUDE.md`. This repo is **not** part of the IDP.
