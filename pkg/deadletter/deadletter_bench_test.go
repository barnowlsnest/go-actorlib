package deadletter

import (
	"fmt"
	"testing"
)

// BenchmarkPublish_AtCapacity measures steady-state Publish once the queue is
// full, so every call evicts the oldest letter.
func BenchmarkPublish_AtCapacity(b *testing.B) {
	q := New() // default capacity 1000
	q.OnDeadLetter(func(Letter) {})
	l := Letter{Target: "worker", Reason: "actor stopped"}
	for range 1000 {
		q.Publish(l)
	}

	b.ReportAllocs()
	for b.Loop() {
		q.Publish(l)
	}
}

// BenchmarkPublish_NoRetention measures handler notification alone.
func BenchmarkPublish_NoRetention(b *testing.B) {
	for _, n := range []int{1, 8} {
		b.Run(fmt.Sprintf("handlers=%d", n), func(b *testing.B) {
			q := New(WithCapacity(0))
			for range n {
				q.OnDeadLetter(func(Letter) {})
			}
			l := Letter{Target: "worker", Reason: "actor stopped"}

			b.ReportAllocs()
			for b.Loop() {
				q.Publish(l)
			}
		})
	}
}

func BenchmarkLetters(b *testing.B) {
	q := New()
	for range 1500 { // wrapped: oldest 500 evicted
		q.Publish(Letter{Target: "worker", Reason: "actor stopped"})
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = q.Letters()
	}
}
