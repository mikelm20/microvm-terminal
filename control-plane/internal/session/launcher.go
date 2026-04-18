package session

import "context"

// Launcher is the abstraction the session Manager uses to boot a VM. Production
// code uses the Firecracker-backed implementation (manager.go Create path);
// tests use a mock that returns an empty VM handle instantly or after a delay.
//
// Splitting this out lets the warm-pool + HTTP layer be exercised without a
// real hypervisor.
type Launcher interface {
	Launch(ctx context.Context) (*Session, error)
}

// LauncherFunc adapts a function into a Launcher.
type LauncherFunc func(ctx context.Context) (*Session, error)

func (f LauncherFunc) Launch(ctx context.Context) (*Session, error) { return f(ctx) }
