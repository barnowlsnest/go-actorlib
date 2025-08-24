package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/pkg/actor"
)

// MockActor implements both Runnable and Actionable interfaces for testing
type MockActor struct {
	name       string
	state      uint64
	startError error
	stopError  error
	waitError  error
	commands   []actor.Executable[actor.Entity]
	mu         sync.Mutex
	started    bool
	stopped    bool
}

func NewMockActor(name string, initialState uint64) *MockActor {
	return &MockActor{
		name:     name,
		state:    initialState,
		commands: make([]actor.Executable[actor.Entity], 0),
	}
}

func (ma *MockActor) Start(ctx context.Context) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if ma.startError != nil {
		return ma.startError
	}

	ma.started = true
	atomic.StoreUint64(&ma.state, uint64(actor.Started))
	return nil
}

func (ma *MockActor) Stop(timeout time.Duration) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if ma.stopError != nil {
		return ma.stopError
	}

	ma.stopped = true
	atomic.StoreUint64(&ma.state, uint64(actor.Done))
	return nil
}

func (ma *MockActor) WaitReady(ctx context.Context, timeout time.Duration) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	if ma.waitError != nil {
		return ma.waitError
	}

	if !ma.started {
		return context.DeadlineExceeded
	}

	return nil
}

func (ma *MockActor) State() uint64 {
	return atomic.LoadUint64(&ma.state)
}

func (ma *MockActor) Receive(ctx context.Context, executable actor.Executable[actor.Entity]) error {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	ma.commands = append(ma.commands, executable)
	return nil
}

func (ma *MockActor) SetState(state uint64) {
	atomic.StoreUint64(&ma.state, state)
}

func (ma *MockActor) SetStartError(err error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.startError = err
}

func (ma *MockActor) SetStopError(err error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.stopError = err
}

func (ma *MockActor) SetWaitError(err error) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	ma.waitError = err
}

func (ma *MockActor) GetCommands() []actor.Executable[actor.Entity] {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	result := make([]actor.Executable[actor.Entity], len(ma.commands))
	copy(result, ma.commands)
	return result
}

func (ma *MockActor) IsStarted() bool {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return ma.started
}

func (ma *MockActor) IsStopped() bool {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	return ma.stopped
}

func (ma *MockActor) Name() string {
	return ma.name
}

// MockEntity for testing command dispatching
type MockEntity struct {
	value string
}

func (me *MockEntity) IsProvidable() bool {
	return true
}

// MockCommand for testing command dispatching
type MockCommand struct {
	name     string
	executed bool
	mu       sync.Mutex
}

func NewMockCommand(name string) *MockCommand {
	return &MockCommand{name: name}
}

func (mc *MockCommand) Execute(ctx context.Context, entity actor.Entity) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.executed = true
}

func (mc *MockCommand) IsExecuted() bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.executed
}

func (mc *MockCommand) Name() string {
	return mc.name
}

// NotRunnableActor for testing error cases
type NotRunnableActor struct {
	name string
}

func (nra *NotRunnableActor) Name() string {
	return nra.name
}

// NotActionableActor implements Runnable but not Actionable
type NotActionableActor struct {
	name string
}

func (naa *NotActionableActor) Start(ctx context.Context) error  { return nil }
func (naa *NotActionableActor) Stop(timeout time.Duration) error { return nil }
func (naa *NotActionableActor) WaitReady(ctx context.Context, timeout time.Duration) error {
	return nil
}
func (naa *NotActionableActor) State() uint64 { return uint64(actor.Initialized) }
func (naa *NotActionableActor) Name() string  { return naa.name }

// GoRegistryTestSuite provides a test suite for GoRegistry
type GoRegistryTestSuite struct {
	suite.Suite
	ctx      context.Context
	cancel   context.CancelFunc
	registry *GoRegistry
}

func (suite *GoRegistryTestSuite) SetupTest() {
	suite.ctx, suite.cancel = context.WithCancel(context.Background())
	suite.registry = New("test-registry")
}

func (suite *GoRegistryTestSuite) TearDownTest() {
	if suite.cancel != nil {
		suite.cancel()
	}
}

func TestGoRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(GoRegistryTestSuite))
}

// Test registry creation and configuration
func (suite *GoRegistryTestSuite) TestNew_WithDefaultOptions_ShouldCreateRegistry() {
	// Act
	registry := New("test-tag")

	// Assert
	require.NotNil(suite.T(), registry)
	assert.Equal(suite.T(), "test-tag", registry.Tag())
	assert.Equal(suite.T(), 5*time.Second, registry.startTimeout)
	assert.Equal(suite.T(), 5*time.Second, registry.stopTimeout)
	assert.NotNil(suite.T(), registry.actors)
	assert.NotNil(suite.T(), registry.lookupTable)
}

func (suite *GoRegistryTestSuite) TestNew_WithCustomTimeouts_ShouldSetCorrectValues() {
	// Arrange
	startTimeout := 10 * time.Second
	stopTimeout := 15 * time.Second

	// Act
	registry := New("test-tag",
		WithStartTimeout(startTimeout),
		WithStopTimeout(stopTimeout),
	)

	// Assert
	assert.Equal(suite.T(), startTimeout, registry.startTimeout)
	assert.Equal(suite.T(), stopTimeout, registry.stopTimeout)
}

func (suite *GoRegistryTestSuite) TestTag_ShouldReturnCorrectTag() {
	// Arrange
	expectedTag := "custom-registry-tag"
	registry := New(expectedTag)

	// Act
	tag := registry.Tag()

	// Assert
	assert.Equal(suite.T(), expectedTag, tag)
}

// Test actor registration and lookup
func (suite *GoRegistryTestSuite) TestRegister_WithValidActor_ShouldReturnUUID() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))

	// Act
	id, err := suite.registry.Register("test-actor", mockActor)

	// Assert
	require.NoError(suite.T(), err)
	assert.NotEqual(suite.T(), uuid.Nil, id)
}

func (suite *GoRegistryTestSuite) TestRegister_WithNilActor_ShouldReturnError() {
	// Act
	id, err := suite.registry.Register("nil-actor", nil)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), uuid.Nil, id)
	assert.Equal(suite.T(), ErrNilActor, err)
}

func (suite *GoRegistryTestSuite) TestRegister_WithNotRunnableActor_ShouldReturnError() {
	// Arrange
	notRunnable := &NotRunnableActor{name: "not-runnable"}

	// Act
	id, err := suite.registry.Register("not-runnable", notRunnable)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), uuid.Nil, id)
	assert.Equal(suite.T(), ErrActorIsNotRunnable, err)
}

func (suite *GoRegistryTestSuite) TestRegister_WithNotActionableActor_ShouldReturnError() {
	// Arrange
	notActionable := &NotActionableActor{name: "not-actionable"}

	// Act
	id, err := suite.registry.Register("not-actionable", notActionable)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), uuid.Nil, id)
	assert.Equal(suite.T(), ErrActorIsNotActionable, err)
}

func (suite *GoRegistryTestSuite) TestRegister_WithInvalidState_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("invalid-state", uint64(actor.Done))

	// Act
	id, err := suite.registry.Register("invalid-state", mockActor)

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), uuid.Nil, id)
	assert.Equal(suite.T(), ErrActorIsNotRunnable, err)
}

func (suite *GoRegistryTestSuite) TestGet_WithExistingActor_ShouldReturnModel() {
	// Arrange
	mockActor := NewMockActor("existing-actor", uint64(actor.Initialized))
	id, err := suite.registry.Register("existing-actor", mockActor)
	require.NoError(suite.T(), err)

	// Act
	model, err := suite.registry.Get("existing-actor")

	// Assert
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), model)
	assert.Equal(suite.T(), id, model.ID)
	assert.Equal(suite.T(), "existing-actor", model.Name)
	assert.Equal(suite.T(), uint64(actor.Initialized), model.State)
}

func (suite *GoRegistryTestSuite) TestGet_WithNonExistentActor_ShouldReturnError() {
	// Act
	model, err := suite.registry.Get("non-existent")

	// Assert
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), model)
	assert.Equal(suite.T(), ErrActorNotFound, err)
}

func (suite *GoRegistryTestSuite) TestGetAll_WithMultipleActors_ShouldReturnAllModels() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", actor.Started)

	id1, err := suite.registry.Register("actor1", actor1)
	require.NoError(suite.T(), err)

	id2, err := suite.registry.Register("actor2", actor2)
	require.NoError(suite.T(), err)

	// Act
	models := suite.registry.GetAll()

	// Assert
	assert.Len(suite.T(), models, 2)

	// Find models by ID
	var model1, model2 *Model
	for i := range models {
		if models[i].ID == id1 {
			model1 = &models[i]
		} else if models[i].ID == id2 {
			model2 = &models[i]
		}
	}

	require.NotNil(suite.T(), model1)
	require.NotNil(suite.T(), model2)

	assert.Equal(suite.T(), "actor1", model1.Name)
	assert.Equal(suite.T(), uint64(actor.Initialized), model1.State)

	assert.Equal(suite.T(), "actor2", model2.Name)
	assert.Equal(suite.T(), uint64(actor.Started), model2.State)
}

func (suite *GoRegistryTestSuite) TestGetAll_WithNoActors_ShouldReturnEmptySlice() {
	// Act
	models := suite.registry.GetAll()

	// Assert
	assert.Empty(suite.T(), models)
	assert.NotNil(suite.T(), models)
}

// Test actor lifecycle management
func (suite *GoRegistryTestSuite) TestStart_WithValidActor_ShouldStartSuccessfully() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := suite.registry.Register("test-actor", mockActor)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Start(suite.ctx, "test-actor")

	// Assert
	require.NoError(suite.T(), err)
	assert.True(suite.T(), mockActor.IsStarted())
	assert.Equal(suite.T(), uint64(actor.Started), mockActor.State())
}

func (suite *GoRegistryTestSuite) TestStart_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := suite.registry.Start(suite.ctx, "non-existent")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorNotFound)
}

func (suite *GoRegistryTestSuite) TestStart_WithAlreadyStartedActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("started-actor", uint64(actor.Started))
	_, err := suite.registry.Register("started-actor", mockActor)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Start(suite.ctx, "started-actor")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorAlreadyStarted)
}

func (suite *GoRegistryTestSuite) TestStart_WithWaitReadyFailure_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("wait-fail-actor", uint64(actor.Initialized))
	mockActor.SetWaitError(errors.New("wait failed"))
	_, err := suite.registry.Register("wait-fail-actor", mockActor)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Start(suite.ctx, "wait-fail-actor")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorIsNotRunnable)
}

func (suite *GoRegistryTestSuite) TestStartAll_WithMultipleActors_ShouldStartAll() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", uint64(actor.Initialized))

	_, err := suite.registry.Register("actor1", actor1)
	require.NoError(suite.T(), err)

	_, err = suite.registry.Register("actor2", actor2)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.StartAll(suite.ctx)

	// Assert
	require.NoError(suite.T(), err)
	assert.True(suite.T(), actor1.IsStarted())
	assert.True(suite.T(), actor2.IsStarted())
}

func (suite *GoRegistryTestSuite) TestStartAll_WithSomeFailures_ShouldReturnCollectedErrors() {
	// Arrange
	actor1 := NewMockActor("success-actor", uint64(actor.Initialized))
	actor2 := NewMockActor("fail-actor", uint64(actor.Initialized))
	actor2.SetWaitError(errors.New("wait failed"))

	_, err := suite.registry.Register("success-actor", actor1)
	require.NoError(suite.T(), err)

	_, err = suite.registry.Register("fail-actor", actor2)
	require.NoError(suite.T(), err)

	// Don't register the stopped actor as it should fail
	// _, err = suite.registry.Register("stopped-actor", actor3)
	// require.NoError(suite.T(), err)
	// Act
	err = suite.registry.StartAll(suite.ctx)

	// Assert
	assert.Error(suite.T(), err)
	assert.True(suite.T(), actor1.IsStarted())
	assert.True(suite.T(), actor2.IsStarted()) // Should have started despite wait error
	// actor3 was not registered, so no assertion needed
}

// Test actor stopping
func (suite *GoRegistryTestSuite) TestStop_WithStartedActor_ShouldStopSuccessfully() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := suite.registry.Register("test-actor", mockActor)
	require.NoError(suite.T(), err)

	err = suite.registry.Start(suite.ctx, "test-actor")
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Stop("test-actor")

	// Assert
	require.NoError(suite.T(), err)
	assert.True(suite.T(), mockActor.IsStopped())
	assert.Equal(suite.T(), uint64(actor.Done), mockActor.State())
}

func (suite *GoRegistryTestSuite) TestStop_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := suite.registry.Stop("non-existent")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorNotFound)
}

func (suite *GoRegistryTestSuite) TestStop_WithNotStartedActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("not-started-actor", uint64(actor.Initialized))
	_, err := suite.registry.Register("not-started-actor", mockActor)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Stop("not-started-actor")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorNotStarted)
}

func (suite *GoRegistryTestSuite) TestStopAll_WithMultipleStartedActors_ShouldStopAll() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", uint64(actor.Initialized))

	_, err := suite.registry.Register("actor1", actor1)
	require.NoError(suite.T(), err)

	_, err = suite.registry.Register("actor2", actor2)
	require.NoError(suite.T(), err)

	err = suite.registry.StartAll(suite.ctx)
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.StopAll()

	// Assert
	require.NoError(suite.T(), err)
	assert.True(suite.T(), actor1.IsStopped())
	assert.True(suite.T(), actor2.IsStopped())
}

func (suite *GoRegistryTestSuite) TestStopAll_WithMixedStates_ShouldStopOnlyStarted() {
	// Arrange
	actor1 := NewMockActor("started-actor", uint64(actor.Initialized))
	actor2 := NewMockActor("not-started-actor", uint64(actor.Initialized))

	_, err := suite.registry.Register("started-actor", actor1)
	require.NoError(suite.T(), err)

	_, err = suite.registry.Register("not-started-actor", actor2)
	require.NoError(suite.T(), err)

	err = suite.registry.Start(suite.ctx, "started-actor")
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.StopAll()

	// Assert - Should have errors for the not-started actor
	assert.Error(suite.T(), err)
	assert.True(suite.T(), actor1.IsStopped())
	assert.False(suite.T(), actor2.IsStopped())
}

// Test actor unregistration
func (suite *GoRegistryTestSuite) TestUnregister_WithStoppedActor_ShouldRemoveActor() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := suite.registry.Register("test-actor", mockActor)
	require.NoError(suite.T(), err)

	err = suite.registry.Start(suite.ctx, "test-actor")
	require.NoError(suite.T(), err)

	err = suite.registry.Stop("test-actor")
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Unregister("test-actor")

	// Assert
	require.NoError(suite.T(), err)

	// Verify actor is removed
	_, err = suite.registry.Get("test-actor")
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorNotFound, err)
}

func (suite *GoRegistryTestSuite) TestUnregister_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := suite.registry.Unregister("non-existent")

	// Assert
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrActorNotFound, err)
}

func (suite *GoRegistryTestSuite) TestUnregister_WithRunningActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("running-actor", uint64(actor.Initialized))
	_, err := suite.registry.Register("running-actor", mockActor)
	require.NoError(suite.T(), err)

	err = suite.registry.Start(suite.ctx, "running-actor")
	require.NoError(suite.T(), err)

	// Act
	err = suite.registry.Unregister("running-actor")

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorIsNotStopped)
}

// Test command dispatching
func (suite *GoRegistryTestSuite) TestDispatch_WithValidActor_ShouldDispatchCommand() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	id, err := suite.registry.Register("test-actor", mockActor)
	require.NoError(suite.T(), err)

	command := NewMockCommand("test-command")

	// Act
	err = suite.registry.Dispatch(suite.ctx, id, command)

	// Assert
	require.NoError(suite.T(), err)
	commands := mockActor.GetCommands()
	assert.Len(suite.T(), commands, 1)
	assert.Equal(suite.T(), command, commands[0])
}

func (suite *GoRegistryTestSuite) TestDispatch_WithNonExistentActor_ShouldReturnError() {
	// Arrange
	nonExistentID := uuid.New()
	command := NewMockCommand("test-command")

	// Act
	err := suite.registry.Dispatch(suite.ctx, nonExistentID, command)

	// Assert
	assert.Error(suite.T(), err)
	assert.ErrorIs(suite.T(), err, ErrActorNotFound)
}

// Test concurrent access
func (suite *GoRegistryTestSuite) TestConcurrentOperations_ShouldBeSafe() {
	// Arrange
	var wg sync.WaitGroup
	numOperations := 50

	// Act - Perform concurrent registrations, starts, stops, and lookups
	for i := 0; i < numOperations; i++ {
		wg.Add(4)

		go func(index int) {
			defer wg.Done()
			mockActor := NewMockActor(fmt.Sprintf("actor-%d", index), uint64(actor.Initialized))
			suite.registry.Register(fmt.Sprintf("actor-%d", index), mockActor)
		}(i)

		go func(index int) {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond) // Give registration time
			suite.registry.Start(suite.ctx, fmt.Sprintf("actor-%d", index))
		}(i)

		go func(index int) {
			defer wg.Done()
			time.Sleep(20 * time.Millisecond) // Give start time
			suite.registry.Stop(fmt.Sprintf("actor-%d", index))
		}(i)

		go func(index int) {
			defer wg.Done()
			suite.registry.Get(fmt.Sprintf("actor-%d", index))
		}(i)
	}

	// Assert - No panics, operations complete
	assert.NotPanics(suite.T(), func() {
		wg.Wait()
	})
}

// Test error cases and edge conditions
func (suite *GoRegistryTestSuite) TestMustBeRunnable_WithNil_ShouldReturnError() {
	// Act
	runnable, err := mustBeRunnable(nil)

	// Assert
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), runnable)
	assert.Equal(suite.T(), ErrNilActor, err)
}

func (suite *GoRegistryTestSuite) TestMustBeActionable_WithNil_ShouldReturnError() {
	// Act
	actionable, err := mustBeActionable(nil)

	// Assert
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), actionable)
	assert.Equal(suite.T(), ErrNilActor, err)
}

func (suite *GoRegistryTestSuite) TestMustBeRunnable_WithUnknownState_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("unknown-state", 999) // Invalid state

	// Act
	runnable, err := mustBeRunnable(mockActor)

	// Assert
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), runnable)
	assert.Equal(suite.T(), ErrUnknownActorState, err)
}

// Benchmark tests
func BenchmarkGoRegistry_Register(b *testing.B) {
	registry := New("bench-registry")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		registry.Register(fmt.Sprintf("actor-%d", i), mockActor)
	}
}

func BenchmarkGoRegistry_Get(b *testing.B) {
	registry := New("bench-registry")

	// Setup actors
	for i := 0; i < 1000; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		registry.Register(fmt.Sprintf("actor-%d", i), mockActor)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Get(fmt.Sprintf("actor-%d", i%1000))
	}
}

func BenchmarkGoRegistry_ConcurrentAccess(b *testing.B) {
	registry := New("bench-registry")

	// Setup some actors
	for i := 0; i < 100; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		registry.Register(fmt.Sprintf("actor-%d", i), mockActor)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			registry.Get(fmt.Sprintf("actor-%d", b.N%100))
			registry.GetAll()
		}
	})
}
