package guis

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

func TestCommandHistoryCursorMovesThroughUniqueCommandsAndRestoresDraft(t *testing.T) {
	cursor := newCommandHistoryCursor([]terminal.HistoryEntry{
		{Command: "pwd"},
		{Command: "pwd"},
		{Command: "  uname -a  "},
		{Command: ""},
		{Command: "date"},
	})

	for _, step := range []struct {
		delta int
		want  string
	}{
		{-1, "date"},
		{-1, "uname -a"},
		{-1, "pwd"},
		{-1, "pwd"},
		{1, "uname -a"},
		{1, "date"},
		{1, "draft --flag"},
		{1, "draft --flag"},
	} {
		if got := cursor.Move(step.delta, "draft --flag"); got != step.want {
			t.Fatalf("Move(%d) = %q, want %q", step.delta, got, step.want)
		}
	}
}

func TestHandleCommandInputKeyScopesHistoryAndRestoresTypedDraft(t *testing.T) {
	history := terminal.NewHistoryStore(filepath.Join(t.TempDir(), "terminal-history.jsonl"))
	for _, entry := range []terminal.HistoryEntry{
		{Time: time.Now(), ConnectionID: "other", Command: "do-not-recall"},
		{Time: time.Now(), ConnectionID: "local", Command: "printf first"},
		{Time: time.Now(), ConnectionID: "local", Command: "printf latest"},
	} {
		if err := history.Append(entry); err != nil {
			t.Fatal(err)
		}
	}
	workspace := newSessionWorkspace()
	workspace.Open(connectionProfile{ID: "local", Name: "Local Shell", Type: connectionTypeLocal})
	app := &finalShellApp{
		history:  history,
		sessions: workspace,
		cmdInput: uikit.NewUITextView(rect(0, 0, 320, 40)),
	}
	app.cmdInput.SetText("draft --flag")

	if !app.handleCommandInputKey(uikit.KeyEvent{Key: fltk_bridge.UP}) || app.cmdInput.Text() != "printf latest" {
		t.Fatalf("first Up did not recall latest scoped command: %q", app.cmdInput.Text())
	}
	if !app.handleCommandInputKey(uikit.KeyEvent{Key: fltk_bridge.UP}) || app.cmdInput.Text() != "printf first" {
		t.Fatalf("second Up did not recall older command: %q", app.cmdInput.Text())
	}
	if !app.handleCommandInputKey(uikit.KeyEvent{Key: fltk_bridge.DOWN}) || app.cmdInput.Text() != "printf latest" {
		t.Fatalf("first Down did not move toward newest command: %q", app.cmdInput.Text())
	}
	if !app.handleCommandInputKey(uikit.KeyEvent{Key: fltk_bridge.DOWN}) || app.cmdInput.Text() != "draft --flag" {
		t.Fatalf("second Down did not restore draft: %q", app.cmdInput.Text())
	}
	if got := workspace.tabs[workspace.activeIndex].CommandDraft; got != "draft --flag" {
		t.Fatalf("workspace draft = %q, want restored draft", got)
	}
}

func TestHandleCommandInputKeyResetsNavigationWhenEditing(t *testing.T) {
	cursor := newCommandHistoryCursor([]terminal.HistoryEntry{{Command: "old"}})
	app := &finalShellApp{commandHistory: map[string]*commandHistoryCursor{"runtime-1": cursor}}
	app.handleCommandInputKey(uikit.KeyEvent{Key: int('x'), Text: "x"})
	if _, ok := app.commandHistory["runtime-1"]; ok {
		t.Fatal("normal editing key did not reset stale history navigation")
	}
}
