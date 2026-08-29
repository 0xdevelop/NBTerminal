package guis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/george012/gtbox"
)

func TestValidateSettingsDraft(t *testing.T) {
	valid := settingsDraft{Language: "zh-CN", CommandTimeoutSeconds: 45, TerminalFontSize: 16, TerminalScrollbackRows: 8000}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	for _, draft := range []settingsDraft{
		{Language: "unsupported", CommandTimeoutSeconds: 45, TerminalFontSize: 16, TerminalScrollbackRows: 8000},
		{Language: "en", CommandTimeoutSeconds: 0, TerminalFontSize: 16, TerminalScrollbackRows: 8000},
		{Language: "en", CommandTimeoutSeconds: 86401, TerminalFontSize: 16, TerminalScrollbackRows: 8000},
		{Language: "en", CommandTimeoutSeconds: 45, TerminalFontSize: config.TerminalFontSizeMin - 1, TerminalScrollbackRows: 8000},
		{Language: "en", CommandTimeoutSeconds: 45, TerminalFontSize: config.TerminalFontSizeMax + 1, TerminalScrollbackRows: 8000},
		{Language: "en", CommandTimeoutSeconds: 45, TerminalFontSize: 16, TerminalScrollbackRows: config.TerminalScrollbackRowsMin - 1},
		{Language: "en", CommandTimeoutSeconds: 45, TerminalFontSize: 16, TerminalScrollbackRows: config.TerminalScrollbackRowsMax + 1},
	} {
		if err := draft.Validate(); err == nil {
			t.Fatalf("invalid settings accepted: %#v", draft)
		}
	}
}

func TestPersistSettingsDraftCommitsAndReloads(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	path := filepath.Join(t.TempDir(), "config.json")
	config.GlobalConfig = &config.FileConfig{Language: "en", Terminal: &config.TerminalSettings{CommandTimeoutSeconds: 60, FontSize: 14, ScrollbackRows: 4000}}
	config.GlobalConfig.Normalize()
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = path

	draft := settingsDraft{
		Language:                 "zh-CN",
		CommandTimeoutSeconds:    17,
		TerminalFontSize:         18,
		TerminalScrollbackRows:   12000,
		ResetWorkspaceOnStart:    true,
		StartWithFirstConnection: true,
	}
	if err := persistSettingsDraft(draft); err != nil {
		t.Fatalf("persist settings: %v", err)
	}

	config.GlobalConfig = nil
	if err := config.LoadConfig(path); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if config.GlobalConfig.Language != "zh-CN" || config.GlobalConfig.Terminal.CommandTimeoutSeconds != 17 || config.GlobalConfig.Terminal.FontSize != 18 || config.GlobalConfig.Terminal.ScrollbackRows != 12000 ||
		!config.GlobalConfig.ResetWorkspaceOnStart || !config.GlobalConfig.StartWithFirstConnection {
		t.Fatalf("settings did not survive restart: %#v", config.GlobalConfig)
	}
}

func TestPersistSettingsDraftRollsBackOnWriteFailure(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	config.GlobalConfig = &config.FileConfig{Language: "en", Terminal: &config.TerminalSettings{CommandTimeoutSeconds: 60, FontSize: 14, ScrollbackRows: 4000}}
	config.GlobalConfig.Normalize()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")

	draft := settingsDraft{Language: "zh-CN", CommandTimeoutSeconds: 10, TerminalFontSize: 18, TerminalScrollbackRows: 12000, ResetWorkspaceOnStart: true, StartWithFirstConnection: true}
	if err := persistSettingsDraft(draft); err == nil {
		t.Fatal("expected persistence failure")
	}
	if config.GlobalConfig.Language != "en" || config.GlobalConfig.Terminal.CommandTimeoutSeconds != 60 || config.GlobalConfig.Terminal.FontSize != 14 || config.GlobalConfig.Terminal.ScrollbackRows != 4000 ||
		config.GlobalConfig.ResetWorkspaceOnStart || config.GlobalConfig.StartWithFirstConnection {
		t.Fatalf("failed save leaked into live settings: %#v", config.GlobalConfig)
	}
}
