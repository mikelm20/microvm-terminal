// Package netalloc hands out per-VM IP addresses, MAC addresses, and TAP device
// names on the shared host bridge.
//
// MVP simplification: a single /24 bridge network, VMs take sequential addresses
// starting at .2 (host is .1). Free list is tracked in memory; when a session
// ends, its IP returns to the pool. No persistence across daemon restarts
// (restart implies all VMs were torn down anyway).
package netalloc

import (
	"fmt"
	"net/netip"
	"sync"
)

type Allocator struct {
	mu     sync.Mutex
	base   netip.Prefix // e.g., 172.20.0.0/24
	host   netip.Addr   // reserved for the bridge, not handed out
	next   netip.Addr   // next candidate
	in_use map[netip.Addr]bool
	tapIdx int
	cidIdx uint32 // Firecracker guest vsock CIDs; start at 100, monotonic
}

// New parses cidr (e.g. "172.20.0.0/24") and returns an allocator. The first
// host address in the subnet is reserved as the bridge IP.
func New(cidr string) (*Allocator, error) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", cidr, err)
	}
	if !p.Addr().Is4() {
		return nil, fmt.Errorf("only IPv4 supported for MVP: %q", cidr)
	}
	first := p.Masked().Addr().Next() // .1 within .0/24 etc.
	return &Allocator{
		base:   p,
		host:   first,
		next:   first.Next(),
		in_use: make(map[netip.Addr]bool),
		cidIdx: 99, // first Alloc returns 100
	}, nil
}

// HostAddr is the bridge's address, e.g. 172.20.0.1 for a 172.20.0.0/24.
func (a *Allocator) HostAddr() netip.Addr { return a.host }

// PrefixLen returns the network prefix length (e.g., 24).
func (a *Allocator) PrefixLen() int { return a.base.Bits() }

// Alloc returns a unique (VM IP, MAC, TAP name, guest vsock CID) tuple, or an
// error if the pool is exhausted.
func (a *Allocator) Alloc() (ip netip.Addr, mac string, tap string, cid uint32, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := 0; i < 1<<16; i++ {
		candidate := a.next
		a.next = a.next.Next()
		if !a.base.Contains(candidate) {
			return netip.Addr{}, "", "", 0, fmt.Errorf("no free IPs in %s", a.base)
		}
		// skip broadcast (last in /24) and network address
		if candidate == a.host {
			continue
		}
		if a.in_use[candidate] {
			continue
		}
		a.in_use[candidate] = true

		// Stable MAC derived from the last octet: AA:BB:CC:00:00:<last>
		b := candidate.As4()
		mac = fmt.Sprintf("AA:BB:CC:00:%02x:%02x", b[2], b[3])

		a.tapIdx++
		tap = fmt.Sprintf("tap-fc%d", a.tapIdx)

		a.cidIdx++
		cid = a.cidIdx
		return candidate, mac, tap, cid, nil
	}
	return netip.Addr{}, "", "", 0, fmt.Errorf("alloc loop exhausted")
}

// Release returns an IP to the pool. Idempotent.
func (a *Allocator) Release(ip netip.Addr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.in_use, ip)
}

// InUseCount is for metrics / debugging.
func (a *Allocator) InUseCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.in_use)
}
