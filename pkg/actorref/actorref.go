package actorref

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v6/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v6/pkg/ask"
	"github.com/barnowlsnest/go-actorlib/v6/pkg/command"
)

// Ref is an immutable, lightweight handle for interacting with an actor.
// It acts as a proxy, exposing only send, stop, and state observation — hiding
// lifecycle management methods like Start, WaitReady, and CheckState.
// Safe for concurrent use. Immutability guaranteed by unexported fields.
type Ref[T any] struct {
	actor *actor.Actor[T]
}

// New creates a Ref for the given actor.
// Returns ErrActorRefNilActor if a is nil.
func New[T any](a *actor.Actor[T]) (*Ref[T], error) {
	if a == nil {
		return nil, ErrActorRefNilActor
	}

	return &Ref[T]{actor: a}, nil
}

// Send queues a command on the referenced actor.
func (r *Ref[T]) Send(ctx context.Context, cmd actor.Executable[T]) error {
	return r.actor.Receive(ctx, cmd)
}

// Ask sends a request/response command to the referenced actor and waits for
// the result with a timeout.
//
// It creates a [command.Command] from fn, sends it via [Ref.Send], and waits with
// [ask.Await]. The call returns when one of the following happens:
//   - The command completes (successfully or with an error)
//   - The context is canceled
//   - The timeout expires
//
// Returns the command result and nil on success, or the zero value of R and an error
// on failure. Possible errors include:
//   - Actor errors from [Ref.Send] (e.g., actor stopped, receive timeout)
//   - Delegate function errors propagated via [command.Command.Error]
//   - [context.Canceled] or [context.DeadlineExceeded] if ctx is done
//   - [ask.ErrAskTimeout] if the timeout expires before a result is available
func (r *Ref[T]) Ask[R any](ctx context.Context, fn command.DelegateFn[T, R], timeout time.Duration) (R, error) {
	cmd := command.New(fn)

	if err := r.Send(ctx, cmd); err != nil {
		var zero R
		return zero, err
	}

	return ask.Await(ctx, cmd, timeout)
}

// Stop initiates graceful shutdown of the referenced actor.
func (r *Ref[T]) Stop(timeout time.Duration) error {
	return r.actor.Stop(timeout)
}

// State returns the current lifecycle state of the referenced actor.
func (r *Ref[T]) State() uint64 {
	return r.actor.State()
}

// Done returns a channel that is closed when the actor terminates.
func (r *Ref[T]) Done() <-chan struct{} {
	return r.actor.Done()
}
