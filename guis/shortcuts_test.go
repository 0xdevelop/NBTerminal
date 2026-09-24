package guis

import "testing"

func TestShortcutGuideListsEveryProductCommandWithoutDuplicateKeys(t *testing.T) {
	items := shortcutGuideItems()
	if len(items) < 21 {
		t.Fatalf("shortcut guide has %d items, want every product command", len(items))
	}
	seen := make(map[string]string, len(items))
	for _, item := range items {
		if item.Action == "" || item.Shortcut == "" || item.Group == "" {
			t.Fatalf("incomplete shortcut guide item: %#v", item)
		}
		if previous := seen[item.Shortcut]; previous != "" {
			t.Fatalf("shortcut %q is duplicated by %q and %q", item.Shortcut, previous, item.Action)
		}
		seen[item.Shortcut] = item.Action
	}
	for _, required := range []string{"F1", "F2", "Ctrl+K", "Ctrl+F", "Shift+Enter", "Shift+F10 / Menu", "Ctrl+Shift+N", "Ctrl+W", "Ctrl+Shift+T", "Ctrl+Shift+R", "Ctrl+Shift+D", "Ctrl+Shift+P", "Ctrl+Shift+B", "Ctrl+Shift+I", "Ctrl+Shift+C", "Ctrl+Shift+A", "Ctrl+Shift+S", "Ctrl+Shift+F", "F3", "Shift+F3", "Ctrl+Shift+V", "Alt+1…Alt+9"} {
		if seen[required] == "" {
			t.Fatalf("shortcut guide is missing %q", required)
		}
	}
}

func TestShortcutGuideCellTextKeepsGroupActionAndKeySeparate(t *testing.T) {
	item := shortcutGuideItem{Group: "Navigation", Action: "Focus Quick Launcher", Shortcut: "Ctrl+K"}
	if got := shortcutGuideCellText(item, 0); got != "Navigation" {
		t.Fatalf("group cell = %q", got)
	}
	if got := shortcutGuideCellText(item, 1); got != "Focus Quick Launcher" {
		t.Fatalf("action cell = %q", got)
	}
	if got := shortcutGuideCellText(item, 2); got != "Ctrl+K" {
		t.Fatalf("shortcut cell = %q", got)
	}
	if got := shortcutGuideCellText(item, 3); got != "" {
		t.Fatalf("unknown cell = %q", got)
	}
}

func TestShortcutGuideViewportFitsEveryRowWithoutScrolling(t *testing.T) {
	available := shortcutGuideTableHeight - nativeControls.TableHeaderHeight
	if capacity := available / shortcutGuideNativeRowPitch; capacity < len(shortcutGuideItems()) {
		t.Fatalf("shortcut viewport capacity = %d rows, want at least %d", capacity, len(shortcutGuideItems()))
	}
}
