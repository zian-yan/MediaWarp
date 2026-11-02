package utils_test

import (
	"testing"

	"github.com/AkimioJR/MediaWarp/utils"
)

// 日志脱敏：敏感查询参数的值必须被替换，其他参数保持原样
func TestMaskSensitiveQuery(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"小写 api_key":   {"a=1&api_key=secret", "a=1&api_key=***"},
		"大写 ApiKey":    {"ApiKey=secret&x=2", "ApiKey=***&x=2"},
		"混合大小写":        {"API_KEY=secret", "API_KEY=***"},
		"X-Emby-Token": {"X-Emby-Token=secret", "X-Emby-Token=***"},
		"无敏感参数":        {"tag=abc&width=300", "tag=abc&width=300"},
		"空查询串":         {"", ""},
		"只有键没有值":       {"api_key", "api_key"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := utils.MaskSensitiveQuery(c.in); got != c.want {
				t.Errorf("MaskSensitiveQuery(%q) = %q, 期望 %q", c.in, got, c.want)
			}
		})
	}
}

// 完整 URL 脱敏：只处理查询串，路径与 fragment 保持不变
func TestMaskSensitiveURL(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"带 api_key": {
			"http://emby:8096/Items/1/Images/Primary?api_key=secret&tag=9",
			"http://emby:8096/Items/1/Images/Primary?api_key=***&tag=9",
		},
		"无查询串": {
			"http://emby:8096/Items/1/Images/Primary",
			"http://emby:8096/Items/1/Images/Primary",
		},
		"保留 fragment": {
			"http://host/p?X-Emby-Token=secret#frag",
			"http://host/p?X-Emby-Token=***#frag",
		},
		"非标准 URL 也能处理": {
			"/Videos/1/stream?MediaSourceId=1&api_key=secret",
			"/Videos/1/stream?MediaSourceId=1&api_key=***",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := utils.MaskSensitiveURL(c.in); got != c.want {
				t.Errorf("MaskSensitiveURL(%q) = %q, 期望 %q", c.in, got, c.want)
			}
		})
	}
}

// 302 目标只允许 http/https
func TestIsSafeRedirectURL(t *testing.T) {
	cases := map[string]struct {
		in   string
		want bool
	}{
		"http":            {"http://example.com/a", true},
		"https":           {"https://example.com/a?b=1", true},
		"局域网地址允许":         {"http://192.168.1.10:5244/d/a.mkv", true},
		"javascript 伪协议":  {"javascript:alert(1)", false},
		"file 协议":         {"file:///etc/passwd", false},
		"data 协议":         {"data:text/html,<script>", false},
		"缺少协议":            {"example.com/a", false},
		"协议相对地址":          {"//example.com/a", false},
		"空字符串":            {"", false},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := utils.IsSafeRedirectURL(c.in); got != c.want {
				t.Errorf("IsSafeRedirectURL(%q) = %v, 期望 %v", c.in, got, c.want)
			}
		})
	}
}
