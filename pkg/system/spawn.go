package system

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"
)

// Spawn creates, starts, and registers an actor in the system in one operation.
// It combines [actor.StartNew], [actorref.New], and [Register].
//
// The actor is configured with WithName automatically using the provided name.
//
// Returns the actor reference or an error if any step fails.
// On registration failure, the actor is stopped so the system state is unchanged.
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

	a, err := actor.StartNew(ctx, readyTimeout, allOpts...)
	if err != nil {
		return nil, err
	}

	ref, err := actorref.New(a)
	if err != nil {
		_ = a.Stop(readyTimeout)
		return nil, err
	}

	if regErr := Register(s, name, ref); regErr != nil {
		_ = ref.Stop(readyTimeout)
		return nil, regErr
	}

	s.emitEvent(Event{Kind: EventActorStarted, ActorName: name})

	return ref, nil
}
