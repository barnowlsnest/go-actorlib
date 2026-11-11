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
	mu sync.Mutex
	// letters is a fixed-size ring buffer of len capacity. The retained letters
	// are letters[head], letters[head+1], ... (mod capacity), count of them,
	// oldest first.
	letters  []Letter
	head     int
	count    int
	capacity int
	// handlers is append-only: OnDeadLetter appends and nothing ever rewrites
	// an existing element, so a snapshot of the slice header taken under mu
	// stays valid after mu is released and needs no copy.
	handlers []Handler
}

// Option configures a Queue.
type Option func(*Queue)

// WithCapacity sets the maximum number of letters retained in the queue.
// When the capacity is reached, the oldest letter is discarded.
// A capacity of zero or less retains no letters; handlers are still notified.
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
	q.capacity = max(q.capacity, 0)
	q.letters = make([]Letter, q.capacity)
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
//
// At capacity, the oldest letter is overwritten in place, so Publish never
// allocates or copies the buffer. The previous slice-based queue resliced from
// the front and reallocated every few hundred publishes, copying all retained
// letters (91 B/op at the default capacity, BenchmarkPublish_AtCapacity).
// Handlers registered during this call are not notified of this letter.
func (q *Queue) Publish(l Letter) {
	q.mu.Lock()
	if q.capacity > 0 {
		if q.count < q.capacity {
			q.letters[(q.head+q.count)%q.capacity] = l
			q.count++
		} else {
			q.letters[q.head] = l // overwrite the oldest
			q.head = (q.head + 1) % q.capacity
		}
	}

	handlers := q.handlers // append-only; see the handlers field
	q.mu.Unlock()

	for _, h := range handlers {
		h(l)
	}
}

// Letters returns a copy of all retained dead letters, oldest first.
func (q *Queue) Letters() []Letter {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]Letter, q.count)
	n := copy(result, q.letters[q.head:min(q.head+q.count, q.capacity)])
	copy(result[n:], q.letters[:q.count-n])
	return result
}

// Count returns the number of retained dead letters.
func (q *Queue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.count
}

// Clear removes all retained dead letters.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	clear(q.letters) // drop references so evicted strings can be collected
	q.head = 0
	q.count = 0
}
