package ask

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/barnowlsnest/go-actorlib/v6/pkg/command"
)

type counter struct{ n int }

func increment(c *counter) (int, error) {
	c.n++
	return c.n, nil
}

func TestAwait_CommandSucceeds_ShouldReturnResult(t *testing.T) {
	cmd := command.New(increment)
	cmd.Execute(context.Background(), &counter{n: 41})

	result, err := Await(context.Background(), cmd, time.Second)

	require.NoError(t, err)
	require.Equal(t, 42, result)
}

func TestAwait_CommandFails_ShouldReturnDelegateError(t *testing.T) {
	wantErr := errors.New("delegate failed")
	cmd := command.New(func(_ *counter) (int, error) { return 0, wantErr })
	cmd.Execute(context.Background(), &counter{})

	result, err := Await(context.Background(), cmd, time.Second)

	require.ErrorIs(t, err, wantErr)
	require.Zero(t, result)
}

func TestAwait_CommandCanceled_ShouldReturnCancelError(t *testing.T) {
	cancelErr := errors.New("dropped")
	cmd := command.New(increment)
	cmd.Cancel(cancelErr)

	result, err := Await(context.Background(), cmd, time.Second)

	require.ErrorIs(t, err, cancelErr)
	require.Zero(t, result)
}

func TestAwait_ContextDone_ShouldReturnContextError(t *testing.T) {
	cmd := command.New(increment) // never executed
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := Await(ctx, cmd, time.Second)

	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, result)
}

func TestAwait_NoResultBeforeTimeout_ShouldReturnErrAskTimeout(t *testing.T) {
	cmd := command.New(increment) // never executed

	result, err := Await(context.Background(), cmd, 10*time.Millisecond)

	require.ErrorIs(t, err, ErrAskTimeout)
	require.Zero(t, result)
}
