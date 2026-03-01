package actor

import "context"

// contextKey is an unexported type for context value keys to avoid collisions.
type contextKey struct{}

// goActorContextKey is the key used to store/retrieve GoActorContext from context.Context.
var goActorContextKey = contextKey{}

// GoActorContext provides access to actor capabilities during message processing.
// It is available to handlers and middleware via the context.Context parameter.
//
// GoActorContext is only valid during the processing of a single message —
// it must not be stored or used after the handler returns.
// Only accessed from within the actor's goroutine, so it is not safe for concurrent use.
type GoActorContext[T Entity] struct {
	behavior *BehaviorStack[T]
	name     string
}

// Become pushes a new handler onto the behavior stack, making it the current behavior.
// The previous behavior is preserved and can be restored via Unbecome.
// This should only be called from within a message handler or middleware.
func (ac *GoActorContext[T]) Become(handler HandlerFunc[T]) {
	ac.behavior.Become(handler)
}

// BecomeReplace replaces the current behavior with the new handler.
// Unlike Become, the previous behavior is discarded.
func (ac *GoActorContext[T]) BecomeReplace(handler HandlerFunc[T]) {
	ac.behavior.BecomeReplace(handler)
}

// Unbecome pops the current behavior from the stack, restoring the previous one.
// Returns false if only the initial behavior remains.
func (ac *GoActorContext[T]) Unbecome() bool {
	return ac.behavior.Unbecome()
}

// Name returns the actor's registered name, or empty string if not named.
func (ac *GoActorContext[T]) Name() string {
	return ac.name
}

// WithGoActorContext stores a GoActorContext in the given context.Context.
func WithGoActorContext[T Entity](ctx context.Context, ac *GoActorContext[T]) context.Context {
	return context.WithValue(ctx, goActorContextKey, ac)
}

// GetGoActorContext retrieves the GoActorContext from the context.
// Returns nil if no GoActorContext is present.
func GetGoActorContext[T Entity](ctx context.Context) *GoActorContext[T] {
	ac, _ := ctx.Value(goActorContextKey).(*GoActorContext[T])
	return ac
}
