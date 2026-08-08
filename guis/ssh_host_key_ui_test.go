package guis

import (
	"strings"
	"testing"

	"github.com/0xdevelop/NBTerminal/locales"
)

func TestUnknownSSHHostKeyPromptHasExplicitLocalizedSecurityTitle(t *testing.T) {
	previous := locales.CurrentLanguage()
	t.Cleanup(func() { locales.ResetLocaleLanguage(previous.LanguageTag()) })

	const fingerprint = "SHA256:REDACTED-SYNTHETIC-FINGERPRINT"
	for _, language := range locales.SupportedLanguages() {
		locales.ResetLocaleLanguage(language.LanguageTag())
		prompt := unknownSSHHostKeyPrompt(fingerprint)
		if strings.TrimSpace(prompt.Title) == "" || prompt.Title == "ssh.host_key.unknown_title" {
			t.Fatalf("%s prompt has no localized native title: %#v", language.LanguageTag(), prompt)
		}
		if !strings.Contains(prompt.Message, fingerprint) {
			t.Fatalf("%s prompt omitted the public fingerprint", language.LanguageTag())
		}
		if strings.TrimSpace(prompt.Cancel) == "" || strings.TrimSpace(prompt.Trust) == "" {
			t.Fatalf("%s prompt has an empty action label: %#v", language.LanguageTag(), prompt)
		}
	}
}
