#!/usr/bin/env bash
#
# bootstrap.sh - idempotent host setup for microvm-terminal.
# Safe to re-run; each step detects prior state and skips when possible.
#
#   Usage: sudo bash bootstrap.sh
#
# Bump pinned versions in the block below and re-run to upgrade a component.
# Anything this script installs should be installed ONLY via this script.

set -euo pipefail

### --- Pinned versions (intentional upgrades only) ------------------------
FIRECRACKER_VERSION="${FIRECRACKER_VERSION:-v1.10.1}"
GO_VERSION="${GO_VERSION:-1.25.0}"

### --- System layout ------------------------------------------------------
SVC_USER="${SVC_USER:-microvm}"
SVC_HOME="${SVC_HOME:-/var/lib/microvm-terminal}"
SVC_ETC="${SVC_ETC:-/etc/microvm-terminal}"
SVC_LIBEXEC="${SVC_LIBEXEC:-/usr/local/libexec/microvm-terminal}"
FC_DATA_DIR="${FC_DATA_DIR:-/var/lib/firecracker}"
# Set FC_DATA_DEV to a block device to format and mount it at FC_DATA_DIR.
# Leave it empty to keep VM images and scratch on the root filesystem.
FC_DATA_DEV="${FC_DATA_DEV:-}"
JAILER_CHROOT_BASE="${JAILER_CHROOT_BASE:-/srv/jailer}"

### --- Source layout (this repo) ------------------------------------------
REPO_DIR="${REPO_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

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
  [ -c /dev/kvm ] || { echo "FATAL: /dev/kvm missing (no KVM or nested virt not enabled)"; exit 1; }
  grep -Eq 'vmx|svm' /proc/cpuinfo || { echo "FATAL: no vmx/svm CPU flag"; exit 1; }
  log "Virt: /dev/kvm present, $(grep -Eo 'vmx|svm' /proc/cpuinfo | sort -u | tr '\n' ' ')exposed"
}

apt_refresh() {
  log "apt update + upgrade"
  apt-get update -y
  DEBIAN_FRONTEND=noninteractive apt-get upgrade -y
  apt_install unattended-upgrades apt-listchanges
  cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOT'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOT
}

base_packages() {
  log "Base packages"
  apt_install \
    curl wget jq git ca-certificates gnupg lsb-release \
    vim tmux htop tree \
    ufw nftables bridge-utils \
    build-essential pkg-config \
    postgresql
  systemctl enable --now postgresql
}

configure_ufw() {
  log "ufw: default deny incoming, allow 22/80/443"
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
  install -d -m 0755 "${FC_DATA_DIR}"
  if [ -z "${FC_DATA_DEV}" ]; then
    log "Firecracker data: ${FC_DATA_DIR} on the root filesystem (FC_DATA_DEV unset)"
    return 0
  fi
  log "Firecracker data disk: ${FC_DATA_DEV} -> ${FC_DATA_DIR}"
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
  cat > /etc/profile.d/go.sh <<'EOT'
export PATH=$PATH:/usr/local/go/bin
EOT
  chmod 0644 /etc/profile.d/go.sh
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
  # Only needed to build the guest rootfs (vm-image/build-rootfs.sh).
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
}

create_service_user() {
  if id "${SVC_USER}" >/dev/null 2>&1; then
    log "User ${SVC_USER} exists"
  else
    log "Creating system user ${SVC_USER}"
    useradd --system --home-dir "${SVC_HOME}" --create-home --shell /usr/sbin/nologin "${SVC_USER}"
  fi
  install -d -o "${SVC_USER}" -g "${SVC_USER}" -m 0755 "${SVC_HOME}"
  install -d -o root          -g "${SVC_USER}" -m 0750 "${SVC_ETC}"
  install -d -o "${SVC_USER}" -g "${SVC_USER}" -m 0755 "${FC_DATA_DIR}"
  usermod -aG kvm "${SVC_USER}" 2>/dev/null || true
}

create_database() {
  log "Postgres role + database ${SVC_USER}"
  if ! sudo -u postgres psql -tAc "select 1 from pg_roles where rolname='${SVC_USER}'" | grep -q 1; then
    sudo -u postgres psql -c "create role ${SVC_USER} login password '${SVC_USER}'"
  fi
  if ! sudo -u postgres psql -tAc "select 1 from pg_database where datname='${SVC_USER}'" | grep -q 1; then
    sudo -u postgres createdb -O "${SVC_USER}" "${SVC_USER}"
  fi
}

install_config_and_secrets() {
  log "Config and secrets under ${SVC_ETC}"
  if [ ! -s "${SVC_ETC}/config.yaml" ]; then
    install -m 0640 -o root -g "${SVC_USER}" "${REPO_DIR}/control-plane/config.example.yaml" "${SVC_ETC}/config.yaml"
    log "  wrote ${SVC_ETC}/config.yaml from the example; edit public_origin at least"
  fi
  if [ ! -s "${SVC_ETC}/password" ]; then
    openssl rand -base64 18 | tr '/+' '_-' | cut -c1-20 > "${SVC_ETC}/password"
    chmod 0600 "${SVC_ETC}/password"
    log "  generated ${SVC_ETC}/password (shared login password; read it with sudo cat)"
  fi
  if [ ! -s "${SVC_ETC}/claude-oauth-token" ]; then
    log "  ${SVC_ETC}/claude-oauth-token is missing: run 'claude setup-token' on a workstation and write the single line there, mode 0600"
  fi
}

install_gate_scripts() {
  log "Installing jailer.sh + refresh-sets.sh under ${SVC_LIBEXEC}"
  install -d -m 0755 "${SVC_LIBEXEC}"
  install -m 0755 "${REPO_DIR}/infra/jailer/jailer.sh" "${SVC_LIBEXEC}/jailer.sh"
  install -m 0755 "${REPO_DIR}/infra/nftables/refresh-sets.sh" "${SVC_LIBEXEC}/refresh-sets.sh"

  install -d -m 0755 "${SVC_ETC}/jailer"
  install -m 0644 "${REPO_DIR}/infra/jailer/profile.d/seccomp.json" "${SVC_ETC}/jailer/seccomp.json"

  install -d -m 0755 "${SVC_ETC}/nftables"
  install -m 0644 "${REPO_DIR}/infra/nftables/microvm.rules" "${SVC_ETC}/nftables/microvm.rules"

  install -d -m 0755 "${JAILER_CHROOT_BASE}"
}

install_systemd_units() {
  log "Installing systemd units"
  for u in microvm-terminal.service microvm-nftables.service \
           microvm-nftables-refresh.service microvm-nftables-refresh.timer \
           claude-proxy.service firecracker-jailer@.service; do
    install -m 0644 "${REPO_DIR}/infra/systemd/${u}" "/etc/systemd/system/${u}"
  done
  systemctl daemon-reload
  systemctl enable --now microvm-nftables.service
  systemctl enable --now microvm-nftables-refresh.timer
  # The control plane is enabled but not started here: it needs the binary
  # (make -C control-plane install), the token file and the VM images first.
  systemctl enable microvm-terminal.service
  log "enabled microvm-terminal.service (start it after installing the binary, token and images)"
  log "claude-proxy.service is optional; enable it after writing ${SVC_ETC}/proxy.env"
}

harden_ssh() {
  log "Hardening sshd"
  local cfg=/etc/ssh/sshd_config.d/99-microvm-terminal.conf
  cat > "${cfg}" <<'EOT'
# Managed by microvm-terminal bootstrap.sh
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
EOT
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
  apt_refresh
  base_packages
  configure_ufw
  mount_fc_data
  install_firecracker
  install_go
  install_caddy
  install_docker
  create_service_user
  create_database
  install_config_and_secrets
  install_gate_scripts
  install_systemd_units
  harden_ssh
  reboot_hint
  log "bootstrap.sh complete. Next: sudo make -C vm-image all; make -C control-plane install; systemctl start microvm-terminal"
}

main "$@"
