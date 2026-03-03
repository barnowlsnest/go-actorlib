package middleware

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
)

// --- Test Entity ---

type testEntity struct {
	value string
	mu    sync.Mutex
}

func (te *testEntity) IsProvidable() bool { return true }
func (te *testEntity) GetValue() string {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.value
}

type testProvider struct{ entity *testEntity }

func (p *testProvider) Provide() *testEntity { return p.entity }

type testCommand struct {
	executeFn func(ctx context.Context, entity *testEntity)
}

func (tc *testCommand) Execute(ctx context.Context, entity *testEntity) {
	if tc.executeFn != nil {
		tc.executeFn(ctx, entity)
	}
}

// safeBuffer is a thread-safe bytes buffer for capturing log output.
type safeBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.buf = append(sb.buf, p...)
	return len(p), nil
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return string(sb.buf)
}

// --- Test Suite ---

type MiddlewareTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	entity   *testEntity
	provider *testProvider
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = &testEntity{value: "test"}
	s.provider = &testProvider{entity: s.entity}
}

func (s *MiddlewareTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

// --- Logging tests ---

func (s *MiddlewareTestSuite) TestLogging_ShouldLogMessages() {
	// Arrange
	buf := &safeBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	a, err := actor.New(
		actor.WithProvider(s.provider),
		actor.WithMiddleware(Logging[*testEntity](logger)),
		actor.WithName[*testEntity]("test-actor"),
	)
	s.Require().NoError(err)

	s.Require().NoError(a.Start(s.ctx))
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := &testCommand{executeFn: func(_ context.Context, _ *testEntity) {}}

	// Act
	s.NoError(a.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert
	output := buf.String()
	s.Contains(output, "actor processing message")
	s.Contains(output, "actor processed message")
	s.Contains(output, "test-actor")
}

// --- Metrics tests ---

func (s *MiddlewareTestSuite) TestMetrics_ShouldTrackMessageCount() {
	// Arrange
	metrics := &Metrics{}

	a, err := actor.New(
		actor.WithProvider(s.provider),
		actor.WithMiddleware(MetricsMiddleware[*testEntity](metrics)),
	)
	s.Require().NoError(err)

	s.Require().NoError(a.Start(s.ctx))
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	// Act
	var done atomic.Int64
	for range 5 {
		cmd := &testCommand{executeFn: func(_ context.Context, _ *testEntity) {
			done.Add(1)
		}}
		s.NoError(a.Receive(s.ctx, cmd))
	}

	// Wait for all commands to complete
	s.Eventually(func() bool { return done.Load() == 5 }, time.Second, 10*time.Millisecond)

	// Assert
	s.Equal(int64(5), metrics.MessageCount())
	s.Greater(metrics.TotalDuration(), time.Duration(0))
	s.Greater(metrics.AverageDuration(), time.Duration(0))
}

func (s *MiddlewareTestSuite) TestMetrics_NoMessages_ShouldReturnZero() {
	metrics := &Metrics{}
	s.Equal(int64(0), metrics.MessageCount())
	s.Equal(time.Duration(0), metrics.TotalDuration())
	s.Equal(time.Duration(0), metrics.AverageDuration())
}

// --- Recovery tests ---

func (s *MiddlewareTestSuite) TestRecovery_ShouldCatchPanic() {
	// Arrange
	buf := &safeBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	a, err := actor.New(
		actor.WithProvider(s.provider),
		actor.WithMiddleware(Recovery[*testEntity](logger)),
		actor.WithName[*testEntity]("panic-actor"),
	)
	s.Require().NoError(err)

	s.Require().NoError(a.Start(s.ctx))
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	panicCmd := &testCommand{executeFn: func(_ context.Context, _ *testEntity) {
		panic("test panic")
	}}

	// Act
	s.NoError(a.Receive(s.ctx, panicCmd))
	time.Sleep(20 * time.Millisecond)

	// Assert — actor should still be running (panic caught by middleware, not actor)
	output := buf.String()
	s.Contains(output, "actor panic recovered by middleware")
	s.Contains(output, "test panic")
}
