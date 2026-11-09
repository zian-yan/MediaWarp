package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/cache"
	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"
)

type StrmHandlerFunc func(ctx context.Context, content string, ua string) string

const (
	strmCacheMaxBytes      = 16 << 20
	strmCacheMaxEntryBytes = 8 << 10

	strmMaxIdleConns        = 100
	strmMaxIdleConnsPerHost = 10
	strmIdleConnTimeout     = 90 * time.Second
)

var (
	strmCacheMu  sync.Mutex
	strmURLCache *cache.Cache[string]

	strmHTTPClient = &http.Client{
		Timeout: RedirectTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy:               nil,
			MaxIdleConns:        strmMaxIdleConns,
			MaxIdleConnsPerHost: strmMaxIdleConnsPerHost,
			IdleConnTimeout:     strmIdleConnTimeout,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: DialTimeout,
			DialContext:         newSafeDialContext(),
		},
	}
)

func getHTTPStrmHandler() (StrmHandlerFunc, error) {
	strmCacheMu.Lock()
	if strmURLCache != nil {
		strmURLCache.Close()
		strmURLCache = nil
	}

	var urlCache *cache.Cache[string]
	if config.Cache.Enable && config.Cache.HTTPStrmTTL > 0 && config.HTTPStrm.FinalURL {
		urlCache = cache.New[string](cache.Config[string]{
			TTL:           config.Cache.HTTPStrmTTL,
			MaxBytes:      strmCacheMaxBytes,
			MaxEntryBytes: strmCacheMaxEntryBytes,
			SizeOf:        func(v string) int64 { return int64(len(v)) + 256 },
			SweepInterval: time.Minute,
			ReleaseOS:     true,
		})
		logging.Infof("启用 HTTPStrm 缓存，TTL: %s, 上限: %d MiB", config.Cache.HTTPStrmTTL, strmCacheMaxBytes>>20)
	}
	strmURLCache = urlCache
	strmCacheMu.Unlock()

	return func(ctx context.Context, content string, ua string) string {
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
		finalURL, err := getFinalURL(ctx, strmHTTPClient, content, ua)
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
