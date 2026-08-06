//go:build cgo && !windows

package terminal

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLocalPTYSessionPersistsStateResizesAndInterruptsForegroundJob(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/bash")
	conn := DefaultLocalConnection()
	conn.WorkingDir = t.TempDir()
	session := NewLocalPTYSession(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := session.Start(ctx, TerminalSize{Columns: 80, Rows: 24}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	var output bytes.Buffer
	waitFor := func(marker string) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for !strings.Contains(output.String(), marker) {
			select {
			case chunk, ok := <-session.Output():
				if !ok {
					t.Fatalf("PTY output closed before %q; output=%q", marker, output.String())
				}
				output.Write(chunk)
			case <-deadline.C:
				t.Fatalf("timed out waiting for %q; output=%q", marker, output.String())
			}
		}
	}

	workingDir := conn.WorkingDir
	first := fmt.Sprintf("export NBTERMINAL_PTY_STATE='简体·繁體·Русский'; cd %q; printf '\\nNBT_READY\\n'\n", workingDir)
	if err := session.WriteInput([]byte(first)); err != nil {
		t.Fatalf("first WriteInput failed: %v", err)
	}
	waitFor("NBT_READY")
	if err := session.WriteInput([]byte("printf 'NBT_STATE=%s\\n' \"$NBTERMINAL_PTY_STATE\"; printf 'NBT_PWD=%s\\n' \"$PWD\"\n")); err != nil {
		t.Fatalf("state WriteInput failed: %v", err)
	}
	waitFor("NBT_STATE=简体·繁體·Русский")
	waitFor("NBT_PWD=" + workingDir)

	if err := session.Resize(TerminalSize{Columns: 101, Rows: 37}); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if err := session.WriteInput([]byte("stty size | sed 's/^/NBT_SIZE=/'\n")); err != nil {
		t.Fatalf("resize probe WriteInput failed: %v", err)
	}
	waitFor("NBT_SIZE=37 101")

	if err := session.WriteInput([]byte("sleep 30\n")); err != nil {
		t.Fatalf("sleep WriteInput failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := session.Interrupt(); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}
	if err := session.WriteInput([]byte("printf 'NBT_SHELL_ALIVE\\n'\n")); err != nil {
		t.Fatalf("post-interrupt WriteInput failed: %v", err)
	}
	waitFor("NBT_SHELL_ALIVE")

	if err := session.WriteInput([]byte("exit\n")); err != nil {
		t.Fatalf("exit WriteInput failed: %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait after clean shell exit failed: %v", err)
	}
}

func TestLocalPTYSessionRejectsInvalidLifecycleAndSize(t *testing.T) {
	session := NewLocalPTYSession(DefaultLocalConnection())
	if err := session.WriteInput([]byte("pwd\n")); err == nil {
		t.Fatal("WriteInput before Start should fail")
	}
	if err := session.Resize(TerminalSize{}); err == nil {
		t.Fatal("zero terminal size should fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := session.Start(ctx, TerminalSize{Columns: 80, Rows: 24}); err == nil {
		t.Fatal("Start with canceled context should fail")
	}
}
