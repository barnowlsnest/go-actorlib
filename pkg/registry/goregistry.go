package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	
	"github.com/barnowlsnest/go-actorlib/pkg/actor"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
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
	
	GoRegistryOption func(*GoRegistry) *GoRegistry
	
	GoRegistry struct {
		tag          string
		startTimeout time.Duration
		stopTimeout  time.Duration
		mu           sync.Mutex
		actors       map[uuid.UUID]*runnableModelEntry
		lookupTable  map[string]uuid.UUID
	}
	
	Model struct {
		ID    uuid.UUID
		State uint64
		Name  string
	}
	
	runnableModelEntry struct {
		name  string
		actor Runnable
	}
)

func WithStartTimeout(timeout time.Duration) GoRegistryOption {
	return func(container *GoRegistry) *GoRegistry {
		container.startTimeout = timeout
		return container
	}
}

func WithStopTimeout(timeout time.Duration) GoRegistryOption {
	return func(container *GoRegistry) *GoRegistry {
		container.stopTimeout = timeout
		return container
	}
}

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

func (gr *GoRegistry) Tag() string {
	return gr.tag
}

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
	case actor.Stopping, actor.Done, actor.StoppedWithError, actor.Cancelled, actor.Panicked:
		return nil, ErrActorIsNotRunnable
	default: // Unknown state
		return nil, ErrUnknownActorState
	}
	
	return runnableActor, nil
}

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
	case actor.Stopping, actor.Done, actor.StoppedWithError, actor.Cancelled, actor.Panicked:
		return nil, ErrActorAlreadyStopped
	default: // Unknown state
		return nil, ErrUnknownActorState
	}
	
	return stoppableActor, nil
}

func (gr *GoRegistry) Start(ctx context.Context, name string) error {
	gr.mu.Lock()
	runnableActor, err := gr.getRunnable(name)
	if err != nil {
		gr.mu.Unlock()
		return errors.Join(fmt.Errorf("failed to start actor: '%s'", name), err)
	}
	gr.mu.Unlock()
	
	if err = runnableActor.Start(ctx); err != nil {
		return err
	}
	
	if errReady := runnableActor.WaitReady(ctx, gr.startTimeout); errReady != nil {
		return errors.Join(ErrActorIsNotRunnable, errReady)
	}
	
	return nil
}

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
		if err := ra.Start(ctx); err != nil {
			return err
		}
		
		runnableActor := ra // capture per-iteration variable to avoid closure capture issue
		errGroup.Go(func() error {
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
	if err = stoppableActor.Stop(gr.stopTimeout); err != nil {
		return err
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
	case actor.Done, actor.StoppedWithError, actor.Cancelled, actor.Panicked:
		delete(gr.lookupTable, name)
		delete(gr.actors, id)
		return nil
	case actor.Initialized, actor.Started, actor.Stopping:
		return errors.Join(fmt.Errorf("failed to unregister actor: '%s'", name), ErrActorIsNotStopped)
	default:
		return ErrUnknownActorState
	}
}

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
