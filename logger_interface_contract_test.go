package utils_test

import (
	"bytes"
	"encoding/json"
	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"testing"
)

func TestLoggerConcreteChildrenPreserveFieldsAndLevel(t *testing.T) {
	var output bytes.Buffer
	logger, err := utils.NewConcreteLogger(utils.WithLoggerName("typed"), utils.WithLoggerLevel("debug"), utils.WithLoggerZapOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&output), core)
	})))
	if err != nil {
		t.Fatal(err)
	}
	var child *utils.LoggerType = logger.CloneConcrete().NamedConcrete("child").WithConcrete(zap.String("context", "kept")).WithOptionsConcrete()
	if err := child.ChangeLevel("error"); err != nil {
		t.Fatal(err)
	}
	if logger.Level() != zapcore.ErrorLevel {
		t.Fatal("children no longer share the dynamic level")
	}
	logger.Debug("must be filtered")
	child.Error("chosen diagnostic", zap.Int64("sequence", 9007199254740993))
	logger.Error("parent control")
	decoder := json.NewDecoder(&output)
	decoder.UseNumber()
	var record map[string]interface{}
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record["logger"] != "typed.child" || record["msg"] != "chosen diagnostic" || record["context"] != "kept" || record["sequence"] != json.Number("9007199254740993") {
		t.Fatal("child lost its chosen diagnostic or context", record)
	}
	record = nil
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if _, leaked := record["context"]; leaked {
		t.Fatal("child fields leaked to parent")
	}
	if record["logger"] != "typed" || record["msg"] != "parent control" {
		t.Fatal("child mutated parent identity", record)
	}
	// Decode into a fresh map: encoding/json otherwise retains absent keys.
	if err := logger.ChangeLevel("debug"); err != nil {
		t.Fatal(err)
	}
	if child.Level() != zapcore.DebugLevel {
		t.Fatal("parent level no longer reaches children")
	}
}

func TestLoggerInterfaceFactoriesAndCustomConsumer(t *testing.T) {
	original := utils.Logger
	defer func() { utils.Logger = original }()
	constructors := []struct {
		name string
		make func(string, ...zap.Option) (utils.LoggerItf, error)
	}{
		{"base", func(level string, opts ...zap.Option) (utils.LoggerItf, error) {
			return utils.NewLogger(utils.WithLoggerName("interface"), utils.WithLoggerLevel(level), utils.WithLoggerZapOptions(opts...))
		}},
		{"named", func(level string, opts ...zap.Option) (utils.LoggerItf, error) {
			return utils.NewLoggerWithName("interface", level, opts...)
		}},
		{"console", func(level string, opts ...zap.Option) (utils.LoggerItf, error) {
			return utils.NewConsoleLoggerWithName("interface", level, opts...)
		}},
		{"default", func(level string, opts ...zap.Option) (utils.LoggerItf, error) {
			return utils.CreateNewDefaultLogger("interface", level, opts...)
		}},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			var name, message string
			logger, err := constructor.make("debug", zap.Hooks(func(entry zapcore.Entry) error { name, message = entry.LoggerName, entry.Message; return nil }))
			if err != nil {
				t.Fatal(err)
			}
			var child utils.LoggerItf = logger.Clone().Named("child").With(zap.String("context", "kept")).WithOptions()
			child.Info("interface diagnostic")
			if name != "interface.child" || message != "interface diagnostic" {
				t.Fatal("interface fluent chain lost logging behavior")
			}
			if err := child.ChangeLevel("error"); err != nil {
				t.Fatal(err)
			}
			if logger.Level() != zapcore.ErrorLevel {
				t.Fatal("interface children lost shared dynamic level")
			}
			invalid, err := constructor.make("invalid-level")
			if err == nil || invalid != nil {
				t.Fatal("interface constructor did not preserve nil-on-error")
			}
		})
	}
	base, err := utils.NewLogger()
	if err != nil {
		t.Fatal(err)
	}
	custom := &contractCustomLogger{LoggerItf: base}
	consumeContractLogger(custom)
	if custom.message != "custom implementation control" {
		t.Fatal("interface consumer required a concrete logger")
	}
	invalid, err := utils.NewConcreteConsoleLoggerWithName("invalid", "invalid-level")
	if err == nil || invalid != nil {
		t.Fatal("concrete factory did not preserve nil-on-error")
	}
	utils.Logger = custom
	utils.Logger.Info("global interface replacement control")
	if custom.message != "global interface replacement control" {
		t.Fatal("default logger no longer accepts custom interfaces")
	}
}

type contractCustomLogger struct {
	utils.LoggerItf
	message string
}

var _ utils.LoggerItf = (*contractCustomLogger)(nil)

func (logger *contractCustomLogger) Info(message string, fields ...zapcore.Field) {
	logger.message = message
}
func consumeContractLogger(logger utils.LoggerItf) { logger.Info("custom implementation control") }
