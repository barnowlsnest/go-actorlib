package actor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// startWithFullMailbox starts an actor with a 1-slot mailbox, occupies the loop
// with a blocking command and fills the mailbox, so the next Receive must take
// the slow (blocking) path. Closing the returned channel unblocks the actor.
func startWithFullMailbox(t *testing.T, receiveTimeout time.Duration) (a *Actor[TestEntity], release chan struct{}) {
	t.Helper()

	var err error
	a, err = StartNew(context.Background(), time.Second,
		WithProvider[TestEntity](NewTestEntityProvider(NewTestEntity("v", true))),
		WithInputBufferSize[TestEntity](1),
		WithReceiveTimeout[TestEntity](receiveTimeout),
	)
	require.NoError(t, err)

	started := make(chan struct{})
	release = make(chan struct{})
	require.NoError(t, a.Receive(context.Background(), NewTestCommand("blocker", func(context.Context, *TestEntity) {
		close(started)
		<-release
	})))
	<-started // the loop is busy, so the mailbox slot below stays occupied

	require.NoError(t, a.Receive(context.Background(), NewTestCommand("filler", nil)))
	require.Len(t, a.input, cap(a.input), "mailbox should be full")

	t.Cleanup(func() { _ = a.Stop(time.Second) })

	return a, release
}

func TestReceive_WhenMailboxFullAndTimeoutElapses_ShouldReturnReceiveTimeout(t *testing.T) {
	// Arrange
	const timeout = 50 * time.Millisecond
	a, release := startWithFullMailbox(t, timeout)
	defer close(release)

	var executed atomic.Bool
	cmd := NewTestCommand("late", func(context.Context, *TestEntity) { executed.Store(true) })

	// Act
	start := time.Now()
	err := a.Receive(context.Background(), cmd)
	elapsed := time.Since(start)

	// Assert
	require.ErrorIs(t, err, ErrActorReceiveTimeout)
	require.GreaterOrEqual(t, elapsed, timeout, "Receive must wait out the timeout before giving up")
	require.Less(t, elapsed, time.Second)
	require.False(t, executed.Load(), "a rejected command must not be executed")
}

func TestReceive_WhenMailboxFullAndRoomFreesBeforeTimeout_ShouldSucceed(t *testing.T) {
	// Arrange
	a, release := startWithFullMailbox(t, time.Second)

	executed := make(chan struct{})
	cmd := NewTestCommand("waiting", func(context.Context, *TestEntity) { close(executed) })

	// Act - free the mailbox while Receive is blocked on it
	time.AfterFunc(20*time.Millisecond, func() { close(release) })
	err := a.Receive(context.Background(), cmd)

	// Assert
	require.NoError(t, err)
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("command accepted after waiting was never executed")
	}
}

func TestReceive_WhenMailboxFullAndContextCanceled_ShouldReturnContextError(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		// Arrange
		a, release := startWithFullMailbox(t, timeout)

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)

		// Act
		start := time.Now()
		err := a.Receive(ctx, NewTestCommand("canceled", nil))

		// Assert
		require.ErrorIs(t, err, context.Canceled, "timeout=%v", timeout)
		require.Less(t, time.Since(start), 500*time.Millisecond, "timeout=%v", timeout)
		close(release)
	}
}
