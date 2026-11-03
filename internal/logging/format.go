package logging

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/AkimioJR/MediaWarp/constants"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type LoggerServiceFormatter struct{}

func (l *LoggerServiceFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	colorCode := getLogColor(entry.Level)

	var b *bytes.Buffer
	if entry.Buffer == nil {
		b = &bytes.Buffer{}
	} else {
		b = entry.Buffer
	}
	formatTime := entry.Time.Format(time.DateTime)

	fmt.Fprintf(
		b,
		"%s\t%s | %s\n",
		colorCode.ColorString("【"+strings.ToUpper(entry.Level.String())+"】"),
		formatTime,
		entry.Message,
	)
	return b.Bytes(), nil
}

var _ logrus.Formatter = (*LoggerServiceFormatter)(nil)

type LoggerAccessFormatter struct{}

func (l *LoggerAccessFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer == nil {
		b = &bytes.Buffer{}
	} else {
		b = entry.Buffer
	}

	b.WriteString(entry.Message)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

var _ logrus.Formatter = (*LoggerAccessFormatter)(nil)

func getLogColor(level logrus.Level) constants.Color {
	var colorCode constants.Color
	switch level {
	case logrus.DebugLevel:
		colorCode = constants.ColorBlue
	case logrus.InfoLevel:
		colorCode = constants.ColorGreen
	case logrus.WarnLevel:
		colorCode = constants.ColorYellow
	case logrus.ErrorLevel:
		colorCode = constants.ColorRed
	default:
		colorCode = constants.ColorGray
	}
	return colorCode
}

func formatAccessLog(ctx *gin.Context, level logrus.Level, msg string) string {
	var b strings.Builder
	b.WriteString(getLogColor(level).ColorString("【" + strings.ToUpper(level.String()) + "】"))
	b.WriteString(time.Now().Format(time.DateTime))
	b.WriteString(" | ")
	b.WriteString(ctx.ClientIP())
	b.WriteByte('"')
	b.WriteString(ctx.Request.URL.Path)
	b.WriteByte('"')
	b.WriteString(" | ")
	b.WriteString(msg)

	return b.String()
}
