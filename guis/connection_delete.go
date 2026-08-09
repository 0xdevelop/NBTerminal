package guis

import "github.com/0xdevelop/fltk2go/uikit"

type connectionDeletePrompt struct {
	Title, Message, Cancel, Delete string
}

// connectionDeletePromptFor keeps destructive-action copy independent from the
// native dialog. This makes all four supported locales testable without opening
// a modal window and preserves the selected profile while the user reviews it.
func connectionDeletePromptFor(profile connectionProfile) connectionDeletePrompt {
	return connectionDeletePrompt{
		Title:   tr("delete.title"),
		Message: trf("delete.message", profile.Name),
		Cancel:  tr("button.cancel"),
		Delete:  tr("action.delete"),
	}
}

type titledChoiceFunc func(title, message string, options ...string) int

func confirmConnectionDelete(profile connectionProfile, choose titledChoiceFunc) bool {
	if choose == nil {
		return false
	}
	prompt := connectionDeletePromptFor(profile)
	// FLTK renders/focuses its second option on the left. Put Cancel there so
	// Enter and the native default focus fail closed; Delete remains the explicit
	// first-option result on the right.
	return choose(prompt.Title, prompt.Message, prompt.Delete, prompt.Cancel) == 0
}

func showConnectionDeleteConfirmation(profile connectionProfile) bool {
	return confirmConnectionDelete(profile, uikit.TitledChoice)
}
