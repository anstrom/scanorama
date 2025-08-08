// Package errors provides structured error handling for scanorama operations.
// It defines error codes, error types, and provides utilities for creating
// and handling errors with context and structured information.
package errors

import (
	"fmt"
)

// ErrorCode represents different types of errors that can occur.
type ErrorCode string

const (
	// General errors.
	CodeUnknown       ErrorCode = "UNKNOWN"
	CodeValidation    ErrorCode = "VALIDATION"
	CodeConfiguration ErrorCode = "CONFIGURATION"
	CodeTimeout       ErrorCode = "TIMEOUT"
	CodeCanceled      ErrorCode = "CANCELED"
	CodePermission    ErrorCode = "PERMISSION"

	// Network and scanning errors.
	CodeNetworkUnreachable ErrorCode = "NETWORK_UNREACHABLE"
	CodeHostUnreachable    ErrorCode = "HOST_UNREACHABLE"
	CodePortClosed         ErrorCode = "PORT_CLOSED"
	CodeScanFailed         ErrorCode = "SCAN_FAILED"
	CodeDiscoveryFailed    ErrorCode = "DISCOVERY_FAILED"
	CodeTargetInvalid      ErrorCode = "TARGET_INVALID"

	// Database errors.
	CodeDatabaseConnection ErrorCode = "DATABASE_CONNECTION"
	CodeDatabaseQuery      ErrorCode = "DATABASE_QUERY"
	CodeDatabaseMigration  ErrorCode = "DATABASE_MIGRATION"
	CodeDatabaseTimeout    ErrorCode = "DATABASE_TIMEOUT"

	// File system errors.
	CodeFileNotFound    ErrorCode = "FILE_NOT_FOUND"
	CodeFilePermission  ErrorCode = "FILE_PERMISSION"
	CodeDirectoryCreate ErrorCode = "DIRECTORY_CREATE"

	// Service errors.
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeServiceTimeout     ErrorCode = "SERVICE_TIMEOUT"
	CodeRateLimited        ErrorCode = "RATE_LIMITED"
)

// ScanError represents an error that occurred during scanning operations.
type ScanError struct {
	Code      ErrorCode
	Message   string
	Target    string
	Operation string
	Cause     error
	Context   map[string]interface{}
}

// Error implements the error interface.
func (e *ScanError) Error() string {
	if e.Target != "" {
		return fmt.Sprintf("[%s] %s (target: %s)", e.Code, e.Message, e.Target)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for error unwrapping.
func (e *ScanError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information to the error.
func (e *ScanError) WithContext(key string, value interface{}) *ScanError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewScanError creates a new scan error with the specified code and message.
func NewScanError(code ErrorCode, message string) *ScanError {
	return &ScanError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// NewScanErrorWithTarget creates a scan error for a specific target.
func NewScanErrorWithTarget(code ErrorCode, message, target string) *ScanError {
	return &ScanError{
		Code:    code,
		Message: message,
		Target:  target,
		Context: make(map[string]interface{}),
	}
}

// WrapScanError wraps an existing error as a scan error.
func WrapScanError(code ErrorCode, message string, err error) *ScanError {
	return &ScanError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// WrapScanErrorWithTarget wraps an error with target information.
func WrapScanErrorWithTarget(code ErrorCode, message, target string, err error) *ScanError {
	return &ScanError{
		Code:    code,
		Message: message,
		Target:  target,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// DatabaseError represents database-related errors.
type DatabaseError struct {
	Code      ErrorCode
	Message   string
	Operation string
	Query     string
	Cause     error
	Context   map[string]interface{}
}

// Error implements the error interface.
func (e *DatabaseError) Error() string {
	if e.Operation != "" {
		return fmt.Sprintf("[%s] %s (operation: %s)", e.Code, e.Message, e.Operation)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *DatabaseError) Unwrap() error {
	return e.Cause
}

// WithQuery adds the SQL query that caused the error.
func (e *DatabaseError) WithQuery(query string) *DatabaseError {
	e.Query = query
	return e
}

// NewDatabaseError creates a new database error.
func NewDatabaseError(code ErrorCode, message string) *DatabaseError {
	return &DatabaseError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// WrapDatabaseError wraps an existing error as a database error.
func WrapDatabaseError(code ErrorCode, message string, err error) *DatabaseError {
	return &DatabaseError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// DiscoveryError represents network discovery errors.
type DiscoveryError struct {
	Code    ErrorCode
	Message string
	Network string
	Method  string
	Cause   error
	Context map[string]interface{}
}

// Error implements the error interface.
func (e *DiscoveryError) Error() string {
	if e.Network != "" {
		return fmt.Sprintf("[%s] %s (network: %s)", e.Code, e.Message, e.Network)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *DiscoveryError) Unwrap() error {
	return e.Cause
}

// NewDiscoveryError creates a new discovery error.
func NewDiscoveryError(code ErrorCode, message string) *DiscoveryError {
	return &DiscoveryError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// WrapDiscoveryError wraps an existing error as a discovery error.
func WrapDiscoveryError(code ErrorCode, message string, err error) *DiscoveryError {
	return &DiscoveryError{
		Code:    code,
		Message: message,
		Cause:   err,
		Context: make(map[string]interface{}),
	}
}

// ConfigError represents configuration-related errors.
type ConfigError struct {
	Code    ErrorCode
	Message string
	Field   string
	Value   interface{}
	Cause   error
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("[%s] %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a new configuration error.
func NewConfigError(code ErrorCode, message string) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
	}
}

// NewConfigFieldError creates a configuration error for a specific field.
func NewConfigFieldError(code ErrorCode, message, field string, value interface{}) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
		Field:   field,
		Value:   value,
	}
}

// WrapConfigError wraps an existing error as a configuration error.
func WrapConfigError(code ErrorCode, message string, err error) *ConfigError {
	return &ConfigError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// Utility functions for common error operations

// IsCode checks if an error has a specific error code.
func IsCode(err error, code ErrorCode) bool {
	switch e := err.(type) {
	case *ScanError:
		return e.Code == code
	case *DatabaseError:
		return e.Code == code
	case *DiscoveryError:
		return e.Code == code
	case *ConfigError:
		return e.Code == code
	}
	return false
}

// GetCode extracts the error code from an error if it has one.
func GetCode(err error) ErrorCode {
	switch e := err.(type) {
	case *ScanError:
		return e.Code
	case *DatabaseError:
		return e.Code
	case *DiscoveryError:
		return e.Code
	case *ConfigError:
		return e.Code
	}
	return CodeUnknown
}

// IsRetryable determines if an error indicates a retryable condition.
func IsRetryable(err error) bool {
	code := GetCode(err)
	switch code {
	case CodeTimeout, CodeNetworkUnreachable, CodeServiceTimeout, CodeDatabaseTimeout:
		return true
	default:
		return false
	}
}

// IsFatal determines if an error indicates a fatal condition that should stop execution.
func IsFatal(err error) bool {
	code := GetCode(err)
	switch code {
	case CodePermission, CodeConfiguration, CodeDatabaseMigration:
		return true
	default:
		return false
	}
}

// Common error creation functions

// ErrInvalidTarget creates an error for invalid scan targets.
func ErrInvalidTarget(target string) *ScanError {
	return NewScanErrorWithTarget(CodeTargetInvalid, "Invalid target specification", target)
}

// ErrScanTimeout creates an error for scan timeouts.
func ErrScanTimeout(target string) *ScanError {
	return NewScanErrorWithTarget(CodeTimeout, "Scan operation timed out", target)
}

// ErrHostUnreachable creates an error for unreachable hosts.
func ErrHostUnreachable(target string) *ScanError {
	return NewScanErrorWithTarget(CodeHostUnreachable, "Host is unreachable", target)
}

// ErrDatabaseConnection creates an error for database connection failures.
func ErrDatabaseConnection(err error) *DatabaseError {
	return WrapDatabaseError(CodeDatabaseConnection, "Failed to connect to database", err)
}

// ErrDatabaseQuery creates an error for database query failures.
func ErrDatabaseQuery(query string, err error) *DatabaseError {
	return WrapDatabaseError(CodeDatabaseQuery, "Database query failed", err).WithQuery(query)
}

// ErrDiscoveryFailed creates an error for discovery failures.
func ErrDiscoveryFailed(network string, err error) *DiscoveryError {
	return WrapDiscoveryError(CodeDiscoveryFailed, "Network discovery failed", err)
}

// ErrConfigInvalid creates an error for invalid configuration.
func ErrConfigInvalid(field string, value interface{}) *ConfigError {
	return NewConfigFieldError(CodeValidation, "Invalid configuration value", field, value)
}

// ErrConfigMissing creates an error for missing required configuration.
func ErrConfigMissing(field string) *ConfigError {
	return NewConfigFieldError(CodeConfiguration, "Required configuration field missing", field, nil)
}
