package actorref

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// Ref is an immutable, lightweight handle for interacting with an actor.
// It acts as a proxy, exposing only send, stop, and state observation — hiding
// lifecycle management methods like Start, WaitReady, and CheckState.
// Safe for concurrent use. Immutability guaranteed by unexported fields.
type Ref[T actor.Entity] struct {
	actor *actor.GoActor[T]
}

// New creates an Ref handle for the given actor.
// Returns an error if the actor is nil.
func New[T actor.Entity](a *actor.GoActor[T]) (*Ref[T], error) {
	if a == nil {
		return nil, ErrActorRefNilActor
	}

	return &Ref[T]{actor: a}, nil
}

// Send delivers a command to the actor for async processing.
// It delegates to the underlying GoActor's Receive method.
func (r *Ref[T]) Send(ctx context.Context, cmd actor.Executable[T]) error {
	return r.actor.Receive(ctx, cmd)
}

// Stop initiates graceful shutdown of the referenced actor.
// It delegates to the underlying GoActor's Stop method.
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
