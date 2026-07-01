package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type sessionFakeExecutor struct {
	result CommandResult
	err    error
	conn   Connection
	cmd    string
}

func (e *sessionFakeExecutor) Run(_ context.Context, conn Connection, command string) (CommandResult, error) {
	e.conn = conn
	e.cmd = command
	if e.result.Command == "" {
		e.result.Command = command
	}
	if e.result.Connection.ID == "" {
		e.result.Connection = conn
	}
	return e.result, e.err
}

func TestSessionRunCommandAppendsHistory(t *testing.T) {
	historyPath := filepath.Join(t.TempDir(), "history.jsonl")
	conn := DefaultLocalConnection()
	started := time.Now().Add(-time.Second)
	finished := time.Now()
	exec := &sessionFakeExecutor{result: CommandResult{Connection: conn, Command: "echo ok", StartedAt: started, FinishedAt: finished, ExitCode: 0, Stdout: "ok\n"}}
	sess := &Session{Executor: exec, History: NewHistoryStore(historyPath)}

	result, err := sess.RunCommand(context.Background(), conn, "echo ok")
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}
	if result.Stdout != "ok\n" || exec.cmd != "echo ok" || exec.conn.ID != conn.ID {
		t.Fatalf("unexpected execution result/state: result=%#v exec=%#v", result, exec)
	}
	entries, err := NewHistoryStore(historyPath).Load(10)
	if err != nil {
		t.Fatalf("Load history failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Command != "echo ok" || entries[0].Stdout != "ok\n" || entries[0].ConnectionID != conn.ID {
		t.Fatalf("unexpected history entries: %#v", entries)
	}
}

func TestSessionRunCommandPreservesCommandErrorAndReportsHistoryError(t *testing.T) {
	conn := DefaultLocalConnection()
	want := errors.New("exit 9")
	badHistoryPath := filepath.Join(t.TempDir(), "dir-history")
	if err := os.MkdirAll(badHistoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	exec := &sessionFakeExecutor{result: CommandResult{Connection: conn, Command: "false", FinishedAt: time.Now(), ExitCode: 9}, err: want}
	sess := &Session{Executor: exec, History: NewHistoryStore(badHistoryPath)}

	_, err := sess.RunCommand(context.Background(), conn, "false")
	if err == nil || !strings.Contains(err.Error(), want.Error()) || !strings.Contains(err.Error(), "history") {
		t.Fatalf("expected command error plus history context, got %v", err)
	}
}

func TestSessionOnEventReceivesDefaultLocalEvents(t *testing.T) {
	conn := DefaultLocalConnection()
	var events []Event
	sess := NewSession(nil)
	sess.OnEvent = func(event Event) { events = append(events, event) }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := sess.RunCommand(ctx, conn, "printf 'stream-ok\\n'")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "stream-ok" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if len(events) != len(result.Events) {
		t.Fatalf("expected streamed event count %d, got %d (%#v)", len(result.Events), len(events), events)
	}
	if len(events) < 2 || events[0].Stream != StreamStdout || events[0].Line != "stream-ok" || events[len(events)-1].Stream != StreamStatus {
		t.Fatalf("unexpected streamed events: %#v", events)
	}
}

func TestSessionOnEventReplaysNonStreamingExecutorEvents(t *testing.T) {
	conn := DefaultLocalConnection()
	status := Event{ConnectionID: conn.ID, Stream: StreamStatus, Line: "exit code 0"}
	exec := &sessionFakeExecutor{result: CommandResult{Connection: conn, Command: "noop", ExitCode: 0, Events: []Event{status}}}
	var events []Event
	sess := &Session{Executor: exec, OnEvent: func(event Event) { events = append(events, event) }}

	_, err := sess.RunCommand(context.Background(), conn, "noop")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}
	if len(events) != 1 || events[0].Line != status.Line {
		t.Fatalf("expected replayed status event, got %#v", events)
	}
}
