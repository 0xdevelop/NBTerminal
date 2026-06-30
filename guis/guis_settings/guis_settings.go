package guis_settings

import (
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/guis/guis_switch_language"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/fltk_go"
	"github.com/george012/gtbox/gtbox_log"
)

type SettingsGUI struct {
	Frame          *guis_cfg.Frame
	Group          *fltk_go.Group
	Window         *fltk_go.Window
	okButton       *fltk_go.Button
	titleLabel     *fltk_go.Box
	languageChoice *fltk_go.Choice
	callbackFunc   func()
	snDispayBuffer *fltk_go.TextBuffer
	snDisplayLabel *fltk_go.Box
	snDisplayView  *fltk_go.TextDisplay
}

var (
	currentSettingsView *SettingsGUI
)

func (gui *SettingsGUI) createSettingsSubViews(frame *guis_cfg.Frame) {
	grap := 5

	gui.Window = fltk_go.NewWindow(frame.Width, frame.Height, locales.GetLocalesMessage("setting.title"))

	sSize := guis_cfg.GetScreenSize() // 获取主屏幕的尺寸

	// 设置窗口的最小尺寸，不允许小于这个尺寸
	gui.Window.SetSizeRange(gui.Window.W(), gui.Window.H(), sSize.Width, sSize.Height, 0, 0, false)
	gui.Window.SetPosition((sSize.Width-frame.Width)/2, (sSize.Height-frame.Height)/2)

	gui.okButton = fltk_go.NewButton(gui.Window.W()/2-75, gui.Window.H()-65, 150, 35)
	gui.okButton.SetLabel(locales.GetLocalesMessage("button.save"))

	// 左边的标题项
	gui.titleLabel = fltk_go.NewBox(fltk_go.NO_BOX, frame.Width/2-200-grap*2, grap, 200, 40, locales.GetLocalesMessage("setting.language")+" :")
	gui.titleLabel.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)

	// 右边的语言选择项
	gui.languageChoice = guis_switch_language.NewLanguageSwitcher(&guis_cfg.Frame{
		X:      frame.Width/2 + grap*2,
		Y:      gui.titleLabel.Y(),
		Width:  150,
		Height: 35,
	}, func(selectLanguage locales.Language) {
		gtbox_log.LogDebugf("choice language[%s]", selectLanguage.String())

		gui.refreshLabels()
		gui.Window.Redraw()
	})

	// 创建显示加密结果的文本显示器
	gui.snDispayBuffer = fltk_go.NewTextBuffer()
	gui.snDisplayLabel = fltk_go.NewBox(fltk_go.NO_BOX, gui.titleLabel.X(), gui.titleLabel.Y()+gui.titleLabel.H()+5, gui.titleLabel.W(), gui.titleLabel.H(), "SN: ")
	gui.snDisplayLabel.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)

	gui.snDisplayView = fltk_go.NewTextDisplay(gui.languageChoice.X(), gui.snDisplayLabel.Y(), gui.Window.W()/2-30, 72)
	gui.snDisplayView.SetWrapMode(fltk_go.WRAP_AT_BOUNDS)
	gui.snDisplayView.SetBuffer(gui.snDispayBuffer)
	gui.snDisplayView.Buffer().SetText(config.HardSN)

	// 确定按钮
	gui.okButton.SetCallback(func() {
		aSlect := gui.languageChoice.Value()
		aLang := locales.Language(aSlect)
		config.GlobalConfig.Language = aLang.LanguageTag()

		if gui.callbackFunc != nil {
			gui.Window.Redraw()
			gui.callbackFunc()
		}

		gui.destroy()
	})

	gui.Window.Add(gui.titleLabel)
	gui.Window.Add(gui.languageChoice)
	gui.Window.Add(gui.snDisplayLabel)
	gui.Window.Add(gui.snDisplayView)
	gui.Window.Add(gui.okButton)

	// 设置窗口关闭事件的回调函数
	gui.Window.SetCallback(func() {
		gui.destroy()
	})
	gui.Window.End()
}

func (gui *SettingsGUI) refreshLabels() {
	gui.Window.SetLabel(locales.GetLocalesMessage("setting.title"))
	gui.titleLabel.SetLabel(locales.GetLocalesMessage("setting.language") + " :")
	gui.okButton.SetLabel(locales.GetLocalesMessage("button.save"))
}

func (gui *SettingsGUI) destroy() {
	if currentSettingsView != nil {
		guis_switch_language.RemoveFromSupperView()
		gui.Window.Destroy()
		currentSettingsView = nil
	}
}

func RefreshUI() {
	if currentSettingsView != nil {
		currentSettingsView.refreshLabels()
		currentSettingsView.Window.Redraw()
	}
}

func NewSettingsWindow(frame *guis_cfg.Frame, callback func()) *SettingsGUI {
	if currentSettingsView == nil {
		currentSettingsView = &SettingsGUI{
			callbackFunc: callback,
			Frame:        frame,
		}
		currentSettingsView.createSettingsSubViews(frame)
	}

	currentSettingsView.Window.Show()
	return currentSettingsView
}
