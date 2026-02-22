package actor

import (
	"errors"
)

// Error variables define the various error conditions that can occur during actor operations.
// These errors provide specific information about failure scenarios and can be used
// with errors.Is() for error handling and classification.
var (
	// ErrActorPanic is returned when an actor encounters a panic during execution.
	// This error is wrapped with the actual panic value and reported through hooks.
	ErrActorPanic = errors.New("actor panic")

	// ErrActorNilProvider is returned when attempting to create an actor without a provider.
	// A valid EntityProvider is required for all actor instances.
	ErrActorNilProvider = errors.New("entity provider should not be nil")

	// ErrActorNilEntity is returned when the entity provider returns a nil or invalid entity.
	// This can occur if the provider's Provide() method returns nil or if IsProvidable() returns false.
	ErrActorNilEntity = errors.New("entity provider returned nil entity")

	// ErrActorGracefulStopTimeout is returned when an actor fails to stop within the specified timeout.
	// This indicates the actor's shutdown process took longer than expected.
	ErrActorGracefulStopTimeout = errors.New("actor graceful stop timeout")

	// ErrActorReceiveTimeout is returned when a message cannot be queued within the receive timeout.
	// This occurs when the actor's input buffer is full and the timeout expires.
	ErrActorReceiveTimeout = errors.New("actor receive timeout")

	// ErrActorContextCanceled is returned when the actor's context is canceled.
	// This typically happens during a graceful shutdown or when the parent context is canceled.
	ErrActorContextCanceled = errors.New("actor context err")

	// ErrActorContextDeadlineExceeded is returned when the actor's context deadline is exceeded.
	// This indicates a timeout occurred during actor operations.
	ErrActorContextDeadlineExceeded = errors.New("actor context deadline exceeded")

	// ErrActorReceiveNil is returned when attempting to send a nil command to an actor.
	// All commands sent to actors must implement the Executable interface.
	ErrActorReceiveNil = errors.New("actor receive nil command")

	// ErrActorReceiveOnStopped is returned when attempting to send commands to a stopped actor.
	// Actors in Stopping, Done, StoppedWithError, Canceled, or Panicked states cannot receive messages.
	ErrActorReceiveOnStopped = errors.New("actor receive on stopped actor")
)
