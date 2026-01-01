package logging

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Level represents a logging level
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Format represents the log output format
type Format string

const (
	FormatJSON    Format = "json"
	FormatConsole Format = "console"
	FormatText    Format = "text" // alias for console
)

// Config holds logging configuration
type Config struct {
	Level      Level  `json:"level" yaml:"level"`
	Format     Format `json:"format" yaml:"format"`
	Output     string `json:"output" yaml:"output"` // stdout, stderr, or file path
	TimeFormat string `json:"time_format" yaml:"time_format"`
	// Additional options
	AddCaller      bool `json:"add_caller" yaml:"add_caller"`
	AddStacktrace  bool `json:"add_stacktrace" yaml:"add_stacktrace"`
	Development    bool `json:"development" yaml:"development"`
	DisableColor   bool `json:"disable_color" yaml:"disable_color"`
	SamplingConfig *SamplingConfig `json:"sampling" yaml:"sampling"`
}

// SamplingConfig configures log sampling for high-volume logs
type SamplingConfig struct {
	Enabled    bool `json:"enabled" yaml:"enabled"`
	Initial    int  `json:"initial" yaml:"initial"`       // log first N entries
	Thereafter int  `json:"thereafter" yaml:"thereafter"` // then log every Mth entry
}

// DefaultConfig returns a default logging configuration
func DefaultConfig() Config {
	return Config{
		Level:         LevelInfo,
		Format:        FormatJSON,
		Output:        "stdout",
		TimeFormat:    time.RFC3339,
		AddCaller:     true,
		AddStacktrace: false,
		Development:   false,
		DisableColor:  false,
		SamplingConfig: &SamplingConfig{
			Enabled:    false,
			Initial:    100,
			Thereafter: 100,
		},
	}
}

// Logger wraps zap.Logger with additional context support
type Logger struct {
	zap    *zap.Logger
	sugar  *zap.SugaredLogger
	config Config
}

var (
	globalLogger *Logger
	globalMu     sync.RWMutex
)

// NewLogger creates a new logger with the given configuration
func NewLogger(cfg Config) (*Logger, error) {
	level := parseLevel(cfg.Level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	if cfg.TimeFormat != "" {
		encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(cfg.TimeFormat)
	}

	if cfg.Format == FormatConsole || cfg.Format == FormatText {
		if !cfg.DisableColor {
			encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		} else {
			encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		}
	}

	var encoder zapcore.Encoder
	switch cfg.Format {
	case FormatJSON:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case FormatConsole, FormatText:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var output zapcore.WriteSyncer
	switch cfg.Output {
	case "stdout", "":
		output = zapcore.AddSync(os.Stdout)
	case "stderr":
		output = zapcore.AddSync(os.Stderr)
	default:
		file, err := os.OpenFile(cfg.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		output = zapcore.AddSync(file)
	}

	core := zapcore.NewCore(encoder, output, level)

	// Apply sampling if enabled
	if cfg.SamplingConfig != nil && cfg.SamplingConfig.Enabled {
		core = zapcore.NewSamplerWithOptions(
			core,
			time.Second,
			cfg.SamplingConfig.Initial,
			cfg.SamplingConfig.Thereafter,
		)
	}

	opts := []zap.Option{}
	if cfg.AddCaller {
		opts = append(opts, zap.AddCaller(), zap.AddCallerSkip(1))
	}
	if cfg.AddStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	if cfg.Development {
		opts = append(opts, zap.Development())
	}

	zapLogger := zap.New(core, opts...)

	return &Logger{
		zap:    zapLogger,
		sugar:  zapLogger.Sugar(),
		config: cfg,
	}, nil
}

// SetGlobal sets the global logger
func SetGlobal(l *Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

// Global returns the global logger
func Global() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalLogger == nil {
		l, _ := NewLogger(DefaultConfig())
		return l
	}
	return globalLogger
}

// With creates a child logger with additional fields
func (l *Logger) With(fields ...Field) *Logger {
	return &Logger{
		zap:    l.zap.With(fieldsToZap(fields)...),
		sugar:  l.sugar.With(fieldsToInterface(fields)...),
		config: l.config,
	}
}

// WithContext creates a logger with context fields
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := extractContextFields(ctx)
	if len(fields) == 0 {
		return l
	}
	return l.With(fields...)
}

// Named creates a named child logger
func (l *Logger) Named(name string) *Logger {
	return &Logger{
		zap:    l.zap.Named(name),
		sugar:  l.sugar.Named(name),
		config: l.config,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields ...Field) {
	l.zap.Debug(msg, fieldsToZap(fields)...)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields ...Field) {
	l.zap.Info(msg, fieldsToZap(fields)...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields ...Field) {
	l.zap.Warn(msg, fieldsToZap(fields)...)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields ...Field) {
	l.zap.Error(msg, fieldsToZap(fields)...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.zap.Fatal(msg, fieldsToZap(fields)...)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(template string, args ...interface{}) {
	l.sugar.Debugf(template, args...)
}

// Infof logs a formatted info message
func (l *Logger) Infof(template string, args ...interface{}) {
	l.sugar.Infof(template, args...)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(template string, args ...interface{}) {
	l.sugar.Warnf(template, args...)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(template string, args ...interface{}) {
	l.sugar.Errorf(template, args...)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(template string, args ...interface{}) {
	l.sugar.Fatalf(template, args...)
}

// Sync flushes any buffered log entries
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// Writer returns an io.Writer that writes to the logger at the specified level
func (l *Logger) Writer(level Level) io.Writer {
	return &logWriter{logger: l, level: level}
}

// Zap returns the underlying zap logger
func (l *Logger) Zap() *zap.Logger {
	return l.zap
}

// Sugar returns the sugared logger
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

type logWriter struct {
	logger *Logger
	level  Level
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	switch w.level {
	case LevelDebug:
		w.logger.Debug(msg)
	case LevelInfo:
		w.logger.Info(msg)
	case LevelWarn:
		w.logger.Warn(msg)
	case LevelError:
		w.logger.Error(msg)
	default:
		w.logger.Info(msg)
	}
	return len(p), nil
}

func parseLevel(level Level) zapcore.Level {
	switch level {
	case LevelDebug:
		return zapcore.DebugLevel
	case LevelInfo:
		return zapcore.InfoLevel
	case LevelWarn:
		return zapcore.WarnLevel
	case LevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
