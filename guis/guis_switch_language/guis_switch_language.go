package guis_switch_language

import (
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/guis/guis_cfg"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/fltk_go"
)

var (
	languageChoice *fltk_go.Choice
)

func NewLanguageSwitcher(frame *guis_cfg.Frame, callback func(selectLanguage locales.Language)) *fltk_go.Choice {
	if languageChoice == nil {
		DefaultLanguage := locales.GetLanguageFromTag(config.GlobalConfig.Language)

		// 初始化选择语言的菜单
		languageChoice = fltk_go.NewChoice(frame.X, frame.Y, frame.Width, frame.Height)

		languageChoice.Add(locales.LanguageWithEnglish.String(), func() {
			DefaultLanguage = locales.LanguageWithEnglish
			locales.ResetLocaleLanguage(locales.LanguageWithEnglish.LanguageTag())
			callback(locales.LanguageWithEnglish)
		})
		languageChoice.Add(locales.LanguageWithRussia.String(), func() {
			DefaultLanguage = locales.LanguageWithRussia
			locales.ResetLocaleLanguage(locales.LanguageWithRussia.LanguageTag())
			callback(locales.LanguageWithRussia)
		})
		languageChoice.Add(locales.LanguageWithZhHK.String(), func() {
			DefaultLanguage = locales.LanguageWithZhHK
			locales.ResetLocaleLanguage(locales.LanguageWithZhHK.LanguageTag())
			callback(locales.LanguageWithZhHK)
		})
		languageChoice.Add(locales.LanguageWithZhCN.String(), func() {
			DefaultLanguage = locales.LanguageWithZhCN
			locales.ResetLocaleLanguage(locales.LanguageWithZhCN.LanguageTag())
			callback(locales.LanguageWithZhCN)
		})

		// 设置默认选择项
		languageChoice.SetValue(int(DefaultLanguage))

		// 手动触发默认选择项
		locales.ResetLocaleLanguage(DefaultLanguage.LanguageTag())
		callback(DefaultLanguage)
	}

	return languageChoice
}

func selectChoiceItemMethod() {

}

func RemoveFromSupperView() {
	languageChoice = nil
}
