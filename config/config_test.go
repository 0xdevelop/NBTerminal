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
	if len(GlobalConfig.Connections) != 1 || GlobalConfig.Connections[0].Type != terminal.ConnectionTypeLocal {
		t.Fatalf("expected default local connection, got %#v", GlobalConfig.Connections)
	}
}
