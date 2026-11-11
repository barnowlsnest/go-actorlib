package system

import (
	"fmt"
	"testing"
)

func BenchmarkEmitEvent(b *testing.B) {
	for _, n := range []int{1, 8} {
		b.Run(fmt.Sprintf("handlers=%d", n), func(b *testing.B) {
			s := New()
			for range n {
				s.OnEvent(func(Event) {})
			}
			ev := Event{Kind: EventActorStopped, ActorName: "worker"}

			b.ReportAllocs()
			for b.Loop() {
				s.emitEvent(ev)
			}
		})
	}
}
