package session

import "context"

// Launcher is the abstraction the Host uses to boot a VM. Production code
// uses the Firecracker-backed Manager through an adapter in main; tests use a
// mock that returns an in-memory Session instantly or after a delay.
type Launcher interface {
	Launch(ctx context.Context) (*Session, error)
}

// LauncherFunc adapts a function into a Launcher.
type LauncherFunc func(ctx context.Context) (*Session, error)

func (f LauncherFunc) Launch(ctx context.Context) (*Session, error) { return f(ctx) }
