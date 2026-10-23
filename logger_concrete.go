package utils

import (
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// LoggerConfig preserves the public v1 option configuration name.
type LoggerConfig = loggerOption

// LoggerOption preserves the public v1 option function name.
type LoggerOption = LoggerOptFunc

// NewLogger preserves the existing interface-returning factory contract.
func NewLogger(opts ...LoggerOptFunc) (LoggerItf, error) {
	logger, err := newConcreteLogger(opts...)
	if err != nil {
		return nil, err
	}
	return logger, nil
}

// NewConcreteLogger builds the same logger with a concrete static return type.
func NewConcreteLogger(opts ...LoggerOptFunc) (*LoggerType, error) { return newConcreteLogger(opts...) }

// NewConcreteLoggerWithName builds a concrete named JSON logger.
func NewConcreteLoggerWithName(name, level string, opts ...zap.Option) (*LoggerType, error) {
	return NewConcreteLogger(WithLoggerName(name), WithLoggerEncoding(LoggerEncodingJSON), WithLoggerLevel(level), WithLoggerZapOptions(opts...))
}

// NewConcreteConsoleLoggerWithName builds a concrete named console logger.
func NewConcreteConsoleLoggerWithName(name, level string, opts ...zap.Option) (*LoggerType, error) {
	return NewConcreteLogger(WithLoggerName(name), WithLoggerEncoding(LoggerEncodingConsole), WithLoggerLevel(level), WithLoggerZapOptions(opts...))
}

// CreateNewDefaultConcreteLogger sets the interface default and returns its concrete value.
func CreateNewDefaultConcreteLogger(name, level string, opts ...zap.Option) (*LoggerType, error) {
	logger, err := NewConcreteLoggerWithName(name, level, opts...)
	if err != nil {
		return nil, err
	}
	Logger = logger
	return logger, nil
}

// CloneConcrete clones a concrete logger while sharing its dynamic level.
func (l *LoggerType) CloneConcrete() *LoggerType {
	return &LoggerType{Logger: l.Logger.With(), level: l.level}
}

// NamedConcrete creates a named concrete child.
func (l *LoggerType) NamedConcrete(name string) *LoggerType {
	return &LoggerType{Logger: l.Logger.Named(name), level: l.level}
}

// WithConcrete adds fields to a concrete child without mutating its parent.
func (l *LoggerType) WithConcrete(fields ...zapcore.Field) *LoggerType {
	return &LoggerType{Logger: l.Logger.With(fields...), level: l.level}
}

// WithOptionsConcrete applies options to a concrete child.
func (l *LoggerType) WithOptionsConcrete(opts ...zap.Option) *LoggerType {
	return &LoggerType{Logger: l.Logger.WithOptions(opts...), level: l.level}
}
