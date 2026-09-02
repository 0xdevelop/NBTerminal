package guis

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/checkbox"
	uidropdown "github.com/0xdevelop/fltk2go/uikit/dropdown"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	connectionManagerWidth  = 920
	connectionManagerHeight = 650
)

// connectionManagerWindow owns the complete saved-profile surface. The main
// window intentionally presents only favorites/recent connections so terminal
// sessions are not coupled to editing state.
type connectionManagerWindow struct {
	owner             *finalShellApp
	window            *uikit.UIWindow
	group             *uidropdown.UIDropdown
	search            *uikit.Input
	table             *uikit.UITableView
	model             *tableModel
	rows              []connectionProfile
	idx               int
	status            *uikit.UILabel
	closeAfterConnect *checkbox.UICheckbox
	groupOptions      []connectionGroupOption
	selectedGroup     string
	groupRename       *groupRenameWindow
}

type connectionGroupOption struct {
	Path  string
	Label string
}

func (a *finalShellApp) openConnectionManager() {
	if a == nil {
		return
	}
	if a.manager != nil && a.manager.window != nil && !a.manager.window.IsClosed() {
		a.manager.window.Show()
		if raw := a.manager.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}
	m := &connectionManagerWindow{owner: a, idx: -1}
	a.manager = m
	m.build()
}

func (m *connectionManagerWindow) build() {
	layout := connectionManagerLayoutFor(nativeControls)
	windowRect := centeredScreenRect(connectionManagerWidth, connectionManagerHeight)
	if m.owner != nil && m.owner.window != nil && m.owner.window.Raw() != nil {
		raw := m.owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-connectionManagerWidth)/2, raw.YRoot()+(raw.H()-connectionManagerHeight)/2, connectionManagerWidth, connectionManagerHeight)
	}
	m.window = uikit.NewWindowWithRect(windowRect, tr("manager.title"))
	m.window.SetResizable(false)
	if raw := m.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	root := m.window.RootView()
	root.SetAutomationID("connection_manager.window").SetAutomationRole("window").SetAutomationName(tr("manager.title"))
	m.window.OnClose(func() {
		if m.groupRename != nil && m.groupRename.window != nil {
			m.groupRename.window.Close()
		}
		root.SetAutomationID("")
		if m.owner != nil && m.owner.manager == m {
			m.owner.manager = nil
		}
	})

	root.AddSubview(titleLabel(28, 20, 500, 30, tr("manager.title")))
	root.AddSubview(mutedLabel(30, 52, 850, 22, tr("manager.subtitle")))
	root.AddSubview(mutedLabel(30, 99, 72, 20, tr("connections.group")))
	m.group = uidropdown.NewUIDropdown(rect(layout.Group.X, layout.Group.Y, layout.Group.Width, layout.Group.Height))
	styleDropdown(m.group)
	m.group.View().SetAutomationID("connection_manager.group").SetAutomationName(tr("manager.group_filter"))
	root.AddSubview(m.group)
	root.AddSubview(mutedLabel(312, 99, 76, 20, tr("connections.search")))
	m.search = inputNoLabel(layout.Search.X, layout.Search.Y, layout.Search.Width, layout.Search.Height, "connection_manager.search", tr("connections.search_placeholder"))
	m.search.OnChange(m.applySearch)
	m.search.OnNavigation(m.handleSearchKey)
	root.AddSubview(m.search)
	root.AddSubview(button(layout.Find.X, layout.Find.Y, layout.Find.Width, layout.Find.Height, tr("connections.find"), "connection_manager.find", m.applySearch))
	m.syncGroupOptions()
	m.group.OnSelectionChanged(func(index int, _ string) {
		if index < 0 || index >= len(m.groupOptions) {
			return
		}
		m.selectedGroup = m.groupOptions[index].Path
		m.reload("")
	})

	table, err := uikit.NewUITableView(layout.Table.X, layout.Table.Y, layout.Table.Width, layout.Table.Height)
	if err == nil {
		m.table = table
		m.table.SetHeaderHeight(nativeControls.TableHeaderHeight)
		m.table.SetDefaultRowHeight(nativeControls.TableRowHeight)
		m.table.View().SetAutomationID("connection_manager.table").SetAutomationName(tr("manager.title"))
		m.table.AddColumn(tableview.TableColumn{Identifier: "favorite", Title: tr("manager.favorite_compact"), Width: 60})
		m.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: tr("connections.group"), Width: 250})
		m.table.AddColumn(tableview.TableColumn{Identifier: "name", Title: tr("connections.name"), Width: 180})
		m.table.AddColumn(tableview.TableColumn{Identifier: "type", Title: tr("connections.type"), Width: 65})
		m.table.AddColumn(tableview.TableColumn{Identifier: "endpoint", Title: tr("connections.endpoint"), Width: 185})
		m.table.AddColumn(tableview.TableColumn{Identifier: "last", Title: tr("connections.last_used"), Width: 100})
		m.model = &tableModel{cellText: managerConnectionCellText}
		m.table.SetDataSource(m.model)
		m.table.SetDelegate(tableDelegate{onSelect: m.selectRow})
		m.table.OnActivate(m.activate)
		m.table.SetBackgroundColor(tokenColor(modernTheme.card))
		m.table.SetCustomDraw(m.drawCell)
		root.AddSubview(m.table)
	}

	m.status = mutedLabel(layout.Status.X, layout.Status.Y, layout.Status.Width, layout.Status.Height, tr("manager.selection_hint"))
	styleDynamicLabel(m.status)
	m.status.View().SetAutomationID("connection_manager.status")
	root.AddSubview(m.status)
	checkStyle := checkbox.DefaultCheckboxStyle()
	checkStyle.Font = fltk_bridge.HELVETICA
	checkStyle.FontSize = nativeTypography.Body
	checkStyle.TextColor = uint(tokenColor(modernTheme.foreground))
	checkStyle.Color = uint(tokenColor(modernTheme.background))
	m.closeAfterConnect = checkbox.NewUICheckboxWithOptions(rect(layout.CloseAfterConnect.X, layout.CloseAfterConnect.Y, layout.CloseAfterConnect.Width, layout.CloseAfterConnect.Height), tr("manager.close_after_connect"), checkStyle)
	if config.GlobalConfig != nil {
		m.closeAfterConnect.SetValue(config.GlobalConfig.CloseManagerAfterConnect)
	}
	m.closeAfterConnect.View().SetAutomationID("connection_manager.close_after_connect").SetAutomationName(tr("manager.close_after_connect"))
	m.closeAfterConnect.OnValueChanged(m.persistCloseAfterConnect)
	root.AddSubview(m.closeAfterConnect)
	root.AddSubview(button(layout.New.X, layout.New.Y, layout.New.Width, layout.New.Height, tr("action.new"), "connection_manager.new", m.newProfile))
	root.AddSubview(button(layout.Edit.X, layout.Edit.Y, layout.Edit.Width, layout.Edit.Height, tr("action.edit"), "connection_manager.edit", m.editSelected))
	root.AddSubview(button(layout.Duplicate.X, layout.Duplicate.Y, layout.Duplicate.Width, layout.Duplicate.Height, "Duplicate", "connection_manager.duplicate", m.duplicateSelected))
	root.AddSubview(button(layout.RenameGroup.X, layout.RenameGroup.Y, layout.RenameGroup.Width, layout.RenameGroup.Height, "Rename Group", "connection_manager.rename_group", m.renameSelectedGroup))
	root.AddSubview(button(layout.Delete.X, layout.Delete.Y, layout.Delete.Width, layout.Delete.Height, tr("action.delete"), "connection_manager.delete", m.deleteSelected))
	root.AddSubview(button(layout.Test.X, layout.Test.Y, layout.Test.Width, layout.Test.Height, tr("action.test"), "connection_manager.test", m.testSelected))
	root.AddSubview(button(layout.Favorite.X, layout.Favorite.Y, layout.Favorite.Width, layout.Favorite.Height, tr("manager.favorite"), "connection_manager.favorite", m.toggleFavorite))
	root.AddSubview(primaryButton(layout.Connect.X, layout.Connect.Y, layout.Connect.Width, layout.Connect.Height, tr("action.connect"), "connection_manager.connect", m.connectSelected))

	m.reload("")
	m.window.Show()
	if m.search != nil && m.search.View() != nil && m.search.View().Raw() != nil {
		if focusable, ok := m.search.View().Raw().(interface{ TakeFocus() int }); ok {
			focusable.TakeFocus()
		}
	}
}

func (m *connectionManagerWindow) handleSearchKey(action uikit.InputNavigationAction) bool {
	if m == nil || m.search == nil {
		return false
	}
	switch action {
	case uikit.InputNavigationSubmit:
		m.activate(m.idx)
		return true
	case uikit.InputNavigationNext:
		return m.moveSearchSelection(1)
	case uikit.InputNavigationPrevious:
		return m.moveSearchSelection(-1)
	case uikit.InputNavigationCancel:
		if strings.TrimSpace(m.search.Text()) == "" {
			return false
		}
		m.search.SetText("")
		m.applySearch()
		return true
	}
	return false
}

func (m *connectionManagerWindow) reload(preferredID string) {
	if m == nil || m.owner == nil {
		return
	}
	query := ""
	if m.search != nil {
		query = m.search.Text()
	}
	m.syncGroupOptions()
	m.rows = connectionManagerRows(m.owner.allRows, m.selectedGroup, query)
	m.idx = indexProfileByID(m.rows, preferredID)
	if m.idx < 0 && len(m.rows) > 0 {
		m.idx = 0
	}
	if m.model != nil {
		m.model.rows = m.rows
	}
	if m.table != nil {
		m.table.ReloadData()
		if m.idx >= 0 {
			m.table.SelectRow(m.idx)
		}
	}
	m.updateStatus()
}

func (m *connectionManagerWindow) syncGroupOptions() {
	if m == nil || m.owner == nil || m.group == nil {
		return
	}
	m.groupOptions = connectionManagerGroupOptions(m.owner.allRows)
	labels := make([]string, len(m.groupOptions))
	selected := 0
	for index, option := range m.groupOptions {
		labels[index] = option.Label
		if option.Path == "" {
			labels[index] = tr("manager.all_groups")
		}
		if option.Path == m.selectedGroup {
			selected = index
		}
	}
	if selected == 0 && m.selectedGroup != "" {
		m.selectedGroup = ""
	}
	m.group.SetOptions(labels)
	m.group.SetSelectedIndex(selected)
}

func connectionManagerGroupOptions(rows []connectionProfile) []connectionGroupOption {
	unique := make(map[string]struct{})
	for _, row := range rows {
		parts := strings.Split(normalizeConnectionGroup(row.Group), "/")
		for depth := 1; depth <= len(parts) && parts[0] != ""; depth++ {
			unique[strings.Join(parts[:depth], "/")] = struct{}{}
		}
	}
	paths := make([]string, 0, len(unique))
	for group := range unique {
		paths = append(paths, group)
	}
	sort.Strings(paths)
	options := make([]connectionGroupOption, 1, len(paths)+1)
	for _, path := range paths {
		parts := strings.Split(path, "/")
		options = append(options, connectionGroupOption{
			Path:  path,
			Label: strings.Repeat("  ", len(parts)-1) + parts[len(parts)-1],
		})
	}
	return options
}

func normalizeConnectionGroup(group string) string {
	parts := strings.Split(group, "/")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "/")
}

func compactConnectionGroup(group string) string {
	group = normalizeConnectionGroup(group)
	if index := strings.LastIndex(group, "/"); index >= 0 {
		return group[index+1:]
	}
	return group
}

func renameConnectionGroup(rows []connectionProfile, oldPath, newPath string) ([]connectionProfile, int, error) {
	oldPath = normalizeConnectionGroup(oldPath)
	newPath = normalizeConnectionGroup(newPath)
	if oldPath == "" {
		return nil, 0, errors.New("select a group to rename")
	}
	if newPath == "" {
		return nil, 0, errors.New("new group path is required")
	}
	if newPath == oldPath {
		return nil, 0, errors.New("new group path is unchanged")
	}
	if strings.HasPrefix(newPath, oldPath+"/") {
		return nil, 0, errors.New("a group cannot be moved inside itself")
	}

	next := append([]connectionProfile(nil), rows...)
	changed := 0
	for index := range next {
		group := normalizeConnectionGroup(next[index].Group)
		if group != oldPath && !strings.HasPrefix(group, oldPath+"/") {
			continue
		}
		next[index].Group = newPath + strings.TrimPrefix(group, oldPath)
		changed++
	}
	if changed == 0 {
		return nil, 0, errors.New("selected group no longer exists")
	}
	return next, changed, nil
}

func connectionManagerRows(rows []connectionProfile, group, query string) []connectionProfile {
	group = normalizeConnectionGroup(group)
	groupRows := make([]connectionProfile, 0, len(rows))
	for _, row := range rows {
		rowGroup := normalizeConnectionGroup(row.Group)
		if group != "" && rowGroup != group && !strings.HasPrefix(rowGroup, group+"/") {
			continue
		}
		groupRows = append(groupRows, row)
	}
	return filterConnections(groupRows, query)
}

func (m *connectionManagerWindow) applySearch() {
	preferredID := ""
	if profile, ok := m.selectedProfile(); ok {
		preferredID = profile.ID
	}
	m.reload(preferredID)
}

func (m *connectionManagerWindow) moveSearchSelection(delta int) bool {
	if m == nil {
		return false
	}
	next := moveNavigatorSelection(m.idx, len(m.rows), delta)
	if next < 0 {
		return false
	}
	if m.table != nil {
		return m.table.SelectRow(next)
	}
	m.selectRow(next)
	return true
}

func (m *connectionManagerWindow) selectRow(row int) {
	if m == nil || row < 0 || row >= len(m.rows) {
		return
	}
	m.idx = row
	m.updateStatus()
	if m.owner != nil && m.owner.store != nil {
		if err := m.owner.store.SetActive(m.rows[row].ID); err != nil {
			gtbox_log.LogErrorf("save manager selection failed: %s", err.Error())
			m.owner.showTopNotice(tr("status.save_failed"), err.Error(), true)
		}
	}
	if m.table != nil {
		m.table.ReloadData()
	}
}

func (m *connectionManagerWindow) updateStatus() {
	if m == nil || m.status == nil {
		return
	}
	profile, ok := m.selectedProfile()
	if !ok {
		m.status.SetText(tr("connections.none"))
		return
	}
	m.status.SetText(managerSelectionStatus(profile))
}

func managerSelectionStatus(profile connectionProfile) string {
	favorite := tr("manager.not_favorite")
	if profile.Favorite {
		favorite = tr("manager.is_favorite")
	}
	status := trf("manager.selected_format", profile.Name, profile.Group, favorite)
	if description := strings.TrimSpace(profile.Description); description != "" {
		status += " · Description: " + description
	}
	return status
}

func (m *connectionManagerWindow) selectedProfile() (connectionProfile, bool) {
	if m == nil || m.idx < 0 || m.idx >= len(m.rows) {
		return connectionProfile{}, false
	}
	return m.rows[m.idx], true
}

func (m *connectionManagerWindow) newProfile() {
	if m != nil && m.owner != nil {
		m.owner.openConnectionEditor(newConnectionProfileInGroup(os.Getenv("USER"), m.selectedGroup))
	}
}

func (m *connectionManagerWindow) editSelected() {
	profile, ok := m.selectedProfile()
	if ok && m.owner != nil {
		m.owner.openConnectionEditor(profile)
	}
}

// duplicateSelected opens a prefilled, unsaved editor draft. Nothing is written
// until the user explicitly saves, while the existing encrypted credentials are
// retained in memory and later pass through the normal encrypted store boundary.
func (m *connectionManagerWindow) duplicateSelected() {
	profile, ok := m.selectedProfile()
	if !ok || m.owner == nil {
		return
	}
	m.owner.openConnectionEditor(duplicateConnectionProfile(profile, m.owner.allRows, time.Now().UnixNano()))
}

func duplicateConnectionProfile(source connectionProfile, existing []connectionProfile, nonce int64) connectionProfile {
	copy := source
	copy.Favorite = false
	copy.LastUsed = ""

	baseName := strings.TrimSpace(source.Name)
	if baseName == "" {
		baseName = "Connection"
	}
	usedIDs := make(map[string]struct{}, len(existing))
	usedNames := make(map[string]struct{}, len(existing))
	for _, profile := range existing {
		usedIDs[profile.ID] = struct{}{}
		usedNames[strings.ToLower(strings.TrimSpace(profile.Name))] = struct{}{}
	}
	for sequence := 1; ; sequence++ {
		id := fmt.Sprintf("copy-%d", nonce)
		name := baseName + " (Copy)"
		if sequence > 1 {
			id = fmt.Sprintf("%s-%d", id, sequence)
			name = fmt.Sprintf("%s (Copy %d)", baseName, sequence)
		}
		_, idUsed := usedIDs[id]
		_, nameUsed := usedNames[strings.ToLower(name)]
		if !idUsed && !nameUsed {
			copy.ID = id
			copy.Name = name
			return copy
		}
	}
}

func (m *connectionManagerWindow) activate(row int) {
	if m == nil || row < 0 || row >= len(m.rows) || m.owner == nil {
		return
	}
	m.idx = row
	if !m.owner.activateProfile(m.rows[row]) {
		return
	}
	if m.closeAfterConnect != nil && m.closeAfterConnect.Value() && m.window != nil {
		m.window.Close()
		return
	}
	m.reload(m.rows[row].ID)
}

func (m *connectionManagerWindow) connectSelected() { m.activate(m.idx) }

func (m *connectionManagerWindow) testSelected() {
	profile, ok := m.selectedProfile()
	if ok && m.owner != nil {
		m.owner.testProfile(profile)
	}
}

func (m *connectionManagerWindow) persistCloseAfterConnect(value bool) {
	if config.GlobalConfig == nil {
		config.GlobalConfig = &config.FileConfig{}
	}
	previous := config.GlobalConfig.CloseManagerAfterConnect
	config.GlobalConfig.CloseManagerAfterConnect = value
	if config.CurrentApp == nil || config.CurrentApp.AppConfigFilePath == "" {
		return
	}
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		config.GlobalConfig.CloseManagerAfterConnect = previous
		if m.closeAfterConnect != nil {
			m.closeAfterConnect.SetValue(previous)
		}
		if m.owner != nil {
			m.owner.setStatus(tr("status.save_failed"))
			m.owner.showTopNotice(tr("status.save_failed"), err.Error(), true)
		}
	}
}

func (m *connectionManagerWindow) toggleFavorite() {
	profile, ok := m.selectedProfile()
	if !ok || m.owner == nil {
		return
	}
	profile = toggledFavorite(profile)
	if err := m.owner.persistProfile(profile); err != nil {
		m.owner.setStatus(tr("status.save_failed"))
		m.owner.showTopNotice(tr("status.save_failed"), err.Error(), true)
		return
	}
	m.owner.refreshTable()
	m.owner.updateSelectedSummary()
	m.owner.setStatus(trf("manager.favorite_updated", profile.Name))
	m.reload(profile.ID)
}

func (m *connectionManagerWindow) deleteSelected() {
	profile, ok := m.selectedProfile()
	if !ok || m.owner == nil || m.owner.store == nil {
		return
	}
	fltk_bridge.AddTimeout(0, func() {
		m.owner.prepareProfileDeletion(profile, func() {
			if showConnectionDeleteConfirmation(profile) {
				m.deleteProfileConfirmed(profile)
			}
		})
	})
}

func (m *connectionManagerWindow) deleteProfileConfirmed(profile connectionProfile) {
	if m == nil || m.owner == nil || m.owner.store == nil {
		return
	}
	if !canDeleteSelectedProfile(m.rows, m.idx, profile.ID) {
		return
	}
	nextAll := removeProfileByID(append([]connectionProfile(nil), m.owner.allRows...), profile.ID)
	activeID := ""
	if len(nextAll) > 0 {
		activeID = nextAll[0].ID
	}
	if err := m.owner.store.SaveActive(nextAll, activeID); err != nil {
		m.owner.setStatus(tr("status.delete_failed"))
		m.owner.showTopNotice(tr("status.delete_failed"), err.Error(), true)
		return
	}
	m.owner.allRows = nextAll
	query := ""
	if m.owner.searchInput != nil {
		query = m.owner.searchInput.Text()
	}
	m.owner.rows = navigatorRows(nextAll, query, quickConnectionLimit)
	m.owner.idx = activeConnectionIndex(m.owner.rows)
	m.owner.refreshTable()
	if m.owner.table != nil && m.owner.idx >= 0 {
		m.owner.table.SelectRow(m.owner.idx)
	} else {
		m.owner.updateSelectedSummary()
	}
	m.owner.setStatus(trf("status.deleted", profile.Name))
	m.reload(activeID)
}

func canDeleteSelectedProfile(rows []connectionProfile, selected int, expectedID string) bool {
	return strings.TrimSpace(expectedID) != "" && selected >= 0 && selected < len(rows) && rows[selected].ID == expectedID
}

func (m *connectionManagerWindow) drawCell(ctx fltk_bridge.TableContext, row, col, x, y, w, h int) {
	headers := []string{tr("manager.favorite_compact"), tr("connections.group"), tr("connections.name"), tr("connections.type"), tr("connections.endpoint"), tr("connections.last_used")}
	switch ctx {
	case fltk_bridge.ContextTable:
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.card))
		fltk_bridge.PopClip()
	case fltk_bridge.ContextColHeader:
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, tokenColor(modernTheme.elevated))
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.foreground))
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, nativeTypography.Body)
		if col >= 0 && col < len(headers) {
			fltk_bridge.Draw(headers[col], x+nativeControls.TextInset, y, w-nativeControls.TextInset*2, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		}
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.border))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	case fltk_bridge.ContextCell:
		if row < 0 || row >= len(m.rows) {
			return
		}
		background := tokenColor(modernTheme.card)
		if row == m.idx {
			background = tokenColor(modernTheme.selected)
		} else if row%2 == 1 {
			background = tokenColor(modernTheme.elevated)
		}
		fltk_bridge.PushClip(x, y, w, h)
		fltk_bridge.DrawBox(fltk_bridge.FLAT_BOX, x, y, w, h, background)
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.foreground))
		fltk_bridge.SetDrawFont(fltk_bridge.HELVETICA, nativeTypography.Body)
		fltk_bridge.Draw(m.cellText(row, col), x+nativeControls.TextInset, y, w-nativeControls.TextInset*2, h, fltk_bridge.ALIGN_CENTER|fltk_bridge.ALIGN_CLIP)
		fltk_bridge.SetDrawColor(tokenColor(modernTheme.border))
		fltk_bridge.DrawRect(x, y, w, h)
		fltk_bridge.PopClip()
	}
}

func (m *connectionManagerWindow) cellText(row, col int) string {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	return managerConnectionCellText(m.rows[row], col)
}

func managerConnectionCellText(profile connectionProfile, col int) string {
	switch col {
	case 0:
		if profile.Favorite {
			return tr("manager.favorite_mark")
		}
		return ""
	case 1:
		return profile.Group
	case 2:
		return profile.Name
	case 3:
		return strings.ToUpper(string(profile.Type))
	case 4:
		return profile.tableEndpoint()
	case 5:
		return formatLastUsed(profile.LastUsed)
	default:
		return ""
	}
}
