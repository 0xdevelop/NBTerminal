//go:build !windows

package terminal

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
)

func prepareCommandForCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func watchCommandCancellation(ctx context.Context, cmd *exec.Cmd) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
