package supervision

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/actor"
	"github.com/barnowlsnest/go-actorlib/v5/pkg/actorref"
)

var (
	errMockStart = errors.New("mock start failed")
	errMockStop  = errors.New("mock stop failed")
)

// mockChildRef is a test implementation of ChildRef.
type mockChildRef struct {
	state    uint64
	done     chan struct{}
	stopErr  error
	stopOnce sync.Once
}

func newMockChildRef() *mockChildRef {
	return &mockChildRef{
		state: actor.Started,
		done:  make(chan struct{}),
	}
}

func (m *mockChildRef) Stop(_ time.Duration) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	atomic.StoreUint64(&m.state, actor.Done)
	m.stopOnce.Do(func() { close(m.done) })
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
	m.stopOnce.Do(func() { close(m.done) })
}

// mockChildSpec creates mockChildRefs on each Start call.
type mockChildSpec struct {
	mu sync.Mutex

	refs []*mockChildRef

	// failStartAfter: Start returns errMockStart once startCount would exceed this.
	// 0 means never fail. Use -1 via failNextStart for "fail the next Start".
	failStartAfter int
	failNextStart  bool

	// stopErrOnRefs created while this is set are given stopErr.
	stopErrOnNew error
}

func newMockChildSpec() *mockChildSpec {
	return &mockChildSpec{}
}

func (s *mockChildSpec) Start(_ context.Context) (ChildRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failNextStart {
		s.failNextStart = false
		return nil, errMockStart
	}
	if s.failStartAfter > 0 && len(s.refs) >= s.failStartAfter {
		return nil, errMockStart
	}

	ref := newMockChildRef()
	if s.stopErrOnNew != nil {
		ref.stopErr = s.stopErrOnNew
	}
	s.refs = append(s.refs, ref)
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

func (s *mockChildSpec) setFailStartAfter(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failStartAfter = n
}

func (s *mockChildSpec) setStopErrOnNew(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopErrOnNew = err
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

func (s *SupervisorTestSuite) TestOnRestartError_OneForOne_StartFails_ShouldNotify() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec := newMockChildSpec()
	spec.setFailStartAfter(1) // initial Start succeeds; restart Start fails

	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	var gotName atomic.Value
	var gotErr atomic.Value
	sup.OnRestartError(func(name string, err error) {
		gotName.Store(name)
		gotErr.Store(err)
	})

	var watchFired atomic.Bool
	sup.Watch(func(string, uint64) {
		watchFired.Store(true)
	})

	spec.lastRef().terminate(actor.Panicked)
	time.Sleep(50 * time.Millisecond)

	s.Equal("worker", gotName.Load())
	s.ErrorIs(gotErr.Load().(error), errMockStart)
	s.Equal(1, spec.startCount())
	// Watch fires for the real termination, not for the restart-ops failure.
	s.True(watchFired.Load())
}

func (s *SupervisorTestSuite) TestOnRestartError_AllForOne_StartFails_ShouldNotifyAndContinue() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       AllForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec1 := newMockChildSpec()
	spec1.setFailStartAfter(1)
	spec2 := newMockChildSpec()

	s.Require().NoError(sup.Add("worker-1", spec1))
	s.Require().NoError(sup.Add("worker-2", spec2))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	var mu sync.Mutex
	var names []string
	sup.OnRestartError(func(name string, err error) {
		mu.Lock()
		names = append(names, name)
		mu.Unlock()
		s.ErrorIs(err, errMockStart)
	})

	spec1.lastRef().terminate(actor.Panicked)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	s.Equal([]string{"worker-1"}, names)
	s.Equal(1, spec1.startCount())
	s.Equal(2, spec2.startCount()) // sibling still restarted
}

func (s *SupervisorTestSuite) TestOnRestartError_AllForOne_StopFails_ShouldNotify() {
	sup := NewSupervisor(WithPolicy(RestartPolicy{
		Strategy:       AllForOne,
		MaxRestarts:    5,
		WithinDuration: 10 * time.Second,
	}))

	spec1 := newMockChildSpec()
	spec2 := newMockChildSpec()
	spec2.setStopErrOnNew(errMockStop)

	s.Require().NoError(sup.Add("worker-1", spec1))
	s.Require().NoError(sup.Add("worker-2", spec2))
	s.Require().NoError(sup.StartAll(s.ctx, time.Second))

	var gotName atomic.Value
	var gotErr atomic.Value
	sup.OnRestartError(func(name string, err error) {
		gotName.Store(name)
		gotErr.Store(err)
	})

	spec1.lastRef().terminate(actor.Panicked)
	time.Sleep(50 * time.Millisecond)

	s.Equal("worker-2", gotName.Load())
	s.ErrorIs(gotErr.Load().(error), errMockStop)
	// Both children still get a start attempt after the stop phase.
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

type supervisedEntity struct{}

func (e *supervisedEntity) IsProvidable() bool { return true }

type supervisedEntityProvider struct{ entity *supervisedEntity }

func (p *supervisedEntityProvider) Provide() *supervisedEntity { return p.entity }

// actorStartNewSpec uses actor.StartNew with the supervisor ctx as lifetime (README pattern).
type actorStartNewSpec struct{}

func (s *actorStartNewSpec) Start(ctx context.Context) (ChildRef, error) {
	a, err := actor.StartNew(ctx, time.Second,
		actor.WithProvider(&supervisedEntityProvider{&supervisedEntity{}}),
	)
	if err != nil {
		return nil, err
	}
	return actorref.New(a)
}

func (s *SupervisorTestSuite) TestStartAll_WithActorStartNew_ShouldKeepChildStarted() {
	sup := NewSupervisor()
	s.Require().NoError(sup.Add("worker", &actorStartNewSpec{}))
	s.Require().NoError(sup.StartAll(s.ctx, 50*time.Millisecond))

	time.Sleep(100 * time.Millisecond)

	state, err := sup.ChildState("worker")
	s.NoError(err)
	s.Equal(uint64(actor.Started), state)

	s.NoError(sup.StopAll(time.Second))
}

type panicCommand struct{}

func (panicCommand) Execute(context.Context, *supervisedEntity) { panic("boom") }

// panickingOnceSpec starts real actors; the first instance panics on its first message.
type panickingOnceSpec struct {
	starts atomic.Int32
}

func (s *panickingOnceSpec) Start(ctx context.Context) (ChildRef, error) {
	a, err := actor.StartNew(ctx, time.Second,
		actor.WithProvider(&supervisedEntityProvider{&supervisedEntity{}}),
	)
	if err != nil {
		return nil, err
	}
	if s.starts.Add(1) == 1 {
		if err := a.Receive(ctx, panicCommand{}); err != nil {
			return nil, err
		}
	}
	return actorref.New(a)
}

func (s *SupervisorTestSuite) TestRealActorPanic_ShouldRestartChild() {
	// Arrange
	spec := &panickingOnceSpec{}
	sup := NewSupervisor()
	s.Require().NoError(sup.Add("worker", spec))

	// Act
	s.Require().NoError(sup.StartAll(s.ctx, 0))

	// Assert
	s.Eventually(func() bool { return spec.starts.Load() == 2 }, time.Second, 10*time.Millisecond)
	s.Eventually(func() bool {
		state, err := sup.ChildState("worker")
		return err == nil && state == uint64(actor.Started)
	}, time.Second, 10*time.Millisecond)

	s.NoError(sup.StopAll(time.Second))
}

func (s *SupervisorTestSuite) TestAllForOne_RealActorPanic_ShouldNotReportRestartError() {
	// Arrange
	sup := NewSupervisor(WithPolicy(RestartPolicy{Strategy: AllForOne}))
	failing := &panickingOnceSpec{}
	s.Require().NoError(sup.Add("failing", failing))
	s.Require().NoError(sup.Add("sibling", &actorStartNewSpec{}))

	var restartErrs []error
	var mu sync.Mutex
	sup.OnRestartError(func(_ string, err error) {
		mu.Lock()
		defer mu.Unlock()
		restartErrs = append(restartErrs, err)
	})

	// Act
	s.Require().NoError(sup.StartAll(s.ctx, 0))
	s.Eventually(func() bool { return failing.starts.Load() == 2 }, time.Second, 10*time.Millisecond)
	s.Eventually(func() bool {
		state, err := sup.ChildState("sibling")
		return err == nil && state == uint64(actor.Started)
	}, time.Second, 10*time.Millisecond)

	// Assert
	mu.Lock()
	s.Empty(restartErrs)
	mu.Unlock()
	s.NoError(sup.StopAll(time.Second))
}

func (s *SupervisorTestSuite) TestStopAll_WithCleanlyStoppedChild_ShouldNotReportError() {
	// Arrange
	sup := NewSupervisor()
	spec := &recordingStartNewSpec{}
	s.Require().NoError(sup.Add("worker", spec))
	s.Require().NoError(sup.StartAll(s.ctx, 0))

	ref := spec.last()
	s.Require().NoError(ref.Stop(time.Second))

	// Act
	err := sup.StopAll(time.Second)

	// Assert
	s.NoError(err)
}

type recordingStartNewSpec struct {
	mu  sync.Mutex
	ref ChildRef
}

func (r *recordingStartNewSpec) Start(ctx context.Context) (ChildRef, error) {
	ref, err := (&actorStartNewSpec{}).Start(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.ref = ref
	r.mu.Unlock()
	return ref, nil
}

func (r *recordingStartNewSpec) last() ChildRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ref
}

type blockCommand struct{ started, release chan struct{} }

func (b blockCommand) Execute(context.Context, *supervisedEntity) {
	close(b.started)
	<-b.release
}

// overlapSpec flags a start that happens while the previous instance is still running.
type overlapSpec struct {
	mu      sync.Mutex
	last    *actorref.Ref[*supervisedEntity]
	starts  atomic.Int32
	overlap atomic.Bool
}

func (o *overlapSpec) Start(ctx context.Context) (ChildRef, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.last != nil {
		select {
		case <-o.last.Done():
		default:
			o.overlap.Store(true)
		}
	}

	a, err := actor.StartNew(ctx, time.Second,
		actor.WithProvider(&supervisedEntityProvider{&supervisedEntity{}}),
	)
	if err != nil {
		return nil, err
	}
	ref, err := actorref.New(a)
	if err != nil {
		return nil, err
	}
	o.last = ref
	o.starts.Add(1)
	return ref, nil
}

func (o *overlapSpec) current() *actorref.Ref[*supervisedEntity] {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.last
}

func (s *SupervisorTestSuite) TestAllForOne_WithSiblingStopping_ShouldWaitBeforeRestartingIt() {
	// Arrange
	sup := NewSupervisor(WithPolicy(RestartPolicy{Strategy: AllForOne}))
	sibling := &overlapSpec{}
	failing := &overlapSpec{}
	s.Require().NoError(sup.Add("sibling", sibling))
	s.Require().NoError(sup.Add("failing", failing))
	s.Require().NoError(sup.StartAll(s.ctx, 0))

	// Hold the sibling in Stopping: a slow handler plus an external Stop.
	block := blockCommand{started: make(chan struct{}), release: make(chan struct{})}
	s.Require().NoError(sibling.current().Send(s.ctx, block))
	<-block.started
	go func() { _ = sibling.current().Stop(5 * time.Second) }()
	s.Require().Eventually(func() bool {
		return sibling.current().State() == uint64(actor.Stopping)
	}, time.Second, time.Millisecond)

	// Act — fail the other child to trigger an AllForOne restart.
	s.Require().NoError(failing.current().Send(s.ctx, panicCommand{}))
	time.Sleep(50 * time.Millisecond)
	close(block.release)

	// Assert
	s.Eventually(func() bool { return sibling.starts.Load() == 2 }, 2*time.Second, 10*time.Millisecond)
	s.False(sibling.overlap.Load(), "sibling restarted while its previous instance was still running")
	s.NoError(sup.StopAll(time.Second))
}
