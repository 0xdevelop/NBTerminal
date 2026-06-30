package common

import (
	"github.com/0xdevelop/NBTerminal/config"
	"github.com/george012/gtbox"
	"github.com/george012/gtbox/gtbox_coding"
	"github.com/george012/gtbox/gtbox_encryption"
	"github.com/george012/gtbox/gtbox_log"
	"github.com/george012/gtbox/gtbox_net"
	"os"
	"os/signal"
	"syscall"
)

func LoadSigHandle(cleanAction func(), testMethods []func()) {
	if config.CurrentApp == nil {
		return
	}

	if config.CurrentApp.CurrentRunMode == gtbox.RunModeDebug {
		testMethod(testMethods)
	}
	// 创建一个信号通道
	chSig := make(chan os.Signal)
	signal.Notify(chSig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGKILL)

	// 阻断主进程等待signal
	asig := <-chSig
	if cleanAction != nil {
		cleanAction()
	}
	gtbox_log.LogInfof("接收到 [%s] 信号，程序即将退出! ", asig)
	willExitHandle()
}

// willExitHandle 异常退出处理
func willExitHandle() {
	gtbox_log.LogInfof("[程序关闭]---[处理缓存数据] ")

	// 退出
	ExitApp()
}

func testMethod(testMethods []func()) {
	gtbox_log.LogDebugf("enc_lib version: %v", gtbox_encryption.GetEncryptionLibVersion())

	line_No := gtbox_coding.GetProjectCodeLines()
	gtbox_log.LogDebugf("项目有效代码总行数: %v", line_No)
	gtbox_log.LogDebugf("当前公网IP: %v", gtbox_net.GTGetPublicIPV4())
	for _, method := range testMethods {
		go method()
		gtbox_log.LogDebugf("开始执行测试方法: %v", method)

	}
}

func ExitApp() {
	// 发送 os.Interrupt 信号以触发正常退出
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(os.Interrupt)
}
