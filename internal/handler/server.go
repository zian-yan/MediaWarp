package handler

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
)

// 媒体服务器处理接口
type MediaServerHandler interface {
	ReverseProxy(http.ResponseWriter, *http.Request) // 转发请求至上游服务器
	GetRegexpRouteRules() []RegexpRouteRule          // 获取正则路由表
	GetImageCacheRegexp() *regexp.Regexp             // 获取图片缓存正则表达式
	GetSubtitleCacheRegexp() *regexp.Regexp          // 字幕缓存正则表达式
}

// 上游媒体服务器处理器，进程启动时构建一次，运行期不再变更
var mediaServerHandler MediaServerHandler

var ErrInvalidMediaServerType = errors.New("错误的媒体服务器类型")

// 初始化媒体服务器处理器
func Init() error {
	handler, err := build(config.MediaServer)
	if err != nil {
		return err
	}
	mediaServerHandler = handler
	return nil
}

// 获取媒体服务器处理器
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
