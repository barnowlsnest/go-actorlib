package actor

import "context"

// HandlerFunc is a function that handles command execution within an actor's processing loop.
// It receives the context, the executable command, and the entity the actor manages.
// Middleware wraps this function to inject cross-cutting concerns around command execution.
type HandlerFunc[T any] func(ctx context.Context, executable Executable[T], entity *T)

// Middleware is a function that wraps a HandlerFunc to add behavior before and/or after
// command execution. Middleware follows the same composition pattern as Go HTTP middleware:
// each middleware receives the next handler in the chain and returns a new handler.
type Middleware[T any] func(next HandlerFunc[T]) HandlerFunc[T]

// Chain composes multiple middlewares into a single Middleware by folding right-to-left.
// Given Chain(A, B, C), the resulting execution order on entry is A → B → C → handler.
// An empty chain returns a no-op middleware that passes through to the next handler unchanged.
func Chain[T any](middlewares ...Middleware[T]) Middleware[T] {
	return func(next HandlerFunc[T]) HandlerFunc[T] {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// WithMiddleware configures middleware for the actor's command execution pipeline.
// Multiple calls to WithMiddleware append to the existing middleware slice.
// Middleware is composed once at Start() time, so there is no per-message overhead.
func WithMiddleware[T any](middlewares ...Middleware[T]) GoActorOption[T] {
	return func(actor *Actor[T]) *Actor[T] {
		actor.middleware = append(actor.middleware, middlewares...)
		return actor
	}
}

// buildHandler composes the middleware chain into a single handler function.
// When no middleware is configured, the handler calls Execute directly (zero overhead).
// This method is called once during Start().
func (a *Actor[T]) buildHandler() {
	base := func(ctx context.Context, executable Executable[T], entity *T) {
		executable.Execute(ctx, entity)
	}

	if len(a.middleware) == 0 {
		a.handler = base
		return
	}

	a.handler = Chain[T](a.middleware...)(base)
}
