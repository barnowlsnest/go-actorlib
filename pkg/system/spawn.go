package system

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v3/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v3/pkg/actorref"
)

// Spawn creates, starts, and registers an actor in the system in one operation.
// It combines actor.New, actor.Start, actor.WaitReady, actorref.New, and Register
// into a single convenience function.
//
// The actor is configured with WithName automatically using the provided name.
//
// Returns the actor reference or an error if any step fails.
// On failure, the system state is unchanged (no partial registration).
func Spawn[T actor.Entity](
	s *ActorSystem,
	ctx context.Context,
	name string,
	provider actor.EntityProvider[T],
	readyTimeout time.Duration,
	opts ...actor.GoActorOption[T],
) (*actorref.Ref[T], error) {
	allOpts := make([]actor.GoActorOption[T], 0, len(opts)+2)
	allOpts = append(allOpts, actor.WithProvider(provider), actor.WithName[T](name))
	allOpts = append(allOpts, opts...)

	a, err := actor.New(allOpts...)
	if err != nil {
		return nil, err
	}

	if startErr := a.Start(ctx); startErr != nil {
		return nil, startErr
	}

	if readyErr := a.WaitReady(ctx, readyTimeout); readyErr != nil {
		return nil, readyErr
	}

	ref, err := actorref.New(a)
	if err != nil {
		return nil, err
	}

	if regErr := Register(s, name, ref); regErr != nil {
		// Best effort: stop the actor since we can't register it
		_ = ref.Stop(readyTimeout)
		return nil, regErr
	}

	// Emit event
	s.emitEvent(Event{Kind: EventActorStarted, ActorName: name})

	return ref, nil
}
