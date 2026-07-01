package guis

import (
	"context"
	"os"
	"path/filepath"
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
	if !strings.Contains(err.Error(), "ssh password or private key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteLocalCommandUsesWorkingDir(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "nbterminal-marker.txt")
	if err := os.WriteFile(marker, []byte("from-workdir"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := executeCommand(ctx, connectionProfile{Type: connectionTypeLocal, Name: "local", WorkingDir: dir}, "pwd && cat nbterminal-marker.txt")
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if !strings.Contains(out, dir) || !strings.Contains(out, "from-workdir") {
		t.Fatalf("working directory was not used, output: %q", out)
	}
}

func TestProfileToConnectionLoadsPrivateKeyPath(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN TEST KEY-----\nabc\n-----END TEST KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := profileToConnection(connectionProfile{ID: "dev", Name: "Dev", Type: connectionTypeSSH, Host: "example.com", Port: 2200, Username: "me", PrivateKey: keyPath, WorkingDir: "/srv/app"})
	if err != nil {
		t.Fatalf("profileToConnection failed: %v", err)
	}
	if conn.PrivateKey == keyPath || !strings.Contains(conn.PrivateKey, "BEGIN TEST KEY") {
		t.Fatalf("expected private key content to be loaded, got %q", conn.PrivateKey)
	}
	if conn.Port != 2200 || conn.Username != "me" || conn.WorkingDir != "/srv/app" {
		t.Fatalf("unexpected mapped connection: %#v", conn)
	}
}
