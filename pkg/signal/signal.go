// Package signal provides OS signal integration for graceful actor system shutdown.
//
// It listens for SIGTERM and SIGINT signals and triggers a graceful shutdown
// of the actor system when received.
//
// Example usage:
//
//	sys := system.New()
//	// ... register actors ...
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	// Block until signal received, then shut down
//	err := signal.AwaitShutdown(ctx, sys, 10*time.Second)
package signal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Stoppable defines the minimal interface for something that can be stopped.
// Both ActorSystem and Supervisor satisfy this interface.
type Stoppable interface {
	StopAll(timeout time.Duration) error
}

// AwaitShutdown blocks until an OS signal (SIGTERM/SIGINT) is received or the context
// is canceled, then calls StopAll on the provided stoppable.
//
// Returns the error from StopAll, or nil if shutdown was clean.
func AwaitShutdown(ctx context.Context, s Stoppable, timeout time.Duration) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	select {
	case <-sigCh:
		return s.StopAll(timeout)
	case <-ctx.Done():
		return s.StopAll(timeout)
	}
}

// NotifyShutdown returns a channel that receives when an OS signal (SIGTERM/SIGINT) is caught.
// The caller is responsible for calling StopAll. The returned stop function
// deregisters the signal handler.
func NotifyShutdown() (notify <-chan os.Signal, stop func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	return sigCh, func() { signal.Stop(sigCh) }
}
