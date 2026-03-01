package actor

// BehaviorStack manages a stack of handler functions for dynamic behavior changes.
// It is used internally by GoActor to implement Become/Unbecome semantics.
// Only accessed from within the actor's goroutine, so no synchronization is needed.
type BehaviorStack[T Entity] struct {
	stack []HandlerFunc[T]
}

// newBehaviorStack creates a new BehaviorStack with the given initial handler.
func newBehaviorStack[T Entity](initial HandlerFunc[T]) *BehaviorStack[T] {
	return &BehaviorStack[T]{
		stack: []HandlerFunc[T]{initial},
	}
}

// Current returns the handler at the top of the stack.
func (b *BehaviorStack[T]) Current() HandlerFunc[T] {
	return b.stack[len(b.stack)-1]
}

// Become pushes a new handler onto the stack, making it the current behavior.
// The previous behavior is preserved and can be restored via Unbecome.
func (b *BehaviorStack[T]) Become(handler HandlerFunc[T]) {
	b.stack = append(b.stack, handler)
}

// BecomeReplace replaces the current top of the stack with the new handler.
// Unlike Become, the previous behavior is discarded.
func (b *BehaviorStack[T]) BecomeReplace(handler HandlerFunc[T]) {
	b.stack[len(b.stack)-1] = handler
}

// Unbecome pops the current behavior from the stack, restoring the previous one.
// Returns false if only the initial behavior remains (cannot pop the base handler).
func (b *BehaviorStack[T]) Unbecome() bool {
	if len(b.stack) <= 1 {
		return false
	}
	b.stack = b.stack[:len(b.stack)-1]
	return true
}

// Depth returns the number of behaviors on the stack.
func (b *BehaviorStack[T]) Depth() int {
	return len(b.stack)
}
