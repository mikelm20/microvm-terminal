// Package config loads the daemon's runtime configuration from a TOML file.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// ListenAddr is the HTTP bind address. Default 127.0.0.1:8080.
	ListenAddr string

	// MaxConcurrent caps simultaneous VMs. Default 3 (MVP scope).
	MaxConcurrent int

	// IdleTimeoutSeconds tears down a VM whose WS has been closed for this long
	// without reconnection. Default 300.
	IdleTimeoutSeconds int

	// Paths to Firecracker assets.
	FirecrackerBin string // default /usr/local/bin/firecracker
	JailerBin      string // default /usr/local/bin/jailer
	KernelPath     string // default /var/lib/firecracker/images/vmlinux
	RootfsPath     string // default /var/lib/firecracker/images/rootfs.ext4

	// VmDataDir is the per-VM working directory root. Default /var/lib/firecracker/vms.
	VmDataDir string

	// Network.
	BridgeName string // default fc-br0
	VmCIDRBase string // first usable /30, default 172.20.0.0/24

	// ClaudeOAuthTokenFile is read once at session launch and injected into the
	// VM. For the MVP it's a single file; later it will become a per-session lookup.
	ClaudeOAuthTokenFile string // default /etc/learn-platform/claude-oauth-token

	// AuthPasswordFile contains a single-line shared secret that gates
	// POST /sessions (and WS / DELETE for sessions). Everyone gets the same
	// password for MVP; replace with per-user accounts before launch.
	AuthPasswordFile string // default /etc/learn-platform/auth-password

	// AuthCookieSecretFile is HMAC key for signing the login cookie. 32 random
	// bytes hex-encoded. Rotating invalidates all sessions (intentional).
	AuthCookieSecretFile string // default /etc/learn-platform/auth-cookie-secret
}

// Load reads a minimal TOML-like file. Keys are flat: `key = value`. Missing
// file is OK; defaults fill in. Unknown keys are ignored (forward-compat).
func Load(path string) (Config, error) {
	cfg := Config{
		ListenAddr:           "127.0.0.1:8080",
		MaxConcurrent:        3,
		IdleTimeoutSeconds:   300,
		FirecrackerBin:       "/usr/local/bin/firecracker",
		JailerBin:            "/usr/local/bin/jailer",
		KernelPath:           "/var/lib/firecracker/images/vmlinux",
		RootfsPath:           "/var/lib/firecracker/images/rootfs.ext4",
		VmDataDir:            "/var/lib/firecracker/vms",
		BridgeName:           "fc-br0",
		VmCIDRBase:           "172.20.0.0/24",
		ClaudeOAuthTokenFile: "/etc/learn-platform/claude-oauth-token",
		AuthPasswordFile:     "/etc/learn-platform/auth-password",
		AuthCookieSecretFile: "/etc/learn-platform/auth-cookie-secret",
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"`)

		switch key {
		case "listen_addr":
			cfg.ListenAddr = val
		case "max_concurrent":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.MaxConcurrent = n
			}
		case "idle_timeout_seconds":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.IdleTimeoutSeconds = n
			}
		case "firecracker_bin":
			cfg.FirecrackerBin = val
		case "jailer_bin":
			cfg.JailerBin = val
		case "kernel_path":
			cfg.KernelPath = val
		case "rootfs_path":
			cfg.RootfsPath = val
		case "vm_data_dir":
			cfg.VmDataDir = val
		case "bridge_name":
			cfg.BridgeName = val
		case "vm_cidr_base":
			cfg.VmCIDRBase = val
		case "claude_oauth_token_file":
			cfg.ClaudeOAuthTokenFile = val
		case "auth_password_file":
			cfg.AuthPasswordFile = val
		case "auth_cookie_secret_file":
			cfg.AuthCookieSecretFile = val
		}
	}
	if err := sc.Err(); err != nil {
		return cfg, fmt.Errorf("scan %s: %w", path, err)
	}
	return cfg, nil
}
