package logging

import (
	"os"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/sirupsen/logrus"
)

type LoggerFileHook struct {
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
	if h.file == nil || h.day != entry.Time.Day() {
		if h.file != nil {
			h.file.Close()
		}
		err := os.MkdirAll(config.LogDirWithDate(), 0o750) // 仅属主与同组可访问
		if err != nil {
			return err
		}
		var filename string
		if h.isService {
			filename = config.ServiceLogPath()
		} else {
			filename = config.AccessLogPath()
		}
		// 日志可能包含 url 等敏感信息，权限收紧到 0640
		h.file, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
	}

	line, err := entry.String()
	if err != nil {
		return err
	}
	_, err = h.file.WriteString(utils.RemoveColorCodes(line))
	return err
}

var _ logrus.Hook = (*LoggerFileHook)(nil)
