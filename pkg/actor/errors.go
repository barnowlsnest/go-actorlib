package actor

import (
	"errors"
)

var (
	ErrActorPanic = errors.New("actor panic")

	ErrActorNilProvider = errors.New("entity provider should not be nil")

	ErrActorNilEntity = errors.New("entity provider returned nil entity")

	ErrActorGracefulStopTimeout = errors.New("actor graceful stop timeout")

	ErrActorReceiveTimeout = errors.New("actor receive timeout")

	ErrActorContextCanceled = errors.New("actor context err")

	ErrActorContextDeadlineExceeded = errors.New("actor context deadline exceeded")

	ErrActorReceiveNil = errors.New("actor receive nil command")

	ErrActorReceiveOnStopped = errors.New("actor receive on stopped actor")

	ErrInvalidActorReceiveTimeout = errors.New("receive timeout must be positive number")

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
