#!/usr/bin/env bash
#
# Build the VM rootfs as an ext4 image suitable for Firecracker.
# Runs on Linux x86_64 (the learn-01 host). Requires Docker + root (for losetup + mount).
#
#   Usage: sudo OUT_DIR=/var/lib/firecracker/images bash build-rootfs.sh
#
# Output: ${OUT_DIR}/rootfs.ext4
set -euo pipefail

IMAGE_TAG="${IMAGE_TAG:-learn-vm-rootfs:latest}"
OUT_DIR="${OUT_DIR:-./out}"
ROOTFS_SIZE_MB="${ROOTFS_SIZE_MB:-4096}"   # 4 GiB: Node + Claude Code + learner workspace

log() { printf '\n\033[1;34m[build-rootfs]\033[0m %s\n' "$*"; }

[ "${EUID}" -eq 0 ] || { echo "Must run as root (mount/losetup needed)"; exit 1; }
command -v docker >/dev/null || { echo "Docker is required"; exit 1; }
command -v mkfs.ext4 >/dev/null || { echo "e2fsprogs is required"; exit 1; }

install -d -m 0755 "${OUT_DIR}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log "Building Docker image ${IMAGE_TAG}"
docker build -f "${script_dir}/Dockerfile" -t "${IMAGE_TAG}" "${script_dir}"

log "Exporting container filesystem"
cid="$(docker create "${IMAGE_TAG}")"
trap "docker rm -f '${cid}' >/dev/null 2>&1 || true" EXIT

tmp_root="$(mktemp -d)"
docker export "${cid}" | tar -xf - -C "${tmp_root}"

# Sanity: init must be present and runnable as /sbin/init
if [ ! -x "${tmp_root}/sbin/init" ] && [ ! -L "${tmp_root}/sbin/init" ]; then
  echo "FATAL: /sbin/init missing or not executable in exported rootfs"
  exit 1
fi

# Docker export leaves / with mode 0700, which breaks any non-root service
# (systemd-network, systemd-resolved, the learner shell) because they can't
# traverse /. Restore the standard 0755.
chmod 0755 "${tmp_root}"

# Write a static /etc/resolv.conf (Docker bind-mounts this during build, so we
# finalize it here). systemd-networkd inside the VM sets DNS on eth0, but VM
# code that bypasses systemd-resolved reads /etc/resolv.conf directly, so this
# must be populated.
rm -f "${tmp_root}/etc/resolv.conf"
printf 'nameserver 1.1.1.1\nnameserver 1.0.0.1\n' > "${tmp_root}/etc/resolv.conf"
chmod 0644 "${tmp_root}/etc/resolv.conf"

log "Creating ${ROOTFS_SIZE_MB} MiB ext4 image"
rootfs_img="${OUT_DIR}/rootfs.ext4"
rm -f "${rootfs_img}"
truncate -s "${ROOTFS_SIZE_MB}M" "${rootfs_img}"
mkfs.ext4 -q -F -L rootfs "${rootfs_img}"

log "Copying rootfs content into image"
mnt="$(mktemp -d)"
mount -o loop "${rootfs_img}" "${mnt}"
cp -a "${tmp_root}"/. "${mnt}"/
sync
umount "${mnt}"
rmdir "${mnt}"
rm -rf "${tmp_root}"

chown "${SUDO_USER:-root}:${SUDO_USER:-root}" "${rootfs_img}" 2>/dev/null || true

log "Done: ${rootfs_img} ($(du -h "${rootfs_img}" | cut -f1))"
