package guis

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/0xdevelop/NBTerminal/terminal"
	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/checkbox"
	uidropdown "github.com/0xdevelop/fltk2go/uikit/dropdown"
)

const (
	settingsWindowWidth  = 620
	settingsWindowHeight = 470
	maxCommandTimeout    = 86400
)

type settingsDraft struct {
	Language                 string
	CommandTimeoutSeconds    int
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
	return nil
}

func currentSettingsDraft() settingsDraft {
	draft := settingsDraft{Language: locales.CurrentLanguage().LanguageTag(), CommandTimeoutSeconds: config.CommandTimeoutDefaultSeconds}
	if config.GlobalConfig == nil {
		return draft
	}
	draft.Language = config.GlobalConfig.Language
	if config.GlobalConfig.Terminal != nil {
		draft.CommandTimeoutSeconds = config.GlobalConfig.Terminal.CommandTimeoutSeconds
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
	cfg.Terminal = &config.TerminalSettings{CommandTimeoutSeconds: draft.CommandTimeoutSeconds}
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
	language       *uidropdown.UIDropdown
	timeout        *uikit.Input
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

	root := s.window.RootView()
	root.AddSubview(titleLabel(26, 22, 420, 30, tr("settings.window_title")))
	root.AddSubview(mutedLabel(28, 54, 550, 22, tr("settings.subtitle")))

	root.AddSubview(sectionTitle(28, 96, 220, 24, tr("settings.general")))
	root.AddSubview(label(28, layout.Language.Y, 190, layout.Language.Height, tr("settings.language")))
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

	root.AddSubview(label(28, layout.Timeout.Y, 190, layout.Timeout.Height, tr("settings.command_timeout")))
	s.timeout = uikit.NewInputWithType(layout.Timeout.X, layout.Timeout.Y, layout.Timeout.Width, layout.Timeout.Height, "", uikit.IntInput)
	styleInput(s.timeout)
	s.timeout.SetText(strconv.Itoa(draft.CommandTimeoutSeconds))
	s.timeout.View().SetAutomationID("settings.command_timeout").SetAutomationName(tr("settings.command_timeout"))
	root.AddSubview(s.timeout)
	root.AddSubview(mutedLabel(370, layout.Timeout.Y+6, 198, 22, tr("settings.seconds_hint")))

	root.AddSubview(sectionTitle(28, layout.BehaviorTitleY, 300, 24, tr("settings.behavior")))
	checkStyle := checkbox.DefaultCheckboxStyle()
	checkStyle.Font = fltk_bridge.HELVETICA
	checkStyle.FontSize = nativeTypography.Body
	checkStyle.TextColor = uint(tokenColor(modernTheme.foreground))
	checkStyle.Color = uint(tokenColor(modernTheme.card))
	s.resetWorkspace = checkbox.NewUICheckboxWithOptions(rect(28, 270, 540, 34), tr("settings.reset_workspace"), checkStyle)
	s.resetWorkspace.SetValue(draft.ResetWorkspaceOnStart)
	s.resetWorkspace.View().SetAutomationID("settings.reset_workspace").SetAutomationName(tr("settings.reset_workspace"))
	root.AddSubview(s.resetWorkspace)
	s.startFirst = checkbox.NewUICheckboxWithOptions(rect(28, 312, 540, 34), tr("settings.start_first"), checkStyle)
	s.startFirst.SetValue(draft.StartWithFirstConnection)
	s.startFirst.View().SetAutomationID("settings.start_first").SetAutomationName(tr("settings.start_first"))
	root.AddSubview(s.startFirst)
	root.AddSubview(mutedLabel(50, 348, 518, 38, tr("settings.behavior_hint")))

	root.AddSubview(button(layout.Cancel.X, layout.Cancel.Y, layout.Cancel.Width, layout.Cancel.Height, tr("button.cancel"), "settings.cancel", s.close))
	root.AddSubview(primaryButton(layout.Save.X, layout.Save.Y, layout.Save.Width, layout.Save.Height, tr("button.save"), "settings.save", s.save))
	s.window.Show()
}

func (s *settingsWindow) draft() (settingsDraft, error) {
	if s == nil || s.language == nil || s.timeout == nil || s.resetWorkspace == nil || s.startFirst == nil {
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
	return settingsDraft{
		Language:                 languages[index].LanguageTag(),
		CommandTimeoutSeconds:    timeout,
		ResetWorkspaceOnStart:    s.resetWorkspace.Value(),
		StartWithFirstConnection: s.startFirst.Value(),
	}, nil
}

func (s *settingsWindow) save() {
	draft, err := s.draft()
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

	oldLanguage := locales.CurrentLanguage().LanguageTag()
	s.close()
	if s.owner == nil {
		return
	}
	if oldLanguage != draft.Language {
		locales.ResetLocaleLanguage(draft.Language)
		s.owner.rebuildForLanguage(locales.GetLanguageFromTag(draft.Language))
		return
	}
	s.owner.setStatus(tr("settings.saved"))
}

func (s *settingsWindow) close() {
	if s == nil || s.window == nil {
		return
	}
	s.window.Close()
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
	next := &finalShellApp{store: a.store, history: a.history, session: terminal.NewSession(a.history), sessions: a.sessions, idx: -1}
	next.allRows = a.store.List()
	next.rows = navigatorRows(next.allRows, "", quickConnectionLimit)
	next.build()
	next.setStatus(trf("app.language_changed", language.String()))
	if oldWindow != nil {
		oldWindow.Close()
	}
}
