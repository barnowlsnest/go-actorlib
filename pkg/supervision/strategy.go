package supervision

import "time"

// Strategy defines how a supervisor responds to child failures.
type Strategy int

const (
	// OneForOne restarts only the failed child.
	OneForOne Strategy = iota

	// AllForOne restarts all children when any child fails.
	AllForOne
)

// RestartPolicy configures restart behavior for a supervisor.
type RestartPolicy struct {
	// Strategy determines how child failures are handled.
	Strategy Strategy

	// MaxRestarts is the maximum number of restarts allowed within the time window.
	// If exceeded, the supervisor itself fails with ErrMaxRestartsExceeded.
	// Zero means unlimited restarts.
	MaxRestarts int

	// WithinDuration is the time window for counting restarts.
	// Only relevant when MaxRestarts > 0.
	WithinDuration time.Duration
}

// DefaultRestartPolicy returns a sensible default restart policy:
// OneForOne strategy, max 3 restarts within 5 seconds.
func DefaultRestartPolicy() RestartPolicy {
	return RestartPolicy{
		Strategy:       OneForOne,
		MaxRestarts:    3,
		WithinDuration: 5 * time.Second,
	}
}
