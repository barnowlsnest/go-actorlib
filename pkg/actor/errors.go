package actor

import (
	"errors"
)

var (
	ErrActorPanic = errors.New("actor panic")
	
	ErrActorNilProvider = errors.New("entity provider should not be nil")
	
	ErrActorNilEntity = errors.New("entity provider returned nil entity")
	
	ErrActorGracefulStopTimeout = errors.New("actor graceful stop timeout")
	
	ErrActorReceiveTimeout = errors.New("actor receive timeout")
	
	ErrActorContextCanceled = errors.New("actor context err")
	
	ErrActorContextDeadlineExceeded = errors.New("actor context deadline exceeded")
	
	ErrActorReceiveNil = errors.New("actor receive nil command")
	
	ErrActorReceiveOnStopped = errors.New("actor receive on stopped actor")
)
