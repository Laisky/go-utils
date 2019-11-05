package utils_test

import (
	"context"
	"testing"
	"time"

	utils "github.com/Laisky/go-utils"
	zap "github.com/Laisky/zap"
	// zap "github.com/Laisky/zap"
)

func TestSetupLogger(t *testing.T) {
	var err error
	utils.Logger.Info("test", zap.String("arg", "111"))
	if err = utils.Logger.ChangeLevel("error"); err != nil {
		t.Fatalf("set level: %+v", err)
	}
	utils.Logger.Info("test", zap.String("arg", "222"))
	utils.Logger.Debug("test", zap.String("arg", "333"))
	// if err := utils.Logger.Sync(); err != nil {
	// 	t.Fatalf("%+v", err)
	// }

	// t.Error()
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
// 	utils.Logger.ChangeLevel("info")
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
	var err error
	if err = utils.Logger.ChangeLevel("error"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	b.Run("low level log", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Logger.Debug("yooo")
		}
	})

	if err = utils.Logger.ChangeLevel("error"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	// b.Run("log", func(b *testing.B) {
	// 	for i := 0; i < b.N; i++ {
	// 		utils.Logger.Info("yooo")
	// 	}
	// })
}

func BenchmarkSampleLogger(b *testing.B) {
	var err error
	if err = utils.Logger.ChangeLevel("error"); err != nil {
		b.Fatalf("set level: %+v", err)
	}
	b.Run("low level log", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			utils.Logger.DebugSample(100, "yooo")
		}
	})
}

func TestAlertHook(t *testing.T) {
	pusher, err := utils.NewAlertPusherWithAlertType(
		context.Background(),
		"https://blog.laisky.com/graphql/query/",
		"hello",
		"rwkpVuAgaBZQBASKndHK",
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer pusher.Close()
	hook := utils.NewAlertHook(
		pusher,
		utils.WithAlertHookLevel(zap.WarnLevel),
	)
	logger, err := utils.NewLoggerWithName(
		"test",
		"debug",
		zap.Fields(zap.String("logger", "test")),
		zap.Hooks(hook.GetZapHook()),
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	logger.Debug("DEBUG", zap.String("yo", "hello"))
	logger.Info("Info", zap.String("yo", "hello"))
	logger.Warn("Warn", zap.String("yo", "hello"))
	logger.Error("Error", zap.String("yo", "hello"))
	// t.Error()

	time.Sleep(5 * time.Second)
}
func ExampleAlertHook() {
	pusher, err := utils.NewAlertPusherWithAlertType(
		context.Background(),
		"https://blog.laisky.com/graphql/query/",
		"hello",
		"rwkpVuAgaBZQBASKndHK",
	)
	if err != nil {
		utils.Logger.Panic("create alert pusher", zap.Error(err))
	}
	defer pusher.Close()
	hook := utils.NewAlertHook(
		pusher,
		utils.WithAlertHookLevel(zap.WarnLevel),
	)
	logger, err := utils.NewLogger(
		"debug",
		zap.Fields(zap.String("logger", "test")),
		zap.Hooks(hook.GetZapHook()),
	)
	if err != nil {
		utils.Logger.Error("create new logger", zap.Error(err))
	}

	logger.Debug("DEBUG", zap.String("yo", "hello"))
	logger.Info("Info", zap.String("yo", "hello"))
	logger.Warn("Warn", zap.String("yo", "hello"))
	logger.Error("Error", zap.String("yo", "hello"))

	time.Sleep(1 * time.Second)
}
