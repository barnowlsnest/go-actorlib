// Package registry provides centralized actor lifecycle management for the actor system.
//
// This package implements a registry pattern that manages collections of actors,
// providing operations for registration, lifecycle control, and message dispatching.
// The registry supports concurrent operations and maintains actor state consistency.
//
// Key features:
//   - Thread-safe actor registration and lookup
//   - Coordinated lifecycle management (start/stop operations)
//   - Concurrent actor startup and shutdown with error aggregation
//   - UUID-based and name-based actor identification
//   - State validation and enforcement
//   - Message dispatching to registered actors
//
// The main components are:
//   - GoRegistry: Central registry managing multiple actors
//   - Model: Read-only actor metadata for external consumers
//   - Runnable/Actionable: Interfaces defining actor capabilities
//   - Configuration options for timeouts and behavior
//
// Example usage:
//
//	// Create a registry instance
//	r := registry.New("my-system",
//		registry.WithStartTimeout(10*time.Second),
//		registry.WithStopTimeout(5*time.Second),
//	)
//
//	// Register actors
//	id1, err := r.Register("worker-1", actor1)
//	id2, err := r.Register("worker-2", actor2)
//
//	// Start all actors concurrently
//	err = r.StartAll(ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Dispatch commands
//	cmd := command.New(func(entity *MyEntity) (string, error) {
//		return "processed", nil
//	})
//	err = registry.Dispatch(ctx, id1, cmd)
//
//	// Shutdown
//	err = r.StopAll()
package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/barnowlsnest/go-actorlib/v2/pkg/actor"
)

type (
	// Runnable defines the lifecycle interface for actors.
	//
	// Actors implementing this interface can be started and stopped in a controlled manner.
	// The Start method begins message processing, while Stop provides graceful shutdown
	// with timeout support.
	Runnable interface {
		// Start initiates the actor's message processing loop.
		// The provided context is used for cancellation and should be monitored
		// throughout the actor's lifetime.
		Start(context.Context) error

		// Stop gracefully shuts down the actor within the specified timeout.
		// If the actor cannot stop within the timeout, it may be forcefully terminated.
		Stop(timeout time.Duration) error

		// WaitReady blocks until the actor is ready to process messages.
		// It returns an error if the actor is not ready within the specified timeout.
		WaitReady(ctx context.Context, timeout time.Duration) error

		// State returns the current state of the actor.
		State() uint64
	}

	// Actionable defines the interface for actors that can receive and process commands.
	//
	// This generic interface provides type-safe command queuing, ensuring that
	// only compatible commands can be sent to specific actor types.
	Actionable[TEntity actor.Entity] interface {
		// Receive queues a command for execution by the actor.
		// Returns an error if the command cannot be queued (e.g., buffer full,
		// actor stopped, or invalid command).
		Receive(context.Context, actor.Executable[TEntity]) error
	}

	// GoRegistryOption defines a function type for configuring registries during creation.
	//
	// Options follow the functional options pattern, allowing flexible
	// and extensible registry configuration.
	GoRegistryOption func(*GoRegistry) *GoRegistry

	// GoRegistry is a thread-safe registry that manages multiple actors.
	//
	// GoRegistry provides centralized lifecycle management for collections of actors,
	// including registration, startup coordination, shutdown orchestration, and
	// message dispatching. It maintains both UUID-based and name-based lookups
	// for efficient actor access.
	//
	// The registry enforces actor state consistency and provides concurrent
	// operations with proper error aggregation. All public methods are thread-safe.
	GoRegistry struct {
		tag          string
		startTimeout time.Duration
		stopTimeout  time.Duration
		mu           sync.Mutex
		actors       map[uuid.UUID]*runnableModelEntry
		lookupTable  map[string]uuid.UUID
	}

	// Model represents read-only metadata about a registered actor.
	//
	// Model provides external consumers with safe access to actor information
	// without exposing the underlying actor implementation. The state field
	// reflects the actor's current lifecycle state.
	Model struct {
		ID    uuid.UUID // Unique identifier for the actor
		State uint64    // Current actor state (see actor package constants)
		Name  string    // Human-readable name used for registration
	}

	// runnableModelEntry is an internal structure that pairs actor names with their instances.
	//
	// This structure is used internally by the registry to maintain the relationship
	// between actor names and their corresponding Runnable implementations.
	runnableModelEntry struct {
		name  string
		actor Runnable
	}
)

// WithStartTimeout configures the timeout for actor startup operations.
//
// This timeout applies to the WaitReady phase after starting an actor,
// ensuring that actors become ready within a reasonable timeframe.
// The default start timeout is 5 seconds.
func WithStartTimeout(timeout time.Duration) GoRegistryOption {
	return func(container *GoRegistry) *GoRegistry {
		container.startTimeout = timeout
		return container
	}
}

// WithStopTimeout configures the timeout for actor shutdown operations.
//
// This timeout applies to the graceful stop operation, allowing actors
// to complete their current work before being forcefully terminated.
// The default stop timeout is 5 seconds.
func WithStopTimeout(timeout time.Duration) GoRegistryOption {
	return func(container *GoRegistry) *GoRegistry {
		container.stopTimeout = timeout
		return container
	}
}

// New creates a new GoRegistry with the specified tag and options.
//
// The tag serves as an identifier for the registry instance and can be
// useful for logging, monitoring, or distinguishing between multiple
// registries in the same application.
//
// Default configuration:
//   - Start timeout: 5 seconds
//   - Stop timeout: 5 seconds
func New(tag string, opts ...GoRegistryOption) *GoRegistry {
	container := &GoRegistry{
		tag:          tag,
		startTimeout: 5 * time.Second, // default start timeout
		stopTimeout:  5 * time.Second, // default stop timeout
		actors:       make(map[uuid.UUID]*runnableModelEntry),
		lookupTable:  make(map[string]uuid.UUID),
	}

	for _, opt := range opts {
		container = opt(container)
	}

	return container
}

// mustBeRunnable validates that the provided actor implements Runnable and is in a valid state.
//
// This internal function ensures that actors can be managed by the registry.
// It checks both interface compliance and state validity.
func mustBeRunnable(a any) (Runnable, error) {
	if a == nil {
		return nil, ErrNilActor
	}

	runnableActor, ok := a.(Runnable)
	if !ok {
		return nil, ErrActorIsNotRunnable
	}

	switch runnableActor.State() {
	case actor.Initialized, actor.Started:
		return runnableActor, nil
	case actor.Stopping, actor.Done, actor.StoppedWithError:
		return nil, ErrActorIsNotRunnable
	default:
		return nil, ErrUnknownActorState
	}
}

// mustBeActionable validates that the provided actor implements Actionable.
//
// This internal function ensures that actors can receive and process commands.
// It only checks interface compliance, not state.
func mustBeActionable(a any) (Actionable[actor.Entity], error) {
	if a == nil {
		return nil, ErrNilActor
	}

	actionable, ok := a.(Actionable[actor.Entity])
	if !ok {
		return nil, ErrActorIsNotActionable
	}

	return actionable, nil
}

// Tag returns the registry's identifier tag.
//
// The tag was specified during registry creation and can be used
// for logging, monitoring, or distinguishing between registries.
func (gr *GoRegistry) Tag() string {
	return gr.tag
}

// Register adds an actor to the registry with the specified name.
//
// The actor must implement both Runnable and Actionable interfaces and
// be in a valid state (Initialized or Started). A unique UUID is generated
// for the actor, and both name-based and UUID-based lookups are established.
//
// Returns the generated UUID for the actor, or an error if validation fails
// or the actor cannot be registered.
func (gr *GoRegistry) Register(name string, a any) (uuid.UUID, error) {
	runnableActor, runnableErr := mustBeRunnable(a)
	if runnableErr != nil {
		return uuid.Nil, runnableErr
	}

	_, actionableErr := mustBeActionable(a)
	if actionableErr != nil {
		return uuid.Nil, actionableErr
	}

	gr.mu.Lock()
	id := uuid.New()
	gr.actors[id] = &runnableModelEntry{name, runnableActor}
	gr.lookupTable[name] = id
	gr.mu.Unlock()

	return id, nil
}

// Get retrieves actor metadata by name.
//
// Returns a Model containing the actor's ID, current state, and name.
// The Model provides safe, read-only access to actor information without
// exposing the underlying actor implementation.
//
// Returns an error if no actor is registered with the specified name.
func (gr *GoRegistry) Get(name string) (*Model, error) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	id, exists := gr.lookupTable[name]
	if !exists {
		return nil, ErrActorNotFound
	}

	var ptr *Model
	entry, exists := gr.actors[id]
	if !exists {
		return nil, ErrActorNotFound
	}

	ptr = &Model{
		ID:    id,
		Name:  entry.name,
		State: entry.actor.State(),
	}

	return ptr, nil
}

// GetAll returns metadata for all registered actors.
//
// Returns a slice of Model structs containing current information about
// all actors in the registry. The returned slice is a snapshot and safe
// to iterate over without holding registry locks.
func (gr *GoRegistry) GetAll() []Model {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	stateSlice := make([]Model, 0, len(gr.actors))
	for id, model := range gr.actors {
		stateSlice = append(stateSlice, Model{
			ID:    id,
			Name:  model.name,
			State: model.actor.State(),
		})
	}

	return stateSlice
}

// getRunnable retrieves a Runnable actor by name if it exists and is in a runnable state.
// It returns an error if the actor is not found, in an invalid state, or unknown state.
func (gr *GoRegistry) getRunnable(name string) (Runnable, error) {
	id, exists := gr.lookupTable[name]
	if !exists {
		return nil, ErrActorNotFound
	}

	model, exists := gr.actors[id]
	if !exists || model == nil || model.actor == nil {
		return nil, ErrNilActor
	}

	var runnableActor Runnable
	switch model.actor.State() {
	case actor.Initialized:
		runnableActor = model.actor
	case actor.Started:
		return nil, ErrActorAlreadyStarted
	case actor.Stopping, actor.Done, actor.StoppedWithError, actor.Canceled, actor.Panicked:
		return nil, ErrActorIsNotRunnable
	default: // Unknown state
		return nil, ErrUnknownActorState
	}

	return runnableActor, nil
}

// getStoppable retrieves a Runnable actor by name if it exists and is in a stoppable state.
//
// This internal method validates that an actor can be stopped (must be in Started state).
// It returns an error if the actor is not found, not started, or in an invalid state.
func (gr *GoRegistry) getStoppable(name string) (Runnable, error) {
	id, exists := gr.lookupTable[name]
	if !exists {
		return nil, ErrActorNotFound
	}

	model, exists := gr.actors[id]
	if !exists || model == nil || model.actor == nil {
		return nil, ErrNilActor
	}

	var stoppableActor Runnable
	switch model.actor.State() {
	case actor.Initialized:
		return nil, ErrActorNotStarted
	case actor.Started:
		stoppableActor = model.actor
	case actor.Stopping, actor.Done, actor.StoppedWithError, actor.Canceled, actor.Panicked:
		return nil, ErrActorAlreadyStopped
	default: // Unknown state
		return nil, ErrUnknownActorState
	}

	return stoppableActor, nil
}

// Start begins execution of the actor identified by name.
//
// The actor must be in Initialized state to be started. After starting,
// the method waits for the actor to become ready within the configured
// start timeout.
//
// Returns an error if the actor cannot be started, is in the wrong state,
// or fails to become ready within the timeout.
func (gr *GoRegistry) Start(ctx context.Context, name string) error {
	gr.mu.Lock()
	runnableActor, err := gr.getRunnable(name)
	if err != nil {
		gr.mu.Unlock()
		return errors.Join(fmt.Errorf("failed to start actor: '%s'", name), err)
	}
	gr.mu.Unlock()

	if errStart := runnableActor.Start(ctx); errStart != nil {
		return errStart
	}

	if errReady := runnableActor.WaitReady(ctx, gr.startTimeout); errReady != nil {
		return errors.Join(ErrActorIsNotRunnable, errReady)
	}

	return nil
}

// StartAll concurrently starts all actors that are in a startable state.
//
// This method first identifies all startable actors, then launches them
// concurrently using an error group. Each actor must become ready within
// the configured start timeout. If any actor fails to start, the operation
// continues with other actors, and all errors are collected and returned.
//
// This is the recommended way to start multiple actors efficiently.
func (gr *GoRegistry) StartAll(ctx context.Context) error {
	// First, under lock, determine which actors are runnable and collect any errors.
	gr.mu.Lock()
	var joinErr []error
	var runnableActors []Runnable
	for name := range gr.lookupTable {
		runnableActor, err := gr.getRunnable(name)
		if err != nil {
			runnableErr := errors.Join(fmt.Errorf("failed to start actor: '%s'", name), err)
			joinErr = append(joinErr, runnableErr)
			continue
		}

		runnableActors = append(runnableActors, runnableActor)
	}
	gr.mu.Unlock()

	// Start all runnable actors and wait until they are ready without holding the lock.
	errGroup, groupCtx := errgroup.WithContext(ctx)
	for _, ra := range runnableActors {
		runnableActor := ra // capture per-iteration variable to avoid closure capture issue
		errGroup.Go(func() error {
			if err := runnableActor.Start(groupCtx); err != nil {
				return err
			}
			return runnableActor.WaitReady(groupCtx, gr.startTimeout)
		})
	}

	if errWait := errGroup.Wait(); errWait != nil {
		joinErr = append(joinErr, fmt.Errorf("some actors failed to start: '%s'", errWait.Error()))
	}

	if len(joinErr) > 0 {
		return errors.Join(joinErr...)
	}

	return nil
}

// Stop halts the execution of an actor identified by name and waits for its completion within the configured stop timeout.
// Returns an error if the actor cannot be stopped or is in an inappropriate state.
func (gr *GoRegistry) Stop(name string) error {
	gr.mu.Lock()
	stoppableActor, err := gr.getStoppable(name)
	if err != nil {
		gr.mu.Unlock()
		return errors.Join(fmt.Errorf("failed to stop actor: '%s'", name), err)
	}
	gr.mu.Unlock()
	if errStop := stoppableActor.Stop(gr.stopTimeout); errStop != nil {
		return errStop
	}

	return nil
}

// StopAll gracefully stops all actors that are currently in a stoppable state.
// It attempts to stop each started actor within the configured stop timeout.
// For actors that cannot be stopped (e.g., not started or already stopping/stopped),
// the errors are collected and returned as a joined error.
func (gr *GoRegistry) StopAll() error {
	// Under lock, determine which actors are stoppable and collect errors for the rest.
	gr.mu.Lock()
	var joinErr []error
	var stoppableActors []Runnable
	for name := range gr.lookupTable {
		stoppableActor, err := gr.getStoppable(name)
		if err != nil {
			stopErr := errors.Join(fmt.Errorf("failed to stop actor: '%s'", name), err)
			joinErr = append(joinErr, stopErr)
			continue
		}
		stoppableActors = append(stoppableActors, stoppableActor)
	}
	gr.mu.Unlock()

	// Stop all stoppable actors concurrently and wait for completion.
	var wg sync.WaitGroup
	for _, sa := range stoppableActors {
		wg.Add(1)
		stoppableActor := sa // capture
		go func() {
			defer wg.Done()
			_ = stoppableActor.Stop(gr.stopTimeout)
		}()
	}
	wg.Wait()

	if len(joinErr) > 0 {
		return errors.Join(joinErr...)
	}
	return nil
}

// Unregister removes an actor by name only if it is stopped (terminal state) or stale.
// Stale means the lookup table entry exists but the corresponding actor entry is missing or nil.
// Returns an error if the actor is running or in an invalid state.
func (gr *GoRegistry) Unregister(name string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	id, exists := gr.lookupTable[name]
	if !exists {
		return ErrActorNotFound
	}

	entry, ok := gr.actors[id]
	// Stale entry: actor missing or nil
	if !ok || entry == nil || entry.actor == nil {
		delete(gr.lookupTable, name)
		delete(gr.actors, id)
		return nil
	}

	switch entry.actor.State() {
	case actor.Done, actor.StoppedWithError, actor.Canceled, actor.Panicked:
		delete(gr.lookupTable, name)
		delete(gr.actors, id)
		return nil
	case actor.Initialized, actor.Started, actor.Stopping:
		return errors.Join(fmt.Errorf("failed to unregister actor: '%s'", name), ErrActorIsNotStopped)
	default:
		return ErrUnknownActorState
	}
}

// Dispatch sends a command to the actor identified by UUID.
//
// The command is queued for asynchronous execution by the target actor.
// The actor must implement the Actionable interface and be in a state
// that can accept commands.
//
// Returns an error if the actor is not found, doesn't support commands,
// or cannot accept the command (e.g., buffer full, stopped state).
func (gr *GoRegistry) Dispatch(ctx context.Context, id uuid.UUID, e actor.Executable[actor.Entity]) error {
	gr.mu.Lock()
	entry, exists := gr.actors[id]
	if !exists || entry == nil || entry.actor == nil {
		gr.mu.Unlock()
		return errors.Join(fmt.Errorf("failed to find actor: '%s'", id.String()), ErrActorNotFound)
	}
	actionableActor, err := mustBeActionable(entry.actor)
	gr.mu.Unlock()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to dispatch executable to actor: '%s'", id.String()), err)
	}

	return actionableActor.Receive(ctx, e)
}
