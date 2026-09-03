package guis

import (
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
)

const (
	shortcutGuideWidth  = 760
	shortcutGuideHeight = 680
)

type shortcutGuideItem struct {
	Group    string
	Action   string
	Shortcut string
}

func shortcutGuideItems() []shortcutGuideItem {
	return []shortcutGuideItem{
		{Group: "Windows", Action: "Open Keyboard Shortcuts", Shortcut: "F1"},
		{Group: "Windows", Action: "Open Connection Manager", Shortcut: "Ctrl+O"},
		{Group: "Windows", Action: "Open Settings", Shortcut: "Ctrl+,"},
		{Group: "Connections", Action: "Focus Quick Launcher", Shortcut: "Ctrl+K"},
		{Group: "Connections", Action: "Create Connection", Shortcut: "Ctrl+N"},
		{Group: "Sessions", Action: "Open Local Shell", Shortcut: "Ctrl+Shift+N"},
		{Group: "Sessions", Action: "Next Session", Shortcut: "Ctrl+Tab"},
		{Group: "Sessions", Action: "Previous Session", Shortcut: "Ctrl+Shift+Tab"},
		{Group: "Sessions", Action: "Select Session", Shortcut: "Alt+1…Alt+9"},
		{Group: "Sessions", Action: "Move Session Left", Shortcut: "Ctrl+Shift+PageUp"},
		{Group: "Sessions", Action: "Move Session Right", Shortcut: "Ctrl+Shift+PageDown"},
		{Group: "Sessions", Action: "Duplicate Session", Shortcut: "Ctrl+Shift+D"},
		{Group: "Sessions", Action: "Reconnect Session", Shortcut: "Ctrl+Shift+R"},
		{Group: "Sessions", Action: "Reopen Closed Session", Shortcut: "Ctrl+Shift+T"},
		{Group: "Sessions", Action: "Close Session", Shortcut: "Ctrl+W"},
		{Group: "Terminal", Action: "Increase Terminal Text", Shortcut: "Ctrl+="},
		{Group: "Terminal", Action: "Decrease Terminal Text", Shortcut: "Ctrl+-"},
		{Group: "Terminal", Action: "Reset Terminal Text", Shortcut: "Ctrl+0"},
		{Group: "Terminal", Action: "Clear Active Terminal", Shortcut: "Ctrl+Shift+L"},
	}
}

func shortcutGuideCellText(item shortcutGuideItem, column int) string {
	switch column {
	case 0:
		return item.Group
	case 1:
		return item.Action
	case 2:
		return item.Shortcut
	default:
		return ""
	}
}

type shortcutGuideModel struct{ items []shortcutGuideItem }

func (m *shortcutGuideModel) NumberOfRows(_ *tableview.TableView) int { return len(m.items) }

func (m *shortcutGuideModel) CellForColumn(_ *tableview.TableView, row, column int) *tableview.TableViewCell {
	cell := tableview.NewCell("shortcut-guide-cell")
	if row >= 0 && row < len(m.items) {
		cell.SetText(shortcutGuideCellText(m.items[row], column))
	}
	return cell
}

type shortcutGuideWindow struct {
	owner  *finalShellApp
	window *uikit.UIWindow
	table  *uikit.UITableView
	model  *shortcutGuideModel
}

func (a *finalShellApp) openShortcutGuide() {
	if a == nil {
		return
	}
	if a.shortcuts != nil && a.shortcuts.window != nil && !a.shortcuts.window.IsClosed() {
		a.shortcuts.window.Show()
		if raw := a.shortcuts.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}
	guide := &shortcutGuideWindow{owner: a}
	a.shortcuts = guide
	guide.build()
}

func (g *shortcutGuideWindow) build() {
	windowRect := centeredScreenRect(shortcutGuideWidth, shortcutGuideHeight)
	if g.owner != nil && g.owner.window != nil && g.owner.window.Raw() != nil {
		raw := g.owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-shortcutGuideWidth)/2, raw.YRoot()+(raw.H()-shortcutGuideHeight)/2, shortcutGuideWidth, shortcutGuideHeight)
	}
	g.window = uikit.NewWindowWithRect(windowRect, "Keyboard Shortcuts")
	g.window.SetResizable(false)
	if raw := g.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	root := g.window.RootView()
	root.SetAutomationID("shortcuts.window").SetAutomationRole("window").SetAutomationName("Keyboard Shortcuts")
	g.window.OnClose(func() {
		root.SetAutomationID("")
		if g.owner != nil && g.owner.shortcuts == g {
			g.owner.shortcuts = nil
		}
	})

	root.AddSubview(titleLabel(28, 20, 500, 30, "Keyboard Shortcuts"))
	root.AddSubview(mutedLabel(30, 52, 690, 22, "Work faster without sending application commands to the active terminal."))
	table, err := uikit.NewUITableView(28, 92, 704, 514)
	if err == nil {
		g.table = table
		g.table.SetHeaderHeight(nativeControls.TableHeaderHeight)
		g.table.SetDefaultRowHeight(21)
		g.table.View().SetAutomationID("shortcuts.table").SetAutomationName("Keyboard Shortcuts")
		g.table.AddColumn(tableview.TableColumn{Identifier: "group", Title: "Area", Width: 128})
		g.table.AddColumn(tableview.TableColumn{Identifier: "action", Title: "Action", Width: 336})
		g.table.AddColumn(tableview.TableColumn{Identifier: "shortcut", Title: "Shortcut", Width: 220})
		g.model = &shortcutGuideModel{items: shortcutGuideItems()}
		g.table.SetDataSource(g.model)
		g.table.SetBackgroundColor(tokenColor(modernTheme.card))
		g.table.ReloadData()
		root.AddSubview(g.table)
	}
	root.AddSubview(button(620, 622, 112, nativeControls.PrimaryButtonHeight, "Close", "shortcuts.close", g.window.Close))
	g.window.Show()
}
