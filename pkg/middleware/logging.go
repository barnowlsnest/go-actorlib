// Package middleware provides reference middleware implementations for actors.
//
// These middlewares add cross-cutting concerns like logging, metrics, and recovery
// to actor message processing pipelines.
package middleware

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-logslib/v2/pkg/logger"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/actor"
)

// Logging returns a middleware that logs each message processed by an actor.
// It logs the start and completion of each message execution with duration.
//
// Usage:
//
//	log := logger.New(logger.Config{Level: logger.DebugLevel})
//	actor.New(
//		actor.WithProvider(provider),
//		actor.WithMiddleware(middleware.Logging[*MyEntity](log)),
//	)
func Logging[T actor.Entity](log *logger.Logger) actor.Middleware[T] {
	return func(next actor.HandlerFunc[T]) actor.HandlerFunc[T] {
		return func(ctx context.Context, e actor.Executable[T], entity T) {
			actorName := ""
			if ac := actor.GetGoActorContext[T](ctx); ac != nil {
				actorName = ac.Name()
			}

			ctxLog := log.WithContext(ctx)
			ctxLog.Debug("actor processing message",
				logger.StringField("actor", actorName),
			)

			start := time.Now()
			next(ctx, e, entity)
			duration := time.Since(start)

			ctxLog.Debug("actor processed message",
				logger.StringField("actor", actorName),
				logger.DurationField("duration", duration),
			)
		}
	}
}
