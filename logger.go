package utils

import (
	"fmt"
	"math/rand"
	"sync"

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
	sync.RWMutex
	logger *zap.Logger
}

// Debug emit log with Debug level
func (l *LoggerType) Debug(msg string, fields ...zap.Field) {
	l.RLock()
	l.logger.Debug(msg, fields...)
	l.RUnlock()
}

// Info emit log with Info level
func (l *LoggerType) Info(msg string, fields ...zap.Field) {
	l.RLock()
	l.logger.Info(msg, fields...)
	l.RUnlock()
}

// Warn emit log with Warn level
func (l *LoggerType) Warn(msg string, fields ...zap.Field) {
	l.RLock()
	l.logger.Warn(msg, fields...)
	l.RUnlock()
}

// Error emit log with Error level
func (l *LoggerType) Error(msg string, fields ...zap.Field) {
	l.RLock()
	l.logger.Error(msg, fields...)
	l.RUnlock()
}

// Panic emit log with Panic level
func (l *LoggerType) Panic(msg string, fields ...zap.Field) {
	l.RLock()
	l.logger.Panic(msg, fields...)
	l.RUnlock()
}

// ChangeLevel change logger level
func (l *LoggerType) ChangeLevel(level string) (err error) {
	var loglevel zap.AtomicLevel
	switch level {
	case "debug":
		loglevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		loglevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		loglevel = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		loglevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		return fmt.Errorf("log level only be debug/info/warn/error")
	}

	cfg := zap.Config{
		Level:            loglevel,
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
		panic(err)
	}

	l.Lock()
	l.logger = zapLogger
	l.Unlock()

	return
}

// SetupLogger contstruct logger
//
// Deprecated: use `ChangeLevel` instead
func SetupLogger(level string) {
	var loglevel zap.AtomicLevel
	switch level {
	case "debug":
		loglevel = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		loglevel = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		loglevel = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		loglevel = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		panic(fmt.Errorf("log level only be debug/info/warn/error"))
	}

	cfg := zap.Config{
		Level:       loglevel,
		Development: false,
		// Sampling: &zap.SamplingConfig{
		// 	Initial:    100,
		// 	Thereafter: 100,
		// },
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	Logger = &LoggerType{
		logger: zapLogger,
	}

	Logger.Debug("Logger construction succeeded", zap.String("level", level))
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
	SetupLogger("info")
}
