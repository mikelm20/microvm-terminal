#!/usr/bin/env bash
#
# jailer.sh - launch Firecracker under jailer with seccomp, chroot, and cgroups.
#
# Called once per VM by the control plane. Translates a flat set of flags into
# the args Firecracker's jailer expects, assembles the chroot, creates a cgroup
# v2 leaf with CPU + memory limits, and execs jailer. jailer in turn drops to
# the unprivileged learn user and enters a seccomp filter before launching
# Firecracker inside the chroot.
#
# Usage:
#   jailer.sh --vm-id <id> --uid <n> --gid <n> \
#             --chroot-base /srv/jailer \
#             --cpu-quota-us 100000 --mem-bytes 1073741824 \
#             --kernel /path --rootfs /path --vm-dir /path \
#             --firecracker-bin /usr/local/bin/firecracker \
#             [--seccomp-profile /path.json] \
#             [--vsock-uds /path --vsock-cid 3] \
#             [--tap tap0 --guest-mac aa:bb:...] \
#             [--boot-args "console=ttyS0 ..."] \
#             --vcpu 1 --mem-mib 1024
#
# Reads no secrets, emits no secrets. Safe to run with verbose tracing enabled
# via JAILER_SH_TRACE=1.

set -euo pipefail

[[ "${JAILER_SH_TRACE:-0}" == "1" ]] && set -x

VMID=""
UID_ARG=""
GID_ARG=""
CHROOT_BASE="/srv/jailer"
CPU_QUOTA_US="100000"
MEM_BYTES="1073741824"
KERNEL=""
ROOTFS=""
VM_DIR=""
FIRECRACKER_BIN="/usr/local/bin/firecracker"
JAILER_BIN="${JAILER_BIN:-/usr/local/bin/jailer}"
SECCOMP_PROFILE=""
VSOCK_UDS=""
VSOCK_CID=""
TAP=""
GUEST_MAC=""
BOOT_ARGS=""
VCPU="1"
MEM_MIB="1024"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vm-id)           VMID="$2"; shift 2;;
    --uid)             UID_ARG="$2"; shift 2;;
    --gid)             GID_ARG="$2"; shift 2;;
    --chroot-base)     CHROOT_BASE="$2"; shift 2;;
    --cpu-quota-us)    CPU_QUOTA_US="$2"; shift 2;;
    --mem-bytes)       MEM_BYTES="$2"; shift 2;;
    --kernel)          KERNEL="$2"; shift 2;;
    --rootfs)          ROOTFS="$2"; shift 2;;
    --vm-dir)          VM_DIR="$2"; shift 2;;
    --firecracker-bin) FIRECRACKER_BIN="$2"; shift 2;;
    --seccomp-profile) SECCOMP_PROFILE="$2"; shift 2;;
    --vsock-uds)       VSOCK_UDS="$2"; shift 2;;
    --vsock-cid)       VSOCK_CID="$2"; shift 2;;
    --tap)             TAP="$2"; shift 2;;
    --guest-mac)       GUEST_MAC="$2"; shift 2;;
    --boot-args)       BOOT_ARGS="$2"; shift 2;;
    --vcpu)            VCPU="$2"; shift 2;;
    --mem-mib)         MEM_MIB="$2"; shift 2;;
    *) echo "jailer.sh: unknown arg: $1" >&2; exit 2;;
  esac
done

for f in VMID UID_ARG GID_ARG KERNEL ROOTFS VM_DIR FIRECRACKER_BIN; do
  if [[ -z "${!f}" ]]; then
    echo "jailer.sh: missing required --${f,,}" >&2
    exit 2
  fi
done

# Sanitise VMID so nothing ever reaches the filesystem path with ../ or shell
# metacharacters. Alphanumeric and dashes only. Length capped at 64 to avoid
# overflowing jailer's internal buffers.
SAFE_VMID="$(echo -n "$VMID" | tr -c 'A-Za-z0-9-' '_' | head -c 64)"
if [[ "$SAFE_VMID" != "$VMID" ]]; then
  echo "jailer.sh: vm-id contained unsafe chars, normalised to: $SAFE_VMID" >&2
fi

CHROOT_ROOT="${CHROOT_BASE}/firecracker/${SAFE_VMID}/root"
CGROUP_DIR="/sys/fs/cgroup/learn/${SAFE_VMID}"

install -d -m 0755 "${CHROOT_BASE}"
install -d -m 0755 "$(dirname "${CHROOT_ROOT}")"

# jailer creates the root directory itself and refuses if it exists. Clean any
# leftover from a previous aborted launch.
if [[ -e "${CHROOT_ROOT}" ]]; then
  rm -rf "${CHROOT_ROOT}"
fi

# cgroup v2: create a leaf with cpu + memory controllers and set limits. The
# jailer invocation joins this cgroup (via --cgroup cpu.max=...) so every
# descendant inherits the caps.
if [[ ! -d /sys/fs/cgroup/learn ]]; then
  install -d -m 0755 /sys/fs/cgroup/learn
fi
if [[ ! -d "${CGROUP_DIR}" ]]; then
  install -d -m 0755 "${CGROUP_DIR}"
fi
# Ensure the controllers are enabled on the parent before leaf creation.
# Idempotent: writing an already-enabled controller is a no-op.
echo "+cpu +memory +pids" > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
echo "+cpu +memory +pids" > /sys/fs/cgroup/learn/cgroup.subtree_control 2>/dev/null || true

# Firecracker docs: cpu.max takes "<quota> <period>". Period 100000 us = 100ms
# is the default and plenty granular.
echo "${CPU_QUOTA_US} 100000" > "${CGROUP_DIR}/cpu.max"
echo "${MEM_BYTES}"             > "${CGROUP_DIR}/memory.max"
echo "512"                      > "${CGROUP_DIR}/pids.max"

# Assemble Firecracker's JSON config inside a tmp file that we will copy into
# the chroot right after jailer creates it (jailer requires the config path to
# be relative to the chroot root).
VM_JSON="${VM_DIR}/vm.json"
rm -f "${VM_JSON}"

VSOCK_BLOCK=""
if [[ -n "${VSOCK_UDS}" && -n "${VSOCK_CID}" ]]; then
  VSOCK_BLOCK=$(cat <<EOF
  ,"vsock": {
    "guest_cid": ${VSOCK_CID},
    "uds_path": "${VSOCK_UDS}"
  }
EOF
)
fi

NET_BLOCK="[]"
if [[ -n "${TAP}" ]]; then
  NET_BLOCK=$(cat <<EOF
[
    {"iface_id": "eth0", "guest_mac": "${GUEST_MAC}", "host_dev_name": "${TAP}"}
  ]
EOF
)
fi

BA="${BOOT_ARGS:-console=ttyS0 reboot=k panic=1 pci=off rw}"

cat > "${VM_JSON}" <<EOF
{
  "boot-source": {
    "kernel_image_path": "/vmlinux",
    "boot_args": "${BA}"
  },
  "drives": [
    {"drive_id": "rootfs", "path_on_host": "/rootfs.ext4", "is_root_device": true, "is_read_only": false}
  ],
  "network-interfaces": ${NET_BLOCK},
  "machine-config": {"vcpu_count": ${VCPU}, "mem_size_mib": ${MEM_MIB}, "smt": false}
  ${VSOCK_BLOCK}
}
EOF

# jailer builds the chroot lazily. We stage everything it needs on the outside
# so jailer can hard-link our artefacts in. jailer requires these paths to be
# absolute and to have the right ownership before start.
chown "${UID_ARG}:${GID_ARG}" "${VM_JSON}"

SECCOMP_ARGS=()
if [[ -n "${SECCOMP_PROFILE}" ]]; then
  SECCOMP_ARGS=(--seccomp-filter "${SECCOMP_PROFILE}")
else
  SECCOMP_ARGS=(--seccomp-level 2)
fi

# Note: --resource-dir lets jailer copy kernel/rootfs into the chroot; we
# could also hard-link but the bootstrap disk layout makes hard links cross
# filesystems, so copy is safest.
RES_DIR="${VM_DIR}/jail-res"
rm -rf "${RES_DIR}"
install -d -m 0755 "${RES_DIR}"
install -m 0644 "${KERNEL}" "${RES_DIR}/vmlinux"
install -m 0644 "${ROOTFS}" "${RES_DIR}/rootfs.ext4"
install -m 0644 "${VM_JSON}" "${RES_DIR}/vm.json"
chown -R "${UID_ARG}:${GID_ARG}" "${RES_DIR}"

# --new-pid-ns ensures firecracker PID 1 inside the jail is firecracker.
# --cgroup-version 2 selects cgroups v2 (Ubuntu 24.04 default).
exec "${JAILER_BIN}" \
  --id "${SAFE_VMID}" \
  --uid "${UID_ARG}" \
  --gid "${GID_ARG}" \
  --exec-file "${FIRECRACKER_BIN}" \
  --chroot-base-dir "${CHROOT_BASE}" \
  --new-pid-ns \
  --cgroup-version 2 \
  --cgroup "cpu.max=${CPU_QUOTA_US} 100000" \
  --cgroup "memory.max=${MEM_BYTES}" \
  --cgroup "pids.max=512" \
  --resource-dir "${RES_DIR}" \
  "${SECCOMP_ARGS[@]}" \
  -- \
  --config-file /vm.json \
  --api-sock /fc.sock
