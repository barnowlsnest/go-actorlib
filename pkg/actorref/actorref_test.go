package actorref

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v2/pkg/actor"
)

// testEntity is a mock entity for testing purposes.
type testEntity struct {
	value   string
	isReady bool
	mu      sync.Mutex
}

func newTestEntity(value string, isReady bool) *testEntity {
	return &testEntity{value: value, isReady: isReady}
}

func (te *testEntity) IsProvidable() bool {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.isReady
}

func (te *testEntity) GetValue() string {
	te.mu.Lock()
	defer te.mu.Unlock()
	return te.value
}

func (te *testEntity) SetValue(value string) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.value = value
}

// testEntityProvider provides test entities.
type testEntityProvider struct {
	entity *testEntity
}

func (p *testEntityProvider) Provide() *testEntity {
	return p.entity
}

// testCommand implements actor.Executable for testing.
type testCommand struct {
	fn func(ctx context.Context, entity *testEntity)
	mu sync.Mutex
}

func newTestCommand(fn func(ctx context.Context, entity *testEntity)) *testCommand {
	return &testCommand{fn: fn}
}

func (c *testCommand) Execute(ctx context.Context, entity *testEntity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fn != nil {
		c.fn(ctx, entity)
	}
}

// ActorRefTestSuite provides a test suite for the actorref package.
type ActorRefTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	entity   *testEntity
	provider *testEntityProvider
}

func (s *ActorRefTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = newTestEntity("test-value", true)
	s.provider = &testEntityProvider{entity: s.entity}
}

func (s *ActorRefTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestActorRefTestSuite(t *testing.T) {
	suite.Run(t, new(ActorRefTestSuite))
}

func (s *ActorRefTestSuite) newStartedActor() *actor.GoActor[*testEntity] {
	a, err := actor.New(
		actor.WithProvider(s.provider),
		actor.WithInputBufferSize[*testEntity](10),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)

	err = a.WaitReady(s.ctx, 100*time.Millisecond)
	s.Require().NoError(err)

	return a
}

// Construction tests

func (s *ActorRefTestSuite) TestNew_ValidActor_ShouldCreateRef() {
	// Arrange
	a, err := actor.New(actor.WithProvider(s.provider))
	s.Require().NoError(err)

	// Act
	ref, err := New(a)

	// Assert
	s.NoError(err)
	s.NotNil(ref)
}

func (s *ActorRefTestSuite) TestNew_NilActor_ShouldReturnError() {
	// Act
	ref, err := New[*testEntity](nil)

	// Assert
	s.Error(err)
	s.Nil(ref)
	s.ErrorIs(err, ErrActorRefNilActor)
}

// Send tests

func (s *ActorRefTestSuite) TestSend_ValidCommand_ShouldExecute() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	executed := make(chan struct{})
	cmd := newTestCommand(func(ctx context.Context, entity *testEntity) {
		entity.SetValue("updated-via-ref")
		close(executed)
	})

	// Act
	err = ref.Send(s.ctx, cmd)

	// Assert
	s.NoError(err)

	select {
	case <-executed:
		s.Equal("updated-via-ref", s.entity.GetValue())
	case <-time.After(time.Second):
		s.Fail("command was not executed within timeout")
	}
}

func (s *ActorRefTestSuite) TestSend_NilCommand_ShouldReturnError() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	// Act
	err = ref.Send(s.ctx, nil)

	// Assert
	s.Error(err)
	s.ErrorIs(err, actor.ErrActorReceiveNil)
}

func (s *ActorRefTestSuite) TestSend_StoppedActor_ShouldReturnError() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	err = a.Stop(100 * time.Millisecond)
	s.Require().NoError(err)

	cmd := newTestCommand(nil)

	// Act
	err = ref.Send(s.ctx, cmd)

	// Assert
	s.Error(err)
	s.ErrorIs(err, actor.ErrActorReceiveOnStopped)
}

func (s *ActorRefTestSuite) TestSend_CancelledContext_ShouldReturnError() {
	// Arrange — unbuffered channel + no timeout so send blocks on context
	a, err := actor.New(
		actor.WithProvider(s.provider),
		actor.WithInputBufferSize[*testEntity](0),
		actor.WithReceiveTimeout[*testEntity](0),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)

	err = a.WaitReady(s.ctx, 100*time.Millisecond)
	s.Require().NoError(err)

	// Block the actor so the next send cannot be queued
	blockCmd := newTestCommand(func(ctx context.Context, entity *testEntity) {
		time.Sleep(500 * time.Millisecond)
	})
	err = a.Receive(s.ctx, blockCmd)
	s.Require().NoError(err)

	ref, err := New(a)
	s.Require().NoError(err)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := newTestCommand(nil)

	// Act
	err = ref.Send(cancelCtx, cmd)

	// Assert
	s.Error(err)
}

// Stop tests

func (s *ActorRefTestSuite) TestStop_StartedActor_ShouldStopGracefully() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	// Act
	err = ref.Stop(100 * time.Millisecond)

	// Assert
	s.NoError(err)
	s.Equal(uint64(actor.Done), ref.State())
}

func (s *ActorRefTestSuite) TestStop_NotStartedActor_ShouldReturnError() {
	// Arrange
	a, err := actor.New(actor.WithProvider(s.provider))
	s.Require().NoError(err)

	ref, err := New(a)
	s.Require().NoError(err)

	// Act
	err = ref.Stop(100 * time.Millisecond)

	// Assert
	s.Error(err)
}

// State tests

func (s *ActorRefTestSuite) TestState_Initialized_ShouldReturnInitialized() {
	// Arrange
	a, err := actor.New(actor.WithProvider(s.provider))
	s.Require().NoError(err)

	ref, err := New(a)
	s.Require().NoError(err)

	// Act & Assert
	s.Equal(uint64(actor.Initialized), ref.State())
}

func (s *ActorRefTestSuite) TestState_Started_ShouldReturnStarted() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	// Act & Assert
	s.Equal(uint64(actor.Started), ref.State())
}

func (s *ActorRefTestSuite) TestState_AfterStop_ShouldReturnDone() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	err = a.Stop(100 * time.Millisecond)
	s.Require().NoError(err)

	// Act & Assert
	s.Equal(uint64(actor.Done), ref.State())
}

// Concurrency tests

func (s *ActorRefTestSuite) TestMultipleRefs_SameActor_ShouldWork() {
	// Arrange
	a := s.newStartedActor()
	ref1, err := New(a)
	s.Require().NoError(err)
	ref2, err := New(a)
	s.Require().NoError(err)

	var wg sync.WaitGroup
	var count int64

	// Act — send from both refs concurrently
	for i := range 20 {
		ref := ref1
		if i%2 == 0 {
			ref = ref2
		}
		wg.Go(func() {
			cmd := newTestCommand(func(ctx context.Context, entity *testEntity) {
				atomic.AddInt64(&count, 1)
			})
			errSend := ref.Send(s.ctx, cmd)
			s.NoError(errSend)
		})
	}
	wg.Wait()

	// drain processing
	time.Sleep(50 * time.Millisecond)

	// Assert
	s.Equal(int64(20), atomic.LoadInt64(&count))
}

func (s *ActorRefTestSuite) TestConcurrentSends_ShouldBeSafe() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	// Act
	for i := range goroutines {
		wg.Go(func() {
			cmd := newTestCommand(func(ctx context.Context, entity *testEntity) {
				entity.SetValue(fmt.Sprintf("value-%d", i))
			})
			errs[i] = ref.Send(s.ctx, cmd)
		})
	}
	wg.Wait()

	// Assert
	for i := range goroutines {
		s.NoError(errs[i])
	}
}

// Integration: full flow — send command, observe result, stop via ref

func (s *ActorRefTestSuite) TestIntegration_FullFlow_ShouldWork() {
	// Arrange
	a := s.newStartedActor()
	ref, err := New(a)
	s.Require().NoError(err)

	done := make(chan string, 1)
	cmd := newTestCommand(func(ctx context.Context, entity *testEntity) {
		done <- entity.GetValue()
	})

	// Act
	err = ref.Send(s.ctx, cmd)

	// Assert
	s.NoError(err)

	select {
	case val := <-done:
		s.Equal("test-value", val)
	case <-time.After(time.Second):
		s.Fail("timed out waiting for command result")
	}

	// Stop via ref
	err = ref.Stop(100 * time.Millisecond)
	s.NoError(err)
	s.Equal(uint64(actor.Done), ref.State())
}
