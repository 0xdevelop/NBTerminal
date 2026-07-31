package locales

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/george012/gtbox/gtbox_log"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed *.json
var localeFiles embed.FS

type Language int

const (
	LanguageWithEnglish Language = iota
	LanguageWithRussia
	LanguageWithZhHK
	LanguageWithZhCN
)

var supportedLanguages = []Language{
	LanguageWithEnglish,
	LanguageWithRussia,
	LanguageWithZhHK,
	LanguageWithZhCN,
}

func (lg Language) LanguageTag() string {
	tags := [...]string{"en", "ru", "zh-HK", "zh-CN"}
	if int(lg) < 0 || int(lg) >= len(tags) {
		return tags[LanguageWithEnglish]
	}
	return tags[lg]
}

func (lg Language) String() string {
	names := [...]string{"English", "Русский", "繁體中文", "简体中文"}
	if int(lg) < 0 || int(lg) >= len(names) {
		return names[LanguageWithEnglish]
	}
	return names[lg]
}

func SupportedLanguages() []Language {
	return append([]Language(nil), supportedLanguages...)
}

func GetLanguageFromTag(tag string) Language {
	normalized := strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexByte(normalized, ':'); i >= 0 {
		normalized = normalized[:i]
	}
	if i := strings.IndexByte(normalized, '.'); i >= 0 {
		normalized = normalized[:i]
	}
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch {
	case normalized == "ru" || strings.HasPrefix(normalized, "ru-"):
		return LanguageWithRussia
	case normalized == "zh-hk", normalized == "zh-tw", normalized == "zh-mo",
		strings.Contains(normalized, "hant"):
		return LanguageWithZhHK
	case normalized == "zh" || strings.HasPrefix(normalized, "zh-"):
		return LanguageWithZhCN
	default:
		return LanguageWithEnglish
	}
}

func DetectSystemLanguage() Language {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANGUAGE", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" && value != "C" && value != "POSIX" {
			return GetLanguageFromTag(value)
		}
	}
	return LanguageWithEnglish
}

type localeStore struct {
	sync.RWMutex
	localizer *i18n.Localizer
	fallback  *i18n.Localizer
	bundle    *i18n.Bundle
	language  Language
}

var (
	once    sync.Once
	current *localeStore
)

func instanceConfig() *localeStore {
	once.Do(func() {
		current = &localeStore{language: LanguageWithEnglish}
		current.initBundle()
	})
	return current
}

func (s *localeStore) initBundle() {
	s.bundle = i18n.NewBundle(language.English)
	s.bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	entries, err := localeFiles.ReadDir(".")
	if err != nil {
		gtbox_log.LogErrorf("Failed to read locales directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, readErr := localeFiles.ReadFile(entry.Name())
		if readErr != nil {
			gtbox_log.LogErrorf("Failed to read locale file %s: %v", entry.Name(), readErr)
			continue
		}
		if _, parseErr := s.bundle.ParseMessageFileBytes(data, entry.Name()); parseErr != nil {
			gtbox_log.LogErrorf("Failed to parse locale file %s: %v", entry.Name(), parseErr)
		}
	}
	s.fallback = i18n.NewLocalizer(s.bundle, LanguageWithEnglish.LanguageTag())
	s.localizer = s.fallback
}

// ResetLocaleLanguage switches the process-wide UI locale. Unsupported tags
// safely fall back to English instead of making individual widgets panic.
func ResetLocaleLanguage(locale string) {
	s := instanceConfig()
	lang := GetLanguageFromTag(locale)
	s.Lock()
	s.language = lang
	s.localizer = i18n.NewLocalizer(s.bundle, lang.LanguageTag(), LanguageWithEnglish.LanguageTag())
	s.Unlock()
}

func CurrentLanguage() Language {
	s := instanceConfig()
	s.RLock()
	defer s.RUnlock()
	return s.language
}

// GetLocalesMessage returns a translated UTF-8 string. Missing keys fall back
// to English, then to the key itself, so an incomplete translation never crashes
// the GUI or produces an empty control label.
func GetLocalesMessage(messageID string) string {
	s := instanceConfig()
	s.RLock()
	defer s.RUnlock()
	if text, err := s.localizer.Localize(&i18n.LocalizeConfig{MessageID: messageID}); err == nil && text != "" {
		return text
	}
	if text, err := s.fallback.Localize(&i18n.LocalizeConfig{MessageID: messageID}); err == nil && text != "" {
		return text
	}
	return messageID
}

func T(messageID string) string { return GetLocalesMessage(messageID) }
