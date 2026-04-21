# syntax=docker/dockerfile:1.7
#
# VM rootfs for learn-platform Firecracker guests.
# Built on x86_64. Exported via `docker export` into an ext4 image by build-rootfs.sh.
# The running kernel is provided by Firecracker (vmlinux from fetch-kernel.sh), NOT from apt.

FROM ubuntu:24.04

ARG DEBIAN_FRONTEND=noninteractive

# Base userland + init + networking
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl wget gnupg \
        git vim tmux htop less \
        python3 python3-pip \
        sudo openssh-client \
        init systemd systemd-sysv udev dbus \
        iproute2 iputils-ping net-tools \
        build-essential && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Node 22 LTS via NodeSource (matches host)
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && \
    apt-get install -y --no-install-recommends nodejs && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Claude Code
RUN npm install -g @anthropic-ai/claude-code && \
    npm cache clean --force

# Confined learner user. NO sudo (lessons run inside Claude Code, the VM is
# ephemeral, and we don't want a path to root from a slip in the lesson copy).
# Login shell is /usr/local/bin/learn-shell, which loops Claude Code so the
# user can never reach a bash prompt from inside.
COPY learn-shell.sh /usr/local/bin/learn-shell
RUN chmod 0755 /usr/local/bin/learn-shell && \
    useradd -m -s /usr/local/bin/learn-shell learner

# Pre-bake Claude Code config so the TUI doesn't block on first-run prompts:
#   - hasCompletedOnboarding skips the welcome/login flow.
#   - permissions.allow="*" plus --dangerously-skip-permissions in the
#     wrapper makes tool use never prompt mid-lesson.
#   - skipDangerousModePermissionPrompt silences the one-time "are you sure
#     you want to bypass permissions?" warning that --dangerously-skip-
#     permissions otherwise shows on first launch.
#   - .claude/.claude.json: projects entries pre-trust the lesson working
#     directories so the "Do you trust the files in this folder?" dialog
#     does not appear. (Note: GitHub issue #9113 reports this is sometimes
#     ignored on v2.0.9+; defense-in-depth via multiple keys.)
RUN install -d -o learner -g learner -m 0700 /home/learner/.claude && \
    printf '%s\n' \
      '{' \
      '  "hasCompletedOnboarding": true,' \
      '  "permissions": {' \
      '    "allow": ["*"]' \
      '  },' \
      '  "hasTrustDialogAccepted": true,' \
      '  "skipDangerousModePermissionPrompt": true' \
      '}' > /home/learner/.claude/settings.json && \
    printf '%s\n' \
      '{' \
      '  "projects": {' \
      '    "/home/learner": { "allowedTools": [], "hasTrustDialogAccepted": true },' \
      '    "/home/learner/empresa-prueba": { "allowedTools": [], "hasTrustDialogAccepted": true },' \
      '    "/home/learner/empresa-prueba/mi-trabajo": { "allowedTools": [], "hasTrustDialogAccepted": true }' \
      '  }' \
      '}' > /home/learner/.claude/.claude.json && \
    chown learner:learner /home/learner/.claude/settings.json /home/learner/.claude/.claude.json && \
    chmod 0640 /home/learner/.claude/settings.json /home/learner/.claude/.claude.json

# Base tutor CLAUDE.md auto-loaded by Claude Code at session start.
# Per-lesson priming is layered on top via claude-wrap --append-system-prompt.
COPY --chown=learner:learner learn-claude.md /home/learner/.claude/CLAUDE.md
RUN chmod 0640 /home/learner/.claude/CLAUDE.md

# Empresa Prueba sandbox: fictional company every learner shares.
# Lessons in lessons/m{2..8}-*.yml assume this layout exists at
# /home/learner/empresa-prueba on first boot.
COPY --chown=learner:learner sandbox/empresa-prueba /home/learner/empresa-prueba

# Autologin on the serial console so the host's PTY reader lands in a shell.
# Phase 0 only: later replaced by guest-agent wrapping the PTY.
RUN mkdir -p /etc/systemd/system/serial-getty@ttyS0.service.d && \
    printf '[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin learner --noclear %%I 115200,38400,9600 vt102\n' \
      > /etc/systemd/system/serial-getty@ttyS0.service.d/override.conf && \
    systemctl enable serial-getty@ttyS0.service && \
    # Mask the other getty services: /dev/console == /dev/ttyS0 in Firecracker,
    # /dev/tty1 doesn't exist. Having multiple gettys race on the single serial
    # port prevents autologin from firing cleanly.
    systemctl mask console-getty.service getty@tty1.service || true

# learn-guest-agent: emits events over vsock to the host control plane.
# Binary is placed in the build context by vm-image/Makefile (`make guest-agent`).
COPY learn-guest-agent /usr/local/bin/learn-guest-agent
RUN chmod 0755 /usr/local/bin/learn-guest-agent
COPY learn-guest-agent.service /etc/systemd/system/learn-guest-agent.service
RUN systemctl enable learn-guest-agent.service

# claude-wrap: runs inside the VM in place of `claude`, parses stream-json
# output into canonical wizard events and publishes them over a unix socket
# the guest-agent tails. Binary staged into the build context as
# `claude-wrap-bin` by vm-image/Makefile (`make claude-wrap-bin`).
COPY claude-wrap-bin /usr/local/bin/claude-wrap
RUN chmod 0755 /usr/local/bin/claude-wrap && \
    install -d -m 0755 /run/learn

# Make sure systemd mounts root rw (Firecracker mounts it ro at first)
RUN printf '/dev/root / ext4 rw,noatime 0 1\n' > /etc/fstab

# Hostname is set by the host on boot (via kernel cmdline); placeholder here
RUN echo 'learn-vm' > /etc/hostname

# Phase 0 static networking. systemd-networkd handles eth0. DNS is a static
# /etc/resolv.conf written by build-rootfs.sh after export (Docker bind-mounts
# /etc/resolv.conf at build time, so we can't edit it here). When the control
# plane lands, the .network file will be replaced per-VM with the allocated IP
# (overlay file or vsock-delivered drop-in).
RUN mkdir -p /etc/systemd/network && \
    printf '[Match]\nName=eth0\n\n[Network]\nAddress=172.20.0.2/24\nGateway=172.20.0.1\nDNS=1.1.1.1\nDNS=1.0.0.1\n' \
      > /etc/systemd/network/10-eth0.network && \
    systemctl enable systemd-networkd

# Phase 0 debug: mirror journald to the serial console so any boot-time failure
# reasons end up in the spike.log. Remove or set ForwardToConsole=no once things
# are stable.
RUN mkdir -p /etc/systemd/journald.conf.d && \
    printf '[Journal]\nForwardToConsole=yes\nMaxLevelConsole=info\nStorage=volatile\n' \
      > /etc/systemd/journald.conf.d/10-phase0-debug.conf

# Phase 0 debug: dump network state right after multi-user.target.
COPY phase0-diag.sh /usr/local/sbin/phase0-diag.sh
RUN chmod 0755 /usr/local/sbin/phase0-diag.sh && \
    printf '[Unit]\nDescription=Phase 0 diagnostics\nAfter=multi-user.target\nDefaultDependencies=no\n\n[Service]\nType=oneshot\nStandardOutput=journal+console\nStandardError=journal+console\nExecStart=/usr/local/sbin/phase0-diag.sh\nRemainAfterExit=yes\n\n[Install]\nWantedBy=multi-user.target\n' \
      > /etc/systemd/system/phase0-diag.service && \
    systemctl enable phase0-diag.service

# Shrink: drop caches that won't survive the rootfs conversion anyway
RUN rm -rf /tmp/* /var/tmp/* /root/.npm /root/.cache /var/cache/apt/archives/*.deb

# When run as a container, launch systemd. build-rootfs.sh copies the fs out,
# so this only matters if someone docker-runs the image directly.
CMD ["/sbin/init"]
