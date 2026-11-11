package actor

import "context"

// contextKey is an unexported type for context value keys to avoid collisions.
type contextKey struct{}

// actorContextKey is the key used to store/retrieve ActorContext from context.Context.
var actorContextKey = contextKey{}

// ActorContext provides access to actor capabilities during message processing.
// It is available to handlers and middleware via the context.Context parameter.
//
// ActorContext is only valid during the processing of a single message —
// it must not be stored or used after the handler returns.
// Only accessed from within the actor's goroutine, so it is not safe for concurrent use.
type ActorContext[T any] struct {
	behavior *BehaviorStack[T]
	name     string
}

// Become pushes a new handler onto the behavior stack, making it the current behavior.
// The previous behavior is preserved and can be restored via Unbecome.
// This should only be called from within a message handler or middleware.
func (ac *ActorContext[T]) Become(handler HandlerFunc[T]) {
	ac.behavior.Become(handler)
}

// BecomeReplace replaces the current behavior with the new handler.
// Unlike Become, the previous behavior is discarded.
func (ac *ActorContext[T]) BecomeReplace(handler HandlerFunc[T]) {
	ac.behavior.BecomeReplace(handler)
}

// Unbecome pops the current behavior from the stack, restoring the previous one.
// Returns false if only the initial behavior remains.
func (ac *ActorContext[T]) Unbecome() bool {
	return ac.behavior.Unbecome()
}

// Name returns the actor's registered name, or empty string if not named.
func (ac *ActorContext[T]) Name() string {
	return ac.name
}

// withActorContext stores a ActorContext in the given context.Context.
func withActorContext[T any](ctx context.Context, ac *ActorContext[T]) context.Context {
	return context.WithValue(ctx, actorContextKey, ac)
}

// GetActorContext retrieves the ActorContext from the context.
// Returns nil if no ActorContext is present.
func GetActorContext[T any](ctx context.Context) *ActorContext[T] {
	ac, _ := ctx.Value(actorContextKey).(*ActorContext[T])
	return ac
}
