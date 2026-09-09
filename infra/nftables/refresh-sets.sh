#!/usr/bin/env bash
#
# refresh-sets.sh - resolve allowlisted FQDNs into A records and push them
# into the nftables sets `anthropic_v4` and `apt_v4`.
#
# Triggered by microvm-nftables-refresh.service (oneshot) on boot, and by its
# sibling .timer nightly. Safe to run ad hoc. Idempotent: existing elements
# are replaced, not appended.

set -euo pipefail

ANTHROPIC_HOSTS=(
  api.anthropic.com
)

APT_HOSTS=(
  archive.ubuntu.com
  security.ubuntu.com
  deb.debian.org
)

resolve_ipv4() {
  local host="$1"
  # getent is deterministic across systems and handles /etc/hosts overrides.
  getent ahostsv4 "$host" 2>/dev/null | awk '{print $1}' | sort -u
}

flush_and_fill() {
  local set_name="$1"; shift
  local ips=("$@")
  # Atomic replace: flush set, add elements in one transaction.
  local tmp
  tmp="$(mktemp)"
  {
    echo "flush set inet microvm ${set_name}"
    if [[ ${#ips[@]} -gt 0 ]]; then
      echo "add element inet microvm ${set_name} { $(IFS=,; echo "${ips[*]}") }"
    fi
  } > "${tmp}"
  nft -f "${tmp}"
  rm -f "${tmp}"
}

main() {
  local -a anthropic_ips=()
  for h in "${ANTHROPIC_HOSTS[@]}"; do
    while read -r ip; do
      [[ -n "$ip" ]] && anthropic_ips+=("$ip")
    done < <(resolve_ipv4 "$h")
  done
  flush_and_fill anthropic_v4 "${anthropic_ips[@]}"

  local -a apt_ips=()
  for h in "${APT_HOSTS[@]}"; do
    while read -r ip; do
      [[ -n "$ip" ]] && apt_ips+=("$ip")
    done < <(resolve_ipv4 "$h")
  done
  flush_and_fill apt_v4 "${apt_ips[@]}"

  echo "refresh-sets: anthropic=${#anthropic_ips[@]} apt=${#apt_ips[@]}"
}

main "$@"
