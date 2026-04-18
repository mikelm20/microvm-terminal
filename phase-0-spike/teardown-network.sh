#!/usr/bin/env bash
# Reverse of setup-network.sh. Safe to run multiple times.
set -euo pipefail

BRIDGE="${BRIDGE:-fc-br0}"
BRIDGE_NET="${BRIDGE_NET:-172.20.0.0/24}"
TAP="${TAP:-tap-fc0}"

[ "${EUID}" -eq 0 ] || { echo "Must run as root"; exit 1; }

UPSTREAM="$(ip -o route show default | awk '{print $5; exit}' || true)"

if [ -n "${UPSTREAM}" ]; then
  iptables -t nat -D POSTROUTING -s "${BRIDGE_NET}" -o "${UPSTREAM}" -j MASQUERADE 2>/dev/null || true
fi
iptables -D FORWARD -i "${BRIDGE}" -j ACCEPT 2>/dev/null || true
iptables -D FORWARD -o "${BRIDGE}" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true

ip link show "${TAP}" &>/dev/null && ip link del "${TAP}" || true
ip link show "${BRIDGE}" &>/dev/null && ip link del "${BRIDGE}" || true

echo "Network torn down."
