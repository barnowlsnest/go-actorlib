package actor

import (
	"context"
	"testing"
	"time"
)

// FuzzActorLifecycle tests actor state machine transitions with fuzzed inputs.
// It verifies that the actor never enters an invalid state regardless of the
// combination of operations and timing.
func FuzzActorLifecycle(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0))
	f.Add(uint8(1), uint8(1), uint8(1))
	f.Add(uint8(2), uint8(0), uint8(2))
	f.Add(uint8(0), uint8(2), uint8(1))

	f.Fuzz(func(t *testing.T, op1, op2, op3 uint8) {
		entity := NewTestEntity("fuzz", true)
		provider := NewTestEntityProvider(entity)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := New(
			WithProvider(provider),
			WithInputBufferSize[*TestEntity](5),
			WithReceiveTimeout[*TestEntity](10*time.Millisecond),
		)
		if err != nil {
			t.Fatal(err)
		}

		ops := []uint8{op1, op2, op3}
		for _, op := range ops {
			switch op % 4 {
			case 0: // Start (only makes sense once)
				_ = a.Start(ctx)
				_ = a.WaitReady(ctx, 50*time.Millisecond)
			case 1: // Stop
				_ = a.Stop(50 * time.Millisecond)
			case 2: // Send command
				cmd := NewTestCommand("fuzz-cmd", nil)
				_ = a.Receive(ctx, cmd)
			case 3: // Check state
				state := a.State()
				if state > Panicked {
					t.Fatalf("invalid state: %d", state)
				}
			}
		}

		// Final state must be valid
		state := a.State()
		if state > Panicked {
			t.Fatalf("invalid final state: %d", state)
		}
	})
}

// FuzzConcurrentStopAndSend tests that concurrent Stop and Send operations
// never cause a panic or data race.
func FuzzConcurrentStopAndSend(f *testing.F) {
	f.Add(uint8(5), uint8(3))

	f.Fuzz(func(t *testing.T, numSenders, numStoppers uint8) {
		senders := int(numSenders%20) + 1
		stoppers := int(numStoppers%5) + 1

		entity := NewTestEntity("fuzz", true)
		provider := NewTestEntityProvider(entity)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := New(
			WithProvider(provider),
			WithInputBufferSize[*TestEntity](100),
			WithReceiveTimeout[*TestEntity](10*time.Millisecond),
		)
		if err != nil {
			t.Fatal(err)
		}

		if err = a.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if err = a.WaitReady(ctx, 100*time.Millisecond); err != nil {
			t.Fatal(err)
		}

		done := make(chan struct{})

		for range senders {
			go func() {
				cmd := NewTestCommand("fuzz-cmd", nil)
				_ = a.Receive(ctx, cmd)
			}()
		}

		go func() {
			defer close(done)
			for range stoppers {
				_ = a.Stop(50 * time.Millisecond)
			}
		}()

		<-done

		state := a.State()
		if state > Panicked {
			t.Fatalf("invalid final state: %d", state)
		}
	})
}
