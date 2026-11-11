package system

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v6/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v6/pkg/actorref"
	"github.com/barnowlsnest/go-actorlib/v6/pkg/ask"
	"github.com/barnowlsnest/go-actorlib/v6/pkg/command"
)

// Register adds a typed actor reference to the system under the given name.
// It captures a dispatch closure at registration time that enables type-safe
// command routing via [ActorSystem.Send] and [ActorSystem.Ask].
//
// Returns an error if:
//   - The system is stopped ([ErrSystemStopped])
//   - The name is empty ([ErrActorNameEmpty])
//   - The ref is nil ([ErrActorNilRef])
//   - The name is already registered ([ErrActorNameDuplicate])
func (s *ActorSystem) Register[T any](name string, ref *actorref.Ref[T]) error {
	if ref == nil {
		return ErrActorNilRef
	}

	return s.register(name, ref, func(ctx context.Context, cmd any) error {
		exec, ok := cmd.(actor.Executable[T])
		if !ok {
			return ErrCommandTypeMismatch
		}

		return ref.Send(ctx, exec)
	})
}

// Send dispatches a typed command to the actor registered under the given name.
// The type parameter T must match the entity type the actor was registered with.
//
// Returns an error if:
//   - The system is stopped ([ErrSystemStopped])
//   - The actor name is not found ([ErrActorNotFound])
//   - The command type does not match the actor's entity type ([ErrCommandTypeMismatch])
//   - The underlying actor send fails (e.g., buffer full, actor stopped)
func (s *ActorSystem) Send[T any](ctx context.Context, name string, cmd actor.Executable[T]) error {
	return s.send(ctx, name, cmd)
}

// Ask dispatches a request/response command to the actor registered under the given name
// and waits for the result with a timeout.
//
// It creates a [command.Command] from the provided delegate function, dispatches it
// via the system, and waits for the result. The call returns when one of the following happens:
//   - The command completes (successfully or with an error)
//   - The context is canceled
//   - The timeout expires
//
// Returns the command result and nil on success, or the zero value of R and an error on failure.
// Timeout errors are [ask.ErrAskTimeout] (also available as [ErrAskTimeout]).
func (s *ActorSystem) Ask[T, R any](
	ctx context.Context,
	name string,
	fn command.DelegateFn[T, R],
	timeout time.Duration,
) (R, error) {
	var zero R
	cmd := command.New(fn)

	if err := s.send(ctx, name, cmd); err != nil {
		return zero, err
	}

	return ask.Await(ctx, cmd, timeout)
}
