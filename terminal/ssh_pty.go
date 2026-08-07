package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

const sshPTYOutputBuffer = 64

type interactiveSSHDialer interface {
	Dial(context.Context, string, string, *ssh.ClientConfig) (interactiveSSHClient, error)
}

type interactiveSSHClient interface {
	NewSession() (interactiveSSHSession, error)
	Close() error
}

type interactiveSSHSession interface {
	RequestPty(string, int, int, ssh.TerminalModes) error
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	StderrPipe() (io.Reader, error)
	Shell() error
	WindowChange(int, int) error
	Signal(ssh.Signal) error
	Wait() error
	Close() error
}

type sshPTYSession struct {
	conn   Connection
	dialer interactiveSSHDialer

	mu      sync.Mutex
	started bool
	closed  bool
	client  interactiveSSHClient
	session interactiveSSHSession
	stdin   io.WriteCloser
	output  chan []byte
	done    chan struct{}
	waitErr error
}

// NewSSHPTYSession constructs a long-lived remote shell backed by an SSH PTY.
// The byte-oriented InteractiveSession contract is shared with local PTY and
// preserves VT sequences and split UTF-8 packets for the native terminal widget.
func NewSSHPTYSession(conn Connection) InteractiveSession {
	return newSSHPTYSessionWithDialer(conn, interactiveNetSSHDialer{})
}

func newSSHPTYSessionWithDialer(conn Connection, dialer interactiveSSHDialer) InteractiveSession {
	conn.Normalize()
	return &sshPTYSession{
		conn: conn, dialer: dialer,
		output: make(chan []byte, sshPTYOutputBuffer),
		done:   make(chan struct{}),
	}
}

func (s *sshPTYSession) Start(ctx context.Context, size TerminalSize) error {
	if ctx == nil {
		return errors.New("interactive session context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := size.validate(); err != nil {
		return err
	}
	if s.conn.Type != ConnectionTypeSSH {
		return fmt.Errorf("SSH PTY requires an SSH connection, got %q", s.conn.Type)
	}
	if err := s.conn.Validate(); err != nil {
		return err
	}
	cfg, err := sshClientConfig(s.conn)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("interactive session already started")
	}
	if s.closed {
		return errors.New("interactive session is closed")
	}
	client, err := s.dialer.Dial(ctx, "tcp", fmt.Sprintf("%s:%d", s.conn.Host, s.conn.Port), cfg)
	if err != nil {
		return err
	}
	cleanupClient := true
	defer func() {
		if cleanupClient {
			_ = client.Close()
		}
	}()
	remote, err := client.NewSession()
	if err != nil {
		return err
	}
	cleanupRemote := true
	defer func() {
		if cleanupRemote {
			_ = remote.Close()
		}
	}()
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 38400,
		ssh.TTY_OP_OSPEED: 38400,
	}
	if err := remote.RequestPty("xterm-256color", int(size.Rows), int(size.Columns), modes); err != nil {
		return fmt.Errorf("request SSH PTY: %w", err)
	}
	stdin, err := remote.StdinPipe()
	if err != nil {
		return fmt.Errorf("open SSH PTY stdin: %w", err)
	}
	stdout, err := remote.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open SSH PTY stdout: %w", err)
	}
	stderr, err := remote.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open SSH PTY stderr: %w", err)
	}
	if err := remote.Shell(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start SSH PTY shell: %w", err)
	}

	s.client, s.session, s.stdin, s.started = client, remote, stdin, true
	cleanupClient, cleanupRemote = false, false
	var readers sync.WaitGroup
	readers.Add(2)
	go s.readOutput(stdout, &readers)
	go s.readOutput(stderr, &readers)
	go s.waitForExit(remote, client, stdin, &readers)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	if s.conn.WorkingDir != "" {
		if _, err := io.WriteString(stdin, "cd "+shellQuote(s.conn.WorkingDir)+"\r"); err != nil {
			_ = s.Close()
			return fmt.Errorf("set SSH PTY working directory: %w", err)
		}
	}
	return nil
}

func (s *sshPTYSession) readOutput(reader io.Reader, readers *sync.WaitGroup) {
	defer readers.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			s.output <- append([]byte(nil), buf[:n]...)
		}
		if err != nil {
			return
		}
	}
}

func (s *sshPTYSession) waitForExit(remote interactiveSSHSession, client interactiveSSHClient, stdin io.WriteCloser, readers *sync.WaitGroup) {
	err := remote.Wait()
	readers.Wait()
	_ = stdin.Close()
	_ = client.Close()
	s.mu.Lock()
	s.waitErr = err
	s.mu.Unlock()
	close(s.output)
	close(s.done)
}

func (s *sshPTYSession) WriteInput(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stdin == nil || s.closed {
		return errors.New("interactive session is not running")
	}
	_, err := s.stdin.Write(data)
	return err
}

func (s *sshPTYSession) Resize(size TerminalSize) error {
	if err := size.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.session == nil || s.closed {
		return errors.New("interactive session is not running")
	}
	return s.session.WindowChange(int(size.Rows), int(size.Columns))
}

func (s *sshPTYSession) Interrupt() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.session == nil || s.closed {
		return errors.New("interactive session is not running")
	}
	return s.session.Signal(ssh.SIGINT)
}

func (s *sshPTYSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	stdin, remote, client := s.stdin, s.session, s.client
	s.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if remote != nil {
		_ = remote.Close()
	}
	if client != nil {
		_ = client.Close()
	}
	return nil
}

func (s *sshPTYSession) Wait() error {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return errors.New("interactive session was not started")
	}
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitErr
}

func (s *sshPTYSession) Output() <-chan []byte { return s.output }

type interactiveNetSSHDialer struct{}
type interactiveRealSSHClient struct{ *ssh.Client }
type interactiveRealSSHSession struct{ *ssh.Session }

func (interactiveNetSSHDialer) Dial(ctx context.Context, network, addr string, cfg *ssh.ClientConfig) (interactiveSSHClient, error) {
	client, err := (&netDialer{}).Dial(ctx, network, addr, cfg)
	if err != nil {
		return nil, err
	}
	realClient, ok := client.(realSSHClient)
	if !ok || realClient.Client == nil {
		_ = client.Close()
		return nil, errors.New("SSH dialer returned an incompatible client")
	}
	return interactiveRealSSHClient{realClient.Client}, nil
}

func (c interactiveRealSSHClient) NewSession() (interactiveSSHSession, error) {
	session, err := c.Client.NewSession()
	if err != nil {
		return nil, err
	}
	return interactiveRealSSHSession{session}, nil
}
