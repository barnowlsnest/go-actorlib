// Package command provides a generic command implementation for the actor system.
//
// This package implements the Command pattern, allowing operations to be encapsulated
// as objects that can be executed asynchronously by actors. Commands are type-safe,
// support return values, and provide comprehensive error handling.
//
// Key features:
//   - Generic commands that work with specific entity types
//   - Asynchronous execution with result channels
//   - Thread-safe state management
//   - Panic recovery and error propagation
//   - Context cancellation support
//
// The main components are:
//   - GoCommand[E, R]: Generic command that executes operations and returns results
//   - DelegateFn[E, R]: Function type for operations that commands execute
//   - State constants: Track command execution progress
//
// Example usage:
//
//	type MyEntity struct{ value int }
//	func (e *MyEntity) IsProvidable() bool { return true }
//
//	// Create a command that increments the entity's value
//	cmd := command.New(func(entity *MyEntity) (int, error) {
//		entity.value++
//		return entity.value, nil
//	})
//
//	// Send to actor (assuming actor is already started)
//	err := actor.Receive(ctx, cmd)
//	if err != nil {
//		panic(err)
//	}
//
//	// Wait for result
//	select {
//	case result := <-cmd.Done():
//		fmt.Printf("New value: %d\n", result)
//	case <-time.After(5 * time.Second):
//		fmt.Println("Command timed out")
//	}
package command

import (
	"context"
	"errors"
	"sync"

	"github.com/barnowlsnest/go-actorlib/pkg/actor"
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

	// GoCommand is a generic command implementation that executes operations on entities.
	//
	// GoCommand encapsulates an operation (DelegateFn) that can be executed asynchronously
	// by actors. It provides type-safe execution with return values, comprehensive error
	// handling, and thread-safe state management.
	//
	// The command follows this execution flow:
	//   1. Created - Command is instantiated but not executed
	//   2. Started - Command begins execution on the entity
	//   3. Finished/Failed/Canceled/Panic - Terminal states based on execution outcome
	//
	// Results are delivered through a buffered channel accessible via Done(),
	// while errors can be retrieved using Error().
	GoCommand[E actor.Entity, R any] struct {
		mu         sync.Mutex
		state      int
		done       chan R
		err        error
		delegateFn DelegateFn[E, R]
	}
)

// New creates a new GoCommand that will execute the provided delegate function.
//
// The delegate function will be called with the entity when the command is executed
// by an actor. The function should perform the desired operation and return either
// a result of type R or an error.
//
// The command is created in the Created state and remains so until executed.
func New[E actor.Entity, R any](fn DelegateFn[E, R]) *GoCommand[E, R] {
	return &GoCommand[E, R]{
		mu:         sync.Mutex{},
		state:      Created,
		done:       make(chan R, 1),
		delegateFn: fn,
	}
}

// syncState atomically updates the command's state and error information.
//
// This is an internal method used to ensure thread-safe updates to the
// command's state and error fields. It should only be called from within
// the command's execution methods.
func (gc *GoCommand[E, R]) syncState(state int, err error) {
	gc.mu.Lock()
	gc.err = err
	gc.state = state
	gc.mu.Unlock()
}

// Done returns a receive-only channel that will contain the command result.
//
// This channel receives exactly one value when the command completes successfully,
// then is closed. If the command fails, is canceled, or panics, the channel is
// closed without sending a value.
//
// The channel is buffered with size 1, so the result won't be lost if not
// immediately consumed.
func (gc *GoCommand[E, R]) Done() <-chan R {
	return gc.done
}

// Error returns any error that occurred during command execution.
//
// This method is thread-safe and will return:
//   - nil if the command hasn't been executed or completed successfully
//   - The delegate function's error if execution failed
//   - A context error if the command was canceled
//   - ErrCommandPanic if the command panicked during execution
func (gc *GoCommand[E, R]) Error() error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.err
}

// State returns the current execution state of the command.
//
// This method is thread-safe and returns one of the state constants:
// Created, Started, Finished, Failed, Canceled, or Panic.
//
// The state progresses monotonically and never moves backwards.
func (gc *GoCommand[E, R]) State() int {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return gc.state
}

// Execute runs the command's delegate function with the provided entity.
//
// This method implements the actor.Executable interface, allowing commands
// to be sent to actors for execution. The execution process:
//
//  1. Sets state to Started
//  2. Checks for context cancellation
//  3. Calls the delegate function with panic recovery
//  4. Handles the result or error appropriately
//  5. Updates state and closes the result channel
//
// The method handles all error conditions gracefully and ensures the
// result channel is always closed, regardless of outcome.
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
