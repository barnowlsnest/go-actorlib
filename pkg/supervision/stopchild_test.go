package supervision

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
