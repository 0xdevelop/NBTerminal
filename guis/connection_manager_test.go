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
	"github.com/0xdevelop/fltk2go/uikit/checkbox"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
	"github.com/george012/gtbox"
)

func TestConnectionManagerEmptyMessageDistinguishesOnboardingAndFilters(t *testing.T) {
	if got := connectionManagerEmptyMessage(0, 0, false); got != "No saved connections yet. Create one to get started." {
		t.Fatalf("empty store message = %q", got)
	}
	if got := connectionManagerEmptyMessage(3, 0, true); got != "No matching connections. Adjust or reset filters." {
		t.Fatalf("filtered empty message = %q", got)
	}
	if got := connectionManagerEmptyMessage(3, 2, true); got != "" {
		t.Fatalf("non-empty result message = %q", got)
	}
}

func TestQuickLauncherEmptyMessageExplainsProjection(t *testing.T) {
	if got := quickLauncherEmptyMessage(0, 0, false); got != "No saved connections yet. Create one in Connection Manager." {
		t.Fatalf("empty store message = %q", got)
	}
	if got := quickLauncherEmptyMessage(4, 0, false); got != "No favorite or recent connections. Open Connection Manager to connect." {
		t.Fatalf("empty projection message = %q", got)
	}
	if got := quickLauncherEmptyMessage(4, 0, true); got != "No matching connections. Try another search." {
		t.Fatalf("empty search message = %q", got)
	}
	if got := quickLauncherEmptyMessage(4, 1, true); got != "" {
		t.Fatalf("non-empty quick result message = %q", got)
	}
}

func TestConnectionManagerRowContextMenuReflectsFavoriteAndRoutesCommands(t *testing.T) {
	invocations := map[string]int{}
	movedTo := ""
	items := connectionContextMenuItems(connectionProfile{Type: connectionTypeSSH, Group: "Production"}, []connectionGroupOption{
		{},
		{Path: "Development", Label: "Development"},
		{Path: "Production", Label: "Production"},
	}, connectionContextMenuActions{
		connect:     func() { invocations["connect"]++ },
		copyAddress: func() { invocations["copy_address"]++ },
		copyCommand: func() { invocations["copy_command"]++ },
		edit:        func() { invocations["edit"]++ },
		duplicate:   func() { invocations["duplicate"]++ },
		test:        func() { invocations["test"]++ },
		favorite:    func() { invocations["favorite"]++ },
		moveToGroup: func(group string) { movedTo = group },
		delete:      func() { invocations["delete"]++ },
	})
	want := []string{"Connect", "Copy Address", "Copy SSH Command", "Edit…", "Duplicate…", "Test Connection", "Add to Favorites", "Move to Group", "Delete…"}
	if len(items) != len(want) {
		t.Fatalf("context menu item count = %d, want %d", len(items), len(want))
	}
	for index, title := range want {
		if items[index].Title != title {
			t.Fatalf("context menu item %d = %#v", index, items[index])
		}
		if title == "Move to Group" {
			continue
		}
		if items[index].Callback == nil {
			t.Fatalf("context menu item %d has no callback: %#v", index, items[index])
		}
		items[index].Callback()
	}
	for _, action := range []string{"connect", "copy_address", "copy_command", "edit", "duplicate", "test", "favorite", "delete"} {
		if invocations[action] != 1 {
			t.Fatalf("%s callback count = %d, want 1", action, invocations[action])
		}
	}
	move := items[7]
	if len(move.Children) != 3 || move.Children[0].Title != "Ungrouped" || move.Children[1].Title != "Development" || move.Children[2].Title != "Production" {
		t.Fatalf("move submenu = %#v", move.Children)
	}
	if move.Children[2].Flags == 0 {
		t.Fatalf("current group is not disabled: %#v", move.Children[2])
	}
	move.Children[1].Callback()
	if movedTo != "Development" {
		t.Fatalf("move callback destination = %q, want Development", movedTo)
	}
	items = connectionContextMenuItems(connectionProfile{Favorite: true}, nil, connectionContextMenuActions{})
	if len(items) != 7 || items[4].Title != "Remove from Favorites" {
		t.Fatalf("local favorite menu = %#v", items)
	}
}

func TestMoveConnectionProfileGroupPreservesSensitiveFields(t *testing.T) {
	profile := connectionProfile{
		ID: "prod", Name: "Production", Group: "Old/Group", Type: connectionTypeSSH,
		PasswordEnc: "gtenc-password-marker", PrivateKey: "gtenc-key-marker",
	}
	moved, changed := moveConnectionProfileGroup(profile, " New / Nested ")
	if !changed || moved.Group != "New/Nested" {
		t.Fatalf("moved profile = %#v changed=%t", moved, changed)
	}
	if moved.PasswordEnc != profile.PasswordEnc || moved.PrivateKey != profile.PrivateKey || moved.ID != profile.ID {
		t.Fatalf("move changed sensitive or stable fields: %#v", moved)
	}
	if _, changed := moveConnectionProfileGroup(moved, "New/Nested"); changed {
		t.Fatal("same-group move should be a no-op")
	}
	ungrouped, changed := moveConnectionProfileGroup(moved, "")
	if !changed || ungrouped.Group != removedGroupFallback {
		t.Fatalf("ungrouped move = %#v changed=%t", ungrouped, changed)
	}
}

func TestQuickLauncherMoveToGroupPersistsOnlyGroupChange(t *testing.T) {
	store := newConnectionStore(t.TempDir())
	profile := connectionProfile{
		ID: "prod", Name: "Production", Group: "Old/Group", Type: connectionTypeSSH,
		Host: "prod.internal", Username: "deploy", PasswordEnc: "gtenc-password-marker", PrivateKey: "gtenc-key-marker",
	}
	if err := store.SaveActive([]connectionProfile{profile}, profile.ID); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	app := &finalShellApp{store: store, allRows: []connectionProfile{profile}, rows: []connectionProfile{profile}, idx: 0}
	app.moveSelectedProfileToGroup("Production/Database")

	reloaded := newConnectionStore(filepath.Dir(store.path))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload store: %v", err)
	}
	rows := reloaded.List()
	if len(rows) != 1 || rows[0].Group != "Production/Database" {
		t.Fatalf("persisted moved rows = %#v", rows)
	}
	got := rows[0]
	if got.ID != profile.ID || got.Host != profile.Host || got.Username != profile.Username || got.PasswordEnc != profile.PasswordEnc || got.PrivateKey != profile.PrivateKey {
		t.Fatalf("move changed non-group profile data: %#v", got)
	}
}

func TestConnectionClipboardAddressIsExplicitAndSecretFree(t *testing.T) {
	tests := []struct {
		name    string
		profile connectionProfile
		want    string
	}{
		{name: "username and default port", profile: connectionProfile{Type: connectionTypeSSH, Username: "deploy", Host: "db.internal"}, want: "deploy@db.internal:22"},
		{name: "ipv6 and custom port", profile: connectionProfile{Type: connectionTypeSSH, Host: "2001:db8::10", Port: 2222}, want: "[2001:db8::10]:2222"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.profile.PasswordEnc = "gtenc-secret-marker"
			test.profile.PrivateKey = "/private/key-marker"
			got, ok := connectionClipboardAddress(test.profile)
			if !ok || got != test.want {
				t.Fatalf("clipboard address = %q/%t, want %q/true", got, ok, test.want)
			}
			if strings.Contains(got, "secret-marker") || strings.Contains(got, "key-marker") {
				t.Fatalf("clipboard address exposed secret-bearing data: %q", got)
			}
		})
	}
	for _, profile := range []connectionProfile{
		{Type: connectionTypeLocal, WorkingDir: "/srv/private"},
		{Type: connectionTypeSSH, Host: "  "},
		{Type: connectionTypeSSH, Host: "db.internal", Port: -1},
		{Type: connectionTypeSSH, Host: "db.internal", Port: 65536},
	} {
		if got, ok := connectionClipboardAddress(profile); ok || got != "" {
			t.Fatalf("unsupported clipboard address = %q/%t", got, ok)
		}
	}
}

func TestConnectionClipboardSSHCommandIsPasteReadyAndSecretFree(t *testing.T) {
	tests := []struct {
		name    string
		profile connectionProfile
		want    string
	}{
		{name: "default port", profile: connectionProfile{Type: connectionTypeSSH, Username: "deploy", Host: "db.internal"}, want: "ssh -p 22 -- deploy@db.internal"},
		{name: "custom port and ipv6", profile: connectionProfile{Type: connectionTypeSSH, Host: "2001:db8::10", Port: 2222}, want: "ssh -p 2222 -- '2001:db8::10'"},
		{name: "shell metacharacters are quoted", profile: connectionProfile{Type: connectionTypeSSH, Username: "release bot", Host: "build host"}, want: "ssh -p 22 -- 'release bot@build host'"},
		{name: "single quote is escaped", profile: connectionProfile{Type: connectionTypeSSH, Username: "o'reilly", Host: "host"}, want: "ssh -p 22 -- 'o'\"'\"'reilly@host'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.profile.PasswordEnc = "gtenc-secret-marker"
			test.profile.PrivateKey = "/private/key-marker"
			got, ok := connectionClipboardSSHCommand(test.profile)
			if !ok || got != test.want {
				t.Fatalf("clipboard command = %q/%t, want %q/true", got, ok, test.want)
			}
			if strings.Contains(got, "secret-marker") || strings.Contains(got, "key-marker") {
				t.Fatalf("clipboard command exposed secret-bearing data: %q", got)
			}
		})
	}
	for _, profile := range []connectionProfile{
		{Type: connectionTypeLocal},
		{Type: connectionTypeSSH, Host: "  "},
		{Type: connectionTypeSSH, Host: "db.internal", Port: -1},
		{Type: connectionTypeSSH, Host: "db.internal", Port: 65536},
	} {
		if got, ok := connectionClipboardSSHCommand(profile); ok || got != "" {
			t.Fatalf("unsupported clipboard command = %q/%t", got, ok)
		}
	}
}

func TestConnectionManagerContextActionResolvesStableProfileAfterRowsChange(t *testing.T) {
	manager := &connectionManagerWindow{rows: []connectionProfile{{ID: "alpha"}, {ID: "beta"}}, idx: 0}
	if !manager.selectContextProfile("beta") || manager.idx != 1 {
		t.Fatalf("initial stable selection = %d, want 1", manager.idx)
	}
	manager.rows = []connectionProfile{{ID: "beta"}, {ID: "alpha"}}
	manager.idx = 1
	if !manager.selectContextProfile("beta") || manager.idx != 0 {
		t.Fatalf("reordered stable selection = %d, want 0", manager.idx)
	}
	if manager.selectContextProfile("missing") || manager.idx != 0 {
		t.Fatal("missing stable profile changed selection")
	}
}

func TestQuickLauncherContextActionResolvesStableProfileAfterRowsChange(t *testing.T) {
	app := &finalShellApp{rows: []connectionProfile{{ID: "alpha"}, {ID: "beta"}}, idx: 0}
	if !app.selectQuickContextProfile("beta") || app.idx != 1 {
		t.Fatalf("initial stable quick selection = %d, want 1", app.idx)
	}
	app.rows = []connectionProfile{{ID: "beta"}, {ID: "alpha"}}
	app.idx = 1
	if !app.selectQuickContextProfile("beta") || app.idx != 0 {
		t.Fatalf("reordered stable quick selection = %d, want 0", app.idx)
	}
	if app.selectQuickContextProfile("missing") || app.idx != 0 {
		t.Fatal("missing quick-launch profile changed selection")
	}
}

func TestQuickLauncherFavoriteActionPersistsOnlyTargetProfile(t *testing.T) {
	store := newConnectionStore(t.TempDir())
	rows := []connectionProfile{
		{ID: "alpha", Name: "Alpha", Group: "Local", Type: connectionTypeLocal},
		{ID: "beta", Name: "Beta", Group: "Local", Type: connectionTypeLocal},
	}
	if err := store.SaveActive(rows, "alpha"); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	app := &finalShellApp{store: store, allRows: append([]connectionProfile(nil), rows...), rows: append([]connectionProfile(nil), rows...), idx: 1}
	app.toggleSelectedProfileFavorite()
	allBeta := indexProfileByID(app.allRows, "beta")
	quickBeta := indexProfileByID(app.rows, "beta")
	allAlpha := indexProfileByID(app.allRows, "alpha")
	if allBeta < 0 || quickBeta < 0 || allAlpha < 0 || app.allRows[allAlpha].Favorite || !app.allRows[allBeta].Favorite || !app.rows[quickBeta].Favorite {
		t.Fatalf("favorite state = all %#v quick %#v", app.allRows, app.rows)
	}
	reloaded := newConnectionStore(filepath.Dir(store.path))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload store: %v", err)
	}
	persisted := reloaded.List()
	persistedAlpha := indexProfileByID(persisted, "alpha")
	persistedBeta := indexProfileByID(persisted, "beta")
	if persistedAlpha < 0 || persistedBeta < 0 || persisted[persistedAlpha].Favorite || !persisted[persistedBeta].Favorite {
		t.Fatalf("persisted favorite state = %#v", persisted)
	}
}

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

func TestConnectionManagerFavoritesFilterComposesWithGroupAndSearch(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod-db", Name: "Database", Group: "Production", Host: "db.internal", Favorite: true},
		{ID: "prod-web", Name: "Web", Group: "Production", Host: "web.internal"},
		{ID: "dev-db", Name: "Database", Group: "Development", Host: "db.dev", Favorite: true},
	}

	got := connectionManagerRowsFiltered(rows, "Production", "database", true)
	if len(got) != 1 || got[0].ID != "prod-db" {
		t.Fatalf("favorite group/search filter = %#v, want prod-db", got)
	}
	if got := connectionManagerRowsFiltered(rows, "Production", "", true); len(got) != 1 || got[0].ID != "prod-db" {
		t.Fatalf("favorite group filter = %#v, want prod-db", got)
	}
	if got := connectionManagerRowsFiltered(rows, "", "database", false); len(got) != 2 {
		t.Fatalf("disabled favorites filter returned %d rows, want 2", len(got))
	}
	if len(rows) != 3 || !rows[0].Favorite || rows[1].Favorite {
		t.Fatalf("favorites filter mutated source rows: %#v", rows)
	}
}

func TestConnectionManagerColumnSortingTogglesAndPreservesSource(t *testing.T) {
	rows := []connectionProfile{
		{ID: "zeta", Name: "Zeta", Group: "Production", LastUsed: "2026-08-01T10:00:00Z"},
		{ID: "alpha", Name: "Alpha", Group: "Development", LastUsed: ""},
		{ID: "beta", Name: "Beta", Group: "Production", LastUsed: "2026-08-03T10:00:00Z"},
	}

	state := nextConnectionManagerSort(connectionManagerSort{}, managerSortName)
	got := sortConnectionManagerRows(rows, state)
	if state.Descending || got[0].ID != "alpha" || got[1].ID != "beta" || got[2].ID != "zeta" {
		t.Fatalf("ascending name sort = state %#v rows %#v", state, got)
	}
	state = nextConnectionManagerSort(state, managerSortName)
	got = sortConnectionManagerRows(rows, state)
	if !state.Descending || got[0].ID != "zeta" || got[2].ID != "alpha" {
		t.Fatalf("descending name sort = state %#v rows %#v", state, got)
	}
	if rows[0].ID != "zeta" || rows[1].ID != "alpha" || rows[2].ID != "beta" {
		t.Fatalf("sorting mutated source rows: %#v", rows)
	}
}

func TestConnectionManagerRecencyAndFavoriteSortDefaultToMostUsefulFirst(t *testing.T) {
	rows := []connectionProfile{
		{ID: "never", Name: "Never"},
		{ID: "favorite-old", Name: "Favorite old", Favorite: true, LastUsed: "2026-08-01T10:00:00Z"},
		{ID: "recent", Name: "Recent", LastUsed: "2026-08-03T10:00:00Z"},
	}
	lastState := nextConnectionManagerSort(connectionManagerSort{}, managerSortLastUsed)
	last := sortConnectionManagerRows(rows, lastState)
	if !lastState.Descending || last[0].ID != "recent" || last[2].ID != "never" {
		t.Fatalf("default recency sort = state %#v rows %#v", lastState, last)
	}
	favoriteState := nextConnectionManagerSort(connectionManagerSort{}, managerSortFavorite)
	favorites := sortConnectionManagerRows(rows, favoriteState)
	if !favoriteState.Descending || favorites[0].ID != "favorite-old" {
		t.Fatalf("default favorite sort = state %#v rows %#v", favoriteState, favorites)
	}
}

func TestConnectionManagerSortHeaderTitleShowsDirection(t *testing.T) {
	state := connectionManagerSort{Column: managerSortGroup, Active: true}
	if got := connectionManagerHeaderTitle("Group", managerSortGroup, state); got != "Group ▲" {
		t.Fatalf("ascending header = %q", got)
	}
	state.Descending = true
	if got := connectionManagerHeaderTitle("Group", managerSortGroup, state); got != "Group ▼" {
		t.Fatalf("descending header = %q", got)
	}
	if got := connectionManagerHeaderTitle("Name", managerSortName, state); got != "Name" {
		t.Fatalf("inactive header = %q", got)
	}
}

func TestConnectionManagerSearchRanksMultiTermMatchesWithinSelectedGroup(t *testing.T) {
	rows := []connectionProfile{
		{ID: "metadata", Name: "Primary", Group: "Production/Database", Host: "db.internal"},
		{ID: "name", Name: "Production Database", Group: "Production", Host: "primary.internal"},
		{ID: "partial", Name: "Production API", Group: "Production", Host: "api.internal"},
		{ID: "outside", Name: "Production Database", Group: "Development", Host: "dev.internal"},
	}

	got := connectionManagerRows(rows, "Production", "production data")
	if len(got) != 2 || got[0].ID != "name" || got[1].ID != "metadata" {
		t.Fatalf("ranked multi-term manager search = %#v, want name match before metadata match", got)
	}
}

func TestConnectionManagerSelectionStatusIncludesVisibleDescriptionOnly(t *testing.T) {
	profile := connectionProfile{
		Name: "Production DB", Group: "Infrastructure", Favorite: true,
		Description: "  Primary PostgreSQL cluster  ", PasswordEnc: "encrypted-marker", PrivateKey: "/secret/key-marker",
	}
	got := managerSelectionStatus(profile)
	if !strings.Contains(got, "Description: Primary PostgreSQL cluster") {
		t.Fatalf("manager selection status omitted description: %q", got)
	}
	if strings.Contains(got, "encrypted-marker") || strings.Contains(got, "key-marker") {
		t.Fatalf("manager selection status exposed credentials: %q", got)
	}
	profile.Description = ""
	if got := managerSelectionStatus(profile); strings.Contains(got, "Description:") {
		t.Fatalf("empty description left a status suffix: %q", got)
	}
}

func TestConnectionManagerResultStatusMakesActiveFiltersVisible(t *testing.T) {
	profile := connectionProfile{Name: "Production DB", Group: "Infrastructure"}
	manager := &connectionManagerWindow{
		rows:          []connectionProfile{profile},
		idx:           0,
		selectedGroup: "Infrastructure",
		owner: &finalShellApp{allRows: []connectionProfile{
			profile,
			{Name: "Development"},
		}},
	}
	if got := manager.resultStatus(); got != "Showing 1 of 2 connections · "+managerSelectionStatus(profile) {
		t.Fatalf("filtered result status = %q", got)
	}

	manager.rows = nil
	manager.idx = -1
	if got := manager.resultStatus(); got != "Showing 0 of 2 connections · No matching connections" {
		t.Fatalf("empty filtered result status = %q", got)
	}

	manager.selectedGroup = ""
	manager.rows = []connectionProfile{profile}
	manager.idx = 0
	if got := manager.resultStatus(); got != managerSelectionStatus(profile) {
		t.Fatalf("unfiltered result status = %q", got)
	}
}

func TestConnectionManagerResultStatusExplainsInvalidSearch(t *testing.T) {
	manager := &connectionManagerWindow{
		search: uikit.NewInput(0, 0, 240, nativeControls.InputHeight, ""),
		owner:  &finalShellApp{allRows: []connectionProfile{{Name: "Production"}}},
	}
	manager.search.SetText("port:0")
	if got := manager.resultStatus(); got != "Invalid search · port: must be a number from 1 to 65535" {
		t.Fatalf("invalid search status = %q", got)
	}
}

func TestConnectionManagerResetViewClearsAndPersistsEveryViewFilter(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	config.GlobalConfig = &config.FileConfig{Language: "en"}
	config.GlobalConfig.Normalize()
	config.GlobalConfig.ConnectionManager = &config.ConnectionManagerSettings{
		FavoritesOnly: true, SortColumn: "name", SortDescending: true, SelectedGroup: "Production",
	}
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(t.TempDir(), "config.json")

	rows := []connectionProfile{
		{ID: "prod", Name: "Production", Group: "Production", Favorite: true},
		{ID: "dev", Name: "Development", Group: "Development"},
	}
	manager := &connectionManagerWindow{
		owner:         &finalShellApp{allRows: rows},
		rows:          rows[:1],
		idx:           0,
		search:        uikit.NewInput(0, 0, 200, nativeControls.InputHeight, ""),
		favoritesOnly: checkbox.NewUICheckbox(rect(0, 0, 200, nativeControls.CheckboxHeight), "Favorites only"),
		selectedGroup: "Production",
		sort:          connectionManagerSort{Column: managerSortName, Descending: true, Active: true},
	}
	manager.search.SetText("prod")
	manager.favoritesOnly.SetValue(true)
	manager.resetView()

	if manager.search.Text() != "" || manager.favoritesOnly.Value() || manager.selectedGroup != "" || manager.sort.Active {
		t.Fatalf("manager filters were not reset: query=%q favorite=%t group=%q sort=%#v", manager.search.Text(), manager.favoritesOnly.Value(), manager.selectedGroup, manager.sort)
	}
	if len(manager.rows) != 2 || manager.idx != 0 || manager.rows[manager.idx].ID != "prod" {
		t.Fatalf("reset rows/selection = %#v idx=%d", manager.rows, manager.idx)
	}
	buf, err := os.ReadFile(config.CurrentApp.AppConfigFilePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.FileConfig
	if err := json.Unmarshal(buf, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ConnectionManager == nil || saved.ConnectionManager.FavoritesOnly || saved.ConnectionManager.SortColumn != "" || saved.ConnectionManager.SelectedGroup != "" {
		t.Fatalf("persisted manager view was not reset: %#v", saved.ConnectionManager)
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
	if !manager.handleSearchKey(uikit.InputNavigationNext) || manager.idx != 1 {
		t.Fatalf("Down did not move to second manager match: idx=%d", manager.idx)
	}
	if !manager.handleSearchKey(uikit.InputNavigationPrevious) || manager.idx != 0 {
		t.Fatalf("Up did not move to first manager match: idx=%d", manager.idx)
	}
	if !manager.handleSearchKey(uikit.InputNavigationHelp) || manager.owner.searchHelp == nil || manager.owner.searchHelp.window == nil {
		t.Fatal("F1 did not open contextual search help from Connection Manager")
	}
	manager.owner.searchHelp.window.Close()
	if !manager.handleSearchKey(uikit.InputNavigationCancel) || manager.search.Text() != "" || len(manager.rows) != len(rows) {
		t.Fatalf("Escape did not clear manager query: query=%q rows=%d", manager.search.Text(), len(manager.rows))
	}
	if manager.handleSearchKey(uikit.InputNavigationCancel) {
		t.Fatal("empty manager Escape must remain available to native input handling")
	}
}

func TestConnectionManagerTableKeyboardCommandsRejectModifiedKeys(t *testing.T) {
	tests := []struct {
		name  string
		event tableview.TableKeyEvent
		want  connectionManagerTableKeyAction
	}{
		{name: "favorite", event: tableview.TableKeyEvent{Key: ' '}, want: managerTableToggleFavorite},
		{name: "edit", event: tableview.TableKeyEvent{Key: fltk_bridge.F2}, want: managerTableEdit},
		{name: "delete", event: tableview.TableKeyEvent{Key: fltk_bridge.DELETE}, want: managerTableDelete},
		{name: "copy address", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.CTRL}, want: managerTableCopyAddress},
		{name: "copy command", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.CTRL | fltk_bridge.SHIFT}, want: managerTableCopyCommand},
		{name: "duplicate", event: tableview.TableKeyEvent{Key: 'd', State: fltk_bridge.CTRL}, want: managerTableDuplicate},
		{name: "test connection", event: tableview.TableKeyEvent{Key: fltk_bridge.ENTER_KEY, State: fltk_bridge.SHIFT}, want: managerTableTestConnection},
		{name: "control space", event: tableview.TableKeyEvent{Key: ' ', State: fltk_bridge.CTRL}},
		{name: "alt f2", event: tableview.TableKeyEvent{Key: fltk_bridge.F2, State: fltk_bridge.ALT}},
		{name: "meta copy", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.META}},
		{name: "alt duplicate", event: tableview.TableKeyEvent{Key: 'd', State: fltk_bridge.ALT}},
		{name: "unknown", event: tableview.TableKeyEvent{Key: 'x'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionManagerActionForTableKey(test.event); got != test.want {
				t.Fatalf("action = %v, want %v", got, test.want)
			}
		})
	}
}

func TestQuickLauncherTableKeyboardCommandsRejectModifiedKeys(t *testing.T) {
	tests := []struct {
		name  string
		event tableview.TableKeyEvent
		want  quickLauncherTableKeyAction
	}{
		{name: "favorite", event: tableview.TableKeyEvent{Key: ' '}, want: quickLauncherTableToggleFavorite},
		{name: "edit", event: tableview.TableKeyEvent{Key: fltk_bridge.F2}, want: quickLauncherTableEdit},
		{name: "delete", event: tableview.TableKeyEvent{Key: fltk_bridge.DELETE}, want: quickLauncherTableDelete},
		{name: "copy address", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.CTRL}, want: quickLauncherTableCopyAddress},
		{name: "copy command", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.CTRL | fltk_bridge.SHIFT}, want: quickLauncherTableCopyCommand},
		{name: "duplicate", event: tableview.TableKeyEvent{Key: 'd', State: fltk_bridge.CTRL}, want: quickLauncherTableDuplicate},
		{name: "test connection", event: tableview.TableKeyEvent{Key: fltk_bridge.ENTER_KEY, State: fltk_bridge.SHIFT}, want: quickLauncherTableTestConnection},
		{name: "control space", event: tableview.TableKeyEvent{Key: ' ', State: fltk_bridge.CTRL}},
		{name: "control f2", event: tableview.TableKeyEvent{Key: fltk_bridge.F2, State: fltk_bridge.CTRL}},
		{name: "shift delete", event: tableview.TableKeyEvent{Key: fltk_bridge.DELETE, State: fltk_bridge.SHIFT}},
		{name: "plain copy", event: tableview.TableKeyEvent{Key: 'c'}},
		{name: "alt copy", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.ALT}},
		{name: "meta copy", event: tableview.TableKeyEvent{Key: 'c', State: fltk_bridge.META}},
		{name: "unknown", event: tableview.TableKeyEvent{Key: 'x'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := quickLauncherActionForTableKey(test.event); got != test.want {
				t.Fatalf("action = %v, want %v", got, test.want)
			}
		})
	}
}

func TestQuickLauncherTableKeyboardFavoritePersistsSelectedProfile(t *testing.T) {
	store := newConnectionStore(t.TempDir())
	rows := []connectionProfile{
		{ID: "alpha", Name: "Alpha", Group: "Local", Type: connectionTypeLocal},
		{ID: "beta", Name: "Beta", Group: "Local", Type: connectionTypeLocal},
	}
	if err := store.SaveActive(rows, "beta"); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	app := &finalShellApp{store: store, allRows: append([]connectionProfile(nil), rows...), rows: append([]connectionProfile(nil), rows...), idx: 1}
	if !app.handleQuickTableKey(tableview.TableKeyEvent{Key: ' '}) {
		t.Fatal("Space was not consumed by the quick launcher")
	}
	allBeta := indexProfileByID(app.allRows, "beta")
	quickBeta := indexProfileByID(app.rows, "beta")
	if allBeta < 0 || quickBeta < 0 || !app.allRows[allBeta].Favorite || !app.rows[quickBeta].Favorite {
		t.Fatalf("favorite state was not refreshed: all=%#v quick=%#v", app.allRows, app.rows)
	}
	reloaded := newConnectionStore(filepath.Dir(store.path))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload store: %v", err)
	}
	persisted := reloaded.List()
	beta := indexProfileByID(persisted, "beta")
	if beta < 0 || !persisted[beta].Favorite {
		t.Fatalf("favorite keyboard action was not persisted: %#v", persisted)
	}
	if app.handleQuickTableKey(tableview.TableKeyEvent{Key: 'x'}) {
		t.Fatal("unmapped quick-launch key was consumed")
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

func TestDuplicateConnectionProfileCreatesIndependentUnsavedCopy(t *testing.T) {
	source := connectionProfile{
		ID: "prod-db", Name: "Production DB", Group: "Infrastructure/Production",
		Type: connectionTypeSSH, Host: "db.internal", Port: 2222, Username: "operator",
		PasswordEnc: "gtenc-password", PrivateKey: "/keys/prod", WorkingDir: "/srv/db",
		Favorite: true, LastUsed: "2026-08-20T12:00:00Z",
	}
	existing := []connectionProfile{
		source,
		{ID: "copy-42", Name: "Production DB (Copy)"},
		{ID: "copy-42-2", Name: "Production DB (Copy 2)"},
	}

	got := duplicateConnectionProfile(source, existing, 42)
	if got.ID != "copy-42-3" || got.Name != "Production DB (Copy 3)" {
		t.Fatalf("duplicate identity = %q/%q, want copy-42-3/Production DB (Copy 3)", got.ID, got.Name)
	}
	if got.Favorite || got.LastUsed != "" {
		t.Fatalf("duplicate inherited projection metadata: favorite=%t lastUsed=%q", got.Favorite, got.LastUsed)
	}
	if got.Group != source.Group || got.Type != source.Type || got.Host != source.Host || got.Port != source.Port ||
		got.Username != source.Username || got.PasswordEnc != source.PasswordEnc || got.PrivateKey != source.PrivateKey || got.WorkingDir != source.WorkingDir {
		t.Fatalf("duplicate lost reusable connection fields: got=%#v source=%#v", got, source)
	}
	if source.ID != "prod-db" || source.Name != "Production DB" || !source.Favorite || source.LastUsed == "" {
		t.Fatalf("duplicate mutated source profile: %#v", source)
	}
}

func TestDuplicateConnectionProfileUsesCopyNameForUnnamedSource(t *testing.T) {
	got := duplicateConnectionProfile(connectionProfile{ID: "blank", Name: "   "}, nil, 7)
	if got.ID != "copy-7" || got.Name != "Connection (Copy)" {
		t.Fatalf("unnamed duplicate = %#v", got)
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

func TestConnectionEditorGroupOptionsExposeExistingHierarchyWithoutDuplicates(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod-db", Group: " Infrastructure / Production / Database "},
		{ID: "prod-web", Group: "Infrastructure/Production/Web"},
		{ID: "duplicate", Group: "Infrastructure/Production/Database"},
		{ID: "ungrouped", Group: ""},
	}

	got := connectionEditorGroupOptions(rows, "Infrastructure / Staging")
	want := []connectionGroupOption{
		{Path: "Infrastructure", Label: "Infrastructure"},
		{Path: "Infrastructure/Production", Label: "  Production"},
		{Path: "Infrastructure/Production/Database", Label: "    Database"},
		{Path: "Infrastructure/Production/Web", Label: "    Web"},
		{Path: "Infrastructure/Staging", Label: "  Staging"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("editor group options = %#v, want %#v", got, want)
	}
}

func TestNewConnectionFromManagerInheritsSelectedGroup(t *testing.T) {
	got := newConnectionProfileInGroup("operator", " Infrastructure / Production ")
	if got.Group != "Infrastructure/Production" {
		t.Fatalf("new connection group = %q, want selected hierarchy", got.Group)
	}
	if got.Type != connectionTypeSSH || got.Port != 22 || got.Username != "operator" {
		t.Fatalf("new connection lost SSH defaults: %#v", got)
	}

	got = newConnectionProfileInGroup("operator", "  ")
	if got.Group != tr("profile.default_group") {
		t.Fatalf("all-groups new connection group = %q, want default %q", got.Group, tr("profile.default_group"))
	}
}

func TestRenameConnectionGroupMovesExactGroupAndDescendants(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod", Group: "Infrastructure/Production", PasswordEnc: "gtenc-prod"},
		{ID: "db", Group: "Infrastructure/Production/Database", PrivateKey: "/keys/db"},
		{ID: "lab", Group: "Infrastructure/Production-Lab"},
		{ID: "dev", Group: "Infrastructure/Development"},
	}

	got, changed, err := renameConnectionGroup(rows, " Infrastructure / Production ", "Operations/Live")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 2 {
		t.Fatalf("changed = %d, want 2", changed)
	}
	wantGroups := []string{"Operations/Live", "Operations/Live/Database", "Infrastructure/Production-Lab", "Infrastructure/Development"}
	for index, want := range wantGroups {
		if got[index].Group != want {
			t.Fatalf("row %d group = %q, want %q", index, got[index].Group, want)
		}
	}
	if got[0].PasswordEnc != rows[0].PasswordEnc || got[1].PrivateKey != rows[1].PrivateKey {
		t.Fatal("group rename changed secret-bearing profile fields")
	}
	if rows[0].Group != "Infrastructure/Production" || rows[1].Group != "Infrastructure/Production/Database" {
		t.Fatalf("group rename mutated source rows: %#v", rows)
	}
}

func TestRenameConnectionGroupRejectsInvalidMoves(t *testing.T) {
	rows := []connectionProfile{{ID: "prod", Group: "Infrastructure/Production"}}
	for _, test := range []struct {
		name, oldPath, newPath string
	}{
		{name: "all groups", newPath: "Other"},
		{name: "empty destination", oldPath: "Infrastructure/Production"},
		{name: "same destination", oldPath: "Infrastructure/Production", newPath: " Infrastructure / Production "},
		{name: "descendant destination", oldPath: "Infrastructure", newPath: "Infrastructure/Production/Archive"},
		{name: "missing source", oldPath: "Missing", newPath: "Other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := renameConnectionGroup(rows, test.oldPath, test.newPath); err == nil {
				t.Fatal("rename unexpectedly succeeded")
			}
		})
	}
}

func TestRemoveConnectionGroupMovesProfilesToParentWithoutDeletingThem(t *testing.T) {
	rows := []connectionProfile{
		{ID: "prod", Group: "Infrastructure/Production", PasswordEnc: "gtenc-prod"},
		{ID: "db", Group: "Infrastructure/Production/Database", PrivateKey: "/keys/db"},
		{ID: "lab", Group: "Infrastructure/Production-Lab"},
		{ID: "dev", Group: "Infrastructure/Development"},
	}

	got, changed, err := removeConnectionGroup(rows, " Infrastructure / Production ", "Local")
	if err != nil {
		t.Fatalf("remove group: %v", err)
	}
	if changed != 2 || len(got) != len(rows) {
		t.Fatalf("remove result = changed %d rows %#v", changed, got)
	}
	wantGroups := []string{"Infrastructure", "Infrastructure/Database", "Infrastructure/Production-Lab", "Infrastructure/Development"}
	for index, want := range wantGroups {
		if got[index].Group != want {
			t.Fatalf("row %d group = %q, want %q", index, got[index].Group, want)
		}
	}
	if got[0].PasswordEnc != "gtenc-prod" || got[1].PrivateKey != "/keys/db" {
		t.Fatal("remove group changed encrypted profile payload")
	}
	if rows[0].Group != "Infrastructure/Production" || rows[1].Group != "Infrastructure/Production/Database" {
		t.Fatalf("remove group mutated source: %#v", rows)
	}
}

func TestRemoveTopLevelConnectionGroupUsesDedicatedFallbackGroup(t *testing.T) {
	rows := []connectionProfile{{ID: "prod", Group: "Production"}, {ID: "db", Group: "Production/Database"}}
	got, changed, err := removeConnectionGroup(rows, "Production", removedGroupFallback)
	if err != nil || changed != 2 {
		t.Fatalf("remove top-level group = changed %d err %v", changed, err)
	}
	if got[0].Group != "Ungrouped" || got[1].Group != "Database" {
		t.Fatalf("top-level reassignment = %#v", got)
	}
	if _, _, err := removeConnectionGroup(rows, "Missing", "Local"); err == nil {
		t.Fatal("missing group removal should fail closed")
	}
}

func TestRemoveGroupConfirmationDefaultsToCancelAndNamesAffectedProfiles(t *testing.T) {
	prompt := groupRemovePromptFor("Infrastructure/Production", 3)
	if prompt.Title != "Remove Group" || !strings.Contains(prompt.Message, "Infrastructure/Production") || !strings.Contains(prompt.Message, "3 saved connection(s)") {
		t.Fatalf("remove prompt = %#v", prompt)
	}
	var options []string
	if confirmGroupRemove("Infrastructure/Production", 3, func(_, _ string, values ...string) int {
		options = append([]string(nil), values...)
		return 1
	}) {
		t.Fatal("default cancel result removed group")
	}
	if !reflect.DeepEqual(options, []string{"Remove Group", "Cancel"}) {
		t.Fatalf("remove options = %#v", options)
	}
	if !confirmGroupRemove("Infrastructure/Production", 3, func(_, _ string, _ ...string) int { return 0 }) {
		t.Fatal("explicit remove result was rejected")
	}
	if confirmGroupRemove("Infrastructure/Production", 3, nil) {
		t.Fatal("unavailable dialog must fail closed")
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

func TestNavigatorRowsKeepPersistedSelectionVisibleWithoutExpandingLimit(t *testing.T) {
	rows := []connectionProfile{
		{ID: "favorite", Name: "Favorite", Favorite: true},
		{ID: "recent", Name: "Recent", LastUsed: "2026-08-04T10:00:00Z"},
		{ID: "selected", Name: "Selected but not recent"},
	}

	got := navigatorRowsWithSelection(rows, "", 2, "selected")
	if len(got) != 2 || got[0].ID != "selected" || got[1].ID != "favorite" {
		t.Fatalf("selected quick projection = %#v, want selected then favorite", got)
	}
	got = navigatorRowsWithSelection(rows, "favorite", 2, "selected")
	if len(got) != 1 || got[0].ID != "favorite" {
		t.Fatalf("search should not inject a non-matching selection, got %#v", got)
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

func TestConnectionManagerViewPreferencesPersistAndRestore(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	config.GlobalConfig = &config.FileConfig{Language: "en"}
	config.GlobalConfig.Normalize()
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(t.TempDir(), "config.json")

	manager := &connectionManagerWindow{}
	if !manager.persistViewPreferences(true, connectionManagerSort{Column: managerSortName, Descending: true, Active: true}, " Infrastructure / Production ") {
		t.Fatal("persistViewPreferences failed")
	}
	buf, err := os.ReadFile(config.CurrentApp.AppConfigFilePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.FileConfig
	if err := json.Unmarshal(buf, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ConnectionManager == nil || !saved.ConnectionManager.FavoritesOnly || saved.ConnectionManager.SortColumn != "name" || !saved.ConnectionManager.SortDescending || saved.ConnectionManager.SelectedGroup != "Infrastructure/Production" {
		t.Fatalf("saved manager preferences = %#v", saved.ConnectionManager)
	}
	restored := connectionManagerSortFromConfig(&saved)
	if !restored.Active || restored.Column != managerSortName || !restored.Descending {
		t.Fatalf("restored sort = %#v", restored)
	}

	previous := *config.GlobalConfig.ConnectionManager
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")
	if manager.persistViewPreferences(false, connectionManagerSort{Column: managerSortEndpoint, Active: true}, "Other") {
		t.Fatal("blocked manager preference save unexpectedly succeeded")
	}
	if got := *config.GlobalConfig.ConnectionManager; got != previous {
		t.Fatalf("failed save did not roll preferences back: got %#v want %#v", got, previous)
	}
}

func TestConnectionManagerRestoresAvailableGroupAndFallsBackFromStaleGroup(t *testing.T) {
	rows := []connectionProfile{{ID: "prod", Group: "Infrastructure/Production"}}
	cfg := &config.FileConfig{ConnectionManager: &config.ConnectionManagerSettings{SelectedGroup: " Infrastructure / Production "}}
	if got := restoredConnectionManagerGroup(cfg, rows); got != "Infrastructure/Production" {
		t.Fatalf("restored group = %q", got)
	}
	cfg.ConnectionManager.SelectedGroup = "Deleted/Group"
	if got := restoredConnectionManagerGroup(cfg, rows); got != "" {
		t.Fatalf("stale group = %q, want all groups", got)
	}
}
