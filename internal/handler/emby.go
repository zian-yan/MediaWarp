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
	"github.com/AkimioJR/MediaWarp/internal/service/emby"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/gin-gonic/gin"
)

type EmbyHandler struct {
	client          *emby.Client
	routerRules     []RegexpRouteRule
	proxy           *httputil.ReverseProxy
	httpStrmHandler StrmHandlerFunc
}

func NewEmbyServerHandler(addr string, apiKey string) (*EmbyHandler, error) {
	var handler = EmbyHandler{}
	client, err := emby.New(addr, apiKey)
	if err != nil {
		return nil, fmt.Errorf("创建 Emby 客户端实例失败: %w", err)
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
				Regexp:  constants.EmbyRegexp.Router.VideosHandler,
				Handler: handler.VideosHandler,
			},
			{
				Regexp: constants.EmbyRegexp.Router.ModifyPlaybackInfo,
				Handler: responseModifyCreater(
					handler.proxy.Rewrite,
					handler.ModifyPlaybackInfo,
				),
			},
			{
				Regexp: constants.EmbyRegexp.Router.ModifyBaseHtmlPlayer,
				Handler: responseModifyCreater(
					handler.proxy.Rewrite,
					handler.ModifyBaseHtmlPlayer,
				),
			},
		}
	}
	handler.httpStrmHandler, err = getHTTPStrmHandler()
	if err != nil {
		return nil, fmt.Errorf("创建 HTTPStrm 处理器失败: %w", err)
	}
	return &handler, nil
}

func (handler *EmbyHandler) ReverseProxy(rw http.ResponseWriter, req *http.Request) {
	handler.proxy.ServeHTTP(rw, req)
}

func (handler *EmbyHandler) GetRegexpRouteRules() []RegexpRouteRule {
	return handler.routerRules
}

func (handler *EmbyHandler) GetImageCacheRegexp() *regexp.Regexp {
	return constants.EmbyRegexp.Cache.Image
}

func (handler *EmbyHandler) GetSubtitleCacheRegexp() *regexp.Regexp {
	return constants.EmbyRegexp.Cache.Subtitle
}

func (handler *EmbyHandler) ModifyPlaybackInfo(rw *http.Response) error {
	startTime := time.Now()
	defer func() {
		logging.Debugf("处理 ModifyPlaybackInfo 耗时：%s", time.Since(startTime))
	}()

	defer rw.Body.Close()
	body, err := io.ReadAll(rw.Body)
	if err != nil {
		logging.Warning("读取 Body 出错：", err)
		return err
	}

	jsonChain := utils.NewJsonChainFromBytesWithCopy(body, jsonChainOption)

	var playbackInfoResponse emby.PlaybackInfoResponse
	if err = json.Unmarshal(body, &playbackInfoResponse); err != nil {
		logging.Warning("解析 emby.PlaybackInfoResponse Json 错误：", err)
		return err
	}

	for index, mediasource := range playbackInfoResponse.MediaSources {
		startTime := time.Now()

		if mediasource.ID == nil {
			logging.Debug("跳过缺少 Id 的 MediaSource")
			continue
		}

		if mediasource.IsInfiniteStream != nil && *mediasource.IsInfiniteStream {
			logging.Debug("跳过直播 MediaSource：" + *mediasource.ID)
			continue
		}
		if mediasource.RequiresOpening != nil && *mediasource.RequiresOpening {
			logging.Debug("跳过需要打开直播流的 MediaSource：" + *mediasource.ID)
			continue
		}
		if mediasource.ItemID == nil {
			logging.Debug("跳过缺少 ItemId 的 MediaSource：" + *mediasource.ID)
			continue
		}

		logging.Debug("请求 ItemsServiceQueryItem：" + *mediasource.ItemID)
		itemResponse, err := handler.client.ItemsServiceQueryItem(*mediasource.ItemID, 1, "Path,MediaSources")
		if err != nil {
			logging.Warning("请求 ItemsServiceQueryItem 失败：", err)
			continue
		}
		if itemResponse == nil || len(itemResponse.Items) == 0 {
			logging.Warning("请求 ItemsServiceQueryItem 未返回 Item：", *mediasource.ItemID)
			continue
		}

		bsePath := "MediaSources." + strconv.Itoa(index) + "."
		item := itemResponse.Items[0]
		if item.Path == nil {
			logging.Debug("跳过缺少 Path 的 Item：" + *mediasource.ItemID)
			continue
		}

		if !matchHTTPStrmPath(*item.Path) {
			continue
		}

		processHTTPStrmPlaybackInfo(
			jsonChain,
			bsePath,
			*mediasource.ItemID,
			*mediasource.ID,
			mediasource.DirectStreamURL,
		)

		logging.Debugf("处理 %s 的 MediaSource %s 耗时：%s", *item.Path, *mediasource.ID, time.Since(startTime))
	}

	body, err = jsonChain.Result()
	if err != nil {
		logging.Warning("操作 emby.PlaybackInfoResponse Json 错误：", err)
		return err
	}

	rw.Header.Set("Content-Type", "application/json")
	rw.Header.Set("Content-Length", strconv.Itoa(len(body)))
	rw.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func (handler *EmbyHandler) VideosHandler(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodHead {
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		logging.Debug("VideosHandler 不处理 HEAD 请求，转发至上游服务器")
		return
	}

	orginalPath := ctx.Request.URL.Path
	matches := constants.EmbyRegexp.Others.VideoRedirectReg.FindStringSubmatch(orginalPath)
	if len(matches) == 2 {
		redirectPath := fmt.Sprintf("/videos/%s/stream", matches[0])
		logging.Debugf("%s 重定向至：%s", orginalPath, redirectPath)
		ctx.Redirect(http.StatusFound, redirectPath)
		return
	}

	mediaSourceID := ctx.Query("mediasourceid")

	logging.Debugf("请求 ItemsServiceQueryItem：%s", mediaSourceID)
	mediaSourceID_without_prefix := strings.Replace(mediaSourceID, "mediasource_", "", 1)
	itemResponse, err := handler.client.ItemsServiceQueryItem(mediaSourceID_without_prefix, 1, "Path,MediaSources")
	if err != nil {
		logging.Warning("请求 ItemsServiceQueryItem 失败：", err)
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		return
	}

	if len(itemResponse.Items) == 0 {
		logging.Warningf("ItemsServiceQueryItem 返回空结果，mediaSourceID: %s", mediaSourceID)
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		return
	}

	item := itemResponse.Items[0]

	if item.Path == nil || !strings.HasSuffix(strings.ToLower(*item.Path), ".strm") {
		logging.Debug("播放本地视频，不进行处理")
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		return
	}

	if !matchHTTPStrmPath(*item.Path) {
		handler.ReverseProxy(ctx.Writer, ctx.Request)
		return
	}

	for _, mediasource := range item.MediaSources {
		if mediasource.ID == nil {
			logging.Debugf("跳过 ID 为空的 MediaSource, mediaSourceID: %s", mediaSourceID)
			continue
		}
		logging.Debugf("mediasource.ID: %s ; mediaSourceID: %s ; mediaSourceID_without_prefix: %s", *mediasource.ID, mediaSourceID, mediaSourceID_without_prefix)
		if strings.Replace(*mediasource.ID, "mediasource_", "", 1) != mediaSourceID_without_prefix {
			continue
		}
		if mediasource.Path == nil {
			logging.Warningf("MediaSource (ID: %s) 的 Path 为空", *mediasource.ID)
			break
		}
		if mediasource.Protocol != nil && *mediasource.Protocol == emby.HTTP {
			target := handler.httpStrmHandler(*mediasource.Path, ctx.Request.UserAgent())
			if !utils.IsSafeRedirectURL(target) {
				logging.Warningf("HTTPStrm 重定向目标协议不合法，转回上游：%s", utils.MaskSensitiveURL(target))
				handler.ReverseProxy(ctx.Writer, ctx.Request)
				return
			}
			ctx.Redirect(http.StatusFound, target)
			return
		}
		logging.Debugf("MediaSource (ID: %s) 不是 HTTP 协议，不进行重定向", *mediasource.ID)
		break
	}

	handler.ReverseProxy(ctx.Writer, ctx.Request)
}

func (handler *EmbyHandler) ModifyBaseHtmlPlayer(rw *http.Response) error {
	defer rw.Body.Close()
	body, err := io.ReadAll(rw.Body)
	if err != nil {
		return err
	}

	body = bytes.ReplaceAll(body, []byte(`mediaSource.IsRemote&&"DirectPlay"===playMethod?null:"anonymous"`), []byte("null"))
	rw.Header.Set("Content-Length", strconv.Itoa(len(body)))
	rw.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

var _ MediaServerHandler = (*EmbyHandler)(nil)
