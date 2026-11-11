package actor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// cancelableCommand records whether it was executed or canceled.
type cancelableCommand struct {
	executeFn func()
	executed  atomic.Bool
	canceled  atomic.Bool
	cancelErr atomic.Value
}

func (c *cancelableCommand) Execute(_ context.Context, _ *TestEntity) {
	c.executed.Store(true)
	if c.executeFn != nil {
		c.executeFn()
	}
}

func (c *cancelableCommand) Cancel(err error) {
	c.canceled.Store(true)
	c.cancelErr.Store(err)
}

func startDrainActor(t *testing.T, ctx context.Context, bufSize int) *Actor[*TestEntity] {
	t.Helper()
	a, err := StartNew(ctx, time.Second,
		WithProvider[*TestEntity](NewTestEntityProvider(NewTestEntity("v", true))),
		WithInputBufferSize[*TestEntity](bufSize),
	)
	require.NoError(t, err)
	return a
}

func waitDone(t *testing.T, a *Actor[*TestEntity]) {
	t.Helper()
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("actor did not terminate")
	}
}

func TestStop_WithQueuedCommands_ShouldCancelThem(t *testing.T) {
	a := startDrainActor(t, context.Background(), 4)

	started, release := make(chan struct{}), make(chan struct{})
	blocker := &cancelableCommand{executeFn: func() { close(started); <-release }}
	queued := &cancelableCommand{}
	require.NoError(t, a.Receive(context.Background(), blocker))
	<-started
	require.NoError(t, a.Receive(context.Background(), queued))

	stopped := make(chan error, 1)
	go func() { stopped <- a.Stop(time.Second) }()
	// Wait for the stop signal itself: Stop sets Stopping before it closes the
	// stop channel, so the state alone does not mean the loop will see it.
	<-a.stop
	close(release)
	require.NoError(t, <-stopped)

	require.True(t, blocker.executed.Load())
	require.False(t, queued.executed.Load())
	require.True(t, queued.canceled.Load())
	require.ErrorIs(t, queued.cancelErr.Load().(error), ErrActorCommandDropped)
}

func TestContextCancel_WithQueuedCommands_ShouldCancelThem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := startDrainActor(t, ctx, 4)

	release := make(chan struct{})
	blocker := &cancelableCommand{executeFn: func() { <-release }}
	queued := &cancelableCommand{}
	require.NoError(t, a.Receive(context.Background(), blocker))
	require.NoError(t, a.Receive(context.Background(), queued))

	cancel()
	close(release)
	waitDone(t, a)

	require.True(t, queued.canceled.Load())
}

func TestPanic_WithQueuedCommands_ShouldCancelThem(t *testing.T) {
	a := startDrainActor(t, context.Background(), 4)

	release := make(chan struct{})
	panicking := &cancelableCommand{executeFn: func() { <-release; panic("boom") }}
	queued := &cancelableCommand{}
	require.NoError(t, a.Receive(context.Background(), panicking))
	require.NoError(t, a.Receive(context.Background(), queued))

	close(release)
	waitDone(t, a)

	require.True(t, queued.canceled.Load())
}

func TestReceive_ConcurrentWithStop_ShouldExecuteOrCancelEveryAcceptedCommand(t *testing.T) {
	for range 50 {
		a := startDrainActor(t, context.Background(), 8)

		var (
			mu       sync.Mutex
			accepted []*cancelableCommand
			wg       sync.WaitGroup
		)
		for range 8 {
			wg.Go(func() {
				for range 50 {
					cmd := &cancelableCommand{}
					if err := a.Receive(context.Background(), cmd); err != nil {
						return
					}
					mu.Lock()
					accepted = append(accepted, cmd)
					mu.Unlock()
				}
			})
		}

		time.Sleep(time.Millisecond)
		require.NoError(t, a.Stop(time.Second))
		wg.Wait()

		for _, cmd := range accepted {
			require.True(t, cmd.executed.Load() || cmd.canceled.Load(), "accepted command was lost")
		}
	}
}
