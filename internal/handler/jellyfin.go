package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/internal/service/jellyfin"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/gin-gonic/gin"
)

type JellyfinHandler struct {
	client          *jellyfin.Client
	routerRules     []RegexpRouteRule
	proxy           *httputil.ReverseProxy
	httpStrmHandler StrmHandlerFunc
}

func NewJellyfinHandler(addr string, apiKey string) (*JellyfinHandler, error) {
	handler := JellyfinHandler{}
	client, err := jellyfin.New(addr, apiKey)
	if err != nil {
		return nil, fmt.Errorf("创建 Jellyfin 客户端实例失败: %w", err)
	}
	handler.client = client
	target, err := url.Parse(handler.client.GetBaseURLString())
	if err != nil {
		return nil, err
	}
	handler.proxy = newSingleHostRewriteProxy(target)

	{
		handler.routerRules = []RegexpRouteRule{
			{
				Regexp: constants.JellyfinRegexp.Router.ModifyPlaybackInfo,
				Handler: responseModifyCreater(
					handler.proxy.Rewrite,
					handler.ModifyPlaybackInfo,
				),
			},
			{
				Regexp:  constants.JellyfinRegexp.Router.VideosHandler,
				Handler: handler.VideosHandler,
			},
		}
	}

	handler.httpStrmHandler, err = getHTTPStrmHandler()
	if err != nil {
		return nil, fmt.Errorf("创建 HTTPStrm 处理器失败: %w", err)
	}
	return &handler, nil
}

func (handler *JellyfinHandler) ReverseProxy(rw http.ResponseWriter, req *http.Request) {
	handler.proxy.ServeHTTP(rw, req)
}

func (handler *JellyfinHandler) GetRegexpRouteRules() []RegexpRouteRule {
	return handler.routerRules
}

func (handler *JellyfinHandler) GetImageCacheRegexp() *regexp.Regexp {
	return constants.JellyfinRegexp.Cache.Image
}

func (*JellyfinHandler) GetSubtitleCacheRegexp() *regexp.Regexp {
	return constants.JellyfinRegexp.Cache.Subtitle
}

func (handler *JellyfinHandler) ModifyPlaybackInfo(rw *http.Response) error {
	startTime := time.Now()
	defer func() {
		logging.Debugf("处理 ModifyPlaybackInfo 耗时：%s", time.Since(startTime))
	}()

	defer rw.Body.Close()
	data, err := io.ReadAll(rw.Body)
	if err != nil {
		logging.Warning("读取响应体失败：", err)
		return err
	}

	jsonChain := utils.NewJsonChainFromBytesWithCopy(data, jsonChainOption)

	var playbackInfoResponse jellyfin.PlaybackInfoResponse
	if err = json.Unmarshal(data, &playbackInfoResponse); err != nil {
		logging.Warning("解析 jellyfin.PlaybackInfoResponse JSON 错误：", err)
		return err
	}

	for index, mediasource := range playbackInfoResponse.MediaSources {
		if mediasource.ID == nil {
			logging.Debugf("跳过 ID 为空的 MediaSource, index: %d", index)
			continue
		}
		startTime := time.Now()
		logging.Debug("请求 ItemsServiceQueryItem：" + *mediasource.ID)
		itemResponse, err := handler.client.ItemsServiceQueryItem(*mediasource.ID, 1, "Path,MediaSources")
		if err != nil {
			logging.Warning("请求 ItemsServiceQueryItem 失败：", err)
			continue
		}
		if len(itemResponse.Items) == 0 {
			logging.Warningf("ItemsServiceQueryItem 返回空结果，MediaSource ID: %s", *mediasource.ID)
			continue
		}
		item := itemResponse.Items[0]
		if item.Path == nil {
			logging.Warningf("Item 的 Path 为空，MediaSource ID: %s", *mediasource.ID)
			continue
		}
		if !matchHTTPStrmPath(*item.Path) {
			continue
		}

		bsePath := "MediaSources." + strconv.Itoa(index) + "."
		if hide, userAgent := shouldHideEmbeddedSubtitles(rw); hide {
			drop := make([]bool, len(mediasource.MediaStreams))
			for i, stream := range mediasource.MediaStreams {
				drop[i] = stream.Type != nil && *stream.Type == jellyfin.Subtitle &&
					(stream.IsExternal == nil || !*stream.IsExternal) &&
					(stream.DeliveryMethod == nil || *stream.DeliveryMethod != jellyfin.External)
			}
			if removed := removeSubtitleStreams(jsonChain, bsePath, drop); removed > 0 {
				logging.Infof("媒体 %s 隐藏 %d 条内嵌字幕流（客户端: %s），避免服务端提取字幕时下载完整文件",
					*mediasource.ID, removed, userAgent)
			}
		}
		processHTTPStrmPlaybackInfo(
			jsonChain,
			bsePath,
			*mediasource.ID,
			*mediasource.ID,
			mediasource.DirectStreamURL,
		)

		logging.Debugf("处理 %s 的 MediaSource %s 耗时：%s", *item.Path, *mediasource.ID, time.Since(startTime))
	}

	data, err = jsonChain.Result()
	if err != nil {
		logging.Warning("操作 jellyfin.PlaybackInfoResponse Json 错误：", err)
		return err
	}

	rw.Header.Set("Content-Type", "application/json")
	rw.Header.Set("Content-Length", strconv.Itoa(len(data)))
	rw.Body = io.NopCloser(bytes.NewReader(data))
	return nil
}

func (handler *JellyfinHandler) VideosHandler(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodHead {
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		logging.Debug("VideosHandler 不处理 HEAD 请求，转发至上游服务器")
		return
	}

	mediaSourceID := ctx.Query("mediasourceid")
	logging.Debugf("请求 ItemsServiceQueryItem：%s", mediaSourceID)
	itemResponse, err := handler.client.ItemsServiceQueryItem(mediaSourceID, 1, "Path,MediaSources")
	if err != nil {
		logging.Warning("请求 ItemsServiceQueryItem 失败：", err)
		handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
		return
	}

	if len(itemResponse.Items) == 0 {
		logging.Warningf("ItemsServiceQueryItem 返回空结果，mediaSourceID: %s", mediaSourceID)
		handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
		return
	}

	item := itemResponse.Items[0]

	if item.Path == nil || !strings.HasSuffix(strings.ToLower(*item.Path), ".strm") {
		logging.Debugf("播放本地视频，不进行处理")
		handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
		return
	}

	if !matchHTTPStrmPath(*item.Path) {
		handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
		return
	}

	for _, mediasource := range item.MediaSources {
		if mediasource.ID == nil {
			continue
		}
		if *mediasource.ID != mediaSourceID {
			continue
		}
		if mediasource.Path == nil {
			logging.Warningf("MediaSource (ID: %s) 的 Path 为空", *mediasource.ID)
			break
		}
		if mediasource.Protocol != nil && *mediasource.Protocol == jellyfin.HTTP {
			target := handler.httpStrmHandler(*mediasource.Path, ctx.Request.UserAgent())
			if !utils.IsSafeRedirectURL(target) {
				logging.Warningf("HTTPStrm 重定向目标协议不合法，转回上游：%s", utils.MaskSensitiveURL(target))
				handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
				return
			}
			ctx.Redirect(http.StatusFound, target)
			return
		}
		logging.Debugf("MediaSource (ID: %s) 不是 HTTP 协议，不进行重定向", *mediasource.ID)
		break
	}

	handler.proxy.ServeHTTP(ctx.Writer, ctx.Request)
}

var _ MediaServerHandler = (*JellyfinHandler)(nil)
