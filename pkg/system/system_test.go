package system

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v2/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v2/pkg/actorref"
	"github.com/barnowlsnest/go-actorlib/v2/pkg/command"
)

// --- Test helpers ---

type testEntity struct {
	value   string
	isReady bool
	mu      sync.Mutex
}

func newTestEntity(value string) *testEntity {
	return &testEntity{value: value, isReady: true}
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

func (te *testEntity) SetValue(v string) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.value = v
}

type testEntityProvider struct {
	entity *testEntity
}

func (p *testEntityProvider) Provide() *testEntity {
	return p.entity
}

// mockManagedActor records Stop calls for order verification.
type mockManagedActor struct {
	mu      sync.Mutex
	state   uint64
	stopLog *[]string // shared slice pointer to record stop order
	name    string
	stopErr error
}

func newMockActor(name string, stopLog *[]string) *mockManagedActor {
	return &mockManagedActor{
		state:   actor.Started,
		stopLog: stopLog,
		name:    name,
	}
}

func (m *mockManagedActor) Stop(_ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	*m.stopLog = append(*m.stopLog, m.name)
	m.state = actor.Done

	return m.stopErr
}

func (m *mockManagedActor) State() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// differentEntity is a second entity type for type-mismatch tests.
type differentEntity struct{}

func (d *differentEntity) IsProvidable() bool { return true }

// --- Test suite ---

type ActorSystemTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *ActorSystemTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *ActorSystemTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestActorSystemTestSuite(t *testing.T) {
	suite.Run(t, new(ActorSystemTestSuite))
}

func (s *ActorSystemTestSuite) newStartedRef() (*actorref.Ref[*testEntity], *testEntity) {
	entity := newTestEntity("initial")
	provider := &testEntityProvider{entity: entity}

	a, err := actor.New(
		actor.WithProvider(provider),
		actor.WithInputBufferSize[*testEntity](10),
	)
	s.Require().NoError(err)

	err = a.Start(s.ctx)
	s.Require().NoError(err)

	err = a.WaitReady(s.ctx, 100*time.Millisecond)
	s.Require().NoError(err)

	ref, err := actorref.New(a)
	s.Require().NoError(err)

	return ref, entity
}

// --- Constructor tests ---

func (s *ActorSystemTestSuite) TestNew_Default_ShouldCreateSystem() {
	// Act
	sys := New()

	// Assert
	s.NotNil(sys)
	s.Equal(0, sys.Count())
}

// --- Register tests ---

func (s *ActorSystemTestSuite) TestRegister_ValidNameAndRef_ShouldSucceed() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	// Act
	err := Register(sys, "actor-1", ref)

	// Assert
	s.NoError(err)
	s.Equal(1, sys.Count())
}

func (s *ActorSystemTestSuite) TestRegister_EmptyName_ShouldReturnError() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	// Act
	err := Register(sys, "", ref)

	// Assert
	s.ErrorIs(err, ErrActorNameEmpty)
	s.Equal(0, sys.Count())
}

func (s *ActorSystemTestSuite) TestRegister_NilRef_ShouldReturnError() {
	// Arrange
	sys := New()

	// Act
	err := Register[*testEntity](sys, "actor-1", nil)

	// Assert
	s.ErrorIs(err, ErrActorNilRef)
	s.Equal(0, sys.Count())
}

func (s *ActorSystemTestSuite) TestRegister_DuplicateName_ShouldReturnError() {
	// Arrange
	sys := New()
	ref1, _ := s.newStartedRef()
	ref2, _ := s.newStartedRef()

	err := Register(sys, "actor-1", ref1)
	s.Require().NoError(err)

	// Act
	err = Register(sys, "actor-1", ref2)

	// Assert
	s.ErrorIs(err, ErrActorNameDuplicate)
	s.Equal(1, sys.Count())
}

func (s *ActorSystemTestSuite) TestRegister_AfterStopAll_ShouldReturnError() {
	// Arrange
	sys := New()

	err := sys.StopAll(100 * time.Millisecond)
	s.Require().NoError(err)

	ref, _ := s.newStartedRef()

	// Act
	err = Register(sys, "actor-1", ref)

	// Assert
	s.ErrorIs(err, ErrSystemStopped)
}

func (s *ActorSystemTestSuite) TestRegister_MultipleActors_ShouldTrackAll() {
	// Arrange
	sys := New()
	ref1, _ := s.newStartedRef()
	ref2, _ := s.newStartedRef()
	ref3, _ := s.newStartedRef()

	// Act
	s.Require().NoError(Register(sys, "a", ref1))
	s.Require().NoError(Register(sys, "b", ref2))
	s.Require().NoError(Register(sys, "c", ref3))

	// Assert
	s.Equal(3, sys.Count())
}

// --- Get tests ---

func (s *ActorSystemTestSuite) TestGet_ExistingActor_ShouldReturnRef() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	err := Register(sys, "actor-1", ref)
	s.Require().NoError(err)

	// Act
	managed, err := sys.Get("actor-1")

	// Assert
	s.NoError(err)
	s.NotNil(managed)
	s.Equal(uint64(actor.Started), managed.State())
}

func (s *ActorSystemTestSuite) TestGet_NonExistentName_ShouldReturnError() {
	// Arrange
	sys := New()

	// Act
	managed, err := sys.Get("unknown")

	// Assert
	s.ErrorIs(err, ErrActorNotFound)
	s.Nil(managed)
}

func (s *ActorSystemTestSuite) TestGet_AfterStopAll_ShouldReturnError() {
	// Arrange
	sys := New()

	err := sys.StopAll(100 * time.Millisecond)
	s.Require().NoError(err)

	// Act
	managed, err := sys.Get("actor-1")

	// Assert
	s.ErrorIs(err, ErrSystemStopped)
	s.Nil(managed)
}

// --- Unregister tests ---

func (s *ActorSystemTestSuite) TestUnregister_ExistingActor_ShouldRemove() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	err := Register(sys, "actor-1", ref)
	s.Require().NoError(err)

	// Act
	err = sys.Unregister("actor-1")

	// Assert
	s.NoError(err)
	s.Equal(0, sys.Count())

	_, err = sys.Get("actor-1")
	s.ErrorIs(err, ErrActorNotFound)
}

func (s *ActorSystemTestSuite) TestUnregister_NonExistentName_ShouldReturnError() {
	// Arrange
	sys := New()

	// Act
	err := sys.Unregister("unknown")

	// Assert
	s.ErrorIs(err, ErrActorNotFound)
}

func (s *ActorSystemTestSuite) TestUnregister_AfterStopAll_ShouldReturnError() {
	// Arrange
	sys := New()

	err := sys.StopAll(100 * time.Millisecond)
	s.Require().NoError(err)

	// Act
	err = sys.Unregister("actor-1")

	// Assert
	s.ErrorIs(err, ErrSystemStopped)
}

// --- Send (dispatch) tests ---

func (s *ActorSystemTestSuite) TestSend_ValidCommand_ShouldDispatchToActor() {
	// Arrange
	sys := New()
	ref, entity := s.newStartedRef()

	err := Register(sys, "worker", ref)
	s.Require().NoError(err)

	executed := make(chan struct{})
	cmd := command.New(func(e *testEntity) (string, error) {
		e.SetValue("dispatched")
		close(executed)
		return "", nil
	})

	// Act
	err = Send(sys, s.ctx, "worker", cmd)

	// Assert
	s.NoError(err)

	select {
	case <-executed:
		s.Equal("dispatched", entity.GetValue())
	case <-time.After(time.Second):
		s.Fail("command was not executed within timeout")
	}
}

func (s *ActorSystemTestSuite) TestSend_NonExistentActor_ShouldReturnError() {
	// Arrange
	sys := New()

	cmd := command.New(func(e *testEntity) (string, error) { return "", nil })

	// Act
	err := Send(sys, s.ctx, "unknown", cmd)

	// Assert
	s.ErrorIs(err, ErrActorNotFound)
}

func (s *ActorSystemTestSuite) TestSend_AfterStopAll_ShouldReturnError() {
	// Arrange
	sys := New()

	err := sys.StopAll(100 * time.Millisecond)
	s.Require().NoError(err)

	cmd := command.New(func(e *testEntity) (string, error) { return "", nil })

	// Act
	err = Send(sys, s.ctx, "worker", cmd)

	// Assert
	s.ErrorIs(err, ErrSystemStopped)
}

func (s *ActorSystemTestSuite) TestSend_TypeMismatch_ShouldReturnError() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	err := Register(sys, "worker", ref)
	s.Require().NoError(err)

	// Create a command for a different entity type and dispatch via the type-erased path.
	mismatchCmd := command.New(func(e *differentEntity) (string, error) { return "", nil })

	// Act — use the unexported send to bypass compile-time type checking
	err = sys.send(s.ctx, "worker", mismatchCmd)

	// Assert
	s.ErrorIs(err, ErrCommandTypeMismatch)
}

// --- Ask tests ---

func (s *ActorSystemTestSuite) TestAsk_ValidRequest_ShouldReturnResult() {
	// Arrange
	sys := New()
	ref, entity := s.newStartedRef()
	entity.SetValue("hello")

	err := Register(sys, "worker", ref)
	s.Require().NoError(err)

	// Act
	result, err := Ask(sys, s.ctx, "worker", func(e *testEntity) (string, error) {
		return e.GetValue(), nil
	}, time.Second)

	// Assert
	s.NoError(err)
	s.Equal("hello", result)
}

func (s *ActorSystemTestSuite) TestAsk_Timeout_ShouldReturnError() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	err := Register(sys, "worker", ref)
	s.Require().NoError(err)

	// Block the actor so the ask command can't execute in time.
	blockCmd := command.New(func(e *testEntity) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return "", nil
	})
	err = Send(sys, s.ctx, "worker", blockCmd)
	s.Require().NoError(err)

	// Act
	_, err = Ask(sys, s.ctx, "worker", func(e *testEntity) (string, error) {
		return "result", nil
	}, 10*time.Millisecond)

	// Assert
	s.ErrorIs(err, ErrAskTimeout)
}

func (s *ActorSystemTestSuite) TestAsk_NonExistentActor_ShouldReturnError() {
	// Arrange
	sys := New()

	// Act
	_, err := Ask(sys, s.ctx, "unknown", func(e *testEntity) (string, error) {
		return "", nil
	}, time.Second)

	// Assert
	s.ErrorIs(err, ErrActorNotFound)
}

// --- StopAll tests ---

func (s *ActorSystemTestSuite) TestStopAll_MultipleActors_ShouldStopInReverseOrder() {
	// Arrange
	sys := New()
	var stopLog []string

	mockA := newMockActor("A", &stopLog)
	mockB := newMockActor("B", &stopLog)
	mockC := newMockActor("C", &stopLog)

	// Register directly via unexported method to use mocks.
	s.Require().NoError(sys.register("A", mockA, nil))
	s.Require().NoError(sys.register("B", mockB, nil))
	s.Require().NoError(sys.register("C", mockC, nil))

	// Act
	err := sys.StopAll(100 * time.Millisecond)

	// Assert
	s.NoError(err)
	s.Equal([]string{"C", "B", "A"}, stopLog)
	s.Equal(0, sys.Count())
}

func (s *ActorSystemTestSuite) TestStopAll_EmptySystem_ShouldSucceed() {
	// Arrange
	sys := New()

	// Act
	err := sys.StopAll(100 * time.Millisecond)

	// Assert
	s.NoError(err)
}

func (s *ActorSystemTestSuite) TestStopAll_WithStopError_ShouldCollectErrors() {
	// Arrange
	sys := New()
	var stopLog []string

	mockA := newMockActor("A", &stopLog)
	mockB := newMockActor("B", &stopLog)
	mockB.stopErr = fmt.Errorf("stop B failed")

	s.Require().NoError(sys.register("A", mockA, nil))
	s.Require().NoError(sys.register("B", mockB, nil))

	// Act
	err := sys.StopAll(100 * time.Millisecond)

	// Assert
	s.Error(err)
	s.Contains(err.Error(), "stop B failed")
	s.Equal([]string{"B", "A"}, stopLog) // reverse order: B then A
}

func (s *ActorSystemTestSuite) TestStopAll_Idempotent_ShouldReturnErrorOnSecondCall() {
	// Arrange
	sys := New()

	err := sys.StopAll(100 * time.Millisecond)
	s.Require().NoError(err)

	// Act
	err = sys.StopAll(100 * time.Millisecond)

	// Assert
	s.ErrorIs(err, ErrSystemStopped)
}

func (s *ActorSystemTestSuite) TestStopAll_AfterUnregister_ShouldSkipUnregistered() {
	// Arrange
	sys := New()
	var stopLog []string

	mockA := newMockActor("A", &stopLog)
	mockB := newMockActor("B", &stopLog)
	mockC := newMockActor("C", &stopLog)

	s.Require().NoError(sys.register("A", mockA, nil))
	s.Require().NoError(sys.register("B", mockB, nil))
	s.Require().NoError(sys.register("C", mockC, nil))

	err := sys.Unregister("B")
	s.Require().NoError(err)

	// Act
	err = sys.StopAll(100 * time.Millisecond)

	// Assert
	s.NoError(err)
	s.Equal([]string{"C", "A"}, stopLog) // B was unregistered, skipped
}

// --- Concurrency tests ---

func (s *ActorSystemTestSuite) TestConcurrentRegister_ShouldBeSafe() {
	// Arrange
	sys := New()
	const goroutines = 50
	var wg sync.WaitGroup

	// Act
	for i := range goroutines {
		wg.Go(func() {
			ref, _ := s.newStartedRef()
			_ = Register(sys, fmt.Sprintf("actor-%d", i), ref)
		})
	}
	wg.Wait()

	// Assert
	s.Equal(goroutines, sys.Count())
}

func (s *ActorSystemTestSuite) TestConcurrentSend_ShouldBeSafe() {
	// Arrange
	sys := New()
	ref, _ := s.newStartedRef()

	err := Register(sys, "worker", ref)
	s.Require().NoError(err)

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	// Act
	for i := range goroutines {
		wg.Go(func() {
			cmd := command.New(func(e *testEntity) (string, error) {
				return "", nil
			})
			errs[i] = Send(sys, s.ctx, "worker", cmd)
		})
	}
	wg.Wait()

	// Assert
	for i := range goroutines {
		s.NoError(errs[i])
	}
}

func (s *ActorSystemTestSuite) TestConcurrentGetAndRegister_ShouldBeSafe() {
	// Arrange
	sys := New()
	const goroutines = 50
	var wg sync.WaitGroup

	// Act — interleave Gets and Registers
	for i := range goroutines {
		wg.Go(func() {
			if i%2 == 0 {
				ref, _ := s.newStartedRef()
				_ = Register(sys, fmt.Sprintf("actor-%d", i), ref)
			} else {
				_, _ = sys.Get(fmt.Sprintf("actor-%d", i-1))
			}
		})
	}
	wg.Wait()

	// Assert — no panic means success
	s.True(sys.Count() > 0)
}

// --- Integration test ---

func (s *ActorSystemTestSuite) TestIntegration_FullFlow_ShouldWork() {
	// Arrange
	sys := New()
	ref1, entity1 := s.newStartedRef()
	ref2, entity2 := s.newStartedRef()

	entity1.SetValue("entity-1")
	entity2.SetValue("entity-2")

	s.Require().NoError(Register(sys, "worker-1", ref1))
	s.Require().NoError(Register(sys, "worker-2", ref2))
	s.Equal(2, sys.Count())

	// Act — Send command to worker-1
	executed := make(chan struct{})
	cmd := command.New(func(e *testEntity) (string, error) {
		e.SetValue("updated-1")
		close(executed)
		return "", nil
	})
	err := Send(sys, s.ctx, "worker-1", cmd)
	s.Require().NoError(err)

	select {
	case <-executed:
		s.Equal("updated-1", entity1.GetValue())
	case <-time.After(time.Second):
		s.Fail("command was not executed within timeout")
	}

	// Act — Ask worker-2
	result, err := Ask(sys, s.ctx, "worker-2", func(e *testEntity) (string, error) {
		return e.GetValue(), nil
	}, time.Second)
	s.NoError(err)
	s.Equal("entity-2", result)

	// Act — Get
	managed, err := sys.Get("worker-1")
	s.NoError(err)
	s.Equal(uint64(actor.Started), managed.State())

	// Act — StopAll
	err = sys.StopAll(time.Second)
	s.NoError(err)
	s.Equal(0, sys.Count())
}

// --- Benchmarks ---

func BenchmarkRegister(b *testing.B) {
	var stopLog []string
	dispatch := func(_ context.Context, _ any) error { return nil }

	// Pre-generate names and mocks.
	names := make([]string, b.N)
	mocks := make([]*mockManagedActor, b.N)

	for i := range b.N {
		names[i] = fmt.Sprintf("actor-%d", i)
		mocks[i] = newMockActor(names[i], &stopLog)
	}

	sys := New()
	b.ResetTimer()

	for i := range b.N {
		_ = sys.register(names[i], mocks[i], dispatch)
	}
}

func BenchmarkGet(b *testing.B) {
	sys := New()
	var stopLog []string
	mock := newMockActor("bench", &stopLog)

	_ = sys.register("target", mock, func(_ context.Context, _ any) error { return nil })

	b.ResetTimer()

	for range b.N {
		_, _ = sys.Get("target")
	}
}

func BenchmarkSend(b *testing.B) {
	ctx := context.Background()
	sys := New()

	entity := newTestEntity("bench")
	provider := &testEntityProvider{entity: entity}

	a, err := actor.New(
		actor.WithProvider(provider),
		actor.WithInputBufferSize[*testEntity](1000),
	)
	if err != nil {
		b.Fatal(err)
	}

	if err = a.Start(ctx); err != nil {
		b.Fatal(err)
	}

	if err = a.WaitReady(ctx, 100*time.Millisecond); err != nil {
		b.Fatal(err)
	}

	ref, err := actorref.New(a)
	if err != nil {
		b.Fatal(err)
	}

	_ = Register(sys, "target", ref)

	b.ResetTimer()

	for range b.N {
		cmd := command.New(func(e *testEntity) (string, error) { return "", nil })
		_ = Send(sys, ctx, "target", cmd)
	}
}

func BenchmarkStopAll(b *testing.B) {
	const actorCount = 100

	names := make([]string, actorCount)
	for i := range actorCount {
		names[i] = fmt.Sprintf("actor-%d", i)
	}

	// Pre-build all systems before the timed loop.
	systems := make([]*ActorSystem, b.N)
	for n := range b.N {
		sys := New()
		var stopLog []string

		for i := range actorCount {
			mock := newMockActor(names[i], &stopLog)
			_ = sys.register(names[i], mock, nil)
		}

		systems[n] = sys
	}

	b.ResetTimer()

	for n := range b.N {
		_ = systems[n].StopAll(100 * time.Millisecond)
	}
}
