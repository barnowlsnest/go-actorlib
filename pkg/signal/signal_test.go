package signal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type mockStoppable struct {
	stopped bool
	stopErr error
}

func (m *mockStoppable) StopAll(_ time.Duration) error {
	m.stopped = true
	return m.stopErr
}

type SignalTestSuite struct {
	suite.Suite
}

func TestSignalTestSuite(t *testing.T) {
	suite.Run(t, new(SignalTestSuite))
}

func (s *SignalTestSuite) TestAwaitShutdown_ContextCancel_ShouldStopAll() {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockStoppable{}

	// Act — cancel context to trigger shutdown
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := AwaitShutdown(ctx, mock, time.Second)

	// Assert
	s.NoError(err)
	s.True(mock.stopped)
}

func (s *SignalTestSuite) TestNotifyShutdown_ShouldReturnChannelAndStop() {
	// Act
	ch, stop := NotifyShutdown()
	defer stop()

	// Assert
	s.NotNil(ch)
}
