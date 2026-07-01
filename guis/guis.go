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

	fltk2go "github.com/0xYeah/fltk2go"
	"github.com/0xYeah/fltk2go/fltk_bridge"
	"github.com/0xYeah/fltk2go/foundation"
	"github.com/0xYeah/fltk2go/uikit"
	"github.com/0xYeah/fltk2go/uikit/tableview"
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/george012/gtbox/gtbox_encryption"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	connectionStoreFile = "connections.json"
	secretKey           = "nbterminal-connections-v1"
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = append([]connectionProfile(nil), list...)
	return s.saveLocked()
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
	for i := range s.list {
		if s.list[i].ID == "" {
			s.list[i].ID = fmt.Sprintf("conn-%d-%d", time.Now().UnixNano(), i)
		}
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

func defaultConnections() []connectionProfile {
	return []connectionProfile{
		{ID: "local-shell", Name: "Local Shell", Group: "Local", Type: connectionTypeLocal, LastUsed: time.Now().UTC().Format(time.RFC3339), Description: "Run commands on this workstation"},
		{ID: "example-ssh", Name: "Example SSH", Group: "Examples", Type: connectionTypeSSH, Host: "127.0.0.1", Port: 22, Username: os.Getenv("USER"), Description: "Edit and save with your own host/user/password or private key"},
	}
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
		cell.SetText(p.endpoint())
	case 4:
		cell.SetText(p.LastUsed)
	}
	return cell
}

type finalShellApp struct {
	store   *connectionStore
	history *terminal.HistoryStore
	rows    []connectionProfile
	idx     int

	window *uikit.UIWindow
	table  *uikit.UITableView
	model  *tableModel

	nameInput  *uikit.Input
	groupInput *uikit.Input
	typeInput  *uikit.Input
	hostInput  *uikit.Input
	portInput  *uikit.Input
	userInput  *uikit.Input
	passInput  *uikit.Input
	keyInput   *uikit.Input
	cmdInput   *uikit.Input
	output     *uikit.UITextView
	status     *uikit.UILabel
}

func LoadGUIWithFLTKGO(_ []byte) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		fltk2go.Lock()
	}

	app := &finalShellApp{
		store:   newConnectionStore(config.CurrentApp.DataDir),
		history: terminal.NewHistoryStore(filepath.Join(config.CurrentApp.DataDir, "terminal-history.jsonl")),
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
	a.window = uikit.NewUIWindow(1180, 760, config.CurrentApp.AppName+" - FinalShell Mode")
	root := a.window.RootView()

	root.AddSubview(label(16, 10, 320, 28, "NBTerminal - FinalShell style manager"))
	a.status = label(760, 12, 400, 24, "Ready")
	a.status.SetFrame(fltk_bridge.FLAT_BOX)
	a.status.SetBackgroundColor(uint(fltk_bridge.ColorFromRgb(192, 192, 192)))
	a.status.SetAlignment(fltk_bridge.ALIGN_RIGHT | fltk_bridge.ALIGN_CLIP)
	a.status.View().SetAutomationID("app.status")
	root.AddSubview(a.status)

	left := uikit.NewUIGroup(rect(12, 44, 500, 650))
	left.SetBackgroundColor(0xF6F8FA00)
	left.SetAutomationID("connections.panel")
	root.AddSubview(left)

	tv, err := uikit.NewUITableView(22, 58, 480, 380)
	if err == nil {
		a.table = tv
		a.table.View().SetAutomationID("connections.table").SetAutomationName("Connection table")
		a.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: "Group", Width: 80})
		a.table.AddColumn(tableview.TableColumn{Identifier: "name", Title: "Name", Width: 105})
		a.table.AddColumn(tableview.TableColumn{Identifier: "type", Title: "Type", Width: 50})
		a.table.AddColumn(tableview.TableColumn{Identifier: "endpoint", Title: "Endpoint", Width: 140})
		a.table.AddColumn(tableview.TableColumn{Identifier: "last", Title: "Last Used", Width: 100})
		a.model = &tableModel{rows: a.rows}
		a.table.SetDataSource(a.model)
		a.table.SetDelegate(tableDelegate{onSelect: a.selectRow})
		a.table.ReloadData()
		root.AddSubview(a.table)
	}

	a.nameInput = input(95, 452, 150, 28, "Name", "form.name")
	root.AddSubview(a.nameInput)
	a.groupInput = input(330, 452, 168, 28, "Group", "form.group")
	root.AddSubview(a.groupInput)

	a.typeInput = input(95, 490, 90, 28, "Type", "form.type")
	root.AddSubview(a.typeInput)
	a.hostInput = input(255, 490, 145, 28, "Host", "form.host")
	root.AddSubview(a.hostInput)
	a.portInput = input(455, 490, 43, 28, "Port", "form.port")
	root.AddSubview(a.portInput)

	a.userInput = input(95, 528, 150, 28, "User", "form.username")
	root.AddSubview(a.userInput)
	a.passInput = uikit.NewInputWithType(330, 528, 168, 28, "Pass", uikit.SecretInput)
	a.passInput.View().SetAutomationID("form.password")
	root.AddSubview(a.passInput)

	a.keyInput = input(95, 566, 403, 28, "Key", "form.key")
	root.AddSubview(a.keyInput)

	addBtn := button(22, 610, 90, 32, "New", "action.new", a.newProfile)
	root.AddSubview(addBtn)
	saveBtn := button(122, 610, 90, 32, "Save", "action.save", a.saveProfile)
	root.AddSubview(saveBtn)
	deleteBtn := button(222, 610, 90, 32, "Delete", "action.delete", a.deleteProfile)
	root.AddSubview(deleteBtn)
	testBtn := button(322, 610, 90, 32, "Test", "action.test", a.testConnection)
	root.AddSubview(testBtn)
	connectBtn := button(422, 610, 80, 32, "Connect", "action.connect", a.connectSelected)
	root.AddSubview(connectBtn)

	root.AddSubview(label(532, 44, 620, 24, "Terminal / Command Console"))
	a.output = uikit.NewUITextView(rect(532, 76, 628, 560))
	a.output.SetAutomationID("terminal.output").SetAutomationName("Terminal output")
	a.output.SetFontSize(13)
	a.output.SetTextColor(0xD7FFE500)
	a.output.SetBackgroundColor(0x11182700)
	a.output.SetText("Welcome to NBTerminal FinalShell Mode\n- Select or create a connection.\n- Use local shell for this machine or SSH for remote commands.\n- Passwords are saved encrypted in the app data store.\n\n")
	a.appendRecentHistory()
	root.AddSubview(a.output)

	a.cmdInput = input(620, 650, 412, 34, "Command", "terminal.command")
	root.AddSubview(a.cmdInput)
	runBtn := button(1042, 650, 118, 34, "Run Command", "terminal.run", a.runCommand)
	root.AddSubview(runBtn)

	if len(a.rows) > 0 {
		a.selectRow(0)
	}
	a.window.Show()
}

type tableDelegate struct{ onSelect func(int) }

func (d tableDelegate) DidSelectRow(_ *tableview.TableView, row int) {
	if d.onSelect != nil {
		d.onSelect(row)
	}
}
func (d tableDelegate) RowHeight(_ *tableview.TableView, _ int) int { return 0 }

func rect(x, y, w, h int) *foundation.Rect { return &foundation.Rect{X: x, Y: y, Width: w, Height: h} }

func label(x, y, w, h int, text string) *uikit.UILabel {
	l := uikit.NewUILabel(rect(x, y, w, h), text)
	return l
}

func input(x, y, w, h int, placeholder, id string) *uikit.Input {
	in := uikit.NewInput(x, y, w, h, placeholder)
	in.View().SetAutomationID(id).SetAutomationName(placeholder)
	return in
}

func button(x, y, w, h int, title, id string, cb func()) *uikit.UIButton {
	b := uikit.NewUIButton(rect(x, y, w, h), title)
	b.View().SetAutomationID(id).SetAutomationName(title)
	b.OnTouchUpInside(cb)
	return b
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
	a.keyInput.SetText(p.PrivateKey)
	a.setStatus("Selected " + p.Name)
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
	if err := a.store.Save(a.rows); err != nil {
		a.appendOutput("save failed: " + err.Error() + "\n")
		a.setStatus("Save failed")
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
	_ = a.store.Save(a.rows)
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

func (a *finalShellApp) runAsync(p connectionProfile, command string) {
	a.appendOutput(fmt.Sprintf("\n$ [%s] %s\n", p.Name, command))
	a.setStatus("Running...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, result, err := executeCommandResult(ctx, p, command)
		if a.history != nil {
			if logErr := a.history.Append(terminal.HistoryFromResult(result)); logErr != nil {
				gtbox_log.LogErrorf("append command history failed: %s", logErr.Error())
			}
		}
		msg := out
		if err != nil {
			msg += "\nERROR: " + err.Error() + "\n"
		}
		fltk_bridge.Awake(func() {
			a.appendOutput(msg)
			if !strings.HasSuffix(msg, "\n") {
				a.appendOutput("\n")
			}
			if err != nil {
				a.setStatus("Command failed")
			} else {
				a.setStatus("Command completed")
			}
		})
	}()
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
	conn, err := profileToConnection(p)
	if err != nil {
		return "", terminal.CommandResult{Connection: conn, Command: command, ExitCode: -1}, err
	}
	result, err := terminal.NewExecutor().Run(ctx, conn, command)
	return formatCommandResult(result), result, err
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
