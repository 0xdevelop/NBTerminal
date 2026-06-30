package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/0xdevelop/NBTerminal/api"
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/0xdevelop/NBTerminal/custom_cmd"
	"github.com/0xdevelop/NBTerminal/guis"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/gtbox"
	"github.com/george012/gtbox/gtbox_cmd"
	"github.com/george012/gtbox/gtbox_encryption"
	"github.com/george012/gtbox/gtbox_log"
	"github.com/george012/gtbox/gtbox_sys"
)

//go:embed resources/imgs/Icon.png
var aIcon []byte

var (
	mRunMode       = ""
	mGitCommitHash = ""
	mGitCommitTime = ""
	mPackageOS     = ""
	mPackageTime   = ""
	mGoVersion     = ""
)

const (
	ICON_SMALL = 0
	ICON_BIG   = 1
)

func SetupApp() {
	runMode := gtbox.RunModeDebug
	switch mRunMode {
	case "debug":
		runMode = gtbox.RunModeDebug
	case "test":
		runMode = gtbox.RunModeTest
	case "release":
		runMode = gtbox.RunModeRelease
	default:
		runMode = gtbox.RunModeDebug
	}

	config.CurrentApp = config.NewApp(
		config.ProjectName,
		config.ProjectBundleID,
		config.ProjectDescription,
		runMode,
		config.APIPortDefault,
	)

	//	TODO 初始化gtbox及log分片
	if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug {
		cmdMap := map[string]string{
			"git_commit_hash": "git show -s --format=%H",
			"git_commit_time": "git show -s --format=\"%ci\" | cut -d ' ' -f 1,2 | sed 's/ /_/'",
			"build_os":        "go env GOOS",
			"go_version":      "go version | awk '{print $3}'",
		}
		cmdRes := gtbox_cmd.RunWith(cmdMap)

		if cmdRes != nil {
			mGitCommitHash = cmdRes["git_commit_hash"]
			mGitCommitTime = cmdRes["git_commit_time"]
			mPackageOS = cmdRes["build_os"]
			mGoVersion = cmdRes["go_version"]
			mPackageTime = time.Now().UTC().Format("2006-01-02_15:04:05")
		}
	}

	config.CurrentApp.GitCommitHash = mGitCommitHash
	config.CurrentApp.GitCommitTime = mGitCommitTime
	config.CurrentApp.GoVersion = mGoVersion
	config.CurrentApp.PackageOS = mPackageOS
	config.CurrentApp.PackageTime = mPackageTime

	custom_cmd.HandleCustomCmds(os.Args, config.CurrentApp)

	gtbox.SetupGTBox(config.CurrentApp.AppName,
		config.CurrentApp.CurrentRunMode,
		config.CurrentApp.AppLogPath,
		30,
		gtbox_log.GTLogSaveHours,
		config.CurrentApp.HTTPRequestTimeOut,
	)

	en_str := gtbox_encryption.GTEnc("app starting...", "hello")
	gtbox_log.LogInfof(gtbox_encryption.GTDec(en_str, "hello"))

	hard_infos := gtbox_sys.GTGetHardInfo()
	snStr := fmt.Sprintf("%s|%s|%s|%s", hard_infos.CPUNumber, hard_infos.BaseBoardNumber, hard_infos.BiosNumber, hard_infos.DiskNumber)
	snStrEnc := gtbox_encryption.GTEnc(snStr, "sn")
	config.HardSN = snStrEnc
}

func main() {
	// 锁定当前的 goroutine 到操作系统线程
	runtime.LockOSThread()

	SetupApp()

	config.SyncConfigFile(config.CurrentApp.AppConfigFilePath, nil)

	adts := config.LoadData(config.CurrentApp.DataDir)

	gtbox_log.LogDebugf("%v", adts)

	locales.ResetLocaleLanguage(locales.GetLanguageFromTag(config.GlobalConfig.Language).LanguageTag())
	//
	api.StartAPIService(config.GlobalConfig.Api)

	guis.LoadGUIWithFLTKGO(aIcon)

}
