package guis

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

type connectionContextMenuActions struct {
	connect, copyAddress, copyCommand, edit, duplicate, test, favorite, delete func()
	moveToGroup                                                                func(string)
}

func connectionContextMenuItems(profile connectionProfile, groups []connectionGroupOption, actions connectionContextMenuActions) []uikit.MenuItem {
	favoriteTitle := "Add to Favorites"
	if profile.Favorite {
		favoriteTitle = "Remove from Favorites"
	}
	items := []uikit.MenuItem{{Title: "Connect", Callback: actions.connect}}
	if profile.Type == connectionTypeSSH {
		items = append(items,
			uikit.MenuItem{Title: "Copy Address", Callback: actions.copyAddress},
			uikit.MenuItem{Title: "Copy SSH Command", Callback: actions.copyCommand},
		)
	}
	return append(items,
		uikit.MenuItem{Title: "Edit…", Callback: actions.edit},
		uikit.MenuItem{Title: "Duplicate…", Callback: actions.duplicate},
		uikit.MenuItem{Title: "Test Connection", Callback: actions.test},
		uikit.MenuItem{Title: favoriteTitle, Callback: actions.favorite},
		uikit.MenuItem{Title: "Move to Group", Children: connectionMoveGroupItems(profile, groups, actions.moveToGroup)},
		uikit.MenuItem{Title: "Delete…", Callback: actions.delete},
	)
}

func connectionMoveGroupItems(profile connectionProfile, groups []connectionGroupOption, move func(string)) []uikit.MenuItem {
	current := normalizeConnectionGroup(profile.Group)
	if current == "" {
		current = removedGroupFallback
	}
	destinations := []string{removedGroupFallback}
	seen := map[string]struct{}{strings.ToLower(removedGroupFallback): {}}
	for _, option := range groups {
		path := normalizeConnectionGroup(option.Path)
		if path == "" {
			continue
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		destinations = append(destinations, path)
	}
	items := make([]uikit.MenuItem, 0, len(destinations))
	for _, destination := range destinations {
		destination := destination
		item := uikit.MenuItem{Title: strings.ReplaceAll(destination, "/", " › ")}
		if strings.EqualFold(destination, current) {
			item.Flags = fltk_bridge.MENU_INACTIVE
		} else if move != nil {
			item.Callback = func() { move(destination) }
		}
		items = append(items, item)
	}
	return items
}

// moveConnectionProfileGroup changes only the non-sensitive group projection.
// Credentials and every other persisted field stay byte-for-byte unchanged.
func moveConnectionProfileGroup(profile connectionProfile, destination string) (connectionProfile, bool) {
	destination = normalizeConnectionGroup(destination)
	if destination == "" {
		destination = removedGroupFallback
	}
	current := normalizeConnectionGroup(profile.Group)
	if current == "" {
		current = removedGroupFallback
	}
	if strings.EqualFold(current, destination) {
		return profile, false
	}
	profile.Group = destination
	return profile, true
}

// connectionClipboardAddress returns a paste-ready SSH address while excluding
// passwords, private-key paths, descriptions, and every other persisted field.
func connectionClipboardAddress(profile connectionProfile) (string, bool) {
	if profile.Type != connectionTypeSSH {
		return "", false
	}
	host := strings.TrimSpace(profile.Host)
	if host == "" {
		return "", false
	}
	port := profile.Port
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return "", false
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	if username := strings.TrimSpace(profile.Username); username != "" {
		address = username + "@" + address
	}
	return address, true
}

// connectionClipboardSSHCommand builds a command that can be pasted into a
// POSIX shell without allowing profile-controlled fields to become extra shell
// words or options. Credentials, key paths, descriptions, and working
// directories are deliberately excluded from this user-visible projection.
func connectionClipboardSSHCommand(profile connectionProfile) (string, bool) {
	if profile.Type != connectionTypeSSH {
		return "", false
	}
	host := strings.TrimSpace(profile.Host)
	if host == "" {
		return "", false
	}
	port := profile.Port
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return "", false
	}
	target := host
	if username := strings.TrimSpace(profile.Username); username != "" {
		target = username + "@" + host
	}
	return "ssh -p " + strconv.Itoa(port) + " -- " + shellClipboardWord(target), true
}

func shellClipboardWord(value string) string {
	safe := value != ""
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("@%_+=,./-", r) {
			continue
		}
		safe = false
		break
	}
	if safe {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// selectContextProfile resolves a menu's stable profile identity at action
// time. Search, group filters, persistence refreshes, or other windows may have
// changed row indexes while the native menu was open.
func (m *connectionManagerWindow) selectContextProfile(id string) bool {
	if m == nil {
		return false
	}
	index := indexProfileByID(m.rows, id)
	if index < 0 {
		return false
	}
	if m.table != nil {
		return m.table.SelectRow(index)
	}
	m.idx = index
	return true
}

func (m *connectionManagerWindow) installContextMenu(parent interface{ AddSubview(viewable uikit.Viewable) }) {
	if m == nil || m.table == nil || parent == nil {
		return
	}
	menu := uikit.NewUIContextMenu(rect(0, 0, 0, 0))
	parent.AddSubview(menu)
	m.contextMenu = menu
	m.table.OnContextMenu(func(request uikit.TableContextMenuState) {
		if request.Row < 0 || request.Row >= len(m.rows) {
			return
		}
		profile := m.rows[request.Row]
		run := func(action func()) func() {
			return func() {
				if m.selectContextProfile(profile.ID) && action != nil {
					action()
				}
			}
		}
		menu.SetMenu(connectionContextMenuItems(profile, connectionManagerGroupOptions(m.owner.allRows), connectionContextMenuActions{
			connect:     run(m.connectSelected),
			copyAddress: run(m.copySelectedAddress),
			copyCommand: run(m.copySelectedSSHCommand),
			edit:        run(m.editSelected),
			duplicate:   run(m.duplicateSelected),
			test:        run(m.testSelected),
			favorite:    run(m.toggleFavorite),
			moveToGroup: func(destination string) {
				if m.selectContextProfile(profile.ID) {
					m.moveSelectedToGroup(destination)
				}
			},
			delete: run(m.deleteSelected),
		}))
		menu.Popup()
	})
}

func (m *connectionManagerWindow) copySelectedAddress() {
	profile, ok := m.selectedProfile()
	if !ok {
		return
	}
	address, ok := connectionClipboardAddress(profile)
	if !ok {
		return
	}
	fltk_bridge.CopyToClipboard(address)
	if m.owner != nil {
		m.owner.setStatus("Copied address for " + profile.Name)
	}
}

func (m *connectionManagerWindow) copySelectedSSHCommand() {
	profile, ok := m.selectedProfile()
	if !ok {
		return
	}
	command, ok := connectionClipboardSSHCommand(profile)
	if !ok {
		return
	}
	fltk_bridge.CopyToClipboard(command)
	if m.owner != nil {
		m.owner.setStatus("Copied SSH command for " + profile.Name)
	}
}

func (m *connectionManagerWindow) moveSelectedToGroup(destination string) {
	profile, ok := m.selectedProfile()
	if !ok || m.owner == nil {
		return
	}
	if editor := m.owner.editor; editor != nil && editor.profile.ID == profile.ID {
		if editor.queueAfterCloseForProfile(profile.ID, func() {
			if m.selectContextProfile(profile.ID) {
				m.moveSelectedToGroup(destination)
			}
		}) {
			editor.window.RequestClose()
			return
		}
	}
	moved, changed := moveConnectionProfileGroup(profile, destination)
	if !changed {
		return
	}
	if err := m.owner.persistProfile(moved); err != nil {
		m.owner.showTopNotice("Move Connection", err.Error(), true)
		return
	}
	m.owner.refreshTable()
	m.reload(moved.ID)
	m.owner.setStatus("Moved " + moved.Name + " to " + moved.Group)
}

// selectQuickContextProfile resolves the profile again when the menu command
// runs. Search changes can reorder the compact launcher while its native popup
// is open, so the original row index is not authoritative.
func (a *finalShellApp) selectQuickContextProfile(id string) bool {
	if a == nil {
		return false
	}
	index := indexProfileByID(a.rows, id)
	if index < 0 {
		return false
	}
	if a.table != nil {
		return a.table.SelectRow(index)
	}
	a.idx = index
	return true
}

func (a *finalShellApp) duplicateSelectedProfile() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	a.openConnectionEditor(duplicateConnectionProfile(profile, a.allRows, time.Now().UnixNano()))
}

func (a *finalShellApp) testSelectedProfile() {
	if profile, ok := a.selectedProfile(); ok {
		a.testProfile(profile)
	}
}

func (a *finalShellApp) toggleSelectedProfileFavorite() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	profile = toggledFavorite(profile)
	if err := a.persistProfile(profile); err != nil {
		a.setStatus(tr("status.save_failed"))
		a.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return
	}
	a.refreshTable()
	a.updateSelectedSummary()
	a.setStatus(trf("manager.favorite_updated", profile.Name))
	if a.manager != nil {
		a.manager.reload(profile.ID)
	}
}

func (a *finalShellApp) copySelectedProfileAddress() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	address, ok := connectionClipboardAddress(profile)
	if !ok {
		return
	}
	fltk_bridge.CopyToClipboard(address)
	a.setStatus("Copied address for " + profile.Name)
}

func (a *finalShellApp) copySelectedProfileSSHCommand() {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	command, ok := connectionClipboardSSHCommand(profile)
	if !ok {
		return
	}
	fltk_bridge.CopyToClipboard(command)
	a.setStatus("Copied SSH command for " + profile.Name)
}

func (a *finalShellApp) moveSelectedProfileToGroup(destination string) {
	profile, ok := a.selectedProfile()
	if !ok {
		return
	}
	if editor := a.editor; editor != nil && editor.profile.ID == profile.ID {
		if editor.queueAfterCloseForProfile(profile.ID, func() {
			if a.selectQuickContextProfile(profile.ID) {
				a.moveSelectedProfileToGroup(destination)
			}
		}) {
			editor.window.RequestClose()
			return
		}
	}
	moved, changed := moveConnectionProfileGroup(profile, destination)
	if !changed {
		return
	}
	if err := a.persistProfile(moved); err != nil {
		a.showTopNotice("Move Connection", err.Error(), true)
		return
	}
	a.refreshTable()
	a.updateSelectedSummary()
	a.setStatus("Moved " + moved.Name + " to " + moved.Group)
	if a.manager != nil {
		a.manager.reload(moved.ID)
	}
}

func (a *finalShellApp) installQuickConnectionContextMenu(parent interface{ AddSubview(viewable uikit.Viewable) }) {
	if a == nil || a.table == nil || parent == nil {
		return
	}
	menu := uikit.NewUIContextMenu(rect(0, 0, 0, 0))
	parent.AddSubview(menu)
	a.connectionContextMenu = menu
	a.table.OnContextMenu(func(request uikit.TableContextMenuState) {
		if request.Row < 0 || request.Row >= len(a.rows) {
			return
		}
		profile := a.rows[request.Row]
		run := func(action func()) func() {
			return func() {
				if a.selectQuickContextProfile(profile.ID) && action != nil {
					action()
				}
			}
		}
		menu.SetMenu(connectionContextMenuItems(profile, connectionManagerGroupOptions(a.allRows), connectionContextMenuActions{
			connect:     run(a.connectSelected),
			copyAddress: run(a.copySelectedProfileAddress),
			copyCommand: run(a.copySelectedProfileSSHCommand),
			edit:        run(a.editSelectedProfile),
			duplicate:   run(a.duplicateSelectedProfile),
			test:        run(a.testSelectedProfile),
			favorite:    run(a.toggleSelectedProfileFavorite),
			moveToGroup: func(destination string) {
				if a.selectQuickContextProfile(profile.ID) {
					a.moveSelectedProfileToGroup(destination)
				}
			},
			delete: run(a.deleteProfile),
		}))
		menu.Popup()
	})
}
