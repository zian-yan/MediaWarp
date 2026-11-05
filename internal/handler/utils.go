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

func newSingleHostRewriteProxy(target *url.URL) *httputil.ReverseProxy {
	return new(httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.SetURL(target)
		},
	})
}

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

func shouldHideEmbeddedSubtitles(rw *http.Response) (bool, string) {
	if !config.HTTPStrm.HideEmbeddedSubtitles {
		return false, ""
	}
	var userAgent string
	if rw != nil && rw.Request != nil {
		userAgent = rw.Request.UserAgent()
	}
	clients := config.HTTPStrm.HideEmbeddedSubtitlesClients
	if len(clients) == 0 {
		return true, userAgent
	}
	matched := matchUserAgent(userAgent, clients)
	if !matched {
		logging.Debug("客户端不在隐藏内嵌字幕名单中，User-Agent: ", userAgent)
	}
	return matched, userAgent
}

func matchUserAgent(userAgent string, keywords []string) bool {
	lowerUserAgent := strings.ToLower(userAgent)
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			continue
		}
		if strings.Contains(lowerUserAgent, keyword) {
			return true
		}
	}
	return false
}

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
	MaxRedirectAttempts = 10
	RedirectTimeout     = 10 * time.Second
)

var (
	ErrInvalidLocationHeader = errors.New("重定向 Location 头无效")
	ErrMaxRedirectsExceeded  = fmt.Errorf("超过最大重定向次数限制（%d）", MaxRedirectAttempts)
)

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

func getFinalURL(client *http.Client, rawURL string, ua string) (string, error) {
	startTime := time.Now()
	defer func() {
		logging.Debugf("获取 %s 最终URL耗时：%s", rawURL, time.Since(startTime))
	}()

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("非法 URL： %w", err)
	}
	if parsedURL.Scheme == "" {
		return "", fmt.Errorf("URL 缺少协议头： %s", parsedURL)
	}
	if err := validateRedirectTarget(parsedURL); err != nil {
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

	for i := 0; i < MaxRedirectAttempts; i++ {
		if _, exists := visited[currentURL]; exists {
			return "", fmt.Errorf("检测到循环重定向，重定向链: %s", strings.Join(redirectChain, " -> "))
		}
		visited[currentURL] = struct{}{}
		redirectChain = append(redirectChain, currentURL)

		req, err := http.NewRequest(method, currentURL, nil)
		if err != nil {
			return "", fmt.Errorf("创建请求失败: %w", err)
		}
		req.Header.Set("User-Agent", ua)

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("发送 HTTP 请求失败：%w", err)
		}

		if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
			location, err := resp.Location()
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			if err != nil {
				return "", ErrInvalidLocationHeader
			}
			if err := validateRedirectTarget(location); err != nil {
				return "", fmt.Errorf("重定向目标未通过安全校验: %w", err)
			}
			currentURL = location.String()
			continue
		}

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
