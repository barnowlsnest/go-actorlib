package ask

import "errors"

// ErrAskTimeout is returned when the ask operation does not receive a result
// from the command within the specified timeout duration.
var ErrAskTimeout = errors.New("ask timeout waiting for command result")
