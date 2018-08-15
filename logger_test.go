package utils_test

import (
	"testing"

	utils "github.com/Laisky/go-utils"
	zap "go.uber.org/zap"
)

func TestSetupLogger(t *testing.T) {
	utils.SetupLogger("debug")
	utils.Logger.Info("test", zap.String("arg", "yo"))

	t.Error("done")
}
