package command

import (
	"errors"
)

var (
	ErrCommandContextCancelled = errors.New("command context canceled")

	ErrCommandPanic = errors.New("command panicked")
)
