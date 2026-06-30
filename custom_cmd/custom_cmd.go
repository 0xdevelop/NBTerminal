package custom_cmd

import (
	"fmt"
	"strings"

	"github.com/0xdevelop/NBTerminal/common"
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/george012/gtbox"
)

var (
	customcommands = []string{"version", "go"}
)

func versionAction(app *config.ExtendApp) {
	fmt.Printf("  名       字  :  %s\n", app.AppName)
	fmt.Printf("  包       名  :  %s\n", app.BundleID)
	fmt.Printf("  版       本  :  %s\n", app.Version)
	fmt.Printf("  描       述  :  %s\n", app.Description)
	fmt.Printf("打  包  模  式 :  %s\n", app.CurrentRunMode.String())

	if len(app.GitCommitHash) > 0 {
		fmt.Printf("Git 提 交 Hash :  %s\n", app.GitCommitHash[:10])
	} else {
		fmt.Printf("Git 提 交 Hash :  %s\n", app.GitCommitHash)

	}
	fmt.Printf("Git提交时间 UTC:  %s\n", app.GitCommitTime)
	fmt.Printf("构  建  语  言 :  %s\n", app.GoVersion)
	fmt.Printf("构  建  系  统 :  %s\n", app.PackageOS)
	fmt.Printf("构 建 时 间 UTC:  %s\n", app.PackageTime)
}

func HandleCustomCmds(args []string, sApp *config.ExtendApp) {
	if len(args) == 1 {
		return
	}

	a_flag := args[1]
	isContinue := false
	for _, a_cmd := range customcommands {
		if a_cmd == a_flag {
			isContinue = true
			break
		} else {
			isContinue = false
		}
	}

	// 支持以 -test. 开头的命令
	if !isContinue && strings.HasPrefix(a_flag, "-test.") {
		isContinue = true
	}

	if isContinue == false {
		fmt.Printf("not allow cmd\n")
		common.ExitApp()
	}

	switch a_flag {
	case "version":
		versionAction(sApp)
	default:
		if strings.HasPrefix(a_flag, "-test.") {
			if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug {
				return
			}
		}
	}
	common.ExitApp()
}
