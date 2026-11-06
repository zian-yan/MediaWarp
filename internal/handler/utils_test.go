package handler

import (
	"net/url"
	"strings"
	"testing"

	"github.com/AkimioJR/MediaWarp/internal/config"
)

func boolPtr(v bool) *bool { return &v }

func TestValidateRedirectTargetLoopback(t *testing.T) {
	cases := []struct {
		name          string
		rawURL        string
		allowLoopback *bool
		wantErr       string
	}{
		{"未配置时允许 IPv4 环回", "http://127.0.0.1:5244/d/a.mkv", nil, ""},
		{"未配置时允许 127.0.0.0/8 其他地址", "http://127.8.9.10/d/a.mkv", nil, ""},
		{"未配置时允许 IPv6 环回", "http://[::1]:5244/d/a.mkv", nil, ""},
		{"显式开启时允许环回", "http://127.0.0.1/d/a.mkv", boolPtr(true), ""},
		{"显式关闭时拒绝 IPv4 环回", "http://127.0.0.1/d/a.mkv", boolPtr(false), "环回"},
		{"显式关闭时拒绝 127.0.0.0/8 其他地址", "http://127.8.9.10/d/a.mkv", boolPtr(false), "环回"},
		{"显式关闭时拒绝 IPv6 环回", "http://[::1]/d/a.mkv", boolPtr(false), "环回"},
		{"局域网地址始终允许", "http://192.168.1.10:5244/d/a.mkv", nil, ""},
		{"关闭环回不影响局域网地址", "http://192.168.1.10:5244/d/a.mkv", boolPtr(false), ""},
		{"链路本地地址始终拒绝", "http://169.254.169.254/latest/meta-data", nil, "受限网段"},
		{"未指定地址始终拒绝", "http://0.0.0.0/d/a.mkv", nil, "受限网段"},
		{"非 http 协议拒绝", "file:///etc/passwd", nil, "协议"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := config.HTTPStrm
			t.Cleanup(func() { config.HTTPStrm = old })
			config.HTTPStrm.AllowLoopback = c.allowLoopback

			u, err := url.Parse(c.rawURL)
			if err != nil {
				t.Fatalf("测试 URL 解析失败: %v", err)
			}
			err = validateRedirectTarget(u)
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
