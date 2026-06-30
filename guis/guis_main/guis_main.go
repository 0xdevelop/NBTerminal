package guis_main

import (
	"sync"

	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/george012/fltk_go"
)

var (
	currentMainView *MainView
	mainViewRunOnce = sync.Once{}
)

type MainView struct {
	Frame          *guis_cfg.Frame
	SupperWindow   *fltk_go.Window
	Group          *fltk_go.Group
	BackgroundView *fltk_go.Box
	callbackFunc   func()
}

func (mv *MainView) drawMainSubViews(frame *guis_cfg.Frame) {
	mv.Group = fltk_go.NewGroup(frame.X, frame.Y, frame.Width, frame.Height)

	mv.BackgroundView = fltk_go.NewBox(fltk_go.UP_BOX, mv.Group.X(), mv.Group.Y(), mv.Group.W(), mv.Group.H())
	mv.BackgroundView.SetColor(fltk_go.YELLOW)
	mv.Group.Add(mv.BackgroundView)

	mv.Group.End()
}

func (mv *MainView) RefreshUI() {

	mv.SupperWindow.Redraw()
}

func (mv *MainView) ReFreshData() {

	mv.Group.Redraw()
}

func NewMainView(supperWindow *fltk_go.Window, frame *guis_cfg.Frame, callback func()) *MainView {
	mainViewRunOnce.Do(func() {
		currentMainView = &MainView{
			SupperWindow: supperWindow,
			callbackFunc: callback,
			Frame:        frame,
		}
		currentMainView.drawMainSubViews(frame)

		currentMainView.SupperWindow.Add(currentMainView.Group)
		currentMainView.Group.Show()
	})

	return currentMainView
}
