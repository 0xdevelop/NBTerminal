package guis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/terminal"
)

func TestExecuteLocalCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := executeCommand(ctx, connectionProfile{Type: connectionTypeLocal, Name: "local"}, "printf nbterminal-local-ok")
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if strings.TrimSpace(out) != "nbterminal-local-ok" {
		t.Fatalf("unexpected command output: %q", out)
	}
}

func TestSSHCommandRequiresAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := executeCommand(ctx, connectionProfile{Type: connectionTypeSSH, Host: "127.0.0.1", Port: 22, Username: "nobody"}, "true")
	if err == nil {
		t.Fatal("expected error without password or private key")
	}
	if !strings.Contains(err.Error(), "ssh password or private key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteLocalCommandUsesWorkingDir(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "nbterminal-marker.txt")
	if err := os.WriteFile(marker, []byte("from-workdir"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := executeCommand(ctx, connectionProfile{Type: connectionTypeLocal, Name: "local", WorkingDir: dir}, "pwd && cat nbterminal-marker.txt")
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if !strings.Contains(out, dir) || !strings.Contains(out, "from-workdir") {
		t.Fatalf("working directory was not used, output: %q", out)
	}
}

func TestExecuteCommandResultWithSessionPersistsHistory(t *testing.T) {
	history := terminal.NewHistoryStore(filepath.Join(t.TempDir(), "terminal-history.jsonl"))
	sess := terminal.NewSession(history)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, result, err := executeCommandResultWithSession(ctx, sess, connectionProfile{ID: "local", Type: connectionTypeLocal, Name: "local"}, "printf gui-session-ok")
	if err != nil {
		t.Fatalf("executeCommandResultWithSession returned error: %v", err)
	}
	if strings.TrimSpace(out) != "gui-session-ok" || result.ExitCode != 0 {
		t.Fatalf("unexpected output/result: out=%q result=%#v", out, result)
	}
	entries, err := history.Load(10)
	if err != nil {
		t.Fatalf("history load failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Command != "printf gui-session-ok" || entries[0].ConnectionName != "local" {
		t.Fatalf("unexpected GUI command history: %#v", entries)
	}
}

func TestProfileToConnectionLoadsPrivateKeyPath(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(keyPath, []byte("-----BEGIN TEST KEY-----\nabc\n-----END TEST KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := profileToConnection(connectionProfile{ID: "dev", Name: "Dev", Type: connectionTypeSSH, Host: "example.com", Port: 2200, Username: "me", PrivateKey: keyPath, WorkingDir: "/srv/app"})
	if err != nil {
		t.Fatalf("profileToConnection failed: %v", err)
	}
	if conn.PrivateKey == keyPath || !strings.Contains(conn.PrivateKey, "BEGIN TEST KEY") {
		t.Fatalf("expected private key content to be loaded, got %q", conn.PrivateKey)
	}
	if conn.Port != 2200 || conn.Username != "me" || conn.WorkingDir != "/srv/app" {
		t.Fatalf("unexpected mapped connection: %#v", conn)
	}
}

func TestConnectionStoreSeedsFromGlobalConfig(t *testing.T) {
	oldGlobal := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = oldGlobal })
	config.GlobalConfig = &config.FileConfig{Connections: []terminal.Connection{
		{ID: "cfg-local", Name: "Cfg Local", Type: terminal.ConnectionTypeLocal, WorkingDir: "/tmp"},
		{ID: "cfg-ssh", Name: "Cfg SSH", Type: terminal.ConnectionTypeSSH, Host: "example.com", Username: "me", Password: "secret"},
	}}

	store := newConnectionStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	profiles := store.List()
	if len(profiles) != 2 {
		t.Fatalf("expected profiles from config, got %#v", profiles)
	}
	var localProfile, sshProfile connectionProfile
	for _, p := range profiles {
		if p.ID == "cfg-local" {
			localProfile = p
		}
		if p.ID == "cfg-ssh" {
			sshProfile = p
		}
	}
	if localProfile.Type != connectionTypeLocal || localProfile.WorkingDir != "/tmp" {
		t.Fatalf("expected local config profile, got %#v", localProfile)
	}
	if sshProfile.Password() != "secret" {
		t.Fatalf("expected encrypted password round-trip from config seed")
	}
}

func TestConnectionStoreSaveSyncsGlobalConfigWithoutSecrets(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	profile := connectionProfile{ID: "dev", Name: "Dev", Group: "Default", Type: connectionTypeSSH, Host: "example.com", Port: 2200, Username: "me", PrivateKey: "-----BEGIN TEST KEY-----\nsecret\n-----END TEST KEY-----"}
	profile.SetPassword("secret")
	store := newConnectionStore(t.TempDir())
	if err := store.Save([]connectionProfile{profile}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if len(config.GlobalConfig.Connections) != 1 {
		t.Fatalf("expected one synced config connection, got %#v", config.GlobalConfig.Connections)
	}
	conn := config.GlobalConfig.Connections[0]
	if conn.ID != "dev" || conn.Type != terminal.ConnectionTypeSSH || conn.Host != "example.com" || conn.Port != 2200 {
		t.Fatalf("unexpected synced connection: %#v", conn)
	}
	if conn.Password != "" || conn.PrivateKey != "" {
		t.Fatalf("secrets should remain only in GUI store, got password=%q private_key=%q", conn.Password, conn.PrivateKey)
	}
	if config.GlobalConfig.ActiveConnectionID != "dev" {
		t.Fatalf("expected active id dev, got %q", config.GlobalConfig.ActiveConnectionID)
	}
}

func TestConnectionStoreSaveAllowsUnresolvedPrivateKeyPath(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	missingKeyPath := filepath.Join(t.TempDir(), "missing_id_rsa")
	profile := connectionProfile{ID: "dev", Name: "Dev", Group: "Default", Type: connectionTypeSSH, Host: "example.com", Port: 22, Username: "me", PrivateKey: missingKeyPath}
	store := newConnectionStore(t.TempDir())
	if err := store.Save([]connectionProfile{profile}); err != nil {
		t.Fatalf("Save should not require private key file to exist until execution, got: %v", err)
	}
	if got := config.GlobalConfig.Connections[0].PrivateKey; got != missingKeyPath {
		t.Fatalf("expected non-secret key path to sync for defaults, got %q", got)
	}
}

func TestConnectionStoreNormalizesDuplicateIDs(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	store := newConnectionStore(t.TempDir())
	profiles := []connectionProfile{
		{ID: "dup", Name: "One", Group: "Default", Type: connectionTypeLocal},
		{ID: "dup", Name: "Two", Group: "Default", Type: connectionTypeLocal},
		{ID: "dup", Name: "Three", Group: "Default", Type: connectionTypeLocal},
	}
	if err := store.Save(profiles); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	got := store.List()
	ids := make(map[string]bool, len(got))
	for _, p := range got {
		if ids[p.ID] {
			t.Fatalf("duplicate profile id %q after normalize: %#v", p.ID, got)
		}
		ids[p.ID] = true
	}
	for _, want := range []string{"dup", "dup-2", "dup-3"} {
		if !ids[want] {
			t.Fatalf("missing normalized profile id %q in %#v", want, got)
		}
	}
}

func TestConnectionStoreSaveActiveSyncsSelectedGlobalConfig(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{ActiveConnectionID: "first"}

	profiles := []connectionProfile{
		{ID: "first", Name: "First", Group: "Default", Type: connectionTypeLocal},
		{ID: "second", Name: "Second", Group: "Default", Type: connectionTypeLocal},
	}
	store := newConnectionStore(t.TempDir())
	if err := store.SaveActive(profiles, "second"); err != nil {
		t.Fatalf("SaveActive failed: %v", err)
	}
	if config.GlobalConfig.ActiveConnectionID != "second" {
		t.Fatalf("expected selected active connection second, got %q", config.GlobalConfig.ActiveConnectionID)
	}
}

func TestConnectionStoreSaveActiveClearsActiveWhenListEmpty(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{ActiveConnectionID: "old"}

	store := newConnectionStore(t.TempDir())
	if err := store.SaveActive(nil, "old"); err != nil {
		t.Fatalf("SaveActive failed: %v", err)
	}
	if config.GlobalConfig.ActiveConnectionID != "" {
		t.Fatalf("expected active connection to be cleared, got %q", config.GlobalConfig.ActiveConnectionID)
	}
}

func TestPersistRuntimeProfileUpdatesStoreBeforeRun(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	dir := t.TempDir()
	store := newConnectionStore(dir)
	initial := connectionProfile{ID: "local", Name: "Old Local", Group: "Local", Type: connectionTypeLocal}
	app := &finalShellApp{store: store, rows: []connectionProfile{initial}, idx: 0}

	updated := initial
	updated.Name = "Edited Local"
	updated.WorkingDir = dir
	if err := app.persistRuntimeProfile(updated); err != nil {
		t.Fatalf("persistRuntimeProfile failed: %v", err)
	}
	if len(app.rows) != 1 || app.rows[0].Name != "Edited Local" || app.rows[0].WorkingDir != dir {
		t.Fatalf("runtime row was not updated: %#v", app.rows)
	}

	reloaded := newConnectionStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	profiles := reloaded.List()
	if len(profiles) != 1 || profiles[0].Name != "Edited Local" || profiles[0].WorkingDir != dir {
		t.Fatalf("persisted profile mismatch: %#v", profiles)
	}
	if len(config.GlobalConfig.Connections) != 1 || config.GlobalConfig.Connections[0].WorkingDir != dir {
		t.Fatalf("global config was not synced: %#v", config.GlobalConfig.Connections)
	}
}

func TestFormatHistoryEntries(t *testing.T) {
	when := time.Date(2026, 7, 2, 10, 11, 12, 0, time.Local)
	text := formatHistoryEntries(connectionProfile{ID: "local", Name: "Local Shell"}, []terminal.HistoryEntry{
		{Time: when, ConnectionID: "local", Command: "pwd", ExitCode: 0},
		{Time: when.Add(time.Minute), ConnectionID: "local", Command: "false", ExitCode: 1},
	})
	if !strings.Contains(text, "Recent history for Local Shell") || !strings.Contains(text, "exit=0 pwd") || !strings.Contains(text, "exit=1 false") {
		t.Fatalf("unexpected formatted history: %q", text)
	}
	empty := formatHistoryEntries(connectionProfile{Name: "Local Shell"}, nil)
	if !strings.Contains(empty, "no history yet") {
		t.Fatalf("expected empty history message, got %q", empty)
	}
}

func TestCenterRectInBounds(t *testing.T) {
	r := centerRectInBounds(1920, 1080, defaultWindowWidth, defaultWindowHeight)
	if r.X != 240 || r.Y != 90 || r.Width != defaultWindowWidth || r.Height != defaultWindowHeight {
		t.Fatalf("unexpected centered rect: %#v", r)
	}

	small := centerRectInBounds(1024, 768, defaultWindowWidth, defaultWindowHeight)
	if small.X != 0 || small.Y != 0 || small.Width != defaultWindowWidth || small.Height != defaultWindowHeight {
		t.Fatalf("small screen should clamp to visible origin, got %#v", small)
	}
}

func TestTopFloatRectInBounds(t *testing.T) {
	r := topFloatRectInBounds(1920, 1080, 240, 90, defaultWindowWidth, noticeWidth, noticeHeight)
	wantX := 240 + (defaultWindowWidth-noticeWidth)/2
	wantY := 90 + noticeTopOffset
	if r.X != wantX || r.Y != wantY || r.Width != noticeWidth || r.Height != noticeHeight {
		t.Fatalf("unexpected top floating rect: got %#v want x=%d y=%d", r, wantX, wantY)
	}

	edge := topFloatRectInBounds(700, 500, 640, 460, defaultWindowWidth, noticeWidth, noticeHeight)
	if edge.X != 700-noticeWidth-screenEdgePadding || edge.Y != 500-noticeHeight-screenEdgePadding {
		t.Fatalf("edge rect should stay on screen, got %#v", edge)
	}
}

func TestConnectionMatchesQuery(t *testing.T) {
	profile := connectionProfile{
		Name:        "Prod API",
		Group:       "Production",
		Type:        connectionTypeSSH,
		Host:        "10.0.0.8",
		Port:        2222,
		Username:    "deploy",
		Description: "primary backend host",
	}
	for _, query := range []string{"prod", "api", "ssh", "10.0.0.8", "deploy", "2222", "backend", ""} {
		if !connectionMatchesQuery(profile, query) {
			t.Fatalf("expected query %q to match %#v", query, profile)
		}
	}
	if connectionMatchesQuery(profile, "staging") {
		t.Fatalf("unexpected query match for staging")
	}
}

func TestActiveConnectionIndexUsesGlobalConfigSelection(t *testing.T) {
	oldGlobal := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = oldGlobal })
	rows := []connectionProfile{
		{ID: "local", Name: "Local", Type: connectionTypeLocal},
		{ID: "prod", Name: "Prod", Type: connectionTypeSSH},
	}

	config.GlobalConfig = &config.FileConfig{ActiveConnectionID: "prod"}
	if got := activeConnectionIndex(rows); got != 1 {
		t.Fatalf("expected active index 1, got %d", got)
	}

	config.GlobalConfig.ActiveConnectionID = "missing"
	if got := activeConnectionIndex(rows); got != 0 {
		t.Fatalf("missing active id should fall back to first row, got %d", got)
	}

	config.GlobalConfig = nil
	if got := activeConnectionIndex(rows); got != 0 {
		t.Fatalf("nil config should fall back to first row, got %d", got)
	}
	if got := activeConnectionIndex(nil); got != -1 {
		t.Fatalf("empty rows should return -1, got %d", got)
	}
}
