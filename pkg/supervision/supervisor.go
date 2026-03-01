// Package supervision provides supervisor functionality for actor lifecycle management.
//
// A Supervisor monitors child actors and restarts them according to a configurable
// restart policy when they fail (panic or stop with error). It supports two strategies:
//   - OneForOne: only the failed child is restarted
//   - AllForOne: all children are restarted when any child fails
//
// The supervisor tracks restart frequency and will stop itself if the maximum
// number of restarts is exceeded within the configured time window.
//
// Example usage:
//
//	sup := supervision.NewSupervisor(
//		supervision.WithPolicy(supervision.RestartPolicy{
//			Strategy:       supervision.OneForOne,
//			MaxRestarts:    3,
//			WithinDuration: 5 * time.Second,
//		}),
//	)
//
//	sup.Add("worker", &MyChildSpec{})
//	sup.StartAll(ctx, 5*time.Second)
//	defer sup.StopAll(5 * time.Second)
package supervision

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/barnowlsnest/go-actorlib/v3/pkg/actor"
)

// ChildSpec defines how to create and start a child actor.
// Implementations provide the factory logic for actor creation.
type ChildSpec interface {
	// Start creates and starts a new instance of the child actor.
	// It returns a ChildRef that the supervisor can monitor and manage.
	Start(ctx context.Context) (ChildRef, error)
}

// ChildRef is the minimal interface a supervised child must implement.
// It provides lifecycle management and state observation.
type ChildRef interface {
	// Stop initiates graceful shutdown of the child within the timeout.
	Stop(timeout time.Duration) error

	// State returns the current lifecycle state of the child.
	State() uint64

	// Done returns a channel that is closed when the child terminates.
	Done() <-chan struct{}
}

// WatchCallback is called when a child terminates.
// The name is the child's registered name.
type WatchCallback func(name string, state uint64)

type child struct {
	name    string
	spec    ChildSpec
	ref     ChildRef
	version uint64 // incremented on each restart to invalidate stale monitors
}

// Supervisor monitors child actors and restarts them according to a restart policy.
// It is safe for concurrent use.
type Supervisor struct {
	mu       sync.Mutex
	policy   RestartPolicy
	children []*child
	names    map[string]int // name → index in children slice
	restarts []time.Time    // restart timestamps for frequency tracking
	stopped  bool
	watchers []WatchCallback

	stopTimeout time.Duration
}

// Option configures a Supervisor during creation.
type Option func(*Supervisor)

// WithPolicy sets the restart policy for the supervisor.
func WithPolicy(policy RestartPolicy) Option {
	return func(s *Supervisor) {
		s.policy = policy
	}
}

// WithStopTimeout sets the timeout used when stopping children during restarts.
// Defaults to 5 seconds.
func WithStopTimeout(timeout time.Duration) Option {
	return func(s *Supervisor) {
		s.stopTimeout = timeout
	}
}

// NewSupervisor creates a new Supervisor with the given options.
func NewSupervisor(opts ...Option) *Supervisor {
	s := &Supervisor{
		policy:      DefaultRestartPolicy(),
		names:       make(map[string]int),
		stopTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Watch registers a callback that is invoked when any child terminates.
// Multiple watchers can be registered.
func (s *Supervisor) Watch(cb WatchCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, cb)
}

// Add registers a child spec with the supervisor under the given name.
// The child is not started until StartAll is called.
func (s *Supervisor) Add(name string, spec ChildSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return ErrSupervisorStopped
	}
	if name == "" {
		return ErrChildNameEmpty
	}
	if spec == nil {
		return ErrNilChildSpec
	}
	if _, exists := s.names[name]; exists {
		return ErrChildNameDuplicate
	}

	idx := len(s.children)
	s.children = append(s.children, &child{
		name: name,
		spec: spec,
	})
	s.names[name] = idx
	return nil
}

// StartAll starts all registered children in order.
// It monitors each child for termination and handles restarts according to the policy.
func (s *Supervisor) StartAll(ctx context.Context, readyTimeout time.Duration) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrSupervisorStopped
	}
	children := make([]*child, len(s.children))
	copy(children, s.children)
	s.mu.Unlock()

	for _, c := range children {
		if err := s.startChild(ctx, c, readyTimeout); err != nil {
			return err
		}
	}
	return nil
}

func (s *Supervisor) startChild(ctx context.Context, c *child, _ time.Duration) error {
	ref, err := c.spec.Start(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	c.ref = ref
	c.version++
	ver := c.version
	s.mu.Unlock()

	// Monitor child in background — captures the version at start time
	go s.monitor(ctx, c, ver)

	return nil
}

// monitor watches a specific child ref (identified by version).
// If the child's version has changed when done fires, this monitor is stale and exits.
func (s *Supervisor) monitor(ctx context.Context, c *child, version uint64) {
	s.mu.Lock()
	ref := c.ref
	s.mu.Unlock()

	select {
	case <-ref.Done():
		// Check if this monitor is still for the current version
		s.mu.Lock()
		if c.version != version {
			s.mu.Unlock()
			return // Stale monitor — child was already restarted
		}
		s.mu.Unlock()

		s.handleTermination(ctx, c)
	case <-ctx.Done():
		return
	}
}

func (s *Supervisor) handleTermination(ctx context.Context, c *child) {
	s.mu.Lock()
	state := c.ref.State()

	watchers := make([]WatchCallback, len(s.watchers))
	copy(watchers, s.watchers)
	s.mu.Unlock()

	// Notify watchers
	for _, cb := range watchers {
		cb(c.name, state)
	}

	// Only restart on failure states
	if state == actor.Done {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	// Check restart frequency
	if s.policy.MaxRestarts > 0 {
		now := time.Now()
		s.restarts = append(s.restarts, now)

		// Trim restarts outside the window
		cutoff := now.Add(-s.policy.WithinDuration)
		trimIdx := 0
		for trimIdx < len(s.restarts) && s.restarts[trimIdx].Before(cutoff) {
			trimIdx++
		}
		s.restarts = s.restarts[trimIdx:]

		if len(s.restarts) > s.policy.MaxRestarts {
			s.mu.Unlock()
			return
		}
	}

	strategy := s.policy.Strategy
	s.mu.Unlock()

	switch strategy {
	case OneForOne:
		s.restartOne(ctx, c)
	case AllForOne:
		s.restartAll(ctx)
	}
}

func (s *Supervisor) restartOne(ctx context.Context, c *child) {
	if err := s.startChild(ctx, c, 5*time.Second); err != nil {
		s.mu.Lock()
		watchers := make([]WatchCallback, len(s.watchers))
		copy(watchers, s.watchers)
		s.mu.Unlock()

		for _, cb := range watchers {
			cb(c.name, actor.StoppedWithError)
		}
	}
}

func (s *Supervisor) restartAll(ctx context.Context) {
	s.mu.Lock()
	children := make([]*child, len(s.children))
	copy(children, s.children)
	s.mu.Unlock()

	// Stop all children — bump version first to invalidate stale monitors
	for _, c := range children {
		s.mu.Lock()
		c.version++ // Invalidate any pending monitor for this child
		ref := c.ref
		s.mu.Unlock()
		if ref != nil {
			_ = ref.Stop(s.stopTimeout)
		}
	}

	// Restart all children
	for _, c := range children {
		_ = s.startChild(ctx, c, 5*time.Second)
	}
}

// StopAll stops all children in reverse order.
func (s *Supervisor) StopAll(timeout time.Duration) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return ErrSupervisorStopped
	}
	s.stopped = true
	children := make([]*child, len(s.children))
	copy(children, s.children)
	s.mu.Unlock()

	var errs []error
	for i := len(children) - 1; i >= 0; i-- {
		c := children[i]
		s.mu.Lock()
		ref := c.ref
		s.mu.Unlock()
		if ref != nil {
			if err := ref.Stop(timeout); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// Children returns the names of all registered children.
func (s *Supervisor) Children() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, len(s.children))
	for i, c := range s.children {
		names[i] = c.name
	}
	return names
}

// ChildState returns the current state of a named child.
func (s *Supervisor) ChildState(name string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.names[name]
	if !ok {
		return 0, ErrChildNotFound
	}

	c := s.children[idx]
	if c.ref == nil {
		return actor.Initialized, nil
	}
	return c.ref.State(), nil
}
