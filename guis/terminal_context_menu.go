package guis

import (
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

func terminalContextMenuItems(terminal *uikit.UITerminalView, state uikit.ContextMenuState, clear func()) []uikit.MenuItem {
	copyFlags := 0
	if !state.HasSelection {
		copyFlags = fltk_bridge.MENU_INACTIVE
	}
	return []uikit.MenuItem{
		{
			Title: "Copy	Ctrl+Shift+C",
			Flags: copyFlags,
			Callback: func() {
				if terminal != nil {
					terminal.CopySelection()
				}
			},
		},
		{
			Title: "Copy All Output	Ctrl+Shift+A",
			Callback: func() {
				if terminal != nil {
					terminal.CopyAllText()
				}
			},
		},
		{
			Title: "Paste	Ctrl+Shift+V",
			Flags: fltk_bridge.MENU_DIVIDER,
			Callback: func() {
				if terminal != nil {
					terminal.PasteClipboard()
				}
			},
		},
		{Title: "Clear Terminal\tCtrl+Shift+L", Callback: clear},
	}
}

func (a *finalShellApp) installTerminalContextMenu(parent *uikit.UIGroup) {
	if a == nil || parent == nil || a.output == nil {
		return
	}
	menu := uikit.NewUIContextMenu(rect(0, 0, 0, 0))
	parent.AddSubview(menu)
	a.terminalContextMenu = menu
	a.output.OnContextMenu(func(state uikit.ContextMenuState) {
		menu.SetMenu(terminalContextMenuItems(a.output, state, a.clearTerminalOutput))
		menu.Popup()
	})
}
