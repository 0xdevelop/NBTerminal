package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Stream string

const (
	StreamStdout Stream = "stdout"
	StreamStderr Stream = "stderr"
	StreamStatus Stream = "status"
)

type Event struct {
	Time         time.Time `json:"time"`
	ConnectionID string    `json:"connection_id"`
	Stream       Stream    `json:"stream"`
	Line         string    `json:"line"`
	ExitCode     int       `json:"exit_code,omitempty"`
}

type CommandResult struct {
	Connection Connection `json:"connection"`
	Command    string     `json:"command"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt time.Time  `json:"finished_at"`
	ExitCode   int        `json:"exit_code"`
	Stdout     string     `json:"stdout"`
	Stderr     string     `json:"stderr"`
	Events     []Event    `json:"events"`
}

type Executor interface {
	Run(ctx context.Context, conn Connection, command string) (CommandResult, error)
}

type MultiExecutor struct {
	Local Executor
	SSH   Executor
}

func NewExecutor() *MultiExecutor {
	return &MultiExecutor{Local: LocalExecutor{}, SSH: SSHExecutor{Dialer: &netDialer{}}}
}

func (m *MultiExecutor) Run(ctx context.Context, conn Connection, command string) (CommandResult, error) {
	conn.Normalize()
	if err := conn.Validate(); err != nil {
		return CommandResult{Connection: conn, Command: command, ExitCode: -1}, err
	}
	if strings.TrimSpace(command) == "" {
		return CommandResult{Connection: conn, Command: command, ExitCode: -1}, errors.New("command is required")
	}
	switch conn.Type {
	case ConnectionTypeLocal:
		return m.Local.Run(ctx, conn, command)
	case ConnectionTypeSSH:
		return m.SSH.Run(ctx, conn, command)
	default:
		return CommandResult{Connection: conn, Command: command, ExitCode: -1}, fmt.Errorf("unsupported connection type %q", conn.Type)
	}
}

type LocalExecutor struct{}

func (LocalExecutor) Run(ctx context.Context, conn Connection, command string) (CommandResult, error) {
	return runProcess(ctx, conn, command)
}

func runProcess(ctx context.Context, conn Connection, command string) (CommandResult, error) {
	started := time.Now()
	result := CommandResult{Connection: conn, Command: command, StartedAt: started, ExitCode: -1}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if conn.WorkingDir != "" {
		cmd.Dir = conn.WorkingDir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, err
	}
	if err := cmd.Start(); err != nil {
		return result, err
	}

	type scanOut struct {
		stream Stream
		text   string
	}
	ch := make(chan scanOut, 32)
	done := make(chan struct{}, 2)
	scan := func(stream Stream, r io.Reader) {
		defer func() { done <- struct{}{} }()
		s := newLineScanner(r)
		for s.Scan() {
			ch <- scanOut{stream: stream, text: s.Text()}
		}
	}
	go scan(StreamStdout, stdout)
	go scan(StreamStderr, stderr)

	var outBuf, errBuf bytes.Buffer
	go func() {
		<-done
		<-done
		close(ch)
	}()
	for item := range ch {
		event := Event{Time: time.Now(), ConnectionID: conn.ID, Stream: item.stream, Line: item.text}
		result.Events = append(result.Events, event)
		if item.stream == StreamStdout {
			outBuf.WriteString(item.text)
			outBuf.WriteByte('\n')
		} else {
			errBuf.WriteString(item.text)
			errBuf.WriteByte('\n')
		}
	}

	err = cmd.Wait()
	result.FinishedAt = time.Now()
	result.Stdout = outBuf.String()
	result.Stderr = errBuf.String()
	result.ExitCode = cmd.ProcessState.ExitCode()
	result.Events = append(result.Events, Event{Time: result.FinishedAt, ConnectionID: conn.ID, Stream: StreamStatus, Line: fmt.Sprintf("exit code %d", result.ExitCode), ExitCode: result.ExitCode})
	return result, err
}

type SSHDialer interface {
	Dial(ctx context.Context, network, addr string, config *ssh.ClientConfig) (SSHClient, error)
}

type SSHClient interface {
	NewSession() (SSHSession, error)
	Close() error
}

type SSHSession interface {
	SetOutput(stdout, stderr io.Writer)
	Run(command string) error
	Close() error
}

type SSHExecutor struct{ Dialer SSHDialer }

func (e SSHExecutor) Run(ctx context.Context, conn Connection, command string) (CommandResult, error) {
	started := time.Now()
	result := CommandResult{Connection: conn, Command: command, StartedAt: started, ExitCode: -1}
	cfg, err := sshClientConfig(conn)
	if err != nil {
		return result, err
	}
	dialer := e.Dialer
	if dialer == nil {
		dialer = &netDialer{}
	}
	addr := fmt.Sprintf("%s:%d", conn.Host, conn.Port)
	client, err := dialer.Dial(ctx, "tcp", addr, cfg)
	if err != nil {
		return result, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return result, err
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.SetOutput(&stdout, &stderr)
	err = session.Run(command)
	result.FinishedAt = time.Now()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if exitErr, ok := err.(*ssh.ExitError); ok {
		result.ExitCode = exitErr.ExitStatus()
	} else if err == nil {
		result.ExitCode = 0
	}
	appendLines := func(stream Stream, text string) {
		s := newLineScanner(strings.NewReader(text))
		for s.Scan() {
			result.Events = append(result.Events, Event{Time: result.FinishedAt, ConnectionID: conn.ID, Stream: stream, Line: s.Text()})
		}
	}
	appendLines(StreamStdout, result.Stdout)
	appendLines(StreamStderr, result.Stderr)
	result.Events = append(result.Events, Event{Time: result.FinishedAt, ConnectionID: conn.ID, Stream: StreamStatus, Line: fmt.Sprintf("exit code %d", result.ExitCode), ExitCode: result.ExitCode})
	return result, err
}

func sshClientConfig(conn Connection) (*ssh.ClientConfig, error) {
	auth := make([]ssh.AuthMethod, 0, 2)
	if conn.Password != "" {
		auth = append(auth, ssh.Password(conn.Password))
	}
	if conn.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(conn.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	return &ssh.ClientConfig{User: conn.Username, Auth: auth, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 15 * time.Second}, nil
}

type netDialer struct{}

type realSSHClient struct{ *ssh.Client }

type realSSHSession struct{ *ssh.Session }

func (s realSSHSession) SetOutput(stdout, stderr io.Writer) {
	s.Stdout = stdout
	s.Stderr = stderr
}

func (c realSSHClient) NewSession() (SSHSession, error) {
	s, err := c.Client.NewSession()
	if err != nil {
		return nil, err
	}
	return realSSHSession{s}, nil
}

func (*netDialer) Dial(ctx context.Context, network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
	type res struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan res, 1)
	go func() {
		client, err := ssh.Dial(network, addr, config)
		ch <- res{client: client, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, r.err
		}
		return realSSHClient{r.client}, nil
	}
}
