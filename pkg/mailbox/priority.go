// Package mailbox provides alternative mailbox implementations for actors.
//
// The standard actor uses a Go channel as its mailbox, which provides FIFO ordering.
// This package provides a priority mailbox that allows messages with higher priority
// to be processed before lower-priority ones.
//
// The priority mailbox uses go-datalib's Heap for O(log n) insert and O(log n) extract.
//
// Example usage:
//
//	mb := mailbox.NewPriority[*MyEntity](10)
//	mb.Push(myCommand, mailbox.High)
//	msg, ok := mb.Pop()
package mailbox

import (
	"sync"

	"github.com/barnowlsnest/go-datalib/pkg/tree"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// Priority defines message priority levels.
// Lower values are higher priority (processed first).
type Priority int

const (
	// System is the highest priority, used for internal system messages.
	System Priority = iota

	// High is for time-sensitive user messages.
	High

	// Normal is the default priority for user messages.
	Normal

	// Low is for background/best-effort messages.
	Low
)

// message wraps an executable with its priority for heap ordering.
type message[T actor.Entity] struct {
	executable actor.Executable[T]
	priority   Priority
	seq        uint64 // insertion order for stable sorting within same priority
}

// PriorityMailbox is a thread-safe priority queue for actor messages.
// Messages with lower Priority values are dequeued first.
// Within the same priority, FIFO order is maintained.
type PriorityMailbox[T actor.Entity] struct {
	mu      sync.Mutex
	heap    *tree.Heap[message[T]]
	seq     uint64
	notify  chan struct{} // signaled when a message is pushed
	closed  bool
	maxSize int
}

// NewPriority creates a new PriorityMailbox with the given maximum size.
// The notify channel can be used to receive a signal when messages are available.
func NewPriority[T actor.Entity](maxSize int) *PriorityMailbox[T] {
	return &PriorityMailbox[T]{
		heap: tree.NewHeap(func(a, b message[T]) bool {
			if a.priority != b.priority {
				return a.priority < b.priority
			}
			return a.seq < b.seq
		}),
		notify:  make(chan struct{}, 1),
		maxSize: maxSize,
	}
}

// Push adds a message with the given priority to the mailbox.
// Returns false if the mailbox is full or closed.
func (mb *PriorityMailbox[T]) Push(e actor.Executable[T], priority Priority) bool {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.closed {
		return false
	}

	if mb.maxSize > 0 && mb.heap.Size() >= mb.maxSize {
		return false
	}

	mb.seq++
	mb.heap.Push(message[T]{
		executable: e,
		priority:   priority,
		seq:        mb.seq,
	})

	// Non-blocking notify
	select {
	case mb.notify <- struct{}{}:
	default:
	}

	return true
}

// Pop removes and returns the highest-priority message.
// Returns the executable and true, or nil and false if the mailbox is empty.
func (mb *PriorityMailbox[T]) Pop() (actor.Executable[T], bool) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	msg, ok := mb.heap.Pop()
	if !ok {
		return nil, false
	}
	return msg.executable, true
}

// Notify returns a channel that receives a signal when messages are available.
func (mb *PriorityMailbox[T]) Notify() <-chan struct{} {
	return mb.notify
}

// Size returns the number of messages in the mailbox.
func (mb *PriorityMailbox[T]) Size() int {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return mb.heap.Size()
}

// Close marks the mailbox as closed. No more messages can be pushed.
func (mb *PriorityMailbox[T]) Close() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.closed = true
}

// IsEmpty returns true if the mailbox has no messages.
func (mb *PriorityMailbox[T]) IsEmpty() bool {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return mb.heap.IsEmpty()
}
