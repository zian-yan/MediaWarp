package handler

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
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
	return nil
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
