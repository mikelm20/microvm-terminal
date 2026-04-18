# vm-image

Reproducible Firecracker guest image: Ubuntu 24.04 + Claude Code + Node.

## Build

On the learn-01 host (x86_64 Linux with Docker):

```
cd ~/learn-platform/vm-image
sudo make all
```

Artifacts land in `$OUT_DIR` (default `/var/lib/firecracker/images`):
- `rootfs.ext4` - 4 GiB ext4 filesystem image, the VM's `/`
- `vmlinux-<version>` and `vmlinux` (symlink to the current pin)

## Version bumps

Edit the `Makefile` or override at invocation:
```
sudo KERNEL_VERSION=6.1.160 make kernel
```

## What the rootfs contains

- Ubuntu 24.04 userland (apt upgrade -y at build time)
- systemd as PID 1 (serial getty auto-logs in as `learner`)
- Node 22 LTS via NodeSource
- Claude Code (`@anthropic-ai/claude-code` npm global)
- `learner` user with passwordless sudo (MVP sandbox only)
- Standard dev tools: git, vim, tmux, htop, python3, build-essential

## Not in the rootfs

- No kernel (Firecracker supplies it)
- No Claude OAuth token (injected at boot by the control plane or guest agent)
- No SSH server (you talk to the VM via the Firecracker serial console / vsock, not via network SSH)

## Phase 0 caveat

Serial auto-login as `learner` is a MVP convenience so the host PTY reader ends up in a shell immediately. When the guest agent lands (Phase 2-3), the serial getty will be disabled and PTY access will go through the agent, not auto-login.
