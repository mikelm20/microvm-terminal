package firecracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type LaunchSpec struct {
	BinaryPath string // /usr/local/bin/firecracker
	VMDir      string // working directory for this VM's socket + config
	KernelPath string
	RootfsPath string
	VcpuCount  int
	MemMiB     int
	TapName    string
	GuestMAC   string
	BootArgs   string // full kernel cmdline; session_token is included here
	VsockCID   uint32 // guest vsock CID
	VsockUDS   string // path to the host-side vsock multiplexer socket
}

// Process is a running Firecracker VM.
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	sockDir string
}

// Launch writes a Firecracker JSON config and spawns the process with stdio
// attached as pipes we can read/write. Returns a Process handle; caller must
// call Stop() to tear down.
func Launch(ctx context.Context, spec LaunchSpec) (*Process, error) {
	if err := os.MkdirAll(spec.VMDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir vm dir: %w", err)
	}
	// Firecracker refuses to start if the socket already exists.
	sockPath := filepath.Join(spec.VMDir, "fc.sock")
	_ = os.Remove(sockPath)

	bootArgs := spec.BootArgs
	if bootArgs == "" {
		bootArgs = "console=ttyS0 reboot=k panic=1 pci=off rw"
	}
	cfg := fcConfig{
		BootSource: fcBootSource{
			KernelImagePath: spec.KernelPath,
			BootArgs:        bootArgs,
		},
		Drives: []fcDrive{{
			DriveID:      "rootfs",
			PathOnHost:   spec.RootfsPath,
			IsRootDevice: true,
			IsReadOnly:   false,
		}},
		NetworkInterfaces: []fcNetIface{{
			IfaceID:     "eth0",
			GuestMAC:    spec.GuestMAC,
			HostDevName: spec.TapName,
		}},
		MachineConfig: fcMachineConfig{
			VcpuCount: spec.VcpuCount,
			MemSizeMiB: spec.MemMiB,
			Smt:       false,
		},
	}
	if spec.VsockCID > 0 && spec.VsockUDS != "" {
		cfg.Vsock = &fcVsock{
			GuestCID: spec.VsockCID,
			UDSPath:  spec.VsockUDS,
		}
	}
	configPath := filepath.Join(spec.VMDir, "vm.json")
	configBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, configBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	cmd := exec.CommandContext(ctx, spec.BinaryPath,
		"--api-sock", sockPath,
		"--config-file", configPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr with serial output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start firecracker: %w", err)
	}

	return &Process{cmd: cmd, stdin: stdin, stdout: stdout, sockDir: spec.VMDir}, nil
}

// Stdin returns the writer that feeds the guest's serial console (stdin side).
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout returns the reader that drains the guest's serial console (stdout side).
func (p *Process) Stdout() io.Reader { return p.stdout }

// Stop tears down the VM. Sends SIGTERM first, then SIGKILL after 5s.
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

// Wait blocks until the VM exits on its own (e.g., guest shutdown).
func (p *Process) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// VmDir is the working directory we provisioned for this VM; callers may
// delete it after Stop.
func (p *Process) VmDir() string { return p.sockDir }

type fcConfig struct {
	BootSource        fcBootSource    `json:"boot-source"`
	Drives            []fcDrive       `json:"drives"`
	NetworkInterfaces []fcNetIface    `json:"network-interfaces"`
	MachineConfig     fcMachineConfig `json:"machine-config"`
	Vsock             *fcVsock        `json:"vsock,omitempty"`
}

type fcVsock struct {
	GuestCID uint32 `json:"guest_cid"`
	UDSPath  string `json:"uds_path"`
}

type fcBootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args"`
}

type fcDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

type fcNetIface struct {
	IfaceID     string `json:"iface_id"`
	GuestMAC    string `json:"guest_mac"`
	HostDevName string `json:"host_dev_name"`
}

type fcMachineConfig struct {
	VcpuCount  int  `json:"vcpu_count"`
	MemSizeMiB int  `json:"mem_size_mib"`
	Smt        bool `json:"smt"`
}

