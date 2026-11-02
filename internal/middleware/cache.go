package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/logging"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/allegro/bigcache/v3"
	"github.com/gin-gonic/gin"
)

type CacheData struct {
	StatusCode int         // code 响应码
	Header     http.Header // header 响应头信息
	Body       []byte      // body 响应体
}

func (c *CacheData) Json() ([]byte, error) {
	return json.Marshal(c)
}

func (c *CacheData) WriteResponse(ctx *gin.Context) {
	ctx.Status(c.StatusCode)            // 设置响应码
	for key, values := range c.Header { // 设置响应头
		for _, value := range values {
			ctx.Writer.Header().Add(key, value)
		}
	}
	ctx.Writer.Write(c.Body) // 设置响应体
}

func ParseCacheData(data []byte) (*CacheData, error) {
	var cacheData CacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return nil, err
	}
	return &cacheData, nil
}

// maxCacheableBodySize 单个缓存条目的响应体上限
//
// 超过该大小的响应照常转发给客户端，但不写入缓存，
// 避免大文件把内存缓冲撑爆
const maxCacheableBodySize = 8 << 20 // 8 MiB

// 自定义的请求响应器
//
// 用于记录缓存数据
type WriterWarp struct {
	gin.ResponseWriter
	Body     bytes.Buffer
	overflow bool // 响应体超过 maxCacheableBodySize，放弃缓存
}

var _ gin.ResponseWriter = (*WriterWarp)(nil)

func (w *WriterWarp) Write(data []byte) (int, error) {
	if w.Body.Len()+len(data) <= maxCacheableBodySize {
		w.Body.Write(data)
	} else {
		w.overflow = true
	}
	return w.ResponseWriter.Write(data)
}

// WriteString 走 Write，保证缓冲内容与实际响应一致
func (w *WriterWarp) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// 计算Key时忽略的查询参数
var CacheKeyIgnoreQuery = []string{
	"api_key",
	"ApiKey", // Jellyfin 12 使用大写 ApiKey

	// Fileball
	"starttimeticks",
	"x-playback-session-id",

	// Emby
	"playsessionid",
	"tag", // 忽略 ImageTag 参数，防止 Emby 重启后缓存失效
}

// 计算Key时忽略的请求头
// var cacheKeyIgnoreHeaders = []string{
// 	"Range",
// 	"Host",
// 	"Referrer",
// 	"Connection",
// 	"Accept",
// 	"Accept-Encoding",
// 	"Accept-Language",
// 	"Cache-Control",
// 	"Upgrade-Insecure-Requests",
// 	"Referer",
// 	"Origin",

// 	// StreamMusic
// 	"X-Streammusic-Audioid",
// 	"X-Streammusic-Savepath",

// 	// IP
// 	"X-Forwarded-For",
// 	"X-Real-IP",
// 	"Forwarded",
// 	"Client-IP",
// 	"True-Client-IP",
// 	"CF-Connecting-IP",
// 	"X-Cluster-Client-IP",
// 	"Fastly-Client-IP",
// 	"X-Client-IP",
// 	"X-ProxyUser-IP",
// 	"Via",
// 	"Forwarded-For",
// 	"X-From-Cdn",
// }

func getCacheKey(ctx *gin.Context) string {
	var (
		path  string     = ctx.Request.URL.Path    // 请求路径
		query url.Values = ctx.Request.URL.Query() // 查询参数
	)

	// 将查询参数转化为字符串
	for _, key := range CacheKeyIgnoreQuery {
		query.Del(key)
	}
	for key, values := range query { // 对查询参数的值进行排序
		sort.Strings(values)
		query[key] = values
	}

	// 凭据指纹参与缓存键：不同用户互不命中，未认证请求无法窃取已缓存的授权响应
	return path + query.Encode() + "|" + credentialFingerprint(ctx)
}

// credentialFingerprint 提取请求凭据并生成短指纹
//
// 缓存键中只放指纹、不放原始凭据，避免密钥随 CacheKey 落入访问日志
func credentialFingerprint(ctx *gin.Context) string {
	query := ctx.Request.URL.Query()
	for _, key := range []string{"api_key", "ApiKey"} {
		if v := query.Get(key); v != "" {
			return utils.MD5Hash(key+"="+v)[:16]
		}
	}
	if v := ctx.GetHeader("X-Emby-Token"); v != "" {
		return utils.MD5Hash("x-emby-token="+v)[:16]
	}
	if v := ctx.GetHeader("Authorization"); v != "" {
		return utils.MD5Hash("authorization="+v)[:16]
	}
	return "anonymous"
}

// NewCache 创建缓存中间件
//
// reg 为 nil 或 ttl ≤ 0 时该中间件直接放行；缓存池在创建时建立一次，
// 运行期不会因配置变化重建（改 TTL 需重启进程）
func NewCache(name string, reg *regexp.Regexp, ttl time.Duration) gin.HandlerFunc {
	if reg == nil || ttl <= 0 {
		return func(ctx *gin.Context) { ctx.Next() }
	}

	cacheConfig := bigcache.DefaultConfig(ttl)
	cacheConfig.HardMaxCacheSize = 256 // 缓存池内存上限（MB），防止长期运行无限增长
	cachePool, err := bigcache.New(context.Background(), cacheConfig)
	if err != nil {
		logging.Warningf("创建 %s 缓存失败: %v", name, err)
		return func(ctx *gin.Context) { ctx.Next() }
	}
	logging.Infof("%s缓存已启用, TTL: %s", name, ttl)

	return func(ctx *gin.Context) {
		if ctx.Request.Method != http.MethodGet || !reg.MatchString(ctx.Request.URL.Path) {
			ctx.Next()
			return
		}
		cacheFunc(ctx, cachePool, name, reg.String())
	}
}

func cacheFunc(ctx *gin.Context, cachePool *bigcache.BigCache, cacheName string, reg string) {
	cacheKey := getCacheKey(ctx)
	logging.AccessDebugf(ctx, "命中 %s 缓存正则表达式: %s, CacheKey: %s", cacheName, reg, cacheKey)
	if cacheByte, err := cachePool.Get(cacheKey); err == nil {
		if cacheData, err := ParseCacheData(cacheByte); err == nil {
			logging.AccessDebugf(ctx, "命中 %s 缓存: %s", cacheName, cacheKey)
			cacheData.WriteResponse(ctx)
			ctx.Abort()
			return
		} else {
			logging.AccessWarningf(ctx, "解析 %s 缓存失败: %v", cacheName, err)
		}
	}

	writer := &WriterWarp{
		ResponseWriter: ctx.Writer,
		Body:           bytes.Buffer{},
	}
	ctx.Writer = writer

	ctx.Next() // 处理请求

	if writer.overflow { // 响应体过大，不落缓存
		logging.AccessDebugf(ctx, "响应体超过 %d 字节，不进行 %s 缓存", maxCacheableBodySize, cacheName)
		return
	}

	code := ctx.Writer.Status()
	if code >= http.StatusOK && code < http.StatusMultipleChoices { // 响应是2xx的成功响应，更新缓存记录
		cacheData := &CacheData{ // 创建缓存数据
			StatusCode: code, //ctx.Request.Response.StatusCode,
			Header:     ctx.Writer.Header().Clone(),
			Body:       writer.Body.Bytes(),
		}

		if cacheByte, err := cacheData.Json(); err == nil {
			err = cachePool.Set(cacheKey, cacheByte)
			if err != nil {
				logging.AccessWarningf(ctx, "写入 %s 缓存失败: %v", cacheName, err)
			} else {
				logging.AccessDebugf(ctx, "写入 %s 缓存成功", cacheName)
			}
		}
	} else {
		logging.AccessDebugf(ctx, "响应码为: %d, 不进行 %s 缓存", code, cacheName)
	}
}
