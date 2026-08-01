package persistence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileReplacesContentAndTightensPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	want := []byte("{\"name\":\"连接 🚀\"}\n")
	if err := AtomicWriteFile(path, want, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("mode = %o, want 600", gotMode)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestAtomicWriteFilePreservesExistingFileWhenReplacementCannotStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}

	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(filepath.Join(blocker, "config.json"), []byte("new"), 0o600); err == nil {
		t.Fatal("expected replacement to fail")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stable" {
		t.Fatalf("unrelated existing file changed: %q", got)
	}
}
