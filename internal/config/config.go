package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/AkimioJR/MediaWarp/constants"

	"gopkg.in/yaml.v3"
)

var (
	version = VersionInfo{
		AppVersion: appVersion,
		CommitHash: commitHash,
		BuildDate:  parseBuildTime(buildDate),
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}

	Port         uint16
	MediaServer  MediaServerSetting
	Logger       LoggerSetting
	Cache        CacheSetting
	ClientFilter ClientFilterSetting
	HTTPStrm     HTTPStrmSetting

	configFilePath string
)

var ErrInvalidPort = errors.New("端口必须是 1-65535 之间的整数")

func Version() *VersionInfo {
	return &version
}

func ConfigDir() string {
	return "config"
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

func ConfigFilePath() string {
	if configFilePath != "" {
		return configFilePath
	}
	return ConfigPath()
}

func LogDir() string {
	return "logs"
}

func LogDirWithDate() string {
	return filepath.Join(LogDir(), time.Now().Format("2006-1-2"))
}

func AccessLogPath() string {
	return filepath.Join(LogDirWithDate(), "access.log")
}

func ServiceLogPath() string {
	return filepath.Join(LogDirWithDate(), "service.log")
}

func Init(path string) error {
	configFilePath = path
	if err := loadConfig(path); err != nil {
		return err
	}
	if err := createDir(); err != nil {
		return err
	}
	return nil
}

func loadConfig(path string) error {
	var s Setting
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	Port = s.Port
	MediaServer = s.MediaServer
	Logger = s.Logger
	Cache = s.Cache
	ClientFilter = s.ClientFilter
	HTTPStrm = s.HTTPStrm

	return validate()
}

func validate() error {
	if Port < 1 || Port > 65535 {
		return fmt.Errorf("%w，当前值: %d", ErrInvalidPort, Port)
	}

	switch MediaServer.Type {
	case constants.EMBY, constants.JELLYFIN:
	default:
		return fmt.Errorf("媒体服务器类型无效: %s（可选：Emby、Jellyfin）", MediaServer.Type)
	}

	if err := validateAddr(MediaServer.ADDR); err != nil {
		return err
	}

	if ClientFilter.Enable && ClientFilter.Mode == constants.WHITELIST && len(ClientFilter.ClientList) == 0 {
		return errors.New("白名单模式下 client.list 不能为空，否则所有请求都会被拒绝")
	}

	return nil
}

func validateAddr(addr string) error {
	if addr == "" {
		return errors.New("server.addr 不能为空")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("server.addr 格式错误: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("server.addr 必须以 http:// 或 https:// 开头，当前值: %s", addr)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("server.addr 缺少主机名，当前值: %s", addr)
	}
	return nil
}

func createDir() error {
	if err := os.MkdirAll(ConfigDir(), 0o750); err != nil {
		return fmt.Errorf("创建配置文件夹失败: %v", err)
	}
	if err := os.MkdirAll(LogDir(), 0o750); err != nil {
		return fmt.Errorf("创建日志文件夹失败: %v", err)
	}
	return nil
}
