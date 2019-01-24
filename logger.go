package utils

import (
	"fmt"

	zap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
)

// SetupLogger contstruct logger
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
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	Logger, err = cfg.Build()
	if err != nil {
		panic(err)
	}

	defer Logger.Sync()
	Logger.Info("Logger construction succeeded", zap.String("level", level))
}

func init() {
	SetupLogger("info")
}
