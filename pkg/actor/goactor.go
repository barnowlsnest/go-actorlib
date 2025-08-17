package actor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"
)

// actor states
const (
	Initialized = iota
	Started
	Stopping
	Done
	StoppedWithError
	Cancelled
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

	Hooks interface {
		AfterStart()
		AfterStop()
		BeforeStart(entity any)
		BeforeStop(entity any)
		OnError(err error)
	}

	EntityProvider[T Entity] interface {
		Provide() T
	}

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

	GoActorOption[T Entity] func(*GoActor[T]) *GoActor[T]
)

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

func (ga *GoActor[T]) State() uint64 {
	return atomic.LoadUint64(&ga.state)
}

func (ga *GoActor[T]) CheckState(state uint64) error {
	if atomic.LoadUint64(&ga.state) != state {
		return fmt.Errorf("actor state mismatch: expected %d, got %d", state, atomic.LoadUint64(&ga.state))
	}
	return nil
}

func (ga *GoActor[T]) InputBufferSize() int {
	return int(ga.inputBufSize)
}

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

func (ga *GoActor[T]) Start(ctx context.Context) error {
	entity := ga.provider.Provide()

	// Check if entity is nil using reflection
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

func WithInputBufferSize[T Entity](inputBufSize uint64) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.inputBufSize = inputBufSize
		return actor
	}
}

func WithReceiveTimeout[T Entity](timeout time.Duration) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.receiveTimeout = timeout
		return actor
	}
}

func WithProvider[T Entity](provider EntityProvider[T]) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.provider = provider
		return actor
	}
}

func WithHooks[T Entity](hooks Hooks) GoActorOption[T] {
	return func(actor *GoActor[T]) *GoActor[T] {
		actor.hooks = hooks
		return actor
	}
}

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
