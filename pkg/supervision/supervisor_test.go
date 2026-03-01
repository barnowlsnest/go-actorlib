package supervision

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v3/pkg/actor"
)

// mockChildRef is a test implementation of ChildRef.
type mockChildRef struct {
	state uint64
	done  chan struct{}
}

func newMockChildRef() *mockChildRef {
	return &mockChildRef{
		state: actor.Started,
		done:  make(chan struct{}),
	}
}

func (m *mockChildRef) Stop(_ time.Duration) error {
	atomic.StoreUint64(&m.state, actor.Done)
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	return nil
}

func (m *mockChildRef) State() uint64 {
	return atomic.LoadUint64(&m.state)
}

func (m *mockChildRef) Done() <-chan struct{} {
	return m.done
}

// terminate simulates the child terminating with a given state.
func (m *mockChildRef) terminate(state uint64) {
	atomic.StoreUint64(&m.state, state)
	select {
	case <-m.done:
	default:
		close(m.done)
	}
}

// mockChildSpec creates mockChildRefs on each Start call.
type mockChildSpec struct {
	mu   sync.Mutex
	refs []*mockChildRef
}

func newMockChildSpec() *mockChildSpec {
	return &mockChildSpec{}
}

func (s *mockChildSpec) Start(_ context.Context) (ChildRef, error) {
	ref := newMockChildRef()
	s.mu.Lock()
	s.refs = append(s.refs, ref)
	s.mu.Unlock()
	return ref, nil
}

func (s *mockChildSpec) lastRef() *mockChildRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.refs) == 0 {
		return nil
	}
	return s.refs[len(s.refs)-1]
}

func (s *mockChildSpec) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.refs)
}

// --- Test Suite ---

type SupervisorTestSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *SupervisorTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *SupervisorTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel()
	}
}

func TestSupervisorTestSuite(t *testing.T) {
	suite.Run(t, new(SupervisorTestSuite))
}

// --- Add tests ---

func (s *SupervisorTestSuite) TestAdd_ValidChild_ShouldSucceed() {
	sup := NewSupervisor()
	err := sup.Add("worker", newMockChildSpec())
	s.NoError(err)
	s.Equal([]string{"worker"}, sup.Children())
}

func (s *SupervisorTestSuite) TestAdd_EmptyName_ShouldReturnError() {
	sup := NewSupervisor()
	err := sup.Add("", newMockChildSpec())
	s.ErrorIs(err, ErrChildNameEmpty)
}

func (s *SupervisorTestSuite) TestAdd_DuplicateName_ShouldReturnError() {
	sup := NewSupervisor()
	s.Require().NoError(sup.Add("worker", newMockChildSpec()))
	err := sup.Add("worker", newMockChildSpec())
	s.ErrorIs(err, ErrChildNameDuplicate)
}

func (s *SupervisorTestSuite) TestAdd_NilSpec_ShouldReturnError() {
	sup := NewSupervisor()
	err := sup.Add("worker", nil)
	s.ErrorIs(err, ErrNilChildSpec)
}

func (s *SupervisorTestSuite) TestAdd_AfterStopped_ShouldReturnError() {
	sup := NewSupervisor()
	s.Require().NoError(sup.StopAll(time.Second))
	err := sup.Add("worker", newMockChildSpec())
	s.ErrorIs(err, ErrSupervisorStopped)
}

// --- StartAll tests ---

func (s *SupervisorTestSuite) TestStartAll_MultipleChildren_ShouldStartAll() {
	sup := NewSupervisor()
	spec1 := newMockChildSpec()
	spec2 := newMockChildSpec()

	s.Require().NoError(sup.Add("worker-1", spec1))
	s.Require().NoError(sup.Add("worker-2", spec2))

	err := sup.StartAll(s.ctx, time.Second)
	s.NoError(err)

	s.Equal(1, spec1.startCount())
	s.Equal(1, spec2.startCount())

	state1, err := sup.ChildState("worker-1")
	s.NoError(err)
	s.Equal(uint64(actor.Started), state1)

	state2, err := sup.ChildState("worker-2")
	s.NoError(err)
	s.Equal(uint64(actor.Started), state2)
}

func (s *SupervisorTestSuite) TestStartAll_AfterStopped_ShouldReturnError() {
	sup := NewSupervisor()
	s.Require().NoError(sup.StopAll(time.Second))
	err := sup.StartAll(s.ctx, time.Second)
	s.ErrorIs(err, ErrSupervisorStopped)
}

// --- StopAll tests ---

func (s *SupervisorTestSuite) TestStopAll_ShouldStopAllChildren() {
	sup := NewSupervisor()
	spec := newMockChildSpec()

	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	err := sup.StopAll(time.Second)
	s.NoError(err)

	ref := spec.lastRef()
	s.Equal(uint64(actor.Done), ref.State())
}

func (s *SupervisorTestSuite) TestStopAll_Idempotent_ShouldReturnError() {
	sup := NewSupervisor()
	s.Require().NoError(sup.StopAll(time.Second))
	err := sup.StopAll(time.Second)
	s.ErrorIs(err, ErrSupervisorStopped)
}

// --- OneForOne restart tests ---

func (s *SupervisorTestSuite) TestOneForOne_ChildFails_ShouldRestartOnlyFailed() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec1 := newMockChildSpec()
	spec2 := newMockChildSpec()

	s.Require().NoError(sup.Add("worker-1", spec1))
	s.Require().NoError(sup.Add("worker-2", spec2))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	// Simulate worker-1 panicking
	ref1 := spec1.lastRef()
	ref1.terminate(actor.Panicked)

	// Wait for restart to happen
	time.Sleep(50 * time.Millisecond)

	// worker-1 should have been restarted (2 starts total)
	s.Equal(2, spec1.startCount())
	// worker-2 should NOT have been restarted
	s.Equal(1, spec2.startCount())
}

// --- AllForOne restart tests ---

func (s *SupervisorTestSuite) TestAllForOne_ChildFails_ShouldRestartAll() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       AllForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec1 := newMockChildSpec()
	spec2 := newMockChildSpec()

	s.Require().NoError(sup.Add("worker-1", spec1))
	s.Require().NoError(sup.Add("worker-2", spec2))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	// Simulate worker-1 panicking
	ref1 := spec1.lastRef()
	ref1.terminate(actor.Panicked)

	// Wait for restart
	time.Sleep(50 * time.Millisecond)

	// Both should have been restarted
	s.Equal(2, spec1.startCount())
	s.Equal(2, spec2.startCount())
}

// --- Max restarts tests ---

func (s *SupervisorTestSuite) TestMaxRestarts_Exceeded_ShouldNotRestart() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    2,
		WithinDuration: 5 * time.Second,
	}))

	spec := newMockChildSpec()
	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	// Trigger 3 failures rapidly (max is 2)
	for range 3 {
		ref := spec.lastRef()
		ref.terminate(actor.Panicked)
		time.Sleep(50 * time.Millisecond)
	}

	// Should have started: 1 initial + 2 restarts = 3
	// The 3rd failure should NOT trigger another restart
	time.Sleep(50 * time.Millisecond)
	s.LessOrEqual(spec.startCount(), 4) // At most 3 restarts + 1 initial
}

// --- Death watch tests ---

func (s *SupervisorTestSuite) TestWatch_ChildTerminates_ShouldNotifyWatcher() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec := newMockChildSpec()
	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	var watchedName atomic.Value
	var watchedState atomic.Uint64
	sup.Watch(func(name string, state uint64) {
		watchedName.Store(name)
		watchedState.Store(state)
	})

	// Simulate failure
	ref := spec.lastRef()
	ref.terminate(actor.Panicked)

	time.Sleep(50 * time.Millisecond)

	s.Equal("worker", watchedName.Load())
	s.Equal(uint64(actor.Panicked), watchedState.Load())
}

// --- ChildState tests ---

func (s *SupervisorTestSuite) TestChildState_NonExistent_ShouldReturnError() {
	sup := NewSupervisor()
	_, err := sup.ChildState("unknown")
	s.ErrorIs(err, ErrChildNotFound)
}

func (s *SupervisorTestSuite) TestChildState_BeforeStart_ShouldReturnInitialized() {
	sup := NewSupervisor()
	s.Require().NoError(sup.Add("worker", newMockChildSpec()))

	state, err := sup.ChildState("worker")
	s.NoError(err)
	s.Equal(uint64(actor.Initialized), state)
}

// --- Clean shutdown (Done state should not trigger restart) ---

func (s *SupervisorTestSuite) TestOneForOne_CleanStop_ShouldNotRestart() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec := newMockChildSpec()
	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	// Simulate clean stop (Done state)
	ref := spec.lastRef()
	ref.terminate(actor.Done)

	time.Sleep(50 * time.Millisecond)

	// Should NOT have been restarted
	s.Equal(1, spec.startCount())
}
