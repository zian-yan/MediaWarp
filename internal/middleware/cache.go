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
	StatusCode int         // code 响应码
	Header     http.Header // header 响应头信息
	Body       []byte      // body 响应体
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

// cacheDataSize 计算一条缓存记录的真实占用（响应体 + 响应头 + 结构体开销）
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
	// maxCacheableBodySize 单个缓存条目的响应体上限，也决定单请求的内存缓冲上限
	//
	// 超过该大小的响应照常转发给客户端，但不写入缓存
	maxCacheableBodySize = 4 << 20 // 4 MiB

	// maxCacheBytes 单个缓存池的总字节预算
	//
	// 超出预算时按 LRU 淘汰，缓存内存因此有确定上界
	maxCacheBytes = 32 << 20 // 32 MiB
)

// 自定义的请求响应器
//
// 用于记录缓存数据
type WriterWarp struct {
	gin.ResponseWriter
	Body     []byte
	overflow bool // 响应体超过 maxCacheableBodySize，放弃缓存
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

// NewCache 创建缓存中间件
//
// reg 为 nil 或 ttl ≤ 0 时该中间件直接放行；缓存池在创建时建立一次，
// 运行期不会因配置变化重建（改 TTL 需重启进程）
func NewCache(name string, reg *regexp.Regexp, ttl time.Duration) gin.HandlerFunc {
	if reg == nil || ttl <= 0 {
		return func(ctx *gin.Context) { ctx.Next() }
	}

	pool := cache.New[*CacheData](cache.Config[*CacheData]{
		TTL:           ttl,
		MaxBytes:      maxCacheBytes,
		MaxEntryBytes: maxCacheableBodySize,
		SizeOf:        cacheDataSize,
	})
	logging.Infof("%s缓存已启用, TTL: %s, 上限: %d MiB", name, ttl, maxCacheBytes>>20)

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
	if cacheData, ok := pool.Get(cacheKey); ok {
		logging.AccessDebugf(ctx, "命中 %s 缓存: %s", cacheName, cacheKey)
		cacheData.WriteResponse(ctx)
		ctx.Abort()
		return
	}

	origin := ctx.Writer
	writer := &WriterWarp{ResponseWriter: origin}
	ctx.Writer = writer
	// 还原 ResponseWriter：gin 的 Context 会回到池中复用，
	// 留着包装器会让响应缓冲被池长期持有（每个最多 maxCacheableBodySize）
	defer func() { ctx.Writer = origin }()

	ctx.Next() // 处理请求

	if writer.overflow { // 响应体过大，不落缓存
		logging.AccessDebugf(ctx, "响应体超过 %d 字节，不进行 %s 缓存", maxCacheableBodySize, cacheName)
		return
	}

	code := writer.Status()
	if code < http.StatusOK || code >= http.StatusMultipleChoices { // 仅缓存 2xx
		logging.AccessDebugf(ctx, "响应码为: %d, 不进行 %s 缓存", code, cacheName)
		return
	}

	cacheData := &CacheData{
		StatusCode: code,
		Header:     writer.Header().Clone(),
		// 精确拷贝：缓冲切片往往有 2 倍冗余容量，直接复用会把冗余一起留在缓存里
		Body: append([]byte(nil), writer.Body...),
	}
	if !pool.Set(cacheKey, cacheData) {
		logging.AccessDebugf(ctx, "响应体超过单条上限，不进行 %s 缓存", cacheName)
		return
	}
	logging.AccessDebugf(ctx, "写入 %s 缓存成功: %s", cacheName, cacheKey)
}
