package config

import (
	"time"

	"github.com/AkimioJR/MediaWarp/constants"
)

// 程序版本信息
type VersionInfo struct {
	AppVersion string `json:"app_version"` // 程序版本号
	CommitHash string `json:"commit_hash"` // GIt Commit Hash
	BuildDate  string `json:"build_date"`  // 编译时间
	GoVersion  string `json:"go_version"`  // 编译 Golang 版本
	OS         string `json:"os"`          // 操作系统
	Arch       string `json:"arch"`        //  架构
}

// 上游媒体服务器相关设置
type MediaServerSetting struct {
	Type constants.MediaServerType `yaml:"type"` // 媒体服务器类型
	ADDR string                    `yaml:"addr"` // 地址
	AUTH string                    `yaml:"auth"` // 认证授权KEY
}

// 日志设置
type LoggerSetting struct {
	AccessLogger  BaseLoggerSetting `yaml:"access"`  // 访问日志相关配置
	ServiceLogger BaseLoggerSetting `yaml:"service"` // 服务日志相关配置
}

// 基础日志配置字段
type BaseLoggerSetting struct {
	Console bool `yaml:"console"` // 是否将日志输出到终端中
	File    bool `yaml:"file"`    // 是否将日志输出到文件中
}

// 缓存设置
//
// 保留下来的缓存项：HTTPStrm 重定向结果、图片、字幕
type CacheSetting struct {
	Enable      bool          `yaml:"enable"`
	HTTPStrmTTL time.Duration `yaml:"http_strm_ttl"`
	ImageTTL    time.Duration `yaml:"image_ttl"`
	SubtitleTTL time.Duration `yaml:"subtitle_ttl"`
}

// 客户端User-Agent过滤设置
type ClientFilterSetting struct {
	Enable     bool                 `yaml:"enable"`
	Mode       constants.FliterMode `yaml:"mode"`
	ClientList []string             `yaml:"list"`
}

// HTTPStrm播放设置
type HTTPStrmSetting struct {
	Enable            bool     `yaml:"enable"`
	Proxy             bool     `yaml:"proxy"`              // 开启后支持媒体服务器串流、转码播放
	FinalURL          bool     `yaml:"final_url"`          // 对 URL 进行重定向判断，找到非重定向地址再重定向给客户端，减少客户端重定向次数
	CompatibilityMode bool     `yaml:"compatibility_mode"` // 兼容模式，开启后将使用更兼容但效率较低的方式获取最终 URL
	PrefixList        []string `yaml:"prefix_list"`
}

type Setting struct {
	Port         uint16              `yaml:"port"`
	MediaServer  MediaServerSetting  `yaml:"server"`
	Logger       LoggerSetting       `yaml:"log"`
	Cache        CacheSetting        `yaml:"cache"`
	ClientFilter ClientFilterSetting `yaml:"client"`
	HTTPStrm     HTTPStrmSetting     `yaml:"http_strm"`
}
