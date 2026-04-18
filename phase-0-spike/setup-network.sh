#!/usr/bin/env bash
#
# Phase 0 host networking for a single Firecracker VM.
# Idempotent: safe to re-run. Call teardown-network.sh to revert.
#
# Topology:
#   fc-br0 (172.20.0.1/24) <-- tap-fc0 <-- VM eth0 (172.20.0.2/24)
#
#   Outbound VM traffic NATed via the host's default interface.
#
# Egress is currently UNRESTRICTED for spike simplicity. The production
# lockdown (allow api.anthropic.com + apt only) is Phase 2.
set -euo pipefail

BRIDGE="${BRIDGE:-fc-br0}"
BRIDGE_CIDR="${BRIDGE_CIDR:-172.20.0.1/24}"
BRIDGE_NET="${BRIDGE_NET:-172.20.0.0/24}"
TAP="${TAP:-tap-fc0}"

log() { printf '\n\033[1;34m[setup-network]\033[0m %s\n' "$*"; }

[ "${EUID}" -eq 0 ] || { echo "Must run as root"; exit 1; }

# Identify the upstream interface (whatever has the default route)
UPSTREAM="$(ip -o route show default | awk '{print $5; exit}')"
[ -n "${UPSTREAM}" ] || { echo "Can't find upstream interface"; exit 1; }
log "Upstream: ${UPSTREAM}"

# Bridge
if ! ip link show "${BRIDGE}" &>/dev/null; then
  log "Creating bridge ${BRIDGE}"
  ip link add name "${BRIDGE}" type bridge
fi
ip addr replace "${BRIDGE_CIDR}" dev "${BRIDGE}"
ip link set "${BRIDGE}" up

# TAP
if ! ip link show "${TAP}" &>/dev/null; then
  log "Creating tap ${TAP}"
  ip tuntap add "${TAP}" mode tap user "${SUDO_USER:-root}"
fi
ip link set "${TAP}" master "${BRIDGE}"
ip link set "${TAP}" up

# IP forwarding (persist)
if [ "$(sysctl -n net.ipv4.ip_forward)" != "1" ]; then
  sysctl -w net.ipv4.ip_forward=1
fi
grep -q '^net.ipv4.ip_forward=1' /etc/sysctl.d/99-learn-platform.conf 2>/dev/null || \
  echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-learn-platform.conf

# NAT via iptables (iptables on Ubuntu 24.04 is iptables-nft backed).
# Add the rule only if not already present.
if ! iptables -t nat -C POSTROUTING -s "${BRIDGE_NET}" -o "${UPSTREAM}" -j MASQUERADE 2>/dev/null; then
  log "Adding NAT: ${BRIDGE_NET} -> ${UPSTREAM}"
  iptables -t nat -A POSTROUTING -s "${BRIDGE_NET}" -o "${UPSTREAM}" -j MASQUERADE
fi
# Forwarding permit
for rule in \
  "FORWARD -i ${BRIDGE} -j ACCEPT" \
  "FORWARD -o ${BRIDGE} -m state --state RELATED,ESTABLISHED -j ACCEPT" ; do
  # shellcheck disable=SC2086
  if ! iptables -C ${rule} 2>/dev/null; then
    # shellcheck disable=SC2086
    iptables -A ${rule}
  fi
done

log "Network ready: bridge=${BRIDGE} tap=${TAP} upstream=${UPSTREAM}"
ip -br addr show "${BRIDGE}"
ip -br addr show "${TAP}"
