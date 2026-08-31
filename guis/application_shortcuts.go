package guis

import "github.com/0xdevelop/fltk2go/fltk_bridge"

// applicationShortcutRegistrar is implemented by both native windows and
// terminal views. Registering through both routes is required because a focused
// PTY terminal consumes control-key input before FLTK can deliver a window-level
// shortcut event.
type applicationShortcutRegistrar interface {
	OnShortcut(shortcut int, handler func())
}

type applicationShortcutActions struct {
	FocusQuickLauncher func()
	NewConnection      func()
	OpenConnections    func()
	OpenSettings       func()
	NextSession        func()
	PreviousSession    func()
}

func registerApplicationShortcut(window, terminal applicationShortcutRegistrar, shortcut int, handler func()) {
	if window == nil || terminal == nil || shortcut == 0 || handler == nil {
		return
	}
	window.OnShortcut(shortcut, handler)
	terminal.OnShortcut(shortcut, handler)
}

// registerDefaultApplicationShortcuts keeps the window and focused terminal
// routes in sync. Ctrl+N starts a connection draft, Ctrl+O opens the saved
// resource library, and Ctrl+Tab / Ctrl+Shift+Tab cycle runtime sessions without
// leaking tab bytes into the focused PTY.
func registerDefaultApplicationShortcuts(window, terminal applicationShortcutRegistrar, actions applicationShortcutActions) {
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('k'), actions.FocusQuickLauncher)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('n'), actions.NewConnection)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('o'), actions.OpenConnections)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int(','), actions.OpenSettings)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.TAB, actions.NextSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+fltk_bridge.TAB, actions.PreviousSession)
}
