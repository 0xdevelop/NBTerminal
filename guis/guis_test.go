package guis

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/george012/gtbox"
)

func TestCommandTimeoutUsesConfigDefaultAndOverride(t *testing.T) {
	oldGlobal := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = oldGlobal })

	config.GlobalConfig = nil
	if got := commandTimeout(); got != time.Duration(config.CommandTimeoutDefaultSeconds)*time.Second {
		t.Fatalf("expected default command timeout, got %s", got)
	}

	config.GlobalConfig = &config.FileConfig{Terminal: &config.TerminalSettings{CommandTimeoutSeconds: 3}}
	if got := commandTimeout(); got != 3*time.Second {
		t.Fatalf("expected configured command timeout, got %s", got)
	}

	config.GlobalConfig.Terminal.CommandTimeoutSeconds = 0
	if got := commandTimeout(); got != time.Duration(config.CommandTimeoutDefaultSeconds)*time.Second {
		t.Fatalf("expected invalid timeout to fall back to default, got %s", got)
	}
}

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

func TestConnectionStoreSaveActiveRollsBackWhenConfigPersistenceFails(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = &config.FileConfig{}

	dir := t.TempDir()
	store := newConnectionStore(dir)
	initial := []connectionProfile{{ID: "first", Name: "First", Group: "Default", Type: connectionTypeLocal}}
	if err := store.SaveActive(initial, "first"); err != nil {
		t.Fatalf("initial SaveActive failed: %v", err)
	}
	beforeProfiles := store.List()
	beforeConfig := append([]terminal.Connection(nil), config.GlobalConfig.Connections...)

	blocker := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")

	next := []connectionProfile{{ID: "second", Name: "Second", Group: "Default", Type: connectionTypeLocal}}
	if err := store.SaveActive(next, "second"); err == nil {
		t.Fatal("expected config persistence failure")
	}
	if got := store.List(); !reflect.DeepEqual(got, beforeProfiles) {
		t.Fatalf("in-memory connection store was not rolled back: %#v", got)
	}
	reloaded := newConnectionStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if got := reloaded.List(); !reflect.DeepEqual(got, beforeProfiles) {
		t.Fatalf("on-disk connection store was not rolled back: %#v", got)
	}
	if config.GlobalConfig.ActiveConnectionID != "first" || !reflect.DeepEqual(config.GlobalConfig.Connections, beforeConfig) {
		t.Fatalf("global config was not rolled back: %#v", config.GlobalConfig)
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

func TestConnectionStoreAtomicallyPersistsUnicodeWithPrivatePermissions(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.CurrentApp = nil
	config.GlobalConfig = nil

	dir := t.TempDir()
	store := newConnectionStore(dir)
	profile := connectionProfile{
		ID: "本地-🚀", Name: "中文 · 日本語 · 한국어 · 🚀 · é", Group: "多语言",
		Type: connectionTypeLocal, WorkingDir: filepath.Join(dir, "工作目录"),
	}
	if err := store.Save([]connectionProfile{profile}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("connection store mode = %o, want 600", got)
	}
	if err := os.Chmod(store.path, 0o644); err != nil {
		t.Fatal(err)
	}
	reloaded := newConnectionStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	info, err = os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("loaded connection store mode = %o, want 600", got)
	}
	got := reloaded.List()
	if len(got) != 1 || got[0].Name != profile.Name || got[0].WorkingDir != profile.WorkingDir {
		t.Fatalf("UTF-8 profile did not round-trip: %#v", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".connections.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestConnectionStoreSetActivePersistsSelectionWithoutReplacingProfiles(t *testing.T) {
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
	if err := store.SaveActive(profiles, "first"); err != nil {
		t.Fatalf("SaveActive failed: %v", err)
	}
	before := store.List()
	if err := store.SetActive("second"); err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}
	if config.GlobalConfig.ActiveConnectionID != "second" {
		t.Fatalf("expected selected active connection second, got %q", config.GlobalConfig.ActiveConnectionID)
	}
	if got := store.List(); !reflect.DeepEqual(got, before) {
		t.Fatalf("SetActive replaced profiles: before=%#v after=%#v", before, got)
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
	app := &finalShellApp{store: store, allRows: []connectionProfile{initial}, rows: []connectionProfile{initial}, idx: 0}

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

func TestLocalTableEndpointUsesCompactLocaleText(t *testing.T) {
	previous := locales.CurrentLanguage()
	t.Cleanup(func() { locales.ResetLocaleLanguage(previous.LanguageTag()) })
	want := map[string]string{
		"en":    "Local",
		"ru":    "Локально",
		"zh-HK": "本機",
		"zh-CN": "本地",
	}
	for tag, expected := range want {
		locales.ResetLocaleLanguage(tag)
		if got := (connectionProfile{Type: connectionTypeLocal}).tableEndpoint(); got != expected {
			t.Fatalf("table endpoint for %s = %q, want %q", tag, got, expected)
		}
	}
}

func TestProductDefaultsAndFallbacksFollowCurrentLocale(t *testing.T) {
	previous := locales.CurrentLanguage()
	t.Cleanup(func() { locales.ResetLocaleLanguage(previous.LanguageTag()) })
	locales.ResetLocaleLanguage("zh-CN")

	local := connectionProfile{Type: connectionTypeLocal}
	if got := local.endpoint(); got != "本地终端" {
		t.Fatalf("localized local endpoint = %q", got)
	}
	if got := (connectionProfile{Type: connectionTypeSSH}).tableEndpoint(); got != "新主机:22" {
		t.Fatalf("localized empty host = %q", got)
	}
	if got := formatLastUsed(""); got != "从未使用" {
		t.Fatalf("localized empty last-used = %q", got)
	}
	defaults := defaultConnections()
	if len(defaults) != 2 || defaults[0].Name != "本地终端" || defaults[0].Group != "本地" || defaults[1].Name != "SSH 示例" || defaults[1].Group != "示例" {
		t.Fatalf("localized seed profiles = %#v", defaults)
	}
	created := newConnectionProfile("测试用户")
	if created.Name != "新建 SSH" || created.Group != "默认" || created.Username != "测试用户" {
		t.Fatalf("localized new profile = %#v", created)
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

func TestEnsureRuntimeSessionReusesActiveRuntimeIdentity(t *testing.T) {
	app := &finalShellApp{sessions: newSessionWorkspace()}
	profile := connectionProfile{ID: "saved-local", Name: "本地", Type: connectionTypeSSH}

	first, ok := app.ensureRuntimeSession(profile)
	if !ok || first.ID == "" || first.ID == profile.ID {
		t.Fatalf("first runtime session = %#v ok=%t", first, ok)
	}
	second, ok := app.ensureRuntimeSession(profile)
	if !ok || second.ID != first.ID {
		t.Fatalf("active runtime identity changed: first=%#v second=%#v ok=%t", first, second, ok)
	}
	if got := len(app.sessions.Tabs()); got != 1 {
		t.Fatalf("command preparation created duplicate runtime tabs: %d", got)
	}

	other := connectionProfile{ID: "saved-ssh", Name: "Сервер", Type: connectionTypeSSH}
	third, ok := app.ensureRuntimeSession(other)
	if !ok || third.ID == first.ID || third.ProfileID != other.ID {
		t.Fatalf("different profile did not receive a distinct runtime: %#v ok=%t", third, ok)
	}
}

func TestCommandRunLifecycleAllowsIndependentSessionsAndRejectsSameSessionOverlap(t *testing.T) {
	app := &finalShellApp{}
	firstContext, firstCancel := context.WithCancel(context.Background())
	firstID, ok := app.beginCommandRunForSession(firstCancel, "first")
	if !ok || firstID == 0 {
		t.Fatalf("expected first session command to start: id=%d ok=%v", firstID, ok)
	}
	if _, duplicateOK := app.beginCommandRunForSession(func() {}, "first"); duplicateOK {
		t.Fatal("overlapping command in the same session should be rejected")
	}

	secondContext, secondCancel := context.WithCancel(context.Background())
	secondID, secondOK := app.beginCommandRunForSession(secondCancel, "second")
	if !secondOK || secondID == firstID {
		t.Fatalf("independent session command should start: id=%d ok=%v", secondID, secondOK)
	}
	if !app.commandRunningForSession("first") || !app.commandRunningForSession("second") || !app.anyCommandRunning() {
		t.Fatal("run registry did not retain both active sessions")
	}

	if !app.cancelCommandRun("first") {
		t.Fatal("first session run was not cancelled")
	}
	select {
	case <-firstContext.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelling first session did not invoke its cancel function")
	}
	select {
	case <-secondContext.Done():
		t.Fatal("cancelling first session also cancelled the second")
	default:
	}
	if !app.finishCommandRun("first", firstID) {
		t.Fatal("first session command did not finish")
	}
	if app.finishCommandRun("second", firstID) {
		t.Fatal("stale run ID finished a different session")
	}
	if !app.commandRunningForSession("second") || !app.finishCommandRun("second", secondID) || app.anyCommandRunning() {
		t.Fatal("second session run lifecycle did not finish independently")
	}
}

func TestStopCommandCancelsOnlyActiveSession(t *testing.T) {
	app := &finalShellApp{sessions: newSessionWorkspace()}
	app.sessions.Open(connectionProfile{ID: "first", Name: "First", Type: connectionTypeLocal})
	first, _ := app.sessions.Active()
	app.sessions.Open(connectionProfile{ID: "second", Name: "Second", Type: connectionTypeLocal})
	second, _ := app.sessions.Active()
	firstContext, firstCancel := context.WithCancel(context.Background())
	secondContext, secondCancel := context.WithCancel(context.Background())
	if _, ok := app.beginCommandRunForSession(firstCancel, first.ID); !ok {
		t.Fatal("first session run did not start")
	}
	if _, ok := app.beginCommandRunForSession(secondCancel, second.ID); !ok {
		t.Fatal("second session run did not start")
	}

	app.sessions.Select(0)
	app.stopCommand()
	select {
	case <-firstContext.Done():
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the active tab")
	}
	select {
	case <-secondContext.Done():
		t.Fatal("Stop cancelled a background tab")
	default:
	}
}

func TestGUIOutputBatcherCoalescesPendingUnicodeAndPreservesArrivalOrder(t *testing.T) {
	var scheduled []func()
	var appended []string
	var batcher *guiOutputBatcher
	batcher = newGUIOutputBatcher(func(fn func()) {
		scheduled = append(scheduled, fn)
	}, func(text string) {
		appended = append(appended, text)
		if len(appended) == 1 {
			// Output arriving while the GUI is applying the current batch must be
			// scheduled afterward rather than lost or inserted out of order.
			batcher.Enqueue("追加 🚀\n")
		}
	})

	batcher.Enqueue("中文\n")
	batcher.Enqueue("日本語 · 한국어 · é\n")
	completed := false
	batcher.AfterFlush(func() { completed = true })
	if len(scheduled) != 1 {
		t.Fatalf("pending output should share one GUI wake-up, got %d", len(scheduled))
	}
	scheduled[0]()
	if len(appended) != 1 || appended[0] != "中文\n日本語 · 한국어 · é\n" {
		t.Fatalf("first batch lost or reordered Unicode output: %#v", appended)
	}
	if completed {
		t.Fatal("completion ran before output queued during the first append")
	}
	if len(scheduled) != 2 {
		t.Fatalf("output arriving during append should schedule one follow-up, got %d", len(scheduled))
	}
	scheduled[1]()
	if len(appended) != 2 || appended[1] != "追加 🚀\n" {
		t.Fatalf("follow-up batch mismatch: %#v", appended)
	}
	if !completed {
		t.Fatal("completion did not run after all ordered output was appended")
	}
}

func TestCommandBarLayoutKeepsControlsInsideTerminalPanel(t *testing.T) {
	const (
		winW   = defaultWindowWidth
		margin = 22
		leftW  = 492
		gap    = 22
		rightX = margin + leftW + gap
		rightW = winW - rightX - margin
	)
	bar := commandBarLayout(rightX, rightW)
	rightEdge := rightX + rightW
	ordered := []struct {
		name string
		x    int
		w    int
	}{
		{name: "history", x: bar.history.X, w: bar.history.Width},
		{name: "last", x: bar.last.X, w: bar.last.Width},
		{name: "clear", x: bar.clear.X, w: bar.clear.Width},
		{name: "command", x: bar.command.X, w: bar.command.Width},
		{name: "stop", x: bar.stop.X, w: bar.stop.Width},
		{name: "run", x: bar.run.X, w: bar.run.Width},
	}
	prevRight := rightX
	for _, item := range ordered {
		if item.x < rightX || item.x+item.w > rightEdge {
			t.Fatalf("%s is outside terminal panel: x=%d w=%d panel=%d..%d", item.name, item.x, item.w, rightX, rightEdge)
		}
		if item.x < prevRight {
			t.Fatalf("%s overlaps previous control: x=%d previousRight=%d", item.name, item.x, prevRight)
		}
		prevRight = item.x + item.w
	}
	if bar.command.Width < 360 {
		t.Fatalf("command input is too narrow for usable UX: %d", bar.command.Width)
	}
	if bar.commandLabelY >= bar.command.Y {
		t.Fatalf("command label should sit above input: labelY=%d inputY=%d", bar.commandLabelY, bar.command.Y)
	}
}

func TestTerminalWelcomeText(t *testing.T) {
	text := terminalWelcomeText()
	for _, want := range []string{"NBTerminal", "Local", "SSH", "UTF-8"} {
		if !strings.Contains(text, want) {
			t.Fatalf("welcome text missing %q: %q", want, text)
		}
	}
}

func TestNativeFontsCoverSupportedDesktopPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		fonts := nativeFontsForOS(goos)
		for name, font := range map[string]string{
			"sans": fonts.sans, "sans bold": fonts.sansBold,
			"sans italic": fonts.sansItalic, "sans bold italic": fonts.sansBoldItalic,
			"mono": fonts.mono, "mono bold": fonts.monoBold,
			"mono italic": fonts.monoItalic, "mono bold italic": fonts.monoBoldItalic,
			"emoji": fonts.emoji,
		} {
			if strings.TrimSpace(font) == "" {
				t.Fatalf("%s %s font is empty", goos, name)
			}
		}
	}
}

func TestNativeWindowClassSupportsIsolatedGUIRuns(t *testing.T) {
	t.Setenv("NBTERMINAL_WM_CLASS", "NBTerminal-QA")
	if got := nativeWindowClass(); got != "NBTerminal-QA" {
		t.Fatalf("native window class = %q", got)
	}
	t.Setenv("NBTERMINAL_WM_CLASS", "  ")
	if got := nativeWindowClass(); got != "NBTerminal" {
		t.Fatalf("default native window class = %q", got)
	}
}

func TestEmojiRuneDetectionKeepsTextScriptsOnPrimaryFont(t *testing.T) {
	for _, r := range []rune{'🚀', '😀', '☀', '\ufe0f', '\u200d'} {
		if !isEmojiRune(r) {
			t.Errorf("expected %U to use emoji fallback", r)
		}
	}
	for _, r := range []rune{'中', '日', '한', 'e', '\u0301'} {
		if isEmojiRune(r) {
			t.Errorf("expected %U to remain on primary font", r)
		}
	}
}

func TestNavigatorSortsRecentConnectionsDeterministically(t *testing.T) {
	rows := []connectionProfile{
		{ID: "never", Name: "Never", Group: "B"},
		{ID: "older", Name: "Older", Group: "A", LastUsed: "2026-08-01T10:00:00Z"},
		{ID: "recent-b", Name: "Beta", Group: "B", LastUsed: "2026-08-03T10:00:00Z"},
		{ID: "recent-a", Name: "Alpha", Group: "A", LastUsed: "2026-08-03T10:00:00Z"},
	}

	sortConnectionsForNavigator(rows)
	got := []string{rows[0].ID, rows[1].ID, rows[2].ID, rows[3].ID}
	want := []string{"recent-a", "recent-b", "older", "never"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("navigator order = %#v, want %#v", got, want)
	}
}

func TestMarkProfileUsedUpdatesTimestampWithoutTouchingSecrets(t *testing.T) {
	profile := connectionProfile{ID: "target", Name: "目标 🚀", PasswordEnc: "encrypted"}
	usedAt := time.Date(2026, 8, 3, 11, 12, 13, 0, time.UTC)

	used := markProfileUsed(profile, usedAt)
	if used.ID != "target" || used.LastUsed != usedAt.Format(time.RFC3339) {
		t.Fatalf("used profile mismatch: %#v", used)
	}
	if used.PasswordEnc != "encrypted" {
		t.Fatalf("secret preservation failed: %#v", used)
	}
	if profile.LastUsed != "" {
		t.Fatalf("input profile was mutated: %#v", profile)
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

func TestFilterConnectionsNarrowsRows(t *testing.T) {
	rows := []connectionProfile{
		{ID: "local", Name: "Local Shell", Group: "Local", Type: connectionTypeLocal},
		{ID: "prod", Name: "Prod API", Group: "Production", Type: connectionTypeSSH, Host: "10.0.0.8", Username: "deploy"},
		{ID: "stage", Name: "Stage API", Group: "Staging", Type: connectionTypeSSH, Host: "10.0.1.8", Username: "deploy"},
	}
	filtered := filterConnections(rows, "prod")
	if len(filtered) != 1 || filtered[0].ID != "prod" {
		t.Fatalf("expected only prod match, got %#v", filtered)
	}
	all := filterConnections(rows, "")
	if len(all) != len(rows) || &all[0] == &rows[0] {
		t.Fatalf("empty filter should return copied full rows, got %#v", all)
	}
}

func TestUpsertAndRemoveProfileByID(t *testing.T) {
	rows := []connectionProfile{{ID: "local", Name: "Local"}, {ID: "prod", Name: "Prod"}}
	rows = upsertProfile(rows, connectionProfile{ID: "prod", Name: "Prod API"})
	if len(rows) != 2 || rows[1].Name != "Prod API" {
		t.Fatalf("expected upsert to replace prod, got %#v", rows)
	}
	rows = upsertProfile(rows, connectionProfile{ID: "stage", Name: "Stage"})
	if len(rows) != 3 || rows[2].ID != "stage" {
		t.Fatalf("expected upsert to append stage, got %#v", rows)
	}
	rows = removeProfileByID(rows, "prod")
	if len(rows) != 2 || indexProfileByID(rows, "prod") != -1 {
		t.Fatalf("expected prod removal, got %#v", rows)
	}
}

func TestPersistProfileCommitsOnlyAfterPersistenceSucceeds(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.GlobalConfig = nil
	config.CurrentApp = nil

	initial := connectionProfile{ID: "local", Name: "Local", Type: connectionTypeLocal}
	app := &finalShellApp{
		store:   &connectionStore{path: filepath.Join(t.TempDir(), "connections.json")},
		allRows: []connectionProfile{initial},
		rows:    []connectionProfile{initial},
		idx:     0,
	}
	updated := initial
	updated.Name = "Edited Local"
	if err := app.persistProfile(updated); err != nil {
		t.Fatalf("persistProfile failed: %v", err)
	}
	if app.allRows[0].Name != updated.Name || app.rows[0].Name != updated.Name {
		t.Fatalf("successful save was not published: all=%#v rows=%#v", app.allRows, app.rows)
	}

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.store.path = filepath.Join(blocker, "connections.json")
	beforeAll := append([]connectionProfile(nil), app.allRows...)
	beforeRows := append([]connectionProfile(nil), app.rows...)
	failed := updated
	failed.Name = "Must Not Leak Into UI"
	if err := app.persistProfile(failed); err == nil {
		t.Fatal("expected persistence failure")
	}
	if !reflect.DeepEqual(app.allRows, beforeAll) || !reflect.DeepEqual(app.rows, beforeRows) {
		t.Fatalf("failed save leaked into GUI state: all=%#v rows=%#v", app.allRows, app.rows)
	}
}

func TestRemoveSelectedProfileCommitsOnlyAfterPersistenceSucceeds(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })
	config.GlobalConfig = nil
	config.CurrentApp = nil

	rows := []connectionProfile{
		{ID: "local", Name: "Local", Type: connectionTypeLocal},
		{ID: "prod", Name: "Prod", Type: connectionTypeSSH},
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &finalShellApp{
		store:   newConnectionStore(blocker),
		allRows: append([]connectionProfile(nil), rows...),
		rows:    append([]connectionProfile(nil), rows...),
		idx:     1,
	}
	if _, err := app.removeSelectedProfile(); err == nil {
		t.Fatal("expected persistence failure")
	}
	if !reflect.DeepEqual(app.allRows, rows) || !reflect.DeepEqual(app.rows, rows) || app.idx != 1 {
		t.Fatalf("failed delete mutated UI state: all=%#v rows=%#v idx=%d", app.allRows, app.rows, app.idx)
	}

	app.store = newConnectionStore(t.TempDir())
	removed, err := app.removeSelectedProfile()
	if err != nil {
		t.Fatalf("successful delete failed: %v", err)
	}
	if removed.ID != "prod" || len(app.rows) != 1 || app.rows[0].ID != "local" || app.idx != 0 {
		t.Fatalf("unexpected committed delete: removed=%#v rows=%#v idx=%d", removed, app.rows, app.idx)
	}
}
