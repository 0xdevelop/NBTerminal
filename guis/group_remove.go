package guis

import (
	"errors"
	"fmt"
	"strings"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

const removedGroupFallback = "Ungrouped"

type groupRemovePrompt struct {
	Title, Message, Cancel, Remove string
}

func groupRemovePromptFor(path string, affected int) groupRemovePrompt {
	return groupRemovePrompt{
		Title:   "Remove Group",
		Message: fmt.Sprintf("Remove %s? %d saved connection(s) will be moved up one level; no connections or credentials will be deleted.", path, affected),
		Cancel:  "Cancel",
		Remove:  "Remove Group",
	}
}

func confirmGroupRemove(path string, affected int, choose titledChoiceFunc) bool {
	if choose == nil || affected < 1 {
		return false
	}
	prompt := groupRemovePromptFor(path, affected)
	// Keep Cancel as the native default, matching profile deletion.
	return choose(prompt.Title, prompt.Message, prompt.Remove, prompt.Cancel) == 0
}

// removeConnectionGroup removes only the hierarchy node. Exact members move to
// its parent and descendants preserve their relative suffix. Saved connections,
// encrypted credentials, and all other profile fields remain untouched.
func removeConnectionGroup(rows []connectionProfile, path, fallback string) ([]connectionProfile, int, error) {
	path = normalizeConnectionGroup(path)
	fallback = normalizeConnectionGroup(fallback)
	if path == "" {
		return nil, 0, errors.New("select a group to remove")
	}
	parent := ""
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		parent = path[:slash]
	}
	next := append([]connectionProfile(nil), rows...)
	changed := 0
	for index := range next {
		group := normalizeConnectionGroup(next[index].Group)
		if !groupContainsPath(path, group) {
			continue
		}
		suffix := strings.TrimPrefix(group, path)
		suffix = strings.TrimPrefix(suffix, "/")
		destination := normalizeConnectionGroup(strings.Trim(strings.Join([]string{parent, suffix}, "/"), "/"))
		if destination == "" {
			destination = fallback
		}
		next[index].Group = destination
		changed++
	}
	if changed == 0 {
		return nil, 0, errors.New("selected group no longer exists")
	}
	return next, changed, nil
}

func (m *connectionManagerWindow) removeSelectedGroup() {
	if m == nil || m.owner == nil || m.owner.store == nil {
		return
	}
	path := normalizeConnectionGroup(m.selectedGroup)
	if path == "" {
		m.owner.showTopNotice("Remove Group", "Select a concrete group first.", true)
		return
	}
	if editor := m.owner.editor; editor != nil && groupContainsPath(path, editor.profile.Group) {
		if editor.queueAfterCloseForProfile(editor.profile.ID, m.removeSelectedGroup) {
			editor.window.RequestClose()
			return
		}
	}
	// Defer the nested native choice until the current button event completes.
	fltk_bridge.AddTimeout(0, func() {
		if m.owner == nil || normalizeConnectionGroup(m.selectedGroup) != path {
			return
		}
		next, affected, err := removeConnectionGroup(m.owner.allRows, path, removedGroupFallback)
		if err != nil {
			m.owner.showTopNotice("Remove Group", err.Error(), true)
			return
		}
		if !confirmGroupRemove(path, affected, uikit.TitledChoice) {
			return
		}
		// Re-resolve after the modal interaction so stale manager state cannot
		// move a different hierarchy.
		next, affected, err = removeConnectionGroup(m.owner.allRows, path, removedGroupFallback)
		if err != nil || normalizeConnectionGroup(m.selectedGroup) != path {
			return
		}
		activeID := m.owner.store.ActiveID()
		if err := m.owner.store.SaveActive(next, activeID); err != nil {
			m.owner.showTopNotice("Remove Group", err.Error(), true)
			return
		}
		parent := ""
		if slash := strings.LastIndex(path, "/"); slash >= 0 {
			parent = path[:slash]
		}
		m.owner.allRows = next
		m.owner.refreshNavigator(activeID)
		m.owner.refreshTable()
		m.selectedGroup = parent
		m.reload(activeID)
		m.owner.setStatus(fmt.Sprintf("Removed %s and moved %d connection(s)", path, affected))
	})
}
