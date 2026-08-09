package guis

import (
	"strings"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

type unsavedChangesPrompt struct {
	Title, Message, KeepEditing, Discard string
}

func unsavedChangesPromptFor(context string) unsavedChangesPrompt {
	messageKey := "unsaved.settings_message"
	if strings.EqualFold(strings.TrimSpace(context), "connection") {
		messageKey = "unsaved.connection_message"
	}
	return unsavedChangesPrompt{
		Title:       tr("unsaved.title"),
		Message:     tr(messageKey),
		KeepEditing: tr("unsaved.keep_editing"),
		Discard:     tr("unsaved.discard"),
	}
}

func confirmDiscardUnsaved(prompt unsavedChangesPrompt, choose titledChoiceFunc) bool {
	if choose == nil {
		return false
	}
	// As with destructive deletion, FLTK focuses its second, left-side option.
	// Keep Editing therefore owns the native default and Enter fails closed.
	return choose(prompt.Title, prompt.Message, prompt.Discard, prompt.KeepEditing) == 0
}

type unsavedCloseController struct {
	pending bool
}

// handle defers the native prompt out of the triggering release/close callback.
// It always vetoes the original request while a dirty draft is being reviewed;
// an explicit Discard then uses owner Close to bypass the request policy.
func (c *unsavedCloseController) handle(window *uikit.UIWindow, context string, dirty bool, onKeepEditing func()) bool {
	if window == nil || window.IsClosed() || !dirty {
		return true
	}
	if c == nil || c.pending {
		return false
	}
	c.pending = true
	fltk_bridge.AddTimeout(0, func() {
		c.pending = false
		if window.IsClosed() {
			return
		}
		if confirmDiscardUnsaved(unsavedChangesPromptFor(context), uikit.TitledChoice) {
			window.Close()
		} else if onKeepEditing != nil {
			onKeepEditing()
		}
	})
	return false
}

func settingsDraftChanged(baseline, current settingsDraft) bool {
	return baseline != current
}

func connectionEditorDraftChanged(baseline, current connectionEditorDraft, replacementPassword string) bool {
	return baseline != current || replacementPassword != ""
}
