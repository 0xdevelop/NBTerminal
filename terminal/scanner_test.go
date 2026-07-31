package terminal

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeUTF8PreservesMultilingualText(t *testing.T) {
	text := "你好，终端 · 日本語 · 한국어 · 🚀 · e\u0301"
	if got := normalizeUTF8(text); got != text {
		t.Fatalf("valid UTF-8 changed: %q", got)
	}
}

func TestNormalizeUTF8ReplacesInvalidBytes(t *testing.T) {
	input := string([]byte{'o', 'k', ':', 0xff, 0xfe})
	got := normalizeUTF8(input)
	if !utf8.ValidString(got) {
		t.Fatalf("output is not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "ok:") || !strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("unexpected normalized output: %q", got)
	}
}
