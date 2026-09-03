package guis

import "testing"

func TestShortcutGuideListsEveryProductCommandWithoutDuplicateKeys(t *testing.T) {
	items := shortcutGuideItems()
	if len(items) < 19 {
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
	for _, required := range []string{"F1", "Ctrl+K", "Ctrl+Shift+N", "Ctrl+W", "Ctrl+Shift+T", "Ctrl+Shift+R", "Ctrl+Shift+D", "Alt+1…Alt+9"} {
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
