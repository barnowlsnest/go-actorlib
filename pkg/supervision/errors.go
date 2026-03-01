package supervision

import "errors"

var (
	// ErrMaxRestartsExceeded is returned when the supervisor has exceeded the maximum
	// number of restarts within the configured time window.
	ErrMaxRestartsExceeded = errors.New("supervisor: max restarts exceeded within time window")

	// ErrChildNotFound is returned when a child actor is not found in the supervisor.
	ErrChildNotFound = errors.New("supervisor: child not found")

	// ErrChildNameDuplicate is returned when attempting to add a child with a name
	// that is already registered.
	ErrChildNameDuplicate = errors.New("supervisor: child name already registered")

	// ErrChildNameEmpty is returned when attempting to add a child with an empty name.
	ErrChildNameEmpty = errors.New("supervisor: child name must not be empty")

	// ErrNilChildSpec is returned when a nil ChildSpec is provided.
	ErrNilChildSpec = errors.New("supervisor: child spec must not be nil")

	// ErrSupervisorStopped is returned when attempting operations on a stopped supervisor.
	ErrSupervisorStopped = errors.New("supervisor: already stopped")
)
