package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorCodes(t *testing.T) {
	codes := []ErrorCode{
		CodeUnknown,
		CodeValidation,
		CodeConfiguration,
		CodeTimeout,
		CodeCanceled,
		CodePermission,
		CodeNetworkUnreachable,
		CodeHostUnreachable,
		CodePortClosed,
		CodeScanFailed,
		CodeDiscoveryFailed,
		CodeTargetInvalid,
		CodeDatabaseConnection,
		CodeDatabaseQuery,
		CodeDatabaseMigration,
		CodeDatabaseTimeout,
		CodeFileNotFound,
		CodeFilePermission,
		CodeDirectoryCreate,
		CodeServiceUnavailable,
		CodeServiceTimeout,
		CodeRateLimited,
		CodeNotFound,
		CodeConflict,
	}

	for _, code := range codes {
		if string(code) == "" {
			t.Errorf("Error code %v should not be empty", code)
		}
	}
}

func TestScanError(t *testing.T) {
	t.Run("basic error creation", func(t *testing.T) {
		err := NewScanError(CodeScanFailed, "scan failed")
		if err.Code != CodeScanFailed {
			t.Errorf("Expected code %s, got %s", CodeScanFailed, err.Code)
		}
		if err.Message != "scan failed" {
			t.Errorf("Expected message 'scan failed', got '%s'", err.Message)
		}
		if err.Context == nil {
			t.Error("Context should be initialized")
		}
	})

	t.Run("error with target", func(t *testing.T) {
		err := NewScanErrorWithTarget(CodeHostUnreachable, "host down", "192.168.1.1")
		if err.Target != "192.168.1.1" {
			t.Errorf("Expected target '192.168.1.1', got '%s'", err.Target)
		}
		expected := "[HOST_UNREACHABLE] host down (target: 192.168.1.1)"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("error without target", func(t *testing.T) {
		err := NewScanError(CodeValidation, "validation failed")
		expected := "[VALIDATION] validation failed"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("wrapped error", func(t *testing.T) {
		cause := fmt.Errorf("network error")
		err := WrapScanError(CodeNetworkUnreachable, "network issue", cause)
		if err.Unwrap() != cause {
			t.Error("Wrapped error should be unwrappable")
		}
		if err.Cause != cause {
			t.Error("Cause should be set correctly")
		}
	})

	t.Run("wrapped error with target", func(t *testing.T) {
		cause := fmt.Errorf("connection refused")
		err := WrapScanErrorWithTarget(CodeHostUnreachable, "cannot connect", "example.com", cause)
		if err.Target != "example.com" {
			t.Errorf("Expected target 'example.com', got '%s'", err.Target)
		}
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})

	t.Run("with context", func(t *testing.T) {
		err := NewScanError(CodeTimeout, "timeout occurred")
		err.WithContext("duration", "30s").WithContext("retries", 3)

		if err.Context["duration"] != "30s" {
			t.Errorf("Expected duration '30s', got %v", err.Context["duration"])
		}
		if err.Context["retries"] != 3 {
			t.Errorf("Expected retries 3, got %v", err.Context["retries"])
		}
	})
}

func TestDatabaseError(t *testing.T) {
	t.Run("basic database error", func(t *testing.T) {
		err := NewDatabaseError(CodeDatabaseConnection, "connection failed")
		if err.Code != CodeDatabaseConnection {
			t.Errorf("Expected code %s, got %s", CodeDatabaseConnection, err.Code)
		}
		expected := "[DATABASE_CONNECTION] connection failed"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("database error with operation", func(t *testing.T) {
		err := NewDatabaseError(CodeDatabaseQuery, "query failed")
		err.Operation = "SELECT"
		expected := "[DATABASE_QUERY] query failed (operation: SELECT)"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("wrapped database error", func(t *testing.T) {
		cause := fmt.Errorf("connection timeout")
		err := WrapDatabaseError(CodeDatabaseTimeout, "timeout error", cause)
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})

	t.Run("with query", func(t *testing.T) {
		err := NewDatabaseError(CodeDatabaseQuery, "query failed")
		query := "SELECT * FROM hosts"
		err.WithQuery(query)
		if err.Query != query {
			t.Errorf("Expected query '%s', got '%s'", query, err.Query)
		}
	})
}

func TestDiscoveryError(t *testing.T) {
	t.Run("basic discovery error", func(t *testing.T) {
		err := NewDiscoveryError(CodeDiscoveryFailed, "discovery failed")
		if err.Code != CodeDiscoveryFailed {
			t.Errorf("Expected code %s, got %s", CodeDiscoveryFailed, err.Code)
		}
		expected := "[DISCOVERY_FAILED] discovery failed"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("discovery error with network", func(t *testing.T) {
		err := NewDiscoveryError(CodeNetworkUnreachable, "network unreachable")
		err.Network = "192.168.1.0/24"
		expected := "[NETWORK_UNREACHABLE] network unreachable (network: 192.168.1.0/24)"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("wrapped discovery error", func(t *testing.T) {
		cause := fmt.Errorf("ping failed")
		err := WrapDiscoveryError(CodeDiscoveryFailed, "ping discovery failed", cause)
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})
}

func TestConfigError(t *testing.T) {
	t.Run("basic config error", func(t *testing.T) {
		err := NewConfigError(CodeConfiguration, "config invalid")
		if err.Code != CodeConfiguration {
			t.Errorf("Expected code %s, got %s", CodeConfiguration, err.Code)
		}
		expected := "[CONFIGURATION] config invalid"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("config field error", func(t *testing.T) {
		err := NewConfigFieldError(CodeValidation, "invalid port", "database.port", 65536)
		if err.Field != "database.port" {
			t.Errorf("Expected field 'database.port', got '%s'", err.Field)
		}
		if err.Value != 65536 {
			t.Errorf("Expected value 65536, got %v", err.Value)
		}
		expected := "[VALIDATION] invalid port (field: database.port)"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("wrapped config error", func(t *testing.T) {
		cause := fmt.Errorf("file not found")
		err := WrapConfigError(CodeFileNotFound, "config file missing", cause)
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})
}

func TestUtilityFunctions(t *testing.T) {
	t.Run("IsCode", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			code     ErrorCode
			expected bool
		}{
			{
				name:     "scan error matches",
				err:      NewScanError(CodeTimeout, "timeout"),
				code:     CodeTimeout,
				expected: true,
			},
			{
				name:     "scan error does not match",
				err:      NewScanError(CodeTimeout, "timeout"),
				code:     CodeValidation,
				expected: false,
			},
			{
				name:     "database error matches",
				err:      NewDatabaseError(CodeDatabaseConnection, "connection failed"),
				code:     CodeDatabaseConnection,
				expected: true,
			},
			{
				name:     "discovery error matches",
				err:      NewDiscoveryError(CodeDiscoveryFailed, "discovery failed"),
				code:     CodeDiscoveryFailed,
				expected: true,
			},
			{
				name:     "config error matches",
				err:      NewConfigError(CodeConfiguration, "config error"),
				code:     CodeConfiguration,
				expected: true,
			},
			{
				name:     "standard error",
				err:      fmt.Errorf("standard error"),
				code:     CodeUnknown,
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := IsCode(tt.err, tt.code)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("GetCode", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected ErrorCode
		}{
			{
				name:     "scan error",
				err:      NewScanError(CodeTimeout, "timeout"),
				expected: CodeTimeout,
			},
			{
				name:     "database error",
				err:      NewDatabaseError(CodeDatabaseConnection, "connection failed"),
				expected: CodeDatabaseConnection,
			},
			{
				name:     "discovery error",
				err:      NewDiscoveryError(CodeDiscoveryFailed, "discovery failed"),
				expected: CodeDiscoveryFailed,
			},
			{
				name:     "config error",
				err:      NewConfigError(CodeConfiguration, "config error"),
				expected: CodeConfiguration,
			},
			{
				name:     "standard error",
				err:      fmt.Errorf("standard error"),
				expected: CodeUnknown,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := GetCode(tt.err)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("IsNotFound", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected bool
		}{
			{
				name:     "not found error",
				err:      NewScanError(CodeNotFound, "not found"),
				expected: true,
			},
			{
				name:     "file not found error",
				err:      NewScanError(CodeFileNotFound, "file not found"),
				expected: true,
			},
			{
				name:     "other error",
				err:      NewScanError(CodeTimeout, "timeout"),
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := IsNotFound(tt.err)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("IsConflict", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected bool
		}{
			{
				name:     "conflict error",
				err:      NewScanError(CodeConflict, "conflict"),
				expected: true,
			},
			{
				name:     "other error",
				err:      NewScanError(CodeTimeout, "timeout"),
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := IsConflict(tt.err)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("IsRetryable", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected bool
		}{
			{
				name:     "timeout error",
				err:      NewScanError(CodeTimeout, "timeout"),
				expected: true,
			},
			{
				name:     "network unreachable error",
				err:      NewScanError(CodeNetworkUnreachable, "network unreachable"),
				expected: true,
			},
			{
				name:     "service timeout error",
				err:      NewScanError(CodeServiceTimeout, "service timeout"),
				expected: true,
			},
			{
				name:     "database timeout error",
				err:      NewDatabaseError(CodeDatabaseTimeout, "db timeout"),
				expected: true,
			},
			{
				name:     "permission error",
				err:      NewScanError(CodePermission, "permission denied"),
				expected: false,
			},
			{
				name:     "validation error",
				err:      NewScanError(CodeValidation, "validation failed"),
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := IsRetryable(tt.err)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})

	t.Run("IsFatal", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			expected bool
		}{
			{
				name:     "permission error",
				err:      NewScanError(CodePermission, "permission denied"),
				expected: true,
			},
			{
				name:     "configuration error",
				err:      NewConfigError(CodeConfiguration, "config error"),
				expected: true,
			},
			{
				name:     "database migration error",
				err:      NewDatabaseError(CodeDatabaseMigration, "migration failed"),
				expected: true,
			},
			{
				name:     "timeout error",
				err:      NewScanError(CodeTimeout, "timeout"),
				expected: false,
			},
			{
				name:     "validation error",
				err:      NewScanError(CodeValidation, "validation failed"),
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := IsFatal(tt.err)
				if result != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, result)
				}
			})
		}
	})
}

func TestCommonErrorCreationFunctions(t *testing.T) {
	t.Run("ErrInvalidTarget", func(t *testing.T) {
		err := ErrInvalidTarget("invalid-target")
		if err.Code != CodeTargetInvalid {
			t.Errorf("Expected code %s, got %s", CodeTargetInvalid, err.Code)
		}
		if err.Target != "invalid-target" {
			t.Errorf("Expected target 'invalid-target', got '%s'", err.Target)
		}
	})

	t.Run("ErrScanTimeout", func(t *testing.T) {
		err := ErrScanTimeout("192.168.1.1")
		if err.Code != CodeTimeout {
			t.Errorf("Expected code %s, got %s", CodeTimeout, err.Code)
		}
		if err.Target != "192.168.1.1" {
			t.Errorf("Expected target '192.168.1.1', got '%s'", err.Target)
		}
	})

	t.Run("ErrHostUnreachable", func(t *testing.T) {
		err := ErrHostUnreachable("example.com")
		if err.Code != CodeHostUnreachable {
			t.Errorf("Expected code %s, got %s", CodeHostUnreachable, err.Code)
		}
		if err.Target != "example.com" {
			t.Errorf("Expected target 'example.com', got '%s'", err.Target)
		}
	})

	t.Run("ErrDatabaseConnection", func(t *testing.T) {
		cause := fmt.Errorf("connection refused")
		err := ErrDatabaseConnection(cause)
		if err.Code != CodeDatabaseConnection {
			t.Errorf("Expected code %s, got %s", CodeDatabaseConnection, err.Code)
		}
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})

	t.Run("ErrDatabaseQuery", func(t *testing.T) {
		cause := fmt.Errorf("syntax error")
		query := "SELECT * FROM invalid_table"
		err := ErrDatabaseQuery(query, cause)
		if err.Code != CodeDatabaseQuery {
			t.Errorf("Expected code %s, got %s", CodeDatabaseQuery, err.Code)
		}
		if err.Query != query {
			t.Errorf("Expected query '%s', got '%s'", query, err.Query)
		}
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})

	t.Run("ErrDiscoveryFailed", func(t *testing.T) {
		cause := fmt.Errorf("network error")
		err := ErrDiscoveryFailed("192.168.1.0/24", cause)
		if err.Code != CodeDiscoveryFailed {
			t.Errorf("Expected code %s, got %s", CodeDiscoveryFailed, err.Code)
		}
		if err.Unwrap() != cause {
			t.Error("Should unwrap to original error")
		}
	})

	t.Run("ErrConfigInvalid", func(t *testing.T) {
		err := ErrConfigInvalid("port", 65536)
		if err.Code != CodeValidation {
			t.Errorf("Expected code %s, got %s", CodeValidation, err.Code)
		}
		if err.Field != "port" {
			t.Errorf("Expected field 'port', got '%s'", err.Field)
		}
		if err.Value != 65536 {
			t.Errorf("Expected value 65536, got %v", err.Value)
		}
	})

	t.Run("ErrConfigMissing", func(t *testing.T) {
		err := ErrConfigMissing("database.host")
		if err.Code != CodeConfiguration {
			t.Errorf("Expected code %s, got %s", CodeConfiguration, err.Code)
		}
		if err.Field != "database.host" {
			t.Errorf("Expected field 'database.host', got '%s'", err.Field)
		}
		if err.Value != nil {
			t.Errorf("Expected value nil, got %v", err.Value)
		}
	})

	t.Run("ErrNotFound", func(t *testing.T) {
		err := ErrNotFound("host")
		if err.Code != CodeNotFound {
			t.Errorf("Expected code %s, got %s", CodeNotFound, err.Code)
		}
		expected := "[NOT_FOUND] host not found"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrNotFoundWithID", func(t *testing.T) {
		err := ErrNotFoundWithID("host", "123")
		if err.Code != CodeNotFound {
			t.Errorf("Expected code %s, got %s", CodeNotFound, err.Code)
		}
		expected := "[NOT_FOUND] host with ID 123 not found"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrConflict", func(t *testing.T) {
		err := ErrConflict("host")
		if err.Code != CodeConflict {
			t.Errorf("Expected code %s, got %s", CodeConflict, err.Code)
		}
		expected := "[CONFLICT] host already exists or conflict detected"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrConflictWithReason", func(t *testing.T) {
		err := ErrConflictWithReason("host", "duplicate IP address")
		if err.Code != CodeConflict {
			t.Errorf("Expected code %s, got %s", CodeConflict, err.Code)
		}
		expected := "[CONFLICT] host conflict: duplicate IP address"
		if err.Error() != expected {
			t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
		}
	})
}

func TestErrorUnwrapping(t *testing.T) {
	t.Run("nested error unwrapping", func(t *testing.T) {
		baseErr := fmt.Errorf("base error")
		wrappedErr := fmt.Errorf("wrapped: %w", baseErr)
		scanErr := WrapScanError(CodeScanFailed, "scan failed", wrappedErr)

		// Test direct unwrapping
		if scanErr.Unwrap() != wrappedErr {
			t.Error("Should unwrap to wrapped error")
		}

		// Test errors.Is for nested unwrapping
		if !errors.Is(scanErr, baseErr) {
			t.Error("Should be able to find base error using errors.Is")
		}
	})

	t.Run("nil unwrap", func(t *testing.T) {
		err := NewScanError(CodeValidation, "validation error")
		if err.Unwrap() != nil {
			t.Error("Error without cause should unwrap to nil")
		}
	})
}

func TestErrorChaining(t *testing.T) {
	t.Run("multiple context additions", func(t *testing.T) {
		err := NewScanError(CodeTimeout, "timeout occurred")

		// Chain multiple context additions
		err.WithContext("step", "1").
			WithContext("retry", true).
			WithContext("duration", "30s")

		if err.Context["step"] != "1" {
			t.Errorf("Expected step '1', got %v", err.Context["step"])
		}
		if err.Context["retry"] != true {
			t.Errorf("Expected retry true, got %v", err.Context["retry"])
		}
		if err.Context["duration"] != "30s" {
			t.Errorf("Expected duration '30s', got %v", err.Context["duration"])
		}
	})

	t.Run("overwrite context value", func(t *testing.T) {
		err := NewScanError(CodeValidation, "validation error")
		err.WithContext("key", "value1")
		err.WithContext("key", "value2")

		if err.Context["key"] != "value2" {
			t.Errorf("Expected overwritten value 'value2', got %v", err.Context["key"])
		}
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("scan error implements error interface", func(t *testing.T) {
		var err error = NewScanError(CodeValidation, "test")
		if err.Error() == "" {
			t.Error("Error should implement error interface")
		}
	})

	t.Run("database error implements error interface", func(t *testing.T) {
		var err error = NewDatabaseError(CodeDatabaseConnection, "test")
		if err.Error() == "" {
			t.Error("DatabaseError should implement error interface")
		}
	})

	t.Run("discovery error implements error interface", func(t *testing.T) {
		var err error = NewDiscoveryError(CodeDiscoveryFailed, "test")
		if err.Error() == "" {
			t.Error("DiscoveryError should implement error interface")
		}
	})

	t.Run("config error implements error interface", func(t *testing.T) {
		var err error = NewConfigError(CodeConfiguration, "test")
		if err.Error() == "" {
			t.Error("ConfigError should implement error interface")
		}
	})
}

func TestNilErrorHandling(t *testing.T) {
	t.Run("IsCode with nil error", func(t *testing.T) {
		result := IsCode(nil, CodeTimeout)
		if result {
			t.Error("IsCode should return false for nil error")
		}
	})

	t.Run("GetCode with nil error", func(t *testing.T) {
		result := GetCode(nil)
		if result != CodeUnknown {
			t.Errorf("Expected CodeUnknown for nil error, got %s", result)
		}
	})

	t.Run("IsNotFound with nil error", func(t *testing.T) {
		result := IsNotFound(nil)
		if result {
			t.Error("IsNotFound should return false for nil error")
		}
	})

	t.Run("IsConflict with nil error", func(t *testing.T) {
		result := IsConflict(nil)
		if result {
			t.Error("IsConflict should return false for nil error")
		}
	})

	t.Run("IsRetryable with nil error", func(t *testing.T) {
		result := IsRetryable(nil)
		if result {
			t.Error("IsRetryable should return false for nil error")
		}
	})

	t.Run("IsFatal with nil error", func(t *testing.T) {
		result := IsFatal(nil)
		if result {
			t.Error("IsFatal should return false for nil error")
		}
	})
}

func TestErrorFormatting(t *testing.T) {
	t.Run("scan error with all fields", func(t *testing.T) {
		cause := fmt.Errorf("network timeout")
		err := WrapScanErrorWithTarget(CodeTimeout, "operation timed out", "192.168.1.1", cause)
		err.Operation = "port_scan"
		err.WithContext("duration", "30s")

		errorStr := err.Error()
		expected := "[TIMEOUT] operation timed out (target: 192.168.1.1)"
		if errorStr != expected {
			t.Errorf("Expected '%s', got '%s'", expected, errorStr)
		}
	})

	t.Run("database error formatting", func(t *testing.T) {
		err := NewDatabaseError(CodeDatabaseQuery, "syntax error in query")
		err.Operation = "SELECT"
		err.WithQuery("SELECT * FROM invalid_table")

		errorStr := err.Error()
		expected := "[DATABASE_QUERY] syntax error in query (operation: SELECT)"
		if errorStr != expected {
			t.Errorf("Expected '%s', got '%s'", expected, errorStr)
		}
	})

	t.Run("discovery error formatting", func(t *testing.T) {
		err := NewDiscoveryError(CodeNetworkUnreachable, "network scan failed")
		err.Network = "10.0.0.0/8"
		err.Method = "ping"

		errorStr := err.Error()
		expected := "[NETWORK_UNREACHABLE] network scan failed (network: 10.0.0.0/8)"
		if errorStr != expected {
			t.Errorf("Expected '%s', got '%s'", expected, errorStr)
		}
	})

	t.Run("config error formatting", func(t *testing.T) {
		err := NewConfigFieldError(CodeValidation, "invalid value", "database.port", 70000)

		errorStr := err.Error()
		expected := "[VALIDATION] invalid value (field: database.port)"
		if errorStr != expected {
			t.Errorf("Expected '%s', got '%s'", expected, errorStr)
		}
	})
}

func TestBenchmarkErrorCreation(t *testing.T) {
	b := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := NewScanError(CodeTimeout, "benchmark test")
			err.WithContext("iteration", i)
		}
	})

	if b.NsPerOp() > 1000 { // Should be very fast
		t.Logf("Error creation took %d ns/op", b.NsPerOp())
	}
}
