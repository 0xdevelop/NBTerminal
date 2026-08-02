package guis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/george012/gtbox"
)

func TestValidateSettingsDraft(t *testing.T) {
	valid := settingsDraft{Language: "zh-CN", CommandTimeoutSeconds: 45}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	for _, draft := range []settingsDraft{
		{Language: "unsupported", CommandTimeoutSeconds: 45},
		{Language: "en", CommandTimeoutSeconds: 0},
		{Language: "en", CommandTimeoutSeconds: 86401},
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
	config.GlobalConfig = &config.FileConfig{Language: "en", Terminal: &config.TerminalSettings{CommandTimeoutSeconds: 60}}
	config.GlobalConfig.Normalize()
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = path

	draft := settingsDraft{
		Language:                 "zh-CN",
		CommandTimeoutSeconds:    17,
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
	if config.GlobalConfig.Language != "zh-CN" || config.GlobalConfig.Terminal.CommandTimeoutSeconds != 17 ||
		!config.GlobalConfig.ResetWorkspaceOnStart || !config.GlobalConfig.StartWithFirstConnection {
		t.Fatalf("settings did not survive restart: %#v", config.GlobalConfig)
	}
}

func TestPersistSettingsDraftRollsBackOnWriteFailure(t *testing.T) {
	oldGlobal := config.GlobalConfig
	oldApp := config.CurrentApp
	t.Cleanup(func() { config.GlobalConfig, config.CurrentApp = oldGlobal, oldApp })

	config.GlobalConfig = &config.FileConfig{Language: "en", Terminal: &config.TerminalSettings{CommandTimeoutSeconds: 60}}
	config.GlobalConfig.Normalize()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.CurrentApp = config.NewApp("NBTerminal-test", "test.nbterminal", "test", gtbox.RunModeTest, 0)
	config.CurrentApp.AppConfigFilePath = filepath.Join(blocker, "config.json")

	draft := settingsDraft{Language: "zh-CN", CommandTimeoutSeconds: 10, ResetWorkspaceOnStart: true, StartWithFirstConnection: true}
	if err := persistSettingsDraft(draft); err == nil {
		t.Fatal("expected persistence failure")
	}
	if config.GlobalConfig.Language != "en" || config.GlobalConfig.Terminal.CommandTimeoutSeconds != 60 ||
		config.GlobalConfig.ResetWorkspaceOnStart || config.GlobalConfig.StartWithFirstConnection {
		t.Fatalf("failed save leaked into live settings: %#v", config.GlobalConfig)
	}
}
