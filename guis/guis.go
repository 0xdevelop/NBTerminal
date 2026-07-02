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
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/foundation"
	"github.com/0xdevelop/fltk2go/uikit"
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
		return "local shell"
	}
	port := p.Port
	if port == 0 {
		port = 22
	}
	return fmt.Sprintf("%s@%s:%d", p.Username, p.Host, port)
}

func (p connectionProfile) tableEndpoint() string {
	if p.Type == connectionTypeLocal {
		return "local shell"
	}
	host := strings.TrimSpace(p.Host)
	if host == "" {
		host = "new host"
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
		return "never"
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
	return nil
}

func (s *connectionStore) saveLocked() error {
	s.normalizeLocked()
	buf, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, buf, 0600)
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
	s.list = append([]connectionProfile(nil), list...)
	if err := s.saveLocked(); err != nil {
		return err
	}
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
			s.list[i].Name = "New Connection"
		}
		if s.list[i].Group == "" {
			s.list[i].Group = "Default"
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
		{ID: "local-shell", Name: "Local Shell", Group: "Local", Type: connectionTypeLocal, LastUsed: time.Now().UTC().Format(time.RFC3339), Description: "Run commands on this workstation"},
		{ID: "example-ssh", Name: "Example SSH", Group: "Examples", Type: connectionTypeSSH, Host: "127.0.0.1", Port: 22, Username: os.Getenv("USER"), Description: "Edit and save with your own host/user/password or private key"},
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
	return config.SaveConfig(config.CurrentApp.AppConfigFilePath)
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
	rows    []connectionProfile
	idx     int

	window *uikit.UIWindow
	table  *uikit.UITableView
	model  *tableModel

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
	cmdInput    *uikit.Input
	output      *uikit.UITextView
	status      *uikit.UILabel
	notice      *uikit.UIWindow
}

func LoadGUIWithFLTKGO(_ []byte) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		fltk2go.Lock()
	}

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
	app.rows = app.store.List()
	app.build()
	fltk2go.Run()
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

	a.window = centeredWindow(winW, winH, config.CurrentApp.AppName+" - FinalShell Mode")
	if raw := a.window.Raw(); raw != nil {
		raw.SetColor(themeColor(244, 247, 251))
		raw.SetSizeRange(1120, 720, 0, 0, 20, 20, false)
	}
	root := a.window.RootView()

	root.AddSubview(titleLabel(margin, 14, 440, 28, "NBTerminal"))
	root.AddSubview(mutedLabel(margin+2, 38, 520, 22, "FinalShell-style connection manager · local + SSH command console"))
	a.status = pillLabel(rightX+rightW-410, 18, 410, 30, "Ready")
	a.status.View().SetAutomationID("app.status")
	root.AddSubview(a.status)

	left := uikit.NewUIGroup(rect(margin, 72, leftW, 786))
	left.SetBackgroundColor(uint(themeColor(255, 255, 255)))
	left.SetAutomationID("connections.panel")
	root.AddSubview(left)
	root.AddSubview(sectionTitle(margin+18, 86, 260, 24, "Connections"))
	root.AddSubview(mutedLabel(margin+18, 110, 430, 18, "Select a saved endpoint, edit its details, then test or connect."))
	root.AddSubview(mutedLabel(margin+18, 132, 70, 18, "Search"))
	a.searchInput = inputNoLabel(margin+82, 126, leftW-194, 30, "connections.search", "Search connections")
	a.searchInput.OnChange(a.jumpToSearchMatch)
	root.AddSubview(a.searchInput)
	findBtn := button(margin+390, 126, 86, 30, "Find", "connections.find", a.jumpToSearchMatch)
	root.AddSubview(findBtn)

	tv, err := uikit.NewUITableView(margin+14, 166, leftW-28, 362)
	if err == nil {
		a.table = tv
		a.table.View().SetAutomationID("connections.table").SetAutomationName("Connection table")
		a.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: "Group", Width: 78})
		a.table.AddColumn(tableview.TableColumn{Identifier: "name", Title: "Name", Width: 108})
		a.table.AddColumn(tableview.TableColumn{Identifier: "type", Title: "Type", Width: 50})
		a.table.AddColumn(tableview.TableColumn{Identifier: "endpoint", Title: "Endpoint", Width: 126})
		a.table.AddColumn(tableview.TableColumn{Identifier: "last", Title: "Last Used", Width: 96})
		a.model = &tableModel{rows: a.rows}
		a.table.SetDataSource(a.model)
		a.table.SetDelegate(tableDelegate{onSelect: a.selectRow})
		a.table.SetCustomDraw(a.drawConnectionCell)
		a.table.ReloadData()
		root.AddSubview(a.table)
	}

	root.AddSubview(sectionTitle(margin+18, 548, 260, 22, "Connection details"))
	a.nameInput = input(margin+82, 582, 164, 30, "Name", "form.name")
	root.AddSubview(a.nameInput)
	a.groupInput = input(margin+318, 582, 168, 30, "Group", "form.group")
	root.AddSubview(a.groupInput)

	a.typeInput = input(margin+82, 622, 92, 30, "Type", "form.type")
	root.AddSubview(a.typeInput)
	a.hostInput = input(margin+246, 622, 152, 30, "Host", "form.host")
	root.AddSubview(a.hostInput)
	a.portInput = input(margin+446, 622, 40, 30, "Port", "form.port")
	root.AddSubview(a.portInput)

	a.userInput = input(margin+82, 662, 164, 30, "User", "form.username")
	root.AddSubview(a.userInput)
	a.passInput = uikit.NewInputWithType(margin+318, 662, 168, 30, "Pass", uikit.SecretInput)
	styleInput(a.passInput)
	a.passInput.View().SetAutomationID("form.password")
	root.AddSubview(a.passInput)

	a.workInput = input(margin+82, 702, 404, 30, "WorkDir", "form.working_dir")
	root.AddSubview(a.workInput)

	a.keyInput = input(margin+82, 742, 404, 30, "Key", "form.key")
	root.AddSubview(a.keyInput)

	addBtn := button(margin+14, 816, 82, 34, "New", "action.new", a.newProfile)
	root.AddSubview(addBtn)
	saveBtn := button(margin+106, 816, 82, 34, "Save", "action.save", a.saveProfile)
	root.AddSubview(saveBtn)
	deleteBtn := button(margin+198, 816, 82, 34, "Delete", "action.delete", a.deleteProfile)
	root.AddSubview(deleteBtn)
	testBtn := button(margin+290, 816, 82, 34, "Test", "action.test", a.testConnection)
	root.AddSubview(testBtn)
	connectBtn := primaryButton(margin+382, 816, 118, 34, "Connect", "action.connect", a.connectSelected)
	root.AddSubview(connectBtn)

	rightPanel := uikit.NewUIGroup(rect(rightX, 72, rightW, 786))
	rightPanel.SetBackgroundColor(uint(themeColor(255, 255, 255)))
	rightPanel.SetAutomationID("terminal.panel")
	root.AddSubview(rightPanel)
	root.AddSubview(sectionTitle(rightX+18, 86, 330, 24, "Terminal / Command Console"))
	root.AddSubview(mutedLabel(rightX+18, 110, 560, 18, "Run quick diagnostics and review command history without leaving the manager."))

	a.output = uikit.NewUITextView(rect(rightX+18, 140, rightW-36, 608))
	a.output.SetAutomationID("terminal.output").SetAutomationName("Terminal output")
	a.output.SetFontSize(14)
	a.output.SetTextColor(uint(themeColor(219, 255, 231)))
	a.output.SetBackgroundColor(uint(themeColor(15, 23, 42)))
	a.output.SetText("Welcome to NBTerminal FinalShell Mode\n- Select or create a connection.\n- Use local shell for this machine or SSH for remote commands.\n- Passwords are saved encrypted in the app data store.\n\n")
	a.appendRecentHistory()
	root.AddSubview(a.output)

	historyBtn := button(rightX+18, 784, 92, 38, "History", "terminal.history", a.showSelectedHistory)
	root.AddSubview(historyBtn)
	root.AddSubview(mutedLabel(rightX+126, 764, 160, 18, "Command"))
	a.cmdInput = inputNoLabel(rightX+126, 784, rightW-292, 38, "terminal.command", "Command")
	root.AddSubview(a.cmdInput)
	runBtn := primaryButton(rightX+rightW-144, 784, 126, 38, "Run Command", "terminal.run", a.runCommand)
	root.AddSubview(runBtn)

	if len(a.rows) > 0 {
		a.selectRow(activeConnectionIndex(a.rows))
	}
	a.window.Show()
}

func activeConnectionIndex(rows []connectionProfile) int {
	if len(rows) == 0 {
		return -1
	}
	activeID := ""
	if config.GlobalConfig != nil {
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
	case fltk_bridge.ContextColHeader:
		titles := []string{"Group", "Name", "Type", "Endpoint", "Last Used"}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, themeColor(226, 232, 240))
		fltk_bridge.SetDrawColor(themeColor(30, 41, 59))
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, 13)
		if col >= 0 && col < len(titles) {
			fltk_bridge.Draw(titles[col], x+5, y, w-10, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		}
		fltk_bridge.SetDrawColor(themeColor(203, 213, 225))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	case fltk_bridge.ContextCell:
		if row < 0 || row >= len(a.rows) {
			return
		}
		bg := themeColor(255, 255, 255)
		fg := themeColor(15, 23, 42)
		if row == a.idx {
			bg = themeColor(219, 234, 254)
			fg = themeColor(30, 64, 175)
		} else if row%2 == 1 {
			bg = themeColor(248, 250, 252)
		}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, bg)
		fltk_bridge.SetDrawColor(fg)
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, 13)
		fltk_bridge.Draw(a.connectionCellText(row, col), x+6, y, w-12, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		fltk_bridge.SetDrawColor(themeColor(226, 232, 240))
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

func themeColor(r, g, b uint8) fltk_bridge.Color { return fltk_bridge.ColorFromRgb(r, g, b) }

func label(x, y, w, h int, text string) *uikit.UILabel {
	l := uikit.NewUILabel(rect(x, y, w, h), text)
	l.SetFontSize(13)
	l.SetTextColor(uint(themeColor(15, 23, 42)))
	l.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_CLIP)
	return l
}

func titleLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(20)
	l.SetTextColor(uint(themeColor(17, 24, 39)))
	return l
}

func sectionTitle(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(15)
	l.SetTextColor(uint(themeColor(30, 41, 59)))
	return l
}

func mutedLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFontSize(12)
	l.SetTextColor(uint(themeColor(100, 116, 139)))
	return l
}

func pillLabel(x, y, w, h int, text string) *uikit.UILabel {
	l := label(x, y, w, h, text)
	l.SetFrame(fltk_bridge.RFLAT_BOX)
	l.SetBackgroundColor(uint(themeColor(226, 232, 240)))
	l.SetTextColor(uint(themeColor(51, 65, 85)))
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
	in.SetTextColor(uint(themeColor(51, 65, 85)))
	in.SetBackgroundColor(uint(themeColor(255, 255, 255)))
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
		b.SetBackgroundColor(uint(themeColor(37, 99, 235)))
		b.SetTitleColor(uint(themeColor(255, 255, 255)))
		return
	}
	b.SetBackgroundColor(uint(themeColor(226, 232, 240)))
	b.SetTitleColor(uint(themeColor(30, 41, 59)))
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
	if query == "" {
		return
	}
	for i, p := range a.rows {
		if connectionMatchesQuery(p, query) {
			a.selectRow(i)
			a.setStatus("Search matched " + p.Name)
			return
		}
	}
	a.setStatus("No connection matching " + query)
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
	a.setStatus("Selected " + p.Name)
	if a.table != nil {
		a.table.ReloadData()
	}
}

func (a *finalShellApp) newProfile() {
	p := connectionProfile{ID: fmt.Sprintf("conn-%d", time.Now().UnixNano()), Name: "New SSH", Group: "Default", Type: connectionTypeSSH, Host: "", Port: 22, Username: os.Getenv("USER")}
	a.rows = append(a.rows, p)
	a.refreshTable()
	a.selectRow(len(a.rows) - 1)
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
		p.Name = "Unnamed"
	}
	if p.Group == "" {
		p.Group = "Default"
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
	if a.idx >= 0 && a.idx < len(a.rows) {
		a.rows[a.idx] = p
	} else {
		a.rows = append(a.rows, p)
		a.idx = len(a.rows) - 1
	}
	if err := a.store.SaveActive(a.rows, p.ID); err != nil {
		a.appendOutput("save failed: " + err.Error() + "\n")
		a.setStatus("Save failed")
		a.showTopNotice("Save failed", err.Error(), true)
		return
	}
	a.refreshTable()
	a.setStatus("Saved " + p.Name)
}

func (a *finalShellApp) deleteProfile() {
	if a.idx < 0 || a.idx >= len(a.rows) {
		return
	}
	name := a.rows[a.idx].Name
	a.rows = append(a.rows[:a.idx], a.rows[a.idx+1:]...)
	if a.idx >= len(a.rows) {
		a.idx = len(a.rows) - 1
	}
	activeID := ""
	if a.idx >= 0 && a.idx < len(a.rows) {
		activeID = a.rows[a.idx].ID
	}
	_ = a.store.SaveActive(a.rows, activeID)
	a.refreshTable()
	if a.idx >= 0 {
		a.selectRow(a.idx)
	}
	a.setStatus("Deleted " + name)
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
	a.appendOutput(fmt.Sprintf("[%s] ready: %s\n", p.Name, p.endpoint()))
	a.setStatus("Connected profile ready")
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
		a.appendOutput("load history failed: " + err.Error() + "\n")
		a.setStatus("History load failed")
		a.showTopNotice("History load failed", err.Error(), true)
		return
	}
	a.appendOutput(formatHistoryEntries(p, entries))
	a.setStatus(fmt.Sprintf("History: %d entries", len(entries)))
}

func (a *finalShellApp) runAsync(p connectionProfile, command string) {
	if err := a.persistRuntimeProfile(p); err != nil {
		a.appendOutput("save current connection failed: " + err.Error() + "\n")
		a.setStatus("Save failed; running with current form values")
		a.showTopNotice("Save failed", err.Error(), true)
	}
	a.appendOutput(fmt.Sprintf("\n$ [%s] %s\n", p.Name, command))
	a.setStatus("Running...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sess := terminal.NewSession(a.history)
		sess.OnEvent = func(event terminal.Event) {
			switch event.Stream {
			case terminal.StreamStdout:
				line := event.Line
				fltk_bridge.Awake(func() { a.appendOutput(line + "\n") })
			case terminal.StreamStderr:
				line := event.Line
				fltk_bridge.Awake(func() { a.appendOutput("[stderr] " + line + "\n") })
			case terminal.StreamStatus:
				exitCode := event.ExitCode
				fltk_bridge.Awake(func() { a.setStatus(fmt.Sprintf("Command finished: exit %d", exitCode)) })
			}
		}
		_, result, err := executeCommandResultWithSession(ctx, sess, p, command)
		msg := ""
		if err != nil {
			msg = fmt.Sprintf("ERROR: %s\n", err.Error())
		} else if len(result.Events) == 0 && result.Stdout == "" && result.Stderr == "" {
			msg = "[no output]\n"
		}
		fltk_bridge.Awake(func() {
			if msg != "" {
				a.appendOutput(msg)
				if !strings.HasSuffix(msg, "\n") {
					a.appendOutput("\n")
				}
			}
			if err != nil {
				a.setStatus("Command failed")
				a.showTopNotice("Command failed", err.Error(), true)
			} else {
				a.setStatus("Command completed")
			}
		})
	}()
}

// persistRuntimeProfile keeps command execution and connection persistence in
// sync. Users often edit a FinalShell-style connection and immediately press
// Test/Run without pressing Save first; the GUI should execute those form values
// and also make them available on next launch. It updates the in-memory row and
// encrypted connection store without duplicating command execution logic.
func (a *finalShellApp) persistRuntimeProfile(p connectionProfile) error {
	if a == nil || a.store == nil {
		return nil
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	found := false
	if a.idx >= 0 && a.idx < len(a.rows) && (a.rows[a.idx].ID == p.ID || a.rows[a.idx].ID == "") {
		a.rows[a.idx] = p
		found = true
	} else {
		for i := range a.rows {
			if a.rows[i].ID == p.ID {
				a.rows[i] = p
				a.idx = i
				found = true
				break
			}
		}
	}
	if !found {
		a.rows = append(a.rows, p)
		a.idx = len(a.rows) - 1
	}
	if err := a.store.SaveActive(a.rows, p.ID); err != nil {
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
	a.appendOutput("Recent commands:\n")
	for _, entry := range entries {
		a.appendOutput(fmt.Sprintf("- [%s] %s (exit %d)\n", entry.ConnectionName, entry.Command, entry.ExitCode))
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
		b.WriteString(fmt.Sprintf("[exit %d]\n", result.ExitCode))
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
		name = "selected connection"
	}
	b.WriteString(fmt.Sprintf("\nRecent history for %s:\n", name))
	if len(entries) == 0 {
		b.WriteString("- no history yet\n")
		return b.String()
	}
	for _, entry := range entries {
		when := entry.Time.Local().Format("2006-01-02 15:04:05")
		b.WriteString(fmt.Sprintf("- %s exit=%d %s\n", when, entry.ExitCode, entry.Command))
	}
	return b.String()
}

func (a *finalShellApp) appendOutput(s string) {
	if a.output != nil {
		a.output.AppendText(s)
	}
}
func (a *finalShellApp) setStatus(s string) {
	if a.status != nil {
		a.status.SetText(s)
	}
}
