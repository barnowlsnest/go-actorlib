package middleware

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// Metrics collects message processing statistics for an actor.
// It tracks message count and total processing duration.
// All methods are safe for concurrent access.
type Metrics struct {
	messageCount    atomic.Int64
	totalDurationNs atomic.Int64
}

// MessageCount returns the total number of messages processed.
func (m *Metrics) MessageCount() int64 {
	return m.messageCount.Load()
}

// TotalDuration returns the cumulative processing time for all messages.
func (m *Metrics) TotalDuration() time.Duration {
	return time.Duration(m.totalDurationNs.Load())
}

// AverageDuration returns the average processing time per message.
// Returns 0 if no messages have been processed.
func (m *Metrics) AverageDuration() time.Duration {
	count := m.messageCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.totalDurationNs.Load() / count)
}

// MetricsMiddleware returns a middleware that collects processing metrics.
// The returned Metrics object can be queried concurrently.
//
// Usage:
//
//	metrics := &middleware.Metrics{}
//	actor.New(
//		actor.WithProvider(provider),
//		actor.WithMiddleware(middleware.MetricsMiddleware[*MyEntity](metrics)),
//	)
//	// Later: metrics.MessageCount(), metrics.AverageDuration()
func MetricsMiddleware[T actor.Entity](m *Metrics) actor.Middleware[T] {
	return func(next actor.HandlerFunc[T]) actor.HandlerFunc[T] {
		return func(ctx context.Context, e actor.Executable[T], entity T) {
			start := time.Now()
			next(ctx, e, entity)
			duration := time.Since(start)

			m.messageCount.Add(1)
			m.totalDurationNs.Add(int64(duration))
		}
	}
}
