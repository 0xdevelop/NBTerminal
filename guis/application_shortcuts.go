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
	NewLocalSession    func()
	ZoomTerminalIn     func()
	ZoomTerminalOut    func()
	ResetTerminalZoom  func()
	NextSession        func()
	PreviousSession    func()
	CloseSession       func()
	ReopenSession      func()
	ReconnectSession   func()
	DuplicateSession   func()
	MoveSessionLeft    func()
	MoveSessionRight   func()
	SelectSession      func(index int)
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
// resource library, Ctrl+Tab / Ctrl+Shift+Tab cycle runtime sessions, and Ctrl+W
// closes the active session without leaking control bytes into the focused PTY;
// Ctrl+Shift+T reopens the most recently closed profile as a fresh runtime,
// Ctrl+Shift+R replaces the active tab's transport without replacing its tab,
// Ctrl+Shift+N opens an ephemeral local shell without changing saved profiles;
// Ctrl+= / Ctrl+- adjust terminal text and Ctrl+0 restores its default size,
// Ctrl+Shift+D opens the active profile in an independent runtime tab, and
// Ctrl+Shift+PageUp/PageDown moves the active runtime without replacing it.
// Alt+1 through Alt+9 select a runtime tab directly without sending an escape
// sequence to the focused shell.
func registerDefaultApplicationShortcuts(window, terminal applicationShortcutRegistrar, actions applicationShortcutActions) {
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('k'), actions.FocusQuickLauncher)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('n'), actions.NewConnection)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('o'), actions.OpenConnections)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int(','), actions.OpenSettings)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+int('n'), actions.NewLocalSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('='), actions.ZoomTerminalIn)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('+'), actions.ZoomTerminalIn)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('-'), actions.ZoomTerminalOut)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('0'), actions.ResetTerminalZoom)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.TAB, actions.NextSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+fltk_bridge.TAB, actions.PreviousSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+int('w'), actions.CloseSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+int('t'), actions.ReopenSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+int('r'), actions.ReconnectSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+int('d'), actions.DuplicateSession)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+fltk_bridge.PAGE_UP, actions.MoveSessionLeft)
	registerApplicationShortcut(window, terminal, fltk_bridge.CTRL+fltk_bridge.SHIFT+fltk_bridge.PAGE_DOWN, actions.MoveSessionRight)
	if actions.SelectSession != nil {
		for index := 0; index < 9; index++ {
			index := index
			registerApplicationShortcut(window, terminal, fltk_bridge.ALT+int('1'+rune(index)), func() {
				actions.SelectSession(index)
			})
		}
	}
}
