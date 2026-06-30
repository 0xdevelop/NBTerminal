package guis

import (
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_auth"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/guis/guis_ctl_area"
	"github.com/0xdevelop/NBTerminal/guis/guis_main"
	"bytes"
	"github.com/george012/fltk_go"
	"github.com/george012/gtbox/gtbox_log"
	"image"
	"image/draw"
	"image/png"
	"log"
)

var (
	MainWindow *fltk_go.Window
	CtlView    *guis_ctl_area.CtlView
	AuthView   *guis_auth.AuthView
	MainView   *guis_main.MainView
)

func decodePngToRgbImage(data []byte) (*fltk_go.RgbImage, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	return fltk_go.NewRgbImage(rgba.Pix, bounds.Dx(), bounds.Dy(), 4)
}

func LoadGUIWithFLTKGO(iConBuffer []byte) {
	// 锁定 FLTK 库
	fltk_go.Lock()
	icon, err := decodePngToRgbImage(iConBuffer)
	if err != nil {
		log.Fatalf("failed to decode icon: %v", err)
	}

	// 初始化 FLTK 样式
	fltk_go.InitStyles()

	// 创建主窗口
	MainWindow = fltk_go.NewWindow(guis_cfg.DefaultWindowSize.Width, guis_cfg.DefaultWindowSize.Height)
	MainWindow.SetLabel(config.CurrentApp.AppName)
	MainWindow.SetColor(guis_cfg.GUIHexColorWithLightGray)
	MainWindow.SetIcons([]*fltk_go.RgbImage{icon})
	// 获取主屏幕的尺寸
	sSize := guis_cfg.GetScreenSize()

	// 设置窗口的最小尺寸，不允许小于这个尺寸
	MainWindow.SetSizeRange(800, 600, sSize.Width, sSize.Height, 0, 0, false)
	MainWindow.SetPosition(sSize.Width/2-guis_cfg.DefaultWindowSize.Width/2, sSize.Height/2-guis_cfg.DefaultWindowSize.Height/2)

	CtlView = guis_ctl_area.NewControlAreaView(MainWindow, &guis_cfg.Frame{
		X:      0,
		Y:      0,
		Width:  MainWindow.W(),
		Height: 80,
	}, func(ctlActionType guis_ctl_area.ControlActionType) {
		gtbox_log.LogDebugf("handle %s", ctlActionType.String())

		if ctlActionType == guis_ctl_area.ControlActionTypeLoadData {
			if AuthView != nil {
				fltk_go.MessageBox("!!!---warning---!!!", "must login !!!")
			} else {

			}
		}
		config.SaveConfig(config.CurrentApp.AppConfigFilePath)
		if AuthView != nil {
			// 重新绘制窗口
			AuthView.RefreshUI()
		}

		if MainView != nil {
			MainView.ReFreshData()
			MainView.RefreshUI()
		}
	})

	AuthView = guis_auth.NewAuthView(MainWindow, &guis_cfg.Frame{
		X:      0,
		Y:      CtlView.Frame.Y + CtlView.Frame.Height,
		Width:  MainWindow.W(),
		Height: MainWindow.H() - CtlView.Frame.Height,
	}, func(authActionType guis_auth.AuthActionType, isPassed bool) {
		gtbox_log.LogDebugf("auth action with[%s]", authActionType)
		switch authActionType {
		case guis_auth.AuthActionLogin:
			if isPassed {

				MainView = guis_main.NewMainView(MainWindow, &guis_cfg.Frame{
					X:      AuthView.Frame.X,
					Y:      CtlView.Frame.Y + CtlView.Frame.Height,
					Width:  AuthView.Frame.Width,
					Height: AuthView.Frame.Height,
				}, func() {

				})

				MainWindow.Remove(AuthView.Group) // 移除当前视图
				AuthView = nil
				// 重新绘制窗口
				MainWindow.Redraw()
			}
		case guis_auth.AuthActionRegister:
		case guis_auth.AuthActionForgetPassword:
		default:
		}
	})

	// 启用窗口的可调整大小功能
	MainWindow.Resizable(MainWindow)

	MainWindow.End()
	MainWindow.Show()
	fltk_go.Run()
}
