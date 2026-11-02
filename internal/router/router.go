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
		middleware.Logger(),                               // 访问日志
		middleware.Recovery(),                             // panic 恢复
		middleware.SetRefererPolicy(constants.SameOrigin), // Referer 策略
		middleware.ClientFilter(),                         // 客户端过滤
	)

	// 上游处理器在启动时构建完成，这里直接取用，运行期不再变更
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
		middleware.NewCache("图片", imageReg, imageTTL),
		middleware.NewCache("字幕", subtitleReg, subtitleTTL),
		getRegexpRouterHandler(mediaServer),
	)

	logCacheState()
	return ginR
}

// 计算缓存匹配规则与有效期
//
// 缓存总开关关闭或 TTL ≤ 0 时不启用该项缓存
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

// 正则表达式路由处理器
//
// 从媒体服务器处理结构体中获取正则路由规则
// 依次匹配请求, 找到对应的处理器
func getRegexpRouterHandler(mediaServerHandler handler.MediaServerHandler) gin.HandlerFunc {
	middlewareChain := NewMiddlewareChain().
		Add(QueryKeyCaseInsensitive).
		Add(DisableCompression)

	return func(ctx *gin.Context) {
		for _, rule := range mediaServerHandler.GetRegexpRouteRules() {
			if rule.Regexp.MatchString(ctx.Request.URL.Path) { // 不带查询参数的字符串：/emby/Items/54/Images/Primary
				logging.AccessDebugf(ctx, "匹配成功正则表达式: %s", rule.Regexp.String())

				middlewareChain.Execute(rule.Handler)(ctx)
				return
			}
		}

		// 未匹配路由
		mediaServerHandler.ReverseProxy(ctx.Writer, ctx.Request)
	}
}
