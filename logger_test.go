package utils_test

import (
	"testing"

	utils "github.com/Laisky/go-utils"
	zap "github.com/Laisky/zap"
	// zap "github.com/Laisky/zap"
)

func TestSetupLogger(t *testing.T) {
	utils.SetupLogger("debug")
	utils.Logger.Info("test", zap.String("arg", "yo"))
	utils.Logger.Debug("test", zap.String("arg", "yo"))
	utils.Logger.Sync()
}

// func setupLogger(level string) *zap2.Logger {
// 	var loglevel zap2.AtomicLevel
// 	switch level {
// 	case "debug":
// 		loglevel = zap2.NewAtomicLevelAt(zap2.DebugLevel)
// 	case "info":
// 		loglevel = zap2.NewAtomicLevelAt(zap2.InfoLevel)
// 	case "warn":
// 		loglevel = zap2.NewAtomicLevelAt(zap2.WarnLevel)
// 	case "error":
// 		loglevel = zap2.NewAtomicLevelAt(zap2.ErrorLevel)
// 	default:
// 		panic(fmt.Errorf("log level only be debug/info/warn/error"))
// 	}

// 	cfg := zap2.Config{
// 		Level:       loglevel,
// 		Development: false,
// 		Sampling: &zap2.SamplingConfig{
// 			Initial:    100,
// 			Thereafter: 100,
// 		},
// 		Encoding:         "json",
// 		EncoderConfig:    zap2.NewProductionEncoderConfig(),
// 		OutputPaths:      []string{"stdout"},
// 		ErrorOutputPaths: []string{"stderr"},
// 	}
// 	cfg.EncoderConfig.MessageKey = "message"
// 	cfg.EncoderConfig.EncodeTime = zapcore2.ISO8601TimeEncoder

// 	logger, err := cfg.Build()
// 	if err != nil {
// 		panic(err)
// 	}

// 	defer logger.Sync()
// 	// logger.Info("Logger construction succeeded", zap2.String("level", level))

// 	return logger
// }

// func BenchmarkLogger(b *testing.B) {
// 	utils.SetupLogger("info")
// 	b.Run("origin zap", func(b *testing.B) {
// 		for i := 0; i < b.N; i++ {
// 			utils.Logger.Debug("yooo")
// 		}
// 	})

// 	logger := setupLogger("info")
// 	b.Run("new zap", func(b *testing.B) {
// 		for i := 0; i < b.N; i++ {
// 			logger.Debug("yooo")
// 		}
// 	})
// }

func BenchmarkLogger(b *testing.B) {
	utils.SetupLogger("info")
	b.Run("low level log", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Logger.Debug("yooo")
		}
	})

	utils.SetupLogger("")
	// b.Run("log", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		utils.Logger.Info("yooo")
	// 	}
	// })
}
