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

// Guest user baked into the image by vm-image/Dockerfile. Hardcoding the
// uid/gid avoids parsing the guest's /etc/passwd from the host; if the image
// ever changes these values, update here too.
const (
	GuestUser = "dev"
	GuestHome = "/home/" + GuestUser
	guestUID  = 1001
	guestGID  = 1001
)

// PrepareRootfs copies the golden rootfs into dstPath (reflink if supported)
// and writes per-VM state onto the copy: the systemd-networkd unit (so the IP
// is set on boot), the hostname, the Claude Code credentials file and the
// onboarding marker, so the guest shell is already logged in when it starts.
//
// Requires root (mount, loop device).
func PrepareRootfs(goldenPath, dstPath string, ip netip.Addr, prefixLen int, gateway netip.Addr, hostname, oauthToken string) error {
	if err := reflinkCopy(goldenPath, dstPath); err != nil {
		return fmt.Errorf("copy rootfs: %w", err)
	}

	mnt, err := os.MkdirTemp("", "microvm-rootfs-")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.Remove(mnt)

	if err := runCmd("mount", "-o", "loop", dstPath, mnt); err != nil {
		return fmt.Errorf("mount: %w", err)
	}
	unmounted := false
	defer func() {
		if !unmounted {
			_ = runCmd("umount", mnt)
		}
	}()

	// Per-VM systemd-networkd unit.
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

	hnPath := filepath.Join(mnt, "etc/hostname")
	if err := os.WriteFile(hnPath, []byte(hostname+"\n"), 0o644); err != nil {
		return fmt.Errorf("write hostname: %w", err)
	}

	// Claude Code credentials. Interactive `claude` does not read
	// CLAUDE_CODE_OAUTH_TOKEN; it reads ~/.claude/.credentials.json and, if
	// missing, starts an OAuth browser callback that cannot complete in a
	// headless VM. The credentials file is written per VM so the guest is
	// already logged in. The env var is exported too because `claude --print`
	// uses it.
	//
	// The long-lived accessToken is duplicated into refreshToken; sessions
	// are short enough that refresh never fires.
	profPath := filepath.Join(mnt, "etc/profile.d/99-claude-token.sh")
	profContent := fmt.Sprintf("export CLAUDE_CODE_OAUTH_TOKEN=%q\n", oauthToken)
	if err := os.WriteFile(profPath, []byte(profContent), 0o644); err != nil {
		return fmt.Errorf("write profile.d: %w", err)
	}

	home := filepath.Join(mnt, GuestHome)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		return fmt.Errorf("mkdir .claude: %w", err)
	}
	if err := os.Chown(claudeDir, guestUID, guestGID); err != nil {
		return fmt.Errorf("chown .claude: %w", err)
	}

	creds := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  oauthToken,
			"refreshToken": oauthToken,
			"expiresAt":    time.Now().Add(350 * 24 * time.Hour).UnixMilli(),
			"scopes":       []string{"user:inference", "user:profile"},
		},
	}
	if err := writeOwnedJSON(filepath.Join(claudeDir, ".credentials.json"), creds); err != nil {
		return err
	}

	// ~/.claude.json tracks onboarding state. Interactive `claude` shows the
	// "Select login method" prompt unless hasCompletedOnboarding is true,
	// even when credentials.json is valid.
	state := map[string]any{"hasCompletedOnboarding": true}
	if err := writeOwnedJSON(filepath.Join(home, ".claude.json"), state); err != nil {
		return err
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

func writeOwnedJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Chown(path, guestUID, guestGID); err != nil {
		return fmt.Errorf("chown %s: %w", filepath.Base(path), err)
	}
	return nil
}

// reflinkCopy uses `cp --reflink=auto` so the copy is near-instant on
// filesystems that support it; otherwise falls back to a full byte copy.
func reflinkCopy(src, dst string) error {
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
