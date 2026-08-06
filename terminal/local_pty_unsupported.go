//go:build windows || !cgo

package terminal

import (
	"context"
	"errors"
)

type unsupportedLocalPTYSession struct {
	output chan []byte
}

func newLocalPTYSession(Connection) InteractiveSession {
	output := make(chan []byte)
	close(output)
	return &unsupportedLocalPTYSession{output: output}
}

func (*unsupportedLocalPTYSession) Start(context.Context, TerminalSize) error {
	return ErrInteractiveTerminalUnsupported
}
func (*unsupportedLocalPTYSession) WriteInput([]byte) error { return ErrInteractiveTerminalUnsupported }
func (*unsupportedLocalPTYSession) Resize(TerminalSize) error {
	return ErrInteractiveTerminalUnsupported
}
func (*unsupportedLocalPTYSession) Interrupt() error { return ErrInteractiveTerminalUnsupported }
func (*unsupportedLocalPTYSession) Close() error     { return nil }
func (*unsupportedLocalPTYSession) Wait() error {
	return errors.New("interactive session was not started")
}
func (s *unsupportedLocalPTYSession) Output() <-chan []byte { return s.output }
