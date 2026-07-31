package locales

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestLanguageTagNormalization(t *testing.T) {
	cases := map[string]Language{
		"en_US.UTF-8": LanguageWithEnglish,
		"ru_RU.UTF-8": LanguageWithRussia,
		"zh_CN.UTF-8": LanguageWithZhCN,
		"zh-Hans":     LanguageWithZhCN,
		"zh_TW.Big5":  LanguageWithZhHK,
		"zh-Hant":     LanguageWithZhHK,
		"unknown":     LanguageWithEnglish,
	}
	for tag, want := range cases {
		if got := GetLanguageFromTag(tag); got != want {
			t.Errorf("GetLanguageFromTag(%q) = %v, want %v", tag, got, want)
		}
	}
}

func TestLocaleSwitchFallbackAndUTF8(t *testing.T) {
	previous := CurrentLanguage()
	t.Cleanup(func() { ResetLocaleLanguage(previous.LanguageTag()) })

	ResetLocaleLanguage("zh-CN")
	if got := T("app.connections"); got != "连接" {
		t.Fatalf("Chinese translation = %q", got)
	}
	if !utf8.ValidString(T("app.subtitle")) {
		t.Fatal("translated UI text is not valid UTF-8")
	}
	if got := T("missing.translation.key"); got != "missing.translation.key" {
		t.Fatalf("missing translation fallback = %q", got)
	}

	ResetLocaleLanguage("not-supported")
	if CurrentLanguage() != LanguageWithEnglish {
		t.Fatalf("unsupported language did not fall back to English: %v", CurrentLanguage())
	}
	if got := T("app.connections"); got != "Connections" {
		t.Fatalf("English fallback = %q", got)
	}
}

func TestDetectSystemLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "zh_TW.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := DetectSystemLanguage(); got != LanguageWithZhHK {
		t.Fatalf("DetectSystemLanguage = %v", got)
	}
}

func TestSupportedLanguageMetadataIsValidUTF8(t *testing.T) {
	wantTags := []string{"en", "ru", "zh-HK", "zh-CN"}
	languages := SupportedLanguages()
	if len(languages) != len(wantTags) {
		t.Fatalf("supported language count = %d, want %d", len(languages), len(wantTags))
	}
	for i, lang := range languages {
		if lang.LanguageTag() != wantTags[i] {
			t.Fatalf("language %d tag = %q, want %q", i, lang.LanguageTag(), wantTags[i])
		}
		if lang.LanguageTag() == "" || !utf8.ValidString(lang.String()) {
			t.Fatalf("invalid language metadata: tag=%q name=%q", lang.LanguageTag(), lang.String())
		}
	}
}

func TestEmbeddedLocaleFilesAreStrictUTF8JSON(t *testing.T) {
	entries, err := localeFiles.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := localeFiles.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if !utf8.Valid(data) {
			t.Fatalf("%s is not strict UTF-8", entry.Name())
		}
		if !json.Valid(data) {
			t.Fatalf("%s is not valid JSON", entry.Name())
		}
	}
}

func TestEmbeddedLocalesExposeIdenticalKeySets(t *testing.T) {
	var english map[string]string
	if err := json.Unmarshal(mustReadLocale(t, "en.json"), &english); err != nil {
		t.Fatal(err)
	}
	wantKeys := sortedKeys(english)
	for _, name := range []string{"ru.json", "zh-HK.json", "zh-CN.json"} {
		var messages map[string]string
		if err := json.Unmarshal(mustReadLocale(t, name), &messages); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if got := sortedKeys(messages); !reflect.DeepEqual(got, wantKeys) {
			t.Fatalf("%s key set differs from en.json\ngot:  %v\nwant: %v", name, got, wantKeys)
		}
	}
}

func TestLegacyLocaleBaselineTranslationsRemainCompatible(t *testing.T) {
	want := map[string]map[string]string{
		"en.json":    {"setting.title": "Settings", "button.load": "Load", "encryption.title": "Tools"},
		"ru.json":    {"encryption.title": "инструмент"},
		"zh-HK.json": {"encryption.title": "工具"},
		"zh-CN.json": {"encryption.title": "工具"},
	}
	for name, expected := range want {
		var messages map[string]string
		if err := json.Unmarshal(mustReadLocale(t, name), &messages); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for key, value := range expected {
			if messages[key] != value {
				t.Errorf("%s %q = %q, want compatibility value %q", name, key, messages[key], value)
			}
		}
	}
}

func TestConcurrentLocaleSwitchAndRead(t *testing.T) {
	previous := CurrentLanguage()
	t.Cleanup(func() { ResetLocaleLanguage(previous.LanguageTag()) })

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			languages := SupportedLanguages()
			for i := 0; i < 100; i++ {
				ResetLocaleLanguage(languages[(i+offset)%len(languages)].LanguageTag())
				if text := T("app.connections"); text == "" || !utf8.ValidString(text) {
					t.Errorf("invalid concurrent translation: %q", text)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
}

func mustReadLocale(t *testing.T, name string) []byte {
	t.Helper()
	data, err := localeFiles.ReadFile(name)
	if err != nil {
		t.Fatal(fmt.Errorf("read %s: %w", name, err))
	}
	return data
}

func sortedKeys(messages map[string]string) []string {
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
