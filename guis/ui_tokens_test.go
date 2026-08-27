package guis

import (
	"testing"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
)

func TestStyleInputUsesNativeDarkTextAndVisibleCaret(t *testing.T) {
	in := uikit.NewInput(0, 0, 240, nativeControls.InputHeight, "")
	styleInput(in)
	raw, ok := in.Raw().(*fltk_bridge.Input)
	if !ok {
		t.Fatalf("raw input type = %T", in.Raw())
	}
	if raw.TextSize() != nativeTypography.Body || raw.TextColor() != tokenColor(modernTheme.foreground) {
		t.Fatalf("input text style = size:%d color:%v", raw.TextSize(), raw.TextColor())
	}
	if raw.Color() != tokenColor(modernTheme.elevated) || raw.CursorColor() != tokenColor(modernTheme.primary) {
		t.Fatalf("input surface/caret = background:%v cursor:%v", raw.Color(), raw.CursorColor())
	}
}

func TestStyleCommandInputUsesNativeDarkTextCaretAndSelection(t *testing.T) {
	input := uikit.NewUITextView(rect(0, 0, 240, nativeControls.InputHeight))
	styleCommandInput(input)
	raw := input.Raw()
	if raw.TextSize() != nativeTypography.Terminal || raw.TextColor() != tokenColor(modernTheme.foreground) {
		t.Fatalf("command input text style = size:%d color:%v", raw.TextSize(), raw.TextColor())
	}
	if raw.Color() != tokenColor(modernTheme.elevated) || raw.CursorColor() != tokenColor(modernTheme.primary) {
		t.Fatalf("command input surface/caret = background:%v cursor:%v", raw.Color(), raw.CursorColor())
	}
	if raw.SelectionColor() != tokenColor(modernTheme.selected) {
		t.Fatalf("command input selection = %v, want %v", raw.SelectionColor(), tokenColor(modernTheme.selected))
	}
}

func TestNativeDesktopDesignTokensMatchProductBaseline(t *testing.T) {
	if nativeTypography.WindowTitle != 20 || nativeTypography.SectionTitle != 15 ||
		nativeTypography.Body != 13 || nativeTypography.Supporting != 12 || nativeTypography.Terminal != 14 {
		t.Fatalf("unexpected native typography tokens: %#v", nativeTypography)
	}
	if nativeControls.InputHeight != 34 || nativeControls.ButtonHeight != 34 ||
		nativeControls.PrimaryButtonHeight != 36 || nativeControls.TableHeaderHeight != 30 ||
		nativeControls.TableRowHeight != 32 || nativeControls.TextInset != 8 ||
		nativeControls.ButtonHorizontalInset < 14 || nativeControls.FieldLabelGap != 6 ||
		nativeControls.FieldGroupGap != 20 || nativeControls.FieldLabelHeight != 20 ||
		nativeControls.SupportingLineHeight != 22 || nativeControls.SectionTitleHeight != 24 ||
		nativeControls.WindowTitleHeight != 30 || nativeControls.CheckboxHeight != 34 {
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

func TestQuickPanelLayoutKeepsMainWindowFocusedOnQuickLaunchAtMinimumSize(t *testing.T) {
	panel := layoutRect{X: 22, Y: 72, Width: 500, Height: 606}
	layout := quickPanelLayoutFor(panel, nativeControls)
	if layout.Table.Height < 315 {
		t.Fatalf("minimum quick-launch table gives too much space to non-launch chrome: %#v", layout.Table)
	}
	if layout.Table.Bottom() > layout.SummaryTitle.Y-nativeControls.FieldLabelGap {
		t.Fatalf("table overlaps selected-connection summary: %#v", layout)
	}
	for name, control := range map[string]layoutRect{
		"search": layout.Search, "find": layout.Find, "connect": layout.Connect,
	} {
		if control.X < panel.X || control.X+control.Width > panel.X+panel.Width {
			t.Fatalf("%s escapes minimum quick panel: control=%#v panel=%#v", name, control, panel)
		}
	}
	if layout.Search.Height != nativeControls.InputHeight || layout.Find.Height != nativeControls.ButtonHeight {
		t.Fatalf("search row does not preserve semantic heights: %#v", layout)
	}
	if layout.Connect.Height != nativeControls.PrimaryButtonHeight || layout.Connect.Width < 180 {
		t.Fatalf("primary connect action is undersized: %#v", layout.Connect)
	}
	if layout.Connect.Y < layout.SelectedRecent.Bottom()+nativeControls.FieldLabelGap {
		t.Fatalf("quick connect action overlaps selected summary: %#v", layout)
	}
	for name, summary := range map[string]layoutRect{
		"name": layout.SelectedName, "detail": layout.SelectedDetail, "recent": layout.SelectedRecent,
	} {
		if summary.X+summary.Width > panel.X+panel.Width-nativeControls.TextInset {
			t.Fatalf("%s summary reaches the pane clip edge: %#v", name, summary)
		}
	}
}

func TestMainWindowLayoutPreservesHeaderAndWorkspaceAtMinimumSize(t *testing.T) {
	layout := mainWindowLayoutFor(layoutRect{Width: 1120, Height: 720}, nativeControls)
	for name, control := range map[string]layoutRect{
		"manager": layout.Manager, "settings": layout.Settings, "status": layout.Status,
	} {
		if control.Height != nativeControls.ButtonHeight {
			t.Fatalf("%s header control was proportionally crushed: %#v", name, control)
		}
		if control.X < 0 || control.X+control.Width > 1120 {
			t.Fatalf("%s header control escapes minimum window: %#v", name, control)
		}
	}
	if layout.Title.Width < 360 || layout.Status.Width < 300 {
		t.Fatalf("minimum header text regions are unusable: %#v", layout)
	}
	if layout.Workspace != (layoutRect{X: 22, Y: 72, Width: 1076, Height: 606}) {
		t.Fatalf("minimum workspace = %#v", layout.Workspace)
	}
}

func TestSettingsEditorAndManagerLayoutsUseSemanticControlMetrics(t *testing.T) {
	settings := settingsLayoutFor(nativeControls)
	if settings.Title.Height != nativeControls.WindowTitleHeight || settings.Subtitle.Height != nativeControls.SupportingLineHeight ||
		settings.GeneralTitle.Height != nativeControls.SectionTitleHeight || settings.BehaviorTitle.Height != nativeControls.SectionTitleHeight {
		t.Fatalf("settings headings do not use semantic typography carriers: %#v", settings)
	}
	if settings.Language.Height != nativeControls.InputHeight || settings.Timeout.Height != nativeControls.InputHeight {
		t.Fatalf("settings controls do not use input-height token: %#v", settings)
	}
	if settings.Save.Height != nativeControls.PrimaryButtonHeight || settings.Cancel.Height != nativeControls.PrimaryButtonHeight {
		t.Fatalf("settings actions do not use primary-height token: %#v", settings)
	}
	if settings.LanguageLabel.Height != nativeControls.InputHeight || settings.TimeoutLabel.Height != nativeControls.InputHeight ||
		settings.SecondsHint.Height != nativeControls.SupportingLineHeight || settings.ResetWorkspace.Height != nativeControls.CheckboxHeight ||
		settings.StartFirst.Height != nativeControls.CheckboxHeight || settings.BehaviorHint.Height < nativeControls.SupportingLineHeight {
		t.Fatalf("settings labels/checks do not use semantic carriers: %#v", settings)
	}
	if settings.BehaviorTitle.Y-settings.Timeout.Bottom() < nativeControls.FieldGroupGap {
		t.Fatalf("settings field groups are too tight: %#v", settings)
	}

	editor := connectionEditorLayoutFor(nativeControls)
	if editor.Title.Height != nativeControls.WindowTitleHeight || editor.Subtitle.Height < nativeControls.SupportingLineHeight ||
		editor.TypeLabel.Height != nativeControls.FieldLabelHeight || editor.PasswordLabel.Height != nativeControls.FieldLabelHeight {
		t.Fatalf("editor headings/labels do not use semantic typography carriers: %#v", editor)
	}
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
	if editor.PasswordHint.Y < editor.Password.Bottom() || editor.PasswordHint.Height < nativeTypography.Supporting*2 {
		t.Fatalf("password hint cannot wrap localized text: %#v", editor)
	}

	manager := connectionManagerLayoutFor(nativeControls)
	if manager.Group.Height != nativeControls.InputHeight || manager.Search.Height != nativeControls.InputHeight || manager.Find.Height != nativeControls.ButtonHeight ||
		manager.CloseAfterConnect.Height != nativeControls.InputHeight || manager.New.Height != nativeControls.ButtonHeight ||
		manager.Edit.Height != nativeControls.ButtonHeight || manager.Duplicate.Height != nativeControls.ButtonHeight || manager.Delete.Height != nativeControls.ButtonHeight ||
		manager.Test.Height != nativeControls.ButtonHeight || manager.Favorite.Height != nativeControls.ButtonHeight || manager.Connect.Height != nativeControls.PrimaryButtonHeight {
		t.Fatalf("manager controls do not use semantic heights: %#v", manager)
	}
	if manager.Group.X+manager.Group.Width > manager.Search.X-nativeControls.FieldLabelGap {
		t.Fatalf("manager group and search filters overlap: %#v", manager)
	}
	if manager.CloseAfterConnect.Y-manager.Status.Bottom() < nativeControls.FieldLabelGap ||
		manager.New.Y-manager.CloseAfterConnect.Bottom() < nativeControls.FieldLabelGap {
		t.Fatalf("manager footer spacing is too tight: %#v", manager)
	}
	for left, right := range map[layoutRect]layoutRect{
		manager.New: manager.Edit, manager.Edit: manager.Duplicate, manager.Duplicate: manager.Delete,
		manager.Delete: manager.Test, manager.Test: manager.Favorite, manager.Favorite: manager.Connect,
	} {
		if left.X+left.Width > right.X-nativeControls.FieldLabelGap {
			t.Fatalf("manager actions overlap or lose semantic spacing: left=%#v right=%#v", left, right)
		}
	}
}
