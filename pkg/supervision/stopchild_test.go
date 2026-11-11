package supervision

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v5/pkg/actorref"
)

func TestStopChild_WithZeroTimeoutOnTerminatedChild_ShouldAlwaysSucceed(t *testing.T) {
	// Arrange
	ref, err := (&actorStartNewSpec{}).Start(context.Background())
	require.NoError(t, err)
	require.NoError(t, ref.Stop(time.Second))
	<-ref.Done()

	// Act + Assert — a closed Done must win over an already-fired timer.
	for range 500 {
		require.NoError(t, stopChild(ref, 0))
		require.NoError(t, stopChild(ref, -time.Second))
	}
}

func TestStopChild_WithNeverStartedChild_ShouldReturnNotRunningPromptly(t *testing.T) {
	// Arrange
	a, err := actor.New(actor.WithProvider(&supervisedEntityProvider{&supervisedEntity{}}))
	require.NoError(t, err)
	ref, err := actorref.New(a)
	require.NoError(t, err)

	// Act
	start := time.Now()
	err = stopChild(ref, time.Second)

	// Assert
	require.Less(t, time.Since(start), 500*time.Millisecond, "stopChild waited for Done of a child that never started")
	require.ErrorIs(t, err, actor.ErrActorNotRunning)
	require.NotErrorIs(t, err, actor.ErrActorGracefulStopTimeout)
}
