// Package vm wraps Firecracker in Jailer so every microVM runs under a
// seccomp filter, a chroot, and a cgroup. LaunchJailed prepares the chroot,
// materialises the rootfs + kernel inside it, and runs jailer with the right
// flags. The returned handle has the same surface as firecracker.Process.
package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mikelm20/microvm-terminal/control-plane/internal/firecracker"
)

// JailedSpec describes one VM plus the constraints imposed on it.
type JailedSpec struct {
	// Inherit every field Firecracker already accepts.
	firecracker.LaunchSpec

	// JailerBin is the jailer wrapper script. Defaults to
	// /usr/local/libexec/microvm-terminal/jailer.sh which ships with this
	// repo. Must exist and be executable as root; it drops privileges
	// internally.
	JailerBin string

	// JailerUser is the unprivileged system user jailer execs firecracker
	// as, looked up with getent when JailerUID/JailerGID are zero. Defaults
	// to "microvm", which infra/bootstrap.sh creates.
	JailerUser string
	JailerUID  int
	JailerGID  int

	// VMID is the unique identifier used to scope the chroot and cgroup.
	// Callers typically pass the control plane's session id (UUID); the
	// script sanitises it to alphanumeric + dash.
	VMID string

	// CPUQuotaMicros and MemBytes are the cgroup v2 limits. Defaults:
	// 1 vCPU worth of CPU (100000 out of 100000), 1 GiB RAM.
	CPUQuotaMicros int64
	MemBytes       int64

	// SeccompProfile is the path to a JSON seccomp filter. Empty keeps
	// the stricter of Firecracker's built-in profiles.
	SeccompProfile string

	// ChrootBase is where jailer builds the per-VM chroot. Each VM gets
	// $ChrootBase/firecracker/$VMID. Defaults to /srv/jailer.
	ChrootBase string
}

// LaunchJailed starts a Firecracker VM through the jailer wrapper. The caller
// gets a firecracker.Process back and can treat it exactly like one produced
// by firecracker.Launch: same stdin, stdout, Stop, Wait, VmDir.
func LaunchJailed(ctx context.Context, spec JailedSpec) (*Process, error) {
	if spec.VMID == "" {
		return nil, errors.New("vm: JailedSpec.VMID required")
	}
	if spec.JailerBin == "" {
		spec.JailerBin = "/usr/local/libexec/microvm-terminal/jailer.sh"
	}
	if spec.ChrootBase == "" {
		spec.ChrootBase = "/srv/jailer"
	}
	if spec.CPUQuotaMicros == 0 {
		spec.CPUQuotaMicros = 100_000 // 1 CPU
	}
	if spec.MemBytes == 0 {
		spec.MemBytes = 1 << 30 // 1 GiB
	}
	if spec.JailerUser == "" {
		spec.JailerUser = "microvm"
	}
	if spec.JailerUID == 0 || spec.JailerGID == 0 {
		uid, gid, err := lookupUser(spec.JailerUser)
		if err != nil {
			return nil, fmt.Errorf("vm: lookup user %s: %w", spec.JailerUser, err)
		}
		if spec.JailerUID == 0 {
			spec.JailerUID = uid
		}
		if spec.JailerGID == 0 {
			spec.JailerGID = gid
		}
	}

	// VMDir stays outside the chroot; the script bind-mounts what it needs.
	if err := os.MkdirAll(spec.VMDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir vm dir: %w", err)
	}

	args := []string{
		spec.JailerBin,
		"--vm-id", spec.VMID,
		"--uid", strconv.Itoa(spec.JailerUID),
		"--gid", strconv.Itoa(spec.JailerGID),
		"--chroot-base", spec.ChrootBase,
		"--cpu-quota-us", strconv.FormatInt(spec.CPUQuotaMicros, 10),
		"--mem-bytes", strconv.FormatInt(spec.MemBytes, 10),
		"--kernel", spec.KernelPath,
		"--rootfs", spec.RootfsPath,
		"--vm-dir", spec.VMDir,
		"--firecracker-bin", spec.BinaryPath,
	}
	if spec.SeccompProfile != "" {
		args = append(args, "--seccomp-profile", spec.SeccompProfile)
	}
	if spec.VsockUDS != "" {
		args = append(args, "--vsock-uds", spec.VsockUDS)
		args = append(args, "--vsock-cid", strconv.FormatUint(uint64(spec.VsockCID), 10))
	}
	if spec.TapName != "" {
		args = append(args, "--tap", spec.TapName, "--guest-mac", spec.GuestMAC)
	}
	if spec.BootArgs != "" {
		args = append(args, "--boot-args", spec.BootArgs)
	}
	args = append(args, "--vcpu", strconv.Itoa(spec.VcpuCount))
	args = append(args, "--mem-mib", strconv.Itoa(spec.MemMiB))

	cmd := exec.CommandContext(ctx, "/usr/bin/env", append([]string{"bash"}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start jailer: %w", err)
	}

	return &Process{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		vmDir:  spec.VMDir,
		vmID:   spec.VMID,
		chroot: filepath.Join(spec.ChrootBase, "firecracker", spec.VMID, "root"),
	}, nil
}

// Process is the handle for a jailed VM. API mirrors firecracker.Process so
// call sites do not know or care whether Jailer is wrapping.
type Process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	vmDir  string
	vmID   string
	chroot string
}

// Stdin returns the serial console writer.
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout returns the serial console reader.
func (p *Process) Stdout() io.Reader { return p.stdout }

// VmDir returns the per-VM working directory on the host.
func (p *Process) VmDir() string { return p.vmDir }

// VMID returns the identifier used to scope this jail.
func (p *Process) VMID() string { return p.vmID }

// Chroot returns the absolute path of the chroot root used by jailer.
// Useful for smoke tests that assert `readlink /proc/<pid>/root == chroot`.
func (p *Process) Chroot() string { return p.chroot }

// Stop signals SIGTERM then SIGKILL after 5s. Matches firecracker.Process.
func (p *Process) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = p.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
	}
	_ = p.stdin.Close()
	_ = p.stdout.Close()
	return nil
}

// Wait blocks until the VM exits.
func (p *Process) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// lookupUser returns the uid/gid for a system user. The user is provisioned
// by infra/bootstrap.sh. Tests can short-circuit by providing explicit
// UID/GID in JailedSpec.
func lookupUser(name string) (int, int, error) {
	// os/user avoided to keep this file free of cgo dependencies on Linux
	// builds. getent is always present on Ubuntu.
	out, err := exec.Command("getent", "passwd", name).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("getent: %w", err)
	}
	parts := splitColon(string(out))
	if len(parts) < 4 {
		return 0, 0, fmt.Errorf("getent: unexpected output")
	}
	uid, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, fmt.Errorf("uid: %w", err)
	}
	gid, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, fmt.Errorf("gid: %w", err)
	}
	return uid, gid, nil
}

func splitColon(s string) []string {
	out := make([]string, 0, 8)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
