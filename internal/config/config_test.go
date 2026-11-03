package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFromString(t *testing.T, content string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return loadConfig(path)
}

func resetConfig(t *testing.T) {
	t.Helper()
	old := struct {
		port uint16
		ms   MediaServerSetting
		log  LoggerSetting
		c    CacheSetting
		cf   ClientFilterSetting
		hs   HTTPStrmSetting
	}{Port, MediaServer, Logger, Cache, ClientFilter, HTTPStrm}
	t.Cleanup(func() {
		Port, MediaServer, Logger, Cache, ClientFilter, HTTPStrm = old.port, old.ms, old.log, old.c, old.cf, old.hs
	})
}

const validConfig = `
port: 9000
server:
  type: Emby
  addr: http://localhost:8096
  auth: testkey
log:
  access:
    console: true
    file: false
  service:
    console: true
    file: true
cache:
  enable: true
  http_strm_ttl: 1m
  image_ttl: 10m
  subtitle_ttl: 2h
client:
  enable: true
  mode: BlackList
  list:
    - Fileball
http_strm:
  enable: true
  proxy: false
  final_url: true
  compatibility_mode: false
  prefix_list:
    - /media/strm/http
`

func TestLoadConfigValid(t *testing.T) {
	resetConfig(t)

	if err := loadFromString(t, validConfig); err != nil {
		t.Fatalf("合法配置应加载成功，实际报错: %v", err)
	}
	if Port != 9000 {
		t.Errorf("监听端口 = %d，期望 9000", Port)
	}
	if MediaServer.ADDR != "http://localhost:8096" || MediaServer.AUTH != "testkey" {
		t.Errorf("服务器配置未正确加载: %+v", MediaServer)
	}
	if Cache.ImageTTL.String() != "10m0s" || Cache.SubtitleTTL.String() != "2h0m0s" {
		t.Errorf("缓存时长解析错误: %+v", Cache)
	}
	if len(ClientFilter.ClientList) != 1 || ClientFilter.ClientList[0] != "Fileball" {
		t.Errorf("客户端名单未正确加载: %+v", ClientFilter)
	}
	if len(HTTPStrm.PrefixList) != 1 || HTTPStrm.PrefixList[0] != "/media/strm/http" {
		t.Errorf("Strm 前缀未正确加载: %+v", HTTPStrm)
	}
}

func TestLoadConfigIgnoresLegacyWebSection(t *testing.T) {
	resetConfig(t)

	content := validConfig + "\nweb:\n  enable: true\n"
	if err := loadFromString(t, content); err != nil {
		t.Fatalf("带 web 段的旧配置应能正常加载，实际报错: %v", err)
	}
	if Port != 9000 {
		t.Errorf("监听端口 = %d，期望 9000", Port)
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"端口为 0", replaceOnce(validConfig, "port: 9000", "port: 0"), "端口"},
		{"端口超出 uint16", replaceOnce(validConfig, "port: 9000", "port: 70000"), "解析配置文件失败"},
		{"地址为空", replaceOnce(validConfig, "addr: http://localhost:8096", "addr: \"\""), "server.addr 不能为空"},
		{"地址缺协议", replaceOnce(validConfig, "addr: http://localhost:8096", "addr: localhost:8096"), "http://"},
		{"地址缺主机名", replaceOnce(validConfig, "addr: http://localhost:8096", "addr: http://"), "主机名"},
		{"白名单空名单", replaceOnce(replaceOnce(validConfig, "mode: BlackList", "mode: WhiteList"), "    - Fileball", ""), "白名单"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetConfig(t)
			err := loadFromString(t, c.content)
			if err == nil {
				t.Fatalf("非法配置应报错，实际加载成功")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("错误信息应包含 %q，实际: %v", c.want, err)
			}
		})
	}
}

func TestLoadConfigUnknownServerType(t *testing.T) {
	resetConfig(t)

	err := loadFromString(t, replaceOnce(validConfig, "type: Emby", "type: Plex"))
	if err == nil || !strings.Contains(err.Error(), "解析配置文件失败") {
		t.Fatalf("未知类型应解析失败，实际: %v", err)
	}
}

func TestLoadConfigCacheDisabled(t *testing.T) {
	resetConfig(t)

	content := replaceOnce(replaceOnce(validConfig, "image_ttl: 10m", "image_ttl: 0s"), "subtitle_ttl: 2h", "subtitle_ttl: 0s")
	if err := loadFromString(t, content); err != nil {
		t.Fatalf("TTL 为 0 表示关闭缓存，属于合法配置，实际报错: %v", err)
	}
	if Cache.ImageTTL != 0 {
		t.Errorf("图片缓存 TTL 应为 0，实际: %s", Cache.ImageTTL)
	}
}

func replaceOnce(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}
