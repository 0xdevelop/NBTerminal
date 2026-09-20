package guis

import (
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/tableview"
)

const (
	connectionSearchHelpWidth  = 760
	connectionSearchHelpHeight = 540
)

type connectionSearchHelpItem struct {
	Field   string
	Meaning string
	Example string
}

func connectionSearchHelpItems() []connectionSearchHelpItem {
	return []connectionSearchHelpItem{
		{Field: "Text", Meaning: "Match visible profile text", Example: `production database`},
		{Field: "name:", Meaning: "Match a saved connection name", Example: `name:"Primary Database"`},
		{Field: "group:", Meaning: "Match a group path", Example: `group:Production/Database`},
		{Field: "type:", Meaning: "Match the exact connection type", Example: `type:ssh`},
		{Field: "host:", Meaning: "Match an SSH host", Example: `host:db.internal`},
		{Field: "endpoint:", Meaning: "Match the displayed host and port", Example: `endpoint:db.internal:2222`},
		{Field: "port:", Meaning: "Match an exact SSH port", Example: `port:2222`},
		{Field: "description:", Meaning: "Match a profile description", Example: `description:postgres`},
		{Field: "favorite:", Meaning: "Show favorite or non-favorite profiles", Example: `favorite:true`},
		{Field: "used:", Meaning: "Match used, unused, today, or a recent day window", Example: `used:7d`},
		{Field: "Alternatives", Meaning: "Use | between alternative queries", Example: `type:local | group:Production`},
		{Field: "Exclude", Meaning: "Prefix any term with - to exclude it", Example: `type:ssh -group:Archive`},
	}
}

func connectionSearchHelpCellText(item connectionSearchHelpItem, column int) string {
	switch column {
	case 0:
		return item.Field
	case 1:
		return item.Meaning
	case 2:
		return item.Example
	default:
		return ""
	}
}

type connectionSearchHelpModel struct{ items []connectionSearchHelpItem }

func (m *connectionSearchHelpModel) NumberOfRows(_ *tableview.TableView) int { return len(m.items) }

func (m *connectionSearchHelpModel) CellForColumn(_ *tableview.TableView, row, column int) *tableview.TableViewCell {
	cell := tableview.NewCell("connection-search-help-cell")
	if row >= 0 && row < len(m.items) {
		cell.SetText(connectionSearchHelpCellText(m.items[row], column))
	}
	return cell
}

type connectionSearchHelpWindow struct {
	owner  *finalShellApp
	window *uikit.UIWindow
	table  *uikit.UITableView
	model  *connectionSearchHelpModel
}

func (a *finalShellApp) openConnectionSearchHelp() {
	if a == nil {
		return
	}
	if a.searchHelp != nil && a.searchHelp.window != nil && !a.searchHelp.window.IsClosed() {
		a.searchHelp.window.Show()
		if raw := a.searchHelp.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}
	help := &connectionSearchHelpWindow{owner: a}
	a.searchHelp = help
	help.build()
}

func (h *connectionSearchHelpWindow) build() {
	windowRect := centeredScreenRect(connectionSearchHelpWidth, connectionSearchHelpHeight)
	if h.owner != nil && h.owner.window != nil && h.owner.window.Raw() != nil {
		raw := h.owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-connectionSearchHelpWidth)/2, raw.YRoot()+(raw.H()-connectionSearchHelpHeight)/2, connectionSearchHelpWidth, connectionSearchHelpHeight)
	}
	h.window = uikit.NewWindowWithRect(windowRect, "Connection Search Help")
	h.window.SetResizable(false)
	if raw := h.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	root := h.window.RootView()
	root.SetAutomationID("connection_search_help.window").SetAutomationRole("window").SetAutomationName("Connection Search Help")
	h.window.OnClose(func() {
		root.SetAutomationID("")
		if h.owner != nil && h.owner.searchHelp == h {
			h.owner.searchHelp = nil
		}
	})

	root.AddSubview(titleLabel(28, 20, 500, 30, "Connection Search Help"))
	root.AddSubview(mutedLabel(30, 52, 700, 22, "Combine terms to narrow saved connections. Search never inspects passwords, private keys, or working directories."))
	table, err := uikit.NewUITableView(28, 88, 704, 388)
	if err == nil {
		h.table = table
		h.table.SetHeaderHeight(nativeControls.TableHeaderHeight)
		h.table.SetDefaultRowHeight(29)
		h.table.View().SetAutomationID("connection_search_help.table").SetAutomationName("Connection search syntax")
		h.table.AddColumn(tableview.TableColumn{Identifier: "field", Title: "Filter", Width: 116})
		h.table.AddColumn(tableview.TableColumn{Identifier: "meaning", Title: "Matches", Width: 344})
		h.table.AddColumn(tableview.TableColumn{Identifier: "example", Title: "Example", Width: 224})
		h.model = &connectionSearchHelpModel{items: connectionSearchHelpItems()}
		h.table.SetDataSource(h.model)
		h.table.SetBackgroundColor(tokenColor(modernTheme.card))
		h.table.ReloadData()
		root.AddSubview(h.table)
	}
	root.AddSubview(button(620, 490, 112, nativeControls.PrimaryButtonHeight, "Close", "connection_search_help.close", h.window.Close))
	h.window.Show()
}
