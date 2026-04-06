package logger

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a wrapper around zap.Logger with convenience methods.
type Logger struct {
	*zap.Logger
}

// Config holds logger configuration.
type Config struct {
	Level       string // debug, info, warn, error
	Development bool
}

// New creates a new Logger instance.
func New(cfg Config) (*Logger, error) {
	var zapCfg zap.Config

	if cfg.Development || cfg.Level == "debug" {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}

	switch cfg.Level {
	case "debug":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		zapCfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		zapCfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	zapCfg.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format(time.RFC3339))
	}

	zapLogger, err := zapCfg.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return &Logger{Logger: zapLogger}, nil
}

// WithComponent returns a child logger with a named component.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{Logger: l.Logger.Named(component)}
}

// WithError returns a logger with the error field attached.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{Logger: l.Logger.With(zap.Error(err))}
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Logger.Sugar().Infof(format, args...)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Logger.Sugar().Errorf(format, args...)
}

// Fatalf logs a formatted fatal message and terminates.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Logger.Sugar().Fatalf(format, args...)
}
