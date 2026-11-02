package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/allegro/bigcache/v3"
)

// HTTPStrm 处理器
//
// 输入 Strm 文件内容（HTTP 链接），输出用于 302 重定向的链接
type StrmHandlerFunc func(content string, ua string) string

func getHTTPStrmHandler() (StrmHandlerFunc, error) {
	var cache *bigcache.BigCache
	if config.Cache.Enable && config.Cache.HTTPStrmTTL > 0 && config.HTTPStrm.FinalURL {
		cacheConfig := bigcache.DefaultConfig(config.Cache.HTTPStrmTTL)
		cacheConfig.HardMaxCacheSize = 64 // 重定向结果缓存内存上限（MB）
		var err error
		cache, err = bigcache.New(context.Background(), cacheConfig)
		if err != nil {
			return nil, fmt.Errorf("创建 HTTPStrm 缓存失败: %w", err)
		}
		logging.Info("启用 HTTPStrm 缓存，TTL: ", config.Cache.HTTPStrmTTL)
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

		if cache != nil {
			if cachedURL, err := cache.Get(content); err == nil {
				logging.Infof("HTTPStrm 重定向至: %s (缓存)", utils.MaskSensitiveURL(string(cachedURL)))
				return string(cachedURL)
			}
		}

		logging.Debug("HTTPStrm 启用获取最终 URL，开始尝试获取最终 URL")
		finalURL, err := getFinalURL(client, content, ua)
		if err != nil {
			logging.Warning("获取最终 URL 失败，使用原始 URL: ", utils.MaskSensitiveURL(err.Error()))
			return content
		}

		logging.Info("HTTPStrm 重定向至: ", utils.MaskSensitiveURL(finalURL))
		if cache != nil {
			if err := cache.Set(content, []byte(finalURL)); err != nil {
				logging.Warning("缓存 HTTPStrm URL 失败: ", err)
			} else {
				logging.Debug("缓存 HTTPStrm URL 成功")
			}
		}
		return finalURL
	}, nil
}
