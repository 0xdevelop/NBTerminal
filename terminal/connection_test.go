package terminal

import (
	"context"
	"strings"
	"testing"
	"time"
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
