package utils_test

import (
	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"testing"
)

// These are consumer assignments, not assertions about a dynamic value.
var (
	_ utils.LoggerItf                                              = utils.Logger
	_ utils.LoggerItf                                              = (*utils.LoggerType)(nil)
	_ func(...utils.LoggerOptFunc) (utils.LoggerItf, error)        = utils.NewLogger
	_ func(string, string, ...zap.Option) (utils.LoggerItf, error) = utils.CreateNewDefaultLogger
	_ func(string, string, ...zap.Option) (utils.LoggerItf, error) = utils.NewLoggerWithName
	_ func(string, string, ...zap.Option) (utils.LoggerItf, error) = utils.NewConsoleLoggerWithName
	_ func(*utils.LoggerType) utils.LoggerItf                      = (*utils.LoggerType).Clone
	_ func(*utils.LoggerType, string) utils.LoggerItf              = (*utils.LoggerType).Named
	_ func(*utils.LoggerType, ...zapcore.Field) utils.LoggerItf    = (*utils.LoggerType).With
	_ func(*utils.LoggerType, ...zap.Option) utils.LoggerItf       = (*utils.LoggerType).WithOptions
)

func TestLegacyLoggerInterfaceSignatures(t *testing.T) {
	var loggerName string
	logger, err := utils.NewConsoleLoggerWithName("typed-contract", "debug", zap.Hooks(func(e zapcore.Entry) error { loggerName = e.LoggerName; return nil }))
	if err != nil {
		t.Fatal(err)
	}
	var child utils.LoggerItf = logger.Clone().Named("child").With(zap.String("context", "kept")).WithOptions()
	child.Info("synthetic typed contract")
	if loggerName != "typed-contract.child" || child.Level() != zapcore.DebugLevel {
		t.Fatal("typed child identity or level changed")
	}
}
