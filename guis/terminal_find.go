package guis

import (
	"fmt"
	"strings"

	"github.com/0xdevelop/fltk2go/fltk_bridge"
	"github.com/0xdevelop/fltk2go/uikit"
	"github.com/0xdevelop/fltk2go/uikit/checkbox"
)

const (
	terminalFindWidth  = 560
	terminalFindHeight = 224
)

type terminalFindState struct {
	query         string
	caseSensitive bool
	wholeWord     bool
	matches       []uikit.TerminalTextMatch
	index         int
}

func (s *terminalFindState) Search(terminal *uikit.UITerminalView, query string) {
	s.query = query
	s.index = -1
	s.matches = nil
	if terminal == nil || strings.TrimSpace(query) == "" {
		return
	}
	s.matches = terminal.SearchTextWithOptions(query, uikit.TerminalTextSearchOptions{
		CaseSensitive: s.caseSensitive,
		WholeWord:     s.wholeWord,
	})
	if len(s.matches) > 0 {
		s.index = 0
	}
}

func (s *terminalFindState) SetCaseSensitive(terminal *uikit.UITerminalView, enabled bool) {
	if s == nil || s.caseSensitive == enabled {
		return
	}
	s.caseSensitive = enabled
	s.Search(terminal, s.query)
}

func (s *terminalFindState) SetWholeWord(terminal *uikit.UITerminalView, enabled bool) {
	if s == nil || s.wholeWord == enabled {
		return
	}
	s.wholeWord = enabled
	s.Search(terminal, s.query)
}

func (s *terminalFindState) Refresh(terminal *uikit.UITerminalView) {
	if s == nil {
		return
	}
	current, hadCurrent := s.Current()
	previousIndex := s.index
	query := s.query
	s.Search(terminal, query)
	if len(s.matches) == 0 {
		return
	}
	if hadCurrent {
		for i, match := range s.matches {
			if match == current {
				s.index = i
				return
			}
		}
	}
	if previousIndex >= 0 && previousIndex < len(s.matches) {
		s.index = previousIndex
	}
}

func (s *terminalFindState) Move(delta int) {
	if len(s.matches) == 0 {
		s.index = -1
		return
	}
	s.index = (s.index + delta) % len(s.matches)
	if s.index < 0 {
		s.index += len(s.matches)
	}
}

func (s terminalFindState) Current() (uikit.TerminalTextMatch, bool) {
	if s.index < 0 || s.index >= len(s.matches) {
		return uikit.TerminalTextMatch{}, false
	}
	return s.matches[s.index], true
}

func (s terminalFindState) Status() string {
	switch {
	case strings.TrimSpace(s.query) == "":
		return "Type to search"
	case len(s.matches) == 0:
		return "No matches"
	default:
		return fmt.Sprintf("%d of %d", s.index+1, len(s.matches))
	}
}

type terminalFindWindow struct {
	owner         *finalShellApp
	window        *uikit.UIWindow
	input         *uikit.Input
	matchCase     *checkbox.UICheckbox
	wholeWord     *checkbox.UICheckbox
	status        *uikit.UILabel
	state         terminalFindState
	stopObserving func()
}

func (a *finalShellApp) openTerminalFind() {
	if a == nil || a.output == nil {
		return
	}
	if a.terminalFind != nil && a.terminalFind.window != nil && !a.terminalFind.window.IsClosed() {
		a.terminalFind.window.Show()
		a.terminalFind.focusInput()
		return
	}
	finder := &terminalFindWindow{owner: a}
	a.terminalFind = finder
	finder.build()
}

func (f *terminalFindWindow) build() {
	windowRect := centeredScreenRect(terminalFindWidth, terminalFindHeight)
	if f.owner != nil && f.owner.window != nil && f.owner.window.Raw() != nil {
		raw := f.owner.window.Raw()
		windowRect = rect(raw.XRoot()+(raw.W()-terminalFindWidth)/2, raw.YRoot()+96, terminalFindWidth, terminalFindHeight)
	}
	f.window = uikit.NewWindowWithRect(windowRect, "Find in Terminal")
	f.window.SetResizable(false)
	if raw := f.window.Raw(); raw != nil {
		raw.SetXClass(nativeWindowClass())
		raw.SetNonModal()
		raw.SetColor(tokenColor(modernTheme.background))
	}
	root := f.window.RootView()
	root.SetAutomationID("terminal_find.window").SetAutomationRole("window").SetAutomationName("Find in Terminal")
	f.window.OnClose(func() {
		if f.stopObserving != nil {
			f.stopObserving()
			f.stopObserving = nil
		}
		root.SetAutomationID("")
		if f.owner != nil && f.owner.terminalFind == f {
			f.owner.terminalFind = nil
		}
	})

	root.AddSubview(titleLabel(24, 18, 360, 28, "Find in Terminal"))
	root.AddSubview(mutedLabel(26, 48, 500, 20, "Search the active terminal, including retained scrollback."))
	f.input = uikit.NewInput(26, 82, 304, nativeControls.InputHeight, "")
	styleInput(f.input)
	f.input.View().SetAutomationID("terminal_find.query").SetAutomationName("Search terminal output")
	f.input.OnChange(f.search)
	f.input.OnNavigation(f.handleNavigation)
	root.AddSubview(f.input)
	root.AddSubview(button(340, 82, 82, nativeControls.ButtonHeight, "Previous", "terminal_find.previous", func() { f.navigate(-1) }))
	root.AddSubview(primaryButton(432, 82, 102, nativeControls.ButtonHeight, "Next", "terminal_find.next", func() { f.navigate(1) }))

	checkStyle := checkbox.DefaultCheckboxStyle()
	checkStyle.Font = fltk_bridge.HELVETICA
	checkStyle.FontSize = nativeTypography.Body
	checkStyle.TextColor = uint(tokenColor(modernTheme.foreground))
	checkStyle.Color = uint(tokenColor(modernTheme.background))
	f.matchCase = checkbox.NewUICheckboxWithOptions(rect(26, 126, 180, 30), "Match case", checkStyle)
	f.matchCase.View().SetAutomationID("terminal_find.match_case").SetAutomationName("Match case")
	f.matchCase.OnValueChanged(f.setCaseSensitive)
	root.AddSubview(f.matchCase)
	f.wholeWord = checkbox.NewUICheckboxWithOptions(rect(218, 126, 180, 30), "Whole word", checkStyle)
	f.wholeWord.View().SetAutomationID("terminal_find.whole_word").SetAutomationName("Whole word")
	f.wholeWord.OnValueChanged(f.setWholeWord)
	root.AddSubview(f.wholeWord)

	f.status = mutedLabel(26, 174, 330, 28, "Type to search")
	f.status.SetFrame(fltk_bridge.FLAT_BOX)
	f.status.SetBackgroundColor(uint(tokenColor(modernTheme.background)))
	f.status.View().SetAutomationID("terminal_find.status")
	root.AddSubview(f.status)
	root.AddSubview(button(422, 170, 112, nativeControls.PrimaryButtonHeight, "Close", "terminal_find.close", f.close))
	if f.owner != nil && f.owner.output != nil {
		f.stopObserving = f.owner.output.ObserveTextChanged(f.refreshLiveSearch)
	}

	f.window.Show()
	f.focusInput()
}

func (f *terminalFindWindow) focusInput() {
	if f == nil || f.input == nil || f.input.View() == nil {
		return
	}
	if raw := f.input.View().Raw(); raw != nil {
		if focusable, ok := raw.(interface{ TakeFocus() int }); ok {
			focusable.TakeFocus()
		}
	}
}

func (f *terminalFindWindow) search() {
	if f == nil || f.owner == nil || f.input == nil {
		return
	}
	f.state.Search(f.owner.output, f.input.Text())
	f.revealCurrent()
}

func (f *terminalFindWindow) refreshLiveSearch() {
	if f == nil || f.owner == nil || f.owner.output == nil {
		return
	}
	f.state.Refresh(f.owner.output)
	if f.status != nil {
		f.status.SetText(f.state.Status())
	}
}

func (f *terminalFindWindow) setCaseSensitive(enabled bool) {
	if f == nil || f.owner == nil {
		return
	}
	f.state.SetCaseSensitive(f.owner.output, enabled)
	f.revealCurrent()
}

func (f *terminalFindWindow) setWholeWord(enabled bool) {
	if f == nil || f.owner == nil {
		return
	}
	f.state.SetWholeWord(f.owner.output, enabled)
	f.revealCurrent()
}

func (f *terminalFindWindow) navigate(delta int) {
	if f == nil || f.owner == nil || f.input == nil {
		return
	}
	if f.state.query != f.input.Text() {
		f.state.Search(f.owner.output, f.input.Text())
	} else {
		f.state.Move(delta)
	}
	f.revealCurrent()
}

func (f *terminalFindWindow) revealCurrent() {
	if f == nil {
		return
	}
	if f.status != nil {
		f.status.SetText(f.state.Status())
	}
	if match, ok := f.state.Current(); ok && f.owner != nil && f.owner.output != nil {
		f.owner.output.RevealTextMatch(match)
	}
}

func (f *terminalFindWindow) handleNavigation(action uikit.InputNavigationAction) bool {
	switch action {
	case uikit.InputNavigationSubmit, uikit.InputNavigationNext:
		f.navigate(1)
		return true
	case uikit.InputNavigationPrevious:
		f.navigate(-1)
		return true
	case uikit.InputNavigationCancel:
		f.close()
		return true
	default:
		return false
	}
}

func (f *terminalFindWindow) close() {
	if f == nil || f.window == nil {
		return
	}
	f.window.Close()
	if f.owner != nil && f.owner.output != nil && f.owner.output.Raw() != nil {
		f.owner.output.Raw().TakeFocus()
	}
}
