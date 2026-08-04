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

type connectionManagerLayout struct {
	Search, Find, Table, Status, CloseAfterConnect layoutRect
	New, Edit, Delete, Favorite, Connect           layoutRect
}

func connectionManagerLayoutFor(tokens controlMetricSet) connectionManagerLayout {
	return connectionManagerLayout{
		Search:            layoutRect{X: 102, Y: 91, Width: 650, Height: tokens.InputHeight},
		Find:              layoutRect{X: 764, Y: 91, Width: 128, Height: tokens.ButtonHeight},
		Table:             layoutRect{X: 28, Y: 143, Width: 864, Height: 382},
		Status:            layoutRect{X: 30, Y: 530, Width: 862, Height: 18},
		CloseAfterConnect: layoutRect{X: 28, Y: 554, Width: 420, Height: tokens.InputHeight},
		New:               layoutRect{X: 28, Y: 594, Width: 94, Height: tokens.ButtonHeight},
		Edit:              layoutRect{X: 134, Y: 594, Width: 94, Height: tokens.ButtonHeight},
		Delete:            layoutRect{X: 240, Y: 594, Width: 112, Height: tokens.ButtonHeight},
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
	Name, Group, Type, Host, Port, Username, Password layoutRect
	WorkingDir, PrivateKey, Save, Cancel              layoutRect
}

func connectionEditorLayoutFor(tokens controlMetricSet) connectionEditorLayout {
	return connectionEditorLayout{
		Name:       layoutRect{X: 28, Y: 118, Width: 286, Height: tokens.InputHeight},
		Group:      layoutRect{X: 334, Y: 118, Width: 298, Height: tokens.InputHeight},
		Type:       layoutRect{X: 28, Y: 190, Width: 132, Height: tokens.InputHeight},
		Host:       layoutRect{X: 180, Y: 190, Width: 292, Height: tokens.InputHeight},
		Port:       layoutRect{X: 492, Y: 190, Width: 140, Height: tokens.InputHeight},
		Username:   layoutRect{X: 28, Y: 262, Width: 286, Height: tokens.InputHeight},
		Password:   layoutRect{X: 334, Y: 262, Width: 298, Height: tokens.InputHeight},
		WorkingDir: layoutRect{X: 28, Y: 350, Width: 604, Height: tokens.InputHeight},
		PrivateKey: layoutRect{X: 28, Y: 424, Width: 604, Height: tokens.InputHeight},
		Cancel:     layoutRect{X: 420, Y: 512, Width: 96, Height: tokens.PrimaryButtonHeight},
		Save:       layoutRect{X: 528, Y: 512, Width: 104, Height: tokens.PrimaryButtonHeight},
	}
}
