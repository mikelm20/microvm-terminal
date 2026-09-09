# learn-platform

A browser-based Claude Code trainer where every learner session is its own Firecracker microVM. A Go control plane on a bare-metal KVM host boots a VM per session in about 3 seconds, streams the VM's serial console to an xterm.js terminal over WebSocket, runs a real `claude` process inside the guest, and turns the agent's `stream-json` output into typed events that lesson predicates can match. Isolation is jailer plus cgroups v2 plus a custom seccomp filter plus an nftables egress allowlist plus a login shell that never reaches bash. Lesson content is in Spanish and English; the code and this README are in English.

Status: deployed to one host for a pilot, then paused. The isolation stack, VM lifecycle, terminal path, event parser and API proxy are real and tested. The lesson evaluator is not wired into the running daemon. See Known gaps before assuming anything else.

## Layout

```
control-plane/   Go. VM lifecycle, serial-console WebSocket, event bus, HTTP API, Postgres, auth.
guest-agent/     Go. Runs inside the VM. Reports process, port and file events over vsock.
vm-image/        Rootfs build (Docker to ext4), learn-shell login wrapper, claude-wrap stream-json parser.
proxy/           Go. Streaming reverse proxy to api.anthropic.com with per-session quotas and audit log.
apps/web/        Next.js 15. xterm.js terminal plus wizard sidebar.
shared/          TypeScript packages: Zod API schemas (source of truth for Go codegen), lesson schema, copy, tokens.
lessons/         41 YAML lesson files, es/en pairs.
infra/           bootstrap.sh, jailer wrapper, seccomp profile, nftables rules, systemd units, Caddyfile, compose stacks.
tests/           learner-bot harness plus captured stream-json fixtures per lesson step.
docs/            firecracker-boot.md: boot sequence, prerequisites, measured timings.
```

Four independent Go modules (`control-plane`, `proxy`, `guest-agent`, `vm-image/claude-wrap`) plus a pnpm and Turborepo workspace for the TypeScript side.

## Architecture

### VM lifecycle

`POST /sessions` goes through `session.Host` to `session.Manager.CreateWith` in `control-plane/internal/session/manager.go`. Cold boot, in order:

1. Capacity check against `MaxConcurrent` (default 3).
2. Network allocation: sequential IP from `172.20.0.0/24`, MAC derived from the IP, TAP name `tap-fcN`, monotonic vsock CID from 100. In-memory free list.
3. TAP created and attached to bridge `fc-br0`.
4. 32-byte random session token, hex encoded, passed on the kernel command line.
5. Rootfs: `cp --reflink=auto` of the golden ext4 image, then loop-mount the copy and write per-VM state straight into the guest filesystem: the networkd unit with the allocated IP, `/etc/hostname`, the Claude credentials file, and optional system-prompt overrides. `sync`, `umount`.
6. Launch under the jailer (default) or plain Firecracker. Boot args: `console=ttyS0 reboot=k panic=1 pci=off rw learn.session_token=<hex> learn.vsock_port=5555`.
7. A serial tee drains Firecracker stdout into a 256 KiB ring buffer so a detached VM keeps booting and a late WebSocket gets the boot log replayed.
8. vsock listener on `<vmdir>/fc.vsock_5555`.
9. Ready when the guest agent's `hello` frame carries the same session token. Boot timeout 90 s.

No snapshots. Every VM is a cold boot from a reflink copy. `DELETE /sessions/{id}` and a natural Firecracker exit both end in `Manager.Destroy`: SIGTERM, SIGKILL after 5 s, delete TAP, release IP, remove the VM directory.

### Isolation stack

- **Jailer** (`infra/jailer/jailer.sh`, on by default): sanitises the VM id to `[A-Za-z0-9-]` before it touches any path, builds a chroot under `/srv/jailer/firecracker/<vmid>/root`, copies kernel, rootfs and `vm.json` in, and execs `jailer --new-pid-ns --cgroup-version 2` as an unprivileged system user.
- **cgroups v2**: `cpu.max`, `memory.max` and `pids.max=512` written to `/sys/fs/cgroup/learn/<vmid>`. Defaults: 100,000 us quota per 100 ms period (one full CPU) and 1 GiB.
- **seccomp**: `infra/jailer/profile.d/learn-seccomp.json`, `defaultAction: SCMP_ACT_ERRNO`, x86_64 only, 85 explicitly allowed syscalls. Tighter than jailer's built-in level 2, which is the fallback when the profile is unset.
- **nftables egress allowlist** (`infra/nftables/learn.rules`, `table inet learn`): established/related accepted; VM-to-VM traffic dropped; DNS only to 1.1.1.1 and 8.8.8.8; tcp/8443 to the host-side proxy; tcp/443 to `api.anthropic.com` and apt mirror sets; everything else logged with prefix `LEARN_DROP` and dropped. The address sets are refreshed at boot and nightly by a systemd timer (`infra/nftables/refresh-sets.sh`).
- **No shell for the learner**: the `learner` user has no sudo, and its login shell is `vm-image/learn-shell.sh`, which traps INT, QUIT, TSTP and HUP and relaunches `claude-wrap` in a loop. There is no path to a bash prompt. Claude runs with `--dangerously-skip-permissions` because the VM is the permission boundary.
- No user namespaces. The control plane runs as root (`User=root` in the systemd unit) because it needs loop mounts, `ip link`, iptables and `/dev/kvm`.

### Networking

Host bridge `fc-br0` at `172.20.0.1/24`, created idempotently at daemon start. One TAP per VM. Static addressing is written into the guest's networkd unit at rootfs prep, so there is no DHCP. NAT is iptables MASQUERADE out of the auto-detected default interface, with every rule `-C` checked first so restarts do not stack duplicates. The nftables allowlist above sits on the same forward hook.

### Terminal path

There is no PTY over vsock. The terminal is the Firecracker serial console: the control plane holds the child process's stdin and stdout pipes, the guest runs `agetty --autologin learner` on `ttyS0`, and the learner's login shell is `learn-shell`.

```
browser  <-ws->  control plane  <-pipes->  firecracker  <-ttyS0->  agetty  ->  learn-shell  ->  claude-wrap  ->  claude
```

`GET /sessions/{id}/pty` speaks raw binary frames both ways. On connect the server sends the ring-buffer replay as one message, then streams. A second attach replaces the first.

### Events: stream-json to lesson predicates

`claude-wrap` (`vm-image/claude-wrap/`) runs `claude --output-format stream-json --input-format stream-json --verbose`, folds the envelopes into canonical events (`parser/parser.go`, unit-tested against captured fixtures in `tests/fixtures/claude/`), and publishes them one JSON object per line on a Unix socket. The guest agent tails that socket, adds its own `process_started`, `port_listening`, `file_exists` and `file_contents_regex` observations from `/proc`, and relays everything over vsock. The control plane fans events out on `GET /sessions/{id}/ws`, replaying persisted rows from Postgres first.

`control-plane/internal/lesson/evaluator.go` matches events against lesson step predicates. Eight predicate types: `agent_online`, `process_started`, `port_listening`, `claude_prompt_sent`, `claude_tool_call`, `claude_slash_command`, `file_exists`, `file_contents_regex`. Lessons are YAML validated by a Zod schema in `shared/lessons/schema.ts`; the Go types are generated from it with `pnpm codegen`.

The trade-off: learners get claude-wrap's echoed chat, not Claude Code's native TUI, because stream-json is what makes the predicate spine deterministic.

### Anthropic proxy with quotas

`proxy/` is a streaming reverse proxy on `:8443`. It authenticates a session token, reserves quota, forwards `/v1/*` with SSE preserved, parses the stream for token usage, commits actual usage, and writes audit rows to Postgres asynchronously. Keys come from an `ANTHROPIC_API_KEYS` JSON array, round-robin per session, with only the key id in the audit log so one key can be revoked. `/admin/rotate` swaps the key set without dropping in-flight calls.

Per-session defaults in `proxy/internal/quota/quota.go`:

| Quota | Value |
|---|---|
| Input tokens | 60,000 |
| Output tokens | 120,000 |
| Requests | 60 |
| Requests per minute | 12 |

`infra/docker-compose.gate.yml` plus `infra/gate-smoke.sh` rehearse the whole gate without a hypervisor: a VM simulator container, a fake Anthropic endpoint, the proxy and Postgres. The smoke test asserts direct egress is blocked, proxied egress works, an exhausted session gets 429, and audit rows land.

## Measured numbers

Cold boot to guest-agent handshake on the production host, from `docs/firecracker-boot.md`:

| Step | Time |
|---|---|
| Rootfs ready to guest-agent online | 3,214 ms |
| Session ready (logged) | 3,215 ms |
| Integration test pass window | 4 to 8 s |

Limits as configured:

| Limit | Value | Where |
|---|---|---|
| vCPU visible to guest | 2 | `manager.go` |
| RAM visible to guest | 2,048 MiB | `manager.go` |
| cgroup `cpu.max` | 1 CPU | `JailerCPUQuotaMicros` |
| cgroup `memory.max` | 1 GiB | `JailerMemBytes` |
| cgroup `pids.max` | 512 | `jailer.sh` |
| Concurrent VMs | 3 | `MaxConcurrent` |
| Boot timeout | 90 s | `BootTimeoutSeconds` |
| Idle timeout | 300 s, declared, not enforced | `IdleTimeoutSeconds` |
| Warm pool | 0, disabled | `WarmPoolTarget` |
| Prompt body | 4,096 chars | `api/sessions.go` |
| Attach upload | 16 MiB | `api/sessions.go` |
| Preview proxy | 120 req per 10 s per identity | `api/preview.go` |

## How it was built

- 46 commits between 2026-04-18 and 2026-04-22. 27k lines across 278 tracked files, excluding lockfiles and fixtures.
- `CONTRACTS.md` freezes every cross-package interface and, in section 8, splits the work into scoped tracks, each with an owned file set and exit criteria. Files such as the `Makefile` carry an ownership note naming the track that owns them. An ambiguous contract was resolved by opening a `contracts-question` issue before continuing.
- The merge history is a series of scoped branches: `gate/real-launcher`, `spine/predicates`, `api/endpoints`, `gate/security`, `web/scaffold`, `content/f2-lessons`, `tooling/scaffold`.
- `CLAUDE.md` holds the working conventions: pinned versions, codegen rules, style. Coordination lives entirely in those two files.
- Two bug-fix commits were added on 2026-09-09 while preparing this public copy (see Known gaps).

## Known gaps

Stated plainly because a reader who finds them unannounced will trust nothing else here.

- **The lesson evaluator is not wired into the control plane.** `lesson.NewEvaluator` is only constructed by the learner-bot harness and the package's own tests. In the deployed app the wizard advances when the learner clicks Next (`WizardSidebar.tsx` calls this the offline fallback). The predicate spine is exercised only against captured fixtures.
- **Warm pool ships disabled** (`WarmPoolTarget: 0`) and warm VMs cannot carry rootfs overrides, so the capstone endpoints bypass it regardless.
- **cgroup limits contradict the VM config.** The guest is told it has 2 vCPU and 2 GiB; the cgroup caps it at 1 CPU and 1 GiB. Memory pressure shows up as a cgroup OOM kill, not as guest pressure.
- **No PTY resize and no PTY reconnect.** The browser fits xterm locally but never sends geometry, and there is no PTY to apply it to. The terminal socket opens once and prints a disconnect message on close; only the event socket has backoff and reconnect.
- **The proxy is built and tested but not in the request path.** Every VM still carries one shared OAuth token injected at rootfs prep, so none of the quotas above are enforced, and a proxy restart clears in-memory quota state. The control plane has no proxy URL setting yet, whatever older notes say.
- **No idle reaper.** `IdleTimeoutSeconds` is parsed and never read.
- The serial tee has exactly one subscriber and drops writes to a slow one instead of applying back-pressure.
- iptables (written by the daemon) and nftables (loaded by systemd) both manage the forward hook. It works because nftables `drop` wins, but two subsystems on one chain is an operational hazard.
- `learner-bot -mode live` is not implemented; only fixture mode runs.
- `.woodpecker/*.yml` are stale copies from a sibling landing repo and do not deploy this project. There is no deploy pipeline; deployment was manual.
- `run_migrations()` in `infra/bootstrap.sh` is a placeholder.
- Fixed on 2026-09-09, in the two most recent commits: `Host.Destroy` previously dropped the map entry without stopping the VM, which leaked the process, TAP, IP and directory and wedged the host after three sessions; and `s.done` could be closed twice by the reaper goroutine and `Host.Destroy`, which panics in a bare goroutine. Both have regression tests in `control-plane/internal/session/session_test.go`.

## Running locally

Requirements: Go 1.25, Node 22, pnpm, Docker. A real VM boot additionally needs Linux x86_64, `/dev/kvm`, root, and the artefacts described in `docs/firecracker-boot.md`.

```
pnpm install
make dev-up            # Postgres 16 via infra/docker-compose.dev.yml
cp .env.local.example .env.local
make go-build          # builds every Go module present
pnpm typecheck && pnpm lint
cd control-plane && USE_MOCK_LAUNCHER=true go run ./cmd/learn-control-plane   # API without a hypervisor
```

Tests:

```
cd control-plane && go test ./...
cd proxy && go test ./...
cd vm-image/claude-wrap && go test ./...
cd tests/learner-bot && go test ./...   # fixture replay only; the learner-bot workflow runs this on every push
sudo make integration-vm   # boots one real Firecracker VM end to end, Linux + KVM only
infra/gate-smoke.sh        # egress and quota rehearsal in docker-compose
```

Make targets: `help`, `dev-up`, `dev-down`, `dev-restart`, `dev-logs`, `dev-ps`, `psql`, `codegen`, `typecheck`, `lint`, `build`, `go-build`, `integration-vm`.

Host provisioning is `infra/bootstrap.sh`: idempotent, root-only, pins Firecracker v1.10.1, Go 1.25.0 and Node 22, verifies KVM, formats the data disk, installs jailer, seccomp profile, nftables rules, Caddy and the systemd units.

## Placeholders

Host addresses, SSH details, hardware identifiers, contact names and internal document paths in this repository are placeholders such as `<host-ip>`, `<ssh-key-path>` and `<drive-link>`. Substitute your own when deploying.

## License

MIT. See `LICENSE`.
