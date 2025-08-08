// Package logging provides structured logging functionality using Go's slog package.
// It supports both text and JSON output formats, configurable log levels,
// and context-aware logging for the scanorama application.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// File permissions for directories and log files.
	logDirPerm  = 0750
	logFilePerm = 0600
)

// LogLevel represents the available log levels.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogFormat represents the available log formats.
type LogFormat string

const (
	FormatText LogFormat = "text"
	FormatJSON LogFormat = "json"
)

// Config holds logging configuration.
type Config struct {
	Level     LogLevel  `yaml:"level" json:"level"`
	Format    LogFormat `yaml:"format" json:"format"`
	Output    string    `yaml:"output" json:"output"`
	AddSource bool      `yaml:"add_source" json:"add_source"`
}

// DefaultConfig returns a default logging configuration.
func DefaultConfig() Config {
	return Config{
		Level:     LevelInfo,
		Format:    FormatText,
		Output:    "stdout",
		AddSource: false,
	}
}

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
	config Config
}

// New creates a new structured logger with the given configuration.
func New(cfg Config) (*Logger, error) {
	// Parse log level
	var level slog.Level
	switch strings.ToLower(string(cfg.Level)) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Determine output writer
	var writer io.Writer
	switch cfg.Output {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		// Assume it's a file path
		if err := os.MkdirAll(filepath.Dir(cfg.Output), logDirPerm); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFilePerm)
		if err != nil {
			return nil, err
		}
		writer = file
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	// Create handler based on format
	var handler slog.Handler
	switch cfg.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(writer, opts)
	default:
		handler = slog.NewTextHandler(writer, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
		config: cfg,
	}, nil
}

// NewDefault creates a logger with default configuration.
func NewDefault() *Logger {
	logger, _ := New(DefaultConfig())
	return logger
}

// WithContext adds context to the logger for structured logging.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{
		Logger: l.With(),
		config: l.config,
	}
}

// WithFields adds structured fields to the logger.
func (l *Logger) WithFields(fields ...any) *Logger {
	return &Logger{
		Logger: l.With(fields...),
		config: l.config,
	}
}

// WithComponent adds a component field to the logger.
func (l *Logger) WithComponent(component string) *Logger {
	return l.WithFields("component", component)
}

// WithScanID adds a scan ID field to the logger.
func (l *Logger) WithScanID(scanID string) *Logger {
	return l.WithFields("scan_id", scanID)
}

// WithTarget adds a target field to the logger.
func (l *Logger) WithTarget(target string) *Logger {
	return l.WithFields("target", target)
}

// WithError adds an error field to the logger.
func (l *Logger) WithError(err error) *Logger {
	return l.WithFields("error", err)
}

// InfoScan logs scan-related information.
func (l *Logger) InfoScan(msg, target string, fields ...any) {
	allFields := append([]any{"target", target}, fields...)
	l.Info(msg, allFields...)
}

// ErrorScan logs scan-related errors.
func (l *Logger) ErrorScan(msg, target string, err error, fields ...any) {
	allFields := append([]any{"target", target, "error", err}, fields...)
	l.Error(msg, allFields...)
}

// InfoDiscovery logs discovery-related information.
func (l *Logger) InfoDiscovery(msg, network string, fields ...any) {
	allFields := append([]any{"network", network}, fields...)
	l.Info(msg, allFields...)
}

// ErrorDiscovery logs discovery-related errors.
func (l *Logger) ErrorDiscovery(msg, network string, err error, fields ...any) {
	allFields := append([]any{"network", network, "error", err}, fields...)
	l.Error(msg, allFields...)
}

// InfoDatabase logs database-related information.
func (l *Logger) InfoDatabase(msg string, fields ...any) {
	allFields := append([]any{"component", "database"}, fields...)
	l.Info(msg, allFields...)
}

// ErrorDatabase logs database-related errors.
func (l *Logger) ErrorDatabase(msg string, err error, fields ...any) {
	allFields := append([]any{"component", "database", "error", err}, fields...)
	l.Error(msg, allFields...)
}

// InfoDaemon logs daemon-related information.
func (l *Logger) InfoDaemon(msg string, fields ...any) {
	allFields := append([]any{"component", "daemon"}, fields...)
	l.Info(msg, allFields...)
}

// ErrorDaemon logs daemon-related errors.
func (l *Logger) ErrorDaemon(msg string, err error, fields ...any) {
	allFields := append([]any{"component", "daemon", "error", err}, fields...)
	l.Error(msg, allFields...)
}

// Global logger instance - can be replaced for testing.
var defaultLogger = NewDefault()

// SetDefault sets the default logger instance.
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// Default returns the default logger instance.
func Default() *Logger {
	return defaultLogger
}

// Debug logs at debug level using the default logger.
func Debug(msg string, fields ...any) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs at info level using the default logger.
func Info(msg string, fields ...any) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs at warn level using the default logger.
func Warn(msg string, fields ...any) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs at error level using the default logger.
func Error(msg string, fields ...any) {
	defaultLogger.Error(msg, fields...)
}

// InfoScan logs scan-related information using the default logger.
func InfoScan(msg, target string, fields ...any) {
	defaultLogger.InfoScan(msg, target, fields...)
}

// ErrorScan logs scan-related errors using the default logger.
func ErrorScan(msg, target string, err error, fields ...any) {
	defaultLogger.ErrorScan(msg, target, err, fields...)
}

// InfoDiscovery logs discovery-related information using the default logger.
func InfoDiscovery(msg, network string, fields ...any) {
	defaultLogger.InfoDiscovery(msg, network, fields...)
}

// ErrorDiscovery logs discovery-related errors using the default logger.
func ErrorDiscovery(msg, network string, err error, fields ...any) {
	defaultLogger.ErrorDiscovery(msg, network, err, fields...)
}

// InfoDatabase logs database-related information using the default logger.
func InfoDatabase(msg string, fields ...any) {
	defaultLogger.InfoDatabase(msg, fields...)
}

// ErrorDatabase logs database-related errors using the default logger.
func ErrorDatabase(msg string, err error, fields ...any) {
	defaultLogger.ErrorDatabase(msg, err, fields...)
}

// InfoDaemon logs daemon-related information using the default logger.
func InfoDaemon(msg string, fields ...any) {
	defaultLogger.InfoDaemon(msg, fields...)
}

// ErrorDaemon logs daemon-related errors using the default logger.
func ErrorDaemon(msg string, err error, fields ...any) {
	defaultLogger.ErrorDaemon(msg, err, fields...)
}
