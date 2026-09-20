package actorref

import "errors"

// ErrActorRefNilActor is returned by New when the actor pointer is nil.
var ErrActorRefNilActor = errors.New("cannot create actor ref from nil actor")
