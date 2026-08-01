package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdevelop/NBTerminal/terminal"
)

func TestFileConfigNormalizeAddsDefaultConnection(t *testing.T) {
	cfg := &FileConfig{}
	cfg.Normalize()
	if cfg.Api == nil || cfg.Auth == nil || cfg.Language == "" {
		t.Fatalf("expected api/auth/language defaults, got %#v", cfg)
	}
	if cfg.Terminal == nil || cfg.Terminal.CommandTimeoutSeconds != CommandTimeoutDefaultSeconds {
		t.Fatalf("expected terminal timeout default, got %#v", cfg.Terminal)
	}
	if len(cfg.Connections) != 1 {
		t.Fatalf("expected one default connection, got %d", len(cfg.Connections))
	}
	if cfg.Connections[0].Type != terminal.ConnectionTypeLocal {
		t.Fatalf("expected local default, got %#v", cfg.Connections[0])
	}
	if cfg.ActiveConnectionID != cfg.Connections[0].ID {
		t.Fatalf("active id %q does not match first connection %q", cfg.ActiveConnectionID, cfg.Connections[0].ID)
	}
}

func TestLoadConfigKeepsOldConfigCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	oldShape := map[string]any{"language": "en", "api": map[string]any{"enabled": true, "port": 8765}}
	buf, err := json.Marshal(oldShape)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	oldGlobal := GlobalConfig
	t.Cleanup(func() { GlobalConfig = oldGlobal })
	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if GlobalConfig.Auth == nil {
		t.Fatal("expected auth defaults for old config")
	}
	if GlobalConfig.Terminal == nil || GlobalConfig.Terminal.CommandTimeoutSeconds != CommandTimeoutDefaultSeconds {
		t.Fatalf("expected terminal defaults for old config, got %#v", GlobalConfig.Terminal)
	}
	if len(GlobalConfig.Connections) != 1 || GlobalConfig.Connections[0].Type != terminal.ConnectionTypeLocal {
		t.Fatalf("expected default local connection, got %#v", GlobalConfig.Connections)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("loaded config mode = %o, want 600", got)
	}
}

func TestFileConfigNormalizeDetectsAndNormalizesLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	cfg := &FileConfig{}
	cfg.Normalize()
	if cfg.Language != "zh-CN" {
		t.Fatalf("detected language = %q", cfg.Language)
	}

	cfg.Language = "ru_RU.UTF-8"
	cfg.Normalize()
	if cfg.Language != "ru" {
		t.Fatalf("normalized language = %q", cfg.Language)
	}
}

func TestFileConfigNormalizePreservesPositiveTerminalTimeout(t *testing.T) {
	cfg := &FileConfig{Terminal: &TerminalSettings{CommandTimeoutSeconds: 7}}
	cfg.Normalize()
	if cfg.Terminal.CommandTimeoutSeconds != 7 {
		t.Fatalf("expected custom terminal timeout to be preserved, got %d", cfg.Terminal.CommandTimeoutSeconds)
	}

	cfg.Terminal.CommandTimeoutSeconds = -1
	cfg.Normalize()
	if cfg.Terminal.CommandTimeoutSeconds != CommandTimeoutDefaultSeconds {
		t.Fatalf("expected invalid timeout to reset to default, got %d", cfg.Terminal.CommandTimeoutSeconds)
	}
}

func TestFileConfigNormalizeSplitRatio(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "default", in: 0, want: WorkspaceSplitRatioDefault},
		{name: "preserved", in: 0.42, want: 0.42},
		{name: "clamped low", in: 0.01, want: WorkspaceSplitRatioMin},
		{name: "clamped high", in: 0.99, want: WorkspaceSplitRatioMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FileConfig{WorkspaceSplitRatio: tt.in}
			cfg.Normalize()
			if cfg.WorkspaceSplitRatio != tt.want {
				t.Fatalf("split ratio = %v, want %v", cfg.WorkspaceSplitRatio, tt.want)
			}
		})
	}
}

func TestFileConfigNormalizeRepairsStaleActiveConnection(t *testing.T) {
	cfg := &FileConfig{
		ActiveConnectionID: "missing",
		Connections: []terminal.Connection{
			{ID: "local-one", Name: "Local One", Type: terminal.ConnectionTypeLocal},
			{ID: "local-two", Name: "Local Two", Type: terminal.ConnectionTypeLocal},
		},
	}
	cfg.Normalize()
	if cfg.ActiveConnectionID != "local-one" {
		t.Fatalf("expected stale active id to fall back to first connection, got %q", cfg.ActiveConnectionID)
	}

	cfg.ActiveConnectionID = "local-two"
	cfg.Normalize()
	if cfg.ActiveConnectionID != "local-two" {
		t.Fatalf("expected valid active id to be preserved, got %q", cfg.ActiveConnectionID)
	}
}

func TestSaveConfigAtomicallyPersistsUTF8WithPrivatePermissions(t *testing.T) {
	oldGlobal := GlobalConfig
	t.Cleanup(func() { GlobalConfig = oldGlobal })
	GlobalConfig = &FileConfig{
		Language: "zh-CN",
		Connections: []terminal.Connection{
			{ID: "本地-🚀", Name: "中文 · 日本語 · 한국어 · 🚀 · é", Type: terminal.ConnectionTypeLocal},
		},
		ActiveConnectionID: "本地-🚀",
	}

	path := filepath.Join(t.TempDir(), "nested", "config.json")
	if err := SaveConfig(path); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %o, want 600", got)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded FileConfig
	if err := json.Unmarshal(buf, &decoded); err != nil {
		t.Fatalf("saved config is invalid JSON: %v", err)
	}
	if len(decoded.Connections) != 1 || decoded.Connections[0].Name != GlobalConfig.Connections[0].Name {
		t.Fatalf("UTF-8 config did not round-trip: %#v", decoded.Connections)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config.json.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}
