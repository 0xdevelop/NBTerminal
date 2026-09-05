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
