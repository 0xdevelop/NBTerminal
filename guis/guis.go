package guis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/hostmonitor"
	"github.com/0xdevelop/NBTerminal/internal/database"
	"github.com/0xdevelop/NBTerminal/internal/persistence"
	"github.com/0xdevelop/NBTerminal/internal/security"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/foundation"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/automation"
	"github.com/0xdevelop/fltk2go/uikit/progress"
	"github.com/0xdevelop/fltk2go/uikit/screen"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
	"github.com/0xdevelop/fltk2go/uikit/tabview"
	"github.com/george012/gtbox/gtbox_encryption"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	connectionStoreFile = "connections.json"
	secretKey           = "nbterminal-connections-v1"
	defaultWindowWidth  = 1440
	defaultWindowHeight = 900
	noticeWidth         = 560
	noticeHeight        = 118
	noticeTopOffset     = 72
	screenEdgePadding   = 8
)

type colorToken struct{ r, g, b uint8 }

var modernTheme = struct {
	background, card, elevated, terminal, foreground, muted, border colorToken
	primary, primaryText, selected, destructive                     colorToken
}{
	background:  colorToken{15, 23, 42},
	card:        colorToken{27, 35, 54},
	elevated:    colorToken{39, 47, 66},
	terminal:    colorToken{2, 6, 23},
	foreground:  colorToken{248, 250, 252},
	muted:       colorToken{148, 163, 184},
	border:      colorToken{71, 85, 105},
	primary:     colorToken{34, 197, 94},
	primaryText: colorToken{15, 23, 42},
	selected:    colorToken{20, 83, 45},
	destructive: colorToken{220, 38, 38},
}

func tokenColor(token colorToken) fltk_bridge.Color {
	return themeColor(token.r, token.g, token.b)
}

type connectionType string

const (
	connectionTypeSSH   connectionType = "ssh"
	connectionTypeLocal connectionType = "local"
)

type connectionProfile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Group       string         `json:"group"`
	Type        connectionType `json:"type"`
	Host        string         `json:"host"`
	Port        int            `json:"port"`
	Username    string         `json:"username"`
	PasswordEnc string         `json:"password_enc,omitempty"`
	PrivateKey  string         `json:"private_key,omitempty"`
	WorkingDir  string         `json:"working_dir,omitempty"`
	LastUsed    string         `json:"last_used,omitempty"`
	Favorite    bool           `json:"favorite,omitempty"`
	Description string         `json:"description,omitempty"`
}

func (p connectionProfile) Password() string {
	if p.PasswordEnc == "" {
		return ""
	}
	return gtbox_encryption.GTDec(p.PasswordEnc, secretKey)
}

func (p *connectionProfile) SetPassword(password string) {
	if password == "" {
		return
	}
	p.PasswordEnc = gtbox_encryption.GTEnc(password, secretKey)
}

func (p connectionProfile) endpoint() string {
	if p.Type == connectionTypeLocal {
		return tr("profile.local_shell")
	}
	port := p.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s@%s:%d", p.Username, p.Host, port)
}

func (p connectionProfile) tableEndpoint() string {
	if p.Type == connectionTypeLocal {
		return tr("profile.local_compact")
	}
	host := strings.TrimSpace(p.Host)
	if host == "" {
		host = tr("profile.new_host")
	}
	port := p.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func formatLastUsed(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return tr("profile.never")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("01-02 15:04")
	}
	if len(value) > len("01-02 15:04") {
		return value[:len("01-02 15:04")]
	}
	return value
}

func formatLastUsedCompact(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return tr("profile.never")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local().Format("01-02")
	}
	if len(value) > len("01-02") {
		return value[:len("01-02")]
	}
	return value
}

type connectionStore struct {
	path          string
	db            *database.DB
	encryptionKey string
	mu            sync.Mutex
	list          []connectionProfile
}

func newConnectionStore(dataDir string) *connectionStore {
	return &connectionStore{path: filepath.Join(dataDir, connectionStoreFile)}
}

func newSQLiteConnectionStore(dataDir string, db *database.DB, encryptionKey string) *connectionStore {
	return &connectionStore{path: filepath.Join(dataDir, connectionStoreFile), db: db, encryptionKey: encryptionKey}
}

func (s *connectionStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.loadSQLiteLocked()
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	buf, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.list = defaultConnections()
			if cfgProfiles := profilesFromConfig(config.GlobalConfig); len(cfgProfiles) > 0 {
				s.list = cfgProfiles
			}
			return s.saveLocked()
		}
		return err
	}
	if len(strings.TrimSpace(string(buf))) == 0 {
		s.list = defaultConnections()
		return s.saveLocked()
	}
	if err := json.Unmarshal(buf, &s.list); err != nil {
		return err
	}
	s.normalizeLocked()
	return os.Chmod(s.path, 0o600)
}

func (s *connectionStore) saveLocked() error {
	s.normalizeLocked()
	buf, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	return persistence.AtomicWriteFile(s.path, buf, 0o600)
}

func (s *connectionStore) Save(list []connectionProfile) error {
	return s.SaveActive(list, "")
}

// SaveActive persists the connection list and records the selected connection in
// the shared app config. This keeps the FinalShell-style GUI, command runner and
// config file aligned after a user selects/edits a profile and immediately runs
// it without pressing any extra "make active" control.
func (s *connectionStore) SaveActive(list []connectionProfile, activeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.saveActiveSQLiteLocked(list, activeID)
	}
	previous := append([]connectionProfile(nil), s.list...)
	s.list = append([]connectionProfile(nil), list...)
	if err := s.saveLocked(); err != nil {
		s.list = previous
		return err
	}
	if err := syncConfigConnections(s.list, activeID); err != nil {
		s.list = previous
		if rollbackErr := s.saveLocked(); rollbackErr != nil {
			return fmt.Errorf("sync app config: %w; rollback connection store: %v", err, rollbackErr)
		}
		return fmt.Errorf("sync app config: %w", err)
	}
	return nil
}

// SetActive records a table selection without rewriting the encrypted
// connection store. Selection changes are frequent enough to deserve a small,
// explicit path, while the shared config remains the source of truth used at
// the next launch.
func (s *connectionStore) SetActive(activeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.setActiveSQLiteLocked(activeID)
	}
	return syncConfigConnections(s.list, activeID)
}

func (s *connectionStore) List() []connectionProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]connectionProfile(nil), s.list...)
	sortConnectionsForNavigator(out)
	return out
}

func sortConnectionsForNavigator(rows []connectionProfile) {
	sort.SliceStable(rows, func(i, j int) bool {
		leftTime, leftOK := parseLastUsed(rows[i].LastUsed)
		rightTime, rightOK := parseLastUsed(rows[j].LastUsed)
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		if rows[i].Group != rows[j].Group {
			return rows[i].Group < rows[j].Group
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].ID < rows[j].ID
	})
}

func parseLastUsed(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed, err == nil
}

func markProfileUsed(profile connectionProfile, usedAt time.Time) connectionProfile {
	profile.LastUsed = usedAt.UTC().Format(time.RFC3339)
	return profile
}

func (s *connectionStore) normalizeLocked() {
	seenIDs := make(map[string]struct{}, len(s.list))
	for i := range s.list {
		if s.list[i].ID == "" {
			s.list[i].ID = fmt.Sprintf("conn-%d-%d", time.Now().UnixNano(), i)
		}
		s.list[i].ID = uniqueProfileID(s.list[i].ID, seenIDs)
		if s.list[i].Name == "" {
			s.list[i].Name = tr("profile.new_connection")
		}
		if s.list[i].Group == "" {
			s.list[i].Group = tr("profile.default_group")
		}
		if s.list[i].Type == "" {
			s.list[i].Type = connectionTypeSSH
		}
		if s.list[i].Type == connectionTypeSSH && s.list[i].Port == 0 {
			s.list[i].Port = 22
		}
	}
}

func uniqueProfileID(id string, seen map[string]struct{}) string {
	base := strings.TrimSpace(id)
	if base == "" {
		base = "conn"
	}
	candidate := base
	for n := 2; ; n++ {
		if _, ok := seen[candidate]; !ok {
			seen[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

func defaultConnections() []connectionProfile {
	return []connectionProfile{
		{ID: "local-shell", Name: tr("profile.local_shell"), Group: tr("profile.local_group"), Type: connectionTypeLocal, Description: tr("profile.local_description")},
		{ID: "example-ssh", Name: tr("profile.example_ssh"), Group: tr("profile.examples_group"), Type: connectionTypeSSH, Host: "127.0.0.1", Port: 22, Username: os.Getenv("USER"), Description: tr("profile.ssh_description")},
	}
}

func profilesFromConfig(cfg *config.FileConfig) []connectionProfile {
	if cfg == nil || len(cfg.Connections) == 0 {
		return nil
	}
	profiles := make([]connectionProfile, 0, len(cfg.Connections))
	for _, conn := range terminal.NormalizeConnections(cfg.Connections) {
		profile := connectionProfile{
			ID:          conn.ID,
			Name:        conn.Name,
			Group:       "Config",
			Type:        connectionTypeSSH,
			Host:        conn.Host,
			Port:        conn.Port,
			Username:    conn.Username,
			PrivateKey:  conn.PrivateKey,
			WorkingDir:  conn.WorkingDir,
			Description: conn.Description,
		}
		if conn.Type == terminal.ConnectionTypeLocal {
			profile.Type = connectionTypeLocal
			profile.Group = "Local"
		}
		profile.SetPassword(conn.Password)
		profiles = append(profiles, profile)
	}
	return profiles
}

func syncConfigConnections(profiles []connectionProfile, activeID string) error {
	if config.GlobalConfig == nil {
		return nil
	}
	previousConnections := append([]terminal.Connection(nil), config.GlobalConfig.Connections...)
	previousActiveID := config.GlobalConfig.ActiveConnectionID
	config.GlobalConfig.Connections = nil
	config.GlobalConfig.ActiveConnectionID = normalizedActiveProfileID(profiles, activeID)
	if config.CurrentApp == nil || config.CurrentApp.AppConfigFilePath == "" {
		return nil
	}
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		config.GlobalConfig.Connections = previousConnections
		config.GlobalConfig.ActiveConnectionID = previousActiveID
		return err
	}
	return nil
}

type tableModel struct {
	rows     []connectionProfile
	cellText func(connectionProfile, int) string
}

func (m *tableModel) NumberOfRows(_ *tableview.TableView) int { return len(m.rows) }
func (m *tableModel) CellForColumn(_ *tableview.TableView, row, col int) *tableview.TableViewCell {
	cell := tableview.NewCell("connection-cell")
	if row < 0 || row >= len(m.rows) {
		return cell
	}
	p := m.rows[row]
	if m.cellText != nil {
		cell.SetText(m.cellText(p, col))
	}
	return cell
}

type finalShellApp struct {
	database *database.DB
	store    *connectionStore
	history  *terminal.HistoryStore
	session  *terminal.Session
	sessions *sessionWorkspace
	allRows  []connectionProfile
	rows     []connectionProfile
	idx      int

	window     *uikit.UIWindow
	workspace  *uikit.UISplitView
	quickPanel *uikit.UIGroup
	table      *uikit.UITableView
	model      *tableModel

	mainTitle        *uikit.UILabel
	mainSubtitle     *uikit.UILabel
	managerButton    *uikit.UIButton
	settingsButton   *uikit.UIButton
	quickTitle       *uikit.UILabel
	quickSubtitle    *uikit.UILabel
	searchLabel      *uikit.UILabel
	searchInput      *uikit.Input
	findButton       *uikit.UIButton
	summaryTitle     *uikit.UILabel
	selectedName     *uikit.UILabel
	selectedDetail   *uikit.UILabel
	selectedRecent   *uikit.UILabel
	connectButton    *uikit.UIButton
	cmdInput         *uikit.UITextView
	output           *uikit.UITerminalView
	terminalColumns  int
	sessionTabs      *uikit.UITabView
	terminalPanel    *uikit.UIGroup
	terminalTitle    *uikit.UILabel
	terminalSubtitle *uikit.UILabel
	commandLabel     *uikit.UILabel
	closeTabButton   *uikit.UIButton
	historyButton    *uikit.UIButton
	lastButton       *uikit.UIButton
	clearButton      *uikit.UIButton
	status           *uikit.UILabel
	notice           *uikit.UIWindow
	runButton        *uikit.UIButton
	stopButton       *uikit.UIButton
	settings         *settingsWindow
	editor           *connectionEditor
	manager          *connectionManagerWindow
	monitorPanel     *uikit.UIGroup
	monitorTitle     *uikit.UILabel
	monitorStatus    *uikit.UILabel
	monitorUptime    *uikit.UILabel
	monitorLoad      *uikit.UILabel
	monitorCPU       *uikit.UILabel
	monitorMemory    *uikit.UILabel
	monitorNetwork   *uikit.UILabel
	monitorCPUBar    *progress.UIProgressView
	monitorMemBar    *progress.UIProgressView

	runMu       sync.Mutex
	runID       uint64
	runs        map[string]commandRun
	interactive *interactiveRuntimeRegistry
	sshHostKeys *terminal.SSHHostKeyStore
	monitorMu   sync.RWMutex
	monitors    map[string]*hostmonitor.Session
}

type commandRun struct {
	id     uint64
	cancel context.CancelFunc
}

// guiOutputBatcher coalesces command output that arrives faster than FLTK can
// paint it. At most one GUI callback is pending at a time, while output that
// arrives during an append is retained for one ordered follow-up callback. This
// keeps high-volume local/SSH streams responsive without sacrificing live output
// for slower commands or moving native widget mutations off the GUI thread.
type guiOutputBatcher struct {
	mu        sync.Mutex
	pending   strings.Builder
	after     []func()
	scheduled bool
	schedule  func(func())
	append    func(string)
}

func newGUIOutputBatcher(schedule func(func()), appendText func(string)) *guiOutputBatcher {
	return &guiOutputBatcher{schedule: schedule, append: appendText}
}

func (b *guiOutputBatcher) Enqueue(text string) {
	if b == nil || text == "" {
		return
	}
	b.mu.Lock()
	b.pending.WriteString(text)
	if b.scheduled {
		b.mu.Unlock()
		return
	}
	b.scheduled = true
	b.mu.Unlock()
	b.schedule(b.drain)
}

// AfterFlush runs fn on the GUI thread after every output fragment queued before
// command completion has been appended. It keeps the final status transition
// ordered behind output that arrived while an earlier GUI batch was painting.
func (b *guiOutputBatcher) AfterFlush(fn func()) {
	if b == nil || fn == nil {
		return
	}
	b.mu.Lock()
	b.after = append(b.after, fn)
	if b.scheduled {
		b.mu.Unlock()
		return
	}
	b.scheduled = true
	b.mu.Unlock()
	b.schedule(b.drain)
}

func (b *guiOutputBatcher) drain() {
	b.mu.Lock()
	text := b.pending.String()
	b.pending.Reset()
	b.mu.Unlock()
	if text != "" {
		b.append(text)
	}

	b.mu.Lock()
	if b.pending.Len() > 0 {
		b.mu.Unlock()
		b.schedule(b.drain)
		return
	}
	after := append([]func(){}, b.after...)
	b.after = nil
	b.scheduled = false
	b.mu.Unlock()
	for _, fn := range after {
		fn()
	}
}

func LoadGUIWithFLTKGO(_ []byte) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		fltk2go.Lock()
	}
	configureNativeFonts(runtime.GOOS)

	masterKey, err := security.LoadOrCreateMasterKey(
		filepath.Dir(config.CurrentApp.AppConfigFilePath),
		filepath.Join(config.CurrentApp.DataDir, database.FileName),
	)
	if err != nil {
		gtbox_log.LogErrorf("load local encryption key failed: %s", err.Error())
		return
	}
	profileEncryptionKey, err := security.PurposeKey(masterKey, "sqlite-profile-v1")
	if err != nil {
		gtbox_log.LogErrorf("derive profile encryption key failed: %s", err.Error())
		return
	}
	historyEncryptionKey, err := security.PurposeKey(masterKey, "sqlite-history-v1")
	if err != nil {
		gtbox_log.LogErrorf("derive history encryption key failed: %s", err.Error())
		return
	}
	db, err := database.Open(config.CurrentApp.DataDir)
	if err != nil {
		gtbox_log.LogErrorf("open embedded database failed: %s", err.Error())
		return
	}
	defer db.Close()
	history := terminal.NewSQLiteHistoryStore(db, filepath.Join(config.CurrentApp.DataDir, "terminal-history.jsonl"), historyEncryptionKey)
	if _, err := history.MigrateLegacy(); err != nil {
		gtbox_log.LogErrorf("migrate command history failed: %s", err.Error())
		return
	}
	app := &finalShellApp{
		database:    db,
		store:       newSQLiteConnectionStore(config.CurrentApp.DataDir, db, profileEncryptionKey),
		history:     history,
		session:     terminal.NewSession(history),
		sessions:    newSessionWorkspace(),
		interactive: newInteractiveRuntimeRegistry(),
		sshHostKeys: terminal.NewSSHHostKeyStore(filepath.Join(config.CurrentApp.DataDir, "known_hosts")),
		idx:         -1,
	}
	defer app.stopAllMonitors()
	defer app.interactive.CloseAll()
	if err := app.store.Load(); err != nil {
		gtbox_log.LogErrorf("load connection store failed: %s", err.Error())
		return
	}
	app.allRows = app.store.List()
	app.rows = navigatorRows(app.allRows, "", quickConnectionLimit)
	app.build()
	if automation.Enabled() {
		addr := strings.TrimSpace(os.Getenv("FLTK2GO_AUTOMATION_ADDR"))
		if addr == "" {
			// Keep the automation endpoint separate from NBTerminal's application
			// API (8765). The server is compiled out of release-tag builds.
			addr = "127.0.0.1:8766"
		}
		srv, err := automation.StartDebugServer(automation.Config{Addr: addr})
		if err != nil {
			gtbox_log.LogErrorf("start GUI automation debug server failed: %s", err.Error())
		} else {
			defer srv.Close()
			gtbox_log.LogInfof("GUI automation debug server listening on http://%s", srv.Addr())
		}
	}
	fltk2go.Run()
}

type nativeFontSet struct {
	sans, sansBold, sansItalic, sansBoldItalic string
	mono, monoBold, monoItalic, monoBoldItalic string
	emoji                                      string
}

func nativeFontsForOS(goos string) nativeFontSet {
	switch goos {
	case "darwin":
		return nativeFontSet{
			sans: "PingFang SC", sansBold: "PingFang SC Semibold",
			sansItalic: "PingFang SC", sansBoldItalic: "PingFang SC Semibold",
			mono: "Menlo", monoBold: "Menlo Bold", monoItalic: "Menlo Italic", monoBoldItalic: "Menlo Bold Italic",
			emoji: "Apple Color Emoji",
		}
	case "windows":
		return nativeFontSet{
			sans: "Microsoft YaHei UI", sansBold: "Microsoft YaHei UI Bold",
			sansItalic: "Microsoft YaHei UI", sansBoldItalic: "Microsoft YaHei UI Bold",
			mono: "Microsoft YaHei UI", monoBold: "Microsoft YaHei UI Bold",
			monoItalic: "Microsoft YaHei UI", monoBoldItalic: "Microsoft YaHei UI Bold",
			emoji: "Segoe UI Emoji",
		}
	default:
		return nativeFontSet{
			sans: "Noto Sans CJK SC", sansBold: "Noto Sans CJK SC Medium",
			sansItalic: "Noto Sans CJK SC", sansBoldItalic: "Noto Sans CJK SC Medium",
			mono: "Noto Sans Mono CJK SC", monoBold: "Noto Sans Mono CJK SC Bold",
			monoItalic: "Noto Sans Mono CJK SC", monoBoldItalic: "Noto Sans Mono CJK SC Bold",
			emoji: "Noto Color Emoji",
		}
	}
}

// configureNativeFonts assigns Unicode-capable platform fonts to FLTK's
// standard slots before widgets are created. This keeps every native control,
// popup menu and custom-drawn table on the same CJK-capable font path.
func configureNativeFonts(goos string) {
	f := nativeFontsForOS(goos)
	fltk_bridge.SetFont(fltk_bridge.HELVETICA, f.sans)
	fltk_bridge.SetFont(fltk_bridge.HELVETICA_BOLD, f.sansBold)
	fltk_bridge.SetFont(fltk_bridge.HELVETICA_ITALIC, f.sansItalic)
	fltk_bridge.SetFont(fltk_bridge.HELVETICA_BOLD_ITALIC, f.sansBoldItalic)
	fltk_bridge.SetFont(fltk_bridge.COURIER, f.mono)
	fltk_bridge.SetFont(fltk_bridge.COURIER_BOLD, f.monoBold)
	fltk_bridge.SetFont(fltk_bridge.COURIER_ITALIC, f.monoItalic)
	fltk_bridge.SetFont(fltk_bridge.COURIER_BOLD_ITALIC, f.monoBoldItalic)
	fltk_bridge.SetFont(fltk_bridge.FREE_FONT, f.emoji)
}

func (a *finalShellApp) build() {
	const (
		winW   = defaultWindowWidth
		winH   = defaultWindowHeight
		margin = 22
		leftW  = 492
		gap    = 22
		rightX = margin + leftW + gap
		rightW = winW - rightX - margin
	)

	a.window = centeredWindow(winW, winH, tr("app.title"))
	if raw := a.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetColor(tokenColor(modernTheme.background))
		raw.SetSizeRange(1120, 720, 0, 0, 20, 20, false)
	}
	root := a.window.RootView()
	mainLayout := mainWindowLayoutFor(layoutRect{Width: winW, Height: winH}, nativeControls)

	a.mainTitle = titleLabel(mainLayout.Title.X, mainLayout.Title.Y, mainLayout.Title.Width, mainLayout.Title.Height, tr("app.title"))
	root.AddSubview(a.mainTitle)
	a.mainSubtitle = mutedLabel(mainLayout.Subtitle.X, mainLayout.Subtitle.Y, mainLayout.Subtitle.Width, mainLayout.Subtitle.Height, tr("app.subtitle"))
	root.AddSubview(a.mainSubtitle)
	a.managerButton = button(mainLayout.Manager.X, mainLayout.Manager.Y, mainLayout.Manager.Width, mainLayout.Manager.Height, tr("manager.open_compact"), "app.connection_manager", a.openConnectionManager)
	a.managerButton.View().SetAutomationName(tr("manager.open"))
	root.AddSubview(a.managerButton)
	a.settingsButton = button(mainLayout.Settings.X, mainLayout.Settings.Y, mainLayout.Settings.Width, mainLayout.Settings.Height, tr("setting.title"), "app.settings", a.openSettings)
	root.AddSubview(a.settingsButton)
	a.status = pillLabel(mainLayout.Status.X, mainLayout.Status.Y, mainLayout.Status.Width, mainLayout.Status.Height, tr("app.ready"))
	a.status.View().SetAutomationID("app.status")
	root.AddSubview(a.status)

	a.workspace = uikit.NewUISplitView(margin, 72, winW-margin*2, 786, uikit.SplitHorizontal)
	a.workspace.SetAutomationID("workspace.split")
	a.workspace.SetDividerSize(8)
	a.workspace.SetMinimumSizes(500, 568)
	a.workspace.SetDividerColors(
		uint(tokenColor(modernTheme.border)),
		uint(tokenColor(modernTheme.selected)),
		uint(tokenColor(modernTheme.primary)),
	)
	left := uikit.NewUIGroup(rect(margin, 72, leftW, 786))
	left.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	left.SetAutomationID("connections.panel")
	left.Raw().Resizable(left.Raw())
	quickPanel := uikit.NewUIGroup(rect(margin, 72, leftW, 786))
	quickPanel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	quickPanel.SetAutomationID("connections.quick_panel")
	quickPanel.Raw().Resizable(nil)
	a.quickPanel = quickPanel
	rightPanel := uikit.NewUIGroup(rect(rightX, 72, rightW, 786))
	rightPanel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	rightPanel.SetAutomationID("terminal.panel")
	// The terminal panel owns deterministic responsive geometry. FLTK's default
	// proportional scaling makes localized desktop buttons unreadable at 1120×720.
	rightPanel.Raw().Resizable(nil)
	a.terminalPanel = rightPanel
	quickLayout := quickPanelLayoutFor(layoutRect{X: margin, Y: 72, Width: leftW, Height: 786}, nativeControls)
	terminalLayout := terminalPanelLayoutFor(layoutRect{X: rightX, Y: 72, Width: rightW, Height: 786}, nativeControls)
	a.workspace.SetLeftView(left)
	a.workspace.SetRightView(rightPanel)
	// Establish the authored 1440x900 design geometry before pane controls are
	// attached. SplitView lays out empty pane content immediately; resetting this
	// baseline ensures the final persisted-ratio resize translates every absolute
	// FLTK child from the same coordinates used below.
	left.Raw().Resize(margin, 72, leftW, 786)
	rightPanel.Raw().Resize(rightX, 72, rightW, 786)
	root.AddSubview(a.workspace)
	a.quickTitle = sectionTitle(quickLayout.Title.X, quickLayout.Title.Y, quickLayout.Title.Width, quickLayout.Title.Height, tr("quick.title"))
	quickPanel.AddSubview(a.quickTitle)
	a.quickSubtitle = mutedLabel(quickLayout.Subtitle.X, quickLayout.Subtitle.Y, quickLayout.Subtitle.Width, quickLayout.Subtitle.Height, tr("quick.subtitle"))
	quickPanel.AddSubview(a.quickSubtitle)
	a.searchLabel = mutedLabel(quickLayout.SearchLabel.X, quickLayout.SearchLabel.Y, quickLayout.SearchLabel.Width, quickLayout.SearchLabel.Height, tr("connections.search"))
	quickPanel.AddSubview(a.searchLabel)
	a.searchInput = inputNoLabel(quickLayout.Search.X, quickLayout.Search.Y, quickLayout.Search.Width, quickLayout.Search.Height, "connections.search", tr("connections.search_placeholder"))
	a.searchInput.OnChange(a.jumpToSearchMatch)
	a.searchInput.View().On(fltk_bridge.KEYDOWN, func(fltk_bridge.Event) bool {
		return a.handleSearchKey(fltk_bridge.EventKey())
	})
	quickPanel.AddSubview(a.searchInput)
	a.findButton = button(quickLayout.Find.X, quickLayout.Find.Y, quickLayout.Find.Width, quickLayout.Find.Height, tr("connections.find"), "connections.find", a.jumpToSearchMatch)
	quickPanel.AddSubview(a.findButton)

	tv, err := uikit.NewUITableView(quickLayout.Table.X, quickLayout.Table.Y, quickLayout.Table.Width, quickLayout.Table.Height)
	if err == nil {
		a.table = tv
		a.table.SetHeaderHeight(nativeControls.TableHeaderHeight)
		a.table.SetDefaultRowHeight(nativeControls.TableRowHeight)
		a.table.View().SetAutomationID("connections.table").SetAutomationName(tr("app.connections"))
		a.table.AddColumn(tableview.TableColumn{Identifier: "favorite", Title: "★", Width: 36})
		a.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: tr("connections.group"), Width: 88})
		a.table.AddColumn(tableview.TableColumn{Identifier: "name", Title: tr("connections.name"), Width: 130})
		a.table.AddColumn(tableview.TableColumn{Identifier: "type", Title: tr("connections.type"), Width: 45})
		a.table.AddColumn(tableview.TableColumn{Identifier: "endpoint", Title: tr("connections.endpoint"), Width: 80})
		a.table.AddColumn(tableview.TableColumn{Identifier: "last", Title: tr("connections.last_used"), Width: 70})
		a.model = &tableModel{rows: a.rows, cellText: quickConnectionCellText}
		a.table.SetDataSource(a.model)
		a.table.SetDelegate(tableDelegate{onSelect: func(row int) {
			a.selectRow(row)
			if err := a.persistActiveRow(row); err != nil {
				gtbox_log.LogErrorf("save active connection failed: %s", err.Error())
				a.setStatus(tr("status.save_failed"))
				a.showTopNotice(tr("status.save_failed"), err.Error(), true)
			}
		}})
		a.table.OnActivate(a.activateConnectionRow)
		a.table.SetBackgroundColor(tokenColor(modernTheme.card))
		a.table.SetCustomDraw(a.drawConnectionCell)
		a.table.ReloadData()
		quickPanel.AddSubview(a.table)
	}

	a.summaryTitle = sectionTitle(quickLayout.SummaryTitle.X, quickLayout.SummaryTitle.Y, quickLayout.SummaryTitle.Width, quickLayout.SummaryTitle.Height, tr("connections.selected_summary"))
	quickPanel.AddSubview(a.summaryTitle)
	a.selectedName = label(quickLayout.SelectedName.X, quickLayout.SelectedName.Y, quickLayout.SelectedName.Width, quickLayout.SelectedName.Height, "")
	a.selectedName.SetFontSize(nativeTypography.SectionTitle)
	a.selectedName.View().SetAutomationID("connections.selected_name")
	quickPanel.AddSubview(a.selectedName)
	a.selectedDetail = mutedLabel(quickLayout.SelectedDetail.X, quickLayout.SelectedDetail.Y, quickLayout.SelectedDetail.Width, quickLayout.SelectedDetail.Height, "")
	a.selectedDetail.View().SetAutomationID("connections.selected_detail")
	quickPanel.AddSubview(a.selectedDetail)
	a.selectedRecent = mutedLabel(quickLayout.SelectedRecent.X, quickLayout.SelectedRecent.Y, quickLayout.SelectedRecent.Width, quickLayout.SelectedRecent.Height, "")
	a.selectedRecent.View().SetAutomationID("connections.selected_recent")
	quickPanel.AddSubview(a.selectedRecent)

	a.connectButton = primaryButton(quickLayout.Connect.X, quickLayout.Connect.Y, quickLayout.Connect.Width, quickLayout.Connect.Height, tr("action.connect"), "action.connect", a.connectSelected)
	quickPanel.AddSubview(a.connectButton)
	quickPanel.Raw().SetResizeHandler(a.layoutQuickPanel)
	left.AddSubview(quickPanel)
	a.buildMonitorSidebar(left, margin, leftW)

	rightPanel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	rightPanel.SetAutomationID("terminal.panel")
	a.terminalTitle = sectionTitle(terminalLayout.Title.X, terminalLayout.Title.Y, terminalLayout.Title.Width, terminalLayout.Title.Height, tr("terminal.title"))
	rightPanel.AddSubview(a.terminalTitle)
	a.terminalSubtitle = mutedLabel(terminalLayout.Subtitle.X, terminalLayout.Subtitle.Y, terminalLayout.Subtitle.Width, terminalLayout.Subtitle.Height, tr("terminal.subtitle"))
	rightPanel.AddSubview(a.terminalSubtitle)
	a.closeTabButton = button(terminalLayout.CloseTab.X, terminalLayout.CloseTab.Y, terminalLayout.CloseTab.Width, terminalLayout.CloseTab.Height, tr("session.close"), "terminal.session_close", a.closeActiveSession)
	rightPanel.AddSubview(a.closeTabButton)

	if a.sessions == nil {
		a.sessions = newSessionWorkspace()
	}
	a.sessionTabs = uikit.NewUITabView(rect(terminalLayout.Tabs.X, terminalLayout.Tabs.Y, terminalLayout.Tabs.Width, terminalLayout.Tabs.Height))
	a.sessionTabs.SetAutomationID("terminal.sessions")
	a.sessionTabs.SetStyle(tabview.Style{
		BarBackground:     uint(tokenColor(modernTheme.elevated)),
		ContentBackground: uint(tokenColor(modernTheme.card)),
		NormalText:        uint(tokenColor(modernTheme.muted)),
		ActiveText:        uint(tokenColor(modernTheme.foreground)),
		Indicator:         uint(tokenColor(modernTheme.primary)),
		FontSize:          nativeTypography.Body,
	})
	rightPanel.AddSubview(a.sessionTabs)
	activeSessionIndex := a.sessions.ActiveIndex()
	for _, state := range a.sessions.Tabs() {
		a.sessionTabs.AddTabWithID(state.ID, sessionTabTitle(state), nil)
	}
	if activeSessionIndex >= 0 {
		a.sessionTabs.SelectTab(activeSessionIndex)
		a.sessions.Select(activeSessionIndex)
	}
	a.sessionTabs.OnTabChanged(a.selectSessionTab)

	a.output = uikit.NewUITerminalView(rect(terminalLayout.Output.X, terminalLayout.Output.Y, terminalLayout.Output.Width, terminalLayout.Output.Height))
	a.output.SetAutomationID("terminal.output").SetAutomationName(tr("terminal.output_name"))
	// Non-interactive SSH command output keeps a discoverable horizontal
	// scrollbar. Active PTY tabs switch this off and receive fitted-grid resize
	// events through the reusable terminal view.
	a.output.Raw().SetHorizontalScrollbar(fltk_bridge.TerminalScrollbarAuto)
	a.output.SetFont(fltk_bridge.COURIER)
	a.output.SetFontSize(nativeTypography.Terminal)
	a.output.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	a.output.SetBackgroundColor(uint(tokenColor(modernTheme.terminal)))
	a.output.SetSelectionColors(uint(tokenColor(modernTheme.foreground)), uint(tokenColor(modernTheme.selected)))
	a.output.SetMargins(nativeControls.TextInset, nativeControls.TextInset, nativeControls.TextInset, nativeControls.TextInset)
	a.output.SetHistoryRows(4000)
	a.output.SetRedrawRate(0.016)
	a.output.OnInput(a.writeActiveTerminalInput)
	a.output.OnResize(a.terminalViewResized)
	a.setTerminalOutput(terminalWelcomeText())
	if _, ok := a.sessions.Active(); ok {
		a.renderActiveSession()
	} else {
		a.appendRecentHistory()
	}
	rightPanel.AddSubview(a.output)

	a.historyButton = button(terminalLayout.History.X, terminalLayout.History.Y, terminalLayout.History.Width, terminalLayout.History.Height, tr("terminal.history_compact"), "terminal.history", a.showSelectedHistory)
	a.historyButton.View().SetAutomationName(tr("terminal.history"))
	rightPanel.AddSubview(a.historyButton)
	a.lastButton = button(terminalLayout.Last.X, terminalLayout.Last.Y, terminalLayout.Last.Width, terminalLayout.Last.Height, tr("terminal.last_compact"), "terminal.last_command", a.recallLastCommand)
	a.lastButton.View().SetAutomationName(tr("terminal.last"))
	rightPanel.AddSubview(a.lastButton)
	a.clearButton = button(terminalLayout.Clear.X, terminalLayout.Clear.Y, terminalLayout.Clear.Width, terminalLayout.Clear.Height, tr("terminal.clear_compact"), "terminal.clear", a.clearTerminalOutput)
	a.clearButton.View().SetAutomationName(tr("terminal.clear"))
	rightPanel.AddSubview(a.clearButton)
	a.commandLabel = mutedLabel(terminalLayout.CommandLabel.X, terminalLayout.CommandLabel.Y, terminalLayout.CommandLabel.Width, terminalLayout.CommandLabel.Height, tr("terminal.command"))
	rightPanel.AddSubview(a.commandLabel)
	a.cmdInput = uikit.NewUITextView(rect(terminalLayout.Command.X, terminalLayout.Command.Y, terminalLayout.Command.Width, terminalLayout.Command.Height))
	a.cmdInput.SetAutomationID("terminal.command").SetAutomationName(tr("terminal.command"))
	a.cmdInput.SetWrapNone()
	styleCommandInput(a.cmdInput)
	a.cmdInput.SetFallbackFont(fltk_bridge.FREE_FONT, isEmojiRune)
	a.cmdInput.OnKey(func(event uikit.KeyEvent) bool {
		if event.Key != fltk_bridge.ENTER_KEY {
			return false
		}
		a.runCommand()
		return true
	})
	rightPanel.AddSubview(a.cmdInput)
	a.stopButton = button(terminalLayout.Stop.X, terminalLayout.Stop.Y, terminalLayout.Stop.Width, terminalLayout.Stop.Height, tr("terminal.stop"), "terminal.stop", a.stopCommand)
	rightPanel.AddSubview(a.stopButton)
	a.runButton = primaryButton(terminalLayout.Run.X, terminalLayout.Run.Y, terminalLayout.Run.Width, terminalLayout.Run.Height, tr("terminal.run"), "terminal.run", a.runCommand)
	rightPanel.AddSubview(a.runButton)
	rightPanel.Raw().SetResizeHandler(a.layoutTerminalPanel)
	// Apply the persisted ratio only after pane controls are attached. FLTK uses
	// absolute child coordinates, so this final native resize translates and
	// reflows the complete pane hierarchy together (including language rebuilds).
	if config.GlobalConfig != nil && !config.GlobalConfig.ResetWorkspaceOnStart {
		a.workspace.SetPosition(config.GlobalConfig.WorkspaceSplitRatio)
	} else {
		a.workspace.SetPosition(config.WorkspaceSplitRatioDefault)
	}
	a.workspace.OnPositionChanged(a.workspacePositionChanged)
	a.updateCommandControls()
	a.window.Raw().SetResizeHandler(a.layoutMainWindow)

	a.window.Show()
	if len(a.rows) > 0 {
		a.table.SelectRow(activeConnectionIndex(a.rows))
	}
}

func (a *finalShellApp) handleSearchKey(key int) bool {
	if a == nil || a.searchInput == nil {
		return false
	}
	switch key {
	case fltk_bridge.ENTER_KEY:
		a.activateConnectionRow(a.idx)
		return true
	case fltk_bridge.DOWN:
		return a.moveSearchSelection(1)
	case fltk_bridge.UP:
		return a.moveSearchSelection(-1)
	case fltk_bridge.ESCAPE:
		if strings.TrimSpace(a.searchInput.Text()) == "" {
			return false
		}
		a.searchInput.SetText("")
		a.jumpToSearchMatch()
		return true
	}
	return false
}

func nativeWindowClass() string {
	if value := strings.TrimSpace(os.Getenv("NBTERMINAL_WM_CLASS")); value != "" {
		return value
	}
	return "NBTerminal"
}

func (a *finalShellApp) workspacePositionChanged(change uikit.SplitPositionChange) {
	if change.Reason != uikit.SplitChangeDrag || config.GlobalConfig == nil || config.CurrentApp == nil {
		return
	}
	previous := config.GlobalConfig.WorkspaceSplitRatio
	config.GlobalConfig.WorkspaceSplitRatio = change.Geometry.Ratio
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		config.GlobalConfig.WorkspaceSplitRatio = previous
		gtbox_log.LogErrorf("save workspace split ratio failed: %s", err.Error())
		a.showTopNotice(tr("notice.failed.title"), err.Error(), true)
	}
}

func (a *finalShellApp) changeLanguage(index int) {
	languages := locales.SupportedLanguages()
	if index < 0 || index >= len(languages) {
		return
	}
	if a.anyCommandRunning() {
		a.setStatus(tr("status.command_active"))
		return
	}
	lang := languages[index]
	if lang == locales.CurrentLanguage() {
		return
	}
	if config.GlobalConfig == nil || config.CurrentApp == nil {
		return
	}
	previous := config.GlobalConfig.Language
	config.GlobalConfig.Language = lang.LanguageTag()
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		config.GlobalConfig.Language = previous
		a.showTopNotice(tr("notice.failed.title"), err.Error(), true)
		return
	}
	locales.ResetLocaleLanguage(lang.LanguageTag())

	a.rebuildForLanguage(lang)
}

func activeConnectionIndex(rows []connectionProfile) int {
	if len(rows) == 0 {
		return -1
	}
	activeID := ""
	if config.GlobalConfig != nil {
		if config.GlobalConfig.StartWithFirstConnection {
			return 0
		}
		activeID = strings.TrimSpace(config.GlobalConfig.ActiveConnectionID)
	}
	if activeID != "" {
		for i, row := range rows {
			if row.ID == activeID {
				return i
			}
		}
	}
	return 0
}

type tableDelegate struct{ onSelect func(int) }

func (d tableDelegate) DidSelectRow(_ *tableview.TableView, row int) {
	if d.onSelect != nil {
		d.onSelect(row)
	}
}
func (d tableDelegate) RowHeight(_ *tableview.TableView, _ int) int {
	return nativeControls.TableRowHeight
}

func (a *finalShellApp) drawConnectionCell(ctx fltk_bridge.TableContext, row, col, x, y, w, h int) {
	switch ctx {
	case fltk_bridge.ContextTable:
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.card))
		fltk_bridge.PopClip()
	case fltk_bridge.ContextColHeader:
		titles := []string{"★", tr("connections.group"), tr("connections.name"), tr("connections.type"), tr("connections.endpoint"), tr("connections.last_used")}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.elevated))
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.foreground))
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, nativeTypography.Body)
		if col >= 0 && col < len(titles) {
			fltk_bridge.Draw(titles[col], x+nativeControls.TextInset, y, w-nativeControls.TextInset*2, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		}
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.border))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	case fltk_bridge.ContextCell:
		if row < 0 || row >= len(a.rows) {
			return
		}
		bg := tokenColor(modernTheme.card)
		fg := tokenColor(modernTheme.foreground)
		if row == a.idx {
			bg = tokenColor(modernTheme.selected)
			fg = tokenColor(modernTheme.foreground)
		} else if row%2 == 1 {
			bg = tokenColor(modernTheme.elevated)
		}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, bg)
		fltk_bridge.SetDrawColor(fg)
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, nativeTypography.Body)
		fltk_bridge.Draw(a.connectionCellText(row, col), x+nativeControls.TextInset, y, w-nativeControls.TextInset*2, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.border))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	}
}

func (a *finalShellApp) connectionCellText(row, col int) string {
	if row < 0 || row >= len(a.rows) {
		return ""
	}
	return quickConnectionCellText(a.rows[row], col)
}

func quickConnectionCellText(p connectionProfile, col int) string {
	switch col {
	case 0:
		if p.Favorite {
			return "★"
		}
		return ""
	case 1:
		return compactConnectionGroup(p.Group)
	case 2:
		return p.Name
	case 3:
		return string(p.Type)
	case 4:
		return p.tableEndpoint()
	case 5:
		return formatLastUsedCompact(p.LastUsed)
	default:
		return ""
	}
}

func rect(x, y, w, h int) *foundation.Rect { return &foundation.Rect{X: x, Y: y, Width: w, Height: h} }

type commandBarSpec struct {
	history       *foundation.Rect
	last          *foundation.Rect
	clear         *foundation.Rect
	command       *foundation.Rect
	stop          *foundation.Rect
	run           *foundation.Rect
	commandLabelY int
}

func commandBarLayout(rightX, rightW int) commandBarSpec {
	layout := terminalPanelLayoutFor(layoutRect{X: rightX, Y: 72, Width: rightW, Height: 786}, nativeControls)
	return commandBarSpec{
		history:       rect(layout.History.X, layout.History.Y, layout.History.Width, layout.History.Height),
		last:          rect(layout.Last.X, layout.Last.Y, layout.Last.Width, layout.Last.Height),
		clear:         rect(layout.Clear.X, layout.Clear.Y, layout.Clear.Width, layout.Clear.Height),
		command:       rect(layout.Command.X, layout.Command.Y, layout.Command.Width, layout.Command.Height),
		stop:          rect(layout.Stop.X, layout.Stop.Y, layout.Stop.Width, layout.Stop.Height),
		run:           rect(layout.Run.X, layout.Run.Y, layout.Run.Width, layout.Run.Height),
		commandLabelY: layout.CommandLabel.Y,
	}
}

func (a *finalShellApp) layoutMainWindow() {
	if a == nil || a.window == nil || a.window.Raw() == nil {
		return
	}
	raw := a.window.Raw()
	layout := mainWindowLayoutFor(layoutRect{Width: raw.W(), Height: raw.H()}, nativeControls)
	if a.mainTitle != nil && a.mainTitle.Raw() != nil {
		a.mainTitle.Raw().Resize(layout.Title.X, layout.Title.Y, layout.Title.Width, layout.Title.Height)
	}
	if a.mainSubtitle != nil && a.mainSubtitle.Raw() != nil {
		a.mainSubtitle.Raw().Resize(layout.Subtitle.X, layout.Subtitle.Y, layout.Subtitle.Width, layout.Subtitle.Height)
	}
	if a.managerButton != nil && a.managerButton.Raw() != nil {
		a.managerButton.Raw().Resize(layout.Manager.X, layout.Manager.Y, layout.Manager.Width, layout.Manager.Height)
	}
	if a.settingsButton != nil && a.settingsButton.Raw() != nil {
		a.settingsButton.Raw().Resize(layout.Settings.X, layout.Settings.Y, layout.Settings.Width, layout.Settings.Height)
	}
	if a.status != nil && a.status.Raw() != nil {
		a.status.Raw().Resize(layout.Status.X, layout.Status.Y, layout.Status.Width, layout.Status.Height)
	}
	if a.workspace != nil && a.workspace.View() != nil {
		resizeNativeWidget(a.workspace.View().Raw(), layout.Workspace)
	}
	raw.Redraw()
}

func (a *finalShellApp) layoutQuickPanel() {
	if a == nil || a.quickPanel == nil || a.quickPanel.Raw() == nil {
		return
	}
	raw := a.quickPanel.Raw()
	layout := quickPanelLayoutFor(layoutRect{X: raw.X(), Y: raw.Y(), Width: raw.W(), Height: raw.H()}, nativeControls)
	if a.quickTitle != nil && a.quickTitle.Raw() != nil {
		a.quickTitle.Raw().Resize(layout.Title.X, layout.Title.Y, layout.Title.Width, layout.Title.Height)
	}
	if a.quickSubtitle != nil && a.quickSubtitle.Raw() != nil {
		a.quickSubtitle.Raw().Resize(layout.Subtitle.X, layout.Subtitle.Y, layout.Subtitle.Width, layout.Subtitle.Height)
	}
	if a.searchLabel != nil && a.searchLabel.Raw() != nil {
		a.searchLabel.Raw().Resize(layout.SearchLabel.X, layout.SearchLabel.Y, layout.SearchLabel.Width, layout.SearchLabel.Height)
	}
	if a.searchInput != nil && a.searchInput.Raw() != nil {
		resizeNativeWidget(a.searchInput.Raw(), layout.Search)
	}
	if a.findButton != nil && a.findButton.Raw() != nil {
		a.findButton.Raw().Resize(layout.Find.X, layout.Find.Y, layout.Find.Width, layout.Find.Height)
	}
	if a.table != nil && a.table.Raw() != nil {
		resizeNativeWidget(a.table.Raw(), layout.Table)
	}
	if a.summaryTitle != nil && a.summaryTitle.Raw() != nil {
		a.summaryTitle.Raw().Resize(layout.SummaryTitle.X, layout.SummaryTitle.Y, layout.SummaryTitle.Width, layout.SummaryTitle.Height)
	}
	if a.selectedName != nil && a.selectedName.Raw() != nil {
		a.selectedName.Raw().Resize(layout.SelectedName.X, layout.SelectedName.Y, layout.SelectedName.Width, layout.SelectedName.Height)
	}
	if a.selectedDetail != nil && a.selectedDetail.Raw() != nil {
		a.selectedDetail.Raw().Resize(layout.SelectedDetail.X, layout.SelectedDetail.Y, layout.SelectedDetail.Width, layout.SelectedDetail.Height)
	}
	if a.selectedRecent != nil && a.selectedRecent.Raw() != nil {
		a.selectedRecent.Raw().Resize(layout.SelectedRecent.X, layout.SelectedRecent.Y, layout.SelectedRecent.Width, layout.SelectedRecent.Height)
	}
	if a.connectButton != nil && a.connectButton.Raw() != nil {
		a.connectButton.Raw().Resize(layout.Connect.X, layout.Connect.Y, layout.Connect.Width, layout.Connect.Height)
	}
	raw.Redraw()
}

func resizeNativeWidget(raw any, frame layoutRect) {
	if widget, ok := raw.(interface{ Resize(int, int, int, int) }); ok {
		widget.Resize(frame.X, frame.Y, frame.Width, frame.Height)
	}
}

func (a *finalShellApp) layoutTerminalPanel() {
	if a == nil || a.terminalPanel == nil || a.terminalPanel.Raw() == nil {
		return
	}
	raw := a.terminalPanel.Raw()
	layout := terminalPanelLayoutFor(layoutRect{X: raw.X(), Y: raw.Y(), Width: raw.W(), Height: raw.H()}, nativeControls)
	if a.terminalTitle != nil && a.terminalTitle.Raw() != nil {
		a.terminalTitle.Raw().Resize(layout.Title.X, layout.Title.Y, layout.Title.Width, layout.Title.Height)
	}
	if a.terminalSubtitle != nil && a.terminalSubtitle.Raw() != nil {
		a.terminalSubtitle.Raw().Resize(layout.Subtitle.X, layout.Subtitle.Y, layout.Subtitle.Width, layout.Subtitle.Height)
	}
	if a.closeTabButton != nil && a.closeTabButton.Raw() != nil {
		a.closeTabButton.Raw().Resize(layout.CloseTab.X, layout.CloseTab.Y, layout.CloseTab.Width, layout.CloseTab.Height)
	}
	if a.sessionTabs != nil && a.sessionTabs.Raw() != nil {
		a.sessionTabs.Raw().Resize(layout.Tabs.X, layout.Tabs.Y, layout.Tabs.Width, layout.Tabs.Height)
	}
	if a.output != nil && a.output.Raw() != nil {
		a.output.Raw().Resize(layout.Output.X, layout.Output.Y, layout.Output.Width, layout.Output.Height)
	}
	if a.historyButton != nil && a.historyButton.Raw() != nil {
		a.historyButton.Raw().Resize(layout.History.X, layout.History.Y, layout.History.Width, layout.History.Height)
	}
	if a.lastButton != nil && a.lastButton.Raw() != nil {
		a.lastButton.Raw().Resize(layout.Last.X, layout.Last.Y, layout.Last.Width, layout.Last.Height)
	}
	if a.clearButton != nil && a.clearButton.Raw() != nil {
		a.clearButton.Raw().Resize(layout.Clear.X, layout.Clear.Y, layout.Clear.Width, layout.Clear.Height)
	}
	if a.commandLabel != nil && a.commandLabel.Raw() != nil {
		a.commandLabel.Raw().Resize(layout.CommandLabel.X, layout.CommandLabel.Y, layout.CommandLabel.Width, layout.CommandLabel.Height)
	}
	if a.cmdInput != nil && a.cmdInput.Raw() != nil {
		a.cmdInput.Raw().Resize(layout.Command.X, layout.Command.Y, layout.Command.Width, layout.Command.Height)
	}
	if a.stopButton != nil && a.stopButton.Raw() != nil {
		a.stopButton.Raw().Resize(layout.Stop.X, layout.Stop.Y, layout.Stop.Width, layout.Stop.Height)
	}
	if a.runButton != nil && a.runButton.Raw() != nil {
		a.runButton.Raw().Resize(layout.Run.X, layout.Run.Y, layout.Run.Width, layout.Run.Height)
	}
	raw.Redraw()
}

func centeredWindow(w, h int, title string) *uikit.UIWindow {
	return uikit.NewWindowWithRect(centeredScreenRect(w, h), title)
}

func centeredScreenRect(w, h int) *foundation.Rect {
	s := screen.GetScreenSize()
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		s = &screen.ScreenSize{Width: defaultWindowWidth, Height: defaultWindowHeight}
	}
	return centerRectInBounds(s.Width, s.Height, w, h)
}

func centerRectInBounds(screenW, screenH, w, h int) *foundation.Rect {
	x := (screenW - w) / 2
	y := (screenH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return rect(x, y, w, h)
}

func topFloatRectInBounds(screenW, screenH, parentX, parentY, parentW, w, h int) *foundation.Rect {
	x := parentX + (parentW-w)/2
	y := parentY + noticeTopOffset
	if maxX := screenW - w - screenEdgePadding; x > maxX {
		x = maxX
	}
	if maxY := screenH - h - screenEdgePadding; y > maxY {
		y = maxY
	}
	if x < screenEdgePadding {
		x = screenEdgePadding
	}
	if y < screenEdgePadding {
		y = screenEdgePadding
	}
	return rect(x, y, w, h)
}

func (a *finalShellApp) topFloatRect(w, h int) *foundation.Rect {
	if a == nil || a.window == nil || a.window.Raw() == nil {
		return centeredScreenRect(w, h)
	}
	raw := a.window.Raw()
	s := screen.GetScreenSize()
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		s = &screen.ScreenSize{Width: defaultWindowWidth, Height: defaultWindowHeight}
	}
	return topFloatRectInBounds(s.Width, s.Height, raw.XRoot(), raw.YRoot(), raw.W(), w, h)
}

func tr(messageID string) string { return locales.T(messageID) }

func trf(messageID string, args ...any) string { return fmt.Sprintf(tr(messageID), args...) }

func isEmojiRune(r rune) bool {
	switch {
	case r == '\u200d' || r == '\u20e3' || r == '\ufe0e' || r == '\ufe0f':
		return true
	case r == '\u00a9' || r == '\u00ae' || r == '\u203c' || r == '\u2049' || r == '\u2122' || r == '\u2139':
		return true
	case r >= '\u2300' && r <= '\u23ff':
		return true
	case r >= '\u2600' && r <= '\u27bf':
		return true
	case r >= '\u2b00' && r <= '\u2bff':
		return true
	case r >= '\U0001f000' && r <= '\U0001faff':
		return true
	default:
		return false
	}
}

func themeColor(r, g, b uint8) fltk_bridge.Color { return fltk_bridge.ColorFromRgb(r, g, b) }

func label(x, y, w, h int, text string) *uikit.UILabel {
	l := uikit.NewUILabel(rect(x, y, w, h), text)
	l.SetFontSize(nativeTypography.Body)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	l.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_CLIP)
	return l
}

func titleLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(nativeTypography.WindowTitle)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	return l
}

func sectionTitle(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(nativeTypography.SectionTitle)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	return l
}

func mutedLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(nativeTypography.Supporting)
	l.SetTextColor(uint(tokenColor(modernTheme.muted)))
	return l
}

// styleDynamicLabel gives frequently changing native labels an opaque backing
// surface so shorter replacement text cannot leave stale glyphs behind.
func styleDynamicLabel(l *uikit.UILabel) {
	if l == nil {
		return
	}
	l.SetFrame(fltk_bridge.FLAT_BOX)
	l.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
}

func pillLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFrame(fltk_bridge.RFLAT_BOX)
	l.SetBackgroundColor(uint(tokenColor(modernTheme.elevated)))
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	l.SetAlignment(fltk_bridge.ALIGN_RIGHT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_CLIP)
	return l
}

func input(x, y, w, h int, placeholder, id string) *uikit.Input {
	in := uikit.NewInput(x, y, w, h, placeholder)
	styleInput(in)
	in.View().SetAutomationID(id).SetAutomationName(placeholder)
	return in
}

func inputNoLabel(x, y, w, h int, id, name string) *uikit.Input {
	in := uikit.NewInput(x, y, w, h, "")
	styleInput(in)
	in.View().SetAutomationID(id).SetAutomationName(name)
	return in
}

func styleInput(in *uikit.Input) {
	if in == nil {
		return
	}
	// fltk2go styles the editable glyphs and native caret rather than
	// only the external FLTK label. Inputs can therefore share the semantic dark
	// surface without sacrificing UTF-8 text or keyboard-focus visibility.
	in.SetFont(fltk_bridge.HELVETICA)
	in.SetFontSize(nativeTypography.Body)
	in.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	in.SetCursorColor(uint(tokenColor(modernTheme.primary)))
	in.SetBackgroundColor(uint(tokenColor(modernTheme.elevated)))
}

func styleCommandInput(in *uikit.UITextView) {
	if in == nil {
		return
	}
	in.SetFont(fltk_bridge.COURIER)
	in.SetFontSize(nativeTypography.Terminal)
	in.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	in.SetCursorColor(uint(tokenColor(modernTheme.primary)))
	in.SetSelectionColor(uint(tokenColor(modernTheme.selected)))
	in.SetBackgroundColor(uint(tokenColor(modernTheme.elevated)))
}

func button(x, y, w, h int, title, id string, cb func()) *uikit.UIButton {
	b := uikit.NewUIButton(rect(x, y, w, h), title)
	styleButton(b, false)
	b.View().SetAutomationID(id).SetAutomationName(title)
	b.OnTouchUpInside(cb)
	return b
}

func primaryButton(x, y, w, h int, title, id string, cb func()) *uikit.UIButton {
	b := uikit.NewUIButton(rect(x, y, w, h), title)
	styleButton(b, true)
	b.View().SetAutomationID(id).SetAutomationName(title)
	b.OnTouchUpInside(cb)
	return b
}

func styleButton(b *uikit.UIButton, primary bool) {
	if b == nil {
		return
	}
	if raw := b.Raw(); raw != nil {
		raw.SetBox(fltk_bridge.RFLAT_BOX)
		raw.SetDownBox(fltk_bridge.RSHADOW_BOX)
		raw.SetLabelSize(nativeTypography.Body)
	}
	if primary {
		b.SetBackgroundColor(uint(tokenColor(modernTheme.primary)))
		b.SetTitleColor(uint(tokenColor(modernTheme.primaryText)))
		return
	}
	b.SetBackgroundColor(uint(tokenColor(modernTheme.elevated)))
	b.SetTitleColor(uint(tokenColor(modernTheme.foreground)))
}

func (a *finalShellApp) showTopNotice(title, message string, critical bool) {
	if a == nil {
		return
	}
	if a.notice != nil && a.notice.Raw() != nil {
		a.notice.Raw().Hide()
	}
	const w, h = noticeWidth, noticeHeight
	win := uikit.NewWindowWithRect(a.topFloatRect(w, h), title)
	if raw := win.Raw(); raw != nil {
		raw.SetNonModal()
		if critical {
			raw.SetColor(themeColor(254, 242, 242))
		} else {
			raw.SetColor(themeColor(239, 246, 255))
		}
	}
	heading := sectionTitle(18, 16, w-36, 24, title)
	body := mutedLabel(18, 46, w-36, 54, message)
	body.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_WRAP)
	if critical {
		heading.SetTextColor(uint(themeColor(153, 27, 27)))
		body.SetTextColor(uint(themeColor(127, 29, 29)))
	}
	win.RootView().AddSubview(heading)
	win.RootView().AddSubview(body)
	a.notice = win
	win.Show()
	go func(expected *uikit.UIWindow) {
		time.Sleep(3500 * time.Millisecond)
		fltk_bridge.Awake(func() {
			if a.notice == expected && expected != nil && expected.Raw() != nil {
				expected.Raw().Hide()
				a.notice = nil
			}
		})
	}(win)
}

func (a *finalShellApp) jumpToSearchMatch() {
	if a == nil || a.searchInput == nil {
		return
	}
	query := strings.TrimSpace(a.searchInput.Text())
	a.refreshNavigator("")
	a.refreshTable()
	if len(a.rows) == 0 {
		if query == "" {
			a.setStatus(tr("connections.none"))
		} else {
			a.setStatus(trf("connections.no_match", query))
		}
		return
	}
	if a.table != nil {
		a.table.SelectRow(0)
	} else {
		a.selectRow(0)
	}
	if query == "" {
		a.setStatus(trf("connections.showing", len(a.rows)))
		return
	}
	a.setStatus(trf("connections.matched", len(a.rows), a.rows[0].Name))
}

func moveNavigatorSelection(current, count, delta int) int {
	if count <= 0 {
		return -1
	}
	if current < 0 {
		return 0
	}
	next := current + delta
	if next < 0 {
		return 0
	}
	if next >= count {
		return count - 1
	}
	return next
}

func (a *finalShellApp) moveSearchSelection(delta int) bool {
	if a == nil {
		return false
	}
	next := moveNavigatorSelection(a.idx, len(a.rows), delta)
	if next < 0 {
		return false
	}
	if a.table != nil {
		return a.table.SelectRow(next)
	}
	a.selectRow(next)
	return true
}

// refreshNavigator rebuilds the main-window favorite/recent projection from the
// complete saved-profile model. Editors and the Connection Manager must call
// this instead of publishing their full rows into the compact launch surface.
func (a *finalShellApp) refreshNavigator(preferredID string) {
	if a == nil {
		return
	}
	query := ""
	if a.searchInput != nil {
		query = strings.TrimSpace(a.searchInput.Text())
	}
	a.rows = navigatorRows(a.allRows, query, quickConnectionLimit)
	a.idx = indexProfileByID(a.rows, preferredID)
	if a.idx < 0 {
		a.idx = activeConnectionIndex(a.rows)
	}
}

func filterConnections(rows []connectionProfile, query string) []connectionProfile {
	query = strings.TrimSpace(query)
	if query == "" {
		return append([]connectionProfile(nil), rows...)
	}
	out := make([]connectionProfile, 0, len(rows))
	for _, row := range rows {
		if connectionMatchesQuery(row, query) {
			out = append(out, row)
		}
	}
	return out
}

const quickConnectionLimit = 12

// navigatorRows keeps the terminal workspace focused: an empty query shows a
// compact favorite/recent projection, while explicit search reaches every saved
// profile. The independent Connection Manager remains the full edit surface.
func navigatorRows(rows []connectionProfile, query string, limit int) []connectionProfile {
	if strings.TrimSpace(query) != "" {
		return filterConnections(rows, query)
	}
	return quickConnectionProjection(rows, limit)
}

func quickConnectionProjection(rows []connectionProfile, limit int) []connectionProfile {
	out := append([]connectionProfile(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Favorite != out[j].Favorite {
			return out[i].Favorite
		}
		leftTime, leftOK := parseLastUsed(out[i].LastUsed)
		rightTime, rightOK := parseLastUsed(out[j].LastUsed)
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func toggledFavorite(profile connectionProfile) connectionProfile {
	profile.Favorite = !profile.Favorite
	return profile
}

func connectionMatchesQuery(p connectionProfile, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		p.Group,
		p.Name,
		string(p.Type),
		p.Host,
		p.Username,
		p.endpoint(),
		p.tableEndpoint(),
		p.Description,
	}, "\n"))
	return strings.Contains(haystack, query)
}

func indexProfileByID(rows []connectionProfile, id string) int {
	for i, row := range rows {
		if row.ID == id {
			return i
		}
	}
	return -1
}

func upsertProfile(rows []connectionProfile, p connectionProfile) []connectionProfile {
	if i := indexProfileByID(rows, p.ID); i >= 0 {
		rows[i] = p
		return rows
	}
	return append(rows, p)
}

func removeProfileByID(rows []connectionProfile, id string) []connectionProfile {
	if i := indexProfileByID(rows, id); i >= 0 {
		return append(rows[:i], rows[i+1:]...)
	}
	return rows
}

func (a *finalShellApp) selectRow(row int) {
	if row < 0 || row >= len(a.rows) {
		return
	}
	a.idx = row
	p := a.rows[row]
	if a.sessions != nil {
		a.sessions.SetProfileSelection(p.ID)
	}
	a.updateSelectedSummary()
	a.setStatus(trf("status.selected", p.Name))
	if a.table != nil {
		a.table.ReloadData()
	}
}

func (a *finalShellApp) updateSelectedSummary() {
	// These labels sit on the same opaque quick-panel surface. Redrawing that
	// bounded native region after text changes clears shorter prior values without
	// giving every label its own box edge at the SplitView clip boundary.
	if a != nil && a.quickPanel != nil && a.quickPanel.Raw() != nil {
		defer a.quickPanel.Raw().Redraw()
	}
	if a == nil || a.idx < 0 || a.idx >= len(a.rows) {
		if a != nil && a.selectedName != nil {
			a.selectedName.SetText(tr("connections.none"))
			if a.selectedDetail != nil {
				a.selectedDetail.SetText("")
			}
			if a.selectedRecent != nil {
				a.selectedRecent.SetText("")
			}
		}
		return
	}
	p := a.rows[a.idx]
	if a.selectedName != nil {
		a.selectedName.SetText(p.Name)
	}
	if a.selectedDetail != nil {
		a.selectedDetail.SetText(trf("connections.selected_detail_format", p.Group, strings.ToUpper(string(p.Type)), p.tableEndpoint()))
	}
	if a.selectedRecent != nil {
		a.selectedRecent.SetText(trf("connections.selected_recent_format", formatLastUsed(p.LastUsed)))
	}
}

func (a *finalShellApp) persistActiveRow(row int) error {
	if a == nil || a.store == nil || row < 0 || row >= len(a.rows) {
		return nil
	}
	return a.store.SetActive(a.rows[row].ID)
}

func newConnectionProfile(username string) connectionProfile {
	return connectionProfile{
		ID: fmt.Sprintf("conn-%d", time.Now().UnixNano()), Name: tr("profile.new_ssh"),
		Group: tr("profile.default_group"), Type: connectionTypeSSH, Port: 22, Username: username,
	}
}

func (a *finalShellApp) deleteProfile() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	// FLTK choice dialogs run their own native event loop. Opening one inside the
	// button-release callback can consume that same release as a dialog choice.
	// Defer to the next GUI turn so deletion always requires a second, explicit
	// action in the confirmation window.
	fltk_bridge.AddTimeout(0, func() {
		a.prepareProfileDeletion(profile, func() {
			if showConnectionDeleteConfirmation(profile) {
				a.deleteProfileConfirmed(profile)
			}
		})
	})
}

// prepareProfileDeletion prevents a non-modal editor from resurrecting the
// profile after deletion. The editor's own unsaved-draft policy remains
// authoritative: Keep Editing cancels deletion, while a clean close or explicit
// Discard advances to the destructive confirmation on a later GUI turn.
func (a *finalShellApp) prepareProfileDeletion(profile connectionProfile, confirm func()) {
	if a == nil || confirm == nil {
		return
	}
	if a.editor != nil && a.editor.window != nil && !a.editor.window.IsClosed() &&
		a.editor.queueAfterCloseForProfile(profile.ID, confirm) {
		a.editor.window.RequestClose()
		return
	}
	confirm()
}

func (a *finalShellApp) deleteProfileConfirmed(expected connectionProfile) {
	if !canDeleteSelectedProfile(a.rows, a.idx, expected.ID) {
		return
	}
	removed, err := a.removeSelectedProfile()
	if err != nil {
		gtbox_log.LogErrorf("delete connection failed: %s", err.Error())
		a.appendOutput(trf("output.delete_failed", err.Error()))
		a.setStatus(tr("status.delete_failed"))
		a.showTopNotice(tr("status.delete_failed"), err.Error(), true)
		return
	}
	if removed.ID == "" {
		return
	}
	a.refreshTable()
	if a.idx >= 0 {
		a.selectRow(a.idx)
	} else {
		a.updateSelectedSummary()
	}
	a.setStatus(trf("status.deleted", removed.Name))
}

// removeSelectedProfile is the transactional state transition behind Delete.
// Persist first, then publish the new rows to the UI so a disk failure cannot
// produce a false success that reappears after restart.
func (a *finalShellApp) removeSelectedProfile() (connectionProfile, error) {
	if a == nil || a.store == nil || a.idx < 0 || a.idx >= len(a.rows) {
		return connectionProfile{}, nil
	}
	removed := a.rows[a.idx]
	nextAll := removeProfileByID(append([]connectionProfile(nil), a.allRows...), removed.ID)
	nextRows := removeProfileByID(append([]connectionProfile(nil), a.rows...), removed.ID)
	nextIdx := a.idx
	if nextIdx >= len(nextRows) {
		nextIdx = len(nextRows) - 1
	}
	activeID := ""
	if nextIdx >= 0 {
		activeID = nextRows[nextIdx].ID
	}
	if err := a.store.SaveActive(nextAll, activeID); err != nil {
		return connectionProfile{}, err
	}
	a.allRows = nextAll
	a.rows = nextRows
	a.idx = nextIdx
	return removed, nil
}

func (a *finalShellApp) refreshTable() {
	if a.model != nil {
		a.model.rows = a.rows
	}
	if a.table != nil {
		a.table.ReloadData()
	}
}

func (a *finalShellApp) selectedProfile() (connectionProfile, bool) {
	if a.idx < 0 || a.idx >= len(a.rows) {
		return connectionProfile{}, false
	}
	return a.rows[a.idx], true
}

func (a *finalShellApp) connectSelected() {
	a.activateConnectionRow(a.idx)
}

func sessionTabTitle(state terminalTabState) string {
	prefix := ""
	switch state.Status {
	case sessionRunning:
		prefix = "▶ "
	case sessionSucceeded:
		prefix = "✓ "
	case sessionFailed:
		prefix = "! "
	case sessionStopped:
		prefix = "■ "
	}
	name := state.Profile.Name
	if state.InstanceNumber > 1 {
		name = fmt.Sprintf("%s · %d", name, state.InstanceNumber)
	}
	return prefix + name
}

func (a *finalShellApp) syncActiveSessionView() {
	if a == nil || a.sessions == nil {
		return
	}
	if a.cmdInput != nil {
		a.sessions.SetActiveDraft(a.cmdInput.Text())
	}
	if a.output != nil {
		activeID := a.activeSessionID()
		// Interactive tabs retain the original PTY byte stream in the workspace.
		// Fl_Terminal.Text() is a rendered-text projection and would discard VT
		// control sequences needed to reconstruct colors and cursor state.
		if a.interactive == nil || !a.interactive.Has(activeID) {
			a.sessions.SetActiveOutput(a.output.Text())
		}
	}
}

func (a *finalShellApp) renderActiveSession() {
	if a == nil || a.sessions == nil {
		return
	}
	state, ok := a.sessions.Active()
	if !ok {
		a.configureActiveTerminalMode(terminalTabState{}, false)
		a.renderMonitorSidebar(terminalTabState{}, false)
		if a.output != nil {
			a.setTerminalOutput(terminalWelcomeText())
		}
		if a.cmdInput != nil {
			a.cmdInput.SetText("")
		}
		a.updateCommandControls()
		return
	}
	a.configureActiveTerminalMode(state, true)
	if a.output != nil {
		a.setTerminalOutput(state.Output)
	}
	if a.cmdInput != nil {
		a.cmdInput.SetText(state.CommandDraft)
	}
	if a.sessionTabs != nil {
		a.sessionTabs.SetTabTitle(a.sessions.ActiveIndex(), sessionTabTitle(state))
	}
	a.renderMonitorSidebar(state, true)
	a.updateCommandControls()
}

func (a *finalShellApp) selectSessionTab(index int) {
	if a == nil || a.sessions == nil || index == a.sessions.ActiveIndex() {
		return
	}
	a.syncActiveSessionView()
	if a.sessions.Select(index) {
		a.renderActiveSession()
		if state, ok := a.sessions.Active(); ok {
			a.setStatus(trf("session.active", state.Profile.Name))
		}
	}
}

func (a *finalShellApp) openSession(profile connectionProfile) {
	if a == nil {
		return
	}
	if a.sessions == nil {
		a.sessions = newSessionWorkspace()
	}
	a.syncActiveSessionView()
	index, created := a.sessions.Open(profile)
	state, stateOK := a.sessions.Active()
	if created && stateOK {
		a.startMonitorForSession(state)
	}
	if a.sessionTabs != nil && stateOK {
		if created {
			a.sessionTabs.AddTabWithID(state.ID, sessionTabTitle(state), nil)
			a.sessionTabs.SelectTab(index)
		} else {
			a.sessionTabs.SetTabTitle(index, sessionTabTitle(state))
			a.sessionTabs.SelectTab(index)
		}
	}
	a.renderActiveSession()
	if created && stateOK && (state.Profile.Type == connectionTypeLocal || state.Profile.Type == connectionTypeSSH) {
		if err := a.startInteractiveSession(state); err != nil {
			a.appendSessionOutput(state.ID, trf("output.session_shell_start_failed", err.Error()))
			a.setStatus(tr("status.session_shell_start_failed"))
			a.showTopNotice(tr("notice.session_shell_start_failed.title"), err.Error(), true)
		} else {
			a.configureActiveTerminalMode(state, true)
			a.updateCommandControls()
		}
	}
}

func (a *finalShellApp) ensureRuntimeSession(profile connectionProfile) (terminalTabState, bool) {
	if a == nil || strings.TrimSpace(profile.ID) == "" {
		return terminalTabState{}, false
	}
	if a.sessions != nil {
		if state, ok := a.sessions.Active(); ok && state.ProfileID == profile.ID {
			return state, true
		}
	}
	a.openSession(profile)
	if a.sessions == nil {
		return terminalTabState{}, false
	}
	state, ok := a.sessions.Active()
	return state, ok && state.ProfileID == profile.ID
}

func (a *finalShellApp) closeActiveSession() {
	if a == nil || a.sessions == nil {
		return
	}
	index := a.sessions.ActiveIndex()
	state, ok := a.sessions.Active()
	if !ok {
		return
	}
	a.syncActiveSessionView()
	if !a.sessions.Close(index) {
		a.setStatus(tr("session.close_running"))
		a.showTopNotice(tr("session.close_running_title"), tr("session.close_running"), false)
		return
	}
	if a.interactive != nil {
		a.interactive.Close(state.ID)
	}
	a.stopMonitorForSession(state.ID)
	if a.sessionTabs != nil {
		a.sessionTabs.RemoveTab(index)
	}
	a.renderActiveSession()
	if next, exists := a.sessions.Active(); exists {
		a.setStatus(trf("session.closed_active", state.Profile.Name, next.Profile.Name))
	} else {
		a.setStatus(trf("session.closed", state.Profile.Name))
	}
}

func (a *finalShellApp) activeSessionProfile() (connectionProfile, bool) {
	if a != nil && a.sessions != nil {
		if state, ok := a.sessions.Active(); ok {
			return state.Profile, true
		}
	}
	profile, ok := a.selectedProfile()
	if ok {
		a.openSession(profile)
	}
	return profile, ok
}

func (a *finalShellApp) updateSessionTab(runID string, status sessionStatus) {
	if a == nil || a.sessions == nil || !a.sessions.FinishRun(runID, status) {
		return
	}
	a.refreshSessionTabs()
}

func (a *finalShellApp) refreshSessionTabs() {
	if a == nil || a.sessions == nil || a.sessionTabs == nil {
		return
	}
	for index, state := range a.sessions.Tabs() {
		a.sessionTabs.SetTabTitle(index, sessionTabTitle(state))
	}
}

func (a *finalShellApp) activateConnectionRow(row int) {
	if a == nil || row < 0 || row >= len(a.rows) {
		return
	}
	if a.idx != row {
		a.selectRow(row)
	}
	a.activateProfile(a.rows[row])
}

func (a *finalShellApp) activateProfile(p connectionProfile) bool {
	if a == nil || strings.TrimSpace(p.ID) == "" {
		return false
	}
	p = markProfileUsed(p, time.Now())
	if err := a.persistProfile(p); err != nil {
		gtbox_log.LogErrorf("activate connection failed: %s", err.Error())
		a.appendOutput(trf("output.save_failed", err.Error()))
		a.setStatus(tr("status.save_failed"))
		a.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return false
	}
	a.refreshTable()
	if a.idx >= 0 && a.table != nil {
		a.table.SelectRow(a.idx)
	}
	a.openSession(p)
	a.appendOutput(trf("output.profile_ready", p.Name))
	a.setStatus(trf("status.connection_activated", p.Name))
	if (p.Type == connectionTypeLocal || p.Type == connectionTypeSSH) && a.output != nil && a.output.Raw() != nil {
		a.output.Raw().TakeFocus()
	} else if a.cmdInput != nil && a.cmdInput.View() != nil {
		if raw := a.cmdInput.View().Raw(); raw != nil {
			if focusable, ok := raw.(interface{ TakeFocus() int }); ok {
				focusable.TakeFocus()
			}
		}
	}
	return true
}

func (a *finalShellApp) testProfile(p connectionProfile) {
	a.openSession(p)
	if p.Type == connectionTypeLocal {
		a.runAsync(p, "pwd && whoami && uname -a")
		return
	}
	a.runAsync(p, "echo connected && uname -a")
}

func (a *finalShellApp) runCommand() {
	p, ok := a.activeSessionProfile()
	if !ok {
		return
	}
	cmd := strings.TrimSpace(a.cmdInput.Text())
	if cmd == "" {
		cmd = "pwd"
	}
	if a.runInteractiveCommand(cmd) {
		return
	}
	a.runAsync(p, cmd)
}

func (a *finalShellApp) showSelectedHistory() {
	p, ok := a.activeSessionProfile()
	if !ok || a.history == nil {
		return
	}
	entries, err := a.history.LoadForConnection(p.ID, 10)
	if err != nil {
		a.appendOutput(trf("output.history_failed", err.Error()))
		a.setStatus(tr("status.history_failed"))
		a.showTopNotice(tr("status.history_failed"), err.Error(), true)
		return
	}
	a.appendOutput(formatHistoryEntries(p, entries))
	a.setStatus(trf("status.history_count", len(entries)))
}

func (a *finalShellApp) recallLastCommand() {
	p, ok := a.activeSessionProfile()
	if !ok || a.history == nil || a.cmdInput == nil {
		return
	}
	cmd, found, err := a.history.LastCommand(p.ID)
	if err != nil {
		a.appendOutput(trf("output.last_failed", err.Error()))
		a.setStatus(tr("status.last_failed"))
		a.showTopNotice(tr("status.last_failed"), err.Error(), true)
		return
	}
	if !found {
		a.setStatus(trf("status.no_previous", p.Name))
		a.showTopNotice(tr("notice.no_previous.title"), tr("notice.no_previous.message"), false)
		return
	}
	a.cmdInput.SetText(cmd)
	a.setStatus(tr("status.recalled"))
}

func (a *finalShellApp) clearTerminalOutput() {
	if a.output == nil {
		return
	}
	a.setTerminalOutput(terminalWelcomeText())
	if a.sessions != nil {
		a.sessions.SetActiveOutput(terminalWelcomeText())
	}
	a.setStatus(tr("terminal.cleared"))
}

func terminalWelcomeText() string {
	return tr("terminal.welcome")
}

func (a *finalShellApp) beginCommandRunForSession(cancel context.CancelFunc, sessionID string) (uint64, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if cancel == nil || sessionID == "" {
		return 0, false
	}
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if _, running := a.runs[sessionID]; running {
		return 0, false
	}
	a.runID++
	if a.runs == nil {
		a.runs = make(map[string]commandRun)
	}
	a.runs[sessionID] = commandRun{id: a.runID, cancel: cancel}
	return a.runID, true
}

func (a *finalShellApp) finishCommandRun(sessionID string, id uint64) bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	run, ok := a.runs[sessionID]
	if !ok || run.id != id {
		return false
	}
	delete(a.runs, sessionID)
	return true
}

func (a *finalShellApp) commandRunningForSession(sessionID string) bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	_, ok := a.runs[sessionID]
	return ok
}

func (a *finalShellApp) anyCommandRunning() bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	return len(a.runs) > 0
}

func (a *finalShellApp) cancelCommandRun(sessionID string) bool {
	a.runMu.Lock()
	run, ok := a.runs[sessionID]
	a.runMu.Unlock()
	if !ok || run.cancel == nil {
		return false
	}
	run.cancel()
	return true
}

func (a *finalShellApp) activeSessionID() string {
	if a == nil || a.sessions == nil {
		return ""
	}
	state, ok := a.sessions.Active()
	if !ok {
		return ""
	}
	return state.ID
}

func (a *finalShellApp) stopCommand() {
	sessionID := a.activeSessionID()
	if a.interruptInteractiveSession(sessionID) {
		return
	}
	if sessionID == "" || !a.cancelCommandRun(sessionID) {
		a.setStatus(tr("status.no_command"))
		return
	}
	a.appendSessionOutput(sessionID, tr("output.stop_requested"))
	a.setStatus(tr("status.stopping"))
}

func (a *finalShellApp) updateCommandControls() {
	running := a.commandRunningForSession(a.activeSessionID())
	interactive := a.interactive != nil && a.interactive.Has(a.activeSessionID())
	if a.runButton != nil && a.runButton.Raw() != nil {
		if running {
			a.runButton.Raw().Deactivate()
		} else {
			a.runButton.Raw().Activate()
		}
		a.runButton.Raw().Redraw()
	}
	if a.stopButton != nil && a.stopButton.Raw() != nil {
		if running || interactive {
			a.stopButton.Raw().Activate()
			a.stopButton.SetBackgroundColor(uint(tokenColor(modernTheme.destructive)))
			a.stopButton.SetTitleColor(uint(tokenColor(modernTheme.foreground)))
		} else {
			a.stopButton.Raw().Deactivate()
			a.stopButton.SetBackgroundColor(uint(tokenColor(modernTheme.elevated)))
			a.stopButton.SetTitleColor(uint(tokenColor(modernTheme.muted)))
		}
		a.stopButton.Raw().Redraw()
	}
}

func (a *finalShellApp) runAsync(p connectionProfile, command string) {
	p = markProfileUsed(p, time.Now())
	if err := a.persistRuntimeProfile(p); err != nil {
		a.appendOutput(trf("output.save_current_failed", err.Error()))
		a.setStatus(tr("status.save_runtime_failed"))
		a.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return
	}
	state, ok := a.ensureRuntimeSession(p)
	if !ok {
		a.setStatus(tr("status.command_active"))
		return
	}
	sessionID := state.ID
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout())
	runID, ok := a.beginCommandRunForSession(cancel, sessionID)
	if !ok {
		cancel()
		a.setStatus(tr("status.command_active"))
		a.showTopNotice(tr("notice.command_active.title"), tr("notice.command_active.message"), false)
		return
	}
	runToken := fmt.Sprintf("%s:%d", sessionID, runID)
	if a.sessions == nil || !a.sessions.BeginRun(runToken) {
		a.finishCommandRun(sessionID, runID)
		cancel()
		a.setStatus(tr("status.command_active"))
		return
	}
	a.sessions.SetActiveDraft(command)
	a.refreshSessionTabs()
	a.appendSessionOutput(sessionID, fmt.Sprintf("\n$ [%s] %s\n", p.Name, command))
	a.setStatus(trf("status.running", p.Name))
	a.updateCommandControls()
	outputBatcher := newGUIOutputBatcher(func(fn func()) {
		fltk_bridge.Awake(fn)
	}, func(text string) { a.appendSessionOutput(sessionID, text) })
	go func() {
		defer cancel()
		sess := terminal.NewSession(a.history)
		sess.OnEvent = func(event terminal.Event) {
			switch event.Stream {
			case terminal.StreamStdout:
				outputBatcher.Enqueue(event.Line + "\n")
			case terminal.StreamStderr:
				outputBatcher.Enqueue(trf("output.stderr", event.Line))
			}
		}
		_, result, err := executeCommandResultWithSession(ctx, sess, p, command)
		outputBatcher.AfterFlush(func() {
			if !a.finishCommandRun(sessionID, runID) {
				return
			}
			a.updateCommandControls()
			active := a.activeSessionID() == sessionID
			finalStatus := sessionSucceeded
			switch {
			case errors.Is(err, context.Canceled):
				finalStatus = sessionStopped
				a.appendSessionOutput(sessionID, tr("output.cancelled"))
				if active {
					a.setStatus(tr("status.cancelled"))
				}
			case errors.Is(err, context.DeadlineExceeded):
				finalStatus = sessionFailed
				message := trf("notice.timeout.message", commandTimeout())
				a.appendSessionOutput(sessionID, trf("output.error", message))
				if active {
					a.setStatus(tr("status.timed_out"))
					a.showTopNotice(tr("notice.timeout.title"), message, true)
				}
			case err != nil:
				finalStatus = sessionFailed
				a.appendSessionOutput(sessionID, trf("output.error", err.Error()))
				if active {
					a.setStatus(trf("status.failed", result.ExitCode))
					a.showTopNotice(tr("notice.failed.title"), err.Error(), true)
				}
			case len(result.Events) == 0 && result.Stdout == "" && result.Stderr == "":
				a.appendSessionOutput(sessionID, tr("output.no_output"))
				if active {
					a.setStatus(trf("status.completed", 0))
				}
			default:
				if active {
					a.setStatus(trf("status.completed", result.ExitCode))
				}
			}
			a.updateSessionTab(runToken, finalStatus)
		})
	}()
}

func commandTimeout() time.Duration {
	seconds := config.CommandTimeoutDefaultSeconds
	if config.GlobalConfig != nil && config.GlobalConfig.Terminal != nil && config.GlobalConfig.Terminal.CommandTimeoutSeconds > 0 {
		seconds = config.GlobalConfig.Terminal.CommandTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// persistProfile publishes an edited profile to GUI state only after both the
// encrypted connection store and shared config commit successfully. This avoids
// a false success where an edit appears active until the next restart after a
// disk error.
func (a *finalShellApp) persistProfile(p connectionProfile) error {
	if a == nil || a.store == nil {
		return nil
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	nextAll := upsertProfile(append([]connectionProfile(nil), a.allRows...), p)
	sortConnectionsForNavigator(nextAll)
	if err := a.store.SaveActive(nextAll, p.ID); err != nil {
		return err
	}
	a.allRows = nextAll
	a.refreshNavigator(p.ID)
	return nil
}

// persistRuntimeProfile keeps command execution and connection persistence in
// sync. Users often edit a FinalShell-style connection and immediately press
// Test/Run without pressing Save first; the GUI should execute those form values
// and also make them available on next launch.
func (a *finalShellApp) persistRuntimeProfile(p connectionProfile) error {
	if err := a.persistProfile(p); err != nil {
		return err
	}
	a.refreshTable()
	if a.table != nil && a.idx >= 0 {
		a.table.SelectRow(a.idx)
	}
	return nil
}

func (a *finalShellApp) appendRecentHistory() {
	if a.history == nil {
		return
	}
	entries, err := a.history.Load(5)
	if err != nil {
		gtbox_log.LogErrorf("load command history failed: %s", err.Error())
		return
	}
	if len(entries) == 0 {
		return
	}
	a.appendOutput(tr("history.recent"))
	for _, entry := range entries {
		a.appendOutput(trf("history.recent_entry", entry.ConnectionName, entry.Command, entry.ExitCode))
	}
	a.appendOutput("\n")
}

func executeCommand(ctx context.Context, p connectionProfile, command string) (string, error) {
	out, _, err := executeCommandResult(ctx, p, command)
	return out, err
}

func executeCommandResult(ctx context.Context, p connectionProfile, command string) (string, terminal.CommandResult, error) {
	return executeCommandResultWithSession(ctx, nil, p, command)
}

func executeCommandResultWithSession(ctx context.Context, sess *terminal.Session, p connectionProfile, command string) (string, terminal.CommandResult, error) {
	conn, err := profileToConnection(p)
	if err != nil {
		return "", terminal.CommandResult{Connection: conn, Command: command, ExitCode: -1}, err
	}
	if sess == nil {
		sess = terminal.NewSession(nil)
	}
	result, err := sess.RunCommand(ctx, conn, command)
	return formatCommandResult(result), result, err
}

func profileToConfigConnection(p connectionProfile) terminal.Connection {
	connType := terminal.ConnectionTypeSSH
	if p.Type == connectionTypeLocal {
		connType = terminal.ConnectionTypeLocal
	}
	privateKey := strings.TrimSpace(p.PrivateKey)
	// The GUI's dedicated connection store owns encrypted/secret material. Keep
	// the shared app config useful for selectors/defaults without duplicating
	// passwords or pasted private-key contents, and without failing Save when a
	// user enters a key path that only needs to exist at execution time.
	if strings.Contains(privateKey, "-----BEGIN") {
		privateKey = ""
	}
	conn := terminal.Connection{
		ID:          p.ID,
		Name:        p.Name,
		Type:        connType,
		Host:        p.Host,
		Port:        p.Port,
		Username:    p.Username,
		PrivateKey:  privateKey,
		WorkingDir:  strings.TrimSpace(p.WorkingDir),
		Description: p.Description,
	}
	conn.Normalize()
	return conn
}

func profileToConnection(p connectionProfile) (terminal.Connection, error) {
	connType := terminal.ConnectionTypeSSH
	if p.Type == connectionTypeLocal {
		connType = terminal.ConnectionTypeLocal
	}
	conn := terminal.Connection{
		ID:          p.ID,
		Name:        p.Name,
		Type:        connType,
		Host:        p.Host,
		Port:        p.Port,
		Username:    p.Username,
		Password:    p.Password(),
		PrivateKey:  strings.TrimSpace(p.PrivateKey),
		WorkingDir:  strings.TrimSpace(p.WorkingDir),
		Description: p.Description,
	}
	if conn.PrivateKey != "" && !strings.Contains(conn.PrivateKey, "-----BEGIN") {
		keyBytes, err := os.ReadFile(conn.PrivateKey)
		if err != nil {
			return conn, fmt.Errorf("read private key %q: %w", conn.PrivateKey, err)
		}
		conn.PrivateKey = string(keyBytes)
	}
	conn.Normalize()
	return conn, nil
}

func formatCommandResult(result terminal.CommandResult) string {
	var b strings.Builder
	if result.Stdout != "" {
		b.WriteString(result.Stdout)
	}
	if result.Stderr != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(result.Stderr)
	}
	if result.ExitCode != 0 {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(trf("output.exit", result.ExitCode))
	}
	return b.String()
}

func formatHistoryEntries(p connectionProfile, entries []terminal.HistoryEntry) string {
	var b strings.Builder
	name := p.Name
	if name == "" {
		name = p.ID
	}
	if name == "" {
		name = tr("history.selected_connection")
	}
	b.WriteString(trf("history.title", name))
	if len(entries) == 0 {
		b.WriteString(tr("history.none"))
		return b.String()
	}
	for _, entry := range entries {
		when := entry.Time.Local().Format("2006-01-02 15:04:05")
		if entry.Interactive {
			b.WriteString(fmt.Sprintf("\n%s  interactive  %s", when, entry.Command))
			continue
		}
		b.WriteString(trf("history.entry", when, entry.ExitCode, entry.Command))
	}
	return b.String()
}

func (a *finalShellApp) setTerminalOutput(text string) {
	if a == nil || a.output == nil {
		return
	}
	// Session switches reconstruct the native ANSI/VT surface from that tab's
	// retained stream. Reset clears both FLTK display state and the reusable
	// stream filter's partial escape state, preventing one tab's split OSC/CSI
	// sequence from consuming bytes in the next tab.
	a.output.Reset()
	a.output.ClearHistory()
	if text != "" {
		a.output.Append(text)
	}
}

func (a *finalShellApp) reflowTerminalOutput(size uikit.TerminalSize) {
	if a == nil || size.Columns <= 0 || size.Columns == a.terminalColumns {
		return
	}
	a.terminalColumns = size.Columns
	if a.sessions != nil {
		if state, ok := a.sessions.Active(); ok {
			a.setTerminalOutput(state.Output)
			return
		}
	}
	a.setTerminalOutput(terminalWelcomeText())
}

func (a *finalShellApp) appendOutput(s string) {
	if a == nil || s == "" {
		return
	}
	if a.sessions != nil {
		a.sessions.AppendActiveOutput(s)
	}
	if a.output != nil {
		a.output.Append(s)
	}
}

func (a *finalShellApp) appendSessionOutput(sessionID, s string) {
	if a == nil || s == "" {
		return
	}
	if a.sessions == nil || !a.sessions.AppendOutput(sessionID, s) {
		return
	}
	if active, ok := a.sessions.Active(); ok && active.ID == sessionID && a.output != nil {
		a.output.Append(s)
	}
}

func (a *finalShellApp) setStatus(s string) {
	if a.status != nil {
		a.status.SetText(s)
	}
}
