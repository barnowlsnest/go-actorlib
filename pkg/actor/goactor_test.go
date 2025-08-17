package actor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TestEntity is a mock entity for testing purposes
type TestEntity struct {
	value    string
	isReady  bool
	callLog  []string
	mu       sync.Mutex
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
	name       string
	executeFunc func(ctx context.Context, entity *TestEntity)
	executed   bool
	mu         sync.Mutex
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
	calls   []string
	errors  []error
	mu      sync.Mutex
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

func (suite *GoActorTestSuite) SetupTest() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())
	suite.entity = NewTestEntity("test-value", true)
	suite.provider = NewTestEntityProvider(suite.entity)
	suite.hooks = NewTestHooks()
}

func (suite *GoActorTestSuite) TearDownTest() {
	if suite.cancel != nil {
		suite.cancel()
	}
}

func TestGoActorTestSuite(t *testing.T) {
	suite.Run(t, new(GoActorTestSuite))
}

// Test actor creation and configuration
func (suite *GoActorTestSuite) TestNew_WithValidProvider_ShouldCreateActor() {
	// Act
	actor, err := New(WithProvider[*TestEntity](suite.provider))

	// Assert
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), actor)
	assert.Equal(suite.T(), uint64(Initialized), actor.State())
	assert.Equal(suite.T(), 1, actor.InputBufferSize())
	assert.NotNil(suite.T(), actor.input)
	assert.NotNil(suite.T(), actor.done)
	assert.NotNil(suite.T(), actor.ready)
}

func (suite *GoActorTestSuite) TestNew_WithoutProvider_ShouldReturnError() {
	// Act
	actor, err := New[*TestEntity]()

	// Assert
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), actor)
	assert.Equal(suite.T(), ErrActorNilProvider, err)
}

func (suite *GoActorTestSuite) TestNew_WithInputBufferSize_ShouldSetCorrectBufferSize() {
	// Arrange
	bufferSize := uint64(10)

	// Act
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithInputBufferSize[*TestEntity](bufferSize),
	)

	// Assert
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int(bufferSize), actor.InputBufferSize())
}

func (suite *GoActorTestSuite) TestNew_WithReceiveTimeout_ShouldSetCorrectTimeout() {
	// Arrange
	timeout := 10 * time.Second

	// Act
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithReceiveTimeout[*TestEntity](timeout),
	)

	// Assert
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), timeout, actor.receiveTimeout)
}

func (suite *GoActorTestSuite) TestNew_WithHooks_ShouldSetCustomHooks() {
	// Act
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](suite.hooks),
	)

	// Assert
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), suite.hooks, actor.hooks)
}

func (suite *GoActorTestSuite) TestNew_WithoutHooks_ShouldUseNoopHooks() {
	// Act
	actor, err := New(WithProvider[*TestEntity](suite.provider))

	// Assert
	require.NoError(suite.T(), err)
	assert.IsType(suite.T(), &noopHooks{}, actor.hooks)
}

// Test actor lifecycle
func (suite *GoActorTestSuite) TestStart_WithValidEntity_ShouldStartSuccessfully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](suite.hooks),
	)
	require.NoError(suite.T(), err)

	// Act
	err = actor.Start(suite.ctx)

	// Assert
	require.NoError(suite.T(), err)
	
	// Wait for actor to be ready
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)
	
	assert.Equal(suite.T(), uint64(Started), actor.State())
	
	// Check hooks were called
	calls := suite.hooks.GetCalls()
	assert.Contains(suite.T(), calls, "BeforeStart:*actor.TestEntity")
	assert.Contains(suite.T(), calls, "AfterStart")
}

func (suite *GoActorTestSuite) TestStart_WithNilEntity_ShouldReturnError() {
	// Arrange - Create a provider that returns nil
	nilProvider := &TestEntityProvider{entity: nil}
	actor, err := New(WithProvider[*TestEntity](nilProvider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.Start(suite.ctx)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorNilEntity, err)
}

func (suite *GoActorTestSuite) TestStart_WithNonProvidableEntity_ShouldReturnError() {
	// Arrange
	nonProvidableEntity := NewTestEntity("test", false)
	nonProvidableProvider := NewTestEntityProvider(nonProvidableEntity)
	actor, err := New(WithProvider[*TestEntity](nonProvidableProvider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.Start(suite.ctx)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorNilEntity, err)
}

func (suite *GoActorTestSuite) TestStop_AfterStart_ShouldStopGracefully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](suite.hooks),
	)
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	// Act
	err = actor.Stop(100 * time.Millisecond)

	// Assert
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), uint64(Done), actor.State())
	
	// Check hooks were called
	calls := suite.hooks.GetCalls()
	assert.Contains(suite.T(), calls, "BeforeStop:*actor.TestEntity")
	assert.Contains(suite.T(), calls, "AfterStop")
}

func (suite *GoActorTestSuite) TestStop_WithoutStart_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.Stop(100 * time.Millisecond)

	// Assert
	assert.Error(suite.T(), err)
}

func (suite *GoActorTestSuite) TestWaitReady_BeforeStart_ShouldTimeout() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.WaitReady(suite.ctx, 10*time.Millisecond)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), context.DeadlineExceeded, err)
}

func (suite *GoActorTestSuite) TestWaitReady_AfterStart_ShouldReturnImmediately() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)

	// Act
	start := time.Now()
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	duration := time.Since(start)

	// Assert
	require.NoError(suite.T(), err)
	assert.Less(suite.T(), duration, 50*time.Millisecond)
}

func (suite *GoActorTestSuite) TestWaitReady_WithCancelledContext_ShouldReturnContextError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	suite.cancel() // Cancel context

	// Act
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), context.Canceled, err)
}

// Test message receiving and processing
func (suite *GoActorTestSuite) TestReceive_WithValidCommand_ShouldExecuteSuccessfully() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	executed := false
	command := NewTestCommand("test-cmd", func(ctx context.Context, entity *TestEntity) {
		executed = true
		entity.SetValue("updated-by-command")
	})

	// Act
	err = actor.Receive(suite.ctx, command)
	
	// Give some time for command to execute
	time.Sleep(10 * time.Millisecond)

	// Assert
	require.NoError(suite.T(), err)
	assert.True(suite.T(), executed)
	assert.True(suite.T(), command.IsExecuted())
	
	callLog := suite.entity.GetCallLog()
	assert.Contains(suite.T(), callLog, "SetValue:updated-by-command")
}

func (suite *GoActorTestSuite) TestReceive_WithNilCommand_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)

	// Act
	err = actor.Receive(suite.ctx, nil)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorReceiveNil, err)
}

func (suite *GoActorTestSuite) TestReceive_OnStoppedActor_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)
	
	err = actor.Stop(100 * time.Millisecond)
	require.NoError(suite.T(), err)

	command := NewTestCommand("test-cmd", nil)

	// Act
	err = actor.Receive(suite.ctx, command)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorReceiveOnStopped, err)
}

func (suite *GoActorTestSuite) TestReceive_WithZeroTimeout_ShouldNotTimeout() {
	// Arrange - zero timeout means no timeout
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithInputBufferSize[*TestEntity](2),
		WithReceiveTimeout[*TestEntity](0), // No timeout
	)
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	command := NewTestCommand("test", func(ctx context.Context, entity *TestEntity) {
		entity.SetValue("updated")
	})

	// Act
	err = actor.Receive(suite.ctx, command)

	// Assert
	assert.NoError(suite.T(), err)
}

// Test error handling and panic recovery
func (suite *GoActorTestSuite) TestExecute_WithPanicInCommand_ShouldRecover() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](suite.hooks),
	)
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	panicCommand := NewTestCommand("panic-cmd", func(ctx context.Context, entity *TestEntity) {
		panic("test panic")
	})

	// Act
	err = actor.Receive(suite.ctx, panicCommand)
	require.NoError(suite.T(), err)
	
	// Give time for panic to be handled
	time.Sleep(20 * time.Millisecond)

	// Assert
	assert.Equal(suite.T(), uint64(Panicked), actor.State())
	
	hookErrors := suite.hooks.GetErrors()
	assert.NotEmpty(suite.T(), hookErrors)
	
	foundPanicError := false
	for _, hookErr := range hookErrors {
		if errors.Is(hookErr, ErrActorPanic) {
			foundPanicError = true
			break
		}
	}
	assert.True(suite.T(), foundPanicError)
}

// PanicHooks implements Hooks interface and panics in AfterStart
type PanicHooks struct {
	*TestHooks
}

func (ph *PanicHooks) AfterStart() {
	ph.TestHooks.AfterStart()
	panic("hook panic")
}

func (suite *GoActorTestSuite) TestExecute_WithHookPanic_ShouldRecover() {
	// Arrange
	baseHooks := NewTestHooks()
	panicHooks := &PanicHooks{TestHooks: baseHooks}
	
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](panicHooks),
	)
	require.NoError(suite.T(), err)

	// Act
	err = actor.Start(suite.ctx)

	// Assert
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)
	
	// Actor should still be running despite hook panic
	// But it may have transitioned to Panicked state due to the panic
	state := actor.State()
	assert.True(suite.T(), state == uint64(Started) || state == uint64(Panicked))
}

// Test context handling
func (suite *GoActorTestSuite) TestStart_WithCancelledContext_ShouldHandleGracefully() {
	// Arrange
	actor, err := New(
		WithProvider[*TestEntity](suite.provider),
		WithHooks[*TestEntity](suite.hooks),
	)
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	// Act
	suite.cancel()
	
	// Give time for context cancellation to be processed
	time.Sleep(20 * time.Millisecond)

	// Assert
	assert.Equal(suite.T(), uint64(Cancelled), actor.State())
	
	hookErrors := suite.hooks.GetErrors()
	assert.NotEmpty(suite.T(), hookErrors)
	
	foundContextError := false
	for _, hookErr := range hookErrors {
		if errors.Is(hookErr, ErrActorContextCanceled) {
			foundContextError = true
			break
		}
	}
	assert.True(suite.T(), foundContextError)
}

// Test state transitions
func (suite *GoActorTestSuite) TestStateTransitions_ShouldFollowCorrectSequence() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)

	// Assert initial state
	assert.Equal(suite.T(), uint64(Initialized), actor.State())

	// Act & Assert - Start
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)
	
	assert.Equal(suite.T(), uint64(Started), actor.State())

	// Act & Assert - Stop
	err = actor.Stop(100 * time.Millisecond)
	require.NoError(suite.T(), err)
	
	assert.Equal(suite.T(), uint64(Done), actor.State())
}

func (suite *GoActorTestSuite) TestCheckState_WithCorrectState_ShouldReturnNil() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.CheckState(Initialized)

	// Assert
	assert.NoError(suite.T(), err)
}

func (suite *GoActorTestSuite) TestCheckState_WithIncorrectState_ShouldReturnError() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)

	// Act
	err = actor.CheckState(Started)

	// Assert
	assert.Error(suite.T(), err)
}

// Test concurrent access
func (suite *GoActorTestSuite) TestConcurrentAccess_ShouldBeSafe() {
	// Arrange
	actor, err := New(WithProvider[*TestEntity](suite.provider))
	require.NoError(suite.T(), err)
	
	err = actor.Start(suite.ctx)
	require.NoError(suite.T(), err)
	
	err = actor.WaitReady(suite.ctx, 100*time.Millisecond)
	require.NoError(suite.T(), err)

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
			
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			
			actor.Receive(ctx, command)
		}(i)
	}

	// Concurrent state reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				state := actor.State()
				assert.True(suite.T(), state >= Initialized && state <= Panicked)
			}
		}()
	}

	wg.Wait()

	// Give time for all commands to execute
	time.Sleep(200 * time.Millisecond)

	// Assert
	assert.Greater(suite.T(), atomic.LoadInt64(&executionCount), int64(0))
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
		
		actor.WaitReady(ctx, time.Second)
		actor.Stop(time.Second)
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
		
		err := actor.Receive(ctx, command)
		if err != nil {
			b.Fatal(err)
		}
	}
	
	actor.Stop(time.Second)
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
	
	actor.Stop(time.Second)
}