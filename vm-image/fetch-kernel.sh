#!/usr/bin/env bash
#
# Download the Firecracker-compatible kernel (vmlinux) for x86_64.
# Firecracker-CI publishes pre-built, minimal-config kernels that match the supported
# Firecracker release line. Pin the version; bump intentionally.
#
#   Usage: OUT_DIR=/var/lib/firecracker/images bash fetch-kernel.sh
set -euo pipefail

OUT_DIR="${OUT_DIR:-./out}"
FC_LINE="${FC_LINE:-v1.10}"
KERNEL_VERSION="${KERNEL_VERSION:-6.1.102}"
KERNEL_URL="${KERNEL_URL:-https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/${FC_LINE}/x86_64/vmlinux-${KERNEL_VERSION}}"

log() { printf '\n\033[1;34m[fetch-kernel]\033[0m %s\n' "$*"; }

install -d -m 0755 "${OUT_DIR}"
out="${OUT_DIR}/vmlinux-${KERNEL_VERSION}"
if [ -s "${out}" ]; then
  log "Already present: ${out} ($(du -h "${out}" | cut -f1))"
  exit 0
fi

log "Fetching ${KERNEL_URL}"
curl -fsSL "${KERNEL_URL}" -o "${out}.partial"
mv "${out}.partial" "${out}"
log "Wrote ${out} ($(du -h "${out}" | cut -f1))"

# Also update a stable symlink the spike and control plane can rely on
ln -sfn "vmlinux-${KERNEL_VERSION}" "${OUT_DIR}/vmlinux"
