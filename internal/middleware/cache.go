package middleware

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/cache"
	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/gin-gonic/gin"
)

type CacheData struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func (c *CacheData) WriteResponse(ctx *gin.Context) {
	ctx.Status(c.StatusCode)
	for key, values := range c.Header {
		for _, value := range values {
			ctx.Writer.Header().Add(key, value)
		}
	}
	ctx.Writer.Write(c.Body)
}

func cacheDataSize(c *CacheData) int64 {
	if c == nil {
		return 0
	}
	size := int64(len(c.Body)) + 96
	for key, values := range c.Header {
		size += int64(len(key)) + 16
		for _, value := range values {
			size += int64(len(value)) + 16
		}
	}
	return size
}

const (
	maxCacheableBodySize = 4 << 20

	ImagePoolBytes = 64 << 20

	SubtitlePoolBytes = 32 << 20

	cacheHeader = "X-Cache"

	statsLogInterval = 500

	cacheSweepInterval = time.Minute
)

type WriterWarp struct {
	gin.ResponseWriter
	Body     []byte
	overflow bool
}

var _ gin.ResponseWriter = (*WriterWarp)(nil)

func (w *WriterWarp) Write(data []byte) (int, error) {
	if len(w.Body)+len(data) <= maxCacheableBodySize {
		w.Body = append(w.Body, data...)
	} else {
		w.overflow = true
	}
	return w.ResponseWriter.Write(data)
}

func (w *WriterWarp) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

var CacheKeyIgnoreQuery = []string{
	"api_key",
	"ApiKey",

	"starttimeticks",
	"x-playback-session-id",

	"playsessionid",
	"tag",
}

func getCacheKey(ctx *gin.Context) string {
	var (
		path  string     = ctx.Request.URL.Path
		query url.Values = ctx.Request.URL.Query()
	)

	for _, key := range CacheKeyIgnoreQuery {
		query.Del(key)
	}
	for key, values := range query {
		sort.Strings(values)
		query[key] = values
	}

	return path + query.Encode() + "|" + credentialFingerprint(ctx)
}

func credentialFingerprint(ctx *gin.Context) string {
	query := ctx.Request.URL.Query()
	for _, key := range []string{"api_key", "ApiKey"} {
		if v := query.Get(key); v != "" {
			return utils.MD5Hash(key + "=" + v)[:16]
		}
	}
	if v := ctx.GetHeader("X-Emby-Token"); v != "" {
		return utils.MD5Hash("x-emby-token=" + v)[:16]
	}
	if v := ctx.GetHeader("Authorization"); v != "" {
		return utils.MD5Hash("authorization=" + v)[:16]
	}
	return "anonymous"
}

func NewCache(name string, reg *regexp.Regexp, ttl time.Duration, poolBytes int64) gin.HandlerFunc {
	if reg == nil || ttl <= 0 {
		return func(ctx *gin.Context) { ctx.Next() }
	}
	if poolBytes <= 0 {
		poolBytes = maxCacheableBodySize
	}

	pool := cache.New[*CacheData](cache.Config[*CacheData]{
		TTL:           ttl,
		MaxBytes:      poolBytes,
		MaxEntryBytes: maxCacheableBodySize,
		SizeOf:        cacheDataSize,
		SweepInterval: cacheSweepInterval,
		ReleaseOS:     true,
	})
	logging.Infof("%s缓存已启用, TTL: %s, 上限: %d MiB", name, ttl, poolBytes>>20)

	return func(ctx *gin.Context) {
		if ctx.Request.Method != http.MethodGet || !reg.MatchString(ctx.Request.URL.Path) {
			ctx.Next()
			return
		}
		cacheFunc(ctx, pool, name, reg.String())
	}
}

func cacheFunc(ctx *gin.Context, pool *cache.Cache[*CacheData], cacheName string, reg string) {
	cacheKey := getCacheKey(ctx)
	logging.AccessDebugf(ctx, "命中 %s 缓存正则表达式: %s, CacheKey: %s", cacheName, reg, cacheKey)

	cacheData, ok := pool.Get(cacheKey)
	logPoolStats(pool, cacheName)

	if ok {
		logging.AccessDebugf(ctx, "命中 %s 缓存: %s", cacheName, cacheKey)
		ctx.Writer.Header().Set(cacheHeader, "HIT")
		cacheData.WriteResponse(ctx)
		ctx.Abort()
		return
	}

	ctx.Writer.Header().Set(cacheHeader, "MISS")

	origin := ctx.Writer
	writer := &WriterWarp{ResponseWriter: origin}
	ctx.Writer = writer
	defer func() { ctx.Writer = origin }()

	ctx.Next()

	if writer.overflow {
		logging.AccessDebugf(ctx, "响应体超过 %d 字节，不进行 %s 缓存", maxCacheableBodySize, cacheName)
		return
	}

	code := writer.Status()
	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		logging.AccessDebugf(ctx, "响应码为: %d, 不进行 %s 缓存", code, cacheName)
		return
	}

	cacheData = &CacheData{
		StatusCode: code,
		Header:     writer.Header().Clone(),
		Body:       append([]byte(nil), writer.Body...),
	}
	cacheData.Header.Del(cacheHeader)

	if !pool.Set(cacheKey, cacheData) {
		logging.AccessDebugf(ctx, "响应体超过单条上限，不进行 %s 缓存", cacheName)
		return
	}
	logging.AccessDebugf(ctx, "写入 %s 缓存成功: %s", cacheName, cacheKey)
}

func logPoolStats(pool *cache.Cache[*CacheData], cacheName string) {
	s := pool.Stats()
	total := s.Hits + s.Misses
	if total == 0 || total%statsLogInterval != 0 {
		return
	}
	rate := float64(s.Hits) / float64(total) * 100
	logging.Infof("%s缓存: 查询 %d 次，命中 %d，命中率 %.1f%%，当前 %d 条 / %.1f MiB",
		cacheName, total, s.Hits, rate, s.Entries, float64(s.Bytes)/(1<<20))
}
