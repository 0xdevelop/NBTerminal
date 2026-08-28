package guis

import (
	"fmt"
	"strings"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

const (
	groupRenameWidth  = 520
	groupRenameHeight = 260
)

// groupRenameWindow is a native, manager-owned editor for moving an entire
// hierarchy. It keeps group management out of the terminal workspace and makes
// the affected subtree explicit before the transactional save.
type groupRenameWindow struct {
	manager *connectionManagerWindow
	window  *uikit.UIWindow
	oldPath string
	path    *uikit.Input
	status  *uikit.UILabel
}

func (m *connectionManagerWindow) renameSelectedGroup() {
	if m == nil || m.owner == nil {
		return
	}
	oldPath := normalizeConnectionGroup(m.selectedGroup)
	if oldPath == "" {
		m.owner.showTopNotice("Rename Group", "Select a concrete group first.", true)
		return
	}
	if editor := m.owner.editor; editor != nil && groupContainsPath(oldPath, editor.profile.Group) {
		if editor.queueAfterCloseForProfile(editor.profile.ID, m.renameSelectedGroup) {
			editor.window.RequestClose()
			return
		}
	}
	if m.groupRename != nil && m.groupRename.window != nil && !m.groupRename.window.IsClosed() {
		m.groupRename.window.Show()
		if raw := m.groupRename.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}
	dialog := &groupRenameWindow{manager: m, oldPath: oldPath}
	m.groupRename = dialog
	dialog.build()
}

func groupContainsPath(parent, candidate string) bool {
	parent = normalizeConnectionGroup(parent)
	candidate = normalizeConnectionGroup(candidate)
	return parent != "" && (candidate == parent || strings.HasPrefix(candidate, parent+"/"))
}

func (d *groupRenameWindow) build() {
	if d == nil || d.manager == nil {
		return
	}
	windowRect := centeredScreenRect(groupRenameWidth, groupRenameHeight)
	if d.manager.window != nil && d.manager.window.Raw() != nil {
		raw := d.manager.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-groupRenameWidth)/2, raw.YRoot()+(raw.H()-groupRenameHeight)/2, groupRenameWidth, groupRenameHeight)
	}
	d.window = uikit.NewWindowWithRect(windowRect, "Rename Group")
	d.window.SetResizable(false)
	if raw := d.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	root := d.window.RootView()
	root.SetAutomationID("group_rename.window").SetAutomationRole("window").SetAutomationName("Rename Group")
	d.window.OnClose(func() {
		root.SetAutomationID("")
		if d.manager != nil && d.manager.groupRename == d {
			d.manager.groupRename = nil
		}
	})

	root.AddSubview(titleLabel(26, 20, 460, nativeControls.WindowTitleHeight, "Rename Group"))
	subtitle := mutedLabel(28, 54, 464, nativeControls.SupportingLineHeight*2, "Move this group and every nested connection to a new path.")
	subtitle.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_WRAP)
	root.AddSubview(subtitle)
	root.AddSubview(mutedLabel(28, 98, 464, nativeControls.FieldLabelHeight, "Current path: "+d.oldPath))
	d.path = inputNoLabel(28, 124, 464, nativeControls.InputHeight, "group_rename.path", "New group path")
	d.path.SetText(d.oldPath)
	root.AddSubview(d.path)
	d.status = mutedLabel(28, 164, 464, nativeControls.SupportingLineHeight, fmt.Sprintf("%d saved connection(s) will move.", d.affectedCount()))
	styleDynamicLabel(d.status)
	d.status.View().SetAutomationID("group_rename.status")
	root.AddSubview(d.status)
	root.AddSubview(button(286, 204, 96, nativeControls.PrimaryButtonHeight, "Cancel", "group_rename.cancel", d.close))
	root.AddSubview(primaryButton(392, 204, 100, nativeControls.PrimaryButtonHeight, "Rename", "group_rename.save", d.save))
	d.window.Show()
	if raw := d.path.View().Raw(); raw != nil {
		if focusable, ok := raw.(interface{ TakeFocus() int }); ok {
			focusable.TakeFocus()
		}
	}
}

func (d *groupRenameWindow) affectedCount() int {
	if d == nil || d.manager == nil || d.manager.owner == nil {
		return 0
	}
	count := 0
	for _, profile := range d.manager.owner.allRows {
		if groupContainsPath(d.oldPath, profile.Group) {
			count++
		}
	}
	return count
}

func (d *groupRenameWindow) save() {
	if d == nil || d.manager == nil || d.manager.owner == nil || d.manager.owner.store == nil || d.path == nil {
		return
	}
	manager := d.manager
	owner := manager.owner
	next, changed, err := renameConnectionGroup(owner.allRows, d.oldPath, d.path.Text())
	if err != nil {
		d.status.SetText(err.Error())
		return
	}
	activeID := ""
	if profile, ok := manager.selectedProfile(); ok {
		activeID = profile.ID
	}
	if err := owner.store.SaveActive(next, activeID); err != nil {
		d.status.SetText("Could not save the renamed group.")
		owner.showTopNotice("Rename Group", err.Error(), true)
		return
	}

	newPath := normalizeConnectionGroup(d.path.Text())
	owner.allRows = next
	owner.refreshNavigator(activeID)
	owner.refreshTable()
	if owner.table != nil && owner.idx >= 0 {
		owner.table.SelectRow(owner.idx)
	} else {
		owner.updateSelectedSummary()
	}
	manager.selectedGroup = newPath
	manager.reload(activeID)
	owner.setStatus(fmt.Sprintf("Moved %d connection(s) to %s", changed, newPath))
	d.close()
}

func (d *groupRenameWindow) close() {
	if d != nil && d.window != nil {
		d.window.Close()
	}
}
