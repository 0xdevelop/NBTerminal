package guis

import (
	"testing"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
)

type shortcutRecorder struct {
	handlers map[int]func()
}

func (r *shortcutRecorder) OnShortcut(shortcut int, handler func()) {
	if r.handlers == nil {
		r.handlers = make(map[int]func())
	}
	r.handlers[shortcut] = handler
}

func TestRegisterApplicationShortcutRoutesFromWindowAndTerminal(t *testing.T) {
	window := &shortcutRecorder{}
	terminal := &shortcutRecorder{}
	invocations := 0
	shortcut := fltk_bridge.CTRL + int(',')

	registerApplicationShortcut(window, terminal, shortcut, func() { invocations++ })

	if window.handlers[shortcut] == nil || terminal.handlers[shortcut] == nil {
		t.Fatalf("shortcut %d was not registered on both native routes", shortcut)
	}
	window.handlers[shortcut]()
	terminal.handlers[shortcut]()
	if invocations != 2 {
		t.Fatalf("shortcut action invoked %d times, want 2", invocations)
	}
}

func TestRegisterApplicationShortcutRejectsIncompleteRegistration(t *testing.T) {
	window := &shortcutRecorder{}
	registerApplicationShortcut(window, nil, fltk_bridge.CTRL+int(','), func() {})
	if len(window.handlers) != 0 {
		t.Fatal("partial shortcut registration leaves the consuming terminal route uncovered")
	}
}
