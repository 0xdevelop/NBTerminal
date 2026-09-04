package terminal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xdevelop/NBTerminal/internal/database"
	"github.com/0xdevelop/NBTerminal/internal/security"
)

func TestSQLiteHistoryStoreMigratesEncryptsAndDecryptsOnUse(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "terminal-history.jsonl")
	legacy := NewHistoryStore(legacyPath)
	legacyEntry := HistoryEntry{
		Time: time.Now().UTC(), ConnectionID: "profile-one", ConnectionName: "Synthetic Host",
		ConnectionType: ConnectionTypeSSH, Command: "synthetic-command-marker",
		Stdout: "synthetic-output-marker", Stderr: "synthetic-error-marker", ExitCode: 7,
	}
	if err := legacy.Append(legacyEntry); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	master, err := security.LoadOrCreateMasterKey(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	historyKey, err := security.PurposeKey(master, "sqlite-history-v1")
	if err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteHistoryStore(db, legacyPath, historyKey)
	migrated, err := store.MigrateLegacy()
	if err != nil || !migrated {
		t.Fatalf("migrated=%t err=%v", migrated, err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy history should be removed after committed migration, err=%v", err)
	}
	entries, err := store.LoadForConnection("profile-one", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Command != legacyEntry.Command || entries[0].Stdout != legacyEntry.Stdout || entries[0].ExitCode != 7 {
		t.Fatalf("migrated history=%#v", entries)
	}
	if err := store.Append(HistoryEntry{Time: time.Now().UTC(), ConnectionID: "profile-two", Command: "synthetic-second-command"}); err != nil {
		t.Fatal(err)
	}
	recent, err := store.Load(1)
	if err != nil || len(recent) != 1 || recent[0].Command != "synthetic-second-command" {
		t.Fatalf("recent history=%#v err=%v", recent, err)
	}
	raw, err := os.ReadFile(db.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, plaintext := range []string{
		"profile-one", "profile-two",
		"Synthetic Host", "synthetic-command-marker", "synthetic-output-marker",
		"synthetic-error-marker", "synthetic-second-command",
	} {
		if bytes.Contains(raw, []byte(plaintext)) {
			t.Fatalf("SQLite file contains plaintext history field marker %q", plaintext)
		}
	}
}

func TestSQLiteHistoryMigrationRetriesCleanupAfterCommittedMarker(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "terminal-history.jsonl")
	legacyEntry := HistoryEntry{Time: time.Now().UTC(), ConnectionID: "legacy", Command: "legacy-command"}
	legacy := NewHistoryStore(legacyPath)
	if err := legacy.Append(legacyEntry); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	master, err := security.LoadOrCreateMasterKey(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := security.PurposeKey(master, "sqlite-history-v1")
	if err != nil {
		t.Fatal(err)
	}
	source, err := historyLegacySource(legacyPath, key)
	if err != nil {
		t.Fatal(err)
	}
	row, err := encryptedHistoryRow(legacyEntry, key)
	if err != nil {
		t.Fatal(err)
	}
	row.LegacySource, row.LegacyOrdinal = source, 1
	if applied, err := db.MigrateHistory(context.Background(), []database.HistoryRow{row}); err != nil || !applied {
		t.Fatalf("precommit migration applied=%t err=%v", applied, err)
	}
	store := NewSQLiteHistoryStore(db, legacyPath, key)
	if applied, err := store.MigrateLegacy(); err != nil || applied {
		t.Fatalf("cleanup retry applied=%t err=%v", applied, err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("committed legacy file was not removed on retry: %v", err)
	}
	entries, err := store.Load(0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("history after cleanup retry=%#v err=%v", entries, err)
	}
}

func TestSQLiteHistoryFiltersByDecryptedConnectionAssociation(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	master, err := security.LoadOrCreateMasterKey(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := security.PurposeKey(master, "sqlite-history-v1")
	if err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteHistoryStore(db, filepath.Join(dir, "terminal-history.jsonl"), key)
	for _, entry := range []HistoryEntry{
		{Time: time.Unix(1, 0), ConnectionID: "one", Command: "one-old"},
		{Time: time.Unix(2, 0), ConnectionID: "two", Command: "two"},
		{Time: time.Unix(3, 0), ConnectionID: "one", Command: "one-new"},
	} {
		if err := store.Append(entry); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.LoadForConnection("one", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Command != "one-new" {
		t.Fatalf("filtered history=%#v", entries)
	}
}

func TestSQLiteHistoryRejectsMismatchedEncryptedConnectionIndex(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	master, err := security.LoadOrCreateMasterKey(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := security.PurposeKey(master, "sqlite-history-v1")
	if err != nil {
		t.Fatal(err)
	}
	row, err := encryptedHistoryRow(HistoryEntry{Time: time.Now().UTC(), ConnectionID: "expected", Command: "test"}, key)
	if err != nil {
		t.Fatal(err)
	}
	row.ConnectionID, err = encryptedHistoryConnectionID("wrong", key)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AppendHistory(context.Background(), row); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteHistoryStore(db, filepath.Join(dir, "missing.jsonl"), key)
	if _, err := store.Load(0); err == nil {
		t.Fatal("mismatched history connection index was accepted")
	}
}
