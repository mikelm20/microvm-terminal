# syntax=docker/dockerfile:1.7
#
# Guest rootfs for microvm-terminal Firecracker VMs.
# Built on x86_64. Exported via `docker export` into an ext4 image by build-rootfs.sh.
# The kernel is provided by Firecracker (vmlinux from fetch-kernel.sh), NOT from apt.

FROM ubuntu:24.04

ARG DEBIAN_FRONTEND=noninteractive

# Base userland + init + networking + the usual dev tools.
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl wget gnupg \
        git vim tmux htop less jq \
        python3 python3-pip python3-venv \
        sudo openssh-client \
        init systemd systemd-sysv udev dbus \
        iproute2 iputils-ping net-tools \
        build-essential && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Node 22 LTS via NodeSource.
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Claude Code.
RUN npm install -g @anthropic-ai/claude-code && \
    npm cache clean --force

# The `dev` user (uid 1001; control-plane/internal/firecracker/rootfs.go
# hardcodes it). No general sudo: the only privileged command is poweroff,
# which the menu uses for "exit". Login shell is guest-shell, which loops the
# menu so the user never reaches a bash prompt from the console.
COPY guest-shell.sh /usr/local/bin/guest-shell
RUN chmod 0755 /usr/local/bin/guest-shell && \
    useradd -m -u 1001 -s /usr/local/bin/guest-shell dev && \
    printf 'dev ALL=(root) NOPASSWD: /sbin/poweroff\n' > /etc/sudoers.d/dev-poweroff && \
    chmod 0440 /etc/sudoers.d/dev-poweroff

# Pre-bake Claude Code config so the TUI does not block on first-run prompts:
#   - permissions.allow="*" plus --dangerously-skip-permissions in the menu
#     means tool use never prompts.
#   - skipDangerousModePermissionPrompt silences the one-time "bypass
#     permissions?" warning.
#   - .claude.json trusts the workspace so the folder-trust dialog is skipped.
# Credentials and the onboarding marker are written per VM at rootfs prep.
RUN install -d -o dev -g dev -m 0700 /home/dev/.claude /home/dev/workspace && \
    printf '%s\n' \
      '{' \
      '  "permissions": { "allow": ["*"] },' \
      '  "hasTrustDialogAccepted": true,' \
      '  "skipDangerousModePermissionPrompt": true' \
      '}' > /home/dev/.claude/settings.json && \
    printf '%s\n' \
      '{' \
      '  "projects": {' \
      '    "/home/dev": { "allowedTools": [], "hasTrustDialogAccepted": true },' \
      '    "/home/dev/workspace": { "allowedTools": [], "hasTrustDialogAccepted": true }' \
      '  }' \
      '}' > /home/dev/.claude/.claude.json && \
    chown dev:dev /home/dev/.claude/settings.json /home/dev/.claude/.claude.json && \
    chmod 0640 /home/dev/.claude/settings.json /home/dev/.claude/.claude.json

# Autologin on the serial console: the host's console bridge lands straight
# in guest-shell. Other gettys are masked because /dev/console is ttyS0 in
# Firecracker and /dev/tty1 does not exist; two gettys racing on one port
# breaks autologin.
RUN mkdir -p /etc/systemd/system/serial-getty@ttyS0.service.d && \
    printf '[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin dev --noclear %%I 115200,38400,9600 vt102\n' \
      > /etc/systemd/system/serial-getty@ttyS0.service.d/override.conf && \
    systemctl enable serial-getty@ttyS0.service && \
    systemctl mask console-getty.service getty@tty1.service || true

# Guest agent: vsock handshake + terminal resize. The binary is staged into
# the build context by vm-image/Makefile (`make guest-agent`).
COPY microvm-guest-agent /usr/local/bin/microvm-guest-agent
RUN chmod 0755 /usr/local/bin/microvm-guest-agent
COPY microvm-guest-agent.service /etc/systemd/system/microvm-guest-agent.service
RUN systemctl enable microvm-guest-agent.service

# Make sure systemd mounts root rw (Firecracker mounts it ro at first).
RUN printf '/dev/root / ext4 rw,noatime 0 1\n' > /etc/fstab

# Hostname is rewritten per VM at rootfs prep; placeholder here.
RUN echo 'microvm' > /etc/hostname

# Static networking. The .network file is replaced per VM with the allocated
# IP at rootfs prep. DNS is a static /etc/resolv.conf written by
# build-rootfs.sh after export (Docker bind-mounts it at build time).
RUN mkdir -p /etc/systemd/network && \
    printf '[Match]\nName=eth0\n\n[Network]\nAddress=172.20.0.2/24\nGateway=172.20.0.1\nDNS=1.1.1.1\nDNS=1.0.0.1\n' \
      > /etc/systemd/network/10-eth0.network && \
    systemctl enable systemd-networkd

# Keep the journal off the serial console: the console is the user's
# terminal. Kernel messages still print during boot (console=ttyS0) and are
# replayed to a late-attaching browser, which is useful when a boot fails.
RUN mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nForwardToConsole=no\nStorage=volatile\n' \
      > /etc/systemd/journald.conf.d/10-console.conf

# Shrink: drop caches that will not survive the rootfs conversion anyway.
RUN rm -rf /tmp/* /var/tmp/* /root/.npm /root/.cache /var/cache/apt/archives/*.deb

CMD ["/sbin/init"]
