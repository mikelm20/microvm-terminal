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
	ClaudeOAuthTokenFile string

	// UseJailer routes every Firecracker launch through
	// infra/jailer/jailer.sh, gaining seccomp + chroot + cgroup v2 isolation.
	// Default true on real launches; opt-out only for bring-up on a clean box.
	UseJailer bool

	// JailerScript is the absolute path to the wrapper script the control
	// plane execs when UseJailer is true. Installed by infra/bootstrap.sh at
	// /usr/local/libexec/learn-platform/jailer.sh.
	JailerScript string

	// JailerChrootBase is where jailer stages each VM's chroot. Defaults to
	// /srv/jailer; the wrapper script creates the subtree at
	// <base>/firecracker/<vm-id>/root.
	JailerChrootBase string

	// JailerCPUQuotaMicros is the cgroup cpu.max quota in microseconds per
	// 100000 us period. Default 100000 (one full vCPU). Set higher to allow
	// burst across cores.
	JailerCPUQuotaMicros int64

	// JailerMemBytes caps cgroup memory.max per VM. Default 1 GiB.
	JailerMemBytes int64

	// JailerSeccompProfile is a path to a JSON seccomp filter consumed by
	// jailer via --seccomp-filter. Empty falls back to --seccomp-level 2.
	JailerSeccompProfile string

	// JailerUID / JailerGID override the unprivileged uid/gid jailer drops
	// to. When 0, the launcher looks up the learn system user via getent.
	JailerUID int
	JailerGID int

	// BootTimeoutSeconds bounds how long a Create call waits for the VM to
	// reach ready (guest-agent hello over vsock). Default 90.
	BootTimeoutSeconds int

	// Legacy single-password gate. Kept for backwards compat with the old MVP.
	AuthPasswordFile     string
	AuthCookieSecretFile string

	// DatabaseURL is the Postgres DSN the new contract API writes to.
	DatabaseURL string // default postgres://learn:learn@localhost:5432/learn?sslmode=disable

	// IdentityCookieSecret is the HMAC key for the signed identity cookie.
	// Hex-encoded 32 bytes. If empty at startup, the server generates one
	// at runtime (dev convenience; production must set this explicitly).
	IdentityCookieSecret string

	// ResendAPIKey enables real email delivery. Empty falls back to stdout.
	ResendAPIKey string

	// ResendFromAddress is the "From" header on magic-link emails.
	ResendFromAddress string

	// PublicOrigin is the scheme+host the API is reachable at. Used when
	// building magic-link URLs and WS URLs returned in responses.
	PublicOrigin string // default http://localhost:8080

	// LessonsDir is where the server reads lessons/*.yml from.
	LessonsDir string

	// VoiceDir is where the server reads shared/voice/<lang>.json from.
	VoiceDir string

	// WarmPoolTarget is how many pre-booted VMs to keep ready. 0 disables.
	WarmPoolTarget int

	// SecureCookies sets the Secure flag on issued cookies. Disable for
	// local http development.
	SecureCookies bool

	// CookieDomain, when non-empty, is set as the Domain attribute on every
	// cookie issued by the control plane. Used to share cookies across
	// subdomains (e.g. ".example.com" so learn.example.com and api.learn.example.com
	// see the same identity). Leave empty in local http dev.
	CookieDomain string

	// UseMockLauncher disables Firecracker and serves mock Sessions instead.
	// Useful for macOS dev and CI.
	UseMockLauncher bool
}

// Load reads a minimal TOML-like file. Keys are flat: `key = value`. Missing
// file is OK; defaults fill in. Unknown keys are ignored (forward-compat).
// Environment variables override file values (useful for Doppler / CI).
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
		UseJailer:            true,
		JailerScript:         "/usr/local/libexec/learn-platform/jailer.sh",
		JailerChrootBase:     "/srv/jailer",
		JailerCPUQuotaMicros: 100_000,
		JailerMemBytes:       1 << 30,
		BootTimeoutSeconds:   90,
		AuthPasswordFile:     "/etc/learn-platform/auth-password",
		AuthCookieSecretFile: "/etc/learn-platform/auth-cookie-secret",
		DatabaseURL:          "postgres://learn:learn@localhost:5432/learn?sslmode=disable",
		PublicOrigin:         "http://localhost:8080",
		LessonsDir:           "lessons",
		VoiceDir:             "shared/voice",
		WarmPoolTarget:       0,
		SecureCookies:        false,
		CookieDomain:         "",
		UseMockLauncher:      false,
	}

	if path != "" {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			if err := parseTOML(f, &cfg); err != nil {
				return cfg, err
			}
		} else if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("open %s: %w", path, err)
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

func parseTOML(f *os.File, cfg *Config) error {
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
		case "use_jailer":
			cfg.UseJailer = (val == "true" || val == "1")
		case "jailer_script":
			cfg.JailerScript = val
		case "jailer_chroot_base":
			cfg.JailerChrootBase = val
		case "jailer_cpu_quota_us":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				cfg.JailerCPUQuotaMicros = n
			}
		case "jailer_mem_bytes":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				cfg.JailerMemBytes = n
			}
		case "jailer_seccomp_profile":
			cfg.JailerSeccompProfile = val
		case "jailer_uid":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.JailerUID = n
			}
		case "jailer_gid":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.JailerGID = n
			}
		case "boot_timeout_seconds":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.BootTimeoutSeconds = n
			}
		case "auth_password_file":
			cfg.AuthPasswordFile = val
		case "auth_cookie_secret_file":
			cfg.AuthCookieSecretFile = val
		case "database_url":
			cfg.DatabaseURL = val
		case "identity_cookie_secret":
			cfg.IdentityCookieSecret = val
		case "resend_api_key":
			cfg.ResendAPIKey = val
		case "resend_from_address":
			cfg.ResendFromAddress = val
		case "public_origin":
			cfg.PublicOrigin = val
		case "lessons_dir":
			cfg.LessonsDir = val
		case "voice_dir":
			cfg.VoiceDir = val
		case "warm_pool_target":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.WarmPoolTarget = n
			}
		case "secure_cookies":
			cfg.SecureCookies = (val == "true" || val == "1")
		case "cookie_domain":
			cfg.CookieDomain = val
		case "use_mock_launcher":
			cfg.UseMockLauncher = (val == "true" || val == "1")
		}
	}
	return sc.Err()
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("LEARN_DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("IDENTITY_COOKIE_SECRET"); v != "" {
		cfg.IdentityCookieSecret = v
	}
	if v := os.Getenv("RESEND_API_KEY"); v != "" {
		cfg.ResendAPIKey = v
	}
	if v := os.Getenv("RESEND_FROM"); v != "" {
		cfg.ResendFromAddress = v
	}
	if v := os.Getenv("PUBLIC_ORIGIN"); v != "" {
		cfg.PublicOrigin = v
	}
	if v := os.Getenv("LESSONS_DIR"); v != "" {
		cfg.LessonsDir = v
	}
	if v := os.Getenv("VOICE_DIR"); v != "" {
		cfg.VoiceDir = v
	}
	if v := os.Getenv("LEARN_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("WARM_POOL_TARGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WarmPoolTarget = n
		}
	}
	if v := os.Getenv("SECURE_COOKIES"); v != "" {
		cfg.SecureCookies = (v == "true" || v == "1")
	}
	if v := os.Getenv("COOKIE_DOMAIN"); v != "" {
		cfg.CookieDomain = v
	}
	if v := os.Getenv("USE_MOCK_LAUNCHER"); v != "" {
		cfg.UseMockLauncher = (v == "true" || v == "1")
	}
	if v := os.Getenv("USE_JAILER"); v != "" {
		cfg.UseJailer = (v == "true" || v == "1")
	}
	if v := os.Getenv("JAILER_SCRIPT"); v != "" {
		cfg.JailerScript = v
	}
	if v := os.Getenv("JAILER_CHROOT_BASE"); v != "" {
		cfg.JailerChrootBase = v
	}
	if v := os.Getenv("JAILER_SECCOMP_PROFILE"); v != "" {
		cfg.JailerSeccompProfile = v
	}
	if v := os.Getenv("FIRECRACKER_BIN"); v != "" {
		cfg.FirecrackerBin = v
	}
	if v := os.Getenv("KERNEL_PATH"); v != "" {
		cfg.KernelPath = v
	}
	if v := os.Getenv("ROOTFS_PATH"); v != "" {
		cfg.RootfsPath = v
	}
	if v := os.Getenv("VM_DATA_DIR"); v != "" {
		cfg.VmDataDir = v
	}
	if v := os.Getenv("BOOT_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.BootTimeoutSeconds = n
		}
	}
}
