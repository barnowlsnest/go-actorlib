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
//   - Actor[T]: The main actor implementation that processes Executable commands
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
	"sync"
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

	// Canceled indicates the actor was stopped due to context cancellation.
	// This is a terminal state indicating cancellation-based termination.
	Canceled

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

	// EntityPtr constrains PT to *T and requires *T to implement [Entity].
	//
	// It lets actors be parameterized by the entity's base type (Actor[Counter])
	// while the actor always manages a *T. Constructors take it as a trailing
	// type parameter that is inferred from T, so callers never spell it out.
	EntityPtr[T any] interface {
		*T
		Entity
	}

	// Executable defines the interface for commands that actors can execute.
	//
	// This generic interface ensures type safety by binding commands to specific
	// entity types. The actor system executes commands implementing this interface asynchronously.
	Executable[T any] interface {
		// Execute performs the command operation on the provided entity.
		// The context can be used for cancellation and timeout control.
		Execute(context.Context, *T)
	}

	// Cancelable is an optional interface for executables that must be notified
	// when they are accepted by Receive but never executed.
	//
	// When an actor terminates (Stop, context cancellation, or panic), every
	// command still queued in its mailbox is drained; those implementing
	// Cancelable receive Cancel with [ErrActorCommandDropped]. Executables
	// that do not implement it are discarded silently.
	//
	// Cancel runs on the actor's goroutine before Done is closed, so that a
	// closed Done means every accepted command has been executed or canceled.
	// It must therefore return promptly and must not block on the actor itself
	// (for example by waiting on its Done channel): a blocking Cancel delays
	// termination and makes Stop time out. Calling Receive or Stop on the same
	// actor is safe — they return ErrActorReceiveOnStopped / an error instead
	// of blocking. A panic in Cancel is recovered and reported via OnError;
	// the remaining queued commands are still canceled.
	Cancelable interface {
		Cancel(err error)
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
	EntityProvider[T any] interface {
		// Provide returns the entity instance that the actor will manage.
		// This method is called once during actor startup.
		Provide() *T
	}

	// Actor is a generic actor implementation that processes commands asynchronously.
	//
	// Actor manages an entity of type *T and processes Executable[T] commands
	// in a dedicated goroutine. It provides thread-safe state management,
	// configurable timeouts, and lifecycle hooks for monitoring.
	//
	// The actor follows the Actor Model pattern:
	//   - Sequential message processing (no concurrent access to entity)
	//   - Isolated state (entity is only accessible within the actor's goroutine)
	//   - Asynchronous communication through message passing
	//   - Supervision and error handling through hooks
	//   - Dynamic behavior change via Become/Unbecome
	Actor[T any] struct {
		receiveTimeout time.Duration
		inputBufSize   int
		state          uint64

		input   chan Executable[T]
		done    chan struct{}
		ready   chan struct{}
		stop    chan struct{}
		exiting chan struct{} // closed when the loop exits, before draining

		// sendMu serializes mailbox sends (read lock) against the final drain
		// (write lock) so no accepted command is lost after termination.
		sendMu sync.RWMutex
		closed bool

		starting atomic.Bool // set by the Start call that owns the loop goroutine

		hooks      Hooks
		provider   EntityProvider[T]
		providable func(*T) bool // (*T).IsProvidable, bound by New via EntityPtr
		name       string

		middleware []Middleware[T]
		handler    HandlerFunc[T]
		behavior   *BehaviorStack[T]
		actorCtx   *ActorContext[T]
	}

	// Option defines a function type for configuring actors during creation.
	//
	// Options follow the functional options pattern, allowing flexible
	// and extensible actor configuration.
	Option[T any] func(*Actor[T]) *Actor[T]
)

// defaultInputBufSize is the mailbox capacity when WithInputBufferSize is not set.
// With a capacity of 1, nearly every send waits for the actor to make room,
// so senders and the actor park and wake each other per message. Measured
// end to end with one sender and a trivial handler on an M1 Max: 3.2M msgs/s
// at 1, 8.9M at 16, 12.9M at 256. 16 captures most of the gain while keeping
// backpressure tight and the idle footprint small.
const defaultInputBufSize = 16

// New creates a new Actor with the specified options.
//
// The actor is created in the Initialized state and must be started
// using the Start method before it can process messages.
//
// At minimum, a provider must be specified using WithProvider.
// If no hooks are provided, a no-op implementation is used.
//
// Returns an error if the provider is nil or if any configuration is invalid.
func New[T any, PT EntityPtr[T]](opts ...Option[T]) (*Actor[T], error) {
	actor := &Actor[T]{
		inputBufSize:   defaultInputBufSize,
		receiveTimeout: 5 * time.Second, // default receive timeout
		providable:     func(e *T) bool { return PT(e).IsProvidable() },
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
	actor.stop = make(chan struct{})
	actor.exiting = make(chan struct{})
	actor.state = Initialized

	return actor, nil
}

// State returns the current state of the actor.
//
// This method is thread-safe and can be called from any goroutine.
// The returned value corresponds to one of the state constants
// (Initialized, Started, Stopping, etc.).
func (a *Actor[T]) State() uint64 {
	return atomic.LoadUint64(&a.state)
}

// CheckState verifies that the actor is in the expected state.
//
// Returns an error if the actor's current state does not match
// the expected state. This is useful for ensuring state preconditions
// before performing operations.
func (a *Actor[T]) CheckState(state uint64) error {
	current := atomic.LoadUint64(&a.state)
	if current != state {
		return fmt.Errorf("actor state mismatch: expected %d, got %d", state, current)
	}
	return nil
}

// InputBufferSize returns the size of the actor's input message buffer.
//
// This represents the maximum number of messages that can be queued
// for processing before senders will block or timeout.
func (a *Actor[T]) InputBufferSize() int {
	return a.inputBufSize
}

// WaitReady blocks until the actor is ready to process messages or times out.
//
// This method should be called after Start() to ensure the actor has
// completed initialization and is ready to receive commands.
//
// Returns an error if the context is canceled, the timeout is exceeded,
// or the actor fails to start properly.
func (a *Actor[T]) WaitReady(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-a.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

func (a *Actor[T]) processExecutable(ctx context.Context, e Executable[T]) error {
	select {
	case a.input <- e:
		return nil
	case <-a.stop:
		return ErrActorReceiveOnStopped
	case <-a.exiting:
		return ErrActorReceiveOnStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Actor[T]) processExecutableWithTimeout(ctx context.Context, e Executable[T], t time.Duration) error {
	timer := time.NewTimer(t)
	defer timer.Stop()
	select {
	case a.input <- e:
		return nil
	case <-a.stop:
		return ErrActorReceiveOnStopped
	case <-a.exiting:
		return ErrActorReceiveOnStopped
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrActorReceiveTimeout
	}
}

func (a *Actor[T]) handleCtxErr(err error) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, context.Canceled):
		atomic.StoreUint64(&a.state, Canceled)
		a.hooks.OnError(ErrActorContextCanceled)
	case errors.Is(err, context.DeadlineExceeded):
		atomic.StoreUint64(&a.state, StoppedWithError)
		a.hooks.OnError(ErrActorContextDeadlineExceeded)
	default:
		atomic.StoreUint64(&a.state, StoppedWithError)
		a.hooks.OnError(err)
	}
}

// Receive queues a command for execution by the actor.
//
// The command will be processed asynchronously by the actor's goroutine.
// If a receive timeout is configured, the operation will fail if the
// message cannot be queued within that timeframe.
//
// Returns an error if:
//   - The executable is nil ([ErrActorReceiveNil])
//   - The actor is still Initialized ([ErrActorNotStarted])
//   - The actor has left Started ([ErrActorReceiveOnStopped])
//   - The context is canceled
//   - The receive timeout is exceeded ([ErrActorReceiveTimeout])
func (a *Actor[T]) Receive(ctx context.Context, e Executable[T]) error {
	if e == nil {
		return ErrActorReceiveNil
	}

	switch atomic.LoadUint64(&a.state) {
	case Started:
		// ok
	case Initialized:
		return ErrActorNotStarted
	default:
		return ErrActorReceiveOnStopped
	}

	a.sendMu.RLock()
	defer a.sendMu.RUnlock()

	if a.closed {
		return ErrActorReceiveOnStopped
	}

	// Fail fast on a done ctx. Without this, the fast path below would enqueue
	// whenever the mailbox has room, and the select after it would pick at
	// random between enqueuing and ctx.Done().
	if err := ctx.Err(); err != nil {
		return err
	}

	// Fast path: when the mailbox has room, enqueue without arming a timer.
	// time.NewTimer costs 3 allocs (248 B) and ~225 ns per call on the default
	// 5s timeout, even though it almost never fires
	// (BenchmarkReceive_MailboxHasRoom: 401 ns/3 allocs -> 175 ns/0 allocs).
	// Accepted commands are still covered by sendMu + drain, so a concurrent
	// Stop either executes or cancels this one.
	select {
	case a.input <- e:
		return nil
	default:
	}

	t := a.receiveTimeout

	if t == 0 {
		return a.processExecutable(ctx, e)
	}

	return a.processExecutableWithTimeout(ctx, e, t)
}

// Start begins the actor's message processing loop in a new goroutine.
//
// The actor will remain active until the context is canceled or Stop is called.
// The provided entity must be valid (non-nil and IsProvidable() returns true).
//
// Lifecycle hooks are called in this order:
//  1. BeforeStart (synchronously, before goroutine starts)
//  2. AfterStart (asynchronously, after goroutine starts)
//  3. BeforeStop (when stopping)
//  4. AfterStop (after stopped)
//
// Start succeeds at most once. Concurrent or repeated calls return
// [ErrActorAlreadyStarted]. A Start that fails validation (nil or
// non-providable entity) may be retried.
//
// Returns an error if the entity is nil, not providable, or if startup fails.
func (a *Actor[T]) Start(ctx context.Context) error {
	// Claim the one-shot start slot: state stays Initialized until the loop
	// goroutine runs, so the state alone cannot guard concurrent Start calls.
	if atomic.LoadUint64(&a.state) != Initialized || !a.starting.CompareAndSwap(false, true) {
		return ErrActorAlreadyStarted
	}

	entity := a.provider.Provide()

	if entity == nil || !a.providable(entity) {
		a.starting.Store(false) // allow a retry after a failed Start
		return ErrActorNilEntity
	}

	a.buildHandler()
	a.behavior = newBehaviorStack(a.handler)
	a.actorCtx = &ActorContext[T]{
		behavior: a.behavior,
		name:     a.name,
	}

	func(e *T) {
		defer a.catchPanic()
		a.hooks.BeforeStart(e)
	}(entity)

	go func(ctx context.Context, entity *T) {
		defer close(a.done)
		defer a.drain()

		// Enrich context with actor context for handlers and middleware
		actorScopedCtx := withActorContext(ctx, a.actorCtx)

		atomic.StoreUint64(&a.state, Started)
		close(a.ready) // Signal that the actor is ready to receive messages

		func() {
			defer a.catchPanic()
			a.hooks.AfterStart()
		}()

		if a.panicked() {
			return
		}

		a.loop(ctx, actorScopedCtx, entity)
	}(ctx, entity)
	return nil
}

// loop processes commands sequentially until Stop, ctx cancellation, or a panic.
//
// The per-message path avoids multi-case selects: selectgo locks every channel
// it names, so the old loop (a blocking 3-case select plus a 3-case select with
// default per message) fought senders for the mailbox lock and spent most of
// its CPU spinning in runtime.lock2. Now each message costs one non-blocking
// receive on input plus two non-blocking receives on ctx.Done and stop, which
// are lock-free while those channels are open. The blocking select runs only
// when the mailbox is empty.
func (a *Actor[T]) loop(ctx, actorScopedCtx context.Context, entity *T) {
	ctxDone := ctx.Done() // nil for a non-cancelable ctx: never ready

	for {
		e, ok := a.next(ctx, ctxDone, entity)
		if !ok {
			return
		}

		if a.interrupted(ctx, ctxDone, e, entity) {
			return
		}

		func() {
			defer a.catchPanic()
			a.behavior.Current()(actorScopedCtx, e, entity)
		}()

		// A recovered panic is terminal: exit so Done closes and
		// supervisors observe the failure.
		if a.panicked() {
			return
		}
	}
}

// next returns the next queued command, blocking while the mailbox is empty.
// It returns false once the loop must exit because ctx is done or Stop was called.
func (a *Actor[T]) next(ctx context.Context, ctxDone <-chan struct{}, entity *T) (Executable[T], bool) {
	// Fast path: a single-channel non-blocking receive takes only the mailbox lock.
	select {
	case e := <-a.input:
		return e, true
	default:
	}

	select {
	case <-ctxDone:
		a.handleCtxErr(ctx.Err())
		return nil, false
	case <-a.stop:
		a.beforeStop(entity)
		return nil, false
	case e := <-a.input:
		return e, true
	}
}

// interrupted reports whether ctx is done or Stop was called after e was
// dequeued. If so, it records the terminal state and cancels e instead of
// running it, so commands still queued at Stop are canceled rather than run
// (see the TestDrain_* tests).
func (a *Actor[T]) interrupted(ctx context.Context, ctxDone <-chan struct{}, e Executable[T], entity *T) bool {
	select {
	case <-ctxDone:
		// Record the terminal state first: Cancel may call back into
		// the actor and must see it as no longer Started.
		a.handleCtxErr(ctx.Err())
		a.cancelExecutable(e)
		return true
	default:
	}

	select {
	case <-a.stop:
		a.cancelExecutable(e)
		a.beforeStop(entity)
		return true
	default:
	}

	return false
}

// beforeStop runs the BeforeStop hook, recovering a panic in it.
func (a *Actor[T]) beforeStop(entity *T) {
	defer a.catchPanic()
	a.hooks.BeforeStop(entity)
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
// Returns [ErrActorGracefulStopTimeout] if the actor does not terminate within
// the timeout, or an error wrapping [ErrActorNotRunning] if the actor is not
// in the Started state (never started, already stopping, or terminated).
func (a *Actor[T]) Stop(timeout time.Duration) error {
	if !atomic.CompareAndSwapUint64(&a.state, Started, Stopping) {
		return fmt.Errorf("%w: state %s", ErrActorNotRunning, stateName(atomic.LoadUint64(&a.state)))
	}

	close(a.stop)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var err error

	select {
	case <-a.done:
		atomic.StoreUint64(&a.state, Done)
	case <-timer.C:
		atomic.StoreUint64(&a.state, StoppedWithError)
		a.hooks.OnError(ErrActorGracefulStopTimeout)
		err = ErrActorGracefulStopTimeout
	}

	func() {
		defer a.catchPanic()
		a.hooks.AfterStop()
	}()

	return err
}

// WithInputBufferSize configures the size of the actor's input message buffer.
//
// A larger buffer allows more messages to be queued before senders block,
// but uses more memory. The default buffer size is 16 ([defaultInputBufSize]).
//
// A buffer size of 0 creates an unbuffered channel, which means senders
// will block until the actor is ready to process the message.
func WithInputBufferSize[T any](inputBufSize int) Option[T] {
	return func(actor *Actor[T]) *Actor[T] {
		actor.inputBufSize = inputBufSize
		return actor
	}
}

// WithReceiveTimeout configures how long senders will wait to queue messages.
//
// If set to 0, the Receive method will block indefinitely until the message
// can be queued or the context is canceled.
//
// If set to a positive duration, Receive will return an error if the message
// cannot be queued within that timeframe. The default timeout is 5 seconds.
func WithReceiveTimeout[T any](timeout time.Duration) Option[T] {
	return func(actor *Actor[T]) *Actor[T] {
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
func WithProvider[T any](provider EntityProvider[T]) Option[T] {
	return func(actor *Actor[T]) *Actor[T] {
		actor.provider = provider
		return actor
	}
}

// WithName configures the actor's name.
// The name is available via ActorContext.Name() during message processing.
func WithName[T any](name string) Option[T] {
	return func(actor *Actor[T]) *Actor[T] {
		actor.name = name
		return actor
	}
}

// Name returns the actor's name.
func (a *Actor[T]) Name() string {
	return a.name
}

// Done returns a channel that is closed when the actor's goroutine exits.
// This can be used to wait for the actor to fully terminate.
func (a *Actor[T]) Done() <-chan struct{} {
	return a.done
}

// WithHooks configures lifecycle event handlers for the actor.
//
// Hooks allow monitoring and extending actor behavior at key lifecycle
// points. If no hooks are provided, a no-op implementation is used.
//
// All hook methods should be non-blocking and handle errors gracefully,
// as they run within the actor's critical execution paths.
func WithHooks[T any](hooks Hooks) Option[T] {
	return func(actor *Actor[T]) *Actor[T] {
		actor.hooks = hooks
		return actor
	}
}

// drain runs on the actor goroutine when the loop exits. It wakes blocked senders,
// waits for in-flight sends to finish, rejects further sends, and cancels every
// command left in the mailbox.
func (a *Actor[T]) drain() {
	close(a.exiting)

	a.sendMu.Lock()
	a.closed = true
	a.sendMu.Unlock()

	for {
		select {
		case e := <-a.input:
			a.cancelExecutable(e)
		default:
			return
		}
	}
}

// cancelExecutable notifies a dropped command if it implements Cancelable.
// A panic in Cancel is reported via OnError without changing the actor state.
func (a *Actor[T]) cancelExecutable(e Executable[T]) {
	c, ok := e.(Cancelable)
	if !ok {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			a.hooks.OnError(errors.Join(ErrActorPanic, fmt.Errorf("%v", r)))
		}
	}()

	c.Cancel(ErrActorCommandDropped)
}

// stateName returns a human-readable name for an actor state.
func stateName(state uint64) string {
	switch state {
	case Initialized:
		return "Initialized"
	case Started:
		return "Started"
	case Stopping:
		return "Stopping"
	case Done:
		return "Done"
	case StoppedWithError:
		return "StoppedWithError"
	case Canceled:
		return "Canceled"
	case Panicked:
		return "Panicked"
	default:
		return fmt.Sprintf("Unknown(%d)", state)
	}
}

// panicked reports whether a recovered panic has moved the actor to the Panicked state.
func (a *Actor[T]) panicked() bool {
	return atomic.LoadUint64(&a.state) == Panicked
}

// catchPanic recovers from panics that occur during hook execution or command processing.
//
// When a panic is recovered, the actor state is set to Panicked and the error
// is reported through the OnError hook. Different panic types are handled
// appropriately to provide meaningful error information.
func (a *Actor[T]) catchPanic() {
	if err := recover(); err != nil {
		atomic.StoreUint64(&a.state, Panicked)
		switch e := err.(type) {
		case error:
			a.hooks.OnError(errors.Join(ErrActorPanic, e))
		default:
			a.hooks.OnError(errors.Join(ErrActorPanic, fmt.Errorf("%v", e)))
		}
	}
}
