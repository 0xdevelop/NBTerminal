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
	"github.com/0xdevelop/NBTerminal/internal/persistence"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/foundation"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/automation"
	"github.com/0xdevelop/fltk2go/uikit/screen"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
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
		return tr("profile.local_shell")
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

type connectionStore struct {
	path string
	mu   sync.Mutex
	list []connectionProfile
}

func newConnectionStore(dataDir string) *connectionStore {
	return &connectionStore{path: filepath.Join(dataDir, connectionStoreFile)}
}

func (s *connectionStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	return syncConfigConnections(s.list, activeID)
}

func (s *connectionStore) List() []connectionProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]connectionProfile(nil), s.list...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group == out[j].Group {
			return out[i].Name < out[j].Name
		}
		return out[i].Group < out[j].Group
	})
	return out
}

func (s *connectionStore) normalizeLocked() {
	now := time.Now().UTC().Format(time.RFC3339)
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
		if s.list[i].LastUsed == "" {
			s.list[i].LastUsed = now
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
		{ID: "local-shell", Name: tr("profile.local_shell"), Group: tr("profile.local_group"), Type: connectionTypeLocal, LastUsed: time.Now().UTC().Format(time.RFC3339), Description: tr("profile.local_description")},
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
			LastUsed:    time.Now().UTC().Format(time.RFC3339),
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
	connections := make([]terminal.Connection, 0, len(profiles))
	for _, profile := range profiles {
		conn := profileToConfigConnection(profile)
		connections = append(connections, conn)
	}
	if len(connections) == 0 {
		config.GlobalConfig.Connections = nil
		config.GlobalConfig.ActiveConnectionID = ""
	} else {
		config.GlobalConfig.Connections = terminal.NormalizeConnections(connections)
		if activeID != "" {
			config.GlobalConfig.ActiveConnectionID = activeID
		}
		active := config.GlobalConfig.ActiveConnectionID
		found := active == ""
		for _, conn := range config.GlobalConfig.Connections {
			if conn.ID == active {
				found = true
				break
			}
		}
		if !found || active == "" {
			config.GlobalConfig.ActiveConnectionID = config.GlobalConfig.Connections[0].ID
		}
	}
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
	rows []connectionProfile
}

func (m *tableModel) NumberOfRows(_ *tableview.TableView) int { return len(m.rows) }
func (m *tableModel) CellForColumn(_ *tableview.TableView, row, col int) *tableview.TableViewCell {
	cell := tableview.NewCell("connection-cell")
	if row < 0 || row >= len(m.rows) {
		return cell
	}
	p := m.rows[row]
	switch col {
	case 0:
		cell.SetText(p.Group)
	case 1:
		cell.SetText(p.Name)
	case 2:
		cell.SetText(string(p.Type))
	case 3:
		cell.SetText(p.tableEndpoint())
	case 4:
		cell.SetText(formatLastUsed(p.LastUsed))
	}
	return cell
}

type finalShellApp struct {
	store   *connectionStore
	history *terminal.HistoryStore
	session *terminal.Session
	allRows []connectionProfile
	rows    []connectionProfile
	idx     int

	window    *uikit.UIWindow
	workspace *uikit.UISplitView
	table     *uikit.UITableView
	model     *tableModel

	searchInput *uikit.Input
	nameInput   *uikit.Input
	groupInput  *uikit.Input
	typeInput   *uikit.Input
	hostInput   *uikit.Input
	portInput   *uikit.Input
	userInput   *uikit.Input
	passInput   *uikit.Input
	keyInput    *uikit.Input
	workInput   *uikit.Input
	cmdInput    *uikit.UITextView
	output      *uikit.UITextView
	status      *uikit.UILabel
	notice      *uikit.UIWindow
	runButton   *uikit.UIButton
	stopButton  *uikit.UIButton
	settings    *settingsWindow

	runMu     sync.Mutex
	runCancel context.CancelFunc
	runID     uint64
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

	history := terminal.NewHistoryStore(filepath.Join(config.CurrentApp.DataDir, "terminal-history.jsonl"))
	app := &finalShellApp{
		store:   newConnectionStore(config.CurrentApp.DataDir),
		history: history,
		session: terminal.NewSession(history),
		idx:     -1,
	}
	if err := app.store.Load(); err != nil {
		gtbox_log.LogErrorf("load connection store failed: %s", err.Error())
	}
	app.allRows = app.store.List()
	app.rows = append([]connectionProfile(nil), app.allRows...)
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
		raw.SetColor(tokenColor(modernTheme.background))
		raw.SetSizeRange(1120, 720, 0, 0, 20, 20, false)
	}
	root := a.window.RootView()

	root.AddSubview(titleLabel(margin, 14, 440, 28, tr("app.title")))
	root.AddSubview(mutedLabel(margin+2, 38, 520, 22, tr("app.subtitle")))
	settingsButton := button(rightX+18, 18, 124, 30, tr("setting.title"), "app.settings", a.openSettings)
	root.AddSubview(settingsButton)
	a.status = pillLabel(rightX+158, 18, rightW-158, 30, tr("app.ready"))
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
	rightPanel := uikit.NewUIGroup(rect(rightX, 72, rightW, 786))
	rightPanel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	rightPanel.SetAutomationID("terminal.panel")
	rightPanel.Raw().Resizable(rightPanel.Raw())
	a.workspace.SetLeftView(left)
	a.workspace.SetRightView(rightPanel)
	// Establish the authored 1440x900 design geometry before pane controls are
	// attached. SplitView lays out empty pane content immediately; resetting this
	// baseline ensures the final persisted-ratio resize translates every absolute
	// FLTK child from the same coordinates used below.
	left.Raw().Resize(margin, 72, leftW, 786)
	rightPanel.Raw().Resize(rightX, 72, rightW, 786)
	root.AddSubview(a.workspace)
	left.AddSubview(sectionTitle(margin+18, 86, 260, 24, tr("app.connections")))
	left.AddSubview(mutedLabel(margin+18, 110, 430, 18, tr("connections.subtitle")))
	left.AddSubview(mutedLabel(margin+18, 132, 70, 18, tr("connections.search")))
	a.searchInput = inputNoLabel(margin+82, 126, leftW-194, 30, "connections.search", tr("connections.search_placeholder"))
	a.searchInput.OnChange(a.jumpToSearchMatch)
	left.AddSubview(a.searchInput)
	findBtn := button(margin+390, 126, 86, 30, tr("connections.find"), "connections.find", a.jumpToSearchMatch)
	left.AddSubview(findBtn)

	tv, err := uikit.NewUITableView(margin+14, 166, leftW-28, 362)
	if err == nil {
		a.table = tv
		a.table.View().SetAutomationID("connections.table").SetAutomationName(tr("app.connections"))
		a.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: tr("connections.group"), Width: 78})
		a.table.AddColumn(tableview.TableColumn{Identifier: "name", Title: tr("connections.name"), Width: 108})
		a.table.AddColumn(tableview.TableColumn{Identifier: "type", Title: tr("connections.type"), Width: 50})
		a.table.AddColumn(tableview.TableColumn{Identifier: "endpoint", Title: tr("connections.endpoint"), Width: 126})
		a.table.AddColumn(tableview.TableColumn{Identifier: "last", Title: tr("connections.last_used"), Width: 96})
		a.model = &tableModel{rows: a.rows}
		a.table.SetDataSource(a.model)
		a.table.SetDelegate(tableDelegate{onSelect: func(row int) {
			a.selectRow(row)
			if err := a.persistActiveRow(row); err != nil {
				gtbox_log.LogErrorf("save active connection failed: %s", err.Error())
				a.setStatus(tr("status.save_failed"))
				a.showTopNotice(tr("status.save_failed"), err.Error(), true)
			}
		}})
		a.table.SetBackgroundColor(tokenColor(modernTheme.card))
		a.table.SetCustomDraw(a.drawConnectionCell)
		a.table.ReloadData()
		left.AddSubview(a.table)
	}

	left.AddSubview(sectionTitle(margin+18, 548, 260, 22, tr("details.title")))
	a.nameInput = input(margin+82, 582, 164, 30, tr("field.name"), "form.name")
	left.AddSubview(a.nameInput)
	a.groupInput = input(margin+318, 582, 168, 30, tr("field.group"), "form.group")
	left.AddSubview(a.groupInput)

	a.typeInput = input(margin+82, 622, 92, 30, tr("field.type"), "form.type")
	left.AddSubview(a.typeInput)
	a.hostInput = input(margin+246, 622, 152, 30, tr("field.host"), "form.host")
	left.AddSubview(a.hostInput)
	a.portInput = input(margin+446, 622, 40, 30, tr("field.port"), "form.port")
	left.AddSubview(a.portInput)

	a.userInput = input(margin+82, 662, 164, 30, tr("field.user"), "form.username")
	left.AddSubview(a.userInput)
	a.passInput = uikit.NewInputWithType(margin+318, 662, 168, 30, tr("field.password"), uikit.SecretInput)
	styleInput(a.passInput)
	a.passInput.View().SetAutomationID("form.password")
	left.AddSubview(a.passInput)

	a.workInput = input(margin+82, 702, 404, 30, tr("field.workdir"), "form.working_dir")
	left.AddSubview(a.workInput)

	a.keyInput = input(margin+82, 742, 404, 30, tr("field.key"), "form.key")
	left.AddSubview(a.keyInput)

	addBtn := button(margin+14, 816, 82, 34, tr("action.new"), "action.new", a.newProfile)
	left.AddSubview(addBtn)
	saveBtn := button(margin+106, 816, 82, 34, tr("action.save"), "action.save", a.saveProfile)
	left.AddSubview(saveBtn)
	deleteBtn := button(margin+198, 816, 82, 34, tr("action.delete"), "action.delete", a.deleteProfile)
	left.AddSubview(deleteBtn)
	testBtn := button(margin+290, 816, 82, 34, tr("action.test"), "action.test", a.testConnection)
	left.AddSubview(testBtn)
	connectBtn := primaryButton(margin+382, 816, 118, 34, tr("action.connect"), "action.connect", a.connectSelected)
	left.AddSubview(connectBtn)

	rightPanel.SetBackgroundColor(uint(tokenColor(modernTheme.card)))
	rightPanel.SetAutomationID("terminal.panel")
	rightPanel.AddSubview(sectionTitle(rightX+18, 86, 330, 24, tr("terminal.title")))
	rightPanel.AddSubview(mutedLabel(rightX+18, 110, 650, 18, tr("terminal.subtitle")))

	a.output = uikit.NewUITextView(rect(rightX+18, 140, rightW-36, 608))
	a.output.SetAutomationID("terminal.output").SetAutomationName(tr("terminal.output_name"))
	a.output.SetFontSize(14)
	a.output.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	a.output.SetFallbackFont(fltk_bridge.FREE_FONT, isEmojiRune)
	a.output.SetBackgroundColor(uint(tokenColor(modernTheme.terminal)))
	a.output.SetText(terminalWelcomeText())
	a.appendRecentHistory()
	rightPanel.AddSubview(a.output)

	bar := commandBarLayout(rightX, rightW)
	historyBtn := button(bar.history.X, bar.history.Y, bar.history.Width, bar.history.Height, tr("terminal.history"), "terminal.history", a.showSelectedHistory)
	rightPanel.AddSubview(historyBtn)
	recallBtn := button(bar.last.X, bar.last.Y, bar.last.Width, bar.last.Height, tr("terminal.last"), "terminal.last_command", a.recallLastCommand)
	rightPanel.AddSubview(recallBtn)
	clearBtn := button(bar.clear.X, bar.clear.Y, bar.clear.Width, bar.clear.Height, tr("terminal.clear"), "terminal.clear", a.clearTerminalOutput)
	rightPanel.AddSubview(clearBtn)
	rightPanel.AddSubview(mutedLabel(bar.command.X, bar.commandLabelY, 160, 18, tr("terminal.command")))
	a.cmdInput = uikit.NewUITextView(rect(bar.command.X, bar.command.Y, bar.command.Width, bar.command.Height))
	a.cmdInput.SetAutomationID("terminal.command").SetAutomationName(tr("terminal.command"))
	a.cmdInput.SetWrapNone()
	a.cmdInput.SetFontSize(14)
	a.cmdInput.SetTextColor(uint(themeColor(51, 65, 85)))
	a.cmdInput.SetFallbackFont(fltk_bridge.FREE_FONT, isEmojiRune)
	a.cmdInput.SetBackgroundColor(uint(themeColor(248, 250, 252)))
	a.cmdInput.OnKey(func(event uikit.KeyEvent) bool {
		if event.Key != fltk_bridge.ENTER_KEY {
			return false
		}
		a.runCommand()
		return true
	})
	rightPanel.AddSubview(a.cmdInput)
	a.stopButton = button(bar.stop.X, bar.stop.Y, bar.stop.Width, bar.stop.Height, tr("terminal.stop"), "terminal.stop", a.stopCommand)
	rightPanel.AddSubview(a.stopButton)
	a.runButton = primaryButton(bar.run.X, bar.run.Y, bar.run.Width, bar.run.Height, tr("terminal.run"), "terminal.run", a.runCommand)
	rightPanel.AddSubview(a.runButton)
	// Apply the persisted ratio only after pane controls are attached. FLTK uses
	// absolute child coordinates, so this final native resize translates and
	// reflows the complete pane hierarchy together (including language rebuilds).
	if config.GlobalConfig != nil && !config.GlobalConfig.ResetWorkspaceOnStart {
		a.workspace.SetPosition(config.GlobalConfig.WorkspaceSplitRatio)
	} else {
		a.workspace.SetPosition(config.WorkspaceSplitRatioDefault)
	}
	a.workspace.OnPositionChanged(a.workspacePositionChanged)
	a.setCommandRunning(false)

	if len(a.rows) > 0 {
		a.selectRow(activeConnectionIndex(a.rows))
	}
	a.window.Show()
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
	a.runMu.Lock()
	running := a.runCancel != nil
	a.runMu.Unlock()
	if running {
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
func (d tableDelegate) RowHeight(_ *tableview.TableView, _ int) int { return 0 }

func (a *finalShellApp) drawConnectionCell(ctx fltk_bridge.TableContext, row, col, x, y, w, h int) {
	switch ctx {
	case fltk_bridge.ContextTable:
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.card))
		fltk_bridge.PopClip()
	case fltk_bridge.ContextColHeader:
		titles := []string{tr("connections.group"), tr("connections.name"), tr("connections.type"), tr("connections.endpoint"), tr("connections.last_used")}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.elevated))
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.foreground))
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, 13)
		if col >= 0 && col < len(titles) {
			fltk_bridge.Draw(titles[col], x+5, y, w-10, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
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
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, 13)
		fltk_bridge.Draw(a.connectionCellText(row, col), x+6, y, w-12, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.border))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	}
}

func (a *finalShellApp) connectionCellText(row, col int) string {
	if row < 0 || row >= len(a.rows) {
		return ""
	}
	p := a.rows[row]
	switch col {
	case 0:
		return p.Group
	case 1:
		return p.Name
	case 2:
		return string(p.Type)
	case 3:
		return p.tableEndpoint()
	case 4:
		return formatLastUsed(p.LastUsed)
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
	const (
		y      = 784
		h      = 38
		gap    = 8
		runW   = 116
		stopW  = 68
		leftX  = 18
		histW  = 86
		recW   = 64
		clearW = 68
	)
	run := rect(rightX+rightW-runW-18, y, runW, h)
	stop := rect(run.X-gap-stopW, y, stopW, h)
	x := rightX + leftX
	history := rect(x, y, histW, h)
	x += histW + gap
	last := rect(x, y, recW, h)
	x += recW + gap
	clear := rect(x, y, clearW, h)
	cmdX := x + clearW + 14
	return commandBarSpec{
		history:       history,
		last:          last,
		clear:         clear,
		command:       rect(cmdX, y, stop.X-cmdX-14, h),
		stop:          stop,
		run:           run,
		commandLabelY: y - 20,
	}
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
	l.SetFontSize(13)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	l.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_CLIP)
	return l
}

func titleLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(20)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	return l
}

func sectionTitle(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(15)
	l.SetTextColor(uint(tokenColor(modernTheme.foreground)))
	return l
}

func mutedLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(12)
	l.SetTextColor(uint(tokenColor(modernTheme.muted)))
	return l
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
	in.SetFontSize(13)
	in.SetTextColor(uint(tokenColor(modernTheme.muted)))
	// FLTK input text color is not exposed by the current uikit bridge; keep a
	// high-contrast light input surface so typed UTF-8 text remains readable.
	in.SetBackgroundColor(uint(themeColor(248, 250, 252)))
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
		raw.SetLabelSize(13)
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
	a.rows = filterConnections(a.allRows, query)
	a.idx = -1
	a.refreshTable()
	if len(a.rows) == 0 {
		if query == "" {
			a.setStatus(tr("connections.none"))
		} else {
			a.setStatus(trf("connections.no_match", query))
		}
		return
	}
	a.selectRow(0)
	if query == "" {
		a.setStatus(trf("connections.showing", len(a.rows)))
		return
	}
	a.setStatus(trf("connections.matched", len(a.rows), a.rows[0].Name))
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
	a.nameInput.SetText(p.Name)
	a.groupInput.SetText(p.Group)
	a.typeInput.SetText(string(p.Type))
	a.hostInput.SetText(p.Host)
	if p.Port > 0 {
		a.portInput.SetText(fmt.Sprintf("%d", p.Port))
	} else {
		a.portInput.SetText("")
	}
	a.userInput.SetText(p.Username)
	a.passInput.SetText("")
	a.workInput.SetText(p.WorkingDir)
	a.keyInput.SetText(p.PrivateKey)
	a.setStatus(trf("status.selected", p.Name))
	if a.table != nil {
		a.table.ReloadData()
	}
}

func (a *finalShellApp) persistActiveRow(row int) error {
	if a == nil || a.store == nil || row < 0 || row >= len(a.rows) {
		return nil
	}
	return a.store.SetActive(a.rows[row].ID)
}

func (a *finalShellApp) newProfile() {
	p := newConnectionProfile(os.Getenv("USER"))
	a.allRows = append(a.allRows, p)
	if a.searchInput != nil {
		a.searchInput.SetText("")
	}
	a.rows = append([]connectionProfile(nil), a.allRows...)
	a.refreshTable()
	a.selectRow(len(a.rows) - 1)
}

func newConnectionProfile(username string) connectionProfile {
	return connectionProfile{
		ID: fmt.Sprintf("conn-%d", time.Now().UnixNano()), Name: tr("profile.new_ssh"),
		Group: tr("profile.default_group"), Type: connectionTypeSSH, Port: 22, Username: username,
	}
}

func (a *finalShellApp) profileFromForm() connectionProfile {
	p := connectionProfile{}
	if a.idx >= 0 && a.idx < len(a.rows) {
		p = a.rows[a.idx]
	}
	p.Name = strings.TrimSpace(a.nameInput.Text())
	p.Group = strings.TrimSpace(a.groupInput.Text())
	p.Type = connectionType(strings.ToLower(strings.TrimSpace(a.typeInput.Text())))
	p.Host = strings.TrimSpace(a.hostInput.Text())
	fmt.Sscanf(strings.TrimSpace(a.portInput.Text()), "%d", &p.Port)
	p.Username = strings.TrimSpace(a.userInput.Text())
	p.WorkingDir = strings.TrimSpace(a.workInput.Text())
	p.PrivateKey = strings.TrimSpace(a.keyInput.Text())
	if pw := a.passInput.Text(); pw != "" {
		p.SetPassword(pw)
	}
	p.LastUsed = time.Now().UTC().Format(time.RFC3339)
	if p.ID == "" {
		p.ID = fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	if p.Name == "" {
		p.Name = tr("profile.unnamed")
	}
	if p.Group == "" {
		p.Group = tr("profile.default_group")
	}
	if p.Type != connectionTypeLocal && p.Type != connectionTypeSSH {
		p.Type = connectionTypeSSH
	}
	if p.Type == connectionTypeSSH && p.Port == 0 {
		p.Port = 22
	}
	return p
}

func (a *finalShellApp) saveProfile() {
	p := a.profileFromForm()
	if err := a.persistProfile(p); err != nil {
		a.appendOutput(trf("output.save_failed", err.Error()))
		a.setStatus(tr("status.save_failed"))
		a.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return
	}
	if a.searchInput != nil {
		a.searchInput.SetText("")
	}
	a.refreshTable()
	if i := indexProfileByID(a.rows, p.ID); i >= 0 {
		a.selectRow(i)
	}
	a.setStatus(trf("status.saved", p.Name))
}

func (a *finalShellApp) deleteProfile() {
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
	return a.profileFromForm(), true
}

func (a *finalShellApp) connectSelected() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	a.saveProfile()
	a.appendOutput(trf("output.profile_ready", p.Name, p.endpoint()))
	a.setStatus(tr("status.profile_ready"))
}

func (a *finalShellApp) testConnection() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	if p.Type == connectionTypeLocal {
		a.runAsync(p, "pwd && whoami && uname -a")
		return
	}
	a.runAsync(p, "echo connected && uname -a")
}

func (a *finalShellApp) runCommand() {
	p, ok := a.selectedProfile()
	if !ok {
		return
	}
	cmd := strings.TrimSpace(a.cmdInput.Text())
	if cmd == "" {
		cmd = "pwd"
	}
	a.runAsync(p, cmd)
}

func (a *finalShellApp) showSelectedHistory() {
	p, ok := a.selectedProfile()
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
	p, ok := a.selectedProfile()
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
	a.output.SetText(terminalWelcomeText())
	a.setStatus(tr("terminal.cleared"))
}

func terminalWelcomeText() string {
	return tr("terminal.welcome")
}

func (a *finalShellApp) beginCommandRun(cancel context.CancelFunc) (uint64, bool) {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.runCancel != nil {
		return 0, false
	}
	a.runID++
	a.runCancel = cancel
	return a.runID, true
}

func (a *finalShellApp) finishCommandRun(id uint64) bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.runCancel == nil || a.runID != id {
		return false
	}
	a.runCancel = nil
	return true
}

func (a *finalShellApp) stopCommand() {
	a.runMu.Lock()
	cancel := a.runCancel
	a.runMu.Unlock()
	if cancel == nil {
		a.setStatus(tr("status.no_command"))
		return
	}
	cancel()
	a.appendOutput(tr("output.stop_requested"))
	a.setStatus(tr("status.stopping"))
}

func (a *finalShellApp) setCommandRunning(running bool) {
	if a.runButton != nil && a.runButton.Raw() != nil {
		if running {
			a.runButton.Raw().Deactivate()
		} else {
			a.runButton.Raw().Activate()
		}
		a.runButton.Raw().Redraw()
	}
	if a.stopButton != nil && a.stopButton.Raw() != nil {
		if running {
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
	if err := a.persistRuntimeProfile(p); err != nil {
		a.appendOutput(trf("output.save_current_failed", err.Error()))
		a.setStatus(tr("status.save_runtime_failed"))
		a.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout())
	runID, ok := a.beginCommandRun(cancel)
	if !ok {
		cancel()
		a.setStatus(tr("status.command_active"))
		a.showTopNotice(tr("notice.command_active.title"), tr("notice.command_active.message"), false)
		return
	}
	a.appendOutput(fmt.Sprintf("\n$ [%s] %s\n", p.Name, command))
	a.setStatus(trf("status.running", p.Name))
	a.setCommandRunning(true)
	outputBatcher := newGUIOutputBatcher(func(fn func()) {
		fltk_bridge.Awake(fn)
	}, a.appendOutput)
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
			if !a.finishCommandRun(runID) {
				return
			}
			a.setCommandRunning(false)
			switch {
			case errors.Is(err, context.Canceled):
				a.appendOutput(tr("output.cancelled"))
				a.setStatus(tr("status.cancelled"))
			case errors.Is(err, context.DeadlineExceeded):
				message := trf("notice.timeout.message", commandTimeout())
				a.appendOutput(trf("output.error", message))
				a.setStatus(tr("status.timed_out"))
				a.showTopNotice(tr("notice.timeout.title"), message, true)
			case err != nil:
				a.appendOutput(trf("output.error", err.Error()))
				a.setStatus(trf("status.failed", result.ExitCode))
				a.showTopNotice(tr("notice.failed.title"), err.Error(), true)
			case len(result.Events) == 0 && result.Stdout == "" && result.Stderr == "":
				a.appendOutput(tr("output.no_output"))
				a.setStatus(trf("status.completed", 0))
			default:
				a.setStatus(trf("status.completed", result.ExitCode))
			}
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
	if err := a.store.SaveActive(nextAll, p.ID); err != nil {
		return err
	}
	a.allRows = nextAll
	a.rows = filterConnections(nextAll, "")
	a.idx = indexProfileByID(a.rows, p.ID)
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
		b.WriteString(trf("history.entry", when, entry.ExitCode, entry.Command))
	}
	return b.String()
}

func (a *finalShellApp) appendOutput(s string) {
	if a.output != nil {
		a.output.AppendAndScroll(s)
	}
}
func (a *finalShellApp) setStatus(s string) {
	if a.status != nil {
		a.status.SetText(s)
	}
}
