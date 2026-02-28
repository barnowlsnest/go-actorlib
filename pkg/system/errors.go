package system

import "errors"

// ErrSystemStopped is returned when attempting to perform operations on a stopped actor system.
// After StopAll is called, no further operations are allowed.
var ErrSystemStopped = errors.New("actor system is stopped")

// ErrActorNameEmpty is returned when attempting to register an actor with an empty name.
// All actors must have a non-empty name for identification.
var ErrActorNameEmpty = errors.New("actor name must not be empty")

// ErrActorNameDuplicate is returned when attempting to register an actor with a name
// that is already in use by another actor.
var ErrActorNameDuplicate = errors.New("actor name already registered")

// ErrActorNilRef is returned when attempting to register a nil actor reference.
// A valid actorref.Ref pointer is required for registration.
var ErrActorNilRef = errors.New("actor ref must not be nil")

// ErrActorNotFound is returned when a lookup or dispatch targets an actor name
// that is not registered in the system.
var ErrActorNotFound = errors.New("actor not found")

// ErrCommandTypeMismatch is returned when a dispatched command's type parameter
// does not match the entity type of the target actor.
var ErrCommandTypeMismatch = errors.New("command type does not match actor entity type")

// ErrAskTimeout is returned when the ask operation does not receive a result
// from the command within the specified timeout duration.
var ErrAskTimeout = errors.New("ask timeout waiting for command result")
