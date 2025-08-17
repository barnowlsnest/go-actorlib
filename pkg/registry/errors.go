package registry

import (
	"errors"
)

var (
	ErrActorIsNotRunnable = errors.New("actor is not runnable")

	ErrActorAlreadyStarted = errors.New("actor already started")

	ErrActorAlreadyStopped = errors.New("actor already stopped")

	ErrActorNotStarted = errors.New("actor not started")

	ErrActorIsNotActionable = errors.New("actor is not actionable")

	ErrNilActor = errors.New("actor is nil")

	ErrActorNotFound = errors.New("actor not found")

	ErrUnknownActorState = errors.New("unknown actor state")

	ErrActorIsNotStopped = errors.New("actor is not stopped")
)
