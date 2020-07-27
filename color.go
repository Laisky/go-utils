// =====================================
// Colorfy string by ANSI color
//
// inspired by github.com/fatih/color
// =====================================

package utils

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// ANSIColorEscape escape string for ANSI color
const ANSIColorEscape = "\x1b"

// Base attributes
const (
	ANSIColorReset int = iota
	ANSIColorBold
	ANSIColorFaint
	ANSIColorItalic
	ANSIColorUnderline
	ANSIColorBlinkSlow
	ANSIColorBlinkRapid
	ANSIColorReverseVideo
	ANSIColorConcealed
	ANSIColorCrossedOut
)

// Foreground text colors
const (
	ANSIColorFgBlack int = iota + 30
	ANSIColorFgRed
	ANSIColorFgGreen
	ANSIColorFgYellow
	ANSIColorFgBlue
	ANSIColorFgMagenta
	ANSIColorFgCyan
	ANSIColorFgWhite
)

// Foreground Hi-Intensity text colors
const (
	ANSIColorFgHiBlack int = iota + 90
	ANSIColorFgHiRed
	ANSIColorFgHiGreen
	ANSIColorFgHiYellow
	ANSIColorFgHiBlue
	ANSIColorFgHiMagenta
	ANSIColorFgHiCyan
	ANSIColorFgHiWhite
)

// Background text colors
const (
	ANSIColorBgBlack int = iota + 40
	ANSIColorBgRed
	ANSIColorBgGreen
	ANSIColorBgYellow
	ANSIColorBgBlue
	ANSIColorBgMagenta
	ANSIColorBgCyan
	ANSIColorBgWhite
)

// Background Hi-Intensity text colors
const (
	ANSIColorBgHiBlack int = iota + 100
	ANSIColorBgHiRed
	ANSIColorBgHiGreen
	ANSIColorBgHiYellow
	ANSIColorBgHiBlue
	ANSIColorBgHiMagenta
	ANSIColorBgHiCyan
	ANSIColorBgHiWhite
)

// Color wrap with ANSI color
func Color(color int, s string) string {
	return fmt.Sprintf("\033[1;%dm%s\033[0m", color, s)
}

type gormLoggerItf interface {
	Debug(string, ...zap.Field)
	Info(string, ...zap.Field)
	Error(string, ...zap.Field)
}

// GormLogger colored logger for gorm
type GormLogger struct {
	logger    gormLoggerItf
	formatter func(...interface{}) []interface{}
}

// NewGormLogger new gorm sql logger
func NewGormLogger(formatter func(...interface{}) []interface{}, logger gormLoggerItf) *GormLogger {
	return &GormLogger{
		logger:    logger,
		formatter: formatter,
	}
}

// Print print sql logger
func (l *GormLogger) Print(vs ...interface{}) {
	fvs := l.formatter(vs...)
	var fields []zapcore.Field
	for i, v := range vs {
		switch i {
		case 0:
			fields = append(fields, zap.Any("type", v))
		case 1:
			fields = append(fields, zap.Any("caller", v))
		case 2:
			fields = append(fields, zap.Any("ms", v))
		case 3:
			if len(fvs) < 4 {
				fields = append(fields, zap.Any("sql", v))
			}
		case 4:
			if len(fvs) < 4 {
				fields = append(fields, zap.Any("args", v))
			}
		case 5:
			fields = append(fields, zap.Any("affected", v))
		default:
			fields = append(fields, zap.Any(strconv.FormatInt(int64(i), 10), v))
		}
	}

	if len(fvs) >= 4 {
		switch fvs[3].(type) {
		case string:
			s := strings.TrimSpace(strings.ToLower(fvs[3].(string)))
			if strings.HasPrefix(s, "delete") || strings.HasPrefix(s, "drop") {
				l.logger.Info(Color(ANSIColorFgMagenta, s), fields...)
			} else if strings.HasPrefix(s, "insert") {
				l.logger.Info(Color(ANSIColorFgGreen, s), fields...)
			} else if strings.HasPrefix(s, "update") {
				l.logger.Info(Color(ANSIColorFgYellow, s), fields...)
			} else if strings.HasPrefix(s, "select") {
				l.logger.Debug(Color(ANSIColorFgCyan, s), fields...)
			} else if strings.HasPrefix(s, "error") {
				l.logger.Error(Color(ANSIColorFgHiRed, s), fields...)
			} else {
				l.logger.Debug(Color(ANSIColorFgBlue, s), fields...)
			}
		default:
			l.logger.Debug(Color(ANSIColorFgBlue, fmt.Sprint(fvs[3])), fields...)
		}
	} else {
		l.logger.Debug("", fields...)
	}
}
