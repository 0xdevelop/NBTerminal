package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestConnectionNormalizeAndValidateLocal(t *testing.T) {
	conn := Connection{}
	conn.Normalize()
	if conn.Type != ConnectionTypeLocal {
		t.Fatalf("expected local type, got %q", conn.Type)
	}
	if conn.ID == "" || conn.Name == "" {
		t.Fatalf("expected default id and name, got %#v", conn)
	}
	if err := conn.Validate(); err != nil {
		t.Fatalf("local connection should validate: %v", err)
	}
}

func TestConnectionValidateSSH(t *testing.T) {
	conn := Connection{ID: "prod", Name: "Prod", Type: ConnectionTypeSSH, Host: "example.com", Username: "root", Password: "secret"}
	conn.Normalize()
	if conn.Port != 22 {
		t.Fatalf("expected default ssh port 22, got %d", conn.Port)
	}
	if err := conn.Validate(); err != nil {
		t.Fatalf("ssh connection should validate: %v", err)
	}

	bad := Connection{ID: "bad", Name: "Bad", Type: ConnectionTypeSSH, Host: "example.com", Username: "root"}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected missing auth validation error")
	}
}

func TestLocalExecutorRunCapturesOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := DefaultLocalConnection()
	result, err := NewExecutor().Run(ctx, conn, "printf 'hello\\n'; printf 'warn\\n' >&2")
	if err != nil {
		t.Fatalf("local command failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Fatalf("stdout missing hello: %q", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "warn") {
		t.Fatalf("stderr missing warn: %q", result.Stderr)
	}
	if len(result.Events) < 3 {
		t.Fatalf("expected stdout, stderr and status events, got %#v", result.Events)
	}
}

func TestLocalExecutorRunReportsExitCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := NewExecutor().Run(ctx, DefaultLocalConnection(), "exit 7")
	if err == nil {
		t.Fatal("expected non-zero command to return error")
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected exit 7, got %d", result.ExitCode)
	}
}

func TestLocalExecutorRunCapturesLongLine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := NewExecutor().Run(ctx, DefaultLocalConnection(), "printf '%70000s\\n' x")
	if err != nil {
		t.Fatalf("local long-line command failed: %v", err)
	}
	if len(result.Stdout) < 70000 {
		t.Fatalf("expected long stdout line to be preserved, got %d bytes", len(result.Stdout))
	}
	if len(result.Events) < 2 || len(result.Events[0].Line) < 70000 {
		t.Fatalf("expected long line event, got %#v", result.Events)
	}
}

type fakeSSHDialer struct {
	client SSHClient
	err    error
	addr   string
	user   string
}

func (d *fakeSSHDialer) Dial(_ context.Context, _, addr string, cfg *ssh.ClientConfig) (SSHClient, error) {
	d.addr = addr
	d.user = cfg.User
	if d.err != nil {
		return nil, d.err
	}
	return d.client, nil
}

type fakeSSHClient struct {
	session SSHSession
	err     error
	closed  bool
}

func (c *fakeSSHClient) NewSession() (SSHSession, error) {
	return c.session, c.err
}
func (c *fakeSSHClient) Close() error { c.closed = true; return nil }

type fakeSSHSession struct {
	stdout io.Writer
	stderr io.Writer
	err    error
	cmd    string
	closed bool
}

func (s *fakeSSHSession) SetOutput(stdout, stderr io.Writer) { s.stdout, s.stderr = stdout, stderr }
func (s *fakeSSHSession) Run(command string) error {
	s.cmd = command
	_, _ = io.WriteString(s.stdout, "remote-ok\n")
	_, _ = io.WriteString(s.stderr, "remote-warn\n")
	return s.err
}
func (s *fakeSSHSession) Close() error { s.closed = true; return nil }

func TestSSHExecutorUsesInjectableDialer(t *testing.T) {
	session := &fakeSSHSession{}
	client := &fakeSSHClient{session: session}
	dialer := &fakeSSHDialer{client: client}
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 2200, Username: "root", Password: "secret"}
	result, err := (SSHExecutor{Dialer: dialer}).Run(context.Background(), conn, "uname -a")
	if err != nil {
		t.Fatalf("ssh run failed: %v", err)
	}
	if dialer.addr != "example.com:2200" || dialer.user != "root" {
		t.Fatalf("unexpected dial target/user: %s %s", dialer.addr, dialer.user)
	}
	if session.cmd != "uname -a" || !session.closed || !client.closed {
		t.Fatalf("session/client lifecycle mismatch: cmd=%q sessionClosed=%v clientClosed=%v", session.cmd, session.closed, client.closed)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "remote-ok") || !strings.Contains(result.Stderr, "remote-warn") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Events) < 3 {
		t.Fatalf("expected stdout/stderr/status events, got %#v", result.Events)
	}
}

func TestSSHExecutorReportsDialError(t *testing.T) {
	want := errors.New("dial blocked")
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "root", Password: "secret"}
	result, err := (SSHExecutor{Dialer: &fakeSSHDialer{err: want}}).Run(context.Background(), conn, "true")
	if !errors.Is(err, want) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("expected pending exit code on dial error, got %d", result.ExitCode)
	}
}
