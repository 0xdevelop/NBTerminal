package guis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/george012/gtbox"
)

func TestConnectionDeleteConfirmationIsLocalizedAndDefaultsToCancel(t *testing.T) {
	previous := locales.CurrentLanguage()
	t.Cleanup(func() { locales.ResetLocaleLanguage(previous.LanguageTag()) })

	profile := connectionProfile{ID: "prod", Name: "Production 数据库"}
	for _, language := range locales.SupportedLanguages() {
		locales.ResetLocaleLanguage(language.LanguageTag())
		prompt := connectionDeletePromptFor(profile)
		if strings.TrimSpace(prompt.Title) == "" || !strings.Contains(prompt.Message, profile.Name) ||
			strings.TrimSpace(prompt.Cancel) == "" || strings.TrimSpace(prompt.Delete) == "" {
			t.Fatalf("%s delete prompt is incomplete: %#v", language.LanguageTag(), prompt)
		}

		var gotOptions []string
		cancelled := confirmConnectionDelete(profile, func(_, _ string, options ...string) int {
			gotOptions = append([]string(nil), options...)
			return 1
		})
		if cancelled || len(gotOptions) != 2 || gotOptions[0] != prompt.Delete || gotOptions[1] != prompt.Cancel {
			t.Fatalf("%s confirmation did not default to cancel: confirmed=%v options=%#v", language.LanguageTag(), cancelled, gotOptions)
		}
		if !confirmConnectionDelete(profile, func(_, _ string, _ ...string) int { return 0 }) {
			t.Fatalf("%s explicit delete action was not accepted", language.LanguageTag())
		}
		if confirmConnectionDelete(profile, nil) {
			t.Fatalf("%s unavailable dialog must fail closed", language.LanguageTag())
		}
	}
}

func TestConnectionDeletionRevalidatesStableSelection(t *testing.T) {
	rows := []connectionProfile{{ID: "alpha"}, {ID: "beta"}}
	if !canDeleteSelectedProfile(rows, 0, "alpha") {
		t.Fatal("unchanged selected profile should remain deletable")
	}
	if canDeleteSelectedProfile(rows, 1, "alpha") {
		t.Fatal("selection changed while confirmation was open; deletion must fail closed")
	}
	if canDeleteSelectedProfile(rows, -1, "alpha") || canDeleteSelectedProfile(rows, 0, "") {
		t.Fatal("missing selection or target ID must fail closed")
	}
}

func TestConnectionManagerGroupOptionsIncludeHierarchicalParents(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod-2", Group: " Infrastructure / Production / Web "},
		{ID: "local", Group: ""},
		{ID: "dev", Group: "Infrastructure/Development"},
		{ID: "prod-1", Group: "Infrastructure/Production/Database"},
	}

	got := connectionManagerGroupOptions(rows)
	want := []connectionGroupOption{
		{},
		{Path: "Infrastructure", Label: "Infrastructure"},
		{Path: "Infrastructure/Development", Label: "  Development"},
		{Path: "Infrastructure/Production", Label: "  Production"},
		{Path: "Infrastructure/Production/Database", Label: "    Database"},
		{Path: "Infrastructure/Production/Web", Label: "    Web"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group options = %#v, want %#v", got, want)
	}
}

func TestConnectionManagerRowsCombineGroupAndSearchWithoutMutatingSource(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod-db", Name: "Database", Group: "Production", Host: "db.internal"},
		{ID: "prod-web", Name: "Web", Group: "Production", Host: "web.internal"},
		{ID: "dev-db", Name: "Database", Group: "Development", Host: "db.dev"},
	}

	got := connectionManagerRows(rows, "Production", "database")
	if len(got) != 1 || got[0].ID != "prod-db" {
		t.Fatalf("combined group/search filter = %#v", got)
	}
	if rows[0].ID != "prod-db" || len(rows) != 3 {
		t.Fatalf("manager filter mutated source rows: %#v", rows)
	}
	if got := connectionManagerRows(rows, "", "database"); len(got) != 2 {
		t.Fatalf("all-groups search returned %d rows, want 2", len(got))
	}
}

func TestConnectionManagerSearchKeyboardMovesSelectionAndEscapeClearsQuery(t *testing.T) {
	rows := []connectionProfile{
		{ID: "alpha", Name: "Alpha", Type: connectionTypeLocal},
		{ID: "beta-one", Name: "Beta One", Type: connectionTypeLocal},
		{ID: "beta-two", Name: "Beta Two", Type: connectionTypeLocal},
	}
	manager := &connectionManagerWindow{
		owner:  &finalShellApp{allRows: rows},
		search: uikit.NewInput(0, 0, 240, nativeControls.InputHeight, ""),
		idx:    0,
	}
	manager.search.SetText("beta")
	manager.applySearch()
	if len(manager.rows) != 2 || manager.idx != 0 {
		t.Fatalf("search setup = rows %d idx %d, want 2/0", len(manager.rows), manager.idx)
	}
	if !manager.handleSearchKey(fltk_bridge.DOWN) || manager.idx != 1 {
		t.Fatalf("Down did not move to second manager match: idx=%d", manager.idx)
	}
	if !manager.handleSearchKey(fltk_bridge.UP) || manager.idx != 0 {
		t.Fatalf("Up did not move to first manager match: idx=%d", manager.idx)
	}
	if !manager.handleSearchKey(fltk_bridge.ESCAPE) || manager.search.Text() != "" || len(manager.rows) != len(rows) {
		t.Fatalf("Escape did not clear manager query: query=%q rows=%d", manager.search.Text(), len(manager.rows))
	}
	if manager.handleSearchKey(fltk_bridge.ESCAPE) || manager.handleSearchKey('x') {
		t.Fatal("empty manager Escape or unrelated key must remain available to native input handling")
	}
}

func TestConnectionManagerParentGroupIncludesDescendantsOnly(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod", Group: "Infrastructure/Production"},
		{ID: "prod-db", Group: "Infrastructure / Production / Database"},
		{ID: "production-like", Group: "Infrastructure/Production-Lab"},
		{ID: "dev", Group: "Infrastructure/Development"},
	}

	got := connectionManagerRows(rows, "Infrastructure/Production", "")
	if len(got) != 2 || got[0].ID != "prod" || got[1].ID != "prod-db" {
		t.Fatalf("parent group filter = %#v, want exact group plus descendants", got)
	}
}

func TestConnectionEditorNormalizesHierarchicalGroupPath(t *testing.T) {
	draft := connectionEditorDraft{Name: "DB", Group: " Infrastructure // Production / Database ", Type: "local"}
	profile, err := draft.Profile(connectionProfile{ID: "db"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Group != "Infrastructure/Production/Database" {
		t.Fatalf("normalized group = %q", profile.Group)
	}
}

func TestCompactNavigatorUsesLeafGroupName(t *testing.T) {
	if got := compactConnectionGroup("Infrastructure/Production/Database"); got != "Database" {
		t.Fatalf("compact group = %q, want leaf name", got)
	}
	if got := compactConnectionGroup(" Local "); got != "Local" {
		t.Fatalf("flat compact group = %q", got)
	}
}

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

func TestQuickConnectionCellsExposeFavoriteWithoutLeakingSecrets(t *testing.T) {
	profile := connectionProfile{
		ID:          "prod-db",
		Name:        "生产数据库",
		Group:       "Infrastructure/Production/Database",
		Type:        connectionTypeSSH,
		Host:        "db.internal",
		Port:        2222,
		Username:    "operator",
		PasswordEnc: "encrypted-secret",
		PrivateKey:  "private-key-secret",
		Favorite:    true,
		LastUsed:    "2026-08-04T10:00:00Z",
	}

	want := []string{"★", "Database", "生产数据库", "ssh", "db.internal:2222", formatLastUsedCompact(profile.LastUsed)}
	for column, expected := range want {
		if got := quickConnectionCellText(profile, column); got != expected {
			t.Fatalf("column %d = %q, want %q", column, got, expected)
		}
	}
	if got := quickConnectionCellText(profile, len(want)); got != "" {
		t.Fatalf("out-of-range column = %q, want empty", got)
	}
	for _, secret := range []string{profile.Username, profile.PasswordEnc, profile.PrivateKey} {
		for column := range want {
			if strings.Contains(quickConnectionCellText(profile, column), secret) {
				t.Fatalf("column %d exposed a secret-bearing field", column)
			}
		}
	}
	profile.Favorite = false
	if got := quickConnectionCellText(profile, 0); got != "" {
		t.Fatalf("non-favorite marker = %q, want empty", got)
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

func TestRefreshNavigatorKeepsMainWindowAsCompactProjectionAfterEditorSave(t *testing.T) {
	rows := make([]connectionProfile, 0, quickConnectionLimit+3)
	for index := 0; index < quickConnectionLimit+3; index++ {
		rows = append(rows, connectionProfile{
			ID:       fmt.Sprintf("profile-%02d", index),
			Name:     fmt.Sprintf("Profile %02d", index),
			LastUsed: fmt.Sprintf("2026-08-%02dT10:00:00Z", index+1),
		})
	}
	app := &finalShellApp{allRows: rows, idx: -1}

	app.refreshNavigator("profile-00")

	if len(app.rows) != quickConnectionLimit {
		t.Fatalf("main navigator expanded to %d rows after save, want compact limit %d", len(app.rows), quickConnectionLimit)
	}
	if app.idx < 0 || app.idx >= len(app.rows) {
		t.Fatalf("navigator did not retain a valid fallback selection: idx=%d rows=%#v", app.idx, app.rows)
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
