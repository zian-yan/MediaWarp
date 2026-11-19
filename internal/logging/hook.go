package logging

import (
	"os"
	"sync"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/sirupsen/logrus"
)

// logDateLayout 是分目录名与「当前该写哪天」的判定基准，与 config.LogDirWithDate 一致。
const logDateLayout = "2006-01-02"

type LoggerFileHook struct {
	mu        sync.Mutex
	isService bool
	file      *os.File
	// date 是当前打开文件对应的日期（2006-01-02）。按完整日期而不是「几号」比较：
	// 只用 Day() 时，11-19 打开的文件到 12-19 会被判定为「同一天」，日志继续写进
	// 上个月的目录，该目录随后会被保留期清理当成过期目录删掉。
	date string
}

func NewLoggerFileHook(isService bool) *LoggerFileHook {
	return &LoggerFileHook{
		isService: isService,
	}
}

func (h *LoggerFileHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *LoggerFileHook) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	date := entry.Time.Format(logDateLayout)
	if h.file == nil || h.date != date {
		if h.file != nil {
			h.file.Close()
			h.file = nil
		}
		if err := os.MkdirAll(config.LogDirWithDate(), 0o750); err != nil {
			return err
		}
		var filename string
		if h.isService {
			filename = config.ServiceLogPath()
		} else {
			filename = config.AccessLogPath()
		}
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			return err
		}
		h.file = file
		h.date = date
		// 每次跨天换文件时顺手清理过期目录：常驻进程只在启动时清理的话，
		// 一年不重启就会攒满 365 个目录。
		pruneExpiredLogs(config.LogDir(), KeepDays)
	}

	_, err = h.file.WriteString(utils.RemoveColorCodes(line))
	return err
}

var _ logrus.Hook = (*LoggerFileHook)(nil)
