# vm-image

Reproducible Firecracker guest image: Ubuntu 24.04 + Node 22 + Claude Code + the guest agent.

## Build

On the Linux x86_64 host (Docker, root):

```
cd vm-image
sudo make all
```

Artifacts land in `$OUT_DIR` (default `/var/lib/firecracker/images`):

- `rootfs.ext4`: 4 GiB ext4 image, the VM's `/`
- `vmlinux-<version>` and `vmlinux` (symlink to the pinned version)

Bump versions in the `Makefile` or at invocation: `sudo KERNEL_VERSION=6.1.160 make kernel`.

## What the rootfs contains

- Ubuntu 24.04 userland, systemd as PID 1
- `agetty --autologin dev` on `ttyS0`, so the serial console lands in the menu
- `dev` user (uid 1001), login shell `guest-shell` (`guest-shell.sh`), no sudo except `poweroff`
- Node 22 and `@anthropic-ai/claude-code`
- `microvm-guest-agent`: vsock hello for readiness, `TIOCSWINSZ` on `/dev/ttyS0` for resize
- git, vim, tmux, python3, build-essential

## Not in the rootfs

- The kernel (Firecracker supplies it)
- The Claude OAuth token, the per-VM hostname and IP (written by the control plane at rootfs prep, see `control-plane/internal/firecracker/rootfs.go`)
- An SSH server (the VM is reached only through the serial console and vsock)
