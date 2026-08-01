package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestHistoryStoreAppendTightensLegacyPermissionsAndPreservesUnicode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewHistoryStore(path)
	want := HistoryEntry{
		Time: time.Now().UTC(), ConnectionID: "本地-🚀", ConnectionName: "中文 · 日本語 · 한국어 · 🚀 · é",
		ConnectionType: ConnectionTypeLocal, Command: "printf '历史恢复 🚀 é'", Stdout: "历史恢复 🚀 é\n",
	}
	if err := store.Append(want); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("history mode = %o, want 600", got)
	}
	entries, err := store.Load(0)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(entries) != 1 || entries[0].ConnectionName != want.ConnectionName || entries[0].Stdout != want.Stdout {
		t.Fatalf("Unicode history did not round-trip: %#v", entries)
	}
}

func TestHistoryStoreLoadRecoversCompleteRecordsBeforeTruncatedTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	complete := `{"connection_id":"本地-🚀","connection_name":"中文","command":"printf 历史","stdout":"历史 🚀\n"}` + "\n"
	if err := os.WriteFile(path, []byte(complete+`{"connection_id":"broken"`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := NewHistoryStore(path).Load(0)
	if err != nil {
		t.Fatalf("Load should recover complete records before a crash-truncated tail: %v", err)
	}
	if len(entries) != 1 || entries[0].ConnectionID != "本地-🚀" || entries[0].Stdout != "历史 🚀\n" {
		t.Fatalf("unexpected recovered history: %#v", entries)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("loaded history mode = %o, want 600", got)
	}
}

func TestHistoryStoreLoadRejectsMalformedCompleteRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHistoryStore(path).Load(0); err == nil {
		t.Fatal("expected a malformed complete history record to be rejected")
	}
}

func TestHistoryStoreAppendRepairsCrashTailAndSeparatesLegacyRecord(t *testing.T) {
	t.Run("truncated tail", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.jsonl")
		first := `{"connection_id":"first","command":"完整"}` + "\n"
		if err := os.WriteFile(path, []byte(first+`{"connection_id":"broken"`), 0o600); err != nil {
			t.Fatal(err)
		}
		store := NewHistoryStore(path)
		if err := store.Append(HistoryEntry{ConnectionID: "second", Command: "恢复 🚀"}); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		entries, err := store.Load(0)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if len(entries) != 2 || entries[0].ConnectionID != "first" || entries[1].ConnectionID != "second" {
			t.Fatalf("unexpected repaired entries: %#v", entries)
		}
	})

	t.Run("valid record without newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.jsonl")
		if err := os.WriteFile(path, []byte(`{"connection_id":"first","command":"旧记录"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		store := NewHistoryStore(path)
		if err := store.Append(HistoryEntry{ConnectionID: "second", Command: "新记录"}); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
		entries, err := store.Load(0)
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		if len(entries) != 2 || entries[0].Command != "旧记录" || entries[1].Command != "新记录" {
			t.Fatalf("legacy record was not separated: %#v", entries)
		}
	})
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

func TestHistoryStoreLastCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history", "commands.jsonl")
	store := NewHistoryStore(path)
	entries := []HistoryEntry{
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "pwd", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "ssh", ConnectionName: "SSH", ConnectionType: ConnectionTypeSSH, Command: "uname -a", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "whoami", ExitCode: 0},
		{Time: time.Now().UTC(), ConnectionID: "local", ConnectionName: "Local", ConnectionType: ConnectionTypeLocal, Command: "", ExitCode: 0},
	}
	for _, entry := range entries {
		if err := store.Append(entry); err != nil {
			t.Fatalf("append failed: %v", err)
		}
	}

	cmd, ok, err := store.LastCommand("local")
	if err != nil {
		t.Fatalf("LastCommand(local) failed: %v", err)
	}
	if !ok || cmd != "whoami" {
		t.Fatalf("expected latest non-empty local command, got cmd=%q ok=%v", cmd, ok)
	}
	cmd, ok, err = store.LastCommand("")
	if err != nil {
		t.Fatalf("LastCommand(all) failed: %v", err)
	}
	if !ok || cmd != "whoami" {
		t.Fatalf("expected latest non-empty global command, got cmd=%q ok=%v", cmd, ok)
	}
	cmd, ok, err = store.LastCommand("missing")
	if err != nil || ok || cmd != "" {
		t.Fatalf("expected missing command to be absent, got cmd=%q ok=%v err=%v", cmd, ok, err)
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

func TestHistoryFromResultTruncatesLargeOutput(t *testing.T) {
	large := strings.Repeat("世", historyOutputMaxBytes) // multi-byte: validates UTF-8-safe cut
	entry := HistoryFromResult(CommandResult{
		Connection: DefaultLocalConnection(),
		Command:    "large-output",
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
		ExitCode:   0,
		Stdout:     large,
	})
	if len(entry.Stdout) >= len(large) {
		t.Fatalf("expected stdout to be truncated, original=%d got=%d", len(large), len(entry.Stdout))
	}
	if !strings.Contains(entry.Stdout, "NBTerminal history output truncated") {
		t.Fatalf("expected truncation marker, got suffix %q", entry.Stdout[len(entry.Stdout)-80:])
	}
	if !utf8.ValidString(entry.Stdout) {
		t.Fatalf("truncated history output must remain valid UTF-8")
	}
}

func TestHistoryFromResultNormalizesInvalidUTF8(t *testing.T) {
	invalid := string([]byte{'x', 0xff})
	entry := HistoryFromResult(CommandResult{
		Connection: Connection{ID: invalid, Name: "连接 🚀", Type: ConnectionTypeLocal},
		Command:    invalid,
		Stdout:     "中文 " + invalid,
		Stderr:     "ошибка",
	})
	for name, value := range map[string]string{
		"connection id":   entry.ConnectionID,
		"connection name": entry.ConnectionName,
		"command":         entry.Command,
		"stdout":          entry.Stdout,
		"stderr":          entry.Stderr,
	} {
		if !utf8.ValidString(value) {
			t.Fatalf("%s is not valid UTF-8: %q", name, value)
		}
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
