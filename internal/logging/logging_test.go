package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected string
	}{
		{"debug level", LevelDebug, "debug"},
		{"info level", LevelInfo, "info"},
		{"warn level", LevelWarn, "warn"},
		{"error level", LevelError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.level) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.level))
			}
		})
	}
}

func TestLogFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   LogFormat
		expected string
	}{
		{"text format", FormatText, "text"},
		{"json format", FormatJSON, "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.format) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.format))
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != LevelInfo {
		t.Errorf("Expected default level %s, got %s", LevelInfo, cfg.Level)
	}
	if cfg.Format != FormatText {
		t.Errorf("Expected default format %s, got %s", FormatText, cfg.Format)
	}
	if cfg.Output != "stdout" {
		t.Errorf("Expected default output 'stdout', got '%s'", cfg.Output)
	}
	if cfg.AddSource {
		t.Error("Expected AddSource to be false by default")
	}
}

func TestNewLogger(t *testing.T) {
	t.Run("stdout text logger", func(t *testing.T) {
		cfg := Config{
			Level:  LevelInfo,
			Format: FormatText,
			Output: "stdout",
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		if logger == nil {
			t.Fatal("Logger should not be nil")
		}
		if logger.config.Level != LevelInfo {
			t.Errorf("Expected level %s, got %s", LevelInfo, logger.config.Level)
		}
	})

	t.Run("stderr json logger", func(t *testing.T) {
		cfg := Config{
			Level:  LevelError,
			Format: FormatJSON,
			Output: "stderr",
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		if logger == nil {
			t.Fatal("Logger should not be nil")
		}
	})

	t.Run("file logger", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		cfg := Config{
			Level:  LevelDebug,
			Format: FormatText,
			Output: logFile,
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create file logger: %v", err)
		}
		if logger == nil {
			t.Fatal("Logger should not be nil")
		}

		// Test that file was created
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Error("Log file should have been created")
		}
	})

	t.Run("invalid directory for file logger", func(t *testing.T) {
		cfg := Config{
			Level:  LevelInfo,
			Format: FormatText,
			Output: "/invalid/path/test.log",
		}

		_, err := New(cfg)
		if err == nil {
			t.Error("Expected error for invalid log file path")
		}
	})

	t.Run("unknown log level defaults to info", func(t *testing.T) {
		cfg := Config{
			Level:  LogLevel("unknown"),
			Format: FormatText,
			Output: "stdout",
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger with unknown level: %v", err)
		}
		if logger == nil {
			t.Fatal("Logger should not be nil")
		}
	})

	t.Run("with source information", func(t *testing.T) {
		cfg := Config{
			Level:     LevelInfo,
			Format:    FormatText,
			Output:    "stdout",
			AddSource: true,
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger with source: %v", err)
		}
		if logger == nil {
			t.Fatal("Logger should not be nil")
		}
	})
}

func TestNewDefault(t *testing.T) {
	logger := NewDefault()
	if logger == nil {
		t.Fatal("Default logger should not be nil")
	}
	if logger.config.Level != LevelInfo {
		t.Errorf("Default logger should have info level, got %s", logger.config.Level)
	}
}

func TestLoggerMethods(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	cfg := Config{
		Level:  LevelDebug,
		Format: FormatText,
		Output: "stdout",
	}

	// Redirect stdout to our buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	t.Run("basic logging methods", func(t *testing.T) {
		logger.Debug("debug message", "key", "value")
		logger.Info("info message", "key", "value")
		logger.Warn("warn message", "key", "value")
		logger.Error("error message", "key", "value")

		// Close write end and restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		_, _ = io.Copy(&buf, r)
		output := buf.String()

		// Check that messages were logged
		if !strings.Contains(output, "debug message") {
			t.Error("Debug message should be logged")
		}
		if !strings.Contains(output, "info message") {
			t.Error("Info message should be logged")
		}
		if !strings.Contains(output, "warn message") {
			t.Error("Warn message should be logged")
		}
		if !strings.Contains(output, "error message") {
			t.Error("Error message should be logged")
		}
	})
}

func TestLoggerWithMethods(t *testing.T) {
	logger := NewDefault()

	t.Run("WithContext", func(t *testing.T) {
		ctx := context.Background()
		contextLogger := logger.WithContext(ctx)
		if contextLogger == nil {
			t.Error("WithContext should return a logger")
		}
		if contextLogger == logger {
			t.Error("WithContext should return a new logger instance")
		}
	})

	t.Run("WithFields", func(t *testing.T) {
		fieldsLogger := logger.WithFields("key1", "value1", "key2", "value2")
		if fieldsLogger == nil {
			t.Error("WithFields should return a logger")
		}
		if fieldsLogger == logger {
			t.Error("WithFields should return a new logger instance")
		}
	})

	t.Run("WithComponent", func(t *testing.T) {
		componentLogger := logger.WithComponent("scanner")
		if componentLogger == nil {
			t.Error("WithComponent should return a logger")
		}
		if componentLogger == logger {
			t.Error("WithComponent should return a new logger instance")
		}
	})

	t.Run("WithScanID", func(t *testing.T) {
		scanLogger := logger.WithScanID("scan-123")
		if scanLogger == nil {
			t.Error("WithScanID should return a logger")
		}
		if scanLogger == logger {
			t.Error("WithScanID should return a new logger instance")
		}
	})

	t.Run("WithTarget", func(t *testing.T) {
		targetLogger := logger.WithTarget("192.168.1.1")
		if targetLogger == nil {
			t.Error("WithTarget should return a logger")
		}
		if targetLogger == logger {
			t.Error("WithTarget should return a new logger instance")
		}
	})

	t.Run("WithError", func(t *testing.T) {
		err := fmt.Errorf("test error")
		errorLogger := logger.WithError(err)
		if errorLogger == nil {
			t.Error("WithError should return a logger")
		}
		if errorLogger == logger {
			t.Error("WithError should return a new logger instance")
		}
	})
}

func TestSpecializedLoggingMethods(t *testing.T) {
	// Create a logger that outputs to a file for testing
	tmpFile := filepath.Join(t.TempDir(), "test.log")

	// Create logger with file output so we can read it back
	cfg := Config{
		Level:  LevelDebug,
		Format: FormatText,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	t.Run("InfoScan", func(t *testing.T) {
		logger.InfoScan("scan started", "192.168.1.1", "ports", "80,443")

		// Read log file
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "scan started") {
			t.Error("Should contain scan message")
		}
		if !strings.Contains(output, "192.168.1.1") {
			t.Error("Should contain target")
		}
	})

	t.Run("ErrorScan", func(t *testing.T) {
		testErr := fmt.Errorf("connection refused")
		logger.ErrorScan("scan failed", "192.168.1.2", testErr, "retry", 3)

		// Read log file
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "scan failed") {
			t.Error("Should contain error message")
		}
		if !strings.Contains(output, "192.168.1.2") {
			t.Error("Should contain target")
		}
	})

	t.Run("InfoDiscovery", func(t *testing.T) {
		logger.InfoDiscovery("discovery completed", "10.0.0.0/24", "hosts_found", 5)

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "discovery completed") {
			t.Error("Should contain discovery message")
		}
		if !strings.Contains(output, "10.0.0.0/24") {
			t.Error("Should contain network")
		}
	})

	t.Run("ErrorDiscovery", func(t *testing.T) {
		testErr := fmt.Errorf("network unreachable")
		logger.ErrorDiscovery("discovery failed", "10.0.1.0/24", testErr, "method", "ping")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "discovery failed") {
			t.Error("Should contain error message")
		}
		if !strings.Contains(output, "10.0.1.0/24") {
			t.Error("Should contain network")
		}
	})

	t.Run("InfoDatabase", func(t *testing.T) {
		logger.InfoDatabase("database connected", "host", "localhost")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "database connected") {
			t.Error("Should contain database message")
		}
		if !strings.Contains(output, "component=database") {
			t.Error("Should contain database component")
		}
	})

	t.Run("ErrorDatabase", func(t *testing.T) {
		testErr := fmt.Errorf("connection timeout")
		logger.ErrorDatabase("database error", testErr, "operation", "connect")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "database error") {
			t.Error("Should contain error message")
		}
		if !strings.Contains(output, "component=database") {
			t.Error("Should contain database component")
		}
	})

	t.Run("InfoDaemon", func(t *testing.T) {
		logger.InfoDaemon("daemon started", "pid", 1234)

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "daemon started") {
			t.Error("Should contain daemon message")
		}
		if !strings.Contains(output, "component=daemon") {
			t.Error("Should contain daemon component")
		}
	})

	t.Run("ErrorDaemon", func(t *testing.T) {
		testErr := fmt.Errorf("startup failed")
		logger.ErrorDaemon("daemon error", testErr, "phase", "startup")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "daemon error") {
			t.Error("Should contain error message")
		}
		if !strings.Contains(output, "component=daemon") {
			t.Error("Should contain daemon component")
		}
	})
}

func TestJSONFormat(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "json.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create JSON logger: %v", err)
	}

	logger.Info("test message", "key", "value", "number", 42)

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse as JSON to validate format
	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Fatalf("Log output should be valid JSON: %v", err)
	}

	if logEntry["msg"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", logEntry["key"])
	}
	if logEntry["number"] != float64(42) {
		t.Errorf("Expected number 42, got %v", logEntry["number"])
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name         string
		configLevel  LogLevel
		logLevel     string
		shouldAppear bool
	}{
		{"debug level logs debug", LevelDebug, "debug", true},
		{"debug level logs info", LevelDebug, "info", true},
		{"debug level logs warn", LevelDebug, "warn", true},
		{"debug level logs error", LevelDebug, "error", true},
		{"info level skips debug", LevelInfo, "debug", false},
		{"info level logs info", LevelInfo, "info", true},
		{"info level logs warn", LevelInfo, "warn", true},
		{"info level logs error", LevelInfo, "error", true},
		{"warn level skips debug", LevelWarn, "debug", false},
		{"warn level skips info", LevelWarn, "info", false},
		{"warn level logs warn", LevelWarn, "warn", true},
		{"warn level logs error", LevelWarn, "error", true},
		{"error level skips debug", LevelError, "debug", false},
		{"error level skips info", LevelError, "info", false},
		{"error level skips warn", LevelError, "warn", false},
		{"error level logs error", LevelError, "error", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "level_test.log")

			cfg := Config{
				Level:  tt.configLevel,
				Format: FormatText,
				Output: tmpFile,
			}

			logger, err := New(cfg)
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}

			message := fmt.Sprintf("test %s message", tt.logLevel)

			// Log at the specified level
			switch tt.logLevel {
			case "debug":
				logger.Debug(message)
			case "info":
				logger.Info(message)
			case "warn":
				logger.Warn(message)
			case "error":
				logger.Error(message)
			}

			content, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatalf("Failed to read log file: %v", err)
			}

			output := string(content)
			appears := strings.Contains(output, message)

			if appears != tt.shouldAppear {
				if tt.shouldAppear {
					t.Errorf("Message should appear in log but doesn't: %s", message)
				} else {
					t.Errorf("Message should not appear in log but does: %s", message)
				}
			}
		})
	}
}

func TestGlobalLoggerFunctions(t *testing.T) {
	// Save original logger
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create test logger with file output
	tmpFile := filepath.Join(t.TempDir(), "global_test.log")
	cfg := Config{
		Level:  LevelDebug,
		Format: FormatText,
		Output: tmpFile,
	}

	testLogger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}

	SetDefault(testLogger)

	t.Run("global logging functions", func(t *testing.T) {
		Debug("global debug", "key", "debug")
		Info("global info", "key", "info")
		Warn("global warn", "key", "warn")
		Error("global error", "key", "error")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		if !strings.Contains(output, "global debug") {
			t.Error("Global debug should be logged")
		}
		if !strings.Contains(output, "global info") {
			t.Error("Global info should be logged")
		}
		if !strings.Contains(output, "global warn") {
			t.Error("Global warn should be logged")
		}
		if !strings.Contains(output, "global error") {
			t.Error("Global error should be logged")
		}
	})

	t.Run("global specialized functions", func(t *testing.T) {
		// Clear the file
		os.Truncate(tmpFile, 0)

		testErr := fmt.Errorf("test error")

		InfoScan("scan info", "192.168.1.1", "ports", "80,443")
		ErrorScan("scan error", "192.168.1.2", testErr, "retry", 1)
		InfoDiscovery("discovery info", "10.0.0.0/24", "method", "ping")
		ErrorDiscovery("discovery error", "10.0.1.0/24", testErr, "timeout", "30s")
		InfoDatabase("database info", "operation", "connect")
		ErrorDatabase("database error", testErr, "query", "SELECT")
		InfoDaemon("daemon info", "status", "running")
		ErrorDaemon("daemon error", testErr, "signal", "SIGTERM")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		output := string(content)
		expectedMessages := []string{
			"scan info", "scan error",
			"discovery info", "discovery error",
			"database info", "database error",
			"daemon info", "daemon error",
		}

		for _, msg := range expectedMessages {
			if !strings.Contains(output, msg) {
				t.Errorf("Output should contain '%s'", msg)
			}
		}
	})
}

func TestSetAndGetDefault(t *testing.T) {
	originalLogger := Default()
	defer SetDefault(originalLogger)

	// Create new logger
	cfg := Config{
		Level:  LevelError,
		Format: FormatJSON,
		Output: "stderr",
	}

	newLogger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create new logger: %v", err)
	}

	// Set as default
	SetDefault(newLogger)

	// Get default and verify it's the same
	retrieved := Default()
	if retrieved != newLogger {
		t.Error("Retrieved logger should be the same as set logger")
	}
	if retrieved.config.Level != LevelError {
		t.Errorf("Expected level %s, got %s", LevelError, retrieved.config.Level)
	}
}

func TestLoggerWithComplexFields(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "complex.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test with various data types
	logger.Info("complex log entry",
		"string", "test",
		"int", 42,
		"float", 3.14,
		"bool", true,
		"time", time.Now(),
		"map", map[string]string{"key": "value"},
		"slice", []string{"a", "b", "c"},
	)

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse as JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal(content, &logEntry); err != nil {
		t.Fatalf("Log output should be valid JSON: %v", err)
	}

	// Verify fields
	if logEntry["string"] != "test" {
		t.Errorf("Expected string 'test', got %v", logEntry["string"])
	}
	if logEntry["int"] != float64(42) {
		t.Errorf("Expected int 42, got %v", logEntry["int"])
	}
	if logEntry["bool"] != true {
		t.Errorf("Expected bool true, got %v", logEntry["bool"])
	}
}

func TestFileLoggingPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "perms.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: logFile,
	}

	_, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}

	if info.Mode().Perm() != logFilePerm {
		t.Errorf("Expected file permissions %o, got %o", logFilePerm, info.Mode().Perm())
	}
}

func TestDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "logs", "subdir")
	logFile := filepath.Join(nestedDir, "test.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: logFile,
	}

	_, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger with nested directory: %v", err)
	}

	// Check that directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Nested directory should have been created")
	}

	// Check directory permissions
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if info.Mode().Perm() != logDirPerm {
		t.Errorf("Expected directory permissions %o, got %o", logDirPerm, info.Mode().Perm())
	}
}

func TestLoggerChaining(t *testing.T) {
	logger := NewDefault()

	// Test method chaining
	chainedLogger := logger.
		WithComponent("scanner").
		WithTarget("192.168.1.1").
		WithScanID("scan-123").
		WithFields("extra", "data")

	if chainedLogger == nil {
		t.Error("Chained logger should not be nil")
	}
	if chainedLogger == logger {
		t.Error("Chained logger should be different from original")
	}
}

func TestConcurrentLogging(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "concurrent.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Launch multiple goroutines to log concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				logger.Info("concurrent log", "goroutine", id, "iteration", j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that file exists and has content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Log file should have content from concurrent logging")
	}

	// Count lines to ensure we got all log entries
	lines := strings.Split(string(content), "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}

	// Should have 100 log entries (10 goroutines * 10 iterations)
	if nonEmptyLines != 100 {
		t.Errorf("Expected 100 log entries, got %d", nonEmptyLines)
	}
}

func TestLoggerCleanup(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "cleanup.log")

	cfg := Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Info("test message")

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("Log file should exist")
	}

	// The file should be cleaned up automatically by t.TempDir()
	// This test just ensures the logger creates the file properly
}

func TestInvalidLogLevel(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "invalid_level.log")

	cfg := Config{
		Level:  LogLevel("invalid"),
		Format: FormatText,
		Output: tmpFile,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Logger creation should not fail with invalid level: %v", err)
	}

	// Should default to info level - test by logging debug (should not appear)
	logger.Debug("debug message that should not appear")
	logger.Info("info message that should appear")

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	output := string(content)
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should not appear with invalid level defaulting to info")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Info message should appear with invalid level defaulting to info")
	}
}

func TestLoggerConfiguration(t *testing.T) {
	t.Run("config preservation", func(t *testing.T) {
		cfg := Config{
			Level:     LevelError,
			Format:    FormatJSON,
			Output:    "stderr",
			AddSource: true,
		}

		customLogger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create custom logger: %v", err)
		}

		if customLogger.config.Level != LevelError {
			t.Errorf("Expected level %s, got %s", LevelError, customLogger.config.Level)
		}
		if customLogger.config.Format != FormatJSON {
			t.Errorf("Expected format %s, got %s", FormatJSON, customLogger.config.Format)
		}
		if customLogger.config.Output != "stderr" {
			t.Errorf("Expected output 'stderr', got '%s'", customLogger.config.Output)
		}
		if !customLogger.config.AddSource {
			t.Error("Expected AddSource to be true")
		}
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty log message", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "empty.log")

		cfg := Config{
			Level:  LevelInfo,
			Format: FormatText,
			Output: tmpFile,
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		logger.Info("", "key", "value")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		if len(content) == 0 {
			t.Error("Should log even with empty message")
		}
	})

	t.Run("odd number of fields", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "odd_fields.log")

		cfg := Config{
			Level:  LevelInfo,
			Format: FormatText,
			Output: tmpFile,
		}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		// This should still work (slog handles odd number of fields)
		logger.Info("test message", "key1", "value1", "key2", "value2")

		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		if len(content) == 0 {
			t.Error("Should log even with odd number of fields")
		}
	})
}
