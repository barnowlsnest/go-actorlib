package actor

import (
	"context"
	"testing"
	"time"
)

// noopBenchCommand is reused across iterations so the benchmarks below report
// only the allocations made by Receive itself, not by the caller's command.
type noopBenchCommand struct{}

func (noopBenchCommand) Execute(context.Context, *TestEntity) {}

func startBenchActor(b *testing.B, opts ...GoActorOption[*TestEntity]) *Actor[*TestEntity] {
	b.Helper()

	provider := NewTestEntityProvider(NewTestEntity("bench", true))
	opts = append([]GoActorOption[*TestEntity]{WithProvider[*TestEntity](provider)}, opts...)

	a, err := StartNew(context.Background(), time.Second, opts...)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() { _ = a.Stop(time.Second) })

	return a
}

// BenchmarkReceive_MailboxHasRoom measures Receive when the mailbox usually has
// free capacity (the common case for a sized mailbox), with the default 5s
// receive timeout.
func BenchmarkReceive_MailboxHasRoom(b *testing.B) {
	a := startBenchActor(b, WithInputBufferSize[*TestEntity](1024))
	ctx := context.Background()
	cmd := noopBenchCommand{}

	b.ReportAllocs()
	for b.Loop() {
		if err := a.Receive(ctx, cmd); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReceive_MailboxHasRoom_NoTimeout is the same with receiveTimeout = 0.
func BenchmarkReceive_MailboxHasRoom_NoTimeout(b *testing.B) {
	a := startBenchActor(b, WithInputBufferSize[*TestEntity](1024), WithReceiveTimeout[*TestEntity](0))
	ctx := context.Background()
	cmd := noopBenchCommand{}

	b.ReportAllocs()
	for b.Loop() {
		if err := a.Receive(ctx, cmd); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReceive_ParallelSenders measures many goroutines sending to one actor.
func BenchmarkReceive_ParallelSenders(b *testing.B) {
	a := startBenchActor(b, WithInputBufferSize[*TestEntity](1024))
	ctx := context.Background()
	cmd := noopBenchCommand{}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := a.Receive(ctx, cmd); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
