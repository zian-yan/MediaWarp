package logging

import (
	"os"
	"sync"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/sirupsen/logrus"
)

type LoggerFileHook struct {
	mu        sync.Mutex // logrus 会并发调用 Fire，句柄切换必须串行
	isService bool
	file      *os.File
	day       int
}

func NewLoggerFileHook(isService bool) *LoggerFileHook {
	return &LoggerFileHook{
		isService: isService,
	}
}

func (h *LoggerFileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// HOOK
//
// 将日志写入文件
func (h *LoggerFileHook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.file == nil || h.day != entry.Time.Day() {
		if h.file != nil {
			h.file.Close()
			h.file = nil
		}
		if err := os.MkdirAll(config.LogDirWithDate(), 0o750); err != nil { // 仅属主与同组可访问
			return err
		}
		var filename string
		if h.isService {
			filename = config.ServiceLogPath()
		} else {
			filename = config.AccessLogPath()
		}
		// 日志可能包含 url 等敏感信息，权限收紧到 0640
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
		h.file = file
		h.day = entry.Time.Day() // 记录归属日，避免每行日志都重开文件
	}

	_, err = h.file.WriteString(utils.RemoveColorCodes(line))
	return err
}

var _ logrus.Hook = (*LoggerFileHook)(nil)
