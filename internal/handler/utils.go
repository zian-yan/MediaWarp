package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
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
	var userAgent string
	if rw != nil && rw.Request != nil {
		userAgent = rw.Request.UserAgent()
	}
	return shouldHideEmbeddedSubtitlesFor(userAgent), userAgent
}

func shouldHideEmbeddedSubtitlesFor(userAgent string) bool {
	if !config.HTTPStrm.HideEmbeddedSubtitles {
		return false
	}
	clients := config.HTTPStrm.HideEmbeddedSubtitlesClients
	if len(clients) == 0 {
		return true
	}
	matched := matchUserAgent(userAgent, clients)
	if !matched {
		logging.Debug("客户端不在隐藏内嵌字幕名单中，User-Agent: ", userAgent)
	}
	return matched
}

func parseSubtitleStreamPath(reg *regexp.Regexp, path string) (string, int, bool) {
	matches := reg.FindStringSubmatch(path)
	if matches == nil {
		return "", 0, false
	}
	itemIdx, indexIdx := reg.SubexpIndex("item"), reg.SubexpIndex("index")
	if itemIdx <= 0 || indexIdx <= 0 || itemIdx >= len(matches) || indexIdx >= len(matches) {
		return "", 0, false
	}
	index, err := strconv.Atoi(matches[indexIdx])
	if err != nil || matches[itemIdx] == "" {
		return "", 0, false
	}
	return matches[itemIdx], index, true
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

	RedirectTotalTimeout = 30 * time.Second
	DialTimeout          = 5 * time.Second

	loopbackAllowLogInterval = 1000
)

var (
	ErrInvalidLocationHeader = errors.New("重定向 Location 头无效")
	ErrMaxRedirectsExceeded  = fmt.Errorf("超过最大重定向次数限制（%d）", MaxRedirectAttempts)
)

var loopbackAllowCount atomic.Int64

func logLoopbackAllowed(host string) {
	count := loopbackAllowCount.Add(1)
	if count == 1 || count%loopbackAllowLogInterval == 0 {
		logging.Infof("目标地址 %s 属于环回网段，已按 http_strm.allow_loopback 放行（累计 %d 次）", host, count)
	}
}

func checkTargetIP(ip net.IP, allowLoopback bool) error {
	if ip == nil {
		return errors.New("目标地址为空")
	}
	if ip.IsLoopback() {
		if !allowLoopback {
			return fmt.Errorf("目标地址 %s 属于环回网段，已被 http_strm.allow_loopback 拦截", ip)
		}
		return nil
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("目标地址 %s 属于受限网段，已拦截", ip)
	}
	return nil
}

func validateRedirectTargetWith(u *url.URL, allowLoopback bool) error {
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
	var (
		allowed    int
		allowLoop  bool
		blockedErr error
	)
	for _, ip := range ips {
		if err := checkTargetIP(ip, allowLoopback); err != nil {
			if blockedErr == nil {
				blockedErr = err
			}
			continue
		}
		if ip.IsLoopback() {
			allowLoop = true
		}
		allowed++
	}
	if allowed == 0 {
		if blockedErr != nil {
			return fmt.Errorf("主机 %s 无可用目标地址: %w", host, blockedErr)
		}
		return fmt.Errorf("主机 %s 无可用目标地址", host)
	}
	if allowLoop {
		logLoopbackAllowed(host)
	}
	return nil
}

func newSafeDialContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: DialTimeout, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("解析拨号地址 %s 失败: %w", addr, err)
		}
		allowLoopback := config.HTTPStrm.IsAllowLoopback()
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("解析主机名 %s 失败: %w", host, err)
		}
		var lastErr error
		for _, ipAddr := range addrs {
			if err := checkTargetIP(ipAddr.IP, allowLoopback); err != nil {
				lastErr = err
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			return nil, fmt.Errorf("主机 %s 无可用目标地址", host)
		}
		return nil, fmt.Errorf("连接 %s 失败: %w", host, lastErr)
	}
}

func getFinalURL(ctx context.Context, client *http.Client, rawURL string, ua string) (string, error) {
	startTime := time.Now()
	defer func() {
		logging.Debugf("获取 %s 最终URL耗时：%s", rawURL, time.Since(startTime))
	}()

	ctx, cancel := context.WithTimeout(ctx, RedirectTotalTimeout)
	defer cancel()

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("非法 URL： %w", err)
	}
	if parsedURL.Scheme == "" {
		return "", fmt.Errorf("URL 缺少协议头： %s", parsedURL)
	}
	allowLoopback := config.HTTPStrm.IsAllowLoopback()
	if err := validateRedirectTargetWith(parsedURL, allowLoopback); err != nil {
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

		req, err := http.NewRequestWithContext(ctx, method, currentURL, nil)
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
			if err := validateRedirectTargetWith(location, allowLoopback); err != nil {
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
