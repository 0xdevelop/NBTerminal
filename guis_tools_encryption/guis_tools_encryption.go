package guis_tools_encryption

import (
	"sync"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/fltk_go"
	"github.com/george012/gtbox/gtbox_encryption"
)

type EncryptionGUI struct {
	Frame            *guis_cfg.Frame
	Group            *fltk_go.Group
	Window           *fltk_go.Window
	saveButton       *fltk_go.Button
	cancelButton     *fltk_go.Button
	inputLable       *fltk_go.Box
	inputView        *fltk_go.Input
	inputLockButton  *fltk_go.Button
	textBuffer       *fltk_go.TextBuffer
	textEditLable    *fltk_go.Box
	textEdit         *fltk_go.TextEditor
	displayBuffer    *fltk_go.TextBuffer
	textDisplayLable *fltk_go.Box
	textDisplay      *fltk_go.TextDisplay
	callbackFunc     func()
}

var (
	currentEncryptionView *EncryptionGUI
	encryptionViewRunOnce = sync.Once{}
)

func (encGui *EncryptionGUI) createEncryptionSubViews(frame *guis_cfg.Frame) {

	encGui.Window = fltk_go.NewWindow(frame.Width, frame.Height, locales.GetLocalesMessage("encryption.title"))

	sSize := guis_cfg.GetScreenSize() // 获取主屏幕的尺寸

	// 设置窗口的最小尺寸，不允许小于这个尺寸
	encGui.Window.SetSizeRange(encGui.Window.W(), encGui.Window.H(), sSize.Width, sSize.Height, 0, 0, false)
	encGui.Window.SetPosition((sSize.Width-frame.Width)/2, (sSize.Height-frame.Height)/2)

	encGui.saveButton = fltk_go.NewButton(encGui.Window.W()/2-150-10, encGui.Window.H()-35*2, 150, 35)
	encGui.saveButton.SetLabel(locales.GetLocalesMessage("button.save"))
	encGui.saveButton.SetLabelColor(guis_cfg.GUIHexColorWithCustomCyan)
	encGui.saveButton.SetCallback(encGui.saveEncryptionOverData)

	encGui.cancelButton = fltk_go.NewButton(encGui.Window.W()/2+10, encGui.Window.H()-35*2, 150, 35)
	encGui.cancelButton.SetLabel(locales.GetLocalesMessage("button.exit"))
	encGui.cancelButton.SetLabelColor(fltk_go.MAGENTA)

	// 取消按钮点击事件
	encGui.cancelButton.SetCallback(func() {

		if encGui.callbackFunc != nil {
			encGui.Window.Redraw()
			encGui.callbackFunc()
		}
		encGui.Window.Destroy()
		currentEncryptionView = nil
	})
	encGui.inputLable = fltk_go.NewBox(fltk_go.NO_BOX, 10, 10, 100, 24)
	encGui.inputLable.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)

	encGui.inputView = fltk_go.NewInput(encGui.inputLable.X()+encGui.inputLable.W(), encGui.inputLable.Y(), encGui.Window.W()-encGui.inputLable.W()*2-30, encGui.inputLable.H())
	encGui.inputView.SetCallbackCondition(fltk_go.WhenChanged)
	encGui.inputLockButton = fltk_go.NewButton(encGui.inputView.X()+encGui.inputView.W()+10, encGui.inputView.Y(), 80, encGui.inputView.H())
	encGui.inputLockButton.SetCallback(func() {
		if encGui.inputView.IsActive() == true {
			encGui.inputView.Deactivate()

		} else {
			encGui.inputView.Activate()
		}
		encGui.refreshLabels()
		encGui.updateTextDisplay()
	})

	// 创建文本缓冲区和编辑器
	encGui.textBuffer = fltk_go.NewTextBuffer()
	encGui.textEditLable = fltk_go.NewBox(fltk_go.NO_BOX, encGui.inputLable.X(), encGui.inputView.Y()+encGui.inputView.H()+20, encGui.inputLable.W(), encGui.inputLable.H())
	encGui.textEditLable.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)

	encGui.textEdit = fltk_go.NewTextEditor(encGui.textEditLable.X()+encGui.textEditLable.W(), encGui.textEditLable.Y(), encGui.Window.W()-encGui.textEditLable.W()-20, 200)
	encGui.textEdit.Activate()
	encGui.textEdit.SetCallbackCondition(fltk_go.WhenChanged)
	encGui.textEdit.SetBuffer(encGui.textBuffer)
	encGui.textEdit.Parent().Resizable(encGui.textEdit)
	encGui.textEdit.SetWrapMode(fltk_go.WRAP_AT_BOUNDS)

	// 创建显示加密结果的文本显示器
	encGui.displayBuffer = fltk_go.NewTextBuffer()
	encGui.textDisplayLable = fltk_go.NewBox(fltk_go.NO_BOX, encGui.inputLable.X(), encGui.textEdit.Y()+encGui.textEdit.H()+20, encGui.textEditLable.W(), encGui.textEditLable.H())
	encGui.textDisplayLable.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)

	encGui.textDisplay = fltk_go.NewTextDisplay(encGui.textEdit.X(), encGui.textDisplayLable.Y(), encGui.textEdit.W(), encGui.textEdit.H())
	encGui.textDisplay.SetBuffer(encGui.displayBuffer)
	encGui.textDisplay.SetWrapMode(fltk_go.WRAP_AT_BOUNDS)

	// 设置文本编辑器内容改变的回调函数
	previousText := encGui.textBuffer.Text()
	encGui.textEdit.SetCallback(func() {

		if encGui.inputView.IsActive() == true {
			// 恢复文本编辑器内容
			encGui.textBuffer.SetText(previousText)
			fltk_go.MessageBox("!!!---warning---!!!", locales.GetLocalesMessage("encryption.alert.password_no_set"))
			return
		} else {
			// 更新显示内容之前，保存当前内容
			previousText = encGui.textBuffer.Text()
		}
		encGui.updateTextDisplay()
	})

	// 添加组件到窗口
	encGui.Window.Add(encGui.saveButton)
	encGui.Window.Add(encGui.cancelButton)
	encGui.Window.Add(encGui.inputLable)
	encGui.Window.Add(encGui.inputView)
	encGui.Window.Add(encGui.inputLockButton)
	encGui.Window.Add(encGui.textEditLable)
	encGui.Window.Add(encGui.textEdit)
	encGui.Window.Add(encGui.textDisplayLable)
	encGui.Window.Add(encGui.textDisplay)

	// 设置窗口关闭事件的回调函数
	encGui.Window.SetCallback(func() {
		encGui.Window.Destroy()
		currentEncryptionView = nil
	})

	encGui.Window.End()
}

func (encGui *EncryptionGUI) updateTextDisplay() {
	encryptedText := gtbox_encryption.GTEnc(encGui.textBuffer.Text(), encGui.inputView.Value())
	encGui.textDisplay.Buffer().SetText(encryptedText)
}

func (encGui *EncryptionGUI) refreshLabels() {
	if encGui == nil {
		return
	}
	encGui.Window.SetLabel(locales.GetLocalesMessage("encryption.title"))
	encGui.inputLable.SetLabel(locales.GetLocalesMessage("encryption.lable_input"))
	if encGui.inputView.IsActive() == true {
		if locales.GetLanguageFromTag(config.GlobalConfig.Language) == locales.LanguageWithRussia {
			encGui.inputView.Resize(encGui.inputLable.X()+encGui.inputLable.W(), encGui.inputLable.Y(), encGui.Window.W()-encGui.inputLable.W()*2-90, encGui.inputLable.H())
			encGui.inputLockButton.Resize(encGui.inputView.X()+encGui.inputView.W()+10, encGui.inputView.Y(), 160, encGui.inputView.H())
		} else if locales.GetLanguageFromTag(config.GlobalConfig.Language) == locales.LanguageWithEnglish {
			encGui.inputView.Resize(encGui.inputLable.X()+encGui.inputLable.W(), encGui.inputLable.Y(), encGui.Window.W()-encGui.inputLable.W()*2-50, encGui.inputLable.H())
			encGui.inputLockButton.Resize(encGui.inputView.X()+encGui.inputView.W()+10, encGui.inputView.Y(), 120, encGui.inputView.H())
		}
		encGui.inputLockButton.SetLabel(locales.GetLocalesMessage("encryption.button_input_lock"))
	} else {
		if locales.GetLanguageFromTag(config.GlobalConfig.Language) == locales.LanguageWithRussia {
			encGui.inputView.Resize(encGui.inputLable.X()+encGui.inputLable.W(), encGui.inputLable.Y(), encGui.Window.W()-encGui.inputLable.W()*2-180, encGui.inputLable.H())
			encGui.inputLockButton.Resize(encGui.inputView.X()+encGui.inputView.W()+10, encGui.inputView.Y(), 250, encGui.inputView.H())
		} else if locales.GetLanguageFromTag(config.GlobalConfig.Language) == locales.LanguageWithEnglish {
			encGui.inputView.Resize(encGui.inputLable.X()+encGui.inputLable.W(), encGui.inputLable.Y(), encGui.Window.W()-encGui.inputLable.W()*2-130, encGui.inputLable.H())
			encGui.inputLockButton.Resize(encGui.inputView.X()+encGui.inputView.W()+10, encGui.inputView.Y(), 200, encGui.inputView.H())
		}
		encGui.inputLockButton.SetLabel(locales.GetLocalesMessage("encryption.button_input_unlock"))
	}

	encGui.textEditLable.SetLabel(locales.GetLocalesMessage("encryption.lable_textEdit"))
	encGui.textDisplayLable.SetLabel(locales.GetLocalesMessage("encryption.lable_textDisplay"))
	encGui.saveButton.SetLabel(locales.GetLocalesMessage("button.save"))
	encGui.cancelButton.SetLabel(locales.GetLocalesMessage("button.exit"))
	encGui.Window.Redraw()
}

func (encGui *EncryptionGUI) saveEncryptionOverData() {
	if encGui.inputView.IsActive() == false {

	}
}

func RefreshUI() {
	if currentEncryptionView != nil {
		currentEncryptionView.refreshLabels()
		currentEncryptionView.Window.Redraw()
	}
}

func NewEncryptionWindow(frame *guis_cfg.Frame, callback func()) *EncryptionGUI {
	if currentEncryptionView == nil {
		currentEncryptionView = &EncryptionGUI{
			callbackFunc: callback,
			Frame:        frame,
		}
		currentEncryptionView.createEncryptionSubViews(frame)
	}

	currentEncryptionView.refreshLabels()
	currentEncryptionView.Window.Show()
	return currentEncryptionView
}
