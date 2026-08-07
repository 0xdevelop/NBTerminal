package guis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/internal/database"
	"github.com/0xdevelop/NBTerminal/internal/persistence"
	"github.com/0xdevelop/NBTerminal/internal/security"
	"github.com/george012/gtbox"
)

func TestSQLiteConnectionStoreMigratesAndEncryptsWholeProfile(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{ActiveConnectionID: "ssh-profile"}

	dir := t.TempDir()
	legacyPath := filepath.Join(dir, connectionStoreFile)
	profile := connectionProfile{
		ID: "ssh-profile", Name: "Encrypted Profile", Group: "Private", Type: connectionTypeSSH,
		Host: "sensitive-host.invalid", Port: 2222, Username: "synthetic-user",
		PrivateKey: "synthetic-private-key-marker", WorkingDir: "/synthetic/private/path",
		Description: "synthetic-description-marker",
	}
	profile.SetPassword("synthetic-password-marker")
	legacyPayload, err := json.Marshal([]connectionProfile{profile})
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.AtomicWriteFile(legacyPath, legacyPayload, 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sqliteProfileTestKey(t)
	store := newSQLiteConnectionStore(dir, db, key)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	got := store.List()
	if len(got) != 1 || !reflect.DeepEqual(got[0], profile) || got[0].Password() != "synthetic-password-marker" {
		t.Fatalf("profile did not round-trip through encrypted SQLite store: %#v", got)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy connection file should be removed after committed migration, err=%v", err)
	}
	raw, err := os.ReadFile(db.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, plaintext := range []string{
		"ssh-profile",
		"Encrypted Profile", "sensitive-host.invalid", "synthetic-user",
		"synthetic-private-key-marker", "/synthetic/private/path",
		"synthetic-description-marker", "synthetic-password-marker",
	} {
		if bytes.Contains(raw, []byte(plaintext)) {
			t.Fatalf("SQLite file contains plaintext profile field marker %q", plaintext)
		}
	}
	if len(config.GlobalConfig.Connections) != 0 || config.GlobalConfig.ActiveConnectionID != "" {
		t.Fatalf("config should not retain connection payloads or identifiers: %#v", config.GlobalConfig)
	}
}

func TestSQLiteConnectionStoreSaveReloadAndDeleteAllWithoutReseeding(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sqliteProfileTestKey(t)
	store := newSQLiteConnectionStore(dir, db, key)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	profile := connectionProfile{ID: "local", Name: "Local", Group: "Local", Type: connectionTypeLocal, WorkingDir: "/tmp"}
	if err := store.SaveActive([]connectionProfile{profile}, "local"); err != nil {
		t.Fatal(err)
	}
	reloaded := newSQLiteConnectionStore(dir, db, key)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.List(); len(got) != 1 || !reflect.DeepEqual(got[0], profile) {
		t.Fatalf("reloaded profiles=%#v", got)
	}
	if err := reloaded.SaveActive(nil, ""); err != nil {
		t.Fatal(err)
	}
	emptyReload := newSQLiteConnectionStore(dir, db, key)
	if err := emptyReload.Load(); err != nil {
		t.Fatal(err)
	}
	if got := emptyReload.List(); len(got) != 0 {
		t.Fatalf("intentionally empty database was reseeded: %#v", got)
	}
}

func TestSQLiteConnectionMigrationMergesIntoNonEmptyDatabaseWithoutOverwriting(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sqliteProfileTestKey(t)
	existing := connectionProfile{ID: "existing", Name: "SQLite Newer", Type: connectionTypeLocal}
	rows, err := encryptedConnectionRows([]connectionProfile{existing}, key)
	if err != nil {
		t.Fatal(err)
	}
	active, err := encryptedProfileStorageID(existing.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveConnections(context.Background(), rows, active); err != nil {
		t.Fatal(err)
	}
	legacy := []connectionProfile{
		{ID: "existing", Name: "Legacy Stale", Type: connectionTypeLocal},
		{ID: "legacy-only", Name: "Legacy Only", Type: connectionTypeSSH, Host: "legacy.invalid", Port: 22},
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(dir, connectionStoreFile)
	if err := persistence.AtomicWriteFile(legacyPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store := newSQLiteConnectionStore(dir, db, key)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	got := store.List()
	byID := make(map[string]connectionProfile, len(got))
	for _, profile := range got {
		byID[profile.ID] = profile
	}
	if len(got) != 2 || byID["existing"].Name != "SQLite Newer" || byID["legacy-only"].Name != "Legacy Only" {
		t.Fatalf("merged profiles=%#v", got)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("completed legacy migration was not cleaned up: %v", err)
	}
}

func TestSQLiteConnectionSaveKeepsCommittedDatabaseWhenConfigCleanupFails(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.GlobalConfig = &config.FileConfig{ActiveConnectionID: "legacy-active"}
	config.CurrentApp = nil

	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sqliteProfileTestKey(t)
	store := newSQLiteConnectionStore(dir, db, key)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")
	profile := connectionProfile{ID: "committed", Name: "Committed", Type: connectionTypeLocal}
	if err := store.SaveActive([]connectionProfile{profile}, profile.ID); err == nil {
		t.Fatal("expected deprecated config cleanup failure")
	}
	if got := store.List(); len(got) != 1 || got[0].ID != profile.ID {
		t.Fatalf("committed in-memory profile was rolled back: %#v", got)
	}
	config.CurrentApp = nil
	reloaded := newSQLiteConnectionStore(dir, db, key)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.List(); len(got) != 1 || got[0].ID != profile.ID {
		t.Fatalf("committed SQLite profile was rolled back: %#v", got)
	}
}

func TestSQLiteConnectionStoreRejectsWrongEncryptionKey(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.GlobalConfig = &config.FileConfig{}
	config.CurrentApp = nil
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := sqliteProfileTestKey(t)
	store := newSQLiteConnectionStore(dir, db, key)
	if err := store.SaveActive([]connectionProfile{{ID: "secret", Name: "Secret", Type: connectionTypeLocal}}, "secret"); err != nil {
		t.Fatal(err)
	}
	wrongKey := sqliteProfileTestKey(t)
	if err := newSQLiteConnectionStore(dir, db, wrongKey).Load(); err == nil {
		t.Fatal("wrong database encryption key was accepted")
	}
}

func sqliteProfileTestKey(t *testing.T) string {
	t.Helper()
	master, err := security.LoadOrCreateMasterKey(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := security.PurposeKey(master, "sqlite-profile-v1")
	if err != nil {
		t.Fatal(err)
	}
	return key
}
