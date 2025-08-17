package actor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopHooks_AfterStart_ShouldNotPanic(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		hooks.AfterStart()
	})
}

func TestNoopHooks_AfterStop_ShouldNotPanic(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		hooks.AfterStop()
	})
}

func TestNoopHooks_BeforeStart_ShouldNotPanic(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		hooks.BeforeStart("test entity")
	})
	
	assert.NotPanics(t, func() {
		hooks.BeforeStart(nil)
	})
	
	assert.NotPanics(t, func() {
		hooks.BeforeStart(123)
	})
}

func TestNoopHooks_BeforeStop_ShouldNotPanic(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		hooks.BeforeStop("test entity")
	})
	
	assert.NotPanics(t, func() {
		hooks.BeforeStop(nil)
	})
	
	assert.NotPanics(t, func() {
		hooks.BeforeStop(struct{ value int }{42})
	})
}

func TestNoopHooks_OnError_ShouldNotPanic(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Act & Assert - should not panic
	assert.NotPanics(t, func() {
		hooks.OnError(errors.New("test error"))
	})
	
	assert.NotPanics(t, func() {
		hooks.OnError(nil)
	})
}

func TestNoopHooks_ImplementsHooksInterface(t *testing.T) {
	// Arrange
	hooks := &noopHooks{}

	// Assert - should implement Hooks interface
	var _ Hooks = hooks
}