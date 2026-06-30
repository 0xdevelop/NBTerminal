package guis_auth

import (
	"fmt"
	"sync"

	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/fltk_go"
	"github.com/george012/gtbox"
)

type AuthActionType int

const (
	AuthActionNone AuthActionType = iota
	AuthActionLogin
	AuthActionRegister
	AuthActionForgetPassword
)

func (a AuthActionType) String() string {
	return [...]string{"none", "login", "register", "forget_password"}[a]
}

func getAuthTypeWithLocalesStr(localesKey string) AuthActionType {
	switch localesKey {
	case "login":
		return AuthActionLogin
	case "register":
		return AuthActionRegister
	case "forget_password":
		return AuthActionForgetPassword
	default:
		return AuthActionNone
	}
}

var (
	currentAuthView *AuthView
	authViewRunOnce = sync.Once{}
)

type AuthView struct {
	Frame          *guis_cfg.Frame
	SupperWindow   *fltk_go.Window
	Group          *fltk_go.Group
	BackgroundView *fltk_go.Box
	IsLogined      bool
	callbackFunc   func(authActionType AuthActionType, isPassed bool)
}

func createButton(label string, x, y, w, h int, callback func()) *fltk_go.Button {
	btn := fltk_go.NewButton(x, y, w, h, label)
	btn.SetCallback(callback)
	return btn
}

func (av *AuthView) createAuthSubViews(frame *guis_cfg.Frame) {
	aSpace := 5

	av.Group = fltk_go.NewGroup(frame.X, frame.Y, frame.Width, frame.Height)
	av.BackgroundView = fltk_go.NewBox(fltk_go.UP_BOX, av.Group.X(), av.Group.Y(), av.Group.W(), av.Group.H())
	av.Group.Add(av.BackgroundView)

	// imageview
	imgView := fltk_go.NewBox(fltk_go.UP_BOX, av.BackgroundView.W()/2-250-aSpace*4, av.BackgroundView.Y()+av.BackgroundView.H()/2-125, 250, 250, locales.GetLocalesMessage("Image"))
	imgView.SetColor(fltk_go.ColorFromRgb(0, 200, 0))
	imgView.SetAlign(fltk_go.ALIGN_CENTER)
	av.Group.Add(imgView)

	// email input
	emailLabel := fltk_go.NewBox(fltk_go.NO_BOX, imgView.X()+imgView.W()+aSpace*4, imgView.Y()+aSpace*3, 80, 30, locales.GetLocalesMessage("auth.username"))
	emailLabel.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)
	av.Group.Add(emailLabel)
	emailInput := fltk_go.NewInput(emailLabel.X()+emailLabel.W(), emailLabel.Y(), 250, emailLabel.H())
	if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug {
		emailInput.SetValue(config.GetAuthInfo().Username)
	}
	av.Group.Add(emailInput)

	// password
	pwdLabel := fltk_go.NewBox(fltk_go.NO_BOX, emailLabel.X(), emailLabel.Y()+emailLabel.H()+aSpace*3, emailLabel.W(), emailLabel.H(), locales.GetLocalesMessage("auth.password"))
	pwdLabel.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_RIGHT)
	av.Group.Add(pwdLabel)
	pwdInput := fltk_go.NewInput(emailInput.X(), pwdLabel.Y(), emailInput.W(), emailInput.H())
	if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug {
		pwdInput.SetValue(config.GetAuthInfo().Password)
	}
	av.Group.Add(pwdInput)

	// login button
	loginBtn := createButton(
		locales.GetLocalesMessage(fmt.Sprintf("auth.%s", AuthActionLogin.String())),
		pwdLabel.X()+(pwdLabel.W()+pwdInput.W())-(pwdLabel.W()+pwdInput.W())/3,
		pwdInput.Y()+pwdInput.H()+aSpace*4,
		(pwdLabel.W()+pwdInput.W())/3,
		pwdLabel.H(),
		func() {
			if emailInput.Value() != config.GetAuthInfo().Username {
				av.IsLogined = false
				av.callbackFunc(AuthActionLogin, av.IsLogined)
				return
			}

			if pwdInput.Value() != config.GetAuthInfo().Password {
				av.IsLogined = false
				av.callbackFunc(AuthActionLogin, av.IsLogined)
				return
			}

			av.IsLogined = true
			av.callbackFunc(AuthActionLogin, av.IsLogined)
		})
	av.Group.Add(loginBtn)

	// 欢迎标签
	welcomeLabel := fltk_go.NewBox(
		fltk_go.NO_BOX,
		imgView.X(),
		imgView.Y()-35,
		imgView.W()+emailLabel.W()+emailInput.W(),
		30,
		locales.GetLocalesMessage("welcome"),
	)
	welcomeLabel.SetColor(guis_cfg.GUIHexColorWithCustomCyan)

	welcomeLabel.SetLabelColor(guis_cfg.GUIHexColorWithSelBlue)
	welcomeLabel.SetAlign(fltk_go.ALIGN_INSIDE | fltk_go.ALIGN_CENTER)
	av.Group.Add(welcomeLabel)

	av.Group.End()
}

func (av *AuthView) RefreshUI() {
	if av.Group != nil {
		av.SupperWindow.Remove(av.Group)
		av.Group.Hide()
		av.Group.Destroy()
		av.Group = nil
	}

	av.createAuthSubViews(av.Frame)
	av.SupperWindow.Add(av.Group)
	av.Group.Show()
	av.SupperWindow.Redraw()
}

func NewAuthView(supperWindow *fltk_go.Window, frame *guis_cfg.Frame, callback func(authActionType AuthActionType, isPassed bool)) *AuthView {
	authViewRunOnce.Do(func() {
		currentAuthView = &AuthView{
			SupperWindow: supperWindow,
			IsLogined:    false,
			callbackFunc: callback,
			Frame:        frame,
		}
		currentAuthView.createAuthSubViews(frame)

		currentAuthView.SupperWindow.Add(currentAuthView.Group)
		currentAuthView.Group.Show()
	})

	return currentAuthView
}
