package guis

import (
	"testing"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

func TestTerminalContextMenuReflectsSelectionAndRoutesCommands(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 320, 160))
	cleared := 0
	items := terminalContextMenuItems(terminal, uikit.ContextMenuState{}, func() { cleared++ })
	if len(items) != 3 {
		t.Fatalf("context menu item count = %d, want 3", len(items))
	}
	if items[0].Title != "Copy	Ctrl+Shift+C" || items[0].Flags&fltk_bridge.MENU_INACTIVE == 0 {
		t.Fatalf("copy item without selection = %#v, want disabled Copy", items[0])
	}
	if items[1].Title != "Paste	Ctrl+Shift+V" || items[1].Flags&fltk_bridge.MENU_DIVIDER == 0 {
		t.Fatalf("paste item = %#v, want Paste followed by divider", items[1])
	}
	if items[2].Title != "Clear Terminal\tCtrl+Shift+L" {
		t.Fatalf("clear item title = %q", items[2].Title)
	}
	items[2].Callback()
	if cleared != 1 {
		t.Fatalf("clear callback count = %d, want 1", cleared)
	}

	items = terminalContextMenuItems(terminal, uikit.ContextMenuState{HasSelection: true}, func() {})
	if items[0].Flags&fltk_bridge.MENU_INACTIVE != 0 {
		t.Fatal("copy item stayed disabled for selected terminal text")
	}
}
