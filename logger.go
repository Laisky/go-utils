package utils

import (
	"fmt"
	"math/rand"

	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

var (
	/*Logger logging tool.

	* Info(msg string, fields ...Field)
	* Debug(msg string, fields ...Field)
	* Warn(msg string, fields ...Field)
	* Error(msg string, fields ...Field)
	* Panic(msg string, fields ...Field)
	* DebugSample(sample int, msg string, fields ...zap.Field)
	* InfoSample(sample int, msg string, fields ...zap.Field)
	* WarnSample(sample int, msg string, fields ...zap.Field)
	 */
	Logger *LoggerType
)

// SampleRateDenominator sample rate = sample / SampleRateDenominator
const SampleRateDenominator = 1000

// LoggerType extend from zap.Logger
type LoggerType struct {
	*zap.Logger
	level zap.AtomicLevel
}

// NewLogger create new logger
func NewLogger(level string) (l *LoggerType, err error) {
	zl := zap.NewAtomicLevel()
	cfg := zap.Config{
		Level:            zl,
		Development:      false,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %+v", err)
	}

	l = &LoggerType{
		Logger: zapLogger,
		level:  zl,
	}
	return l, l.ChangeLevel(level)
}

// ChangeLevel change logger level
func (l *LoggerType) ChangeLevel(level string) (err error) {
	switch level {
	case "debug":
		l.level.SetLevel(zap.DebugLevel)
	case "info":
		l.level.SetLevel(zap.InfoLevel)
	case "warn":
		l.level.SetLevel(zap.WarnLevel)
	case "error":
		l.level.SetLevel(zap.ErrorLevel)
	default:
		return fmt.Errorf("log level only be debug/info/warn/error")
	}

	return
}

// DebugSample emit debug log with propability sample/SampleRateDenominator.
// sample could be [0, 1000], less than 0 means never, great than 1000 means certainly
func (l *LoggerType) DebugSample(sample int, msg string, fields ...zap.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Debug(msg, fields...)
}

// InfoSample emit info log with propability sample/SampleRateDenominator
func (l *LoggerType) InfoSample(sample int, msg string, fields ...zap.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Info(msg, fields...)
}

// WarnSample emit warn log with propability sample/SampleRateDenominator
func (l *LoggerType) WarnSample(sample int, msg string, fields ...zap.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Warn(msg, fields...)
}

func init() {
	var err error
	if Logger, err = NewLogger("info"); err != nil {
		panic(fmt.Sprintf("create logger: %+v", err))
	}
	Logger.Info("create logger", zap.String("level", "info"))
}
