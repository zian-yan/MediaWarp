package logging

import (
	"os"
	"sync"

	"github.com/AkimioJR/MediaWarp/internal/config"
	"github.com/AkimioJR/MediaWarp/utils"

	"github.com/sirupsen/logrus"
)

type LoggerFileHook struct {
	mu        sync.Mutex
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
		h.day = entry.Time.Day()
	}

	_, err = h.file.WriteString(utils.RemoveColorCodes(line))
	return err
}

var _ logrus.Hook = (*LoggerFileHook)(nil)
