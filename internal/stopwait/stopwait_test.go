package stopwait

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/actor"
)

func TestWait_WithClosedDoneAndNonPositiveTimeout_ShouldReturnNil(t *testing.T) {
	done := make(chan struct{})
	close(done)

	for range 1000 {
		require.NoError(t, Wait(done, 0, nil))
		require.NoError(t, Wait(done, -time.Second, nil))
	}
}

func TestWait_WhenDoneClosesInTime_ShouldReturnNil(t *testing.T) {
	done := make(chan struct{})
	time.AfterFunc(10*time.Millisecond, func() { close(done) })

	require.NoError(t, Wait(done, time.Second, nil))
}

func TestWait_WhenTimeoutElapses_ShouldWrapTimeoutAndCause(t *testing.T) {
	cause := errors.New("cause")

	err := Wait(make(chan struct{}), 10*time.Millisecond, cause)

	require.ErrorIs(t, err, actor.ErrActorGracefulStopTimeout)
	require.ErrorIs(t, err, cause)
}
