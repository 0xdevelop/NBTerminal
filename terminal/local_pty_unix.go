//go:build cgo && !windows

package terminal

/*
#define _XOPEN_SOURCE 600
#include <errno.h>
#include <fcntl.h>
#include <stdlib.h>
#include <unistd.h>

static int nbterminal_openpt(char **slave_name) {
	int master = posix_openpt(O_RDWR | O_NOCTTY);
	if (master < 0) return -1;
	if (grantpt(master) < 0 || unlockpt(master) < 0) {
		int saved = errno;
		close(master);
		errno = saved;
		return -1;
	}
	char *name = ptsname(master);
	if (name == NULL) {
		int saved = errno;
		close(master);
		errno = saved;
		return -1;
	}
	*slave_name = name;
	return master;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const localPTYOutputBuffer = 64

type localPTYSession struct {
	conn Connection

	mu      sync.Mutex
	started bool
	closed  bool
	master  *os.File
	cmd     *exec.Cmd
	output  chan []byte
	done    chan struct{}
	waitErr error
}

func newLocalPTYSession(conn Connection) InteractiveSession {
	return &localPTYSession{conn: conn, output: make(chan []byte, localPTYOutputBuffer), done: make(chan struct{})}
}

func (s *localPTYSession) Start(ctx context.Context, size TerminalSize) error {
	if ctx == nil {
		return errors.New("interactive session context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := size.validate(); err != nil {
		return err
	}
	if s.conn.Type != ConnectionTypeLocal {
		return fmt.Errorf("local PTY requires a local connection, got %q", s.conn.Type)
	}
	if err := s.conn.Validate(); err != nil {
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
	master, slave, err := openPTY()
	if err != nil {
		return err
	}
	if err := setPTYSize(master, size); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return err
	}

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-i")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	if s.conn.WorkingDir != "" {
		cmd.Dir = s.conn.WorkingDir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return fmt.Errorf("start local PTY shell: %w", err)
	}
	_ = slave.Close()
	s.master, s.cmd, s.started = master, cmd, true

	readDone := make(chan struct{})
	go s.readOutput(master, readDone)
	go s.waitForExit(cmd, readDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return nil
}

func openPTY() (*os.File, *os.File, error) {
	var slaveName *C.char
	masterFD, err := C.nbterminal_openpt(&slaveName)
	if masterFD < 0 {
		return nil, nil, fmt.Errorf("open PTY master: %w", err)
	}
	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	name := C.GoString(slaveName)
	slave, openErr := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if openErr != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("open PTY slave: %w", openErr)
	}
	return master, slave, nil
}

func setPTYSize(master *os.File, size TerminalSize) error {
	if err := size.validate(); err != nil {
		return err
	}
	return unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: size.Columns, Row: size.Rows})
}

func (s *localPTYSession) readOutput(master *os.File, done chan<- struct{}) {
	defer close(done)
	defer close(s.output)
	buf := make([]byte, 32*1024)
	for {
		n, err := master.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.output <- chunk
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO) {
				return
			}
			return
		}
	}
}

func (s *localPTYSession) waitForExit(cmd *exec.Cmd, readDone <-chan struct{}) {
	err := cmd.Wait()
	<-readDone
	s.mu.Lock()
	s.waitErr = err
	if s.master != nil {
		_ = s.master.Close()
		s.master = nil
	}
	s.mu.Unlock()
	close(s.done)
}

func (s *localPTYSession) WriteInput(input []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.master == nil {
		return errors.New("interactive session is not running")
	}
	_, err := s.master.Write(input)
	return err
}

func (s *localPTYSession) Resize(size TerminalSize) error {
	if err := size.validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.master == nil {
		return errors.New("interactive session is not running")
	}
	return setPTYSize(s.master, size)
}

func (s *localPTYSession) Interrupt() error { return s.WriteInput([]byte{0x03}) }

func (s *localPTYSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cmd, master := s.cmd, s.master
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		if master != nil {
			return master.Close()
		}
		return nil
	}

	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
	select {
	case <-s.done:
		return nil
	case <-time.After(750 * time.Millisecond):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if master != nil {
			_ = master.Close()
		}
		return nil
	}
}

func (s *localPTYSession) Wait() error {
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

func (s *localPTYSession) Output() <-chan []byte { return s.output }
