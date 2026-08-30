package guis

// applicationShortcutRegistrar is implemented by both native windows and
// terminal views. Registering through both routes is required because a focused
// PTY terminal consumes control-key input before FLTK can deliver a window-level
// shortcut event.
type applicationShortcutRegistrar interface {
	OnShortcut(shortcut int, handler func())
}

func registerApplicationShortcut(window, terminal applicationShortcutRegistrar, shortcut int, handler func()) {
	if window == nil || terminal == nil || shortcut == 0 || handler == nil {
		return
	}
	window.OnShortcut(shortcut, handler)
	terminal.OnShortcut(shortcut, handler)
}
