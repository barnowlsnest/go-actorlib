package actor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type BehaviorTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	entity   *TestEntity
	provider *TestEntityProvider
}

func (s *BehaviorTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = NewTestEntity("test-value", true)
	s.provider = NewTestEntityProvider(s.entity)
}

func (s *BehaviorTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestBehaviorTestSuite(t *testing.T) {
	suite.Run(t, new(BehaviorTestSuite))
}

func (s *BehaviorTestSuite) TestBehaviorStack_Current_ShouldReturnInitialHandler() {
	// Arrange
	var called bool
	handler := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {
		called = true
	}
	stack := newBehaviorStack(handler)

	// Act
	stack.Current()(context.Background(), nil, nil)

	// Assert
	s.True(called)
	s.Equal(1, stack.Depth())
}

func (s *BehaviorTestSuite) TestBehaviorStack_Become_ShouldSwitchHandler() {
	// Arrange
	var handlerID int
	initial := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 1 }
	replacement := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 2 }
	stack := newBehaviorStack(initial)

	// Act
	stack.Become(replacement)
	stack.Current()(context.Background(), nil, nil)

	// Assert
	s.Equal(2, handlerID)
	s.Equal(2, stack.Depth())
}

func (s *BehaviorTestSuite) TestBehaviorStack_Unbecome_ShouldRestorePrevious() {
	// Arrange
	var handlerID int
	initial := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 1 }
	replacement := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 2 }
	stack := newBehaviorStack(initial)
	stack.Become(replacement)

	// Act
	ok := stack.Unbecome()
	stack.Current()(context.Background(), nil, nil)

	// Assert
	s.True(ok)
	s.Equal(1, handlerID)
	s.Equal(1, stack.Depth())
}

func (s *BehaviorTestSuite) TestBehaviorStack_Unbecome_OnInitialBehavior_ShouldReturnFalse() {
	// Arrange
	handler := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) {}
	stack := newBehaviorStack(handler)

	// Act
	ok := stack.Unbecome()

	// Assert
	s.False(ok)
	s.Equal(1, stack.Depth())
}

func (s *BehaviorTestSuite) TestBehaviorStack_BecomeReplace_ShouldReplaceWithoutPush() {
	// Arrange
	var handlerID int
	initial := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 1 }
	replacement := func(_ context.Context, _ Executable[*TestEntity], _ *TestEntity) { handlerID = 2 }
	stack := newBehaviorStack(initial)

	// Act
	stack.BecomeReplace(replacement)
	stack.Current()(context.Background(), nil, nil)

	// Assert
	s.Equal(2, handlerID)
	s.Equal(1, stack.Depth()) // No push, same depth
}

func (s *BehaviorTestSuite) TestGoActorContext_Become_ShouldSwitchActorBehavior() {
	// Arrange
	var handlerVersion atomic.Int64
	handlerVersion.Store(1)

	a, err := New(
		WithProvider[*TestEntity](s.provider),
		WithInputBufferSize[*TestEntity](10),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	// First command — switch behavior via GoActorContext
	switchCmd := NewTestCommand("switch", func(ctx context.Context, _ *TestEntity) {
		ac := GetGoActorContext[*TestEntity](ctx)
		s.Require().NotNil(ac)
		ac.Become(func(_ context.Context, e Executable[*TestEntity], entity *TestEntity) {
			handlerVersion.Store(2)
			e.Execute(context.Background(), entity)
		})
	})

	s.NoError(a.Receive(s.ctx, switchCmd))
	time.Sleep(20 * time.Millisecond)

	// Second command — should use the new behavior
	verifyCmd := NewTestCommand("verify", func(_ context.Context, _ *TestEntity) {
		// This runs inside the new handler, which sets version to 2
	})

	s.NoError(a.Receive(s.ctx, verifyCmd))
	time.Sleep(20 * time.Millisecond)

	// Assert
	s.Equal(int64(2), handlerVersion.Load())
}

func (s *BehaviorTestSuite) TestGoActorContext_Unbecome_ShouldRestorePreviousBehavior() {
	// Arrange
	var handlerVersion atomic.Int64

	a, err := New(
		WithProvider[*TestEntity](s.provider),
		WithInputBufferSize[*TestEntity](10),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	// Command 1 — switch to custom behavior
	switchCmd := NewTestCommand("switch", func(ctx context.Context, _ *TestEntity) {
		ac := GetGoActorContext[*TestEntity](ctx)
		ac.Become(func(innerCtx context.Context, e Executable[*TestEntity], entity *TestEntity) {
			handlerVersion.Store(2)
			e.Execute(innerCtx, entity)
		})
	})
	s.NoError(a.Receive(s.ctx, switchCmd))
	time.Sleep(20 * time.Millisecond)

	// Command 2 — verify custom behavior and unbecome
	unbecomeCmd := NewTestCommand("unbecome", func(ctx context.Context, _ *TestEntity) {
		// This executes inside handler v2 which sets handlerVersion to 2
		ac := GetGoActorContext[*TestEntity](ctx)
		ac.Unbecome()
	})
	s.NoError(a.Receive(s.ctx, unbecomeCmd))
	time.Sleep(20 * time.Millisecond)
	s.Equal(int64(2), handlerVersion.Load())

	// Command 3 — should use original behavior (default handler)
	handlerVersion.Store(0)
	verifyCmd := NewTestCommand("verify", func(_ context.Context, _ *TestEntity) {
		handlerVersion.Store(1) // Original handler just executes the command
	})
	s.NoError(a.Receive(s.ctx, verifyCmd))
	time.Sleep(20 * time.Millisecond)

	// Assert — original handler executed the command directly
	s.Equal(int64(1), handlerVersion.Load())
}

func (s *BehaviorTestSuite) TestGoActorContext_Name_ShouldReturnActorName() {
	// Arrange
	var receivedName atomic.Value

	a, err := New(
		WithProvider[*TestEntity](s.provider),
		WithName[*TestEntity]("my-actor"),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)
	s.Require().NoError(a.WaitReady(s.ctx, 100*time.Millisecond))

	cmd := NewTestCommand("check-name", func(ctx context.Context, _ *TestEntity) {
		ac := GetGoActorContext[*TestEntity](ctx)
		receivedName.Store(ac.Name())
	})

	// Act
	s.NoError(a.Receive(s.ctx, cmd))
	time.Sleep(20 * time.Millisecond)

	// Assert
	s.Equal("my-actor", receivedName.Load())
	s.Equal("my-actor", a.Name())
}

func (s *BehaviorTestSuite) TestGetGoActorContext_OutsideActor_ShouldReturnNil() {
	// Act
	ac := GetGoActorContext[*TestEntity](context.Background())

	// Assert
	s.Nil(ac)
}
