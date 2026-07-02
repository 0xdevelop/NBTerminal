package terminal

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryStoreAppendAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	store := NewHistoryStore(path)

	first := HistoryEntry{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "pwd", ExitCode: 0, Stdout: "/tmp\n"}
	second := HistoryEntry{Time: time.Now().UTC(), ConnectionID: "ssh", ConnectionName: "SSH", ConnectionType: ConnectionTypeSSH, Command: "false", ExitCode: 1, Stderr: "no\n"}
	if err := store.Append(first); err != nil {
		t.Fatalf("append first failed: %v", err)
	}
	if err := store.Append(second); err != nil {
		t.Fatalf("append second failed: %v", err)
	}

	entries, err := store.Load(0)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Command != "pwd" || entries[1].ConnectionType != ConnectionTypeSSH || entries[1].ExitCode != 1 {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	limited, err := store.Load(1)
	if err != nil {
		t.Fatalf("limited load failed: %v", err)
	}
	if len(limited) != 1 || limited[0].Command != "false" {
		t.Fatalf("expected most recent entry only, got %#v", limited)
	}
}

func TestHistoryStoreLoadForConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	store := NewHistoryStore(path)
	entries := []HistoryEntry{
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "pwd", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "ssh", ConnectionName: "SSH", ConnectionType: ConnectionTypeSSH, Command: "uname", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "whoami", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "date", ExitCode: 0},
	}
	for _, entry := range entries {
		if err := store.Append(entry); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	local, err := store.LoadForConnection("local", 2)
	if err != nil {
		t.Fatalf("LoadForConnection failed: %v", err)
	}
	if len(local) != 2 || local[0].Command != "whoami" || local[1].Command != "date" {
		t.Fatalf("expected two most recent local entries, got %#v", local)
	}
	allRecent, err := store.LoadForConnection("", 2)
	if err != nil {
		t.Fatalf("LoadForConnection all failed: %v", err)
	}
	if len(allRecent) != 2 || allRecent[0].Command != "whoami" || allRecent[1].Command != "date" {
		t.Fatalf("expected two most recent entries, got %#v", allRecent)
	}
}

func TestHistoryStoreLoadLongOutputRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	store := NewHistoryStore(path)
	long := strings.Repeat("x", 70_000)
	if err := store.Append(HistoryEntry{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "long", ExitCode: 0, Stdout: long}); err != nil {
		t.Fatalf("append long record failed: %v", err)
	}
	entries, err := store.Load(10)
	if err != nil {
		t.Fatalf("load long record failed: %v", err)
	}
	if len(entries) != 1 || len(entries[0].Stdout) != len(long) {
		t.Fatalf("long history record was not preserved: %#v", entries)
	}
}

func TestHistoryFromResultOmitsSecrets(t *testing.T) {
	started := time.Now().UTC()
	result := CommandResult{
		Connection: Connection{ID: "prod", Name: "Prod", Type: ConnectionTypeSSH, Host: "example.com", Username: "root", Password: "secret", PrivateKey: "private"},
		Command:    "uname -a",
		StartedAt:  started,
		FinishedAt: started.Add(150 * time.Millisecond),
		ExitCode:   0,
		Stdout:     "ok\n",
	}
	entry := HistoryFromResult(result)
	if entry.DurationMS != 150 || entry.ConnectionID != "prod" || entry.Command != "uname -a" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	rendered := strings.Join([]string{entry.ConnectionID, entry.ConnectionName, entry.Command, entry.Stdout, entry.Stderr}, " ")
	if strings.Contains(rendered, "secret") || strings.Contains(rendered, "private") {
		t.Fatalf("history entry leaked secret material: %#v", entry)
	}
}
