package terminal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestNormalizeConnectionsDeduplicatesAndSlugsIDs(t *testing.T) {
	conns := NormalizeConnections([]Connection{
		{ID: "prod", Name: "Prod", Type: ConnectionTypeLocal},
		{ID: "prod", Name: "Prod copy", Type: ConnectionTypeLocal},
		{ID: "prod", Name: "Prod third", Type: ConnectionTypeLocal},
		{Name: "QA Box #1", Type: ConnectionTypeLocal},
	})
	ids := make(map[string]bool, len(conns))
	for _, conn := range conns {
		if ids[conn.ID] {
			t.Fatalf("duplicate normalized id %q in %#v", conn.ID, conns)
		}
		ids[conn.ID] = true
	}
	for _, want := range []string{"prod", "prod-2", "prod-3", "qa-box-1"} {
		if !ids[want] {
			t.Fatalf("missing normalized id %q in %#v", want, conns)
		}
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

func TestLocalExecutorRunHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewExecutor().Run(ctx, DefaultLocalConnection(), "sleep 30")
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("local command did not stop after cancellation")
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

type blockingSSHSession struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (s *blockingSSHSession) SetOutput(_, _ io.Writer) {}
func (s *blockingSSHSession) Run(string) error {
	close(s.started)
	<-s.closed
	return errors.New("session closed")
}
func (s *blockingSSHSession) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

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

func TestSSHExecutorRunHonorsCancellation(t *testing.T) {
	session := &blockingSSHSession{started: make(chan struct{}), closed: make(chan struct{})}
	client := &fakeSSHClient{session: session}
	dialer := &fakeSSHDialer{client: client}
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "root", Password: "secret"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (SSHExecutor{Dialer: dialer}).Run(ctx, conn, "sleep 30")
		done <- err
	}()

	select {
	case <-session.started:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("SSH command did not start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
		if !client.closed {
			t.Fatal("SSH client was not closed after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSH command did not stop after cancellation")
	}
}

func TestSSHExecutorUsesWorkingDir(t *testing.T) {
	session := &fakeSSHSession{}
	client := &fakeSSHClient{session: session}
	dialer := &fakeSSHDialer{client: client}
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "root", Password: "secret", WorkingDir: "/srv/my app's/current"}
	_, err := (SSHExecutor{Dialer: dialer}).Run(context.Background(), conn, "pwd && printf ok")
	if err != nil {
		t.Fatalf("ssh run failed: %v", err)
	}
	want := "cd '/srv/my app'\\''s/current' && pwd && printf ok"
	if session.cmd != want {
		t.Fatalf("expected working-dir command %q, got %q", want, session.cmd)
	}
}

func TestSSHExecutorAcceptsPrivateKeyPath(t *testing.T) {
	keyPath := writeTestPrivateKey(t)
	session := &fakeSSHSession{}
	client := &fakeSSHClient{session: session}
	dialer := &fakeSSHDialer{client: client}
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "root", PrivateKey: keyPath}
	result, err := (SSHExecutor{Dialer: dialer}).Run(context.Background(), conn, "true")
	if err != nil {
		t.Fatalf("ssh run with private key path failed: %v", err)
	}
	if result.ExitCode != 0 || dialer.addr != "example.com:22" {
		t.Fatalf("unexpected key-path SSH result: result=%#v addr=%q", result, dialer.addr)
	}
}

func TestPrivateKeyMaterialReadsPathAndPreservesPEM(t *testing.T) {
	keyPath := writeTestPrivateKey(t)
	fromPath, err := privateKeyMaterial(keyPath)
	if err != nil {
		t.Fatalf("privateKeyMaterial(path) failed: %v", err)
	}
	if !strings.Contains(string(fromPath), "BEGIN RSA PRIVATE KEY") {
		t.Fatalf("expected PEM from path, got %q", string(fromPath))
	}
	raw := string(fromPath)
	fromRaw, err := privateKeyMaterial(raw)
	if err != nil {
		t.Fatalf("privateKeyMaterial(raw) failed: %v", err)
	}
	if string(fromRaw) != raw {
		t.Fatalf("expected raw PEM to be preserved")
	}
}

func TestSSHExecutorReportsMissingPrivateKeyPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing_id_rsa")
	conn := Connection{ID: "dev", Name: "Dev", Type: ConnectionTypeSSH, Host: "example.com", Port: 22, Username: "root", PrivateKey: missing}
	result, err := (SSHExecutor{Dialer: &fakeSSHDialer{client: &fakeSSHClient{session: &fakeSSHSession{}}}}).Run(context.Background(), conn, "true")
	if err == nil || !strings.Contains(err.Error(), "read private key") {
		t.Fatalf("expected missing private key path error, got result=%#v err=%v", result, err)
	}
}

func writeTestPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	path := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

func TestSSHExecutorValidatesDirectCalls(t *testing.T) {
	result, err := (SSHExecutor{Dialer: &fakeSSHDialer{}}).Run(context.Background(), Connection{Type: ConnectionTypeSSH, Host: "example.com", Username: "root"}, "true")
	if err == nil || !strings.Contains(err.Error(), "ssh password or private key is required") {
		t.Fatalf("expected auth validation error, got result=%#v err=%v", result, err)
	}
	if result.ExitCode != -1 || result.Connection.Port != 22 {
		t.Fatalf("expected normalized pending result, got %#v", result)
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
