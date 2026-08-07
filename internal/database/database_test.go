package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseEncryptedRowsRoundTripAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	connections := []ConnectionRow{
		{ID: "one", PayloadEnc: "cipher-one", Position: 0},
		{ID: "two", PayloadEnc: "cipher-two", Position: 1},
	}
	if err := db.SaveConnections(ctx, connections, "two"); err != nil {
		t.Fatal(err)
	}
	got, active, err := db.LoadConnections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PayloadEnc != "cipher-one" || active != "two" {
		t.Fatalf("connections=%#v active=%q", got, active)
	}
	if err := db.AppendHistory(ctx, HistoryRow{RecordedAtNS: 1, ConnectionID: "one", PayloadEnc: "history-one"}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendHistory(ctx, HistoryRow{RecordedAtNS: 2, ConnectionID: "two", PayloadEnc: "history-two"}); err != nil {
		t.Fatal(err)
	}
	history, err := db.LoadHistory(ctx, "two", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].PayloadEnc != "history-two" {
		t.Fatalf("history=%#v", history)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, db.Path(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode=%o want=%o", path, got, want)
		}
	}
}

func TestLegacyMigrationsAreTransactionalAndIdempotent(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	migrated, err := db.MigrateConnections(ctx, []ConnectionRow{{ID: "legacy", PayloadEnc: "cipher", Position: 0}}, "legacy")
	if err != nil || !migrated {
		t.Fatalf("first connection migration migrated=%t err=%v", migrated, err)
	}
	migrated, err = db.MigrateConnections(ctx, []ConnectionRow{{ID: "additional", PayloadEnc: "other", Position: 1}}, "additional")
	if err != nil || !migrated {
		t.Fatalf("non-empty connection migration migrated=%t err=%v", migrated, err)
	}
	migrated, err = db.MigrateConnections(ctx, []ConnectionRow{{ID: "additional", PayloadEnc: "changed-legacy", Position: 1}}, "additional")
	if err != nil || migrated {
		t.Fatalf("repeated connection migration migrated=%t err=%v", migrated, err)
	}
	rows, active, err := db.LoadConnections(ctx)
	if err != nil || len(rows) != 2 || rows[0].ID != "legacy" || rows[1].ID != "additional" || rows[1].PayloadEnc != "other" || active != "legacy" {
		t.Fatalf("rows=%#v active=%q err=%v", rows, active, err)
	}

	migrated, err = db.MigrateHistory(ctx, []HistoryRow{{RecordedAtNS: 1, ConnectionID: "legacy", PayloadEnc: "history-cipher", LegacySource: "source-a", LegacyOrdinal: 1}})
	if err != nil || !migrated {
		t.Fatalf("first history migration migrated=%t err=%v", migrated, err)
	}
	migrated, err = db.MigrateHistory(ctx, []HistoryRow{{RecordedAtNS: 2, ConnectionID: "legacy", PayloadEnc: "second-source", LegacySource: "source-b", LegacyOrdinal: 1}})
	if err != nil || !migrated {
		t.Fatalf("non-empty history migration migrated=%t err=%v", migrated, err)
	}
	migrated, err = db.MigrateHistory(ctx, []HistoryRow{{RecordedAtNS: 3, ConnectionID: "legacy", PayloadEnc: "duplicate", LegacySource: "source-b", LegacyOrdinal: 1}})
	if err != nil || migrated {
		t.Fatalf("repeated history migration migrated=%t err=%v", migrated, err)
	}
	history, err := db.LoadHistory(ctx, "", 0)
	if err != nil || len(history) != 2 || history[0].PayloadEnc != "history-cipher" || history[1].PayloadEnc != "second-source" {
		t.Fatalf("history=%#v err=%v", history, err)
	}
}

func TestSaveConnectionsRollsBackInvalidEncryptedRow(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.SaveConnections(ctx, []ConnectionRow{{ID: "kept", PayloadEnc: "cipher", Position: 0}}, "kept"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveConnections(ctx, []ConnectionRow{{ID: "broken", PayloadEnc: "", Position: 0}}, "broken"); err == nil {
		t.Fatal("expected invalid encrypted row to fail")
	}
	rows, active, err := db.LoadConnections(ctx)
	if err != nil || len(rows) != 1 || rows[0].ID != "kept" || active != "kept" {
		t.Fatalf("rollback rows=%#v active=%q err=%v", rows, active, err)
	}
}

func TestDatabaseOpenRejectsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.db")
	if err := os.WriteFile(target, []byte("not-the-application-database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, FileName)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("sqlite symlink path was accepted")
	}
}

func TestDatabaseOpenUsesIndependentGTBoxGORMHandles(t *testing.T) {
	for index := 0; index < 4; index++ {
		index := index
		t.Run(fmt.Sprintf("db-%d", index), func(t *testing.T) {
			t.Parallel()
			db, err := Open(filepath.Join(t.TempDir(), "data"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			row := ConnectionRow{ID: fmt.Sprintf("opaque-%d", index), PayloadEnc: "ciphertext-only"}
			if err := db.SaveConnections(context.Background(), []ConnectionRow{row}, row.ID); err != nil {
				t.Fatal(err)
			}
			rows, activeID, err := db.LoadConnections(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0] != row || activeID != row.ID {
				t.Fatalf("independent database round trip: rows=%#v active=%q", rows, activeID)
			}
		})
	}
}
