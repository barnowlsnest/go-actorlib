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
