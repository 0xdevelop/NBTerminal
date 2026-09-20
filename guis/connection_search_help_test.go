package guis

import "testing"

func TestConnectionSearchHelpDocumentsEverySupportedFilter(t *testing.T) {
	items := connectionSearchHelpItems()
	wantFields := []string{
		"Text", "name:", "group:", "type:", "host:", "endpoint:", "port:",
		"description:", "favorite:", "used:", "Alternatives", "Exclude",
	}
	if len(items) != len(wantFields) {
		t.Fatalf("help item count = %d, want %d", len(items), len(wantFields))
	}
	for index, want := range wantFields {
		if items[index].Field != want {
			t.Fatalf("help item %d field = %q, want %q", index, items[index].Field, want)
		}
		if items[index].Meaning == "" || items[index].Example == "" {
			t.Fatalf("help item %q is incomplete: %#v", want, items[index])
		}
	}
}

func TestConnectionSearchHelpAppliesSelectedExample(t *testing.T) {
	items := connectionSearchHelpItems()
	var applied string
	help := &connectionSearchHelpWindow{
		model:    &connectionSearchHelpModel{items: items},
		selected: 3,
		apply:    func(query string) { applied = query },
	}

	if !help.applySelected() {
		t.Fatal("selected search example was not applied")
	}
	if applied != items[3].Example {
		t.Fatalf("applied query = %q, want %q", applied, items[3].Example)
	}

	applied = ""
	help.selected = len(items)
	if help.applySelected() || applied != "" {
		t.Fatalf("out-of-range example applied: query=%q", applied)
	}
}

func TestConnectionSearchExampleTargetsQuickLauncherAndManager(t *testing.T) {
	rows := []connectionProfile{
		{ID: "local", Name: "Local", Type: connectionTypeLocal},
		{ID: "remote", Name: "Remote", Type: connectionTypeSSH},
	}
	quick := &finalShellApp{
		allRows:     rows,
		searchInput: inputNoLabel(0, 0, 240, nativeControls.InputHeight, "quick-search-test", ""),
		idx:         -1,
	}
	quick.applyQuickSearchExample("type:ssh")
	if quick.searchInput.Text() != "type:ssh" || len(quick.rows) != 1 || quick.rows[0].ID != "remote" {
		t.Fatalf("quick example result = query %q rows %#v", quick.searchInput.Text(), quick.rows)
	}

	manager := &connectionManagerWindow{
		owner:  &finalShellApp{allRows: rows},
		search: inputNoLabel(0, 0, 240, nativeControls.InputHeight, "manager-search-test", ""),
		idx:    -1,
	}
	manager.applySearchExample("type:local")
	if manager.search.Text() != "type:local" || len(manager.rows) != 1 || manager.rows[0].ID != "local" {
		t.Fatalf("manager example result = query %q rows %#v", manager.search.Text(), manager.rows)
	}
}

func TestSearchHelpButtonsDoNotOverlapSearchOrFind(t *testing.T) {
	quick := quickPanelLayoutFor(layoutRect{X: 22, Y: 72, Width: 430, Height: 786}, nativeControls)
	if quick.Search.Bottom() != quick.SearchHelp.Bottom() || quick.SearchHelp.Bottom() != quick.Find.Bottom() {
		t.Fatalf("quick search row is not aligned: %#v", quick)
	}
	if quick.Search.X+quick.Search.Width+8 > quick.SearchHelp.X || quick.SearchHelp.X+quick.SearchHelp.Width+8 > quick.Find.X {
		t.Fatalf("quick search/help/find controls overlap: %#v", quick)
	}

	manager := connectionManagerLayoutFor(nativeControls)
	if manager.Search.Bottom() != manager.SearchHelp.Bottom() || manager.SearchHelp.Bottom() != manager.Find.Bottom() {
		t.Fatalf("manager search row is not aligned: %#v", manager)
	}
	if manager.Search.X+manager.Search.Width+8 > manager.SearchHelp.X || manager.SearchHelp.X+manager.SearchHelp.Width+8 > manager.Find.X {
		t.Fatalf("manager search/help/find controls overlap: %#v", manager)
	}
}
