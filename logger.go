package utils

import (
	"encoding/json"
	"fmt"

	zap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
)

// SetupLogger contstruct logger
func SetupLogger(level string) {
	rawJSON := []byte(fmt.Sprintf(`{
		"level": "%v",
		"encoding": "json",
		"outputPaths": ["stdout", "/tmp/logs"],
		"errorOutputPaths": ["stderr"]
	  }`, level))

	var cfg zap.Config
	err := json.Unmarshal(rawJSON, &cfg)
	if err != nil {
		panic(err)
	}

	cfg.EncoderConfig = zap.NewProductionEncoderConfig()
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
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
