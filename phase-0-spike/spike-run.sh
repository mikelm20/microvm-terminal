#!/usr/bin/env bash
#
# Phase 0 spike: boot ONE Firecracker VM with stdio attached to the serial console.
# When you see a login prompt (it auto-logs in as `learner`), you're in the VM.
# Ctrl+A then X to exit Firecracker (when running under screen); otherwise kill from another shell.
#
# This script does NOT use jailer yet (Phase 0 is about proving primitives).
# Phase 1 control plane will wrap Firecracker with jailer.
set -euo pipefail

IMAGES_DIR="${IMAGES_DIR:-/var/lib/firecracker/images}"
VM_ID="${VM_ID:-spike}"
VM_DIR="${VM_DIR:-/var/lib/firecracker/vms/${VM_ID}}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[ "${EUID}" -eq 0 ] || { echo "Must run as root (for Firecracker + network)"; exit 1; }

# 1) Make sure host networking is up
bash "${script_dir}/setup-network.sh"

# 2) Copy golden rootfs into a per-VM working copy (so we don't mutate the original)
install -d -m 0755 "${VM_DIR}"
if [ ! -f "${VM_DIR}/rootfs.ext4" ]; then
  cp --reflink=auto "${IMAGES_DIR}/rootfs.ext4" "${VM_DIR}/rootfs.ext4"
fi

# 3) Render Firecracker config
config="${VM_DIR}/vm.json"
sed \
  -e "s|__KERNEL__|${IMAGES_DIR}/vmlinux|g" \
  -e "s|__ROOTFS__|${VM_DIR}/rootfs.ext4|g" \
  "${script_dir}/vm.json.tpl" > "${config}"

# 4) Remove stale API socket from a prior run
sock="${VM_DIR}/fc.sock"
rm -f "${sock}"

echo "Launching Firecracker. Press Ctrl+C to kill the VM."
echo "Config: ${config}"
echo "Socket: ${sock}"
echo

# stdin/stdout attach to the guest serial console
exec /usr/local/bin/firecracker --api-sock "${sock}" --config-file "${config}"
