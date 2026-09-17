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
	connect, copyAddress, edit, duplicate, test, favorite, delete func()
}

func connectionContextMenuItems(profile connectionProfile, actions connectionContextMenuActions) []uikit.MenuItem {
	favoriteTitle := "Add to Favorites"
	if profile.Favorite {
		favoriteTitle = "Remove from Favorites"
	}
	items := []uikit.MenuItem{{Title: "Connect", Callback: actions.connect}}
	if profile.Type == connectionTypeSSH {
		items = append(items, uikit.MenuItem{Title: "Copy Address", Callback: actions.copyAddress})
	}
	return append(items,
		uikit.MenuItem{Title: "Edit…", Callback: actions.edit},
		uikit.MenuItem{Title: "Duplicate…", Callback: actions.duplicate},
		uikit.MenuItem{Title: "Test Connection", Callback: actions.test},
		uikit.MenuItem{Title: favoriteTitle, Callback: actions.favorite},
		uikit.MenuItem{Title: "Delete…", Callback: actions.delete},
	)
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
		menu.SetMenu(connectionContextMenuItems(profile, connectionContextMenuActions{
			connect:     run(m.connectSelected),
			copyAddress: run(m.copySelectedAddress),
			edit:        run(m.editSelected),
			duplicate:   run(m.duplicateSelected),
			test:        run(m.testSelected),
			favorite:    run(m.toggleFavorite),
			delete:      run(m.deleteSelected),
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
		menu.SetMenu(connectionContextMenuItems(profile, connectionContextMenuActions{
			connect:     run(a.connectSelected),
			copyAddress: run(a.copySelectedProfileAddress),
			edit:        run(a.editSelectedProfile),
			duplicate:   run(a.duplicateSelectedProfile),
			test:        run(a.testSelectedProfile),
			favorite:    run(a.toggleSelectedProfileFavorite),
			delete:      run(a.deleteProfile),
		}))
		menu.Popup()
	})
}
