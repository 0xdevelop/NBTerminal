package guis_ctl_area

import (
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/guis/guis_settings"
	"github.com/0xdevelop/NBTerminal/guis_tools_encryption"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/fltk_go"
)

type ControlActionType int

const (
	ControlActionTypeNone        ControlActionType = iota // 默认 未知 触发类事件
	ControlActionTypeSettingOver                          // 设置项结束
	ControlActionTypeLoadData                             // 加载数据
)

func (ctlActionType ControlActionType) String() string {
	switch ctlActionType {
	case ControlActionTypeSettingOver:
		return "settingOver"
	case ControlActionTypeLoadData:
		return "loadData"
	case ControlActionTypeNone:
		return "none"
	default:
		return "none"
	}
}

type CtlView struct {
	Frame          *guis_cfg.Frame
	SupperWindow   *fltk_go.Window
	Group          *fltk_go.Group
	BackgroundView *fltk_go.Box
	ActionCallBack func(ctlActionType ControlActionType)
	settingButton  *fltk_go.Button
	encToolButton  *fltk_go.Button
	LoadButton     *fltk_go.Button
}

func NewControlAreaView(mainWindow *fltk_go.Window, frame *guis_cfg.Frame, actionCallBack func(ctlActionType ControlActionType)) *CtlView {

	controlArea := fltk_go.NewGroup(0, 0, frame.Width, frame.Height)
	controlArea.SetBox(fltk_go.THIN_UP_FRAME)

	ctlV := &CtlView{
		Frame:          frame,
		SupperWindow:   mainWindow,
		Group:          controlArea,
		BackgroundView: nil,
		ActionCallBack: actionCallBack,
	}

	ctlV.settingButton = fltk_go.NewButton(mainWindow.W()-80-5, 0, 80, 32, locales.GetLocalesMessage("setting.title"))
	ctlV.settingButton.SetCallback(func() {
		guis_settings.NewSettingsWindow(&guis_cfg.Frame{
			X:      mainWindow.W() - 80 - 5,
			Y:      0,
			Width:  800,
			Height: 600,
		}, func() {
			if ctlV.settingButton != nil {
				ctlV.settingButton.SetLabel(locales.GetLocalesMessage("setting.title"))
				guis_settings.RefreshUI()
			}

			if ctlV.encToolButton != nil {
				ctlV.encToolButton.SetLabel(locales.GetLocalesMessage("encryption.title"))
				aLang := locales.GetLanguageFromTag(config.GlobalConfig.Language)
				if aLang == locales.LanguageWithRussia {
					ctlV.encToolButton.Resize(mainWindow.W()-80-85-5*2-95, ctlV.encToolButton.Y(), ctlV.encToolButton.W()+95, ctlV.encToolButton.H())
				} else {
					ctlV.encToolButton.Resize(mainWindow.W()-80-85-5*2, 0, 85, 32)
				}
				guis_tools_encryption.RefreshUI()
			}

			if ctlV.LoadButton != nil {
				ctlV.LoadButton.SetLabel(locales.GetLocalesMessage("button.load"))
			}

			ctlV.ActionCallBack(ControlActionTypeSettingOver)
		})
	})

	ctlV.encToolButton = fltk_go.NewButton(mainWindow.W()-80-85-5*2, 0, 85, 32, locales.GetLocalesMessage("encryption.title"))
	aLang := locales.GetLanguageFromTag(config.GlobalConfig.Language)
	if aLang == locales.LanguageWithRussia {
		ctlV.encToolButton.Resize(mainWindow.W()-80-85-5*2-95, ctlV.encToolButton.Y(), ctlV.encToolButton.W()+95, ctlV.encToolButton.H())
	} else {
		ctlV.encToolButton.Resize(mainWindow.W()-80-85-5*2, 0, 85, 32)
	}

	ctlV.encToolButton.SetCallback(func() {
		guis_tools_encryption.NewEncryptionWindow(&guis_cfg.Frame{
			X:      mainWindow.W() - 80*2 - 10,
			Y:      0,
			Width:  800,
			Height: 600,
		}, func() {

		})
	})

	// 示例更新数据按钮
	ctlV.LoadButton = fltk_go.NewButton(5, 5, 80, 30, locales.GetLocalesMessage("button.load"))

	ctlV.LoadButton.SetCallback(func() {
		ctlV.ActionCallBack(ControlActionTypeLoadData)
	})
	controlArea.Add(ctlV.settingButton)
	controlArea.Add(ctlV.encToolButton)
	controlArea.Add(ctlV.LoadButton)

	controlArea.End()
	mainWindow.Add(controlArea)

	return ctlV
}
