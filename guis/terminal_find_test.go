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
