// Package firecracker wraps the Firecracker binary: per-VM rootfs preparation,
// process launch, and teardown.
package firecracker

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PrepareRootfs copies the golden rootfs into dstPath (reflink if supported)
// and writes per-VM overrides onto the copy: the systemd-networkd unit (so the
// IP is set on boot), the hostname, and /etc/profile.d/99-learn.sh (which
// exports CLAUDE_CODE_OAUTH_TOKEN into every interactive shell).
//
// Requires root (mount, loop device).
func PrepareRootfs(goldenPath, dstPath string, ip netip.Addr, prefixLen int, gateway netip.Addr, hostname, oauthToken string) error {
	if err := reflinkCopy(goldenPath, dstPath); err != nil {
		return fmt.Errorf("copy rootfs: %w", err)
	}

	mnt, err := os.MkdirTemp("", "learn-rootfs-")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.Remove(mnt)

	if err := runCmd("mount", "-o", "loop", dstPath, mnt); err != nil {
		return fmt.Errorf("mount: %w", err)
	}
	// Best-effort unmount even if writes fail
	unmounted := false
	defer func() {
		if !unmounted {
			_ = runCmd("umount", mnt)
		}
	}()

	// Per-VM systemd-networkd unit
	netPath := filepath.Join(mnt, "etc/systemd/network/10-eth0.network")
	netContent := fmt.Sprintf(`[Match]
Name=eth0

[Network]
Address=%s/%d
Gateway=%s
DNS=1.1.1.1
DNS=1.0.0.1
`, ip.String(), prefixLen, gateway.String())
	if err := os.WriteFile(netPath, []byte(netContent), 0o644); err != nil {
		return fmt.Errorf("write networkd config: %w", err)
	}

	// Hostname
	hnPath := filepath.Join(mnt, "etc/hostname")
	if err := os.WriteFile(hnPath, []byte(hostname+"\n"), 0o644); err != nil {
		return fmt.Errorf("write hostname: %w", err)
	}

	// Claude Code credentials. Interactive `claude` (no --print) does NOT read
	// CLAUDE_CODE_OAUTH_TOKEN; it reads ~/.claude/.credentials.json and, if
	// missing, starts an OAuth browser callback that cannot complete in a
	// headless VM. We bake the credentials file so the learner shell is already
	// logged in. We keep the env var too because `claude --print` uses it.
	//
	// We duplicate the long-lived accessToken into refreshToken; the MVP session
	// is short enough that refresh won't fire. Real refresh plumbing comes with
	// the guest-agent vsock channel delivering per-session credentials.
	profPath := filepath.Join(mnt, "etc/profile.d/99-learn.sh")
	profContent := fmt.Sprintf("export CLAUDE_CODE_OAUTH_TOKEN=%q\n", oauthToken)
	if err := os.WriteFile(profPath, []byte(profContent), 0o644); err != nil {
		return fmt.Errorf("write profile.d/99-learn.sh: %w", err)
	}

	// learner's UID/GID inside the rootfs (see vm-image/Dockerfile). Hardcoding
	// avoids parsing /etc/passwd from inside this code; if the image ever changes
	// these values, update here too.
	const learnerUID, learnerGID = 1001, 1001

	claudeDir := filepath.Join(mnt, "home/learner/.claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		return fmt.Errorf("mkdir .claude: %w", err)
	}
	if err := os.Chown(claudeDir, learnerUID, learnerGID); err != nil {
		return fmt.Errorf("chown .claude: %w", err)
	}

	credsPath := filepath.Join(claudeDir, ".credentials.json")
	expiresMs := time.Now().Add(350 * 24 * time.Hour).UnixMilli()
	creds := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  oauthToken,
			"refreshToken": oauthToken, // see note above
			"expiresAt":    expiresMs,
			"scopes":       []string{"user:inference", "user:profile"},
		},
	}
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal creds: %w", err)
	}
	if err := os.WriteFile(credsPath, credsJSON, 0o600); err != nil {
		return fmt.Errorf("write .credentials.json: %w", err)
	}
	if err := os.Chown(credsPath, learnerUID, learnerGID); err != nil {
		return fmt.Errorf("chown .credentials.json: %w", err)
	}

	// ~/.claude.json tracks onboarding state. Interactive `claude` shows the
	// "Select login method" prompt unless `hasCompletedOnboarding: true` is set
	// here, even when credentials.json is valid. Seeding it skips the prompt.
	claudeStatePath := filepath.Join(mnt, "home/learner/.claude.json")
	state := map[string]any{"hasCompletedOnboarding": true}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(claudeStatePath, stateJSON, 0o600); err != nil {
		return fmt.Errorf("write .claude.json: %w", err)
	}
	if err := os.Chown(claudeStatePath, learnerUID, learnerGID); err != nil {
		return fmt.Errorf("chown .claude.json: %w", err)
	}

	if err := runCmd("sync"); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err := runCmd("umount", mnt); err != nil {
		return fmt.Errorf("umount: %w", err)
	}
	unmounted = true
	return nil
}

// reflinkCopy uses `cp --reflink=auto` so ext4 makes the copy near-instant when
// the filesystem supports it; otherwise falls back to a full byte copy.
func reflinkCopy(src, dst string) error {
	// Remove dst if it exists so cp doesn't complain.
	_ = os.Remove(dst)
	cmd := exec.Command("cp", "--reflink=auto", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cp --reflink=auto: %s: %w", string(out), err)
	}
	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, string(out), err)
	}
	return nil
}
