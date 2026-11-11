package actor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// funcCancelCommand is a Cancelable whose Cancel runs an arbitrary callback.
type funcCancelCommand struct {
	executeFn func()
	cancelFn  func(err error)
	canceled  atomic.Bool
}

func (c *funcCancelCommand) Execute(context.Context, *TestEntity) {
	if c.executeFn != nil {
		c.executeFn()
	}
}

func (c *funcCancelCommand) Cancel(err error) {
	c.canceled.Store(true)
	if c.cancelFn != nil {
		c.cancelFn(err)
	}
}

// startBlocked starts an actor, occupies it with a blocking command and
// returns the release channel.
func startBlocked(
	t *testing.T, ctx context.Context, opts ...GoActorOption[*TestEntity],
) (a *GoActor[*TestEntity], release chan struct{}) {
	t.Helper()
	all := append([]GoActorOption[*TestEntity]{
		WithProvider[*TestEntity](NewTestEntityProvider(NewTestEntity("v", true))),
		WithInputBufferSize[*TestEntity](4),
	}, opts...)
	a, err := StartNew(ctx, time.Second, all...)
	require.NoError(t, err)

	started := make(chan struct{})
	release = make(chan struct{})
	require.NoError(t, a.Receive(context.Background(), &funcCancelCommand{executeFn: func() {
		close(started)
		<-release
	}}))
	<-started
	return a, release
}

func TestDrain_WithSenderBlockedOnFullMailboxAndNoTimeout_ShouldReleaseSender(t *testing.T) {
	// Arrange — buffer of 1 is filled; the next sender blocks with no receive timeout.
	ctx, cancel := context.WithCancel(context.Background())
	a, release := startBlocked(t, ctx,
		WithInputBufferSize[*TestEntity](1),
		WithReceiveTimeout[*TestEntity](0),
	)
	require.NoError(t, a.Receive(context.Background(), &funcCancelCommand{}))

	sendErr := make(chan error, 1)
	blocked := &funcCancelCommand{}
	go func() { sendErr <- a.Receive(context.Background(), blocked) }()
	time.Sleep(20 * time.Millisecond) // let the sender block on the full mailbox

	// Act — terminate via ctx (the stop channel is never closed on this path).
	cancel()
	close(release)

	// Assert
	var err error
	select {
	case err = <-sendErr:
	case <-time.After(time.Second):
		t.Fatal("sender blocked on a full mailbox was not released when the actor exited")
	}
	waitDone(t, a)

	// The send either lost to the exit (rejected) or won a freed slot, in which
	// case the drain — complete once Done is closed — must have canceled it.
	if err == nil {
		require.True(t, blocked.canceled.Load(), "accepted command must be canceled")
	} else {
		require.ErrorIs(t, err, ErrActorReceiveOnStopped)
	}
}

func TestDrain_WhenCancelReceivesOnSameActor_ShouldNotDeadlock(t *testing.T) {
	// Arrange
	a, release := startBlocked(t, context.Background())

	var reentrantErr atomic.Value
	reentrant := &funcCancelCommand{cancelFn: func(error) {
		reentrantErr.Store(a.Receive(context.Background(), &funcCancelCommand{}))
	}}
	require.NoError(t, a.Receive(context.Background(), reentrant))

	// Act
	stopped := make(chan error, 1)
	go func() { stopped <- a.Stop(time.Second) }()
	<-a.stop
	close(release)

	// Assert
	require.NoError(t, <-stopped)
	require.True(t, reentrant.canceled.Load())
	require.ErrorIs(t, reentrantErr.Load().(error), ErrActorReceiveOnStopped)
}

func TestDrain_WhenCancelStopsSameActor_ShouldNotDeadlock(t *testing.T) {
	// Arrange
	a, release := startBlocked(t, context.Background())

	var reentrantErr atomic.Value
	reentrant := &funcCancelCommand{cancelFn: func(error) {
		reentrantErr.Store(a.Stop(time.Second))
	}}
	require.NoError(t, a.Receive(context.Background(), reentrant))

	// Act
	stopped := make(chan error, 1)
	go func() { stopped <- a.Stop(time.Second) }()
	<-a.stop
	close(release)

	// Assert
	require.NoError(t, <-stopped)
	require.Error(t, reentrantErr.Load().(error), "Stop on an exiting actor must be rejected, not block")
}

func TestDrain_WhenCancelPanics_ShouldStillCancelRemainingCommandsAndReportError(t *testing.T) {
	// Arrange
	hooks := NewTestHooks()
	a, release := startBlocked(t, context.Background(), WithHooks[*TestEntity](hooks))

	panicking := &funcCancelCommand{cancelFn: func(error) { panic("cancel boom") }}
	after := &funcCancelCommand{}
	require.NoError(t, a.Receive(context.Background(), panicking))
	require.NoError(t, a.Receive(context.Background(), after))

	// Act
	stopped := make(chan error, 1)
	go func() { stopped <- a.Stop(time.Second) }()
	<-a.stop
	close(release)

	// Assert
	require.NoError(t, <-stopped)
	require.True(t, after.canceled.Load(), "a panicking Cancel must not abort the drain")
	found := false
	for _, err := range hooks.GetErrors() {
		found = found || errors.Is(err, ErrActorPanic)
	}
	require.True(t, found, "Cancel panic should be reported via OnError")
}
