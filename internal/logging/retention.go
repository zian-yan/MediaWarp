package logging

import (
	"os"
	"path/filepath"
	"time"
)

// KeepDays 是日志分目录的保留天数，与 AutoFilm 的按天日志保持一致。
//
// 超过这个天数的分目录会在启动时、以及每次跨天切换日志文件时删除；当天与保留期
// 内的目录一律不动，因此不会影响正在写入的文件。
const KeepDays = 7

// dateLayouts 兼容两种目录名：新版本带前导零（2026-01-02），旧版本由
// time.Format("2006-1-2") 生成（2026-1-2）。两种都要能解析，否则升级后旧目录
// 永远不会被清理。
var dateLayouts = []string{"2006-01-02", "2006-1-2"}

// pruneExpiredLogs 删除 root 下超过 keepDays 天的日期分目录。
//
// 只处理名字能解析成日期的目录：其它文件与目录（例如手工放进来的脚本、说明文件）
// 一律不动。删除失败静默忽略——清理跑在日志写入路径上，删不掉旧目录不能影响写日志。
func pruneExpiredLogs(root string, keepDays int) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := today.AddDate(0, 0, -keepDays)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		day, ok := parseLogDay(entry.Name())
		if !ok || !day.Before(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
}

// parseLogDay 把分目录名解析成本地时区的当天零点。
func parseLogDay(name string) (time.Time, bool) {
	for _, layout := range dateLayouts {
		if day, err := time.ParseInLocation(layout, name, time.Local); err == nil {
			return day, true
		}
	}
	return time.Time{}, false
}
