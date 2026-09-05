package guis

import (
	"testing"

	"github.com/0xdevelop/fltk2go/uikit"
)

func TestTerminalFindStateCyclesMatchesAndTracksQueryChanges(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 420, 180))
	terminal.Append("old NEEDLE\r\nother\r\nnew needle")
	state := terminalFindState{}

	state.Search(terminal, "needle")
	if len(state.matches) != 2 || state.index != 0 || state.Status() != "1 of 2" {
		t.Fatalf("initial search = matches:%d index:%d status:%q", len(state.matches), state.index, state.Status())
	}
	state.Move(1)
	if state.index != 1 || state.Status() != "2 of 2" {
		t.Fatalf("next search = index:%d status:%q", state.index, state.Status())
	}
	state.Move(1)
	if state.index != 0 {
		t.Fatalf("wrapped next index = %d, want 0", state.index)
	}
	state.Move(-1)
	if state.index != 1 {
		t.Fatalf("wrapped previous index = %d, want 1", state.index)
	}

	state.Search(terminal, "missing")
	if len(state.matches) != 0 || state.index != -1 || state.Status() != "No matches" {
		t.Fatalf("missing search = matches:%d index:%d status:%q", len(state.matches), state.index, state.Status())
	}
	state.Search(terminal, "")
	if state.Status() != "Type to search" {
		t.Fatalf("empty search status = %q", state.Status())
	}
}

func TestTerminalFindStateRefreshesLiveOutputWithoutLosingCurrentMatch(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 420, 180))
	terminal.Append("first needle\r\nsecond needle")
	state := terminalFindState{}
	state.Search(terminal, "needle")
	state.Move(1)
	wantCurrent, ok := state.Current()
	if !ok {
		t.Fatal("expected current match before refresh")
	}

	terminal.Append("\r\nthird needle")
	state.Refresh(terminal)
	gotCurrent, ok := state.Current()
	if !ok || gotCurrent != wantCurrent {
		t.Fatalf("current match after refresh = %#v, %t; want %#v, true", gotCurrent, ok, wantCurrent)
	}
	if len(state.matches) != 3 || state.Status() != "2 of 3" {
		t.Fatalf("refreshed search = matches:%d index:%d status:%q", len(state.matches), state.index, state.Status())
	}
}

func TestTerminalFindStateMatchesUnicodeSimpleFoldEquivalents(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 420, 180))
	terminal.Append("SIGMA · ΟΣ / ος / οσ")
	state := terminalFindState{}

	state.Search(terminal, "οσ")
	if len(state.matches) != 3 || state.Status() != "1 of 3" {
		t.Fatalf("Unicode folded search = matches:%d status:%q", len(state.matches), state.Status())
	}
	state.Move(1)
	if state.Status() != "2 of 3" {
		t.Fatalf("Unicode folded navigation status = %q", state.Status())
	}
}

func TestTerminalFindStateCanMatchCaseExactly(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 420, 180))
	terminal.Append("Needle needle NEEDLE")
	state := terminalFindState{}
	state.Search(terminal, "needle")
	if len(state.matches) != 3 {
		t.Fatalf("case-insensitive matches = %d, want 3", len(state.matches))
	}

	state.SetCaseSensitive(terminal, true)
	match, ok := state.Current()
	if len(state.matches) != 1 || !ok || match.Column != 7 || state.Status() != "1 of 1" {
		t.Fatalf("case-sensitive search = matches:%d match:%#v current:%t status:%q", len(state.matches), match, ok, state.Status())
	}
}

func TestTerminalFindStateCanMatchWholeWords(t *testing.T) {
	terminal := uikit.NewUITerminalView(rect(0, 0, 420, 180))
	terminal.Append("prod production PROD prod_1 prod-prod")
	state := terminalFindState{}
	state.Search(terminal, "prod")
	if len(state.matches) != 6 {
		t.Fatalf("substring matches = %d, want 6", len(state.matches))
	}

	state.SetWholeWord(terminal, true)
	if len(state.matches) != 4 || state.Status() != "1 of 4" {
		t.Fatalf("whole-word search = matches:%d status:%q", len(state.matches), state.Status())
	}
}
