# Booting a real Firecracker VM from the control plane

Agent-Gate notes. This is the sequence `pickLauncher` executes when the
control plane starts with `USE_MOCK_LAUNCHER` unset, and the contract the
integration test in `control-plane/tests/integration/firecracker_boot_test.go`
exercises. Until 2026-04-21 the production host `learn-01` was logging
`using mock launcher` because `session.NewManager` failed to initialise and
the old `pickLauncher` silently fell back to the mock. The current binary
refuses to start when the real manager fails unless mock is explicitly
requested; the failure message points at this document.

## Prerequisites on a Linux dev box or on `learn-01`

Ubuntu 24.04, kernel 6.8, x86_64. The host script `infra/bootstrap.sh`
installs everything below idempotently. Manually:

- `/dev/kvm` present and writable by the user that runs the control plane.
  On the learn user: `sudo setfacl -m u:learn:rw /dev/kvm` or add `learn`
  to group `kvm`.
- Firecracker `v1.8+` at `/usr/local/bin/firecracker`, jailer at
  `/usr/local/bin/jailer`. Versions pinned in `infra/bootstrap.sh`.
- `vmlinux` (uncompressed Linux kernel) at `/var/lib/firecracker/images/vmlinux`.
  Fetch via `vm-image/fetch-kernel.sh`.
- `rootfs.ext4` at `/var/lib/firecracker/images/rootfs.ext4`. Build with
  `sudo vm-image/build-rootfs.sh` (needs Docker + losetup + root).
- Host bridge `fc-br0` with a private `/24` (defaults to `172.20.0.0/24`).
  The control plane creates it on startup if missing.
- nftables allow-list at `/etc/nftables.d/learn.rules` active. See
  `infra/nftables/learn.rules` and `infra/systemd/learn-nftables.service`.
- System user `learn` (uid/gid created by bootstrap) whose jailer drops privileges
  to. Created by bootstrap.
- `/etc/learn-platform/claude-oauth-token` (0600, owner `learn`) with the
  long-lived token minted by `claude setup-token`.
- `/usr/local/libexec/learn-platform/jailer.sh` installed from
  `infra/jailer/jailer.sh`.

## What boot looks like, stage by stage

The control plane emits structured slog JSON for every transition and
republishes the corresponding `vm_booting` stage event on the session's
event bus so the wizard WS sees the same timeline.

```
launcher: real firecracker               kernel=..., jailer=true
session created                          id=<uuid> ip=172.20.0.2 tap=tap-fc1
vm boot: rootfs ready                    session=<uuid> path=/var/lib/firecracker/vms/<uuid>/rootfs.ext4
vm boot: kernel start requested          session=<uuid> vcpu=2 mem_mib=2048
vm boot: vsock listener up               session=<uuid> addr=/var/lib/firecracker/vms/<uuid>/fc.vsock_5555
vm boot: guest-agent handshake complete  session=<uuid> boot_elapsed_ms=3214
session ready                            session=<uuid> boot_ms=3215
```

Stages in the `GuestEvent` bus, matching `CONTRACTS.md` section 5:

| stage          | meaning                                                              |
|----------------|----------------------------------------------------------------------|
| jailer         | jailer script invoked (implicit; no event fires for pass-through)    |
| kernel         | firecracker child spawned, kernel booting (implicit)                 |
| rootfs         | per-VM ext4 ready with systemd-networkd + claude creds               |
| guest_agent    | host-side vsock listener bound, waiting for guest hello              |
| claude         | claude binary is available inside the VM; covered by the guest agent via process_started |

The explicit session-wide `vm_ready` event fires the moment the guest-agent
hello frame's `session_token` matches the value baked into the kernel
cmdline. That is the handshake: a 32-byte random, hex-encoded, passed via
`learn.session_token=...` in `boot_args` and echoed by the guest in the
first JSON frame over vsock. A mismatch drops the connection; the session
never becomes ready; the daemon times out after `BOOT_TIMEOUT_SECONDS`
(default 90) and calls `Destroy` to release the IP, TAP, and chroot.

## Running the boot test

### On `learn-01`

```bash
ssh -i <ssh-key-path> <user>@<host-ip>
cd /var/lib/learn-platform/repo
sudo -u root \
  LEARN_IT_KERNEL=/var/lib/firecracker/images/vmlinux \
  LEARN_IT_ROOTFS=/var/lib/firecracker/images/rootfs.ext4 \
  LEARN_IT_TOKEN_FILE=/etc/learn-platform/claude-oauth-token \
  make integration-vm
```

Passing the `LEARN_IT_USE_JAILER=0` env var temporarily bypasses the jailer
path; use it only when debugging a broken seccomp profile. The test then
exercises `firecracker.Launch` directly.

Expected output: `PASS` within roughly 4 to 8 seconds. Tear-down removes
the chroot at `/srv/jailer/firecracker/<vm-id>/`, the `tap-fc*` device, and
the per-VM scratch in `/var/lib/firecracker/vms/<vm-id>/`.

### On a fresh Linux dev box

Provision identically via `infra/bootstrap.sh`, then run the same commands.
Firecracker needs `/dev/kvm` and root-ish privileges (TAP creation and
loop-mounting the rootfs). Running the test as the `learn` user with
`setcap cap_net_admin,cap_sys_admin+ep` on the test binary is possible but
brittle; root is simpler.

### On macOS (this worktree's env)

The test body calls `t.Skipf` with "requires linux/kvm". `go build` and the
rest of the unit test suite still pass, so the code is kept honest by the
compile step. Full end-to-end verification has to happen on Linux.

## Wiring recap (what changed 2026-04-21)

- `session.Manager.Create` now routes through `vm.LaunchJailed` when
  `cfg.UseJailer` is true (default). The legacy `firecracker.Launch` path
  remains reachable for bring-up debugging via `USE_JAILER=0`.
- `pickLauncher` no longer silently downgrades to the mock when the
  Firecracker manager fails to initialise; it returns the error and the
  daemon aborts. Set `USE_MOCK_LAUNCHER=true` to opt into the mock.
- `Session` now exposes `MarkReady`, `WaitReady`, and `ReadyAt`. The vsock
  listener calls `MarkReady` after validating the hello frame; the real
  launcher in `pickLauncher` waits on that signal before returning.
- `config.Config` gained `UseJailer`, `JailerScript`, `JailerChrootBase`,
  `JailerCPUQuotaMicros`, `JailerMemBytes`, `JailerSeccompProfile`,
  `JailerUID`, `JailerGID`, `BootTimeoutSeconds`. All overridable via
  environment so Doppler injection keeps working.

## Troubleshooting

- `vm did not become ready within boot timeout`: the most common cause is
  the guest-agent not being present on the rootfs. Rebuild with
  `sudo vm-image/build-rootfs.sh`; confirm `/usr/local/bin/learn-guest-agent`
  and the `learn-guest-agent.service` symlink are in the image.
- `vsock hello rejected token_match=false`: the kernel cmdline token did not
  reach the guest. Check `/proc/cmdline` inside the VM via the serial
  console (attached as the PTY tee in `Session.Serial`). Newline stripping
  in the token file is a frequent source of drift.
- `init firecracker manager: detect upstream`: no default route. Happens on
  macOS (intentional) and on hosts that egress via a non-`ip route`
  interface. Set `IP_UPSTREAM` or run on `learn-01`.
- `bridge addr: ... File exists`: another control-plane instance or a stale
  `bootstrap.sh` run still holds the bridge. `ip link del fc-br0` and
  restart.
