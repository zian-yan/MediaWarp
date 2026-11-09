package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/config"
)

func newFakeStrmRemote(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/d/a.mkv", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final/a.mkv", http.StatusFound)
	})
	mux.HandleFunc("/final/a.mkv", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func setStrmTestConfig(t *testing.T, allowLoopback *bool) {
	t.Helper()
	oldStrm := config.HTTPStrm
	oldCache := config.Cache
	t.Cleanup(func() {
		config.HTTPStrm = oldStrm
		config.Cache = oldCache
	})
	config.HTTPStrm = config.HTTPStrmSetting{
		Enable:        true,
		FinalURL:      true,
		AllowLoopback: allowLoopback,
	}
	config.Cache.Enable = false
	config.Cache.HTTPStrmTTL = 0
}

func TestStrmHandlerBlocksLoopbackByDefault(t *testing.T) {
	setStrmTestConfig(t, nil)
	srv := newFakeStrmRemote(t)
	defer srv.Close()

	handler, err := getHTTPStrmHandler()
	if err != nil {
		t.Fatalf("创建 HTTPStrm 处理器失败: %v", err)
	}

	raw := srv.URL + "/d/a.mkv"
	if got := handler(context.Background(), raw, "test-agent"); got != raw {
		t.Fatalf("默认配置下应回退原 URL，实际: %s", got)
	}
}

func TestStrmHandlerResolvesFinalURLWhenLoopbackAllowed(t *testing.T) {
	setStrmTestConfig(t, boolPtrLoopback(true))
	srv := newFakeStrmRemote(t)
	defer srv.Close()

	handler, err := getHTTPStrmHandler()
	if err != nil {
		t.Fatalf("创建 HTTPStrm 处理器失败: %v", err)
	}

	got := handler(context.Background(), srv.URL+"/d/a.mkv", "test-agent")
	want := srv.URL + "/final/a.mkv"
	if got != want {
		t.Fatalf("应返回重定向后的最终 URL\n实际: %s\n期望: %s", got, want)
	}
}

func TestStrmHandlerCachesFinalURL(t *testing.T) {
	setStrmTestConfig(t, boolPtrLoopback(true))
	config.Cache.Enable = true
	config.Cache.HTTPStrmTTL = time.Minute

	srv := newFakeStrmRemote(t)
	defer srv.Close()

	handler, err := getHTTPStrmHandler()
	if err != nil {
		t.Fatalf("创建 HTTPStrm 处理器失败: %v", err)
	}

	raw := srv.URL + "/d/a.mkv"
	want := srv.URL + "/final/a.mkv"
	if got := handler(context.Background(), raw, "test-agent"); got != want {
		t.Fatalf("首次解析应返回最终 URL，实际: %s", got)
	}

	srv.Close()
	if got := handler(context.Background(), raw, "test-agent"); got != want {
		t.Fatalf("后端已下线时应命中缓存\n实际: %s\n期望: %s", got, want)
	}
}
