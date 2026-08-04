package guis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/george012/gtbox"
)

func TestQuickConnectionProjectionPrioritizesFavoritesThenRecent(t *testing.T) {
	rows := []connectionProfile{
		{ID: "old-favorite", Name: "Old favorite", Favorite: true, LastUsed: "2026-08-01T10:00:00Z"},
		{ID: "recent", Name: "Recent", LastUsed: "2026-08-04T10:00:00Z"},
		{ID: "new-favorite", Name: "New favorite", Favorite: true, LastUsed: "2026-08-03T10:00:00Z"},
		{ID: "never", Name: "Never"},
	}

	gotRows := quickConnectionProjection(rows, 3)
	got := make([]string, 0, len(gotRows))
	for _, row := range gotRows {
		got = append(got, row.ID)
	}
	want := []string{"new-favorite", "old-favorite", "recent"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quick projection = %#v, want %#v", got, want)
	}
	if rows[0].ID != "old-favorite" {
		t.Fatalf("projection mutated source order: %#v", rows)
	}
}

func TestQuickConnectionProjectionSearchesAllSavedConnections(t *testing.T) {
	rows := []connectionProfile{
		{ID: "favorite", Name: "Favorite", Favorite: true},
		{ID: "recent", Name: "Recent", LastUsed: "2026-08-04T10:00:00Z"},
		{ID: "hidden", Name: "Production database", Host: "db.internal"},
	}

	got := navigatorRows(rows, "database", 2)
	if len(got) != 1 || got[0].ID != "hidden" {
		t.Fatalf("search should include full saved set, got %#v", got)
	}
	got = navigatorRows(rows, "", 2)
	if len(got) != 2 || got[0].ID != "favorite" || got[1].ID != "recent" {
		t.Fatalf("empty search should use quick projection, got %#v", got)
	}
}

func TestToggleFavoritePreservesProfileAndChangesOnlyFavorite(t *testing.T) {
	profile := connectionProfile{ID: "prod", Name: "生产", Host: "example.com", PasswordEnc: "encrypted", LastUsed: "2026-08-04T10:00:00Z"}
	got := toggledFavorite(profile)
	if !got.Favorite || got.ID != profile.ID || got.Name != profile.Name || got.Host != profile.Host || got.PasswordEnc != profile.PasswordEnc || got.LastUsed != profile.LastUsed {
		t.Fatalf("favorite toggle corrupted profile: %#v", got)
	}
	if toggledFavorite(got).Favorite {
		t.Fatal("second toggle should clear favorite")
	}
}

func TestCloseAfterConnectPreferencePersistsAndRollsBackOnFailure(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	config.GlobalConfig = &config.FileConfig{Language: "en"}
	config.GlobalConfig.Normalize()
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(t.TempDir(), "config.json")

	manager := &connectionManagerWindow{}
	manager.persistCloseAfterConnect(true)
	buf, err := os.ReadFile(config.CurrentApp.AppConfigFilePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.FileConfig
	if err := json.Unmarshal(buf, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.CloseManagerAfterConnect || !config.GlobalConfig.CloseManagerAfterConnect {
		t.Fatal("close-after-connect preference was not durably saved")
	}

	config.GlobalConfig.CloseManagerAfterConnect = false
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")
	manager.persistCloseAfterConnect(true)
	if config.GlobalConfig.CloseManagerAfterConnect {
		t.Fatal("failed save did not roll preference back")
	}
}
