package terminal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type fakeInteractiveSSHDialer struct {
	client *fakeInteractiveSSHClient
	addr   string
	user   string
}

func (d *fakeInteractiveSSHDialer) Dial(_ context.Context, _, addr string, cfg *ssh.ClientConfig) (interactiveSSHClient, error) {
	d.addr, d.user = addr, cfg.User
	return d.client, nil
}

type fakeInteractiveSSHClient struct {
	session *fakeInteractiveSSHSession
	closed  bool
}

func (c *fakeInteractiveSSHClient) NewSession() (interactiveSSHSession, error) { return c.session, nil }
func (c *fakeInteractiveSSHClient) Close() error {
	c.closed = true
	return nil
}

type fakeInteractiveSSHSession struct {
	mu          sync.Mutex
	stdin       bytes.Buffer
	stdoutR     *io.PipeReader
	stdoutW     *io.PipeWriter
	stderrR     *io.PipeReader
	stderrW     *io.PipeWriter
	requested   TerminalSize
	term        string
	shell       bool
	windowSizes []TerminalSize
	signals     []ssh.Signal
	closed      chan struct{}
	closeOnce   sync.Once
	waitErr     error
}

func newFakeInteractiveSSHSession() *fakeInteractiveSSHSession {
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	return &fakeInteractiveSSHSession{
		stdoutR: stdoutR, stdoutW: stdoutW, stderrR: stderrR, stderrW: stderrW,
		closed: make(chan struct{}),
	}
}

func (s *fakeInteractiveSSHSession) RequestPty(term string, height, width int, _ ssh.TerminalModes) error {
	s.mu.Lock()
	s.term = term
	s.requested = TerminalSize{Columns: uint16(width), Rows: uint16(height)}
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSSHSession) StdinPipe() (io.WriteCloser, error) {
	return nopWriteCloser{Writer: &s.stdin}, nil
}
func (s *fakeInteractiveSSHSession) StdoutPipe() (io.Reader, error) { return s.stdoutR, nil }
func (s *fakeInteractiveSSHSession) StderrPipe() (io.Reader, error) { return s.stderrR, nil }
func (s *fakeInteractiveSSHSession) Shell() error {
	s.mu.Lock()
	s.shell = true
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSSHSession) WindowChange(height, width int) error {
	s.mu.Lock()
	s.windowSizes = append(s.windowSizes, TerminalSize{Columns: uint16(width), Rows: uint16(height)})
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSSHSession) Signal(signal ssh.Signal) error {
	s.mu.Lock()
	s.signals = append(s.signals, signal)
	s.mu.Unlock()
	return nil
}
func (s *fakeInteractiveSSHSession) Wait() error { <-s.closed; return s.waitErr }
func (s *fakeInteractiveSSHSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.stdoutW.Close()
		_ = s.stderrW.Close()
		close(s.closed)
	})
	return nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestSSHPTYSessionRoutesRawStreamInputResizeInterruptAndCleanup(t *testing.T) {
	remote := newFakeInteractiveSSHSession()
	client := &fakeInteractiveSSHClient{session: remote}
	dialer := &fakeInteractiveSSHDialer{client: client}
	conn := Connection{ID: "remote", Name: "Remote", Type: ConnectionTypeSSH, Host: "example.com", Port: 2200, Username: "tester", Password: "secret"}
	session := newSSHPTYSessionWithDialer(conn, dialer)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.Start(ctx, TerminalSize{Columns: 101, Rows: 37}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if dialer.addr != "example.com:2200" || dialer.user != "tester" {
		t.Fatalf("dial metadata addr=%q user=%q", dialer.addr, dialer.user)
	}
	if err := session.WriteInput([]byte("printf '简体·繁體·Русский 🚀'\r")); err != nil {
		t.Fatalf("WriteInput: %v", err)
	}
	if err := session.Resize(TerminalSize{Columns: 132, Rows: 42}); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if err := session.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	wantOutput := []byte("\x1b[32m简体·繁體·Русский 🚀\x1b[0m\r\n")
	go func() {
		_, _ = remote.stdoutW.Write(wantOutput[:11])
		_, _ = remote.stdoutW.Write(wantOutput[11:])
		_ = remote.Close()
	}()
	var got bytes.Buffer
	for chunk := range session.Output() {
		got.Write(chunk)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !bytes.Equal(got.Bytes(), wantOutput) {
		t.Fatalf("raw output changed: got=%q want=%q", got.Bytes(), wantOutput)
	}

	remote.mu.Lock()
	defer remote.mu.Unlock()
	if remote.term != "xterm-256color" || remote.requested != (TerminalSize{Columns: 101, Rows: 37}) || !remote.shell {
		t.Fatalf("PTY startup term=%q size=%+v shell=%v", remote.term, remote.requested, remote.shell)
	}
	if gotInput := remote.stdin.String(); gotInput != "printf '简体·繁體·Русский 🚀'\r" {
		t.Fatalf("stdin=%q", gotInput)
	}
	if len(remote.windowSizes) != 1 || remote.windowSizes[0] != (TerminalSize{Columns: 132, Rows: 42}) {
		t.Fatalf("window changes=%+v", remote.windowSizes)
	}
	if len(remote.signals) != 1 || remote.signals[0] != ssh.SIGINT {
		t.Fatalf("signals=%+v", remote.signals)
	}
	if !client.closed {
		t.Fatal("SSH client was not closed")
	}
}

func TestSSHPTYSessionRejectsWrongConnectionAndInvalidLifecycle(t *testing.T) {
	local := NewSSHPTYSession(DefaultLocalConnection())
	if err := local.Start(context.Background(), TerminalSize{Columns: 80, Rows: 24}); err == nil {
		t.Fatal("SSH PTY accepted a local connection")
	}
	conn := Connection{ID: "remote", Name: "Remote", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "tester", Password: "secret"}
	remote := newFakeInteractiveSSHSession()
	session := newSSHPTYSessionWithDialer(conn, &fakeInteractiveSSHDialer{client: &fakeInteractiveSSHClient{session: remote}})
	if err := session.WriteInput([]byte("lost")); err == nil {
		t.Fatal("WriteInput before Start should fail")
	}
	if err := session.Resize(TerminalSize{}); err == nil {
		t.Fatal("Resize accepted zero size")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := session.Start(ctx, TerminalSize{Columns: 80, Rows: 24}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Start error=%v", err)
	}
}
