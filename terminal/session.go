package terminal

import (
	"context"
	"errors"
	"fmt"
)

// Session is the reusable command-running core shared by the GUI and tests. It
// keeps execution, event collection and command-history persistence in the
// terminal package so UI code does not duplicate business logic.
type Session struct {
	Executor Executor
	History  *HistoryStore
}

// NewSession creates a command session with the default local/SSH executor.
func NewSession(history *HistoryStore) *Session {
	return &Session{Executor: NewExecutor(), History: history}
}

// RunCommand executes command against conn and appends a secret-free history
// entry when a HistoryStore is configured. A command failure is returned as the
// primary error; a history write failure is returned only when the command itself
// succeeded.
func (s *Session) RunCommand(ctx context.Context, conn Connection, command string) (CommandResult, error) {
	if s == nil {
		return CommandResult{Connection: conn, Command: command, ExitCode: -1}, errors.New("terminal session is nil")
	}
	exec := s.Executor
	if exec == nil {
		exec = NewExecutor()
	}
	result, runErr := exec.Run(ctx, conn, command)
	if s.History == nil {
		return result, runErr
	}
	if histErr := s.History.Append(HistoryFromResult(result)); histErr != nil {
		if runErr != nil {
			return result, fmt.Errorf("%w; additionally failed to append command history: %v", runErr, histErr)
		}
		return result, fmt.Errorf("append command history: %w", histErr)
	}
	return result, runErr
}
