package logging

import (
	"fmt"
	"io"
	"os"

	"github.com/AkimioJR/MediaWarp/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	accessLogger  = logrus.New()
	serviceLogger = logrus.New()
)

func init() {
	accessLogger.SetFormatter(&LoggerAccessFormatter{})
	serviceLogger.SetFormatter(&LoggerServiceFormatter{})
}

func Init() {
	serviceLogger.SetReportCaller(false)

	if config.Logger.AccessLogger.Console {
		accessLogger.SetOutput(os.Stdout)
	} else {
		accessLogger.SetOutput(io.Discard)
	}
	if config.Logger.ServiceLogger.Console {
		serviceLogger.SetOutput(os.Stdout)
	} else {
		serviceLogger.SetOutput(io.Discard)
	}

	if config.Logger.AccessLogger.File {
		accessLogger.AddHook(NewLoggerFileHook(false))
	}
	if config.Logger.ServiceLogger.File {
		serviceLogger.AddHook(NewLoggerFileHook(true))
	}

	// 启动时先清一次过期日志：容器可能每天重启，跨天换文件那条清理路径不一定
	// 有机会执行；只删保留期之外的日期分目录，不影响当天正在写的文件。
	pruneExpiredLogs(config.LogDir(), KeepDays)
}

func AccessLogf(format string, args ...any) {
	accessLogger.Info(fmt.Sprintf(format, args...))
}

func AccessDebug(ctx *gin.Context, args ...any) {
	accessLogger.Debug(formatAccessLog(ctx, logrus.DebugLevel, fmt.Sprint(args...)))
}

func AccessDebugf(ctx *gin.Context, format string, args ...any) {
	accessLogger.Debug(formatAccessLog(ctx, logrus.DebugLevel, fmt.Sprintf(format, args...)))
}

func AccessWarning(ctx *gin.Context, args ...any) {
	accessLogger.Warning(formatAccessLog(ctx, logrus.WarnLevel, fmt.Sprint(args...)))
}

func AccessWarningf(ctx *gin.Context, format string, args ...any) {
	accessLogger.Warning(formatAccessLog(ctx, logrus.WarnLevel, fmt.Sprintf(format, args...)))
}

func Debug(args ...any) {
	serviceLogger.Debug(args...)
}

func Debugf(format string, args ...any) {
	serviceLogger.Debugf(format, args...)
}

func Info(args ...any) {
	serviceLogger.Info(args...)
}

func Infof(format string, args ...any) {
	serviceLogger.Infof(format, args...)
}

func Warning(args ...any) {
	serviceLogger.Warning(args...)
}

func Warningf(format string, args ...any) {
	serviceLogger.Warningf(format, args...)
}

func Error(args ...any) {
	serviceLogger.Error(args...)
}

func Errorf(format string, args ...any) {
	serviceLogger.Errorf(format, args...)
}

func SetLevel(level logrus.Level) {
	accessLogger.SetLevel(level)
	serviceLogger.SetLevel(level)
}
