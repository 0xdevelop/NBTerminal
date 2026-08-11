package guis

// typographyTokenSet is the single native desktop type scale for NBTerminal.
// Widget helpers and custom drawing consume these semantic roles instead of
// introducing per-window font-size literals.
type typographyTokenSet struct {
	WindowTitle  int
	SectionTitle int
	Body         int
	Supporting   int
	Terminal     int
}

var nativeTypography = typographyTokenSet{
	WindowTitle:  20,
	SectionTitle: 15,
	Body:         13,
	Supporting:   12,
	Terminal:     14,
}

// controlMetricSet captures the desktop mouse/keyboard density baseline. These
// values intentionally do not copy mobile 44/48-point touch targets.
type controlMetricSet struct {
	InputHeight           int
	ButtonHeight          int
	PrimaryButtonHeight   int
	TableHeaderHeight     int
	TableRowHeight        int
	TextInset             int
	ButtonHorizontalInset int
	FieldLabelGap         int
	FieldGroupGap         int
}

var nativeControls = controlMetricSet{
	InputHeight:           34,
	ButtonHeight:          34,
	PrimaryButtonHeight:   36,
	TableHeaderHeight:     30,
	TableRowHeight:        32,
	TextInset:             8,
	ButtonHorizontalInset: 14,
	FieldLabelGap:         6,
	FieldGroupGap:         20,
}

type layoutRect struct {
	X, Y, Width, Height int
}

func (r layoutRect) Bottom() int { return r.Y + r.Height }

type mainWindowLayout struct {
	Title, Subtitle, Manager, Settings, Status, Workspace layoutRect
}

func mainWindowLayoutFor(bounds layoutRect, tokens controlMetricSet) mainWindowLayout {
	const (
		margin          = 22
		workspaceTop    = 72
		workspaceBottom = 42
		managerWidth    = 174
		settingsWidth   = 124
		controlGap      = 12
		statusGap       = 14
	)
	managerX := bounds.X + bounds.Width - 900
	if minimum := bounds.X + 420; managerX < minimum {
		managerX = minimum
	}
	settingsX := managerX + managerWidth + controlGap
	statusX := settingsX + settingsWidth + statusGap
	return mainWindowLayout{
		Title:     layoutRect{X: bounds.X + margin, Y: bounds.Y + 14, Width: managerX - (bounds.X + margin) - 18, Height: 28},
		Subtitle:  layoutRect{X: bounds.X + margin + 2, Y: bounds.Y + 38, Width: managerX - (bounds.X + margin) - 22, Height: 22},
		Manager:   layoutRect{X: managerX, Y: bounds.Y + 16, Width: managerWidth, Height: tokens.ButtonHeight},
		Settings:  layoutRect{X: settingsX, Y: bounds.Y + 16, Width: settingsWidth, Height: tokens.ButtonHeight},
		Status:    layoutRect{X: statusX, Y: bounds.Y + 16, Width: bounds.X + bounds.Width - margin - statusX, Height: tokens.ButtonHeight},
		Workspace: layoutRect{X: bounds.X + margin, Y: bounds.Y + workspaceTop, Width: bounds.Width - margin*2, Height: bounds.Height - workspaceTop - workspaceBottom},
	}
}

// terminalPanelLayout is the deterministic native geometry for the terminal
// workspace. At the minimum desktop width, secondary actions move to their own
// row instead of letting FLTK proportionally crush localized button labels.
type terminalPanelLayout struct {
	Title, Subtitle, CloseTab, Tabs, Output                layoutRect
	History, Last, Clear, CommandLabel, Command, Stop, Run layoutRect
	Compact                                                bool
}

func terminalPanelLayoutFor(panel layoutRect, tokens controlMetricSet) terminalPanelLayout {
	const (
		inset          = 18
		gap            = 8
		groupGap       = 14
		closeWidth     = 150
		historyWidth   = 86
		lastWidth      = 64
		clearWidth     = 68
		stopWidth      = 68
		runWidth       = 116
		compactAtWidth = 760
	)
	h := tokens.PrimaryButtonHeight
	bottomRowY := panel.Bottom() - 74
	run := layoutRect{X: panel.X + panel.Width - inset - runWidth, Y: bottomRowY, Width: runWidth, Height: h}
	stop := layoutRect{X: run.X - gap - stopWidth, Y: bottomRowY, Width: stopWidth, Height: h}
	command := layoutRect{X: panel.X + inset, Y: bottomRowY, Width: stop.X - groupGap - (panel.X + inset), Height: h}
	historyY := bottomRowY
	compact := panel.Width < compactAtWidth
	if compact {
		historyY = bottomRowY - h - 32
	}
	history := layoutRect{X: panel.X + inset, Y: historyY, Width: historyWidth, Height: h}
	last := layoutRect{X: history.X + history.Width + gap, Y: historyY, Width: lastWidth, Height: h}
	clear := layoutRect{X: last.X + last.Width + gap, Y: historyY, Width: clearWidth, Height: h}
	if !compact {
		command.X = clear.X + clear.Width + groupGap
		command.Width = stop.X - groupGap - command.X
	}
	commandLabel := layoutRect{X: command.X, Y: bottomRowY - 20, Width: command.Width, Height: 18}
	outputBottom := commandLabel.Y - 16
	if compact {
		outputBottom = history.Y - 14
	}
	outputY := panel.Y + 108
	return terminalPanelLayout{
		Title:        layoutRect{X: panel.X + inset, Y: panel.Y + 14, Width: panel.Width - inset*2 - closeWidth - 12, Height: 24},
		Subtitle:     layoutRect{X: panel.X + inset, Y: panel.Y + 38, Width: panel.Width - inset*2, Height: 18},
		CloseTab:     layoutRect{X: panel.X + panel.Width - inset - closeWidth, Y: panel.Y + 18, Width: closeWidth, Height: tokens.ButtonHeight},
		Tabs:         layoutRect{X: panel.X + inset, Y: panel.Y + 62, Width: panel.Width - inset*2, Height: 40},
		Output:       layoutRect{X: panel.X + inset, Y: outputY, Width: panel.Width - inset*2, Height: outputBottom - outputY},
		History:      history,
		Last:         last,
		Clear:        clear,
		CommandLabel: commandLabel,
		Command:      command,
		Stop:         stop,
		Run:          run,
		Compact:      compact,
	}
}

type quickPanelLayout struct {
	Title, Subtitle, SearchLabel, Search, Find, Table          layoutRect
	SummaryTitle, SelectedName, SelectedDetail, SelectedRecent layoutRect
	Connect                                                    layoutRect
}

// quickPanelLayoutFor keeps the main window a fast-launch surface rather than a
// second profile editor. Saved-profile actions live in Connection Manager, so
// the native table remains useful at 1120×720 in every frozen locale.
func quickPanelLayoutFor(panel layoutRect, tokens controlMetricSet) quickPanelLayout {
	const (
		inset        = 14
		titleInset   = 18
		gap          = 8
		searchLabelW = 64
		findWidth    = 104
		connectWidth = 180
		summaryArea  = 126
	)
	innerWidth := panel.Width - inset*2
	searchY := panel.Y + 52
	find := layoutRect{
		X: panel.X + panel.Width - inset - findWidth, Y: searchY,
		Width: findWidth, Height: tokens.ButtonHeight,
	}
	search := layoutRect{
		X: panel.X + titleInset + searchLabelW, Y: searchY,
		Width: find.X - gap - (panel.X + titleInset + searchLabelW), Height: tokens.InputHeight,
	}
	connectY := panel.Bottom() - tokens.PrimaryButtonHeight - 8
	summaryY := connectY - summaryArea
	tableY := panel.Y + 94
	tableBottom := summaryY - 22
	summaryWidth := panel.Width - titleInset*2 - tokens.TextInset
	return quickPanelLayout{
		Title:          layoutRect{X: panel.X + titleInset, Y: panel.Y + 14, Width: panel.Width - titleInset*2, Height: 24},
		Subtitle:       layoutRect{X: panel.X + titleInset, Y: panel.Y + 38, Width: panel.Width - titleInset*2, Height: 18},
		SearchLabel:    layoutRect{X: panel.X + titleInset, Y: searchY, Width: searchLabelW, Height: tokens.InputHeight},
		Search:         search,
		Find:           find,
		Table:          layoutRect{X: panel.X + inset, Y: tableY, Width: innerWidth, Height: tableBottom - tableY},
		SummaryTitle:   layoutRect{X: panel.X + titleInset, Y: summaryY, Width: panel.Width - titleInset*2, Height: 22},
		SelectedName:   layoutRect{X: panel.X + titleInset, Y: summaryY + 28, Width: summaryWidth, Height: 24},
		SelectedDetail: layoutRect{X: panel.X + titleInset, Y: summaryY + 56, Width: summaryWidth, Height: 22},
		SelectedRecent: layoutRect{X: panel.X + titleInset, Y: summaryY + 82, Width: summaryWidth, Height: 22},
		Connect:        layoutRect{X: panel.X + panel.Width - inset - connectWidth, Y: connectY, Width: connectWidth, Height: tokens.PrimaryButtonHeight},
	}
}

type connectionManagerLayout struct {
	Group, Search, Find, Table, Status, CloseAfterConnect layoutRect
	New, Edit, Delete, Test, Favorite, Connect            layoutRect
}

func connectionManagerLayoutFor(tokens controlMetricSet) connectionManagerLayout {
	return connectionManagerLayout{
		Group:             layoutRect{X: 102, Y: 91, Width: 190, Height: tokens.InputHeight},
		Search:            layoutRect{X: 390, Y: 91, Width: 362, Height: tokens.InputHeight},
		Find:              layoutRect{X: 764, Y: 91, Width: 128, Height: tokens.ButtonHeight},
		Table:             layoutRect{X: 28, Y: 143, Width: 864, Height: 382},
		Status:            layoutRect{X: 30, Y: 530, Width: 862, Height: 18},
		CloseAfterConnect: layoutRect{X: 28, Y: 554, Width: 420, Height: tokens.InputHeight},
		New:               layoutRect{X: 28, Y: 594, Width: 94, Height: tokens.ButtonHeight},
		Edit:              layoutRect{X: 134, Y: 594, Width: 94, Height: tokens.ButtonHeight},
		Delete:            layoutRect{X: 240, Y: 594, Width: 112, Height: tokens.ButtonHeight},
		Test:              layoutRect{X: 364, Y: 594, Width: 124, Height: tokens.ButtonHeight},
		Favorite:          layoutRect{X: 500, Y: 594, Width: 180, Height: tokens.ButtonHeight},
		Connect:           layoutRect{X: 692, Y: 592, Width: 200, Height: tokens.PrimaryButtonHeight},
	}
}

type settingsLayout struct {
	Language, Timeout, Save, Cancel layoutRect
	BehaviorTitleY                  int
}

func settingsLayoutFor(tokens controlMetricSet) settingsLayout {
	return settingsLayout{
		Language:       layoutRect{X: 238, Y: 132, Width: 330, Height: tokens.InputHeight},
		Timeout:        layoutRect{X: 238, Y: 178, Width: 120, Height: tokens.InputHeight},
		BehaviorTitleY: 238,
		Cancel:         layoutRect{X: 360, Y: 408, Width: 96, Height: tokens.PrimaryButtonHeight},
		Save:           layoutRect{X: 468, Y: 408, Width: 100, Height: tokens.PrimaryButtonHeight},
	}
}

type connectionEditorLayout struct {
	Name, Group, Type, Host, Port, Username, Password, PasswordHint layoutRect
	WorkingDir, PrivateKey, Save, Cancel                            layoutRect
}

func connectionEditorLayoutFor(tokens controlMetricSet) connectionEditorLayout {
	return connectionEditorLayout{
		Name:         layoutRect{X: 28, Y: 118, Width: 286, Height: tokens.InputHeight},
		Group:        layoutRect{X: 334, Y: 118, Width: 298, Height: tokens.InputHeight},
		Type:         layoutRect{X: 28, Y: 190, Width: 132, Height: tokens.InputHeight},
		Host:         layoutRect{X: 180, Y: 190, Width: 292, Height: tokens.InputHeight},
		Port:         layoutRect{X: 492, Y: 190, Width: 140, Height: tokens.InputHeight},
		Username:     layoutRect{X: 28, Y: 262, Width: 286, Height: tokens.InputHeight},
		Password:     layoutRect{X: 334, Y: 262, Width: 298, Height: tokens.InputHeight},
		PasswordHint: layoutRect{X: 334, Y: 300, Width: 298, Height: 30},
		WorkingDir:   layoutRect{X: 28, Y: 350, Width: 604, Height: tokens.InputHeight},
		PrivateKey:   layoutRect{X: 28, Y: 424, Width: 604, Height: tokens.InputHeight},
		Cancel:       layoutRect{X: 420, Y: 512, Width: 96, Height: tokens.PrimaryButtonHeight},
		Save:         layoutRect{X: 528, Y: 512, Width: 104, Height: tokens.PrimaryButtonHeight},
	}
}
