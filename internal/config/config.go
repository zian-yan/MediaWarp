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

	Port         uint16              // MediaWarp 监听端口
	MediaServer  MediaServerSetting  // 上游媒体服务器设置
	Logger       LoggerSetting       // 日志设置
	Cache        CacheSetting        // 缓存设置
	ClientFilter ClientFilterSetting // 客户端过滤设置
	HTTPStrm     HTTPStrmSetting     // HTTPSTRM设置

	configFilePath string // 启动时实际使用的配置文件路径（--config 可覆盖）
)

var ErrInvalidPort = errors.New("端口必须是 1-65535 之间的整数")

// 获取版本信息
func Version() *VersionInfo {
	return &version
}

// 配置文件目录
func ConfigDir() string {
	return "config"
}

// 配置文件路径（默认值，除非通过 --config 指定）
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// 当前实际使用的配置文件路径
func ConfigFilePath() string {
	if configFilePath != "" {
		return configFilePath
	}
	return ConfigPath()
}

// 获取日志目录
//
// 总日志目录
// ./logs
func LogDir() string {
	return "logs"
}

// 获取日志目录
//
// 带有日期
// ./logs/2024-9-29
func LogDirWithDate() string {
	return filepath.Join(LogDir(), time.Now().Format("2006-1-2"))
}

// 访问日志文件路径
func AccessLogPath() string {
	return filepath.Join(LogDirWithDate(), "access.log")
}

// 服务日志文件路径
func ServiceLogPath() string {
	return filepath.Join(LogDirWithDate(), "service.log")
}

// 初始化配置
//
// 配置只在启动时从配置文件加载一次，运行期不再回写；
// 修改配置需要编辑配置文件并重启进程
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

// 读取并解析配置文件
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

// 校验当前配置是否可用
//
// 只校验「配错就一定跑不起来」的项，其余保持宽松：
// 缓存 TTL ≤ 0 表示关闭该项缓存，属于合法配置
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

// 校验上游媒体服务器地址
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

// 创建文件夹
//
// 含 API 密钥的配置目录与日志目录都收紧到 0750，避免同机其他用户读取
func createDir() error {
	if err := os.MkdirAll(ConfigDir(), 0o750); err != nil {
		return fmt.Errorf("创建配置文件夹失败: %v", err)
	}
	if err := os.MkdirAll(LogDir(), 0o750); err != nil {
		return fmt.Errorf("创建日志文件夹失败: %v", err)
	}
	return nil
}
