#!/usr/bin/env bash
#
# bootstrap.sh - idempotent host setup for learn-01 (learn.example.com).
# Safe to re-run; each step detects prior state and skips when possible.
#
#   Usage: sudo bash bootstrap.sh
#
# Bump pinned versions in the block below and re-run to upgrade a component.
# Anything this script installs should be installed ONLY via this script.
# Do not apt-install, curl|sh, or npm-install-globally outside of here.

set -euo pipefail

### --- Pinned versions (intentional upgrades only) ------------------------
FIRECRACKER_VERSION="${FIRECRACKER_VERSION:-v1.10.1}"
GO_VERSION="${GO_VERSION:-1.25.0}"
NODE_MAJOR="${NODE_MAJOR:-22}"

### --- Host identity ------------------------------------------------------
HOSTNAME_TARGET="${HOSTNAME_TARGET:-learn-01}"
FQDN_TARGET="${FQDN_TARGET:-learn-01.example.com}"

### --- System layout ------------------------------------------------------
LEARN_USER="${LEARN_USER:-learn}"
LEARN_HOME="${LEARN_HOME:-/var/lib/learn-platform}"
LEARN_ETC="${LEARN_ETC:-/etc/learn-platform}"
LEARN_LIBEXEC="${LEARN_LIBEXEC:-/usr/local/libexec/learn-platform}"
FC_DATA_DIR="${FC_DATA_DIR:-/var/lib/firecracker}"
FC_DATA_DEV="${FC_DATA_DEV:-/dev/nvme0n1}"
JAILER_CHROOT_BASE="${JAILER_CHROOT_BASE:-/srv/jailer}"

### --- Source layout (this repo) ------------------------------------------
# Where bootstrap.sh expects to find the infra payload when it runs. If you
# clone elsewhere, set LEARN_REPO_DIR.
LEARN_REPO_DIR="${LEARN_REPO_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

### --- Helpers ------------------------------------------------------------
log()     { printf '\n\033[1;34m[bootstrap]\033[0m %s\n' "$*"; }
require_root() { [ "${EUID:-$(id -u)}" -eq 0 ] || { echo "Must run as root (use sudo)"; exit 1; }; }
apt_install() { DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$@"; }

### --- Steps --------------------------------------------------------------

check_os() {
  . /etc/os-release
  if [ "${ID:-}" != "ubuntu" ] || [[ "${VERSION_ID:-}" != 24.04* ]]; then
    echo "Expected Ubuntu 24.04; got ${PRETTY_NAME:-unknown}"
    exit 1
  fi
  log "OS: ${PRETTY_NAME}"
}

check_virt() {
  # Firecracker requires /dev/kvm and AMD-V or VT-x exposed
  [ -c /dev/kvm ] || { echo "FATAL: /dev/kvm missing (nested virt not available)"; exit 1; }
  grep -Eq 'vmx|svm' /proc/cpuinfo || { echo "FATAL: no vmx/svm CPU flag"; exit 1; }
  log "Virt: /dev/kvm present, $(grep -Eo 'vmx|svm' /proc/cpuinfo | sort -u | tr '\n' ' ')exposed"
}

set_hostname() {
  log "Hostname: ${HOSTNAME_TARGET} (${FQDN_TARGET})"
  hostnamectl set-hostname "${HOSTNAME_TARGET}"
  # /etc/hosts must have a 127.0.1.1 line with the new name so sudo stops warning.
  sed -i '/^127\.0\.1\.1[[:space:]]/d' /etc/hosts
  printf '127.0.1.1\t%s %s\n' "${FQDN_TARGET}" "${HOSTNAME_TARGET}" >> /etc/hosts
}

apt_refresh() {
  log "apt update + upgrade"
  apt-get update -y
  DEBIAN_FRONTEND=noninteractive apt-get upgrade -y
  apt_install unattended-upgrades apt-listchanges
  # Enable daily unattended security upgrades
  cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOF
}

base_packages() {
  log "Base packages"
  apt_install \
    curl wget jq git ca-certificates gnupg lsb-release \
    vim tmux htop tree \
    ufw nftables bridge-utils \
    build-essential pkg-config \
    sqlite3
  # ufw manages host inbound; VM-bridge egress rules will be managed via nftables
  # directly (separate config file loaded by nftables.service). iptables-persistent
  # is avoided because it conflicts with ufw on Ubuntu 24.04.
}

configure_ufw() {
  log "ufw: default deny incoming, allow 22/80/443"
  # Reset only if not already our shape (first-run detection)
  if ! ufw status | grep -q '22/tcp.*ALLOW'; then
    ufw --force reset
    ufw default deny incoming
    ufw default allow outgoing
    ufw allow 22/tcp comment 'ssh'
    ufw allow 80/tcp comment 'caddy http challenge'
    ufw allow 443/tcp comment 'caddy tls'
    ufw --force enable
  fi
}

mount_fc_data() {
  log "Firecracker data disk: ${FC_DATA_DEV} -> ${FC_DATA_DIR}"
  install -d -m 0755 "${FC_DATA_DIR}"
  if mountpoint -q "${FC_DATA_DIR}"; then
    return 0
  fi
  local fstype
  fstype="$(blkid -o value -s TYPE "${FC_DATA_DEV}" || true)"
  if [ -z "${fstype}" ]; then
    log "  formatting ${FC_DATA_DEV} as ext4"
    mkfs.ext4 -F -L firecracker "${FC_DATA_DEV}"
  elif [ "${fstype}" != "ext4" ]; then
    echo "FATAL: ${FC_DATA_DEV} already has filesystem '${fstype}'. Manual check required."
    exit 1
  fi
  local uuid; uuid="$(blkid -o value -s UUID "${FC_DATA_DEV}")"
  if ! grep -q "${uuid}" /etc/fstab; then
    printf 'UUID=%s\t%s\text4\tdefaults,noatime,nofail\t0\t2\n' "${uuid}" "${FC_DATA_DIR}" >> /etc/fstab
  fi
  mount "${FC_DATA_DIR}"
}

install_firecracker() {
  local bin=/usr/local/bin/firecracker
  local jailer=/usr/local/bin/jailer
  if [ -x "${bin}" ] && "${bin}" --version 2>/dev/null | grep -q "${FIRECRACKER_VERSION#v}"; then
    log "Firecracker ${FIRECRACKER_VERSION} already installed"
    return 0
  fi
  log "Installing Firecracker ${FIRECRACKER_VERSION}"
  local tmp; tmp="$(mktemp -d)"
  local arch=x86_64
  local tgz="firecracker-${FIRECRACKER_VERSION}-${arch}.tgz"
  local url="https://github.com/firecracker-microvm/firecracker/releases/download/${FIRECRACKER_VERSION}/${tgz}"
  curl -fsSL "${url}" -o "${tmp}/fc.tgz"
  tar -xzf "${tmp}/fc.tgz" -C "${tmp}"
  local rel_dir; rel_dir="$(find "${tmp}" -maxdepth 1 -type d -name 'release-*' | head -1)"
  install -m 0755 "${rel_dir}/firecracker-${FIRECRACKER_VERSION}-${arch}" "${bin}"
  install -m 0755 "${rel_dir}/jailer-${FIRECRACKER_VERSION}-${arch}"      "${jailer}"
  rm -rf "${tmp}"
}

install_go() {
  local current=""
  [ -x /usr/local/go/bin/go ] && current="$(/usr/local/go/bin/go version 2>/dev/null | awk '{print $3}')"
  if [ "${current}" = "go${GO_VERSION}" ]; then
    log "Go ${GO_VERSION} already installed"
    return 0
  fi
  log "Installing Go ${GO_VERSION}"
  local tmp; tmp="$(mktemp -d)"
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o "${tmp}/go.tgz"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "${tmp}/go.tgz"
  rm -rf "${tmp}"
  cat > /etc/profile.d/go.sh <<'EOF'
export PATH=$PATH:/usr/local/go/bin
EOF
  chmod 0644 /etc/profile.d/go.sh
}

install_node() {
  if command -v node >/dev/null && node --version | grep -q "^v${NODE_MAJOR}\."; then
    log "Node ${NODE_MAJOR} already installed"
    return 0
  fi
  log "Installing Node ${NODE_MAJOR} LTS via NodeSource"
  curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
  apt_install nodejs
}

install_caddy() {
  if command -v caddy >/dev/null; then
    log "Caddy already installed"
    return 0
  fi
  log "Installing Caddy (official apt repo)"
  apt_install debian-keyring debian-archive-keyring apt-transport-https
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
    > /etc/apt/sources.list.d/caddy-stable.list
  apt-get update -y
  apt_install caddy
}

install_docker() {
  if command -v docker >/dev/null; then
    log "Docker already installed"
    return 0
  fi
  log "Installing Docker (official apt repo)"
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  . /etc/os-release
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    > /etc/apt/sources.list.d/docker.list
  apt-get update -y
  apt_install docker-ce docker-ce-cli containerd.io docker-buildx-plugin
  systemctl enable --now docker
  # Let the default human user run docker without sudo; still use sudo for root-needing ops
  usermod -aG docker ubuntu 2>/dev/null || true
}

create_learn_user() {
  if id "${LEARN_USER}" >/dev/null 2>&1; then
    log "User ${LEARN_USER} exists"
  else
    log "Creating system user ${LEARN_USER}"
    useradd --system --home-dir "${LEARN_HOME}" --create-home --shell /usr/sbin/nologin "${LEARN_USER}"
  fi
  install -d -o "${LEARN_USER}" -g "${LEARN_USER}" -m 0755 "${LEARN_HOME}"
  install -d -o root              -g "${LEARN_USER}" -m 0750 "${LEARN_ETC}"
  install -d -o "${LEARN_USER}" -g "${LEARN_USER}" -m 0755 "${FC_DATA_DIR}"
  # Access to /dev/kvm for Firecracker
  usermod -aG kvm "${LEARN_USER}" 2>/dev/null || true
}

install_doppler() {
  if command -v doppler >/dev/null; then
    log "Doppler CLI already installed"
    return 0
  fi
  log "Installing Doppler CLI (official apt repo)"
  # Source: https://docs.doppler.com/docs/install-cli
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL --retry 3 'https://packages.doppler.com/public/cli/gpg.DE2A7741A397C129.key' \
    | gpg --dearmor -o /etc/apt/keyrings/doppler.gpg
  chmod a+r /etc/apt/keyrings/doppler.gpg
  echo "deb [signed-by=/etc/apt/keyrings/doppler.gpg] https://packages.doppler.com/public/cli/deb/debian any-version main" \
    > /etc/apt/sources.list.d/doppler-cli.list
  apt-get update -y
  apt_install doppler
}

install_gate_scripts() {
  log "Installing jailer.sh + refresh-sets.sh under ${LEARN_LIBEXEC}"
  install -d -m 0755 "${LEARN_LIBEXEC}"
  install -m 0755 "${LEARN_REPO_DIR}/infra/jailer/jailer.sh" \
    "${LEARN_LIBEXEC}/jailer.sh"
  install -m 0755 "${LEARN_REPO_DIR}/infra/nftables/refresh-sets.sh" \
    "${LEARN_LIBEXEC}/refresh-sets.sh"

  install -d -m 0755 "${LEARN_ETC}/jailer"
  install -m 0644 "${LEARN_REPO_DIR}/infra/jailer/profile.d/learn-seccomp.json" \
    "${LEARN_ETC}/jailer/learn-seccomp.json"

  install -d -m 0755 "${LEARN_ETC}/nftables"
  install -m 0644 "${LEARN_REPO_DIR}/infra/nftables/learn.rules" \
    "${LEARN_ETC}/nftables/learn.rules"

  install -d -m 0755 "${JAILER_CHROOT_BASE}"
}

install_systemd_units() {
  log "Installing systemd units for the gate"
  install -m 0644 "${LEARN_REPO_DIR}/infra/systemd/learn-nftables.service" \
    /etc/systemd/system/learn-nftables.service
  install -m 0644 "${LEARN_REPO_DIR}/infra/systemd/learn-nftables-refresh.service" \
    /etc/systemd/system/learn-nftables-refresh.service
  install -m 0644 "${LEARN_REPO_DIR}/infra/systemd/learn-nftables-refresh.timer" \
    /etc/systemd/system/learn-nftables-refresh.timer
  install -m 0644 "${LEARN_REPO_DIR}/infra/systemd/claude-proxy.service" \
    /etc/systemd/system/claude-proxy.service
  install -m 0644 "${LEARN_REPO_DIR}/infra/systemd/firecracker-jailer@.service" \
    /etc/systemd/system/firecracker-jailer@.service

  systemctl daemon-reload
  systemctl enable --now learn-nftables.service
  systemctl enable --now learn-nftables-refresh.timer
  # claude-proxy requires Doppler to be configured with a service token; we
  # enable the unit but do not start. Operator enables after
  #   `sudo -u learn doppler configure set token <prd-token> --scope /var/lib/learn-platform`
  systemctl enable claude-proxy.service
  log "enabled claude-proxy.service (not started; needs Doppler service token)"
}

run_migrations() {
  # Postgres migrations live in control-plane/internal/db/migrations, owned by
  # Agent-API. For now, the proxy creates its own schema on boot (idempotent)
  # so a bootstrap on a fresh box does not need goose until Agent-API merges.
  if command -v goose >/dev/null; then
    log "goose present; running control-plane migrations"
    : # placeholder: control plane sets DATABASE_URL via doppler run; we don't
      # have it here. Leave this to the control-plane systemd unit's
      # ExecStartPre once Agent-API wires it.
  else
    log "goose not installed; skipping migrations (proxy self-migrates its table)"
  fi
}

harden_ssh() {
  log "Hardening sshd"
  local cfg=/etc/ssh/sshd_config.d/99-learn-platform.conf
  cat > "${cfg}" <<'EOF'
# Managed by learn-platform bootstrap.sh
PasswordAuthentication no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
PubkeyAuthentication yes
X11Forwarding no
AllowTcpForwarding no
ClientAliveInterval 300
ClientAliveCountMax 2
MaxAuthTries 3
LoginGraceTime 30
EOF
  chmod 0644 "${cfg}"
  sshd -t
  systemctl reload ssh 2>/dev/null || systemctl reload sshd
}

reboot_hint() {
  if [ -f /var/run/reboot-required ]; then
    log "Kernel/system updated - a reboot is pending. Run: sudo reboot"
  fi
}

main() {
  require_root
  check_os
  check_virt
  set_hostname
  apt_refresh
  base_packages
  configure_ufw
  mount_fc_data
  install_firecracker
  install_go
  install_node
  install_caddy
  install_docker
  install_doppler
  create_learn_user
  install_gate_scripts
  install_systemd_units
  run_migrations
  harden_ssh
  reboot_hint
  log "bootstrap.sh complete"
}

main "$@"
