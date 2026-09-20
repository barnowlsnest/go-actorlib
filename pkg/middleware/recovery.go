package middleware

import (
	"context"
	"fmt"

	"github.com/barnowlsnest/go-logslib/v2/pkg/logger"

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
//	log := logger.New(logger.Config{Level: logger.ErrorLevel})
//	actor.New(
//		actor.WithProvider(provider),
//		actor.WithMiddleware(middleware.Recovery[*MyEntity](log)),
//	)
func Recovery[T actor.Entity](log *logger.Logger) actor.Middleware[T] {
	return func(next actor.HandlerFunc[T]) actor.HandlerFunc[T] {
		return func(ctx context.Context, e actor.Executable[T], entity T) {
			defer func() {
				if r := recover(); r != nil {
					actorName := ""
					if ac := actor.GetGoActorContext[T](ctx); ac != nil {
						actorName = ac.Name()
					}

					log.WithContext(ctx).Error("actor panic recovered by middleware",
						logger.StringField("actor", actorName),
						logger.StringField("panic", fmt.Sprintf("%v", r)),
					)
				}
			}()

			next(ctx, e, entity)
		}
	}
}
