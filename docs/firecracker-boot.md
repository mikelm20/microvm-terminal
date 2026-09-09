# Booting a real Firecracker VM from the control plane

This is the sequence `pickLauncher` executes when the control plane starts
with `use_mock_launcher` false, and the contract the integration test in
`control-plane/tests/integration/firecracker_boot_test.go` exercises. The
binary refuses to start when the real manager fails to initialise unless the
mock is explicitly requested; the failure message points at this document.

## Prerequisites on the host

Ubuntu 24.04, kernel 6.8, x86_64. `infra/bootstrap.sh` installs everything
below idempotently. Manually:

- `/dev/kvm` present. The control plane runs as root; the jailer drops
  Firecracker to the `microvm` user, which bootstrap adds to group `kvm`.
- Firecracker `v1.8+` at `/usr/local/bin/firecracker`, jailer at
  `/usr/local/bin/jailer`. Versions pinned in `infra/bootstrap.sh`.
- `vmlinux` (uncompressed Linux kernel) at `/var/lib/firecracker/images/vmlinux`.
  Fetch via `vm-image/fetch-kernel.sh`.
- `rootfs.ext4` at `/var/lib/firecracker/images/rootfs.ext4`. Build with
  `sudo make -C vm-image rootfs` (needs Docker + losetup + root).
- Host bridge `fc-br0` with a private `/24` (defaults to `172.20.0.0/24`).
  The control plane creates it on startup if missing.
- nftables allowlist at `/etc/microvm-terminal/nftables/microvm.rules`
  active. See `infra/nftables/microvm.rules` and
  `infra/systemd/microvm-nftables.service`.
- System user `microvm` that jailer drops privileges to. Created by bootstrap.
- `/etc/microvm-terminal/claude-oauth-token` (0600) with the long-lived
  token minted by `claude setup-token`.
- `/usr/local/libexec/microvm-terminal/jailer.sh` installed from
  `infra/jailer/jailer.sh`.

## What boot looks like, stage by stage

The control plane emits structured slog JSON for every transition.

```
launcher: real firecracker               kernel=..., jailer=true
vm boot: rootfs ready                    session=<uuid> path=/var/lib/firecracker/vms/<uuid>/rootfs.ext4
vm boot: kernel start requested          session=<uuid> vcpu=2 mem_mib=2048
vm boot: vsock listener up               session=<uuid> addr=/var/lib/firecracker/vms/<uuid>/fc.vsock_5555
session created                          id=<uuid> ip=172.20.0.2 tap=tap-fc1
vm boot: guest-agent handshake complete  session=<uuid> boot_elapsed_ms=3214
session ready                            session=<uuid> boot_ms=3215
session booted                           id=<uuid> owner=<name> ip=172.20.0.2
```

The session becomes ready the moment the guest agent's hello frame carries
the session token baked into the kernel command line: a 32-byte random,
hex-encoded, passed via `mvt.session_token=...` in `boot_args` and echoed by
the guest in the first JSON frame over vsock. A mismatch drops the
connection; the session never becomes ready; the daemon times out after
`boot_timeout_seconds` (default 90) and calls `Destroy` to release the IP,
TAP and chroot.

After the handshake the same vsock connection carries host-to-guest
`resize` frames, which the agent applies to `/dev/ttyS0` with `TIOCSWINSZ`.
The last geometry a browser sent before the handshake is replayed right
after it.

## Measured timings

On the original host, with the previous guest image (same systemd, networkd,
agetty and vsock agent boot path):

| Step | Time |
|---|---|
| Rootfs ready to guest agent online | 3,214 ms |
| Session ready (logged) | 3,215 ms |
| Integration test pass window | 4 to 8 s |

## Running the boot test

On the host:

```bash
cd /path/to/microvm-terminal
sudo \
  IT_KERNEL=/var/lib/firecracker/images/vmlinux \
  IT_ROOTFS=/var/lib/firecracker/images/rootfs.ext4 \
  IT_TOKEN_FILE=/etc/microvm-terminal/claude-oauth-token \
  make integration-vm
```

`IT_USE_JAILER=0` temporarily bypasses the jailer path; use it only when
debugging a broken seccomp profile. The test then exercises
`firecracker.Launch` directly.

Expected output: `PASS` within roughly 4 to 8 seconds. Tear-down removes the
chroot at `/srv/jailer/firecracker/<vm-id>/`, the `tap-fc*` device, and the
per-VM scratch in `/var/lib/firecracker/vms/<vm-id>/`.

On macOS the test body calls `t.Skipf` with "requires linux/kvm". `go build`
and the rest of the unit test suite still pass, so the code is kept honest
by the compile step. Full end-to-end verification has to happen on Linux.

## Troubleshooting

- `vm did not become ready within boot timeout`: the most common cause is
  the guest agent not being present on the rootfs. Rebuild with
  `sudo make -C vm-image rootfs`; confirm `/usr/local/bin/microvm-guest-agent`
  and the `microvm-guest-agent.service` symlink are in the image.
- `vsock hello rejected token_match=false`: the kernel cmdline token did not
  reach the guest. Check `/proc/cmdline` inside the VM via the serial console
  (attach a browser; the boot log is replayed). Newline stripping in the
  token file is a frequent source of drift.
- `init firecracker manager: detect upstream`: no default route. Happens on
  macOS (intentional) and on hosts that egress via a non-`ip route`
  interface.
- `bridge addr: ... File exists`: another control-plane instance or a stale
  bootstrap run still holds the bridge. `ip link del fc-br0` and restart.
- Resize has no effect: `journalctl -u microvm-guest-agent` inside the VM
  should show `resize applied: <cols>x<rows>`. If it shows nothing, the
  browser never sent a text frame; if it shows an open error, `/dev/ttyS0`
  is not the console device in this kernel.
