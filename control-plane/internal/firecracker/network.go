package firecracker

import (
	"fmt"
	"net/netip"
	"os"
)

// EnsureBridge creates the host bridge if missing, assigns the host IP, enables
// IP forwarding, and installs NAT rules. Idempotent.
//
// upstreamIface is the interface with the default route (e.g., "eno1"); the
// session manager reads it from `ip route` at startup.
func EnsureBridge(bridgeName string, hostAddr netip.Addr, prefixLen int, subnet netip.Prefix, upstreamIface string) error {
	if err := ensureLink(bridgeName, "bridge"); err != nil {
		return fmt.Errorf("bridge: %w", err)
	}
	addrCIDR := fmt.Sprintf("%s/%d", hostAddr.String(), prefixLen)
	if err := runCmd("ip", "addr", "replace", addrCIDR, "dev", bridgeName); err != nil {
		return fmt.Errorf("bridge addr: %w", err)
	}
	if err := runCmd("ip", "link", "set", bridgeName, "up"); err != nil {
		return fmt.Errorf("bridge up: %w", err)
	}
	// IP forwarding
	_ = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0o644)

	// NAT from subnet out the upstream iface. Skip if already present (`-C`
	// tests; non-zero exit means "not there"). Avoid stacking duplicate rules
	// across restarts.
	if upstreamIface == "" {
		return fmt.Errorf("upstream iface required")
	}
	natCheck := []string{"-t", "nat", "-C", "POSTROUTING", "-s", subnet.String(), "-o", upstreamIface, "-j", "MASQUERADE"}
	if !runCmdSilent("iptables", natCheck...) {
		natAdd := []string{"-t", "nat", "-A", "POSTROUTING", "-s", subnet.String(), "-o", upstreamIface, "-j", "MASQUERADE"}
		if err := runCmd("iptables", natAdd...); err != nil {
			return fmt.Errorf("nat add: %w", err)
		}
	}
	// Forwarding permit rules (also idempotent via -C/-A)
	addIfMissing := func(args ...string) {
		check := append([]string{"-C"}, args...)
		if !runCmdSilent("iptables", check...) {
			_ = runCmd("iptables", append([]string{"-A"}, args...)...)
		}
	}
	addIfMissing("FORWARD", "-i", bridgeName, "-j", "ACCEPT")
	addIfMissing("FORWARD", "-o", bridgeName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	return nil
}

// CreateTAP creates a TAP device owned by root and attaches it to the bridge.
// Safe to call multiple times; returns error only if the final state isn't right.
func CreateTAP(tapName, bridgeName string) error {
	// Create
	if err := runCmd("ip", "tuntap", "add", tapName, "mode", "tap"); err != nil {
		// Already exists? That's fine.
		if !runCmdSilent("ip", "link", "show", tapName) {
			return fmt.Errorf("create tap %s: %w", tapName, err)
		}
	}
	if err := runCmd("ip", "link", "set", tapName, "master", bridgeName); err != nil {
		return fmt.Errorf("attach tap to bridge: %w", err)
	}
	if err := runCmd("ip", "link", "set", tapName, "up"); err != nil {
		return fmt.Errorf("tap up: %w", err)
	}
	return nil
}

// DeleteTAP removes a TAP device. Safe if it doesn't exist.
func DeleteTAP(tapName string) error {
	_ = runCmd("ip", "link", "set", tapName, "down")
	_ = runCmd("ip", "link", "del", tapName)
	return nil
}

func ensureLink(name, kind string) error {
	if runCmdSilent("ip", "link", "show", name) {
		return nil
	}
	return runCmd("ip", "link", "add", "name", name, "type", kind)
}

// runCmdSilent returns true if the command succeeds.
func runCmdSilent(name string, args ...string) bool {
	return runCmd(name, args...) == nil
}
