package guis

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/0xdevelop/NBTerminal/terminal"
)

type fakeInteractiveSession struct {
	mu         sync.Mutex
	started    []terminal.TerminalSize
	inputs     [][]byte
	resizes    []terminal.TerminalSize
	interrupts int
	closes     int
	output     chan []byte
	done       chan struct{}
	startErr   error
	waitErr    error
}

func newFakeInteractiveSession() *fakeInteractiveSession {
	return &fakeInteractiveSession{output: make(chan []byte, 4), done: make(chan struct{})}
}

func (s *fakeInteractiveSession) Start(_ context.Context, size terminal.TerminalSize) error {
	s.mu.Lock()
	s.started = append(s.started, size)
	s.mu.Unlock()
	return s.startErr
}
func (s *fakeInteractiveSession) WriteInput(data []byte) error {
	s.mu.Lock()
	s.inputs = append(s.inputs, append([]byte(nil), data...))
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSession) Resize(size terminal.TerminalSize) error {
	s.mu.Lock()
	s.resizes = append(s.resizes, size)
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSession) Interrupt() error {
	s.mu.Lock()
	s.interrupts++
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSession) Close() error {
	s.mu.Lock()
	s.closes++
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSession) Wait() error           { <-s.done; return s.waitErr }
func (s *fakeInteractiveSession) Output() <-chan []byte { return s.output }

func TestInteractiveRuntimeRegistryRoutesByStableSessionID(t *testing.T) {
	registry := newInteractiveRuntimeRegistry()
	first := newFakeInteractiveSession()
	second := newFakeInteractiveSession()
	size := terminal.TerminalSize{Columns: 100, Rows: 30}

	if err := registry.Start(context.Background(), "runtime-1", first, size); err != nil {
		t.Fatalf("start first runtime: %v", err)
	}
	if err := registry.Start(context.Background(), "runtime-2", second, size); err != nil {
		t.Fatalf("start second runtime: %v", err)
	}
	if err := registry.WriteInput("runtime-1", []byte("printf '简体·繁體·Русский'\r")); err != nil {
		t.Fatalf("write first runtime: %v", err)
	}
	if err := registry.Resize("runtime-2", terminal.TerminalSize{Columns: 132, Rows: 42}); err != nil {
		t.Fatalf("resize second runtime: %v", err)
	}
	if err := registry.Interrupt("runtime-1"); err != nil {
		t.Fatalf("interrupt first runtime: %v", err)
	}

	first.mu.Lock()
	if len(first.inputs) != 1 || string(first.inputs[0]) != "printf '简体·繁體·Русский'\r" || first.interrupts != 1 {
		t.Fatalf("first runtime received wrong events: inputs=%q interrupts=%d", first.inputs, first.interrupts)
	}
	first.mu.Unlock()
	second.mu.Lock()
	if len(second.inputs) != 0 || len(second.resizes) != 1 || second.resizes[0].Columns != 132 {
		t.Fatalf("second runtime state leaked or resize missing: inputs=%q resizes=%+v", second.inputs, second.resizes)
	}
	second.mu.Unlock()
}

func TestInteractiveRuntimeRegistryOwnsCloseAndRejectsDuplicates(t *testing.T) {
	registry := newInteractiveRuntimeRegistry()
	session := newFakeInteractiveSession()
	if err := registry.Start(context.Background(), "runtime-1", session, terminal.TerminalSize{Columns: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Start(context.Background(), "runtime-1", newFakeInteractiveSession(), terminal.TerminalSize{Columns: 80, Rows: 24}); !errors.Is(err, errInteractiveRuntimeExists) {
		t.Fatalf("duplicate start error = %v", err)
	}
	if !registry.Close("runtime-1") {
		t.Fatal("close did not find runtime")
	}
	if registry.Has("runtime-1") {
		t.Fatal("closed runtime remained registered")
	}
	session.mu.Lock()
	if session.closes != 1 {
		t.Fatalf("transport close count = %d, want 1", session.closes)
	}
	session.mu.Unlock()
	if err := registry.WriteInput("runtime-1", []byte("lost")); !errors.Is(err, errInteractiveRuntimeNotFound) {
		t.Fatalf("write after close error = %v", err)
	}
}

func TestInteractiveRuntimeRegistryReplacesOnlyExistingRuntime(t *testing.T) {
	registry := newInteractiveRuntimeRegistry()
	first := newFakeInteractiveSession()
	second := newFakeInteractiveSession()
	failing := newFakeInteractiveSession()
	failing.startErr = errors.New("replacement start failed")
	size := terminal.TerminalSize{Columns: 100, Rows: 30}
	if err := registry.Start(context.Background(), "runtime-1", first, size); err != nil {
		t.Fatal(err)
	}

	if err := registry.Replace(context.Background(), "missing", second, size); !errors.Is(err, errInteractiveRuntimeNotFound) {
		t.Fatalf("replace missing runtime error = %v", err)
	}
	if err := registry.Replace(context.Background(), "runtime-1", failing, size); !errors.Is(err, failing.startErr) {
		t.Fatalf("failed replacement error = %v", err)
	}
	if current, ok := registry.Session("runtime-1"); !ok || current != first {
		t.Fatal("failed replacement discarded the working runtime")
	}
	if err := registry.Replace(context.Background(), "runtime-1", second, size); err != nil {
		t.Fatal(err)
	}
	if current, ok := registry.Session("runtime-1"); !ok || current != second {
		t.Fatalf("replacement runtime = %#v, ok=%t", current, ok)
	}
	first.mu.Lock()
	firstCloses := first.closes
	first.mu.Unlock()
	if firstCloses != 1 {
		t.Fatalf("replaced transport close count = %d, want 1", firstCloses)
	}
	registry.CloseAll()
}

func TestRunInteractiveCommandPersistsSubmittedCommandHistory(t *testing.T) {
	profile := connectionProfile{ID: "local", Name: "Local", Type: connectionTypeLocal}
	workspace := newSessionWorkspace()
	if _, ok := workspace.Open(profile); !ok {
		t.Fatal("open runtime tab")
	}
	state, ok := workspace.Active()
	if !ok {
		t.Fatal("active runtime tab missing")
	}
	registry := newInteractiveRuntimeRegistry()
	fake := newFakeInteractiveSession()
	if err := registry.Start(context.Background(), state.ID, fake, terminal.TerminalSize{Columns: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { registry.CloseAll() })
	history := terminal.NewHistoryStore(filepath.Join(t.TempDir(), "history.jsonl"))
	app := &finalShellApp{sessions: workspace, interactive: registry, history: history}
	if !app.runInteractiveCommand("printf history-smoke") {
		t.Fatal("interactive command was not handled")
	}
	fake.mu.Lock()
	if len(fake.inputs) != 1 || string(fake.inputs[0]) != "printf history-smoke\r" {
		t.Fatalf("interactive input=%q", fake.inputs)
	}
	fake.mu.Unlock()
	entries, err := history.LoadForConnection("local", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Command != "printf history-smoke" || !entries[0].Interactive {
		t.Fatalf("interactive history=%#v", entries)
	}
}

func TestRunInteractiveSSHCommandUsesLongLivedRuntimeAndSSHHistory(t *testing.T) {
	profile := connectionProfile{ID: "remote", Name: "Remote", Type: connectionTypeSSH}
	workspace := newSessionWorkspace()
	if _, ok := workspace.Open(profile); !ok {
		t.Fatal("open SSH runtime tab")
	}
	state, _ := workspace.Active()
	registry := newInteractiveRuntimeRegistry()
	fake := newFakeInteractiveSession()
	if err := registry.Start(context.Background(), state.ID, fake, terminal.TerminalSize{Columns: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registry.CloseAll)
	history := terminal.NewHistoryStore(filepath.Join(t.TempDir(), "history.jsonl"))
	app := &finalShellApp{sessions: workspace, interactive: registry, history: history}
	if !app.runInteractiveCommand("printf remote-history") {
		t.Fatal("SSH interactive command was not handled")
	}
	fake.mu.Lock()
	if len(fake.inputs) != 1 || string(fake.inputs[0]) != "printf remote-history\r" {
		t.Fatalf("SSH interactive input=%q", fake.inputs)
	}
	fake.mu.Unlock()
	entries, err := history.LoadForConnection("remote", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ConnectionType != terminal.ConnectionTypeSSH || !entries[0].Interactive {
		t.Fatalf("SSH interactive history=%#v", entries)
	}
}

func TestDrainInteractiveOutputBatchesBurstsAndCompletesAfterFinalFlush(t *testing.T) {
	session := newFakeInteractiveSession()
	session.output <- []byte("\x1b[32m简体·")
	session.output <- []byte("繁體·Русский·한국어·e\u0301 🚀\x1b[0m\r\n")
	close(session.output)
	close(session.done)

	var scheduled []func()
	var events []string
	batcher := newGUIOutputBatcher(func(fn func()) {
		scheduled = append(scheduled, fn)
	}, func(text string) {
		events = append(events, "output:"+text)
	})
	drainInteractiveOutput(session, batcher, func(err error) {
		if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		}
		events = append(events, "exit")
	})

	if len(scheduled) != 1 {
		t.Fatalf("burst scheduled %d GUI callbacks, want 1", len(scheduled))
	}
	scheduled[0]()
	if len(events) != 2 || events[0] != "output:\x1b[32m简体·繁體·Русский·한국어·e\u0301 🚀\x1b[0m\r\n" || events[1] != "exit" {
		t.Fatalf("ordered GUI events = %#v", events)
	}
}
