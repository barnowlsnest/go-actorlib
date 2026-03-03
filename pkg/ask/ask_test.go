package ask

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actorref"

	"github.com/barnowlsnest/go-actorlib/v4/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v4/pkg/command"
)

// TestEntity is a mock entity for testing purposes
type TestEntity struct {
	value   string
	isReady bool
	mu      sync.Mutex
}

func NewTestEntity(value string, isReady bool) *TestEntity {
	return &TestEntity{
		value:   value,
		isReady: isReady,
	}
}

func (te *TestEntity) IsProvidable() bool {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.isReady
}

func (te *TestEntity) GetValue() string {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.value
}

func (te *TestEntity) SetValue(value string) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.value = value
}

// TestEntityProvider provides test entities
type TestEntityProvider struct {
	entity *TestEntity
}

func (tep *TestEntityProvider) Provide() *TestEntity {
	return tep.entity
}

// GoAskTestSuite provides a test suite for the ask package
type GoAskTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	entity *TestEntity
	actor  *actor.GoActor[*TestEntity]
	ref    *actorref.Ref[*TestEntity]
}

func (s *GoAskTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = NewTestEntity("test-value", true)
	provider := &TestEntityProvider{entity: s.entity}

	var err error
	s.actor, err = actor.New(
		actor.WithProvider(provider),
		actor.WithInputBufferSize[*TestEntity](10),
	)
	s.Require().NoError(err)

	err = s.actor.Start(s.ctx)
	s.Require().NoError(err)

	err = s.actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.Require().NoError(err)

	s.ref, err = actorref.New(s.actor)
	s.Require().NoError(err)
}

func (s *GoAskTestSuite) TearDownTest() {
	if s.actor.State() == actor.Started {
		_ = s.actor.Stop(100 * time.Millisecond)
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func TestGoAskTestSuite(t *testing.T) {
	suite.Run(t, new(GoAskTestSuite))
}

func (s *GoAskTestSuite) TestNew_SuccessfulExecution_ShouldReturnResult() {
	// Arrange
	fn := func(entity *TestEntity) (string, error) {
		return entity.GetValue(), nil
	}

	// Act
	result, err := New(s.ctx, s.ref, fn, time.Second)

	// Assert
	s.NoError(err)
	s.Equal("test-value", result)
}

func (s *GoAskTestSuite) TestNew_DelegateFunctionError_ShouldReturnError() {
	// Arrange
	expectedErr := errors.New("delegate failed")
	fn := func(entity *TestEntity) (string, error) {
		return "", expectedErr
	}

	// Act
	result, err := New(s.ctx, s.ref, fn, time.Second)

	// Assert
	s.Error(err)
	s.Equal(expectedErr, err)
	s.Empty(result)
}

func (s *GoAskTestSuite) TestNew_DelegateFunctionPanic_ShouldReturnPanicError() {
	// Arrange
	fn := func(entity *TestEntity) (string, error) {
		panic("something went wrong")
	}

	// Act
	result, err := New(s.ctx, s.ref, fn, time.Second)

	// Assert
	s.Error(err)
	s.ErrorIs(err, command.ErrCommandPanic)
	s.Empty(result)
}

func (s *GoAskTestSuite) TestNew_OnStoppedActor_ShouldReturnReceiveError() {
	// Arrange
	err := s.actor.Stop(100 * time.Millisecond)
	s.Require().NoError(err)

	fn := func(entity *TestEntity) (string, error) {
		return entity.GetValue(), nil
	}

	// Act
	result, err := New(s.ctx, s.ref, fn, time.Second)

	// Assert
	s.Error(err)
	s.ErrorIs(err, actor.ErrActorReceiveOnStopped)
	s.Empty(result)
}

func (s *GoAskTestSuite) TestNew_ContextCancelledDuringWait_ShouldReturnContextError() {
	// Arrange
	fn := func(entity *TestEntity) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return entity.GetValue(), nil
	}

	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.cancel()
	}()

	// Act
	result, err := New(s.ctx, s.ref, fn, 5*time.Second)

	// Assert
	s.Error(err)
	s.ErrorIs(err, context.Canceled)
	s.Empty(result)
}

func (s *GoAskTestSuite) TestNew_SlowCommand_ShouldReturnTimeoutError() {
	// Arrange
	fn := func(entity *TestEntity) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return entity.GetValue(), nil
	}

	// Act
	result, err := New(s.ctx, s.ref, fn, 50*time.Millisecond)

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrAskTimeout)
	s.Empty(result)
}

func (s *GoAskTestSuite) TestNew_ConcurrentAsks_ShouldBeSafe() {
	// Arrange
	const goroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	results := make([]string, goroutines)

	// Act
	for i := range goroutines {
		wg.Go(func() {
			fn := func(entity *TestEntity) (string, error) {
				return fmt.Sprintf("result-%d", i), nil
			}
			results[i], errs[i] = New(s.ctx, s.ref, fn, time.Second)
		})
	}
	wg.Wait()

	// Assert
	for i := range goroutines {
		s.NoError(errs[i])
		s.Equal(fmt.Sprintf("result-%d", i), results[i])
	}
}

// Benchmark tests
func BenchmarkNew_Success(b *testing.B) {
	entity := NewTestEntity("bench-value", true)
	provider := &TestEntityProvider{entity: entity}

	a, err := actor.New(
		actor.WithProvider(provider),
		actor.WithInputBufferSize[*TestEntity](10),
	)
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		b.Fatal(err)
	}
	if err := a.WaitReady(ctx, time.Second); err != nil {
		b.Fatal(err)
	}

	ref, err := actorref.New(a)
	if err != nil {
		b.Fatal(err)
	}

	fn := func(entity *TestEntity) (string, error) {
		return entity.GetValue(), nil
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := New(ctx, ref, fn, time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	if err := a.Stop(time.Second); err != nil {
		b.Fatal(err)
	}
}
