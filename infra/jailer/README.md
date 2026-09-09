# infra/jailer

Jailer wraps every Firecracker microVM so the guest kernel, the `claude`
process, and the user's shell run with three layers of isolation:

1. **seccomp** filter (`profile.d/seccomp.json`) that denies every syscall
   Firecracker does not need.
2. **chroot** under `/srv/jailer/firecracker/<vm-id>/root`. The guest filesystem
   is the only thing visible; the host `/etc`, `/var/lib/microvm-terminal`,
   and friends are not reachable.
3. **cgroup v2** leaf at `/sys/fs/cgroup/microvm/<vm-id>` with `cpu.max`,
   `memory.max`, and `pids.max` so a single runaway VM cannot starve the host.

## Threat model

The adversary is the VM user. Their Claude instance can run arbitrary bash
inside the VM with permission prompts disabled. We assume they will attempt:

- host escape via the VM's ext4 image or vsock path.
- CPU or RAM exhaustion (fork bombs, `stress --vm`).
- lateral network calls to other VMs on the same tap bridge.
- disk fill of the host (`/var/log`, `/srv/jailer`).
- kernel exploits through KVM ioctl surface.

What Jailer blocks:

- seccomp denies syscalls like `ptrace`, `perf_event_open`, `bpf`,
  `kexec_load`, module ops, and user-namespace creation, so a kernel exploit
  needs a zero-day that reaches KVM through the narrow ioctl surface.
- chroot denies visibility of the host filesystem: even if Firecracker is
  compromised, its view of the world is the chroot.
- cgroups bound CPU and memory to the configured guest size (plus 256 MiB
  for the Firecracker process), pids at 512 per VM.

What Jailer does NOT block. These are the job of the **nftables** rules in
`../nftables/microvm.rules`:

- VM-to-VM lateral traffic.
- VM egress to arbitrary internet destinations.
- VM access to the host control plane port.

## Files

- `jailer.sh` - entrypoint, called by `control-plane/internal/vm.LaunchJailed`.
  Idempotent for a given `--vm-id`: removes stale chroot, recreates cgroup.
- `profile.d/seccomp.json` - custom seccomp filter. Stricter than
  `jailer --seccomp-level 2` (the fallback when unset).

## Adding a syscall

If a new Firecracker release needs a syscall not on the allowlist, you will
see `Operation not permitted` inside the VM logs. Process:

1. Reproduce under `--seccomp-level 2` (default). If that still fails, it is
   not a seccomp issue.
2. Audit the syscall in the kernel man pages. Is it creating namespaces,
   loading modules, or doing ioctl passthrough? If yes, reject and file an
   issue.
3. If benign, add it to `profile.d/seccomp.json` with a commit message
   citing the Firecracker release notes line that introduced the need.
4. Re-run `make integration-vm` on the host.

Never ship the allowlist with `ptrace`, `perf_event_open`, `bpf`, `keyctl`,
`mount`, `pivot_root`, `kexec_load`, `init_module`, `setns`,
`unshare(CLONE_NEWUSER)`. Those are bright-line denials.

## Manual smoke test

With a VM running, find its Firecracker pid and check the chroot took:

```
readlink /proc/<firecracker-pid>/root
# must print a path under /srv/jailer/firecracker/<id>/root, not `/`.
```
