package actorref

import "errors"

// ErrActorRefNilActor is returned when attempting to create an ActorRef from a nil actor.
// A valid GoActor pointer is required to create an ActorRef.
var ErrActorRefNilActor = errors.New("cannot create actor ref from nil actor")
