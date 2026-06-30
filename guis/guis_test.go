package guis

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecuteLocalCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := executeCommand(ctx, connectionProfile{Type: connectionTypeLocal, Name: "local"}, "printf nbterminal-local-ok")
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if strings.TrimSpace(out) != "nbterminal-local-ok" {
		t.Fatalf("unexpected command output: %q", out)
	}
}

func TestSSHCommandRequiresAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executeCommand(ctx, connectionProfile{Type: connectionTypeSSH, Host: "127.0.0.1", Port: 22, Username: "nobody"}, "true")
	if err == nil {
		t.Fatal("expected error without password or private key")
	}
	if !strings.Contains(err.Error(), "no SSH auth method") {
		t.Fatalf("unexpected error: %v", err)
	}
}
