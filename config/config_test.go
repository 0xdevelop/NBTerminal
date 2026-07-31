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
