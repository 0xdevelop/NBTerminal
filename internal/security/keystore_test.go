package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateMasterKeyPersistsWithPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	first, err := LoadOrCreateMasterKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateMasterKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second || len(first) != 64 {
		t.Fatalf("master key did not persist consistently")
	}
	for path, want := range map[string]os.FileMode{
		dir: 0o700, filepath.Join(dir, MasterKeyFileName): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode=%o want=%o", path, got, want)
		}
	}
	profileKey, err := PurposeKey(first, "profile-v1")
	if err != nil {
		t.Fatal(err)
	}
	historyKey, err := PurposeKey(first, "history-v1")
	if err != nil {
		t.Fatal(err)
	}
	if profileKey == historyKey || profileKey == first || historyKey == first {
		t.Fatal("purpose keys were not domain separated")
	}
}

func TestLoadOrCreateMasterKeyRejectsInvalidExistingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MasterKeyFileName)
	if err := os.WriteFile(path, []byte("invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateMasterKey(dir); err == nil {
		t.Fatal("expected invalid master key to be rejected")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("invalid key mode=%o want=600", got)
	}
}

func TestLoadOrCreateMasterKeyRejectsMissingKeyForExistingEncryptedData(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	databasePath := filepath.Join(root, "data", "nbterminal.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(databasePath, []byte("existing-encrypted-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateMasterKey(configDir, databasePath); err == nil {
		t.Fatal("missing key for existing encrypted database was silently replaced")
	}
	if _, err := os.Stat(filepath.Join(configDir, MasterKeyFileName)); !os.IsNotExist(err) {
		t.Fatalf("replacement master key should not be created, err=%v", err)
	}
}
