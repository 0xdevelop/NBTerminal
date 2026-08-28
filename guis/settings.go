package guis

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/checkbox"
	uidropdown "github.com/0xdevelop/fltk2go/uikit/dropdown"
)

const (
	settingsWindowWidth  = 620
	settingsWindowHeight = 530
	maxCommandTimeout    = 86400
)

type settingsDraft struct {
	Language                 string
	CommandTimeoutSeconds    int
	TerminalFontSize         int
	ResetWorkspaceOnStart    bool
	StartWithFirstConnection bool
}

func (d settingsDraft) Validate() error {
	languageSupported := false
	for _, language := range locales.SupportedLanguages() {
		if d.Language == language.LanguageTag() {
			languageSupported = true
			break
		}
	}
	if !languageSupported {
		return fmt.Errorf("unsupported language %q", d.Language)
	}
	if d.CommandTimeoutSeconds < 1 || d.CommandTimeoutSeconds > maxCommandTimeout {
		return fmt.Errorf("command timeout must be between 1 and %d seconds", maxCommandTimeout)
	}
	if d.TerminalFontSize < config.TerminalFontSizeMin || d.TerminalFontSize > config.TerminalFontSizeMax {
		return fmt.Errorf("terminal font size must be between %d and %d", config.TerminalFontSizeMin, config.TerminalFontSizeMax)
	}
	return nil
}

func currentSettingsDraft() settingsDraft {
	draft := settingsDraft{Language: locales.CurrentLanguage().LanguageTag(), CommandTimeoutSeconds: config.CommandTimeoutDefaultSeconds, TerminalFontSize: config.TerminalFontSizeDefault}
	if config.GlobalConfig == nil {
		return draft
	}
	draft.Language = config.GlobalConfig.Language
	if config.GlobalConfig.Terminal != nil {
		draft.CommandTimeoutSeconds = config.GlobalConfig.Terminal.CommandTimeoutSeconds
		draft.TerminalFontSize = config.GlobalConfig.Terminal.FontSize
	}
	draft.ResetWorkspaceOnStart = config.GlobalConfig.ResetWorkspaceOnStart
	draft.StartWithFirstConnection = config.GlobalConfig.StartWithFirstConnection
	return draft
}

// persistSettingsDraft publishes settings only after the atomic config write
// succeeds. A failed write restores the complete in-memory settings snapshot so
// the running UI and restart state cannot diverge.
func persistSettingsDraft(draft settingsDraft) error {
	if err := draft.Validate(); err != nil {
		return err
	}
	if config.GlobalConfig == nil || config.CurrentApp == nil || strings.TrimSpace(config.CurrentApp.AppConfigFilePath) == "" {
		return errors.New("application config is unavailable")
	}
	cfg := config.GlobalConfig
	previousLanguage := cfg.Language
	previousTerminal := cfg.Terminal
	previousReset := cfg.ResetWorkspaceOnStart
	previousFirst := cfg.StartWithFirstConnection

	cfg.Language = draft.Language
	cfg.Terminal = &config.TerminalSettings{CommandTimeoutSeconds: draft.CommandTimeoutSeconds, FontSize: draft.TerminalFontSize}
	cfg.ResetWorkspaceOnStart = draft.ResetWorkspaceOnStart
	cfg.StartWithFirstConnection = draft.StartWithFirstConnection
	if err := config.SaveConfig(config.CurrentApp.AppConfigFilePath); err != nil {
		cfg.Language = previousLanguage
		cfg.Terminal = previousTerminal
		cfg.ResetWorkspaceOnStart = previousReset
		cfg.StartWithFirstConnection = previousFirst
		return err
	}
	return nil
}

type settingsWindow struct {
	owner          *finalShellApp
	window         *uikit.UIWindow
	initial        settingsDraft
	closing        unsavedCloseController
	language       *uidropdown.UIDropdown
	timeout        *uikit.Input
	terminalFont   *uikit.Input
	resetWorkspace *checkbox.UICheckbox
	startFirst     *checkbox.UICheckbox
}

func (a *finalShellApp) openSettings() {
	if a == nil {
		return
	}
	if a.settings != nil && a.settings.window != nil && !a.settings.window.IsClosed() {
		a.settings.window.Show()
		if raw := a.settings.window.Raw(); raw != nil {
			raw.TakeFocus()
		}
		return
	}

	s := &settingsWindow{owner: a}
	a.settings = s
	s.build(currentSettingsDraft())
}

func (s *settingsWindow) build(draft settingsDraft) {
	owner := s.owner
	s.initial = draft
	layout := settingsLayoutFor(nativeControls)
	windowRect := centeredScreenRect(settingsWindowWidth, settingsWindowHeight)
	if owner != nil && owner.window != nil && owner.window.Raw() != nil {
		raw := owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-settingsWindowWidth)/2, raw.YRoot()+(raw.H()-settingsWindowHeight)/2, settingsWindowWidth, settingsWindowHeight)
	}
	s.window = uikit.NewWindowWithRect(windowRect, tr("settings.window_title"))
	s.window.SetResizable(false)
	if raw := s.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	s.window.RootView().SetAutomationID("settings.window").SetAutomationRole("window").SetAutomationName(tr("settings.window_title"))
	s.window.OnClose(func() {
		s.window.RootView().SetAutomationID("")
		if owner != nil && owner.settings == s {
			owner.settings = nil
		}
	})
	s.window.OnCloseRequest(s.shouldClose)

	root := s.window.RootView()
	root.AddSubview(titleLabel(layout.Title.X, layout.Title.Y, layout.Title.Width, layout.Title.Height, tr("settings.window_title")))
	root.AddSubview(mutedLabel(layout.Subtitle.X, layout.Subtitle.Y, layout.Subtitle.Width, layout.Subtitle.Height, tr("settings.subtitle")))

	root.AddSubview(sectionTitle(layout.GeneralTitle.X, layout.GeneralTitle.Y, layout.GeneralTitle.Width, layout.GeneralTitle.Height, tr("settings.general")))
	root.AddSubview(label(layout.LanguageLabel.X, layout.LanguageLabel.Y, layout.LanguageLabel.Width, layout.LanguageLabel.Height, tr("settings.language")))
	s.language = uidropdown.NewUIDropdown(rect(layout.Language.X, layout.Language.Y, layout.Language.Width, layout.Language.Height))
	languages := locales.SupportedLanguages()
	names := make([]string, 0, len(languages))
	selected := 0
	for index, language := range languages {
		names = append(names, language.String())
		if draft.Language == language.LanguageTag() {
			selected = index
		}
	}
	s.language.SetOptions(names)
	s.language.SetSelectedIndex(selected)
	styleDropdown(s.language)
	s.language.View().SetAutomationID("settings.language").SetAutomationName(tr("settings.language"))
	root.AddSubview(s.language)

	root.AddSubview(label(layout.TimeoutLabel.X, layout.TimeoutLabel.Y, layout.TimeoutLabel.Width, layout.TimeoutLabel.Height, tr("settings.command_timeout")))
	s.timeout = uikit.NewInputWithType(layout.Timeout.X, layout.Timeout.Y, layout.Timeout.Width, layout.Timeout.Height, "", uikit.IntInput)
	styleInput(s.timeout)
	s.timeout.SetText(strconv.Itoa(draft.CommandTimeoutSeconds))
	s.timeout.View().SetAutomationID("settings.command_timeout").SetAutomationName(tr("settings.command_timeout"))
	root.AddSubview(s.timeout)
	root.AddSubview(mutedLabel(layout.SecondsHint.X, layout.SecondsHint.Y, layout.SecondsHint.Width, layout.SecondsHint.Height, tr("settings.seconds_hint")))
	root.AddSubview(label(layout.TerminalFontLabel.X, layout.TerminalFontLabel.Y, layout.TerminalFontLabel.Width, layout.TerminalFontLabel.Height, "Terminal font size"))
	s.terminalFont = uikit.NewInputWithType(layout.TerminalFont.X, layout.TerminalFont.Y, layout.TerminalFont.Width, layout.TerminalFont.Height, "", uikit.IntInput)
	styleInput(s.terminalFont)
	s.terminalFont.SetText(strconv.Itoa(draft.TerminalFontSize))
	s.terminalFont.View().SetAutomationID("settings.terminal_font_size").SetAutomationName("Terminal font size")
	root.AddSubview(s.terminalFont)
	root.AddSubview(mutedLabel(layout.TerminalFontHint.X, layout.TerminalFontHint.Y, layout.TerminalFontHint.Width, layout.TerminalFontHint.Height, fmt.Sprintf("%d–%d px", config.TerminalFontSizeMin, config.TerminalFontSizeMax)))

	root.AddSubview(sectionTitle(layout.BehaviorTitle.X, layout.BehaviorTitle.Y, layout.BehaviorTitle.Width, layout.BehaviorTitle.Height, tr("settings.behavior")))
	checkStyle := checkbox.DefaultCheckboxStyle()
	checkStyle.Font = fltk_bridge.HELVETICA
	checkStyle.FontSize = nativeTypography.Body
	checkStyle.TextColor = uint(tokenColor(modernTheme.foreground))
	checkStyle.Color = uint(tokenColor(modernTheme.card))
	s.resetWorkspace = checkbox.NewUICheckboxWithOptions(rect(layout.ResetWorkspace.X, layout.ResetWorkspace.Y, layout.ResetWorkspace.Width, layout.ResetWorkspace.Height), tr("settings.reset_workspace"), checkStyle)
	s.resetWorkspace.SetValue(draft.ResetWorkspaceOnStart)
	s.resetWorkspace.View().SetAutomationID("settings.reset_workspace").SetAutomationName(tr("settings.reset_workspace"))
	root.AddSubview(s.resetWorkspace)
	s.startFirst = checkbox.NewUICheckboxWithOptions(rect(layout.StartFirst.X, layout.StartFirst.Y, layout.StartFirst.Width, layout.StartFirst.Height), tr("settings.start_first"), checkStyle)
	s.startFirst.SetValue(draft.StartWithFirstConnection)
	s.startFirst.View().SetAutomationID("settings.start_first").SetAutomationName(tr("settings.start_first"))
	root.AddSubview(s.startFirst)
	behaviorHint := mutedLabel(layout.BehaviorHint.X, layout.BehaviorHint.Y, layout.BehaviorHint.Width, layout.BehaviorHint.Height, tr("settings.behavior_hint"))
	behaviorHint.SetAlignment(fltk_bridge.ALIGN_LEFT | fltk_bridge.ALIGN_INSIDE | fltk_bridge.ALIGN_WRAP)
	root.AddSubview(behaviorHint)

	root.AddSubview(button(layout.Cancel.X, layout.Cancel.Y, layout.Cancel.Width, layout.Cancel.Height, tr("button.cancel"), "settings.cancel", s.requestClose))
	root.AddSubview(primaryButton(layout.Save.X, layout.Save.Y, layout.Save.Width, layout.Save.Height, tr("button.save"), "settings.save", s.save))
	s.window.Show()
}

func (s *settingsWindow) draft() (settingsDraft, error) {
	if s == nil || s.language == nil || s.timeout == nil || s.terminalFont == nil || s.resetWorkspace == nil || s.startFirst == nil {
		return settingsDraft{}, errors.New("settings controls are unavailable")
	}
	languages := locales.SupportedLanguages()
	index := s.language.SelectedIndex()
	if index < 0 || index >= len(languages) {
		return settingsDraft{}, errors.New("select a language")
	}
	timeout, err := strconv.Atoi(strings.TrimSpace(s.timeout.Text()))
	if err != nil {
		return settingsDraft{}, errors.New("command timeout must be an integer")
	}
	fontSize, err := strconv.Atoi(strings.TrimSpace(s.terminalFont.Text()))
	if err != nil {
		return settingsDraft{}, errors.New("terminal font size must be an integer")
	}
	return settingsDraft{
		Language:                 languages[index].LanguageTag(),
		CommandTimeoutSeconds:    timeout,
		TerminalFontSize:         fontSize,
		ResetWorkspaceOnStart:    s.resetWorkspace.Value(),
		StartWithFirstConnection: s.startFirst.Value(),
	}, nil
}

func (s *settingsWindow) save() {
	draft, err := s.draft()
	oldLanguage := locales.CurrentLanguage().LanguageTag()
	if err == nil && oldLanguage != draft.Language && s.owner != nil && s.owner.editor != nil && s.owner.editor.window != nil {
		// A locale change reconstructs native windows. Never let that owner-driven
		// teardown bypass an unsaved connection draft: require the editor's normal
		// close policy first, then let the user press Save again after resolving it.
		if !s.owner.editor.window.RequestClose() {
			return
		}
	}
	if err == nil {
		err = persistSettingsDraft(draft)
	}
	if err != nil {
		if s.owner != nil {
			s.owner.setStatus(tr("settings.save_failed"))
			s.owner.showTopNotice(tr("settings.save_failed"), err.Error(), true)
		}
		return
	}

	s.close()
	if s.owner == nil {
		return
	}
	if oldLanguage != draft.Language {
		locales.ResetLocaleLanguage(draft.Language)
		s.owner.rebuildForLanguage(locales.GetLanguageFromTag(draft.Language))
		return
	}
	s.owner.applyTerminalFontSize(draft.TerminalFontSize)
	s.owner.setStatus(tr("settings.saved"))
}

func (s *settingsWindow) close() {
	if s == nil || s.window == nil {
		return
	}
	s.window.Close()
}

func (s *settingsWindow) shouldClose() bool {
	if s == nil || s.window == nil {
		return true
	}
	draft, err := s.draft()
	dirty := err != nil
	if err == nil {
		dirty = settingsDraftChanged(s.initial, draft)
	}
	return s.closing.handle(s.window, "settings", dirty, nil)
}

func (s *settingsWindow) requestClose() {
	if s == nil || s.window == nil {
		return
	}
	s.window.RequestClose()
}

func (a *finalShellApp) rebuildForLanguage(language locales.Language) {
	if a == nil {
		return
	}
	if a.editor != nil {
		a.editor.close()
	}
	if a.manager != nil && a.manager.window != nil {
		a.manager.window.Close()
	}
	oldWindow := a.window
	a.syncActiveSessionView()
	// Rebuild the native surface on the same application owner. Long-lived PTY,
	// monitor, and command callbacks route by runtime ID back to this owner; a
	// replacement owner would strand output in the closed window after a locale
	// change.
	a.settings = nil
	a.editor = nil
	a.manager = nil
	a.allRows = a.store.List()
	a.rows = navigatorRows(a.allRows, "", quickConnectionLimit)
	a.idx = -1
	a.build()
	a.setStatus(trf("app.language_changed", language.String()))
	if oldWindow != nil {
		oldWindow.Close()
	}
}
