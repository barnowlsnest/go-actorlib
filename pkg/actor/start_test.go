package actor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStart_Concurrent_ShouldStartExactlyOnce(t *testing.T) {
	for range 500 {
		a, err := New(WithProvider[TestEntity](NewTestEntityProvider(NewTestEntity("v", true))))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())

		var (
			wg        sync.WaitGroup
			successes atomic.Int32
			errs      = make([]error, 4)
		)
		for i := range errs {
			wg.Go(func() {
				errs[i] = a.Start(ctx)
				if errs[i] == nil {
					successes.Add(1)
				}
			})
		}
		wg.Wait()

		require.Equal(t, int32(1), successes.Load())
		for _, err := range errs {
			if err != nil {
				require.ErrorIs(t, err, ErrActorAlreadyStarted)
			}
		}

		cancel()
		<-a.Done()
	}
}

func TestStart_AfterStart_ShouldReturnErrActorAlreadyStarted(t *testing.T) {
	a, err := StartNew(context.Background(), time.Second,
		WithProvider[TestEntity](NewTestEntityProvider(NewTestEntity("v", true))))
	require.NoError(t, err)
	defer func() { _ = a.Stop(time.Second) }()

	require.ErrorIs(t, a.Start(context.Background()), ErrActorAlreadyStarted)
}

// flakyProvider returns a nil entity on the first call and a valid one afterwards.
type flakyProvider struct{ calls atomic.Int32 }

func (p *flakyProvider) Provide() *TestEntity {
	if p.calls.Add(1) == 1 {
		return nil
	}
	return NewTestEntity("v", true)
}

func TestStart_AfterFailedStart_ShouldAllowRetry(t *testing.T) {
	a, err := New(WithProvider[TestEntity](&flakyProvider{}))
	require.NoError(t, err)

	require.ErrorIs(t, a.Start(context.Background()), ErrActorNilEntity)
	require.NoError(t, a.Start(context.Background()))
	require.NoError(t, a.WaitReady(context.Background(), time.Second))
	require.NoError(t, a.Stop(time.Second))
}
