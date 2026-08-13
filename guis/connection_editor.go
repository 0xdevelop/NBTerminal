package guis

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	uidropdown "github.com/0xdevelop/fltk2go/uikit/dropdown"
)

const (
	connectionEditorWidth  = 660
	connectionEditorHeight = 570
)

// connectionEditorDraft is deliberately independent from native controls. It
// makes validation deterministic and ensures opening/editing a connection does
// not mutate durable state until Save succeeds.
type connectionEditorDraft struct {
	Name       string
	Group      string
	Type       string
	Host       string
	Port       string
	Username   string
	WorkingDir string
	PrivateKey string
}

type connectionEditorTypeOption struct {
	Type  connectionType
	Label string
}

func connectionEditorTypeOptions() []connectionEditorTypeOption {
	return []connectionEditorTypeOption{
		{Type: connectionTypeLocal, Label: tr("editor.type_local")},
		{Type: connectionTypeSSH, Label: tr("editor.type_ssh")},
	}
}

type editorFieldPolicy struct {
	Host, Port, Username, Password, WorkingDir, PrivateKey bool
}

func connectionEditorFieldPolicy(connType connectionType) editorFieldPolicy {
	ssh := connType == connectionTypeSSH
	return editorFieldPolicy{
		Host: ssh, Port: ssh, Username: ssh, Password: ssh,
		WorkingDir: true, PrivateKey: ssh,
	}
}

func (d connectionEditorDraft) Profile(base connectionProfile, password string) (connectionProfile, error) {
	p := base
	p.Name = strings.TrimSpace(d.Name)
	p.Group = normalizeConnectionGroup(d.Group)
	p.Type = connectionType(strings.ToLower(strings.TrimSpace(d.Type)))
	p.Host = strings.TrimSpace(d.Host)
	p.Username = strings.TrimSpace(d.Username)
	p.WorkingDir = strings.TrimSpace(d.WorkingDir)
	p.PrivateKey = strings.TrimSpace(d.PrivateKey)

	if p.Name == "" {
		p.Name = tr("profile.unnamed")
	}
	if p.Group == "" {
		p.Group = tr("profile.default_group")
	}
	if p.Type != connectionTypeLocal && p.Type != connectionTypeSSH {
		return connectionProfile{}, fmt.Errorf("%s", tr("editor.invalid_type"))
	}
	if p.Type == connectionTypeLocal {
		// Local profiles must not retain hidden SSH-only state from a previous
		// editor selection. Besides avoiding stale values when switching back to
		// SSH later, this keeps credentials and remote endpoint metadata out of
		// the durable encrypted payload when they are no longer applicable.
		p.Host = ""
		p.Port = 0
		p.Username = ""
		p.PasswordEnc = ""
		p.PrivateKey = ""
	} else {
		portText := strings.TrimSpace(d.Port)
		if portText == "" {
			p.Port = 22
		} else {
			port, err := strconv.Atoi(portText)
			if err != nil || port < 1 || port > 65535 {
				return connectionProfile{}, fmt.Errorf("%s", tr("editor.invalid_port"))
			}
			p.Port = port
		}
		if password != "" {
			p.SetPassword(password)
		}
	}
	return p, nil
}

type connectionEditor struct {
	owner              *finalShellApp
	window             *uikit.UIWindow
	profile            connectionProfile
	creating           bool
	initial            connectionEditorDraft
	closing            unsavedCloseController
	pendingReplacement *connectionProfile
	pendingAfterClose  func()

	name       *uikit.Input
	group      *uikit.Input
	connType   *uidropdown.UIDropdown
	host       *uikit.Input
	port       *uikit.Input
	username   *uikit.Input
	password   *uikit.Input
	workingDir *uikit.Input
	privateKey *uikit.Input
}

func (a *finalShellApp) openConnectionEditor(profile connectionProfile) {
	if a == nil {
		return
	}
	if a.editor != nil && a.editor.window != nil && !a.editor.window.IsClosed() {
		if a.editor.queueReplacement(profile) {
			a.editor.window.RequestClose()
			return
		}
		a.editor.window.Show()
		if raw := a.editor.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}
	e := &connectionEditor{owner: a, profile: profile, creating: indexProfileByID(a.allRows, profile.ID) < 0}
	a.editor = e
	e.build()
}

func (a *finalShellApp) newProfile() {
	a.openConnectionEditor(newConnectionProfile(os.Getenv("USER")))
}

func (a *finalShellApp) editSelectedProfile() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	a.openConnectionEditor(profile)
}

func (e *connectionEditor) build() {
	if e == nil {
		return
	}
	layout := connectionEditorLayoutFor(nativeControls)
	windowRect := centeredScreenRect(connectionEditorWidth, connectionEditorHeight)
	if e.owner != nil && e.owner.window != nil && e.owner.window.Raw() != nil {
		raw := e.owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-connectionEditorWidth)/2, raw.YRoot()+(raw.H()-connectionEditorHeight)/2, connectionEditorWidth, connectionEditorHeight)
	}
	title := tr("editor.title_edit")
	if e.creating {
		title = tr("editor.title_new")
	}
	e.window = uikit.NewWindowWithRect(windowRect, title)
	e.window.SetResizable(false)
	if raw := e.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	e.window.RootView().SetAutomationID("connection_editor.window").SetAutomationRole("window").SetAutomationName(title)
	e.window.OnClose(func() {
		replacement, replace := e.takePendingReplacement()
		afterClose := e.takePendingAfterClose()
		e.window.RootView().SetAutomationID("")
		if e.owner != nil && e.owner.editor == e {
			e.owner.editor = nil
		}
		if afterClose != nil {
			fltk_bridge.AddTimeout(0, afterClose)
		} else if replace && e.owner != nil {
			// Build the replacement on the next native event-loop turn rather
			// than constructing a top-level window inside FLTK's close callback.
			fltk_bridge.AddTimeout(0, func() { e.owner.openConnectionEditor(replacement) })
		}
	})
	e.window.OnCloseRequest(e.shouldClose)

	root := e.window.RootView()
	root.AddSubview(titleLabel(28, 22, 500, 30, title))
	root.AddSubview(mutedLabel(30, 54, 590, 36, tr("editor.subtitle")))

	e.name = editorInput(root, layout.Name, tr("field.name"), "connection_editor.name")
	e.group = editorInput(root, layout.Group, tr("field.group"), "connection_editor.group")
	root.AddSubview(mutedLabel(layout.Type.X, layout.Type.Y-nativeTypography.Supporting-nativeControls.FieldLabelGap, layout.Type.Width, 20, tr("field.type")))
	e.connType = uidropdown.NewUIDropdown(rect(layout.Type.X, layout.Type.Y, layout.Type.Width, layout.Type.Height))
	typeOptions := connectionEditorTypeOptions()
	typeLabels := make([]string, 0, len(typeOptions))
	selectedType := 0
	for index, option := range typeOptions {
		typeLabels = append(typeLabels, option.Label)
		if option.Type == e.profile.Type {
			selectedType = index
		}
	}
	e.connType.SetOptions(typeLabels)
	e.connType.SetSelectedIndex(selectedType)
	styleDropdown(e.connType)
	e.connType.View().SetAutomationID("connection_editor.type").SetAutomationName(tr("field.type"))
	e.connType.OnSelectionChanged(func(index int, _ string) {
		if index >= 0 && index < len(typeOptions) {
			e.applyFieldPolicy(typeOptions[index].Type)
		}
	})
	root.AddSubview(e.connType)
	e.host = editorInput(root, layout.Host, tr("field.host"), "connection_editor.host")
	e.port = editorInput(root, layout.Port, tr("field.port"), "connection_editor.port")
	e.username = editorInput(root, layout.Username, tr("field.user"), "connection_editor.username")
	root.AddSubview(mutedLabel(layout.Password.X, layout.Password.Y-nativeTypography.Supporting-nativeControls.FieldLabelGap, layout.Password.Width, 20, tr("field.password")))
	e.password = uikit.NewInputWithType(layout.Password.X, layout.Password.Y, layout.Password.Width, layout.Password.Height, "", uikit.SecretInput)
	styleInput(e.password)
	e.password.View().SetAutomationID("connection_editor.password").SetAutomationName(tr("field.password"))
	root.AddSubview(e.password)
	passwordHint := mutedLabel(layout.PasswordHint.X, layout.PasswordHint.Y, layout.PasswordHint.Width, layout.PasswordHint.Height, tr("editor.password_hint"))
	passwordHint.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_WRAP)
	root.AddSubview(passwordHint)
	e.workingDir = editorInput(root, layout.WorkingDir, tr("field.workdir"), "connection_editor.working_dir")
	e.privateKey = editorInput(root, layout.PrivateKey, tr("field.key"), "connection_editor.private_key")

	e.name.SetText(e.profile.Name)
	e.group.SetText(e.profile.Group)
	e.host.SetText(e.profile.Host)
	if e.profile.Port > 0 {
		e.port.SetText(strconv.Itoa(e.profile.Port))
	}
	e.username.SetText(e.profile.Username)
	e.workingDir.SetText(e.profile.WorkingDir)
	e.privateKey.SetText(e.profile.PrivateKey)
	e.applyFieldPolicy(typeOptions[selectedType].Type)
	e.initial = connectionEditorDraftForProfile(e.profile)

	root.AddSubview(button(layout.Cancel.X, layout.Cancel.Y, layout.Cancel.Width, layout.Cancel.Height, tr("button.cancel"), "connection_editor.cancel", e.requestClose))
	root.AddSubview(primaryButton(layout.Save.X, layout.Save.Y, layout.Save.Width, layout.Save.Height, tr("button.save"), "connection_editor.save", e.save))
	e.window.Show()
	if raw := e.name.View().Raw(); raw != nil {
		if focusable, ok := raw.(interface{ TakeFocus() int }); ok {
			focusable.TakeFocus()
		}
	}
}

func editorInput(root *uikit.UIView, frame layoutRect, fieldTitle, id string) *uikit.Input {
	root.AddSubview(mutedLabel(frame.X, frame.Y-nativeTypography.Supporting-nativeControls.FieldLabelGap, frame.Width, 20, fieldTitle))
	in := inputNoLabel(frame.X, frame.Y, frame.Width, frame.Height, id, fieldTitle)
	root.AddSubview(in)
	return in
}

func styleDropdown(dropdown *uidropdown.UIDropdown) {
	if dropdown == nil || dropdown.Raw() == nil {
		return
	}
	dropdown.Raw().SetBox(fltk_bridge.RFLAT_BOX)
	dropdown.Raw().SetColor(tokenColor(modernTheme.elevated))
	dropdown.Raw().SetLabelColor(tokenColor(modernTheme.foreground))
	dropdown.Raw().SetLabelSize(nativeTypography.Body)
}

func (e *connectionEditor) selectedConnectionType() (connectionType, error) {
	if e == nil || e.connType == nil {
		return "", errors.New(tr("editor.controls_unavailable"))
	}
	options := connectionEditorTypeOptions()
	index := e.connType.SelectedIndex()
	if index < 0 || index >= len(options) {
		return "", errors.New(tr("editor.invalid_type"))
	}
	return options[index].Type, nil
}

func (e *connectionEditor) applyFieldPolicy(connType connectionType) {
	if e == nil {
		return
	}
	policy := connectionEditorFieldPolicy(connType)
	for input, enabled := range map[*uikit.Input]bool{
		e.host: policy.Host, e.port: policy.Port, e.username: policy.Username,
		e.password: policy.Password, e.workingDir: policy.WorkingDir, e.privateKey: policy.PrivateKey,
	} {
		if input != nil {
			input.SetEnabled(enabled)
		}
	}
}

func (e *connectionEditor) draft() (connectionEditorDraft, error) {
	if e == nil || e.name == nil || e.group == nil || e.connType == nil || e.host == nil || e.port == nil ||
		e.username == nil || e.password == nil || e.workingDir == nil || e.privateKey == nil {
		return connectionEditorDraft{}, errors.New(tr("editor.controls_unavailable"))
	}
	connType, err := e.selectedConnectionType()
	if err != nil {
		return connectionEditorDraft{}, err
	}
	return connectionEditorDraft{
		Name:       e.name.Text(),
		Group:      e.group.Text(),
		Type:       string(connType),
		Host:       e.host.Text(),
		Port:       e.port.Text(),
		Username:   e.username.Text(),
		WorkingDir: e.workingDir.Text(),
		PrivateKey: e.privateKey.Text(),
	}, nil
}

func (e *connectionEditor) save() {
	draft, err := e.draft()
	var profile connectionProfile
	if err == nil {
		profile, err = draft.Profile(e.profile, e.password.Text())
	}
	if err == nil && e.owner != nil {
		if !canPersistEditorProfile(e.owner.allRows, profile.ID, e.creating) {
			err = errors.New(tr("editor.profile_deleted"))
		} else {
			err = e.owner.persistProfile(profile)
		}
	}
	if err != nil {
		if e.owner != nil {
			e.owner.setStatus(tr("status.save_failed"))
			e.owner.showTopNotice(tr("status.save_failed"), err.Error(), true)
		}
		return
	}
	if e.owner != nil {
		if e.owner.searchInput != nil {
			e.owner.searchInput.SetText("")
		}
		e.owner.refreshNavigator(profile.ID)
		e.owner.refreshTable()
		if e.owner.table != nil && e.owner.idx >= 0 {
			e.owner.table.SelectRow(e.owner.idx)
		} else {
			e.owner.updateSelectedSummary()
		}
		e.owner.setStatus(trf("status.saved", profile.Name))
		if e.owner.manager != nil {
			e.owner.manager.reload(profile.ID)
		}
	}
	e.close()
}

func (e *connectionEditor) close() {
	if e == nil || e.window == nil {
		return
	}
	e.window.Close()
}

func connectionEditorDraftForProfile(profile connectionProfile) connectionEditorDraft {
	port := ""
	if profile.Port > 0 {
		port = strconv.Itoa(profile.Port)
	}
	return connectionEditorDraft{
		Name: profile.Name, Group: profile.Group, Type: string(profile.Type),
		Host: profile.Host, Port: port, Username: profile.Username,
		WorkingDir: profile.WorkingDir, PrivateKey: profile.PrivateKey,
	}
}

func (e *connectionEditor) shouldClose() bool {
	if e == nil || e.window == nil {
		return true
	}
	draft, err := e.draft()
	dirty := err != nil
	if err == nil {
		dirty = connectionEditorDraftChanged(e.initial, draft, e.password.Text())
	}
	return e.closing.handle(e.window, "connection", dirty, e.cancelPendingCloseWork)
}

func (e *connectionEditor) requestClose() {
	if e == nil || e.window == nil {
		return
	}
	e.window.RequestClose()
}

// queueReplacement keeps one native editor window authoritative. Reopening the
// same profile only focuses its current draft; choosing another row first runs
// the normal close policy, then reconstructs the editor for the new target.
func (e *connectionEditor) queueReplacement(profile connectionProfile) bool {
	if e == nil || profile.ID == e.profile.ID {
		return false
	}
	copy := profile
	e.pendingReplacement = &copy
	return true
}

func (e *connectionEditor) cancelPendingReplacement() {
	if e != nil {
		e.pendingReplacement = nil
	}
}

func (e *connectionEditor) cancelPendingCloseWork() {
	if e == nil {
		return
	}
	e.cancelPendingReplacement()
	e.pendingAfterClose = nil
}

// queueAfterCloseForProfile serializes a mutation that would invalidate the
// editor's saved target. A dirty draft gets the normal Keep Editing/Discard
// policy first; only an accepted close may continue to the destructive prompt.
func (e *connectionEditor) queueAfterCloseForProfile(profileID string, action func()) bool {
	if e == nil || action == nil || strings.TrimSpace(profileID) == "" || e.profile.ID != profileID {
		return false
	}
	e.pendingReplacement = nil
	e.pendingAfterClose = action
	return true
}

func (e *connectionEditor) takePendingAfterClose() func() {
	if e == nil {
		return nil
	}
	action := e.pendingAfterClose
	e.pendingAfterClose = nil
	return action
}

func canPersistEditorProfile(rows []connectionProfile, profileID string, creating bool) bool {
	return creating || indexProfileByID(rows, profileID) >= 0
}

func (e *connectionEditor) takePendingReplacement() (connectionProfile, bool) {
	if e == nil || e.pendingReplacement == nil {
		return connectionProfile{}, false
	}
	profile := *e.pendingReplacement
	e.pendingReplacement = nil
	return profile, true
}
