// Package config loads the daemon's runtime configuration from a YAML file
// and applies environment overrides on top.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the whole daemon configuration. Every field has a default, so an
// empty or missing file starts a working daemon (given the host artefacts).
type Config struct {
	// ListenAddr is the HTTP bind address. Keep it on loopback and put Caddy
	// (or any TLS terminator) in front.
	ListenAddr string `yaml:"listen_addr"`

	// PublicOrigin is the scheme+host the API is reachable at. Used to build
	// the WebSocket URL returned by POST /sessions.
	PublicOrigin string `yaml:"public_origin"`

	// DatabaseURL is the Postgres DSN for the sessions table.
	DatabaseURL string `yaml:"database_url"`

	// Login. PasswordFile holds the shared password (one line). The cookie
	// secret is generated on first start if the file is missing.
	PasswordFile     string `yaml:"password_file"`
	CookieSecretFile string `yaml:"cookie_secret_file"`
	CookieDomain     string `yaml:"cookie_domain"`
	SecureCookies    bool   `yaml:"secure_cookies"`

	// Capacity and lifetimes.
	MaxConcurrent      int `yaml:"max_concurrent"`
	IdleTimeoutSeconds int `yaml:"idle_timeout_seconds"`
	BootTimeoutSeconds int `yaml:"boot_timeout_seconds"`
	WarmPoolTarget     int `yaml:"warm_pool_target"`

	// VM is what the guest sees.
	VM VMConfig `yaml:"vm"`

	// Firecracker artefacts and host layout.
	FirecrackerBin string `yaml:"firecracker_bin"`
	JailerBin      string `yaml:"jailer_bin"`
	KernelPath     string `yaml:"kernel_path"`
	RootfsPath     string `yaml:"rootfs_path"`
	VMDataDir      string `yaml:"vm_data_dir"`
	BridgeName     string `yaml:"bridge_name"`
	VMCIDR         string `yaml:"vm_cidr"`

	// ClaudeOAuthTokenFile is read at every session launch and written into
	// the guest as ~/.claude/.credentials.json.
	ClaudeOAuthTokenFile string `yaml:"claude_oauth_token_file"`

	Jailer JailerConfig `yaml:"jailer"`

	// UseMockLauncher disables Firecracker and serves in-memory sessions.
	// For development on machines without KVM and for the API tests.
	UseMockLauncher bool `yaml:"use_mock_launcher"`
}

// VMConfig is the guest-visible machine shape.
type VMConfig struct {
	Vcpu   int `yaml:"vcpu"`
	MemMiB int `yaml:"mem_mib"`
}

// JailerConfig controls the jailer wrapper (chroot, seccomp, cgroup v2).
type JailerConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Script     string `yaml:"script"`
	ChrootBase string `yaml:"chroot_base"`
	// CPUQuotaMicros is cgroup cpu.max per 100000 us period. 0 derives it
	// from vm.vcpu (one full core per vCPU).
	CPUQuotaMicros int64 `yaml:"cpu_quota_us"`
	// MemBytes is cgroup memory.max. 0 derives it from vm.mem_mib plus
	// 256 MiB of Firecracker overhead, so the cgroup never contradicts what
	// the guest was told it has.
	MemBytes       int64  `yaml:"mem_bytes"`
	SeccompProfile string `yaml:"seccomp_profile"`
	// User is the unprivileged system user jailer drops to. UID/GID, when
	// non-zero, skip the getent lookup.
	User string `yaml:"user"`
	UID  int    `yaml:"uid"`
	GID  int    `yaml:"gid"`
}

// Defaults returns the built-in configuration.
func Defaults() Config {
	return Config{
		ListenAddr:           "127.0.0.1:8080",
		PublicOrigin:         "http://localhost:8080",
		DatabaseURL:          "postgres://microvm:microvm@localhost:5432/microvm?sslmode=disable",
		PasswordFile:         "/etc/microvm-terminal/password",
		CookieSecretFile:     "/etc/microvm-terminal/cookie-secret",
		SecureCookies:        true,
		MaxConcurrent:        3,
		IdleTimeoutSeconds:   300,
		BootTimeoutSeconds:   90,
		WarmPoolTarget:       0,
		VM:                   VMConfig{Vcpu: 2, MemMiB: 2048},
		FirecrackerBin:       "/usr/local/bin/firecracker",
		JailerBin:            "/usr/local/bin/jailer",
		KernelPath:           "/var/lib/firecracker/images/vmlinux",
		RootfsPath:           "/var/lib/firecracker/images/rootfs.ext4",
		VMDataDir:            "/var/lib/firecracker/vms",
		BridgeName:           "fc-br0",
		VMCIDR:               "172.20.0.0/24",
		ClaudeOAuthTokenFile: "/etc/microvm-terminal/claude-oauth-token",
		Jailer: JailerConfig{
			Enabled:        true,
			Script:         "/usr/local/libexec/microvm-terminal/jailer.sh",
			ChrootBase:     "/srv/jailer",
			SeccompProfile: "/etc/microvm-terminal/jailer/seccomp.json",
			User:           "microvm",
		},
	}
}

// Load reads path (missing file is fine) and applies environment overrides.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &cfg); err != nil {
				return cfg, fmt.Errorf("parse %s: %w", path, err)
			}
		case !os.IsNotExist(err):
			return cfg, fmt.Errorf("open %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	if c.MaxConcurrent <= 0 {
		return fmt.Errorf("max_concurrent must be positive, got %d", c.MaxConcurrent)
	}
	if c.VM.Vcpu <= 0 || c.VM.MemMiB <= 0 {
		return fmt.Errorf("vm.vcpu and vm.mem_mib must be positive, got %d and %d", c.VM.Vcpu, c.VM.MemMiB)
	}
	if c.BootTimeoutSeconds <= 0 {
		return fmt.Errorf("boot_timeout_seconds must be positive, got %d", c.BootTimeoutSeconds)
	}
	return nil
}

// JailerCPUQuotaMicros resolves the cgroup CPU quota: explicit value, or one
// full core (100000 us per 100000 us period) per configured vCPU.
func (c Config) JailerCPUQuotaMicros() int64 {
	if c.Jailer.CPUQuotaMicros > 0 {
		return c.Jailer.CPUQuotaMicros
	}
	return int64(c.VM.Vcpu) * 100_000
}

// JailerMemBytes resolves cgroup memory.max: explicit value, or guest RAM
// plus 256 MiB for the Firecracker process itself.
func (c Config) JailerMemBytes() int64 {
	if c.Jailer.MemBytes > 0 {
		return c.Jailer.MemBytes
	}
	return int64(c.VM.MemMiB+256) << 20
}

// applyEnv lets a systemd EnvironmentFile or a CI job override the file.
func applyEnv(cfg *Config) {
	str := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	num := func(key string, dst *int) {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	boolean := func(key string, dst *bool) {
		if v := os.Getenv(key); v != "" {
			*dst = v == "true" || v == "1"
		}
	}
	str("LISTEN_ADDR", &cfg.ListenAddr)
	str("PUBLIC_ORIGIN", &cfg.PublicOrigin)
	str("DATABASE_URL", &cfg.DatabaseURL)
	str("PASSWORD_FILE", &cfg.PasswordFile)
	str("COOKIE_SECRET_FILE", &cfg.CookieSecretFile)
	str("COOKIE_DOMAIN", &cfg.CookieDomain)
	boolean("SECURE_COOKIES", &cfg.SecureCookies)
	num("MAX_CONCURRENT", &cfg.MaxConcurrent)
	num("IDLE_TIMEOUT_SECONDS", &cfg.IdleTimeoutSeconds)
	num("BOOT_TIMEOUT_SECONDS", &cfg.BootTimeoutSeconds)
	num("WARM_POOL_TARGET", &cfg.WarmPoolTarget)
	num("VM_VCPU", &cfg.VM.Vcpu)
	num("VM_MEM_MIB", &cfg.VM.MemMiB)
	str("FIRECRACKER_BIN", &cfg.FirecrackerBin)
	str("KERNEL_PATH", &cfg.KernelPath)
	str("ROOTFS_PATH", &cfg.RootfsPath)
	str("VM_DATA_DIR", &cfg.VMDataDir)
	str("CLAUDE_OAUTH_TOKEN_FILE", &cfg.ClaudeOAuthTokenFile)
	boolean("USE_JAILER", &cfg.Jailer.Enabled)
	str("JAILER_SCRIPT", &cfg.Jailer.Script)
	str("JAILER_CHROOT_BASE", &cfg.Jailer.ChrootBase)
	str("JAILER_SECCOMP_PROFILE", &cfg.Jailer.SeccompProfile)
	boolean("USE_MOCK_LAUNCHER", &cfg.UseMockLauncher)
}
