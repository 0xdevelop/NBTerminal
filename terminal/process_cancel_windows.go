//go:build windows

package terminal

import (
	"context"
	"os/exec"
	"strconv"
	"sync"
)

func prepareCommandForCancellation(*exec.Cmd) {}

func watchCommandCancellation(ctx context.Context, cmd *exec.Cmd) func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
				_ = cmd.Process.Kill()
			}
		case <-done:
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}
