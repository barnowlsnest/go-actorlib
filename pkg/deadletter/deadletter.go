// Package deadletter provides a dead letter queue for capturing undeliverable messages.
//
// When messages cannot be delivered to their target actor (e.g., the actor has stopped
// or panicked), they can be routed to a dead letter queue for debugging and monitoring.
//
// The Queue is safe for concurrent use from multiple goroutines.
//
// Example usage:
//
//	dlq := deadletter.New(deadletter.WithCapacity(1000))
//	dlq.OnDeadLetter(func(l deadletter.Letter) {
//		log.Printf("dead letter: target=%s reason=%s", l.Target, l.Reason)
//	})
//	dlq.Publish(deadletter.Letter{Target: "worker-1", Reason: "actor stopped"})
package deadletter

import "sync"

// Letter represents an undeliverable message.
type Letter struct {
	// Target is the name of the intended recipient actor.
	Target string

	// Reason describes why the message could not be delivered.
	Reason string
}

// Handler is a callback invoked when a dead letter is published.
type Handler func(Letter)

// Queue collects and notifies about dead letters.
// It is safe for concurrent use.
type Queue struct {
	mu       sync.Mutex
	letters  []Letter
	capacity int
	handlers []Handler
}

// Option configures a Queue.
type Option func(*Queue)

// WithCapacity sets the maximum number of letters retained in the queue.
// When the capacity is reached, the oldest letter is discarded.
// Default is 1000.
func WithCapacity(capacity int) Option {
	return func(q *Queue) {
		q.capacity = capacity
	}
}

// New creates a new dead letter queue with the given options.
func New(opts ...Option) *Queue {
	q := &Queue{
		capacity: 1000,
	}
	for _, opt := range opts {
		opt(q)
	}
	q.letters = make([]Letter, 0, q.capacity)
	return q
}

// OnDeadLetter registers a handler that is called when a dead letter is published.
// Multiple handlers can be registered.
func (q *Queue) OnDeadLetter(h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers = append(q.handlers, h)
}

// Publish adds a dead letter to the queue and notifies all handlers.
func (q *Queue) Publish(l Letter) {
	q.mu.Lock()
	// Evict oldest if at capacity
	if len(q.letters) >= q.capacity {
		q.letters = q.letters[1:]
	}
	q.letters = append(q.letters, l)

	handlers := make([]Handler, len(q.handlers))
	copy(handlers, q.handlers)
	q.mu.Unlock()

	for _, h := range handlers {
		h(l)
	}
}

// Letters returns a copy of all retained dead letters.
func (q *Queue) Letters() []Letter {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]Letter, len(q.letters))
	copy(result, q.letters)
	return result
}

// Count returns the number of retained dead letters.
func (q *Queue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.letters)
}

// Clear removes all retained dead letters.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.letters = q.letters[:0]
}
