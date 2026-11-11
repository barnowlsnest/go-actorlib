// Package stopwait holds the shutdown wait shared by the system and supervision
// packages.
package stopwait

import (
	"fmt"
	"time"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/actor"
)

// Wait returns nil once done is closed, or an error wrapping
// actor.ErrActorGracefulStopTimeout and cause if timeout elapses first.
//
// An already-closed done always wins, including when timeout <= 0, so an actor
// that has exited is never reported as timed out.
func Wait(done <-chan struct{}, timeout time.Duration, cause error) error {
	select {
	case <-done:
		return nil
	default:
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w: actor did not exit within %v (%w)", actor.ErrActorGracefulStopTimeout, timeout, cause)
	}
}
