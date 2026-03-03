package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// Recovery returns a middleware that recovers from panics in downstream handlers.
// When a panic is caught, it logs the error and prevents it from propagating
// to the actor's catch-all panic recovery (which would set the actor to Panicked state).
//
// This middleware should be placed first in the middleware chain to catch all panics.
//
// Usage:
//
//	actor.New(
//		actor.WithProvider(provider),
//		actor.WithMiddleware(middleware.Recovery[*MyEntity](slog.Default())),
//	)
func Recovery[T actor.Entity](logger *slog.Logger) actor.Middleware[T] {
	return func(next actor.HandlerFunc[T]) actor.HandlerFunc[T] {
		return func(ctx context.Context, e actor.Executable[T], entity T) {
			defer func() {
				if r := recover(); r != nil {
					actorName := ""
					if ac := actor.GetGoActorContext[T](ctx); ac != nil {
						actorName = ac.Name()
					}

					logger.LogAttrs(ctx, slog.LevelError, "actor panic recovered by middleware",
						slog.String("actor", actorName),
						slog.String("panic", fmt.Sprintf("%v", r)),
					)
				}
			}()

			next(ctx, e, entity)
		}
	}
}
