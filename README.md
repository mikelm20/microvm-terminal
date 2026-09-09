# microvm-terminal

Open a page, log in, and you are in a terminal running Claude Code inside your own Firecracker microVM. A Go control plane on a KVM host boots one VM per session in about 3 seconds, bridges the VM's serial console to an xterm.js page over WebSocket, and destroys the VM when you close it or leave it idle. Isolation is jailer plus cgroups v2 plus a custom seccomp filter plus an nftables egress allowlist plus a login shell that never reaches bash.

Status: the VM layer, the terminal path and the login flow build and pass their tests. The resize path and the idle reaper are covered by unit tests and a real-host integration test, but the rootfs image has not been rebuilt with the new guest agent by the author on a KVM host. See Known limits before assuming anything else.

## Numbers

Measured on the original host (Ubuntu 24.04, kernel 6.8, Firecracker v1.10.1), from `docs/firecracker-boot.md`:

| Step | Time |
|---|---|
| Rootfs ready to guest agent online | 3,214 ms |
| Session ready (logged) | 3,215 ms |
| Integration test pass window | 4 to 8 s |

Defaults as configured (`control-plane/config.example.yaml`):

| Limit | Value |
|---|---|
| vCPU visible to guest | 2 |
| RAM visible to guest | 2,048 MiB |
| cgroup `cpu.max` | 2 cores (derived from vCPU) |
| cgroup `memory.max` | 2,304 MiB (guest RAM + 256 MiB for Firecracker) |
| cgroup `pids.max` | 512 |
| Concurrent VMs | 3 |
| Boot timeout | 90 s |
| Idle timeout | 300 s, enforced |
| Warm pool | 0, disabled |
| WebSocket keepalive ping | every 30 s, 10 s timeout |
| Client reconnect | 1 s doubling to 15 s, 8 attempts |
| Serial replay buffer | 256 KiB |
| seccomp allowlist | 85 syscalls, x86_64 only, default `SCMP_ACT_ERRNO` |

## Layout

```
control-plane/   Go. Login, sessions API, serial-console WebSocket, VM lifecycle, Postgres. Serves the two pages.
guest-agent/     Go. Runs inside the VM: vsock hello for readiness, TIOCSWINSZ on /dev/ttyS0 for resize.
vm-image/        Rootfs build (Docker to ext4), guest-shell.sh menu loop, guest agent unit.
proxy/           Go. Optional streaming reverse proxy to api.anthropic.com with per-session quotas. Off by default.
infra/           bootstrap.sh, jailer wrapper, seccomp profile, nftables rules, systemd units, Caddyfile, compose stacks.
docs/            firecracker-boot.md: boot sequence, prerequisites, measured timings.
```

Three independent Go modules: `control-plane`, `guest-agent`, `proxy`.

## How a session boots

`POST /sessions` goes through `session.Host` to `session.Manager.Create` in `control-plane/internal/session/manager.go`:

1. Capacity check against `max_concurrent`.
2. Network allocation: sequential IP from `172.20.0.0/24`, MAC derived from the IP, TAP name `tap-fcN`, monotonic vsock CID from 100. In-memory free list.
3. TAP created and attached to bridge `fc-br0`.
4. 32-byte random session token, hex encoded, passed on the kernel command line as `mvt.session_token`.
5. Rootfs: `cp --reflink=auto` of the golden ext4 image, then loop-mount the copy and write per-VM state into the guest filesystem: the networkd unit with the allocated IP, `/etc/hostname`, `~/.claude/.credentials.json` with the OAuth token, and the onboarding marker. `sync`, `umount`.
6. Launch under the jailer (default) or plain Firecracker. Boot args: `console=ttyS0 reboot=k panic=1 pci=off rw mvt.session_token=<hex> mvt.vsock_port=5555`.
7. A serial tee drains Firecracker stdout into a 256 KiB ring buffer so a detached VM keeps booting and a late WebSocket gets the boot log replayed.
8. vsock listener on `<vmdir>/fc.vsock_5555`.
9. Ready when the guest agent's `hello` frame carries the same session token. `POST /sessions` returns only after this, or fails after the boot timeout.

No snapshots. Every VM is a cold boot from a reflink copy. `DELETE /sessions/{id}`, the idle reaper, the menu's exit option (which powers the guest off) and a Firecracker crash all end in `Manager.Destroy`: SIGTERM, SIGKILL after 5 s, delete TAP, release IP, remove the VM directory, stamp `reaped_at` in Postgres.

## Isolation stack

- **Jailer** (`infra/jailer/jailer.sh`, on by default): sanitises the VM id to `[A-Za-z0-9-]` before it touches any path, builds a chroot under `/srv/jailer/firecracker/<vmid>/root`, copies kernel, rootfs and `vm.json` in, and execs `jailer --new-pid-ns --cgroup-version 2` as the unprivileged `microvm` user.
- **cgroups v2**: `cpu.max`, `memory.max` and `pids.max=512` at `/sys/fs/cgroup/microvm/<vmid>`. CPU and memory are derived from the guest size unless set explicitly, so the cgroup never contradicts what the guest was told.
- **seccomp**: `infra/jailer/profile.d/seccomp.json`, 85 allowed syscalls, tighter than jailer's built-in level 2, which is the fallback when the profile is unset.
- **nftables egress allowlist** (`infra/nftables/microvm.rules`, `table inet microvm`): established/related accepted; VM-to-VM traffic dropped; DNS only to 1.1.1.1 and 8.8.8.8; tcp/8443 to the host-side proxy at the bridge address; tcp/443 to `api.anthropic.com` and apt mirror sets; everything else logged with prefix `MICROVM_DROP` and dropped. The address sets are refreshed at boot and nightly by a systemd timer.
- **No shell for the user**: the `dev` user in the guest has no sudo except `poweroff`, and its login shell is `vm-image/guest-shell.sh`, which traps INT, QUIT, TSTP and HUP and loops a menu. There is no path to a bash prompt from the console. Claude runs with `--dangerously-skip-permissions` because the VM is the permission boundary.
- No user namespaces. The control plane runs as root because it needs loop mounts, `ip link`, iptables and `/dev/kvm`. Firecracker itself does not: the jailer drops it to `microvm`.

## Networking

Host bridge `fc-br0` at `172.20.0.1/24`, created idempotently at daemon start. One TAP per VM. Static addressing is written into the guest's networkd unit at rootfs prep, so there is no DHCP. NAT is iptables MASQUERADE out of the auto-detected default interface, with every rule `-C` checked first so restarts do not stack duplicates. The nftables allowlist sits on the same forward hook.

## Terminal path

There is no PTY over vsock. The terminal is the Firecracker serial console: the control plane holds the child process's stdin and stdout pipes, the guest runs `agetty --autologin dev` on `ttyS0`, and the login shell is `guest-shell`.

```
browser  <-ws->  control plane  <-pipes->  firecracker  <-ttyS0->  agetty  ->  guest-shell  ->  claude
```

`GET /sessions/{id}/pty` speaks binary frames both ways for terminal bytes. Text frames from the client are JSON control messages; the only one is `{"type":"resize","cols":N,"rows":M}`. On connect the server sends the ring-buffer replay as one message, then streams. A second attach replaces the first.

**Resize.** A serial console has no window size, so `TIOCSWINSZ` cannot be applied on the host. The control plane stores the last geometry on the session and forwards it over the existing vsock connection to the guest agent, which applies it to `/dev/ttyS0` with `TIOCSWINSZ`. The kernel stores the size on the tty and sends `SIGWINCH` to the foreground process group, which is how bash, the menu and Claude Code see it on a real pty too. A geometry sent before the guest is up is replayed the moment the handshake completes. Until then the tty is the kernel default of 80x24; the menu runs after the agent, so the first interactive program already has the right size.

**Keepalive and reconnect.** The server pings every 30 s and drops a client that does not answer within 10 s. The page reconnects with exponential backoff (1 s to 15 s, 8 attempts) while `GET /sessions/{id}` still reports the VM alive, clearing the screen first because the server replays its ring buffer on every attach. When the VM is gone it shows why (shut down, reaped, unreachable) and offers a new VM.

## The menu loop

`vm-image/guest-shell.sh` is the `dev` user's login shell:

```
1) New session
2) Continue the last session
3) Pick an earlier session
4) Change folder
5) Exit and shut down this VM
```

Options 1 to 3 run `claude --dangerously-skip-permissions`, `--continue` and `--resume`. Option 4 changes directory under `~/workspace` and refuses paths outside it. Option 5 runs `sudo -n /sbin/poweroff`, the single sudoers entry, which makes Firecracker exit, which the control plane reaps, which closes the WebSocket with reason `vm exited`, which makes the page offer a new VM. Ctrl+C, Ctrl+\ and Ctrl+Z at the menu are trapped; Ctrl+D redraws.

## Session flow from the browser

1. `GET /` shows the login form. `POST /login` with a name and the shared password sets an HMAC-signed cookie carrying the slugified name (30 days, `HttpOnly`, `SameSite=Lax`, `Secure` unless disabled). Login is rate-limited to 5 attempts per IP per minute.
2. `GET /terminal` loads the page. It calls `GET /sessions/current`; if the caller has a live VM it reattaches, otherwise `POST /sessions` boots one.
3. The page opens `GET /sessions/{id}/pty`, sends its geometry, and streams.
4. "New VM" does `DELETE /sessions/{id}` then boots again. "Log out" clears the cookie; the VM stays alive until the idle reaper takes it.

Sessions are owned by the cookie's name; another name gets 404 on someone else's VM. The Postgres `sessions` table is a history (owner, IP, created, ready, reaped, last attached), not the source of truth for liveness, which is the in-memory `session.Host`.

## Configuration

One YAML file, `/etc/microvm-terminal/config.yaml`, with every key documented in `control-plane/config.example.yaml`. Environment variables of the same name in upper case override the file (`MAX_CONCURRENT`, `IDLE_TIMEOUT_SECONDS`, `VM_VCPU`, `VM_MEM_MIB`, `PASSWORD_FILE`, `CLAUDE_OAUTH_TOKEN_FILE`, `USE_MOCK_LAUNCHER`, ...). The keys that matter most:

| Key | Default | Meaning |
|---|---|---|
| `max_concurrent` | 3 | VMs at once; `POST /sessions` returns 503 with `retry_after_seconds` above it |
| `vm.vcpu`, `vm.mem_mib` | 2, 2048 | guest size; cgroup limits derive from these |
| `idle_timeout_seconds` | 300 | destroy a VM with no attached terminal for this long; 0 disables |
| `boot_timeout_seconds` | 90 | fail the boot if the guest never handshakes |
| `password_file` | `/etc/microvm-terminal/password` | shared login password, 8+ characters |
| `claude_oauth_token_file` | `/etc/microvm-terminal/claude-oauth-token` | output of `claude setup-token`, written into every VM |
| `jailer.enabled` | true | route launches through jailer.sh |
| `use_mock_launcher` | false | in-memory sessions without KVM (tests, macOS) |

## Host requirements and setup

Ubuntu 24.04 on x86_64 with `/dev/kvm` (bare metal or nested virtualisation). `infra/bootstrap.sh` is idempotent and root-only: it pins Firecracker v1.10.1 and Go 1.25.0, verifies KVM, installs Postgres, Caddy, Docker (for the rootfs build), nftables and ufw, creates the `microvm` service user and database, installs the jailer wrapper, seccomp profile, nftables rules and systemd units, writes a config from the example and generates the login password.

```
sudo bash infra/bootstrap.sh
sudo make -C vm-image all            # rootfs.ext4 + vmlinux into /var/lib/firecracker/images
make -C control-plane install        # /usr/local/bin/microvm-terminal
sudo install -m 0600 <token-file> /etc/microvm-terminal/claude-oauth-token
sudo systemctl start microvm-terminal
```

Put the host name in `infra/caddy/Caddyfile` and `public_origin` in the config. Caddy terminates TLS and proxies everything to `127.0.0.1:8080`.

## Running locally

Requirements: Go 1.25, Docker for Postgres. A real VM boot additionally needs Linux x86_64, `/dev/kvm`, root, and the artefacts described in `docs/firecracker-boot.md`.

```
make dev-up      # Postgres 16 via infra/docker-compose.dev.yml
make run-mock    # control plane with the mock launcher on http://127.0.0.1:8080, password dev-password-1
make test        # go vet + go test in every module
sudo make integration-vm   # boots one real Firecracker VM end to end, Linux + KVM only
infra/gate-smoke.sh        # proxy quota and egress rehearsal in docker-compose
```

With the mock launcher the login, pages and sessions API work, but the terminal has no console to attach to.

## Known limits

- **The rootfs has not been rebuilt by the author with the new guest agent and menu shell.** The Dockerfile and scripts are updated and the agent builds for linux/amd64, but `sudo make -C vm-image all` and `make integration-vm` on a KVM host are the remaining verification steps. Everything measured above was measured with the previous guest image, whose boot path (systemd, networkd, agetty, vsock agent) is unchanged.
- **Resize depends on the guest agent.** Before the handshake, or if the agent dies, the tty stays at its last size. There is no host-side fallback because the serial console has no window size to set.
- **One serial subscriber.** A second browser tab attaching to the same VM replaces the first. The serial tee drops writes to a slow subscriber instead of applying back-pressure.
- **Shared OAuth token.** Every VM carries the same `claude setup-token` credential, so usage is not attributable per user and one leaked VM leaks the token for all. The `proxy/` module (per-session quotas, key rotation, audit log) is built and tested but the control plane does not mint session tokens for it yet.
- **Shared password.** Login is one password plus a free-form name. Two people using the same name share ownership of each other's VMs. Fine for a personal host or a small team, not for strangers.
- **No snapshots, no warm pool by default.** Every session pays the cold boot. The warm pool exists (`warm_pool_target`) but is untested against the new guest image.
- **No persistence inside the VM.** `~/workspace` lives on the reflink copy and is deleted with the VM. Idle reaping after 5 minutes without a terminal will delete work in progress; raise `idle_timeout_seconds` or disable it if that matters.
- **Boot log on the console.** Kernel messages print on `ttyS0` and are replayed to a late-attaching browser. The menu clears the screen when it starts.
- iptables (written by the daemon) and nftables (loaded by systemd) both manage the forward hook. It works because nftables `drop` wins, but two subsystems on one chain is an operational hazard.
- The `TestWithPostgres` API test needs a reachable database and skips otherwise; the other tests run without one.

## Lineage

Derived from the VM layer of a browser-based training platform (Firecracker control plane, jailer, seccomp, nftables, serial-console bridge) and from a small host-uid launcher (password login, xterm page, resize and keepalive over WebSocket, the menu loop). The training content, its guided sidebar, the in-guest event pipeline and the per-user Unix accounts were removed; the idle reaper, the resize path through the guest agent and the client reconnect were added.

## License

MIT. See `LICENSE`.
