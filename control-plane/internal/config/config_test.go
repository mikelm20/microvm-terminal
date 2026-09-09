package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("max_concurrent: 5\nvm:\n  vcpu: 1\n  mem_mib: 1024\njailer:\n  enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConcurrent != 5 || cfg.VM.Vcpu != 1 || cfg.VM.MemMiB != 1024 || cfg.Jailer.Enabled {
		t.Fatalf("file values not applied: %+v", cfg)
	}
	if cfg.ListenAddr != "127.0.0.1:8080" {
		t.Fatalf("default lost: %q", cfg.ListenAddr)
	}
	if got := cfg.JailerCPUQuotaMicros(); got != 100_000 {
		t.Fatalf("derived cpu quota = %d", got)
	}
	if got := cfg.JailerMemBytes(); got != int64(1024+256)<<20 {
		t.Fatalf("derived mem bytes = %d", got)
	}
}

func TestLoadMissingFileAndEnv(t *testing.T) {
	t.Setenv("IDLE_TIMEOUT_SECONDS", "42")
	t.Setenv("USE_MOCK_LAUNCHER", "1")
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IdleTimeoutSeconds != 42 || !cfg.UseMockLauncher {
		t.Fatalf("env override not applied: %+v", cfg)
	}
}

func TestValidate(t *testing.T) {
	t.Setenv("MAX_CONCURRENT", "0")
	if _, err := Load(""); err == nil {
		t.Fatal("expected validation error for max_concurrent 0")
	}
}
