package router

import (
	"regexp"
	"time"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/handler"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/internal/middleware"

	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	ginR := gin.New()
	ginR.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.SetRefererPolicy(constants.SameOrigin),
		middleware.ClientFilter(),
	)

	mediaServer := handler.GetMediaServer()
	if mediaServer == nil {
		logging.Error("上游媒体服务器处理器未初始化，所有请求将返回 500")
		ginR.NoRoute(func(ctx *gin.Context) {
			ctx.AbortWithStatus(500)
		})
		return ginR
	}

	imageReg, imageTTL := cacheSpec(mediaServer.GetImageCacheRegexp(), config.Cache.ImageTTL)
	subtitleReg, subtitleTTL := cacheSpec(mediaServer.GetSubtitleCacheRegexp(), config.Cache.SubtitleTTL)

	ginR.NoRoute(
		middleware.NewCache("图片", imageReg, imageTTL, middleware.ImagePoolBytes),
		middleware.NewCache("字幕", subtitleReg, subtitleTTL, middleware.SubtitlePoolBytes),
		getRegexpRouterHandler(mediaServer),
	)

	logCacheState()
	return ginR
}

func cacheSpec(reg *regexp.Regexp, ttl time.Duration) (*regexp.Regexp, time.Duration) {
	if !config.Cache.Enable || ttl <= 0 {
		return nil, 0
	}
	return reg, ttl
}

func logCacheState() {
	if !config.Cache.Enable {
		logging.Info("全局缓存未启用")
		return
	}
	logCacheItem("图片", config.Cache.ImageTTL)
	logCacheItem("字幕", config.Cache.SubtitleTTL)
}

func logCacheItem(name string, ttl time.Duration) {
	if ttl > 0 {
		logging.Infof("%s缓存中间件已启用, TTL: %s", name, ttl)
	} else {
		logging.Infof("%s缓存中间件未启用, TTL: %s", name, ttl)
	}
}

func getRegexpRouterHandler(mediaServerHandler handler.MediaServerHandler) gin.HandlerFunc {
	middlewareChain := NewMiddlewareChain().
		Add(QueryKeyCaseInsensitive).
		Add(DisableCompression)

	return func(ctx *gin.Context) {
		for _, rule := range mediaServerHandler.GetRegexpRouteRules() {
			if rule.Regexp.MatchString(ctx.Request.URL.Path) {
				logging.AccessDebugf(ctx, "匹配成功正则表达式: %s", rule.Regexp.String())

				middlewareChain.Execute(rule.Handler)(ctx)
				return
			}
		}

		mediaServerHandler.ReverseProxy(ctx.Writer, ctx.Request)
	}
}
