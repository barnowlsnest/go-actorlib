package command

import (
	"context"
	"errors"
	"sync"

	"github.com/barnowlsnest/go-actor-lib/pkg/actor"
)

// Command execution states define the lifecycle stages of command processing.
// These constants are used by command implementations to track their progress
// through the execution pipeline.
const (
	// Created indicates the command has been created but not yet started.
	// This is the initial state for all commands.
	Created = iota

	// Started indicates the command is currently executing.
	// The command has been dequeued and is being processed.
	Started

	// Finished indicates the command completed successfully.
	// Any result data has been made available through the command's Result channel.
	Finished

	// Failed indicates the command failed during execution.
	// Error information is available through the command's error reporting methods.
	Failed

	// Canceled indicates the command was canceled before completion.
	// This typically occurs when the execution context is canceled.
	Canceled

	// Panic indicates the command panicked during execution.
	// The panic has been recovered and converted to an error state.
	Panic
)

type (
	// DelegateFn defines a function type for entity operations with return values.
	//
	// This generic function type is used to wrap operations that need to be
	// executed on entities. It provides type safety and error handling for
	// command implementations.
	DelegateFn[T actor.Entity, R any] func(entity T) (R, error)

	GoCommand[E actor.Entity, R any] struct {
		mu         sync.Mutex
		state      int
		done       chan R
		err        error
		delegateFn DelegateFn[E, R]
	}
)

func New[E actor.Entity, R any](fn DelegateFn[E, R]) *GoCommand[E, R] {
	return &GoCommand[E, R]{
		mu:         sync.Mutex{},
		state:      Created,
		done:       make(chan R, 1),
		delegateFn: fn,
	}
}

func (gc *GoCommand[E, R]) syncState(state int, err error) {
	gc.mu.Lock()
	gc.err = err
	gc.state = state
	gc.mu.Unlock()
}

func (gc *GoCommand[E, R]) Done() <-chan R {
	return gc.done
}

func (gc *GoCommand[E, R]) Error() error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.err
}

func (gc *GoCommand[E, R]) State() int {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.state
}

func (gc *GoCommand[E, R]) Execute(ctx context.Context, entity E) {
	gc.syncState(Started, nil)

	select {
	case <-ctx.Done():
		gc.syncState(Canceled, errors.Join(ErrCommandContextCancelled, ctx.Err()))
		close(gc.done)
		return
	default:
		func() {
			defer func() {
				if r := recover(); r != nil {
					gc.syncState(Panic, ErrCommandPanic)
					close(gc.done)
				}
			}()

			val, err := gc.delegateFn(entity)

			if err != nil {
				gc.syncState(Failed, err)
				close(gc.done)
				return
			}

			gc.done <- val
			gc.syncState(Finished, nil)
			close(gc.done)
		}()
		return
	}
}
