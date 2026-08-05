package guis

import "testing"

func TestNativeDesktopDesignTokensMatchProductBaseline(t *testing.T) {
	if nativeTypography.WindowTitle != 20 || nativeTypography.SectionTitle != 15 ||
		nativeTypography.Body != 13 || nativeTypography.Supporting != 12 || nativeTypography.Terminal != 14 {
		t.Fatalf("unexpected native typography tokens: %#v", nativeTypography)
	}
	if nativeControls.InputHeight != 34 || nativeControls.ButtonHeight != 34 ||
		nativeControls.PrimaryButtonHeight != 36 || nativeControls.TableHeaderHeight != 30 ||
		nativeControls.TableRowHeight != 32 || nativeControls.TextInset != 8 ||
		nativeControls.ButtonHorizontalInset < 14 || nativeControls.FieldLabelGap != 6 ||
		nativeControls.FieldGroupGap != 20 {
		t.Fatalf("unexpected native control tokens: %#v", nativeControls)
	}
}

func TestTerminalPanelLayoutPreservesDesktopControlMetricsAtMinimumWidth(t *testing.T) {
	panel := layoutRect{X: 526, Y: 58, Width: 576, Height: 630}
	layout := terminalPanelLayoutFor(panel, nativeControls)
	if !layout.Compact {
		t.Fatal("minimum terminal pane should use the compact two-row command layout")
	}
	if layout.CloseTab.Width < 150 {
		t.Fatalf("close-tab button is too narrow for Russian: %#v", layout.CloseTab)
	}
	for name, control := range map[string]layoutRect{
		"history": layout.History, "last": layout.Last, "clear": layout.Clear,
		"stop": layout.Stop, "run": layout.Run,
	} {
		if control.Height != nativeControls.PrimaryButtonHeight {
			t.Fatalf("%s height = %d, want %d", name, control.Height, nativeControls.PrimaryButtonHeight)
		}
		if control.X < panel.X || control.X+control.Width > panel.X+panel.Width {
			t.Fatalf("%s escapes minimum terminal pane: control=%#v panel=%#v", name, control, panel)
		}
	}
	if layout.Command.Width < 200 {
		t.Fatalf("minimum command input is not usable: %#v", layout.Command)
	}
	if layout.Output.Height < 360 || layout.Output.Bottom() > layout.History.Y-nativeControls.FieldLabelGap {
		t.Fatalf("terminal output overlaps the compact toolbar: %#v", layout)
	}
	if layout.Run.Width < 2*nativeControls.ButtonHorizontalInset+60 {
		t.Fatalf("primary action cannot carry long localized text with semantic insets: %#v", layout.Run)
	}
}

func TestTerminalPanelLayoutKeepsWideCommandBarOnOneRow(t *testing.T) {
	panel := layoutRect{X: 536, Y: 72, Width: 882, Height: 786}
	layout := terminalPanelLayoutFor(panel, nativeControls)
	if layout.Compact {
		t.Fatal("wide terminal pane unexpectedly used compact layout")
	}
	if layout.History.Y != layout.Command.Y || layout.Command.Y != layout.Run.Y {
		t.Fatalf("wide command controls are not on one row: %#v", layout)
	}
	if layout.Command.Width < 360 {
		t.Fatalf("wide command input is too narrow: %#v", layout.Command)
	}
	if layout.Output.Bottom() > layout.CommandLabel.Y-nativeControls.FieldLabelGap {
		t.Fatalf("terminal output overlaps command label: %#v", layout)
	}
}

func TestSettingsEditorAndManagerLayoutsUseSemanticControlMetrics(t *testing.T) {
	settings := settingsLayoutFor(nativeControls)
	if settings.Language.Height != nativeControls.InputHeight || settings.Timeout.Height != nativeControls.InputHeight {
		t.Fatalf("settings controls do not use input-height token: %#v", settings)
	}
	if settings.Save.Height != nativeControls.PrimaryButtonHeight || settings.Cancel.Height != nativeControls.PrimaryButtonHeight {
		t.Fatalf("settings actions do not use primary-height token: %#v", settings)
	}
	if settings.BehaviorTitleY-settings.Timeout.Bottom() < nativeControls.FieldGroupGap {
		t.Fatalf("settings field groups are too tight: %#v", settings)
	}

	editor := connectionEditorLayoutFor(nativeControls)
	for name, field := range map[string]layoutRect{
		"name": editor.Name, "group": editor.Group, "type": editor.Type,
		"host": editor.Host, "port": editor.Port, "username": editor.Username,
		"password": editor.Password, "working directory": editor.WorkingDir, "private key": editor.PrivateKey,
	} {
		if field.Height != nativeControls.InputHeight {
			t.Fatalf("%s height = %d, want %d", name, field.Height, nativeControls.InputHeight)
		}
	}
	if editor.Save.Height != nativeControls.PrimaryButtonHeight || editor.Cancel.Height != nativeControls.PrimaryButtonHeight {
		t.Fatalf("editor actions do not use primary-height token: %#v", editor)
	}
	if editor.WorkingDir.Y-editor.Password.Bottom() < nativeControls.FieldGroupGap+nativeTypography.Supporting {
		t.Fatalf("password hint/group spacing is too tight: %#v", editor)
	}

	manager := connectionManagerLayoutFor(nativeControls)
	if manager.Search.Height != nativeControls.InputHeight || manager.Find.Height != nativeControls.ButtonHeight ||
		manager.CloseAfterConnect.Height != nativeControls.InputHeight || manager.New.Height != nativeControls.ButtonHeight ||
		manager.Edit.Height != nativeControls.ButtonHeight || manager.Delete.Height != nativeControls.ButtonHeight ||
		manager.Favorite.Height != nativeControls.ButtonHeight || manager.Connect.Height != nativeControls.PrimaryButtonHeight {
		t.Fatalf("manager controls do not use semantic heights: %#v", manager)
	}
	if manager.CloseAfterConnect.Y-manager.Status.Bottom() < nativeControls.FieldLabelGap ||
		manager.New.Y-manager.CloseAfterConnect.Bottom() < nativeControls.FieldLabelGap {
		t.Fatalf("manager footer spacing is too tight: %#v", manager)
	}
}
