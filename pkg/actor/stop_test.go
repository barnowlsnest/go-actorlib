package actor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStop_WhenTimeoutElapses_ShouldReturnGracefulStopTimeout(t *testing.T) {
	// Arrange
	hooks := NewTestHooks()
	a, err := StartNew(context.Background(), time.Second,
		WithProvider[*TestEntity](NewTestEntityProvider(NewTestEntity("v", true))),
		WithHooks[*TestEntity](hooks),
	)
	require.NoError(t, err)

	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	require.NoError(t, a.Receive(context.Background(), NewTestCommand("slow", func(context.Context, *TestEntity) {
		close(started)
		<-release
	})))
	<-started // the handler is running, so Stop must wait for it

	// Act
	err = a.Stop(time.Millisecond)

	// Assert
	require.ErrorIs(t, err, ErrActorGracefulStopTimeout)
	require.Equal(t, uint64(StoppedWithError), a.State())

	found := false
	for _, hookErr := range hooks.GetErrors() {
		found = found || errors.Is(hookErr, ErrActorGracefulStopTimeout)
	}
	require.True(t, found, "OnError should still report the timeout")
}

func TestStop_WhenNotRunning_ShouldReturnErrActorNotRunning(t *testing.T) {
	newActor := func(t *testing.T) *Actor[*TestEntity] {
		t.Helper()
		a, err := New(WithProvider[*TestEntity](NewTestEntityProvider(NewTestEntity("v", true))))
		require.NoError(t, err)
		return a
	}

	t.Run("Initialized", func(t *testing.T) {
		a := newActor(t)
		require.ErrorIs(t, a.Stop(time.Second), ErrActorNotRunning)
	})

	t.Run("Done", func(t *testing.T) {
		a := newActor(t)
		require.NoError(t, a.Start(context.Background()))
		require.NoError(t, a.WaitReady(context.Background(), time.Second))
		require.NoError(t, a.Stop(time.Second))

		require.ErrorIs(t, a.Stop(time.Second), ErrActorNotRunning)
	})

	t.Run("Panicked", func(t *testing.T) {
		a := newActor(t)
		require.NoError(t, a.Start(context.Background()))
		require.NoError(t, a.WaitReady(context.Background(), time.Second))
		require.NoError(t, a.Receive(context.Background(), NewTestCommand("p", func(context.Context, *TestEntity) {
			panic("boom")
		})))
		<-a.Done()

		err := a.Stop(time.Second)
		require.ErrorIs(t, err, ErrActorNotRunning)
		require.ErrorContains(t, err, "Panicked")
	})
}
