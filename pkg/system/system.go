// Package system provides a lightweight actor coordinator that tracks actors by name
// and dispatches typed commands to them.
//
// The ActorSystem serves as a central registry for actors, enabling:
//   - Named actor lookup and management
//   - Type-safe command dispatch via generic free functions
//   - Ordered shutdown (last registered = first stopped)
//
// Actors are created, started, and wrapped in actorref.Ref externally. The system
// only manages references — it does not own actor lifecycles beyond shutdown.
//
// Example usage:
//
//	sys := system.New()
//	system.Register(sys, "worker-1", workerRef)
//	system.Send(sys, ctx, "worker-1", myCommand)
//	sys.StopAll(10 * time.Second)
package system

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ManagedActor defines the minimal interface for actors managed by an ActorSystem.
// It exposes only lifecycle operations needed for system-level management.
type ManagedActor interface {
	// Stop initiates graceful shutdown of the actor within the specified timeout.
	Stop(timeout time.Duration) error

	// State returns the current lifecycle state of the actor.
	State() uint64
}

// Option defines a function type for configuring an ActorSystem during creation.
type Option func(*ActorSystem)

// dispatchFn is a type-erased function for dispatching commands to an actor.
// It accepts any command and performs the type assertion internally.
type dispatchFn func(ctx context.Context, cmd any) error

// entry holds a registered actor and its dispatch closure.
type entry struct {
	managed  ManagedActor
	dispatch dispatchFn
}

// ActorSystem is a lightweight coordinator that tracks actors by name
// and provides typed command dispatch.
//
// It is safe for concurrent use from multiple goroutines.
type ActorSystem struct {
	mu      sync.RWMutex
	actors  map[string]entry
	order   []string // registration order for reverse shutdown
	stopped bool
}

// New creates a new ActorSystem with the specified options.
func New(opts ...Option) *ActorSystem {
	s := &ActorSystem{
		actors: make(map[string]entry),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// register is the unexported method that stores the actor and its dispatch closure.
func (s *ActorSystem) register(name string, managed ManagedActor, dispatch dispatchFn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return ErrSystemStopped
	}

	if name == "" {
		return ErrActorNameEmpty
	}

	if managed == nil {
		return ErrActorNilRef
	}

	if _, exists := s.actors[name]; exists {
		return ErrActorNameDuplicate
	}

	s.actors[name] = entry{
		managed:  managed,
		dispatch: dispatch,
	}
	s.order = append(s.order, name)

	return nil
}

// send is the unexported method that looks up an actor and invokes its dispatch closure.
// The lock is released before invoking dispatch so that writers (StopAll, Unregister)
// are not blocked for the duration of the actor send, and actor callbacks that
// re-enter the system cannot deadlock.
func (s *ActorSystem) send(ctx context.Context, name string, cmd any) error {
	s.mu.RLock()

	if s.stopped {
		s.mu.RUnlock()
		return ErrSystemStopped
	}

	e, exists := s.actors[name]
	if !exists {
		s.mu.RUnlock()
		return ErrActorNotFound
	}

	fn := e.dispatch
	s.mu.RUnlock()

	return fn(ctx, cmd)
}

// Get returns the ManagedActor registered under the given name.
// Returns ErrActorNotFound if no actor is registered with that name,
// or ErrSystemStopped if the system has been shut down.
func (s *ActorSystem) Get(name string) (ManagedActor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.stopped {
		return nil, ErrSystemStopped
	}

	e, exists := s.actors[name]
	if !exists {
		return nil, ErrActorNotFound
	}

	return e.managed, nil
}

// Unregister removes an actor from the system by name.
// The actor is NOT stopped — only its registration is removed.
// Returns ErrActorNotFound if the name is not registered,
// or ErrSystemStopped if the system has been shut down.
func (s *ActorSystem) Unregister(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return ErrSystemStopped
	}

	if _, exists := s.actors[name]; !exists {
		return ErrActorNotFound
	}

	delete(s.actors, name)

	// Tombstone the order slot — replace with empty string so reverse shutdown skips it.
	for i, n := range s.order {
		if n == name {
			s.order[i] = ""
			break
		}
	}

	return nil
}

// StopAll stops all registered actors in reverse registration order and marks
// the system as stopped. Errors from individual actor stops are collected
// and returned as a joined error.
//
// After StopAll, no further operations are allowed on the system.
// Calling StopAll on an already-stopped system returns ErrSystemStopped.
//
// The lock is released before stopping actors so that concurrent callers
// of Get, Send, and Register observe ErrSystemStopped immediately rather
// than blocking until every actor has finished shutting down.
func (s *ActorSystem) StopAll(timeout time.Duration) error {
	s.mu.Lock()

	if s.stopped {
		s.mu.Unlock()
		return ErrSystemStopped
	}

	s.stopped = true

	// Snapshot the stop list and clear internal state while holding the lock.
	snapshot := make([]ManagedActor, 0, len(s.actors))

	for i := len(s.order) - 1; i >= 0; i-- {
		name := s.order[i]
		if name == "" {
			continue
		}

		if e, exists := s.actors[name]; exists {
			snapshot = append(snapshot, e.managed)
		}
	}

	s.actors = make(map[string]entry)
	s.order = nil

	s.mu.Unlock()
	
	var errs []error

	for _, managed := range snapshot {
		if err := managed.Stop(timeout); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Count returns the number of currently registered actors.
func (s *ActorSystem) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.actors)
}
