package handler

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/internal/logging"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// newSingleHostRewrite 创建单主机反向代理的 Rewrite 函数
//
// 替代已弃用的 Director 模式，自动处理 hop-by-hop headers 和 X-Forwarded 头
func newSingleHostRewriteProxy(target *url.URL) *httputil.ReverseProxy {
	return new(httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(target)
		},
	})
}

// 响应修改创建器
//
// 将需要修改上游响应的处理器包装成一个 gin.HandlerFunc 处理器
func responseModifyCreater(rewrite func(*httputil.ProxyRequest), modifyResponseFN func(rw *http.Response) error) gin.HandlerFunc {
	funcPtr := reflect.ValueOf(modifyResponseFN).Pointer()
	funcName := strings.ReplaceAll(runtime.FuncForPC(funcPtr).Name(), "-fm", "")
	logging.Debugf("创建响应修改处理器：%s", funcName)

	proxy := httputil.ReverseProxy{
		Rewrite: rewrite,
		ModifyResponse: func(rw *http.Response) error {
			defer func() {
				if r := recover(); r != nil {
					logging.Errorf("%s 发生 panic：%s\n%s", funcName, r, string(debug.Stack()))
				}
			}()
			return modifyResponseFN(rw)
		},
	}

	return func(ctx *gin.Context) {
		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}

// 判断 Strm 文件路径是否属于 HTTPStrm
//
// 命中 http_strm.prefix_list 中任一前缀即视为 HTTPStrm
func matchHTTPStrmPath(strmFilePath string) bool {
	if !config.HTTPStrm.Enable {
		logging.Debugf("HTTPStrm 未启用，%s 不进行重定向处理", strmFilePath)
		return false
	}
	for _, prefix := range config.HTTPStrm.PrefixList {
		if strings.HasPrefix(strmFilePath, prefix) {
			logging.Debugf("%s 成功匹配路径：%s", strmFilePath, prefix)
			return true
		}
	}
	logging.Debugf("%s 未匹配任何 HTTPStrm 路径前缀", strmFilePath)
	return false
}

const (
	MaxRedirectAttempts = 10               // 最大重定向次数限制
	RedirectTimeout     = 10 * time.Second // 最大超时时间

)

var (
	ErrInvalidLocationHeader = errors.New("重定向 Location 头无效")
	ErrMaxRedirectsExceeded  = fmt.Errorf("超过最大重定向次数限制（%d）", MaxRedirectAttempts)
)

// 校验重定向目标是否允许访问（SSRF 防护）
//
// 只允许 http/https 协议；拦截环回、链路本地（含云元数据 169.254.0.0/16）、
// 组播与未指定地址。RFC1918 私网地址明确放行：HTTPStrm 的常见用途就是
// 把局域网 strm 源通过 final_url 预解析后给公网客户端播放
func validateRedirectTarget(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("不允许的协议: %s", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("URL 缺少主机名")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("解析主机名 %s 失败: %w", host, err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("目标地址 %s（%s）属于受限网段，已拦截", host, ip)
		}
	}
	return nil
}

// 获取URL的最终目标地址（自动跟踪重定向）
func getFinalURL(client *http.Client, rawURL string, ua string) (string, error) {
	startTime := time.Now()
	defer func() {
		logging.Debugf("获取 %s 最终URL耗时：%s", rawURL, time.Since(startTime))
	}()

	parsedURL, err := url.Parse(rawURL) // 验证并解析输入URL
	if err != nil {
		return "", fmt.Errorf("非法 URL： %w", err)
	}
	if parsedURL.Scheme == "" {
		return "", fmt.Errorf("URL 缺少协议头： %s", parsedURL)
	}
	if err := validateRedirectTarget(parsedURL); err != nil { // SSRF 防护：校验初始 URL
		return "", fmt.Errorf("初始 URL 未通过安全校验: %w", err)
	}

	currentURL := parsedURL.String()
	visited := make(map[string]struct{}, MaxRedirectAttempts)
	redirectChain := make([]string, 0, MaxRedirectAttempts+1)

	var method string
	if config.HTTPStrm.CompatibilityMode {
		method = http.MethodGet
	} else {
		method = http.MethodHead
	}

	// 跟踪重定向链
	for i := 0; i < MaxRedirectAttempts; i++ {
		// 检测循环重定向
		if _, exists := visited[currentURL]; exists {
			return "", fmt.Errorf("检测到循环重定向，重定向链: %s", strings.Join(redirectChain, " -> "))
		}
		visited[currentURL] = struct{}{}
		redirectChain = append(redirectChain, currentURL)

		req, err := http.NewRequest(method, currentURL, nil)
		if err != nil {
			return "", fmt.Errorf("创建请求失败: %w", err)
		}
		req.Header.Set("User-Agent", ua) // 设置 User-Agent 头部

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("发送 HTTP 请求失败：%w", err)
		}

		// 检查是否需要重定向 (3xx 状态码)
		if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
			location, err := resp.Location()
			// 排空响应体以便底层连接复用
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			if err != nil {
				return "", ErrInvalidLocationHeader
			}
			if err := validateRedirectTarget(location); err != nil { // SSRF 防护：逐跳校验
				return "", fmt.Errorf("重定向目标未通过安全校验: %w", err)
			}
			currentURL = location.String()
			continue
		}

		// 返回最终的非重定向URL
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		logging.Debug("重定向链：", strings.Join(redirectChain, " -> "))
		return resp.Request.URL.String(), nil
	}

	return "", ErrMaxRedirectsExceeded
}

var jsonChainOption = &sjson.Options{
	Optimistic:     true,
	ReplaceInPlace: true,
}
