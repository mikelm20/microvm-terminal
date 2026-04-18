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
		AuthPasswordFile:     "/etc/learn-platform/auth-password",
		AuthCookieSecretFile: "/etc/learn-platform/auth-cookie-secret",
		DatabaseURL:          "postgres://learn:learn@localhost:5432/learn?sslmode=disable",
		PublicOrigin:         "http://localhost:8080",
		LessonsDir:           "lessons",
		VoiceDir:             "shared/voice",
		WarmPoolTarget:       0,
		SecureCookies:        false,
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
	if v := os.Getenv("USE_MOCK_LAUNCHER"); v != "" {
		cfg.UseMockLauncher = (v == "true" || v == "1")
	}
}
