package guis

import (
	"strings"
	"testing"

	"github.com/0xdevelop/NBTerminal/locales"
)

func TestUnsavedChangesPromptIsLocalizedAndFailsClosed(t *testing.T) {
	previous := locales.CurrentLanguage()
	t.Cleanup(func() { locales.ResetLocaleLanguage(previous.LanguageTag()) })

	for _, language := range locales.SupportedLanguages() {
		locales.ResetLocaleLanguage(language.LanguageTag())
		prompt := unsavedChangesPromptFor("connection")
		if strings.TrimSpace(prompt.Title) == "" || strings.TrimSpace(prompt.Message) == "" ||
			strings.TrimSpace(prompt.KeepEditing) == "" || strings.TrimSpace(prompt.Discard) == "" {
			t.Fatalf("%s unsaved prompt is incomplete: %#v", language.LanguageTag(), prompt)
		}
		var options []string
		if confirmDiscardUnsaved(prompt, func(_, _ string, got ...string) int {
			options = append([]string(nil), got...)
			return 1
		}) {
			t.Fatalf("%s default keep-editing choice discarded changes", language.LanguageTag())
		}
		if len(options) != 2 || options[0] != prompt.Discard || options[1] != prompt.KeepEditing {
			t.Fatalf("%s unsafe option order: %#v", language.LanguageTag(), options)
		}
		if !confirmDiscardUnsaved(prompt, func(_, _ string, _ ...string) int { return 0 }) {
			t.Fatalf("%s explicit discard was rejected", language.LanguageTag())
		}
		if confirmDiscardUnsaved(prompt, nil) {
			t.Fatalf("%s unavailable dialog must keep editing", language.LanguageTag())
		}
	}
}

func TestConnectionEditorDirtyStateIncludesCanonicalFieldsAndNewPassword(t *testing.T) {
	profile := connectionProfile{
		ID: "prod", Name: "Production", Group: "Infra/Prod", Type: connectionTypeSSH,
		Host: "prod.internal", Port: 22, Username: "deploy", WorkingDir: "/srv", PrivateKey: "~/.ssh/id_ed25519",
	}
	baseline := connectionEditorDraftForProfile(profile)
	if connectionEditorDraftChanged(baseline, baseline, "") {
		t.Fatal("unchanged connection draft reported dirty")
	}
	changed := baseline
	changed.Host = "other.internal"
	if !connectionEditorDraftChanged(baseline, changed, "") {
		t.Fatal("changed connection field did not report dirty")
	}
	if !connectionEditorDraftChanged(baseline, baseline, "replacement password") {
		t.Fatal("new password did not report dirty")
	}
}

func TestSettingsDirtyStateUsesCompleteDraft(t *testing.T) {
	baseline := settingsDraft{Language: "en", CommandTimeoutSeconds: 60}
	if settingsDraftChanged(baseline, baseline) {
		t.Fatal("unchanged settings reported dirty")
	}
	changed := baseline
	changed.StartWithFirstConnection = true
	if !settingsDraftChanged(baseline, changed) {
		t.Fatal("changed startup behavior did not report dirty")
	}
}
