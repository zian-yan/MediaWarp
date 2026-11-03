package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newCacheTestEngine(reg *regexp.Regexp, ttl time.Duration, body []byte, status int) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	calls := 0
	r := gin.New()
	r.NoRoute(NewCache("测试", reg, ttl, ImagePoolBytes), func(ctx *gin.Context) {
		calls++
		ctx.Data(status, "image/jpeg", body)
	})
	return r, &calls
}

func serve(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("X-Emby-Token", token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCacheMiddlewareHitsOnSecondRequest(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 1<<10)
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, time.Minute, body, http.StatusOK)

	first := serve(r, "/Items/1/Images/Primary", "tok-a")
	second := serve(r, "/Items/1/Images/Primary", "tok-a")

	if *calls != 1 {
		t.Errorf("回源次数 = %d，期望 1（第二次应命中缓存）", *calls)
	}
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Errorf("状态码 = %d/%d，期望 200/200", first.Code, second.Code)
	}
	if !bytes.Equal(first.Body.Bytes(), body) || !bytes.Equal(second.Body.Bytes(), body) {
		t.Error("缓存响应体与原响应不一致")
	}
	if got := second.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q，期望缓存回放的 image/jpeg", got)
	}
}

func TestCacheMiddlewareIsolatesCredentials(t *testing.T) {
	body := []byte("img")
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, time.Minute, body, http.StatusOK)

	serve(r, "/Items/1/Images/Primary", "tok-a")
	serve(r, "/Items/1/Images/Primary", "tok-b")

	if *calls != 2 {
		t.Errorf("回源次数 = %d，期望 2（不同凭据不应互相命中）", *calls)
	}
}

func TestCacheMiddlewareSkipsNon2xx(t *testing.T) {
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, time.Minute, []byte("nope"), http.StatusNotFound)

	serve(r, "/Items/1/Images/Primary", "tok")
	serve(r, "/Items/1/Images/Primary", "tok")

	if *calls != 2 {
		t.Errorf("回源次数 = %d，期望 2（404 不缓存）", *calls)
	}
}

func TestCacheMiddlewareSkipsOversizeBody(t *testing.T) {
	body := bytes.Repeat([]byte("y"), maxCacheableBodySize+1)
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, time.Minute, body, http.StatusOK)

	first := serve(r, "/Items/1/Images/Primary", "tok")
	second := serve(r, "/Items/1/Images/Primary", "tok")

	if *calls != 2 {
		t.Errorf("回源次数 = %d，期望 2（超限响应不缓存）", *calls)
	}
	if first.Body.Len() != len(body) || second.Body.Len() != len(body) {
		t.Errorf("响应体长度 = %d/%d，期望 %d（超限也要完整转发）",
			first.Body.Len(), second.Body.Len(), len(body))
	}
}

func TestCacheMiddlewareMarksHitAndMiss(t *testing.T) {
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, _ := newCacheTestEngine(reg, time.Minute, []byte("img"), http.StatusOK)

	first := serve(r, "/Items/1/Images/Primary", "tok")
	second := serve(r, "/Items/1/Images/Primary", "tok")
	third := serve(r, "/Items/1/Images/Primary", "tok")

	if got := first.Header().Get(cacheHeader); got != "MISS" {
		t.Errorf("首次响应 %s = %q，期望 MISS", cacheHeader, got)
	}
	if got := second.Header().Get(cacheHeader); got != "HIT" {
		t.Errorf("第二次响应 %s = %q，期望 HIT", cacheHeader, got)
	}
	if got := third.Header().Get(cacheHeader); got != "HIT" {
		t.Errorf("第三次响应 %s = %q，期望 HIT（MISS 不得随缓存回放）", cacheHeader, got)
	}
}

func TestCacheMiddlewarePassThrough(t *testing.T) {
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, time.Minute, []byte("ok"), http.StatusOK)

	serve(r, "/Videos/1/stream", "tok")
	serve(r, "/Videos/1/stream", "tok")

	if *calls != 2 {
		t.Errorf("回源次数 = %d，期望 2（非缓存路径不应拦截）", *calls)
	}
}

func TestNewCacheDisabled(t *testing.T) {
	reg := regexp.MustCompile(`^/Items/\d+/Images/Primary$`)
	r, calls := newCacheTestEngine(reg, 0, []byte("ok"), http.StatusOK)

	serve(r, "/Items/1/Images/Primary", "tok")
	serve(r, "/Items/1/Images/Primary", "tok")

	if *calls != 2 {
		t.Errorf("回源次数 = %d，期望 2（TTL 为 0 时不缓存）", *calls)
	}
}
