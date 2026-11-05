package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/AkimioJR/MediaWarp/constants"
	"github.com/AkimioJR/MediaWarp/internal/config"

	"github.com/gin-gonic/gin"
)

var (
	jellyfinSubtitle = constants.JellyfinRegexp.Router.SubtitleStream
	embySubtitle     = constants.EmbyRegexp.Router.SubtitleStream
)

func TestParseSubtitleStreamPath(t *testing.T) {
	tests := []struct {
		name      string
		reg       *regexp.Regexp
		path      string
		wantItem  string
		wantIndex int
		wantOK    bool
	}{
		{"jellyfin 带 mediaSourceId", jellyfinSubtitle, "/Videos/4dc5c96e/abc123/Subtitles/2/Stream.vtt", "4dc5c96e", 2, true},
		{"jellyfin 带起始位置", jellyfinSubtitle, "/Videos/4dc5c96e/abc123/Subtitles/3/120000/Stream.vtt", "4dc5c96e", 3, true},
		{"jellyfin 省略 mediaSourceId", jellyfinSubtitle, "/Videos/4dc5c96e/Subtitles/0/Stream.srt", "4dc5c96e", 0, true},
		{"emby 带 /emby 前缀", embySubtitle, "/emby/Videos/12345/abc/Subtitles/1/Stream.srt", "12345", 1, true},
		{"emby 无前缀", embySubtitle, "/Videos/12345/abc/Subtitles/4/Stream.ass", "12345", 4, true},
		{"非字幕路径", embySubtitle, "/Videos/12345/stream", "", 0, false},
		{"index 非数字", embySubtitle, "/Videos/12345/abc/Subtitles/x/Stream.srt", "", 0, false},
		{"大小写混合", jellyfinSubtitle, "/videos/4dc5c96e/abc123/subtitles/7/Stream.vtt", "4dc5c96e", 7, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, index, ok := parseSubtitleStreamPath(tt.reg, tt.path)
			if ok != tt.wantOK || item != tt.wantItem || index != tt.wantIndex {
				t.Fatalf("parseSubtitleStreamPath(%q) = (%q, %d, %v)，期望 (%q, %d, %v)",
					tt.path, item, index, ok, tt.wantItem, tt.wantIndex, tt.wantOK)
			}
		})
	}
}

func TestMatchUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		keywords  []string
		want      bool
	}{
		{"命中", "Polaris Player/5.2.5 (iPhone; iOS 18.0)", []string{"Polaris"}, true},
		{"忽略大小写", "polarisplayer/5.2.5", []string{"Polaris"}, true},
		{"未命中", "Mozilla/5.0 (Windows NT 10.0) Chrome/130", []string{"Polaris", "Fileball"}, false},
		{"浏览器 UA 命中 Mozilla", "Mozilla/5.0 (Macintosh) Safari/605", []string{"Mozilla"}, true},
		{"空白项被忽略", "Mozilla/5.0", []string{" ", ""}, false},
		{"空 UA", "", []string{"Polaris"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchUserAgent(tt.userAgent, tt.keywords); got != tt.want {
				t.Fatalf("matchUserAgent(%q, %v) = %v，期望 %v", tt.userAgent, tt.keywords, got, tt.want)
			}
		})
	}
}

func newSubtitleTestHandler(t *testing.T, mediaStreams string) (*JellyfinHandler, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"Items":[{"Path":"/media/strm/http/movie.strm","MediaSources":[{"MediaStreams":`+mediaStreams+`}]}]}`)
	}))
	t.Cleanup(upstream.Close)

	handler, err := NewJellyfinHandler(upstream.URL, "test-key")
	if err != nil {
		t.Fatalf("创建 Jellyfin handler 失败：%v", err)
	}
	return handler, &hits
}

func serveSubtitle(handler gin.HandlerFunc, path string, userAgent string) int {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.NoRoute(handler)

	server := httptest.NewServer(engine)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		return -1
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := server.Client().Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func withHTTPStrmSetting(t *testing.T, setting config.HTTPStrmSetting) {
	t.Helper()
	old := config.HTTPStrm
	config.HTTPStrm = setting
	t.Cleanup(func() { config.HTTPStrm = old })
}

func TestSubtitlesHandlerBlocksEmbedded(t *testing.T) {
	withHTTPStrmSetting(t, config.HTTPStrmSetting{
		Enable:                true,
		PrefixList:            []string{"/media/strm/http"},
		HideEmbeddedSubtitles: true,
	})

	handler, hits := newSubtitleTestHandler(t, `[{"Index":2,"Type":"Subtitle","IsExternal":false}]`)
	code := serveSubtitle(handler.SubtitlesHandler, "/Videos/abc/abc/Subtitles/2/Stream.vtt", "Polaris Player/5.2.5")

	if code != http.StatusNotFound {
		t.Fatalf("内嵌字幕请求应被拦截为 404，实际 %d", code)
	}
	if hits.Load() != 1 {
		t.Fatalf("应只查询一次 Items 且不转发请求，实际上游被请求 %d 次", hits.Load())
	}
}

func TestSubtitlesHandlerKeepsExternal(t *testing.T) {
	withHTTPStrmSetting(t, config.HTTPStrmSetting{
		Enable:                true,
		PrefixList:            []string{"/media/strm/http"},
		HideEmbeddedSubtitles: true,
	})

	handler, hits := newSubtitleTestHandler(t, `[{"Index":2,"Type":"Subtitle","IsExternal":true,"DeliveryMethod":"External"}]`)
	code := serveSubtitle(handler.SubtitlesHandler, "/Videos/abc/abc/Subtitles/2/Stream.vtt", "Polaris Player/5.2.5")

	if code != http.StatusOK {
		t.Fatalf("外挂字幕应正常放行，实际 %d", code)
	}
	if hits.Load() != 2 {
		t.Fatalf("外挂字幕应转发到上游，实际上游被请求 %d 次", hits.Load())
	}
}

func TestSubtitlesHandlerSkipsOtherClients(t *testing.T) {
	withHTTPStrmSetting(t, config.HTTPStrmSetting{
		Enable:                       true,
		PrefixList:                   []string{"/media/strm/http"},
		HideEmbeddedSubtitles:        true,
		HideEmbeddedSubtitlesClients: []string{"Polaris"},
	})

	handler, hits := newSubtitleTestHandler(t, `[{"Index":2,"Type":"Subtitle","IsExternal":false}]`)
	code := serveSubtitle(handler.SubtitlesHandler, "/Videos/abc/abc/Subtitles/2/Stream.vtt", "Mozilla/5.0 Chrome/130")

	if code != http.StatusOK {
		t.Fatalf("不在名单内的客户端应放行，实际 %d", code)
	}
	if hits.Load() != 1 {
		t.Fatalf("不在名单内的客户端应直接转发、不查询 Items，实际上游被请求 %d 次", hits.Load())
	}
}
