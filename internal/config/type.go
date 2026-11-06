package config

import (
	"time"

	"github.com/AkimioJR/MediaWarp/constants"
)

type VersionInfo struct {
	AppVersion string `json:"app_version"`
	CommitHash string `json:"commit_hash"`
	BuildDate  string `json:"build_date"`
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

type MediaServerSetting struct {
	Type constants.MediaServerType `yaml:"type"`
	ADDR string                    `yaml:"addr"`
	AUTH string                    `yaml:"auth"`
}

type LoggerSetting struct {
	AccessLogger  BaseLoggerSetting `yaml:"access"`
	ServiceLogger BaseLoggerSetting `yaml:"service"`
}

type BaseLoggerSetting struct {
	Console bool `yaml:"console"`
	File    bool `yaml:"file"`
}

type CacheSetting struct {
	Enable      bool          `yaml:"enable"`
	HTTPStrmTTL time.Duration `yaml:"http_strm_ttl"`
	ImageTTL    time.Duration `yaml:"image_ttl"`
	SubtitleTTL time.Duration `yaml:"subtitle_ttl"`
}

type ClientFilterSetting struct {
	Enable     bool                 `yaml:"enable"`
	Mode       constants.FliterMode `yaml:"mode"`
	ClientList []string             `yaml:"list"`
}

type HTTPStrmSetting struct {
	Enable                       bool     `yaml:"enable"`
	Proxy                        bool     `yaml:"proxy"`
	FinalURL                     bool     `yaml:"final_url"`
	AllowLoopback                *bool    `yaml:"allow_loopback"`
	CompatibilityMode            bool     `yaml:"compatibility_mode"`
	HideEmbeddedSubtitles        bool     `yaml:"hide_embedded_subtitles"`
	HideEmbeddedSubtitlesClients []string `yaml:"hide_embedded_subtitles_clients"`
	PrefixList                   []string `yaml:"prefix_list"`
}

// IsAllowLoopback 返回是否允许重定向目标指向环回地址（127.0.0.0/8、::1）。
// 未显式配置 allow_loopback 时默认允许；链路本地、组播、未指定地址不受该项影响，始终拒绝。
func (s HTTPStrmSetting) IsAllowLoopback() bool {
	return s.AllowLoopback == nil || *s.AllowLoopback
}

type Setting struct {
	Port         uint16              `yaml:"port"`
	MediaServer  MediaServerSetting  `yaml:"server"`
	Logger       LoggerSetting       `yaml:"log"`
	Cache        CacheSetting        `yaml:"cache"`
	ClientFilter ClientFilterSetting `yaml:"client"`
	HTTPStrm     HTTPStrmSetting     `yaml:"http_strm"`
}
