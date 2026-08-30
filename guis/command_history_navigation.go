package guis

import (
	"strings"

	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

// commandHistoryCursor owns one command-line history traversal. The synthetic
// final position represents the user's in-progress draft, so walking back down
// past the newest persisted command restores exactly what was being typed.
type commandHistoryCursor struct {
	commands []string
	index    int
	draft    string
	started  bool
}

func newCommandHistoryCursor(entries []terminal.HistoryEntry) *commandHistoryCursor {
	commands := make([]string, 0, len(entries))
	for _, entry := range entries {
		command := strings.TrimSpace(entry.Command)
		if command == "" {
			continue
		}
		if len(commands) > 0 && commands[len(commands)-1] == command {
			continue
		}
		commands = append(commands, command)
	}
	return &commandHistoryCursor{commands: commands, index: len(commands)}
}

func (c *commandHistoryCursor) Move(delta int, currentDraft string) string {
	if c == nil || len(c.commands) == 0 {
		return currentDraft
	}
	if !c.started {
		c.draft = currentDraft
		c.index = len(c.commands)
		c.started = true
	}
	if delta < 0 && c.index > 0 {
		c.index--
	} else if delta > 0 && c.index < len(c.commands) {
		c.index++
	}
	if c.index == len(c.commands) {
		return c.draft
	}
	return c.commands[c.index]
}

func (a *finalShellApp) resetCommandHistoryNavigation() {
	if a == nil || len(a.commandHistory) == 0 {
		return
	}
	sessionID := a.activeSessionID()
	if sessionID == "" {
		a.commandHistory = nil
		return
	}
	delete(a.commandHistory, sessionID)
}

func (a *finalShellApp) handleCommandInputKey(event uikit.KeyEvent) bool {
	if a == nil {
		return false
	}
	if event.Key == fltk_bridge.ENTER_KEY {
		a.resetCommandHistoryNavigation()
		a.runCommand()
		return true
	}
	if event.State != 0 || (event.Key != fltk_bridge.UP && event.Key != fltk_bridge.DOWN) {
		a.resetCommandHistoryNavigation()
		return false
	}
	profile, ok := a.activeSessionProfile()
	if !ok || a.history == nil || a.cmdInput == nil {
		return false
	}
	sessionID := a.activeSessionID()
	if sessionID == "" {
		return false
	}
	if a.commandHistory == nil {
		a.commandHistory = make(map[string]*commandHistoryCursor)
	}
	cursor := a.commandHistory[sessionID]
	if cursor == nil {
		entries, err := a.history.LoadForConnection(profile.ID, 100)
		if err != nil {
			a.appendOutput(trf("output.history_failed", err.Error()))
			a.setStatus(tr("status.history_failed"))
			a.showTopNotice(tr("status.history_failed"), err.Error(), true)
			return true
		}
		cursor = newCommandHistoryCursor(entries)
		a.commandHistory[sessionID] = cursor
	}
	delta := -1
	if event.Key == fltk_bridge.DOWN {
		delta = 1
	}
	command := cursor.Move(delta, a.cmdInput.Text())
	a.cmdInput.SetText(command)
	a.cmdInput.ScrollToEnd()
	if a.sessions != nil {
		a.sessions.SetActiveDraft(command)
	}
	return true
}
