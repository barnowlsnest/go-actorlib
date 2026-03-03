// Package middleware provides reference middleware implementations for actors.
//
// These middlewares add cross-cutting concerns like logging, metrics, and recovery
// to actor message processing pipelines.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// Logging returns a middleware that logs each message processed by an actor.
// It logs the start and completion (or error) of each message execution with duration.
//
// Usage:
//
//	actor.New(
//		actor.WithProvider(provider),
//		actor.WithMiddleware(middleware.Logging[*MyEntity](slog.Default())),
//	)
func Logging[T actor.Entity](logger *slog.Logger) actor.Middleware[T] {
	return func(next actor.HandlerFunc[T]) actor.HandlerFunc[T] {
		return func(ctx context.Context, e actor.Executable[T], entity T) {
			actorName := ""
			if ac := actor.GetGoActorContext[T](ctx); ac != nil {
				actorName = ac.Name()
			}

			logger.LogAttrs(ctx, slog.LevelDebug, "actor processing message",
				slog.String("actor", actorName),
			)

			start := time.Now()
			next(ctx, e, entity)
			duration := time.Since(start)

			logger.LogAttrs(ctx, slog.LevelDebug, "actor processed message",
				slog.String("actor", actorName),
				slog.Duration("duration", duration),
			)
		}
	}
}
