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
	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v2/pkg/actor"
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
type MockEntity struct{}

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

func (s *GoRegistryTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.registry = New("test-registry")
}

func (s *GoRegistryTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestGoRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(GoRegistryTestSuite))
}

// Test registry creation and configuration
func (s *GoRegistryTestSuite) TestNew_WithDefaultOptions_ShouldCreateRegistry() {
	// Act
	registry := New("test-tag")

	// Assert
	s.NotNil(registry)
	s.Equal("test-tag", registry.Tag())
	s.Equal(5*time.Second, registry.startTimeout)
	s.Equal(5*time.Second, registry.stopTimeout)
	s.NotNil(registry.actors)
	s.NotNil(registry.lookupTable)
}

func (s *GoRegistryTestSuite) TestNew_WithCustomTimeouts_ShouldSetCorrectValues() {
	// Arrange
	startTimeout := 10 * time.Second
	stopTimeout := 15 * time.Second

	// Act
	registry := New("test-tag",
		WithStartTimeout(startTimeout),
		WithStopTimeout(stopTimeout),
	)

	// Assert
	s.Equal(startTimeout, registry.startTimeout)
	s.Equal(stopTimeout, registry.stopTimeout)
}

func (s *GoRegistryTestSuite) TestTag_ShouldReturnCorrectTag() {
	// Arrange
	expectedTag := "custom-registry-tag"
	registry := New(expectedTag)

	// Act
	tag := registry.Tag()

	// Assert
	s.Equal(expectedTag, tag)
}

// Test actor registration and lookup
func (s *GoRegistryTestSuite) TestRegister_WithValidActor_ShouldReturnUUID() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))

	// Act
	id, err := s.registry.Register("test-actor", mockActor)

	// Assert
	s.NoError(err)
	s.NotEqual(uuid.Nil, id)
}

func (s *GoRegistryTestSuite) TestRegister_WithNilActor_ShouldReturnError() {
	// Act
	id, err := s.registry.Register("nil-actor", nil)

	// Assert
	s.Error(err)
	s.Equal(uuid.Nil, id)
	s.ErrorIs(ErrNilActor, err)
}

func (s *GoRegistryTestSuite) TestRegister_WithNotRunnableActor_ShouldReturnError() {
	// Arrange
	notRunnable := &NotRunnableActor{name: "not-runnable"}

	// Act
	id, err := s.registry.Register("not-runnable", notRunnable)

	// Assert
	s.Error(err)
	s.Equal(uuid.Nil, id)
	s.ErrorIs(ErrActorIsNotRunnable, err)
}

func (s *GoRegistryTestSuite) TestRegister_WithNotActionableActor_ShouldReturnError() {
	// Arrange
	notActionable := &NotActionableActor{name: "not-actionable"}

	// Act
	id, err := s.registry.Register("not-actionable", notActionable)

	// Assert
	s.Error(err)
	s.Equal(uuid.Nil, id)
	s.ErrorIs(ErrActorIsNotActionable, err)
}

func (s *GoRegistryTestSuite) TestRegister_WithInvalidState_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("invalid-state", uint64(actor.Done))

	// Act
	id, err := s.registry.Register("invalid-state", mockActor)

	// Assert
	s.Error(err)
	s.Equal(uuid.Nil, id)
	s.ErrorIs(ErrActorIsNotRunnable, err)
}

func (s *GoRegistryTestSuite) TestGet_WithExistingActor_ShouldReturnModel() {
	// Arrange
	mockActor := NewMockActor("existing-actor", uint64(actor.Initialized))
	id, err := s.registry.Register("existing-actor", mockActor)
	s.NoError(err)

	// Act
	model, err := s.registry.Get("existing-actor")

	// Assert
	s.NoError(err)
	s.NotNil(model)
	s.Equal(id, model.ID)
	s.Equal("existing-actor", model.Name)
	s.Equal(uint64(actor.Initialized), model.State)
}

func (s *GoRegistryTestSuite) TestGet_WithNonExistentActor_ShouldReturnError() {
	// Act
	model, err := s.registry.Get("non-existent")

	// Assert
	s.Error(err)
	s.Nil(model)
	s.Equal(ErrActorNotFound, err)
}

func (s *GoRegistryTestSuite) TestGetAll_WithMultipleActors_ShouldReturnAllModels() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", actor.Started)

	id1, err := s.registry.Register("actor1", actor1)
	s.NoError(err)

	id2, err := s.registry.Register("actor2", actor2)
	s.NoError(err)

	// Act
	models := s.registry.GetAll()

	// Assert
	s.Len(models, 2)

	// Find models by ID
	var model1, model2 *Model
	for i := range models {
		switch models[i].ID {
		case id1:
			model1 = &models[i]
		case id2:
			model2 = &models[i]
		}
	}

	s.NotNil(model1)
	s.NotNil(model2)

	s.Equal("actor1", model1.Name)
	s.Equal(uint64(actor.Initialized), model1.State)

	s.Equal("actor2", model2.Name)
	s.Equal(uint64(actor.Started), model2.State)
}

func (s *GoRegistryTestSuite) TestGetAll_WithNoActors_ShouldReturnEmptySlice() {
	// Act
	models := s.registry.GetAll()

	// Assert
	s.Empty(models)
	s.NotNil(models)
}

// Test actor lifecycle management
func (s *GoRegistryTestSuite) TestStart_WithValidActor_ShouldStartSuccessfully() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := s.registry.Register("test-actor", mockActor)
	s.NoError(err)

	// Act
	err = s.registry.Start(s.ctx, "test-actor")

	// Assert
	s.NoError(err)
	s.True(mockActor.IsStarted())
	s.Equal(uint64(actor.Started), mockActor.State())
}

func (s *GoRegistryTestSuite) TestStart_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := s.registry.Start(s.ctx, "non-existent")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorNotFound)
}

func (s *GoRegistryTestSuite) TestStart_WithAlreadyStartedActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("started-actor", uint64(actor.Started))
	_, err := s.registry.Register("started-actor", mockActor)
	s.NoError(err)

	// Act
	err = s.registry.Start(s.ctx, "started-actor")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorAlreadyStarted)
}

func (s *GoRegistryTestSuite) TestStart_WithWaitReadyFailure_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("wait-fail-actor", uint64(actor.Initialized))
	mockActor.SetWaitError(errors.New("wait failed"))
	_, err := s.registry.Register("wait-fail-actor", mockActor)
	s.NoError(err)

	// Act
	err = s.registry.Start(s.ctx, "wait-fail-actor")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorIsNotRunnable)
}

func (s *GoRegistryTestSuite) TestStartAll_WithMultipleActors_ShouldStartAll() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", uint64(actor.Initialized))

	_, err := s.registry.Register("actor1", actor1)
	s.NoError(err)

	_, err = s.registry.Register("actor2", actor2)
	s.NoError(err)

	// Act
	err = s.registry.StartAll(s.ctx)

	// Assert
	s.NoError(err)
	s.True(actor1.IsStarted())
	s.True(actor2.IsStarted())
}

func (s *GoRegistryTestSuite) TestStartAll_WithSomeFailures_ShouldReturnCollectedErrors() {
	// Arrange
	actor1 := NewMockActor("success-actor", uint64(actor.Initialized))
	actor2 := NewMockActor("fail-actor", uint64(actor.Initialized))
	actor2.SetWaitError(errors.New("wait failed"))

	_, err := s.registry.Register("success-actor", actor1)
	s.NoError(err)

	_, err = s.registry.Register("fail-actor", actor2)
	s.NoError(err)

	// Don't register the stopped actor as it should fail
	// _, err = s.registry.Register("stopped-actor", actor3)
	// s.NoError(err)
	// Act
	err = s.registry.StartAll(s.ctx)

	// Assert
	s.Error(err)
	s.True(actor1.IsStarted())
	s.True(actor2.IsStarted()) // Should have started despite wait error
	// actor3 was not registered, so no assertion needed
}

// Test actor stopping
func (s *GoRegistryTestSuite) TestStop_WithStartedActor_ShouldStopSuccessfully() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := s.registry.Register("test-actor", mockActor)
	s.NoError(err)

	err = s.registry.Start(s.ctx, "test-actor")
	s.NoError(err)

	// Act
	err = s.registry.Stop("test-actor")

	// Assert
	s.NoError(err)
	s.True(mockActor.IsStopped())
	s.Equal(uint64(actor.Done), mockActor.State())
}

func (s *GoRegistryTestSuite) TestStop_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := s.registry.Stop("non-existent")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorNotFound)
}

func (s *GoRegistryTestSuite) TestStop_WithNotStartedActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("not-started-actor", uint64(actor.Initialized))
	_, err := s.registry.Register("not-started-actor", mockActor)
	s.NoError(err)

	// Act
	err = s.registry.Stop("not-started-actor")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorNotStarted)
}

func (s *GoRegistryTestSuite) TestStopAll_WithMultipleStartedActors_ShouldStopAll() {
	// Arrange
	actor1 := NewMockActor("actor1", uint64(actor.Initialized))
	actor2 := NewMockActor("actor2", uint64(actor.Initialized))

	_, err := s.registry.Register("actor1", actor1)
	s.NoError(err)

	_, err = s.registry.Register("actor2", actor2)
	s.NoError(err)

	err = s.registry.StartAll(s.ctx)
	s.NoError(err)

	// Act
	err = s.registry.StopAll()

	// Assert
	s.NoError(err)
	s.True(actor1.IsStopped())
	s.True(actor2.IsStopped())
}

func (s *GoRegistryTestSuite) TestStopAll_WithMixedStates_ShouldStopOnlyStarted() {
	// Arrange
	actor1 := NewMockActor("started-actor", uint64(actor.Initialized))
	actor2 := NewMockActor("not-started-actor", uint64(actor.Initialized))

	_, err := s.registry.Register("started-actor", actor1)
	s.NoError(err)

	_, err = s.registry.Register("not-started-actor", actor2)
	s.NoError(err)

	err = s.registry.Start(s.ctx, "started-actor")
	s.NoError(err)

	// Act
	err = s.registry.StopAll()

	// Assert - Should have errors for the not-started actor
	s.Error(err)
	s.True(actor1.IsStopped())
	s.False(actor2.IsStopped())
}

// Test actor unregistration
func (s *GoRegistryTestSuite) TestUnregister_WithStoppedActor_ShouldRemoveActor() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	_, err := s.registry.Register("test-actor", mockActor)
	s.NoError(err)

	err = s.registry.Start(s.ctx, "test-actor")
	s.NoError(err)

	err = s.registry.Stop("test-actor")
	s.NoError(err)

	// Act
	err = s.registry.Unregister("test-actor")

	// Assert
	s.NoError(err)

	// Verify actor is removed
	_, err = s.registry.Get("test-actor")
	s.Error(err)
	s.Equal(ErrActorNotFound, err)
}

func (s *GoRegistryTestSuite) TestUnregister_WithNonExistentActor_ShouldReturnError() {
	// Act
	err := s.registry.Unregister("non-existent")

	// Assert
	s.Error(err)
	s.Equal(ErrActorNotFound, err)
}

func (s *GoRegistryTestSuite) TestUnregister_WithRunningActor_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("running-actor", uint64(actor.Initialized))
	_, err := s.registry.Register("running-actor", mockActor)
	s.NoError(err)

	err = s.registry.Start(s.ctx, "running-actor")
	s.NoError(err)

	// Act
	err = s.registry.Unregister("running-actor")

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorIsNotStopped)
}

// Test command dispatching
func (s *GoRegistryTestSuite) TestDispatch_WithValidActor_ShouldDispatchCommand() {
	// Arrange
	mockActor := NewMockActor("test-actor", uint64(actor.Initialized))
	id, err := s.registry.Register("test-actor", mockActor)
	s.NoError(err)

	command := NewMockCommand("test-command")

	// Act
	err = s.registry.Dispatch(s.ctx, id, command)

	// Assert
	s.NoError(err)
	commands := mockActor.GetCommands()
	s.Len(commands, 1)
	s.Equal(command, commands[0])
}

func (s *GoRegistryTestSuite) TestDispatch_WithNonExistentActor_ShouldReturnError() {
	// Arrange
	nonExistentID := uuid.New()
	command := NewMockCommand("test-command")

	// Act
	err := s.registry.Dispatch(s.ctx, nonExistentID, command)

	// Assert
	s.Error(err)
	s.ErrorIs(err, ErrActorNotFound)
}

// Test concurrent access
func (s *GoRegistryTestSuite) TestConcurrentOperations_ShouldBeSafe() {
	// Arrange
	var wg sync.WaitGroup
	numOperations := 50

	// Act - Perform concurrent registrations, starts, stops, and lookups
	for i := 0; i < numOperations; i++ {
		wg.Go(func(index int) func() {
			return func() {
				mockActor := NewMockActor(fmt.Sprintf("actor-%d", index), uint64(actor.Initialized))
				_, err := s.registry.Register(fmt.Sprintf("actor-%d", index), mockActor)
				if err != nil {
					s.T().Logf("Register actor-%d: %v", index, err)
				}
			}
		}(i))

		wg.Go(func(index int) func() {
			return func() {
				time.Sleep(10 * time.Millisecond)
				err := s.registry.Start(s.ctx, fmt.Sprintf("actor-%d", index))
				if err != nil {
					s.T().Logf("Start actor-%d: %v", index, err)
				}
			}
		}(i))

		wg.Go(func(index int) func() {
			return func() {
				time.Sleep(20 * time.Millisecond)
				err := s.registry.Stop(fmt.Sprintf("actor-%d", index))
				if err != nil {
					s.T().Logf("Stop actor-%d: %v", index, err)
				}
			}
		}(i))

		wg.Go(func(index int) func() {
			return func() {
				_, err := s.registry.Get(fmt.Sprintf("actor-%d", index))
				if err != nil {
					s.T().Logf("Get actor-%d: %v", index, err)
				}
			}
		}(i))
	}

	// Assert - No panics, operations complete
	s.NotPanics(func() {
		wg.Wait()
	})
}

// Test error cases and edge conditions
func (s *GoRegistryTestSuite) TestMustBeRunnable_WithNil_ShouldReturnError() {
	// Act
	runnable, err := mustBeRunnable(nil)

	// Assert
	s.Error(err)
	s.Nil(runnable)
	s.Equal(ErrNilActor, err)
}

func (s *GoRegistryTestSuite) TestMustBeActionable_WithNil_ShouldReturnError() {
	// Act
	actionable, err := mustBeActionable(nil)

	// Assert
	s.Error(err)
	s.Nil(actionable)
	s.Equal(ErrNilActor, err)
}

func (s *GoRegistryTestSuite) TestMustBeRunnable_WithUnknownState_ShouldReturnError() {
	// Arrange
	mockActor := NewMockActor("unknown-state", 999) // Invalid state

	// Act
	runnable, err := mustBeRunnable(mockActor)

	// Assert
	s.Error(err)
	s.Nil(runnable)
	s.Equal(ErrUnknownActorState, err)
}

// Benchmark tests
func BenchmarkGoRegistry_Register(b *testing.B) {
	registry := New("bench-registry")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		if _, err := registry.Register(fmt.Sprintf("actor-%d", i), mockActor); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoRegistry_Get(b *testing.B) {
	registry := New("bench-registry")

	// Setup actors
	for i := 0; i < 1000; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		if _, err := registry.Register(fmt.Sprintf("actor-%d", i), mockActor); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := registry.Get(fmt.Sprintf("actor-%d", i%1000)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoRegistry_ConcurrentAccess(b *testing.B) {
	registry := New("bench-registry")

	// Setup some actors
	for i := 0; i < 100; i++ {
		mockActor := NewMockActor(fmt.Sprintf("actor-%d", i), uint64(actor.Initialized))
		if _, err := registry.Register(fmt.Sprintf("actor-%d", i), mockActor); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := registry.Get(fmt.Sprintf("actor-%d", b.N%100)); err != nil {
				b.Fatal(err)
			}
			registry.GetAll()
		}
	})
}
