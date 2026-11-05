package handler

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"
)

type MediaServerHandler interface {
	ReverseProxy(http.ResponseWriter, *http.Request)
	GetRegexpRouteRules() []RegexpRouteRule
	GetImageCacheRegexp() *regexp.Regexp
	GetSubtitleCacheRegexp() *regexp.Regexp
}

var mediaServerHandler MediaServerHandler

var ErrInvalidMediaServerType = errors.New("错误的媒体服务器类型")

func Init() error {
	handler, err := build(config.MediaServer)
	if err != nil {
		return err
	}
	mediaServerHandler = handler
	logSubtitleFilterState()
	return nil
}

func logSubtitleFilterState() {
	if !config.HTTPStrm.HideEmbeddedSubtitles {
		return
	}
	if !config.HTTPStrm.Enable {
		logging.Warning("隐藏内嵌字幕流已开启，但 http_strm.enable 为 false，该功能不会生效")
		return
	}
	clients := make([]string, 0, len(config.HTTPStrm.HideEmbeddedSubtitlesClients))
	for _, client := range config.HTTPStrm.HideEmbeddedSubtitlesClients {
		client = strings.TrimSpace(client)
		if client == "" {
			logging.Warning("http_strm.hide_embedded_subtitles_clients 含空白项，已忽略")
			continue
		}
		clients = append(clients, client)
	}
	if len(clients) == 0 {
		logging.Info("隐藏内嵌字幕流已启用：对所有客户端生效（PlaybackInfo 隐藏 + 字幕请求拦截）")
		return
	}
	logging.Infof("隐藏内嵌字幕流已启用：仅对客户端 %s 生效（PlaybackInfo 隐藏 + 字幕请求拦截，User-Agent 包含匹配，不区分大小写）",
		strings.Join(clients, ", "))
}

func GetMediaServer() MediaServerHandler {
	return mediaServerHandler
}

func build(setting config.MediaServerSetting) (MediaServerHandler, error) {
	switch setting.Type {
	case constants.EMBY:
		return NewEmbyServerHandler(setting.ADDR, setting.AUTH)
	case constants.JELLYFIN:
		return NewJellyfinHandler(setting.ADDR, setting.AUTH)
	default:
		return nil, ErrInvalidMediaServerType
	}
}
