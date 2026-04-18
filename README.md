# learn-platform

Interactive learning environment for Claude Code. Browser UI drives real Firecracker microVMs each running Claude Code, with a wizard that advances as the learner triggers real events inside the VM.

Product: `learn.example.com`
Host: `learn-01` (bare metal, provider and SKU redacted)
Owner: Platform Engineering LLC, linea IA

## Layout

```
control-plane/   Go binary on the host: VM lifecycle, wizard event bus, HTTP/WS API
guest-agent/     Go binary inside each VM: PTY bridge, event emitter, claude-wrap
vm-image/        Reproducible Firecracker rootfs build (Ubuntu 24.04 + Claude Code)
web/             Next.js frontend: landing, xterm.js terminal, wizard sidebar
infra/           bootstrap.sh, systemd units, Caddy config, iptables rules
lessons/         YAML lesson definitions (content lives in git, MVP has one throwaway)
```

## Auth (MVP only)

VMs are pre-authenticated with a `CLAUDE_CODE_OAUTH_TOKEN` minted via `claude setup-token` on an operator machine. One token, injected per VM at boot, shared across the 2-3 VMs we run. This is individual-use by the operator for internal testing.

Before the platform is opened to real learners: cut over to per-session Anthropic Console API keys behind a proxy with per-session quotas (see `architecture/infra-mvp-design.md` in `03-tech/learning-claude-code/`). The control plane keeps a proxy-shaped config surface so the swap is a config change, not a rewrite.

## Status

Phase 0 in progress. See the task list in the parent Claude Code session, or `infra/bootstrap.sh` for what the host has been configured to do.
