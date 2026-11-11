// Package ask waits for the result of a request/response command.
//
// [Await] is the shared wait used by [actorref.Ref.Ask] and
// [system.ActorSystem.Ask]: it returns when the command completes, ctx is
// done, or the timeout elapses ([ErrAskTimeout]).
//
// Most callers use those methods directly:
//
//	result, err := ref.Ask(ctx, func(entity *MyEntity) (int, error) {
//	    entity.value++
//	    return entity.value, nil
//	}, 5*time.Second)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Result: %d\n", result)
package ask

import (
	"context"
	"time"

	"github.com/barnowlsnest/go-actorlib/v5/pkg/command"
)

// Await waits for a command result until it completes, ctx is done, or timeout elapses.
func Await[E, R any](
	ctx context.Context,
	cmd *command.Command[E, R],
	timeout time.Duration,
) (R, error) {
	var zero R

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result, ok := <-cmd.Done():
		if !ok {
			return zero, cmd.Error()
		}
		return result, cmd.Error()
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, ErrAskTimeout
	}
}
