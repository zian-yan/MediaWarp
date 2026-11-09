package handler

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/config"
)

func boolPtrLoopback(v bool) *bool { return &v }

func TestCheckTargetIP(t *testing.T) {
	cases := []struct {
		name          string
		ip            string
		allowLoopback bool
		wantErr       string
	}{
		{"环回默认拒绝", "127.0.0.1", false, "环回"},
		{"127.0.0.0/8 其他地址默认拒绝", "127.8.9.10", false, "环回"},
		{"IPv6 环回默认拒绝", "::1", false, "环回"},
		{"IPv4-mapped 环回默认拒绝", "::ffff:127.0.0.1", false, "环回"},
		{"显式允许时放行 IPv4 环回", "127.0.0.1", true, ""},
		{"显式允许时放行 127.0.0.0/8 其他地址", "127.8.9.10", true, ""},
		{"显式允许时放行 IPv6 环回", "::1", true, ""},
		{"局域网地址始终允许", "192.168.1.10", false, ""},
		{"公网地址始终允许", "1.1.1.1", false, ""},
		{"链路本地单播始终拒绝", "169.254.169.254", true, "受限网段"},
		{"IPv6 链路本地始终拒绝", "fe80::1", true, "受限网段"},
		{"组播始终拒绝", "224.0.0.1", true, "受限网段"},
		{"IPv6 组播始终拒绝", "ff02::1", true, "受限网段"},
		{"未指定地址始终拒绝", "0.0.0.0", true, "受限网段"},
		{"IPv6 未指定地址始终拒绝", "::", true, "受限网段"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ip := net.ParseIP(c.ip)
			if ip == nil {
				t.Fatalf("测试 IP 解析失败: %s", c.ip)
			}
			err := checkTargetIP(ip, c.allowLoopback)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过校验，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错包含 %q，实际通过校验", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("错误信息应包含 %q，实际: %v", c.wantErr, err)
			}
		})
	}
}

func TestCheckTargetIPNil(t *testing.T) {
	if err := checkTargetIP(nil, true); err == nil {
		t.Fatal("空 IP 应被拒绝")
	}
}

func TestValidateRedirectTargetWith(t *testing.T) {
	cases := []struct {
		name          string
		rawURL        string
		allowLoopback bool
		wantErr       string
	}{
		{"默认拒绝 IPv4 环回", "http://127.0.0.1:5244/d/a.mkv", false, "环回"},
		{"默认拒绝 127.0.0.0/8 其他地址", "http://127.8.9.10/d/a.mkv", false, "环回"},
		{"默认拒绝 IPv6 环回", "http://[::1]:5244/d/a.mkv", false, "环回"},
		{"显式允许时放行环回", "http://127.0.0.1/d/a.mkv", true, ""},
		{"局域网地址始终允许", "http://192.168.1.10:5244/d/a.mkv", false, ""},
		{"允许环回不影响局域网地址", "http://192.168.1.10:5244/d/a.mkv", true, ""},
		{"链路本地地址始终拒绝", "http://169.254.169.254/latest/meta-data", true, "受限网段"},
		{"未指定地址始终拒绝", "http://0.0.0.0/d/a.mkv", true, "受限网段"},
		{"非 http 协议拒绝", "file:///etc/passwd", true, "协议"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.rawURL)
			if err != nil {
				t.Fatalf("测试 URL 解析失败: %v", err)
			}
			err = validateRedirectTargetWith(u, c.allowLoopback)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过校验，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错包含 %q，实际通过校验", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("错误信息应包含 %q，实际: %v", c.wantErr, err)
			}
		})
	}
}

func TestGetFinalURLBlocksLoopbackByDefault(t *testing.T) {
	old := config.HTTPStrm
	t.Cleanup(func() { config.HTTPStrm = old })

	config.HTTPStrm.AllowLoopback = nil
	_, err := getFinalURL(context.Background(), strmHTTPClient, "http://127.0.0.1:1/a.mkv", "test")
	if err == nil {
		t.Fatal("默认配置下 getFinalURL 应拒绝环回目标")
	}
	if !strings.Contains(err.Error(), "环回") {
		t.Fatalf("拒绝原因应指向环回网段，实际: %v", err)
	}
}

func TestGetFinalURLContextCancel(t *testing.T) {
	old := config.HTTPStrm
	t.Cleanup(func() { config.HTTPStrm = old })

	config.HTTPStrm.AllowLoopback = boolPtrLoopback(true)
	config.HTTPStrm.CompatibilityMode = false

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := getFinalURL(ctx, strmHTTPClient, srv.URL+"/a.mkv", "test")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("请求被取消时应返回错误")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("context 取消未生效，耗时: %s", elapsed)
	}
}

func TestSafeDialContextBlocksLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建本地监听失败: %v", err)
	}
	defer ln.Close()

	old := config.HTTPStrm
	t.Cleanup(func() { config.HTTPStrm = old })

	config.HTTPStrm.AllowLoopback = boolPtrLoopback(false)
	conn, err := newSafeDialContext()(context.Background(), "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("allow_loopback 关闭时拨号层应拒绝环回目标")
	}
	if !strings.Contains(err.Error(), "环回") {
		t.Fatalf("拨号层拒绝原因应指向环回网段，实际: %v", err)
	}
}

func TestSafeDialContextAllowsLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("创建本地监听失败: %v", err)
	}
	defer ln.Close()

	old := config.HTTPStrm
	t.Cleanup(func() { config.HTTPStrm = old })

	config.HTTPStrm.AllowLoopback = boolPtrLoopback(true)
	conn, err := newSafeDialContext()(context.Background(), "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("allow_loopback 开启时应允许环回目标，实际报错: %v", err)
	}
	conn.Close()
}
