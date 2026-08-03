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

func TestSettingsAndEditorLayoutsUseSemanticControlMetrics(t *testing.T) {
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
}
