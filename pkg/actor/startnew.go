package actor

import (
	"context"
	"time"
)

// StartNew creates, configures, and starts an actor in one call.
// It combines New, Start, and WaitReady into a single convenience function.
//
// The provider option is required and must be included in opts.
//
// Returns the started actor or an error if any step fails.
// If WaitReady fails after a successful Start, the actor is stopped best-effort.
func StartNew[T Entity](ctx context.Context, readyTimeout time.Duration, opts ...GoActorOption[T]) (*GoActor[T], error) {
	a, err := New(opts...)
	if err != nil {
		return nil, err
	}

	if startErr := a.Start(ctx); startErr != nil {
		return nil, startErr
	}

	if readyErr := a.WaitReady(ctx, readyTimeout); readyErr != nil {
		_ = a.Stop(readyTimeout)
		return nil, readyErr
	}

	return a, nil
}
