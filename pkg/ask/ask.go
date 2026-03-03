// Package ask provides a convenience function for request/response interactions with actors.
//
// The Ask pattern collapses the typical verbose process of creating a command,
// sending it to an actor, waiting for a result with a timeout, and checking for errors
// into a single function call.
//
// Example usage:
//
//	result, err := ask.New(ctx, myActorRef, func(entity *MyEntity) (int, error) {
//	    entity.value++
//	    return entity.value, nil
//	}, 5*time.Second)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Result: %d\n", result)
package ask

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"
	"github.com/barnowlsnest/go-actorlib/v4/pkg/command"
)

// New sends a command to the actor and waits for the result with a timeout.
//
// It creates a [command.GoCommand] from the provided delegate function, sends it
// to the actor via [actorref.Ref.Send], and waits for the result. The call
// returns when one of the following happens:
//   - The command completes (successfully or with an error)
//   - The context is canceled
//   - The timeout expires
//
// Returns the command result and nil on success, or the zero value of R and an error
// on failure. Possible errors include:
//   - Actor errors from [actorref.Ref.Send] (e.g., actor stopped, receive timeout)
//   - Delegate function errors propagated via [command.GoCommand.Error]
//   - [context.Canceled] or [context.DeadlineExceeded] if ctx is done
//   - [ErrAskTimeout] if the timeout expires before a result is available
func New[E actor.Entity, R any](
	ctx context.Context,
	ref *actorref.Ref[E],
	fn command.DelegateFn[E, R],
	timeout time.Duration,
) (R, error) {
	var zero R
	cmd := command.New(fn)

	if err := ref.Send(ctx, cmd); err != nil {
		return zero, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result, ok := <-cmd.Done():
		if !ok {
			return zero, cmd.Error()
		}
		return result, cmd.Error()
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, ErrAskTimeout
	}
}
