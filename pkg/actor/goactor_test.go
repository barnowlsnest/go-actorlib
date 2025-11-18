package actor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// TestEntity is a mock entity for testing purposes
type TestEntity struct {
	value   string
	isReady bool
	callLog []string
	mu      sync.Mutex
}

func NewTestEntity(value string, isReady bool) *TestEntity {
	return &TestEntity{
		value:   value,
		isReady: isReady,
		callLog: make([]string, 0),
	}
}

func (te *TestEntity) IsProvidable() bool {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, "IsProvidable")
	return te.isReady
}

func (te *TestEntity) GetValue() string {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, "GetValue")
	return te.value
}

func (te *TestEntity) SetValue(value string) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = append(te.callLog, fmt.Sprintf("SetValue:%s", value))
	te.value = value
}

func (te *TestEntity) GetCallLog() []string {
	te.mu.Lock()
	defer te.mu.Unlock()
	result := make([]string, len(te.callLog))
	copy(result, te.callLog)
	return result
}

func (te *TestEntity) ClearCallLog() {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.callLog = te.callLog[:0]
}

// TestEntityProvider provides test entities
type TestEntityProvider struct {
	entity *TestEntity
}

func NewTestEntityProvider(entity *TestEntity) *TestEntityProvider {
	return &TestEntityProvider{entity: entity}
}

func (tep *TestEntityProvider) Provide() *TestEntity {
	return tep.entity
}

// TestCommand implements Executable for testing
type TestCommand struct {
	name        string
	executeFunc func(ctx context.Context, entity *TestEntity)
	executed    bool
	mu          sync.Mutex
}

func NewTestCommand(name string, executeFunc func(ctx context.Context, entity *TestEntity)) *TestCommand {
	return &TestCommand{
		name:        name,
		executeFunc: executeFunc,
	}
}

func (tc *TestCommand) Execute(ctx context.Context, entity *TestEntity) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.executed = true
	if tc.executeFunc != nil {
		tc.executeFunc(ctx, entity)
	}
}

func (tc *TestCommand) IsExecuted() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.executed
}

func (tc *TestCommand) Name() string {
	return tc.name
}

// TestHooks implements Hooks interface for testing
type TestHooks struct {
	calls  []string
	errors []error
	mu     sync.Mutex
}

func NewTestHooks() *TestHooks {
	return &TestHooks{
		calls:  make([]string, 0),
		errors: make([]error, 0),
	}
}

func (th *TestHooks) AfterStart() {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = append(th.calls, "AfterStart")
}

func (th *TestHooks) AfterStop() {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = append(th.calls, "AfterStop")
}

func (th *TestHooks) BeforeStart(entity any) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = append(th.calls, fmt.Sprintf("BeforeStart:%T", entity))
}

func (th *TestHooks) BeforeStop(entity any) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = append(th.calls, fmt.Sprintf("BeforeStop:%T", entity))
}

func (th *TestHooks) OnError(err error) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = append(th.calls, fmt.Sprintf("OnError:%v", err))
	th.errors = append(th.errors, err)
}

func (th *TestHooks) GetCalls() []string {
	th.mu.Lock()
	defer th.mu.Unlock()
	result := make([]string, len(th.calls))
	copy(result, th.calls)
	return result
}

func (th *TestHooks) GetErrors() []error {
	th.mu.Lock()
	defer th.mu.Unlock()
	result := make([]error, len(th.errors))
	copy(result, th.errors)
	return result
}

func (th *TestHooks) ClearCalls() {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.calls = th.calls[:0]
	th.errors = th.errors[:0]
}

// GoActorTestSuite provides a test suite for GoActor
type GoActorTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	entity   *TestEntity
	provider *TestEntityProvider
	hooks    *TestHooks
}

func (s *GoActorTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.entity = NewTestEntity("test-value", true)
	s.provider = NewTestEntityProvider(s.entity)
	s.hooks = NewTestHooks()
}

func (s *GoActorTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestGoActorTestSuite(t *testing.T) {
	suite.Run(t, new(GoActorTestSuite))
}

// Test actor creation and configuration
func (s *GoActorTestSuite) TestNew_WithValidProvider_ShouldCreateActor() {
	// Act
	actor, err := New(WithProvider[*TestEntity](s.provider))

	// Assert
	s.NoError(err)
	s.NotNil(actor)
	s.Equal(uint64(Initialized), actor.State())
	s.Equal(1, actor.InputBufferSize())
	s.NotNil(actor.input)
	s.NotNil(actor.done)
	s.NotNil(actor.ready)
}

func (s *GoActorTestSuite) TestNew_WithoutProvider_ShouldReturnError() {
	// Act
	actor, err := New[*TestEntity]()

	// Assert
	s.Error(err)
	s.Nil(actor)
	s.Equal(ErrActorNilProvider, err)
}

func (s *GoActorTestSuite) TestNew_WithInputBufferSize_ShouldSetCorrectBufferSize() {
	// Arrange
	bufferSize := uint64(10)

	// Act
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithInputBufferSize[*TestEntity](bufferSize),
	)

	// Assert
	s.NoError(err)
	s.Equal(int(bufferSize), actor.InputBufferSize())
}

func (s *GoActorTestSuite) TestNew_WithReceiveTimeout_ShouldSetCorrectTimeout() {
	// Arrange
	timeout := 10 * time.Second

	// Act
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithReceiveTimeout[*TestEntity](timeout),
	)

	// Assert
	s.NoError(err)
	s.Equal(timeout, actor.receiveTimeout)
}

func (s *GoActorTestSuite) TestNew_WithHooks_ShouldSetCustomHooks() {
	// Act
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
	)

	// Assert
	s.NoError(err)
	s.Equal(s.hooks, actor.hooks)
}

func (s *GoActorTestSuite) TestNew_WithoutHooks_ShouldUseNoopHooks() {
	// Act
	actor, err := New(WithProvider[*TestEntity](s.provider))

	// Assert
	s.NoError(err)
	s.IsType(&noopHooks{}, actor.hooks)
}

// Test actor lifecycle
func (s *GoActorTestSuite) TestStart_WithValidEntity_ShouldStartSuccessfully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
	)
	s.NoError(err)

	// Act
	err = actor.Start(s.ctx)

	// Assert
	s.NoError(err)

	// Wait for actor to be ready
	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	s.Equal(uint64(Started), actor.State())

	// Check hooks were called
	calls := s.hooks.GetCalls()
	s.Contains(calls, "BeforeStart:*actor.TestEntity")
	s.Contains(calls, "AfterStart")
}

func (s *GoActorTestSuite) TestStart_WithNilEntity_ShouldReturnError() {
	// Arrange - Create a provider that returns nil
	nilProvider := &TestEntityProvider{entity: nil}
	actor, err := New(WithProvider[*TestEntity](nilProvider))
	s.NoError(err)

	// Act
	err = actor.Start(s.ctx)

	// Assert
	s.Error(err)
	s.Equal(ErrActorNilEntity, err)
}

func (s *GoActorTestSuite) TestStart_WithNonProvidableEntity_ShouldReturnError() {
	// Arrange
	nonProvidableEntity := NewTestEntity("test", false)
	nonProvidableProvider := NewTestEntityProvider(nonProvidableEntity)
	actor, err := New(WithProvider[*TestEntity](nonProvidableProvider))
	s.NoError(err)

	// Act
	err = actor.Start(s.ctx)

	// Assert
	s.Error(err)
	s.Equal(ErrActorNilEntity, err)
}

func (s *GoActorTestSuite) TestStop_AfterStart_ShouldStopGracefully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
	)
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	// Act
	err = actor.Stop(100 * time.Millisecond)

	// Assert
	s.NoError(err)
	s.Equal(uint64(Done), actor.State())

	// Check hooks were called
	calls := s.hooks.GetCalls()
	s.Contains(calls, "BeforeStop:*actor.TestEntity")
	s.Contains(calls, "AfterStop")
}

func (s *GoActorTestSuite) TestStop_WithoutStart_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	// Act
	err = actor.Stop(100 * time.Millisecond)

	// Assert
	s.Error(err)
}

func (s *GoActorTestSuite) TestWaitReady_BeforeStart_ShouldTimeout() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	// Act
	err = actor.WaitReady(s.ctx, 10*time.Millisecond)

	// Assert
	s.Error(err)
	s.Equal(context.DeadlineExceeded, err)
}

func (s *GoActorTestSuite) TestWaitReady_AfterStart_ShouldReturnImmediately() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	// Act
	start := time.Now()
	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	duration := time.Since(start)

	// Assert
	s.NoError(err)
	s.Less(duration, 50*time.Millisecond)
}

func (s *GoActorTestSuite) TestWaitReady_WithCancelledContext_ShouldReturnContextError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	s.cancel() // Cancel context

	// Act
	err = actor.WaitReady(s.ctx, 100*time.Millisecond)

	// Assert
	s.Error(err)
	s.Equal(context.Canceled, err)
}

// Test message receiving and processing
func (s *GoActorTestSuite) TestReceive_WithValidCommand_ShouldExecuteSuccessfully() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	executed := false
	command := NewTestCommand("test-cmd", func(ctx context.Context, entity *TestEntity) {
		executed = true
		entity.SetValue("updated-by-command")
	})

	// Act
	err = actor.Receive(s.ctx, command)

	// Give some time for command to execute
	time.Sleep(10 * time.Millisecond)

	// Assert
	s.NoError(err)
	s.True(executed)
	s.True(command.IsExecuted())

	callLog := s.entity.GetCallLog()
	s.Contains(callLog, "SetValue:updated-by-command")
}

func (s *GoActorTestSuite) TestReceive_WithNilCommand_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	// Act
	err = actor.Receive(s.ctx, nil)

	// Assert
	s.Error(err)
	s.Equal(ErrActorReceiveNil, err)
}

func (s *GoActorTestSuite) TestReceive_OnStoppedActor_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	err = actor.Stop(100 * time.Millisecond)
	s.NoError(err)

	command := NewTestCommand("test-cmd", nil)

	// Act
	err = actor.Receive(s.ctx, command)

	// Assert
	s.Error(err)
	s.Equal(ErrActorReceiveOnStopped, err)
}

func (s *GoActorTestSuite) TestReceive_WithZeroTimeout_ShouldNotTimeout() {
	// Arrange - zero timeout means no timeout
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithInputBufferSize[*TestEntity](2),
		WithReceiveTimeout[*TestEntity](0), // No timeout
	)
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	command := NewTestCommand("test", func(ctx context.Context, entity *TestEntity) {
		entity.SetValue("updated")
	})

	// Act
	err = actor.Receive(s.ctx, command)

	// Assert
	s.NoError(err)
}

// Test error handling and panic recovery
func (s *GoActorTestSuite) TestExecute_WithPanicInCommand_ShouldRecover() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
	)
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	panicCommand := NewTestCommand("panic-cmd", func(ctx context.Context, entity *TestEntity) {
		panic("test panic")
	})

	// Act
	err = actor.Receive(s.ctx, panicCommand)
	s.NoError(err)

	// Give time for panic to be handled
	time.Sleep(20 * time.Millisecond)

	// Assert
	s.Equal(uint64(Panicked), actor.State())

	hookErrors := s.hooks.GetErrors()
	s.NotEmpty(hookErrors)

	foundPanicError := false
	for _, hookErr := range hookErrors {
		if errors.Is(hookErr, ErrActorPanic) {
			foundPanicError = true
			break
		}
	}
	s.True(foundPanicError)
}

// PanicHooks implements Hooks interface and panics in AfterStart
type PanicHooks struct {
	*TestHooks
}

func (ph *PanicHooks) AfterStart() {
	ph.TestHooks.AfterStart()
	panic("hook panic")
}

func (s *GoActorTestSuite) TestExecute_WithHookPanic_ShouldRecover() {
	// Arrange
	baseHooks := NewTestHooks()
	panicHooks := &PanicHooks{TestHooks: baseHooks}

	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](panicHooks),
	)
	s.NoError(err)

	// Act
	err = actor.Start(s.ctx)

	// Assert
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	// Actor should still be running despite hook panic
	// But it may have transitioned to Panicked state due to the panic
	state := actor.State()
	s.True(state == uint64(Started) || state == uint64(Panicked))
}

// Test context handling
func (s *GoActorTestSuite) TestStart_WithCancelledContext_ShouldHandleGracefully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](s.provider),
		WithHooks[*TestEntity](s.hooks),
	)
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	// Act
	s.cancel()

	// Give time for context cancellation to be processed
	time.Sleep(20 * time.Millisecond)

	// Assert
	s.Equal(uint64(Canceled), actor.State())

	hookErrors := s.hooks.GetErrors()
	s.NotEmpty(hookErrors)

	foundContextError := false
	for _, hookErr := range hookErrors {
		if errors.Is(hookErr, ErrActorContextCanceled) {
			foundContextError = true
			break
		}
	}
	s.True(foundContextError)
}

// Test state transitions
func (s *GoActorTestSuite) TestStateTransitions_ShouldFollowCorrectSequence() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	// Assert initial state
	s.Equal(uint64(Initialized), actor.State())

	// Act & Assert - Start
	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	s.Equal(uint64(Started), actor.State())

	// Act & Assert - Stop
	err = actor.Stop(100 * time.Millisecond)
	s.NoError(err)

	s.Equal(uint64(Done), actor.State())
}

func (s *GoActorTestSuite) TestCheckState_WithCorrectState_ShouldReturnNil() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	// Act
	err = actor.CheckState(Initialized)

	// Assert
	s.NoError(err)
}

func (s *GoActorTestSuite) TestCheckState_WithIncorrectState_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	// Act
	err = actor.CheckState(Started)

	// Assert
	s.Error(err)
}

// Test concurrent access
func (s *GoActorTestSuite) TestConcurrentAccess_ShouldBeSafe() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](s.provider))
	s.NoError(err)

	err = actor.Start(s.ctx)
	s.NoError(err)

	err = actor.WaitReady(s.ctx, 100*time.Millisecond)
	s.NoError(err)

	var wg sync.WaitGroup
	var executionCount int64

	// Act - Send multiple commands concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(cmdNum int) {
			defer wg.Done()
			command := NewTestCommand(fmt.Sprintf("cmd-%d", cmdNum), func(ctx context.Context, entity *TestEntity) {
				atomic.AddInt64(&executionCount, 1)
				time.Sleep(time.Millisecond) // Simulate work
			})

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			errReceive := actor.Receive(ctx, command)
			s.NoError(errReceive)
		}(i)
	}

	// Concurrent state reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state := actor.State()
				s.True(state >= Initialized && state <= Panicked)
			}
		}()
	}

	wg.Wait()

	// Give time for all commands to execute
	time.Sleep(200 * time.Millisecond)

	// Assert
	s.Greater(atomic.LoadInt64(&executionCount), int64(0))
}

// Benchmark tests
func BenchmarkGoActor_CreateAndStart(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	provider := NewTestEntityProvider(entity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		actor, err := New(WithProvider[*TestEntity](provider))
		if err != nil {
			b.Fatal(err)
		}

		ctx := context.Background()
		err = actor.Start(ctx)
		if err != nil {
			b.Fatal(err)
		}

		if errReady := actor.WaitReady(ctx, time.Second); errReady != nil {
			b.Fatal(errReady)
		}

		if errStop := actor.Stop(time.Second); errStop != nil {
			b.Fatal(errStop)
		}
	}
}

func BenchmarkGoActor_ReceiveCommand(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	provider := NewTestEntityProvider(entity)

	actor, err := New(WithProvider[*TestEntity](provider))
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	err = actor.Start(ctx)
	if err != nil {
		b.Fatal(err)
	}

	err = actor.WaitReady(ctx, time.Second)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		command := NewTestCommand("bench-cmd", func(ctx context.Context, entity *TestEntity) {
			entity.GetValue()
		})

		if errReceive := actor.Receive(ctx, command); errReceive != nil {
			b.Fatal(errReceive)
		}
	}

	if errStop := actor.Stop(time.Second); errStop != nil {
		b.Fatal(errStop)
	}
}

func BenchmarkGoActor_ConcurrentStateAccess(b *testing.B) {
	entity := NewTestEntity("bench-test", true)
	provider := NewTestEntityProvider(entity)

	actor, err := New(WithProvider[*TestEntity](provider))
	if err != nil {
		b.Fatal(err)
	}

	ctx := context.Background()
	err = actor.Start(ctx)
	if err != nil {
		b.Fatal(err)
	}

	err = actor.WaitReady(ctx, time.Second)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			actor.State()
		}
	})

	if errStop := actor.Stop(time.Second); errStop != nil {
		b.Fatal(errStop)
	}
}
