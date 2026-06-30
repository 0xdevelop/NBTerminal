package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/0xdevelop/NBTerminal/api/api_config"
	"github.com/0xdevelop/NBTerminal/locales"
	"github.com/george012/gtbox/gtbox_encryption"
	"github.com/george012/gtbox/gtbox_log"
)

const (
	ProjectName        = "NBTerminal"
	ProjectVersion     = "v0.0.3"
	ProjectBundleID    = "com.0xdevelop.NBTerminal"
	ProjectDescription = "NB Terminal"
	APIPortDefault     = 8765
)

var (
	GlobalConfig *FileConfig
	HardSN       string
)

type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type FileConfig struct {
	Api      *api_config.ApiConfig `yaml:"api" json:"api"`
	Auth     *Auth                 `yaml:"auth" json:"auth"`
	Language string                `yaml:"language" json:"language"`
}

func LoadConfig(file string) error {
	fInfo, err := os.Stat(file)
	if err != nil {
		return err
	}
	if fInfo.IsDir() {
		return errors.New("config file can not be a dir")
	}

	buf, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buf, &GlobalConfig)
	//err = yaml.Unmarshal(buf, &GlobalConfig)
	if err != nil {
		return err
	}

	return nil
}

func SaveConfig(file string) error {
	//config, err := yaml.Marshal(GlobalConfig)
	config, err := json.MarshalIndent(GlobalConfig, "", "    ")

	if err != nil {
		return err
	}

	err = os.WriteFile(file, config, 644)
	if err != nil {
		return err
	}

	return nil
}
func GetAuthInfo() *Auth {
	return &Auth{
		Username: gtbox_encryption.GTDec(GlobalConfig.Auth.Username, "username"),
		Password: gtbox_encryption.GTDec(GlobalConfig.Auth.Password, "password"),
	}
}

func generateDefaultConfigWithJsonContent() []byte {
	fCfg := &FileConfig{
		Api: &api_config.ApiConfig{
			Enabled: true,
			Port:    CurrentApp.NetListenAPIPortDefault,
		},
		Auth: &Auth{
			Username: gtbox_encryption.GTEnc("root", "username"),
			Password: gtbox_encryption.GTEnc("root", "password"),
		},
		Language: locales.LanguageWithEnglish.LanguageTag(),
	}

	jd, _ := json.MarshalIndent(fCfg, "", "  ")
	return jd
}

func SyncConfigFile(cfgFilePath string, defaultJsonContent []byte) {
	gtbox_log.LogInfof("加载配置文件 [%s]", cfgFilePath)
	_, err := os.Stat(cfgFilePath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		// 获取配置文件的父目录路径
		dir := filepath.Dir(cfgFilePath)

		// 检查父目录是否存在
		if _, err = os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			// 创建父目录
			if err = os.MkdirAll(dir, 0755); err != nil {
				gtbox_log.LogErrorf("无法创建目录 [%s]: %s", dir, err.Error())
				return
			}
		}

		// 写入默认配置文件内容
		if defaultJsonContent == nil {
			defaultJsonContent = generateDefaultConfigWithJsonContent()
		}

		err = os.WriteFile(cfgFilePath, defaultJsonContent, 0755)
		if err != nil {
			gtbox_log.LogErrorf("无法写入配置文件 [%s]: %s", cfgFilePath, err.Error())
			return
		}
	} else {
		buf, err := os.ReadFile(cfgFilePath)
		if err != nil {
			gtbox_log.LogErrorf("读取配置文件 [%s] 错误: %s", cfgFilePath, err.Error())

			return
		}
		if len(buf) == 0 {
			gtbox_log.LogErrorf("配置文件重置")
			if defaultJsonContent == nil {
				defaultJsonContent = generateDefaultConfigWithJsonContent()
			} // 写入默认配置文件内容
			err = os.WriteFile(cfgFilePath, defaultJsonContent, 0755)
			if err != nil {
				gtbox_log.LogErrorf("无法写入配置文件 [%s]: %s", cfgFilePath, err.Error())
				return
			}
		}
	}

	err = LoadConfig(cfgFilePath)

	if err != nil {
		gtbox_log.LogErrorf("无法加载配置文件 [%s]: %s", cfgFilePath, err.Error())
		return
	}
}

func LoadData(cfgFilePath string) map[string]map[int64]string {
	gtbox_log.LogInfof("加载数据目录 [%s]", cfgFilePath)
	var bufs = make(map[string]map[int64]string)

	// 获取配置文件的父目录路径
	dir := cfgFilePath

	// 检查目录是否存在
	_, err := os.Stat(dir)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		// 创建目录
		if err = os.MkdirAll(dir, 0755); err != nil {
			gtbox_log.LogErrorf("无法创建目录 [%s]: %s", dir, err.Error())
			return bufs
		}
	}

	// 定义正则表达式，用于匹配以 _数字 结尾的文件
	re := regexp.MustCompile(`.*_\d+$`)

	// 遍历目录，找到所有符合正则表达式的文件
	err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			gtbox_log.LogErrorf("遍历目录 [%s] 错误: %s", dir, err.Error())
			return err
		}

		if !info.IsDir() && re.MatchString(info.Name()) {
			file, err := os.Open(path)
			if err != nil {
				gtbox_log.LogErrorf("读取数据文件 [%s] 错误: %s", path, err.Error())
				return err
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNum := int64(1)
			bufs[info.Name()] = make(map[int64]string)
			for scanner.Scan() {
				bufs[info.Name()][lineNum] = scanner.Text()
				lineNum++
			}

			if err := scanner.Err(); err != nil {
				gtbox_log.LogErrorf("读取数据文件 [%s] 时扫描错误: %s", path, err.Error())
				return err
			}
		}
		return nil
	})

	if err != nil {
		gtbox_log.LogErrorf("读取目录 [%s] 错误: %s", dir, err.Error())
	}

	return bufs
}
