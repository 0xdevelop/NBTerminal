package terminal

import (
	"context"
	"errors"
)

var ErrInteractiveTerminalUnsupported = errors.New("interactive terminal transport is not supported on this platform")

// TerminalSize is expressed in character cells, matching PTY and SSH window
// sizing APIs. Pixel dimensions belong to the native renderer, not transport.
type TerminalSize struct {
	Columns uint16
	Rows    uint16
}

func (s TerminalSize) validate() error {
	if s.Columns == 0 || s.Rows == 0 {
		return errors.New("terminal rows and columns must be greater than zero")
	}
	return nil
}

// InteractiveSession is the transport contract shared by local PTY, SSH PTY,
// and future ConPTY backends. Output is an arbitrary byte stream; callers must
// not line-scan or assume chunks end on UTF-8 or escape-sequence boundaries.
type InteractiveSession interface {
	Start(context.Context, TerminalSize) error
	WriteInput([]byte) error
	Resize(TerminalSize) error
	Interrupt() error
	Close() error
	Wait() error
	Output() <-chan []byte
}

// NewLocalPTYSession constructs a long-lived local-shell transport. Start owns
// validation and platform capability reporting so callers can create sessions
// before deciding when a tab should launch its shell.
func NewLocalPTYSession(conn Connection) InteractiveSession {
	conn.Normalize()
	return newLocalPTYSession(conn)
}
