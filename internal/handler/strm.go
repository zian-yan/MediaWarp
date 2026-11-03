package handler

import (
	"net/http"

	"github.com/AkimioJR/MediaWarp/internal/cache"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"
)

// HTTPStrm 处理器
//
// 输入 Strm 文件内容（HTTP 链接），输出用于 302 重定向的链接
type StrmHandlerFunc func(content string, ua string) string

// 重定向结果缓存：键是 strm 内容，值是最终 URL
const (
	strmCacheMaxBytes      = 8 << 20 // 8 MiB 总预算
	strmCacheMaxEntryBytes = 8 << 10 // 单条上限 8 KiB
)

func getHTTPStrmHandler() (StrmHandlerFunc, error) {
	var urlCache *cache.Cache[string]
	if config.Cache.Enable && config.Cache.HTTPStrmTTL > 0 && config.HTTPStrm.FinalURL {
		urlCache = cache.New[string](cache.Config[string]{
			TTL:           config.Cache.HTTPStrmTTL,
			MaxBytes:      strmCacheMaxBytes,
			MaxEntryBytes: strmCacheMaxEntryBytes,
			SizeOf:        func(v string) int64 { return int64(len(v)) + 256 }, // 加上键的粗略开销
		})
		logging.Infof("启用 HTTPStrm 缓存，TTL: %s, 上限: %d MiB", config.Cache.HTTPStrmTTL, strmCacheMaxBytes>>20)
	}

	client := &http.Client{ // 创建自定义HTTP客户端配置
		Timeout: RedirectTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 禁止自动重定向，以便手动处理
			return http.ErrUseLastResponse
		},
	}

	return func(content string, ua string) string {
		if !config.HTTPStrm.FinalURL {
			logging.Debug("HTTPStrm 未启用获取最终 URL，直接使用原始 URL: ", utils.MaskSensitiveURL(content))
			return content
		}

		if urlCache != nil {
			if cachedURL, ok := urlCache.Get(content); ok {
				logging.Infof("HTTPStrm 重定向至: %s (缓存)", utils.MaskSensitiveURL(cachedURL))
				return cachedURL
			}
		}

		logging.Debug("HTTPStrm 启用获取最终 URL，开始尝试获取最终 URL")
		finalURL, err := getFinalURL(client, content, ua)
		if err != nil {
			logging.Warning("获取最终 URL 失败，使用原始 URL: ", utils.MaskSensitiveURL(err.Error()))
			return content
		}

		logging.Info("HTTPStrm 重定向至: ", utils.MaskSensitiveURL(finalURL))
		if urlCache != nil && !urlCache.Set(content, finalURL) {
			logging.Debug("HTTPStrm URL 超过单条缓存上限，跳过缓存")
		}
		return finalURL
	}, nil
}
