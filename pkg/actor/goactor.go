// Package actor provides a generic actor implementation following the Actor Model pattern.
//
// This package implements asynchronous message processing using Go's goroutines and channels.
// Actors are lightweight, concurrent entities that:
//   - Process messages sequentially in isolation
//   - Maintain their own state through entities
//   - Communicate only through message passing
//   - Support graceful lifecycle management
//
// The core components are:
//   - GoActor[T]: The main actor implementation that processes Executable commands
//   - Entity: Interface for actor-managed data structures
//   - Executable[T]: Interface for commands that can be executed by actors
//   - Hooks: Lifecycle event handlers for monitoring and extending actor behavior
//
// Example usage:
//
//	type MyEntity struct{ value int }
//	func (e *MyEntity) IsProvidable() bool { return true }
//
//	type MyProvider struct{ entity *MyEntity }
//	func (p *MyProvider) Provide() *MyEntity { return p.entity }
//
//	actor, err := actor.New(
//		actor.WithProvider(&MyProvider{&MyEntity{42}}),
//		actor.WithInputBufferSize(10),
//	)
//	if err != nil {
//		panic(err)
//	}
//
//	ctx := context.Background()
//	if err := actor.Start(ctx); err != nil {
//		panic(err)
//	}
package actor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"
)

// Actor states define the lifecycle stages of an actor.
// These constants represent the possible states an actor can be in during its lifetime.
const (
	// Initialized indicates the actor has been created but not yet started.
	// This is the initial state for all newly created actors.
	Initialized = iota

	// Started indicates the actor is actively processing messages.
	// The actor's goroutine is running and ready to handle commands.
	Started

	// Stopping indicates the actor is in the process of shutting down.
	// No new messages will be accepted, but existing ones may still be processed.
	Stopping

	// Done indicates the actor has completed shutdown successfully.
	// This is a terminal state indicating clean termination.
	Done

	// StoppedWithError indicates the actor stopped due to an error.
	// This is a terminal state indicating abnormal termination.
	StoppedWithError

	// Cancelled indicates the actor was stopped due to context cancellation.
	// This is a terminal state indicating cancellation-based termination.
	Cancelled

	// Panicked indicates the actor stopped due to a recovered panic.
	// This is a terminal state indicating panic-based termination.
	Panicked
)

type (
	// Entity defines the base interface for all data structures managed by actors.
	//
	// Entities must be able to indicate their readiness for use through the
	// IsProvidable method.
	Entity interface {
		// IsProvidable returns true if the entity is ready for use.
		// This method is called during actor initialization to ensure
		// the entity is in a valid state.
		IsProvidable() bool
	}

	// Executable defines the interface for commands that actors can execute.
	//
	// This generic interface ensures type safety by binding commands to specific
	// entity types. The actor system executes commands implementing this interface asynchronously.
	Executable[T Entity] interface {
		// Execute performs the command operation on the provided entity.
		// The context can be used for cancellation and timeout control.
		Execute(context.Context, T)
	}

	// Hooks defines lifecycle event handlers for actors.
	//
	// These methods are called at specific points in an actor's lifecycle,
	// allowing for custom monitoring, logging, or cleanup operations.
	Hooks interface {
		// AfterStart is called after the actor has successfully started
		// and is ready to process messages.
		AfterStart()

		// AfterStop is called after the actor has completed shutdown,
		// regardless of whether it was successful or due to an error.
		AfterStop()

		// BeforeStart is called before the actor begins processing messages.
		// The entity parameter is the actor's managed entity.
		BeforeStart(entity any)

		// BeforeStop is called when the actor is about to stop processing messages.
		// The entity parameter is the actor's managed entity.
		BeforeStop(entity any)

		// OnError is called when the actor encounters an error during processing.
		// This includes context cancellation, timeouts, panics, and other failures.
		OnError(err error)
	}

	// EntityProvider defines the interface for providing entities to actors.
	//
	// This generic interface allows actors to obtain their managed entities
	// in a type-safe manner during initialization.
	EntityProvider[T Entity] interface {
		// Provide returns the entity instance that the actor will manage.
		// This method is called once during actor startup.
		Provide() T
	}

	// GoActor is a generic actor implementation that processes commands asynchronously.
	//
	// GoActor manages an entity of type T and processes Executable[T] commands
	// in a dedicated goroutine. It provides thread-safe state management,
	// configurable timeouts, and lifecycle hooks for monitoring.
	//
	// The actor follows the Actor Model pattern:
	//   - Sequential message processing (no concurrent access to entity)
	//   - Isolated state (entity is only accessible within the actor's goroutine)
	//   - Asynchronous communication through message passing
	//   - Supervision and error handling through hooks
	GoActor[T Entity] struct {
		receiveTimeout time.Duration
		inputBufSize   uint64
		state          uint64

		input chan Executable[T]
		done  chan struct{}
		ready chan struct{}

		hooks    Hooks
		provider EntityProvider[T]
	}

	// GoActorOption defines a function type for configuring actors during creation.
	//
	// Options follow the functional options pattern, allowing flexible
	// and extensible actor configuration.
	GoActorOption[T Entity] func(*GoActor[T]) *GoActor[T]
)

// New creates a new GoActor with the specified options.
//
// The actor is created in the Initialized state and must be started
// using the Start method before it can process messages.
//
// At minimum, a provider must be specified using WithProvider.
// If no hooks are provided, a no-op implementation is used.
//
// Returns an error if the provider is nil or if any configuration is invalid.
func New[T Entity](opts ...GoActorOption[T]) (*GoActor[T], error) {
	actor := &GoActor[T]{
		inputBufSize:   1,               // default input buffer size
		receiveTimeout: 5 * time.Second, // default receive timeout
	}

	for _, opt := range opts {
		actor = opt(actor)
	}

	if actor.provider == nil {
		return nil, ErrActorNilProvider
	}

	if actor.hooks == nil {
		actor.hooks = &noopHooks{}
	}

	actor.input = make(chan Executable[T], actor.inputBufSize)
	actor.done = make(chan struct{})
	actor.ready = make(chan struct{})
	actor.state = Initialized

	return actor, nil
}

// State returns the current state of the actor.
//
// This method is thread-safe and can be called from any goroutine.
// The returned value corresponds to one of the state constants
// (Initialized, Started, Stopping, etc.).
func (ga *GoActor[T]) State() uint64 {
	return atomic.LoadUint64(&ga.state)
}

// CheckState verifies that the actor is in the expected state.
//
// Returns an error if the actor's current state does not match
// the expected state. This is useful for ensuring state preconditions
// before performing operations.
func (ga *GoActor[T]) CheckState(state uint64) error {
	if atomic.LoadUint64(&ga.state) != state {
		return fmt.Errorf("actor state mismatch: expected %d, got %d", state, atomic.LoadUint64(&ga.state))
	}
	return nil
}

// InputBufferSize returns the size of the actor's input message buffer.
//
// This represents the maximum number of messages that can be queued
// for processing before senders will block or timeout.
func (ga *GoActor[T]) InputBufferSize() int {
	return int(ga.inputBufSize)
}

// WaitReady blocks until the actor is ready to process messages or times out.
//
// This method should be called after Start() to ensure the actor has
// completed initialization and is ready to receive commands.
//
// Returns an error if the context is cancelled, the timeout is exceeded,
// or the actor fails to start properly.
func (ga *GoActor[T]) WaitReady(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ga.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

func (ga *GoActor[T]) processExecutable(ctx context.Context, e Executable[T]) error {
	select {
	case ga.input <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ga *GoActor[T]) processExecutableWithTimeout(ctx context.Context, e Executable[T], t time.Duration) error {
	timer := time.NewTimer(t)
	defer timer.Stop()
	select {
	case ga.input <- e:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrActorReceiveTimeout
	}
}

func (ga *GoActor[T]) handleCtxErr(err error) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, context.Canceled):
		atomic.StoreUint64(&ga.state, Cancelled)
		ga.hooks.OnError(ErrActorContextCanceled)
	case errors.Is(err, context.DeadlineExceeded):
		atomic.StoreUint64(&ga.state, StoppedWithError)
		ga.hooks.OnError(ErrActorContextDeadlineExceeded)
	default:
		atomic.StoreUint64(&ga.state, StoppedWithError)
		ga.hooks.OnError(err)
	}
}

// Receive queues a command for execution by the actor.
//
// The command will be processed asynchronously by the actor's goroutine.
// If a receive timeout is configured, the operation will fail if the
// message cannot be queued within that timeframe.
//
// Returns an error if:
//   - The executable is nil
//   - The actor is not in Started state
//   - The context is cancelled
//   - The receive timeout is exceeded
func (ga *GoActor[T]) Receive(ctx context.Context, e Executable[T]) error {
	if e == nil {
		return ErrActorReceiveNil
	}

	if atomic.LoadUint64(&ga.state) > 1 {
		return ErrActorReceiveOnStopped
	}

	t := ga.receiveTimeout

	if t == 0 {
		return ga.processExecutable(ctx, e)
	}

	return ga.processExecutableWithTimeout(ctx, e, t)
}

// Start begins the actor's message processing loop in a new goroutine.
//
// The actor will remain active until the context is cancelled or Stop is called.
// The provided entity must be valid (non-nil and IsProvidable() returns true).
//
// Lifecycle hooks are called in this order:
//  1. BeforeStart (synchronously, before goroutine starts)
//  2. AfterStart (asynchronously, after goroutine starts)
//  3. BeforeStop (when stopping)
//  4. AfterStop (after stopped)
//
// Returns an error if the entity is nil, not providable, or if startup fails.
func (ga *GoActor[T]) Start(ctx context.Context) error {
	entity := ga.provider.Provide()

	// Check if the entity is nil using reflection
	// We can't use any(entity) == nil because it doesn't work with nil pointers converted to interfaces
	rv := reflect.ValueOf(entity)
	if !rv.IsValid() || (rv.Kind() == reflect.Ptr && rv.IsNil()) {
		return ErrActorNilEntity
	}

	if !entity.IsProvidable() {
		return ErrActorNilEntity
	}

	func(e T) {
		defer ga.catchPanic()
		ga.hooks.BeforeStart(e)
	}(entity)

	go func(ctx context.Context, entity T) {
		defer close(ga.done)

		atomic.StoreUint64(&ga.state, Started)
		close(ga.ready) // Signal that the actor is ready to receive messages

		func() {
			defer ga.catchPanic()
			ga.hooks.AfterStart()
		}()

		for {
			select {
			case <-ctx.Done():
				ga.handleCtxErr(ctx.Err())
				return
			case e, ok := <-ga.input:
				if !ok {
					func(e T) {
						defer ga.catchPanic()
						ga.hooks.BeforeStop(e)
					}(entity)

					return
				}

				select {
				case <-ctx.Done():
					ga.handleCtxErr(ctx.Err())
					return
				default:
					func() {
						defer ga.catchPanic()
						e.Execute(ctx, entity)
					}()
				}
			}
		}
	}(ctx, entity)
	return nil
}

// Stop gracefully shuts down the actor within the specified timeout.
//
// The actor will finish processing any currently executing command,
// then stop accepting new messages. If the shutdown does not complete
// within the timeout, the actor state will be set to StoppedWithError.
//
// This method blocks until shutdown completes or times out.
// It can only be called on actors in the Started state.
//
// Returns an error if the actor is not in the correct state for stopping.
func (ga *GoActor[T]) Stop(timeout time.Duration) error {
	if err := ga.CheckState(Started); err != nil {
		return err
	}

	close(ga.input)
	atomic.StoreUint64(&ga.state, Stopping)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ga.done:
		atomic.StoreUint64(&ga.state, Done)
	case <-timer.C:
		atomic.StoreUint64(&ga.state, StoppedWithError)
		ga.hooks.OnError(ErrActorGracefulStopTimeout)
	}

	func() {
		defer ga.catchPanic()
		ga.hooks.AfterStop()
	}()

	return nil
}

// WithInputBufferSize configures the size of the actor's input message buffer.
//
// A larger buffer allows more messages to be queued before senders block,
// but uses more memory. The default buffer size is 1.
//
// A buffer size of 0 creates an unbuffered channel, which means senders
// will block until the actor is ready to process the message.
func WithInputBufferSize[T Entity](inputBufSize uint64) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.inputBufSize = inputBufSize
		return actor
	}
}

// WithReceiveTimeout configures how long senders will wait to queue messages.
//
// If set to 0, the Receive method will block indefinitely until the message
// can be queued or the context is cancelled.
//
// If set to a positive duration, Receive will return an error if the message
// cannot be queued within that timeframe. The default timeout is 5 seconds.
func WithReceiveTimeout[T Entity](timeout time.Duration) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.receiveTimeout = timeout
		return actor
	}
}

// WithProvider configures the entity provider for the actor.
//
// The provider is responsible for supplying the entity instance that
// the actor will manage. This option is required - actors cannot be
// created without a provider.
//
// The provider's Provide() method is called once during actor startup.
func WithProvider[T Entity](provider EntityProvider[T]) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.provider = provider
		return actor
	}
}

// WithHooks configures lifecycle event handlers for the actor.
//
// Hooks allow monitoring and extending actor behavior at key lifecycle
// points. If no hooks are provided, a no-op implementation is used.
//
// All hook methods should be non-blocking and handle errors gracefully,
// as they run within the actor's critical execution paths.
func WithHooks[T Entity](hooks Hooks) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.hooks = hooks
		return actor
	}
}

// catchPanic recovers from panics that occur during hook execution or command processing.
//
// When a panic is recovered, the actor state is set to Panicked and the error
// is reported through the OnError hook. Different panic types are handled
// appropriately to provide meaningful error information.
func (ga *GoActor[T]) catchPanic() {
	if err := recover(); err != nil {
		atomic.StoreUint64(&ga.state, Panicked)
		switch e := err.(type) {
		case error:
			ga.hooks.OnError(errors.Join(
				ErrActorPanic,
				e,
			))
		case string:
			ga.hooks.OnError(errors.Join(
				ErrActorPanic,
				fmt.Errorf("string panic: %s", e),
			))
		case int:
			ga.hooks.OnError(errors.Join(
				ErrActorPanic,
				fmt.Errorf("int panic: %d", e),
			))
		default:
			ga.hooks.OnError(errors.Join(
				ErrActorPanic,
				fmt.Errorf("unknown panic type: %v", e),
			))
		}
	}
}
