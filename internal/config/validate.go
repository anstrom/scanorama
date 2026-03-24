// Package config provides configuration management for scanorama.
// This file provides centralized, thorough configuration validation
// that collects all errors and warnings rather than failing on the first one.
// It can be used from any entry point (CLI, API, config loading).
package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

// Severity represents the severity level of a validation issue.
type Severity int

const (
	// SeverityError indicates a hard validation error that must be fixed.
	SeverityError Severity = iota
	// SeverityWarning indicates a potential issue that should be reviewed.
	SeverityWarning
)

const (
	// outputStdout is the constant for the "stdout" output target.
	outputStdout = "stdout"
	// outputStderr is the constant for the "stderr" output target.
	outputStderr = "stderr"

	// largeWorkerPoolThreshold is the threshold above which
	// a worker pool size warning is emitted.
	largeWorkerPoolThreshold = 1000
)

// ValidationIssue represents a single validation error or warning.
type ValidationIssue struct {
	// Section is the config section where the issue was found
	// (e.g., "database", "api").
	Section string
	// Field is the specific field name within the section.
	Field string
	// Message describes the issue.
	Message string
	// Severity indicates whether this is an error or warning.
	Severity Severity
}

// Error implements the error interface for ValidationIssue.
func (v ValidationIssue) Error() string {
	prefix := "ERROR"
	if v.Severity == SeverityWarning {
		prefix = "WARNING"
	}
	if v.Field != "" {
		return fmt.Sprintf(
			"[%s] %s.%s: %s",
			prefix, v.Section, v.Field, v.Message,
		)
	}
	return fmt.Sprintf("[%s] %s: %s", prefix, v.Section, v.Message)
}

// ValidationResult holds the collected errors and warnings
// from validation.
type ValidationResult struct {
	// Errors contains hard validation errors.
	Errors []ValidationIssue
	// Warnings contains non-fatal validation warnings.
	Warnings []ValidationIssue
}

// HasErrors returns true if there are any validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any validation warnings.
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// IsValid returns true if there are no validation errors
// (warnings are OK).
func (r *ValidationResult) IsValid() bool {
	return !r.HasErrors()
}

// AllIssues returns all errors and warnings combined.
func (r *ValidationResult) AllIssues() []ValidationIssue {
	all := make(
		[]ValidationIssue,
		0,
		len(r.Errors)+len(r.Warnings),
	)
	all = append(all, r.Errors...)
	all = append(all, r.Warnings...)
	return all
}

// Error returns a combined error message from all validation
// errors, or empty string if valid.
func (r *ValidationResult) Error() string {
	if !r.HasErrors() {
		return ""
	}
	msgs := make([]string, 0, len(r.Errors))
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "; ")
}

// AsError returns the ValidationResult as an error if there are
// errors, or nil if valid.
func (r *ValidationResult) AsError() error {
	if r.IsValid() {
		return nil
	}
	return fmt.Errorf(
		"configuration validation failed: %s",
		r.Error(),
	)
}

// addError adds a validation error to the result.
func (r *ValidationResult) addError(
	section, field, message string,
) {
	r.Errors = append(r.Errors, ValidationIssue{
		Section:  section,
		Field:    field,
		Message:  message,
		Severity: SeverityError,
	})
}

// addWarning adds a validation warning to the result.
func (r *ValidationResult) addWarning(
	section, field, message string,
) {
	r.Warnings = append(r.Warnings, ValidationIssue{
		Section:  section,
		Field:    field,
		Message:  message,
		Severity: SeverityWarning,
	})
}

// merge appends all issues from another ValidationResult
// into this one.
func (r *ValidationResult) merge(other *ValidationResult) {
	if other == nil {
		return
	}
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}

// ValidateConfig runs all section validators on a full Config
// and collects all errors and warnings.
// This is the main entry point for comprehensive config validation.
func ValidateConfig(cfg *Config) *ValidationResult {
	result := &ValidationResult{}

	if cfg == nil {
		result.addError("config", "", "configuration is nil")
		return result
	}

	result.merge(ValidateDaemonConfig(&cfg.Daemon))
	result.merge(ValidateDatabaseConfig(&cfg.Database))
	result.merge(ValidateScanningConfig(&cfg.Scanning))
	result.merge(ValidateAPIConfig(&cfg.API))
	result.merge(ValidateLoggingConfig(&cfg.Logging))
	result.merge(ValidateDiscoveryConfig(&cfg.Discovery))

	return result
}

// ValidateAndNormalize validates the config and applies
// normalization (lowercasing, path cleaning, etc.).
// It modifies the config in place. Call this instead of
// ValidateConfig when you want the config to be cleaned up
// as part of validation.
func ValidateAndNormalize(cfg *Config) *ValidationResult {
	if cfg == nil {
		result := &ValidationResult{}
		result.addError("config", "", "configuration is nil")
		return result
	}

	normalizeConfig(cfg)
	return ValidateConfig(cfg)
}

// normalizeConfig applies normalization to the config values
// in place.
func normalizeConfig(cfg *Config) {
	normalizeDaemonConfig(&cfg.Daemon)
	normalizeScanningConfig(&cfg.Scanning)
	normalizeAPIConfig(&cfg.API)
	normalizeLoggingConfig(&cfg.Logging)
	normalizeDiscoveryConfig(&cfg.Discovery)
}

// normalizeDaemonConfig cleans up daemon configuration values.
func normalizeDaemonConfig(cfg *DaemonConfig) {
	if cfg.PIDFile != "" {
		cfg.PIDFile = filepath.Clean(cfg.PIDFile)
	}
	if cfg.WorkDir != "" {
		cfg.WorkDir = filepath.Clean(cfg.WorkDir)
	}
	cfg.User = strings.TrimSpace(cfg.User)
	cfg.Group = strings.TrimSpace(cfg.Group)
}

// normalizeScanningConfig cleans up scanning configuration
// values.
func normalizeScanningConfig(cfg *ScanningConfig) {
	cfg.ScanMode = strings.ToLower(
		strings.TrimSpace(cfg.ScanMode),
	)
	cfg.DefaultPorts = strings.TrimSpace(cfg.DefaultPorts)
}

// normalizeAPIConfig cleans up API configuration values.
func normalizeAPIConfig(cfg *APIConfig) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.TLS.CertFile != "" {
		cfg.TLS.CertFile = filepath.Clean(cfg.TLS.CertFile)
	}
	if cfg.TLS.KeyFile != "" {
		cfg.TLS.KeyFile = filepath.Clean(cfg.TLS.KeyFile)
	}
	if cfg.TLS.CAFile != "" {
		cfg.TLS.CAFile = filepath.Clean(cfg.TLS.CAFile)
	}
}

// normalizeLoggingConfig cleans up logging configuration values.
func normalizeLoggingConfig(cfg *LoggingConfig) {
	cfg.Level = strings.ToLower(strings.TrimSpace(cfg.Level))
	cfg.Format = strings.ToLower(
		strings.TrimSpace(cfg.Format),
	)
	cfg.Output = strings.TrimSpace(cfg.Output)
	// Clean output path if it's a file path (not stdout/stderr)
	if cfg.Output != "" &&
		cfg.Output != outputStdout &&
		cfg.Output != outputStderr {
		cfg.Output = filepath.Clean(cfg.Output)
	}
}

// normalizeDiscoveryConfig cleans up discovery configuration
// values.
func normalizeDiscoveryConfig(cfg *DiscoveryConfig) {
	cfg.Defaults.Method = strings.ToLower(
		strings.TrimSpace(cfg.Defaults.Method),
	)
	cfg.Defaults.Ports = strings.TrimSpace(cfg.Defaults.Ports)
	cfg.Defaults.Schedule = strings.TrimSpace(
		cfg.Defaults.Schedule,
	)
	cfg.Defaults.Timeout = strings.TrimSpace(
		cfg.Defaults.Timeout,
	)

	for i := range cfg.Networks {
		cfg.Networks[i].Name = strings.TrimSpace(
			cfg.Networks[i].Name,
		)
		cfg.Networks[i].CIDR = strings.TrimSpace(
			cfg.Networks[i].CIDR,
		)
		cfg.Networks[i].Method = strings.ToLower(
			strings.TrimSpace(cfg.Networks[i].Method),
		)
		cfg.Networks[i].Schedule = strings.TrimSpace(
			cfg.Networks[i].Schedule,
		)
		cfg.Networks[i].Ports = strings.TrimSpace(
			cfg.Networks[i].Ports,
		)
	}
}

// --- Per-section validators ---

// ValidateDaemonConfig validates the daemon configuration
// section.
func ValidateDaemonConfig(cfg *DaemonConfig) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError(
			"daemon", "", "daemon configuration is nil",
		)
		return result
	}

	const section = "daemon"

	if cfg.ShutdownTimeout < 0 {
		result.addError(
			section, "shutdown_timeout",
			"shutdown timeout must not be negative",
		)
	} else if cfg.ShutdownTimeout == 0 {
		result.addWarning(
			section, "shutdown_timeout",
			"shutdown timeout is zero; the default will be used",
		)
	}

	if cfg.PIDFile != "" {
		cleaned := filepath.Clean(cfg.PIDFile)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "pid_file",
				"path contains directory traversal",
			)
		}
	}

	if cfg.WorkDir != "" {
		cleaned := filepath.Clean(cfg.WorkDir)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "work_dir",
				"path contains directory traversal",
			)
		}
	}

	if cfg.Daemonize {
		if cfg.PIDFile == "" {
			result.addWarning(
				section, "pid_file",
				"PID file not set while daemonize is enabled",
			)
		}
	}

	return result
}

// ValidateDatabaseConfig validates the database configuration
// section.
func ValidateDatabaseConfig(cfg *db.Config) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError(
			"database", "",
			"database configuration is nil",
		)
		return result
	}

	const section = "database"

	if cfg.Host == "" {
		result.addError(
			section, "host",
			"database host is required "+
				"(set SCANORAMA_DB_HOST or configure in file)",
		)
	}
	if cfg.Database == "" {
		result.addError(
			section, "database",
			"database name is required "+
				"(set SCANORAMA_DB_NAME or configure in file)",
		)
	}
	if cfg.Username == "" {
		result.addError(
			section, "username",
			"database username is required "+
				"(set SCANORAMA_DB_USER or configure in file)",
		)
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		result.addError(
			section, "port",
			fmt.Sprintf(
				"database port must be between 1 and 65535,"+
					" got %d", cfg.Port,
			),
		)
	}

	if cfg.Password == "" {
		result.addWarning(
			section, "password",
			"database password is empty; "+
				"ensure this is intentional",
		)
	}

	// Validate SSL mode
	validSSLModes := map[string]bool{
		"disable":     true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
		"prefer":      true,
		"allow":       true,
		"":            true,
	}
	if !validSSLModes[cfg.SSLMode] {
		result.addError(
			section, "ssl_mode",
			fmt.Sprintf(
				"invalid SSL mode: %q (valid: disable,"+
					" require, verify-ca, verify-full,"+
					" prefer, allow)",
				cfg.SSLMode,
			),
		)
	}

	validateDatabaseConnPool(cfg, section, result)

	return result
}

// validateDatabaseConnPool validates database connection pool
// settings.
func validateDatabaseConnPool(
	cfg *db.Config, section string, result *ValidationResult,
) {
	if cfg.MaxOpenConns < 0 {
		result.addError(
			section, "max_open_conns",
			"max open connections must not be negative",
		)
	}
	if cfg.MaxIdleConns < 0 {
		result.addError(
			section, "max_idle_conns",
			"max idle connections must not be negative",
		)
	}
	if cfg.MaxOpenConns > 0 &&
		cfg.MaxIdleConns > cfg.MaxOpenConns {
		result.addWarning(
			section, "max_idle_conns",
			fmt.Sprintf(
				"max idle connections (%d) exceeds max open"+
					" connections (%d); idle conns will"+
					" be capped",
				cfg.MaxIdleConns, cfg.MaxOpenConns,
			),
		)
	}
	if cfg.ConnMaxLifetime < 0 {
		result.addError(
			section, "conn_max_lifetime",
			"connection max lifetime must not be negative",
		)
	}
	if cfg.ConnMaxIdleTime < 0 {
		result.addError(
			section, "conn_max_idle_time",
			"connection max idle time must not be negative",
		)
	}
}

// ValidateScanningConfig validates the scanning configuration
// section.
func ValidateScanningConfig(
	cfg *ScanningConfig,
) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError(
			"scanning", "",
			"scanning configuration is nil",
		)
		return result
	}

	const section = "scanning"

	if cfg.WorkerPoolSize <= 0 {
		result.addError(
			section, "worker_pool_size",
			fmt.Sprintf(
				"worker pool size must be positive, got %d",
				cfg.WorkerPoolSize,
			),
		)
	}
	if cfg.MaxConcurrentTargets <= 0 {
		result.addError(
			section, "max_concurrent_targets",
			fmt.Sprintf(
				"max concurrent targets must be positive,"+
					" got %d",
				cfg.MaxConcurrentTargets,
			),
		)
	}
	if cfg.DefaultInterval <= 0 {
		result.addError(
			section, "default_interval",
			"default scan interval must be positive",
		)
	}
	if cfg.MaxScanTimeout < 0 {
		result.addError(
			section, "max_scan_timeout",
			"max scan timeout must not be negative",
		)
	} else if cfg.MaxScanTimeout == 0 {
		result.addWarning(
			section, "max_scan_timeout",
			"max scan timeout is zero; "+
				"scans may run indefinitely",
		)
	}

	// Validate scan mode
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"ack":           true,
		"udp":           true,
		"aggressive":    true,
		"comprehensive": true,
	}
	scanType := strings.ToLower(
		strings.TrimSpace(cfg.ScanMode),
	)
	if !validScanTypes[scanType] {
		result.addError(
			section, "scan_mode",
			fmt.Sprintf(
				"invalid scan_mode: %q"+
					" (valid: connect, syn, ack, udp,"+
					" aggressive, comprehensive)",
				cfg.ScanMode,
			),
		)
	}

	validateScanningRetry(cfg, section, result)
	validateScanningRateLimit(cfg, section, result)

	// Warn on high worker pool sizes
	if cfg.WorkerPoolSize > largeWorkerPoolThreshold {
		result.addWarning(
			section, "worker_pool_size",
			fmt.Sprintf(
				"worker pool size %d is very large; this"+
					" may consume excessive resources",
				cfg.WorkerPoolSize,
			),
		)
	}

	return result
}

// validateScanningRetry validates the retry sub-section of
// scanning configuration.
func validateScanningRetry(
	cfg *ScanningConfig,
	section string,
	result *ValidationResult,
) {
	if cfg.Retry.MaxRetries < 0 {
		result.addError(
			section, "retry.max_retries",
			"max retries must not be negative",
		)
	}
	if cfg.Retry.RetryDelay < 0 {
		result.addError(
			section, "retry.retry_delay",
			"retry delay must not be negative",
		)
	}
	if cfg.Retry.BackoffMultiplier < 0 {
		result.addError(
			section, "retry.backoff_multiplier",
			"backoff multiplier must not be negative",
		)
	} else if cfg.Retry.BackoffMultiplier > 0 &&
		cfg.Retry.BackoffMultiplier < 1.0 {
		result.addWarning(
			section, "retry.backoff_multiplier",
			fmt.Sprintf(
				"backoff multiplier %.2f is less than 1.0;"+
					" retries will use decreasing delays",
				cfg.Retry.BackoffMultiplier,
			),
		)
	}
}

// validateScanningRateLimit validates the rate_limit sub-section
// of scanning configuration.
func validateScanningRateLimit(
	cfg *ScanningConfig,
	section string,
	result *ValidationResult,
) {
	if !cfg.RateLimit.Enabled {
		return
	}
	if cfg.RateLimit.RequestsPerSecond <= 0 {
		result.addError(
			section, "rate_limit.requests_per_second",
			"requests per second must be positive"+
				" when rate limiting is enabled",
		)
	}
	if cfg.RateLimit.BurstSize <= 0 {
		result.addError(
			section, "rate_limit.burst_size",
			"burst size must be positive"+
				" when rate limiting is enabled",
		)
	}
}

// ValidateAPIConfig validates the API configuration section,
// including TLS and auth.
func ValidateAPIConfig(cfg *APIConfig) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError("api", "", "API configuration is nil")
		return result
	}

	const section = "api"

	// If API is disabled, only produce warnings for obviously
	// wrong settings.
	if !cfg.Enabled {
		if cfg.TLS.Enabled {
			result.addWarning(
				section, "tls.enabled",
				"TLS is enabled but API is disabled",
			)
		}
		if cfg.AuthEnabled {
			result.addWarning(
				section, "auth_enabled",
				"authentication is enabled but API is disabled",
			)
		}
		return result
	}

	validateAPIPortAndHost(cfg, section, result)
	validateAPITimeouts(cfg, section, result)
	validateAPISizeLimits(cfg, section, result)

	// TLS validation
	result.merge(validateTLSConfig(&cfg.TLS))

	validateAPIAuth(cfg, section, result)
	validateAPIRateLimit(cfg, section, result)
	validateAPICORS(cfg, section, result)

	return result
}

// validateAPIPortAndHost validates port and host fields
// for the API configuration.
func validateAPIPortAndHost(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if cfg.Port < 1 || cfg.Port > 65535 {
		result.addError(
			section, "port",
			fmt.Sprintf(
				"API port must be between 1 and 65535,"+
					" got %d", cfg.Port,
			),
		)
	} else if cfg.Port < 1024 {
		result.addWarning(
			section, "port",
			fmt.Sprintf(
				"API port %d is a privileged port; ensure"+
					" the process has appropriate"+
					" permissions",
				cfg.Port,
			),
		)
	}

	if cfg.Host == "" {
		result.addError(
			section, "host",
			"API host address is required "+
				"when API is enabled",
		)
	}
}

// validateAPITimeouts validates timeout fields for the API
// configuration.
func validateAPITimeouts(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if cfg.ReadTimeout <= 0 {
		result.addError(
			section, "read_timeout",
			"API read timeout must be positive",
		)
	}
	if cfg.WriteTimeout <= 0 {
		result.addError(
			section, "write_timeout",
			"API write timeout must be positive",
		)
	}
	if cfg.IdleTimeout <= 0 {
		result.addError(
			section, "idle_timeout",
			"API idle timeout must be positive",
		)
	}
	if cfg.RequestTimeout < 0 {
		result.addError(
			section, "request_timeout",
			"API request timeout must not be negative",
		)
	}
}

// validateAPISizeLimits validates size limit fields for the API
// configuration.
func validateAPISizeLimits(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if cfg.MaxHeaderBytes <= 0 {
		result.addError(
			section, "max_header_bytes",
			"API max header bytes must be positive",
		)
	}
	if cfg.MaxRequestSize < 0 {
		result.addError(
			section, "max_request_size",
			"API max request size must not be negative",
		)
	}
}

// validateAPIAuth validates authentication settings for the API
// configuration.
func validateAPIAuth(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if cfg.AuthEnabled && len(cfg.APIKeys) == 0 {
		result.addError(
			section, "api_keys",
			"at least one API key must be provided"+
				" when authentication is enabled",
		)
	}
	if !cfg.AuthEnabled && !cfg.TLS.Enabled {
		result.addWarning(
			section, "auth_enabled",
			"API has no authentication and no TLS;"+
				" consider enabling at least one"+
				" for security",
		)
	}

	if cfg.AuthEnabled {
		for i, key := range cfg.APIKeys {
			field := fmt.Sprintf("api_keys[%d]", i)
			if strings.TrimSpace(key) == "" {
				result.addError(
					section, field,
					"API key must not be empty"+
						" or whitespace",
				)
			} else if len(key) < 16 {
				result.addWarning(
					section, field,
					"API key is shorter than 16"+
						" characters; consider using"+
						" a longer key",
				)
			}
		}
	}
}

// validateAPIRateLimit validates rate limiting settings for the
// API configuration.
func validateAPIRateLimit(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if !cfg.RateLimitEnabled {
		return
	}
	if cfg.RateLimitRequests <= 0 {
		result.addError(
			section, "rate_limit_requests",
			"rate limit requests must be positive"+
				" when rate limiting is enabled",
		)
	}
	if cfg.RateLimitWindow <= 0 {
		result.addError(
			section, "rate_limit_window",
			"rate limit window must be positive"+
				" when rate limiting is enabled",
		)
	}
}

// validateAPICORS validates CORS settings for the API
// configuration.
func validateAPICORS(
	cfg *APIConfig,
	section string,
	result *ValidationResult,
) {
	if !cfg.EnableCORS {
		return
	}
	for _, origin := range cfg.CORSOrigins {
		if origin == "*" && cfg.AuthEnabled {
			result.addWarning(
				section, "cors_origins",
				"wildcard CORS origin '*' with"+
					" authentication enabled may"+
					" be a security risk",
			)
		}
	}
}

// validateTLSConfig validates TLS configuration as a subsection
// of API.
func validateTLSConfig(cfg *TLSConfig) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		return result
	}

	const section = "api"

	if !cfg.Enabled {
		return result
	}

	if cfg.CertFile == "" {
		result.addError(
			section, "tls.cert_file",
			"TLS certificate file is required"+
				" when TLS is enabled",
		)
	} else {
		cleaned := filepath.Clean(cfg.CertFile)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "tls.cert_file",
				"certificate file path contains"+
					" directory traversal",
			)
		}
	}

	if cfg.KeyFile == "" {
		result.addError(
			section, "tls.key_file",
			"TLS key file is required when TLS is enabled",
		)
	} else {
		cleaned := filepath.Clean(cfg.KeyFile)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "tls.key_file",
				"key file path contains"+
					" directory traversal",
			)
		}
	}

	if cfg.CAFile != "" {
		cleaned := filepath.Clean(cfg.CAFile)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "tls.ca_file",
				"CA file path contains"+
					" directory traversal",
			)
		}
	}

	return result
}

// ValidateLoggingConfig validates the logging configuration
// section.
func ValidateLoggingConfig(
	cfg *LoggingConfig,
) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError(
			"logging", "",
			"logging configuration is nil",
		)
		return result
	}

	const section = "logging"

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	level := strings.ToLower(strings.TrimSpace(cfg.Level))
	if !validLogLevels[level] {
		result.addError(
			section, "level",
			fmt.Sprintf(
				"invalid log level: %q"+
					" (valid: debug, info, warn, error)",
				cfg.Level,
			),
		)
	}

	// Validate log format
	validLogFormats := map[string]bool{
		"text": true,
		"json": true,
	}
	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	if !validLogFormats[format] {
		result.addError(
			section, "format",
			fmt.Sprintf(
				"invalid log format: %q"+
					" (valid: text, json)",
				cfg.Format,
			),
		)
	}

	// Validate output
	if cfg.Output == "" {
		result.addWarning(
			section, "output",
			"log output is empty; defaulting to stdout",
		)
	} else if cfg.Output != outputStdout &&
		cfg.Output != outputStderr {
		// It's a file path - validate it
		cleaned := filepath.Clean(cfg.Output)
		if strings.Contains(cleaned, "..") {
			result.addError(
				section, "output",
				"log output path contains"+
					" directory traversal",
			)
		}
	}

	// Validate rotation
	validateLoggingRotation(cfg, section, result)

	return result
}

// validateLoggingRotation validates the rotation sub-section
// of logging configuration.
func validateLoggingRotation(
	cfg *LoggingConfig,
	section string,
	result *ValidationResult,
) {
	if !cfg.Rotation.Enabled {
		return
	}
	if cfg.Output == outputStdout || cfg.Output == outputStderr {
		result.addWarning(
			section, "rotation.enabled",
			"log rotation is enabled but output is"+
				" stdout/stderr; rotation has no effect",
		)
	}
	if cfg.Rotation.MaxSizeMB <= 0 {
		result.addError(
			section, "rotation.max_size_mb",
			"log rotation max size must be positive"+
				" when rotation is enabled",
		)
	}
	if cfg.Rotation.MaxBackups < 0 {
		result.addError(
			section, "rotation.max_backups",
			"log rotation max backups must not be negative",
		)
	}
	if cfg.Rotation.MaxAgeDays < 0 {
		result.addError(
			section, "rotation.max_age_days",
			"log rotation max age must not be negative",
		)
	}
}

// ValidateDiscoveryConfig validates the discovery configuration
// section.
func ValidateDiscoveryConfig(
	cfg *DiscoveryConfig,
) *ValidationResult {
	result := &ValidationResult{}
	if cfg == nil {
		result.addError(
			"discovery", "",
			"discovery configuration is nil",
		)
		return result
	}

	const section = "discovery"

	// Validate default method
	validMethods := map[string]bool{
		"ping": true,
		"tcp":  true,
		"arp":  true,
		"":     true, // empty is OK
	}
	method := strings.ToLower(
		strings.TrimSpace(cfg.Defaults.Method),
	)
	if !validMethods[method] {
		result.addError(
			section, "defaults.method",
			fmt.Sprintf(
				"invalid default discovery method: %q"+
					" (valid: ping, tcp, arp)",
				cfg.Defaults.Method,
			),
		)
	}

	// Validate default timeout if set
	if cfg.Defaults.Timeout != "" {
		_, err := time.ParseDuration(cfg.Defaults.Timeout)
		if err != nil {
			result.addError(
				section, "defaults.timeout",
				fmt.Sprintf(
					"invalid default timeout: %q"+
						" is not a valid duration",
					cfg.Defaults.Timeout,
				),
			)
		}
	}

	// Validate networks
	validateDiscoveryNetworks(
		cfg, section, validMethods, result,
	)

	// Validate global exclusions
	for i, excl := range cfg.GlobalExclusions {
		field := fmt.Sprintf("global_exclusions[%d]", i)
		if _, _, err := net.ParseCIDR(excl); err != nil {
			if ip := net.ParseIP(excl); ip == nil {
				result.addError(
					section, field,
					fmt.Sprintf(
						"invalid global exclusion: %q"+
							" (must be CIDR or IP)",
						excl,
					),
				)
			}
		}
	}

	return result
}

// validateDiscoveryNetworks validates the networks sub-section
// of discovery configuration.
func validateDiscoveryNetworks(
	cfg *DiscoveryConfig,
	section string,
	validMethods map[string]bool,
	result *ValidationResult,
) {
	networkNames := make(map[string]bool)
	for i := range cfg.Networks {
		network := &cfg.Networks[i]
		prefix := fmt.Sprintf("networks[%d]", i)

		if network.Name == "" {
			result.addError(
				section, prefix+".name",
				"network name is required",
			)
		} else if networkNames[network.Name] {
			result.addError(
				section, prefix+".name",
				fmt.Sprintf(
					"duplicate network name: %q",
					network.Name,
				),
			)
		} else {
			networkNames[network.Name] = true
		}

		if network.CIDR == "" {
			result.addError(section, prefix+".cidr", "network CIDR is required")
		} else if _, ipNet, err := net.ParseCIDR(network.CIDR); err != nil {
			result.addError(
				section, prefix+".cidr",
				fmt.Sprintf("invalid CIDR: %q (%v)", network.CIDR, err),
			)
		} else if ones, bits := ipNet.Mask.Size(); ones == bits {
			result.addError(
				section, prefix+".cidr",
				fmt.Sprintf(
					"/%d address is a single host, not a network — use a broader prefix",
					ones,
				),
			)
		}

		netMethod := strings.ToLower(
			strings.TrimSpace(network.Method),
		)
		if netMethod != "" && !validMethods[netMethod] {
			result.addError(
				section, prefix+".method",
				fmt.Sprintf(
					"invalid discovery method: %q"+
						" (valid: ping, tcp, arp)",
					network.Method,
				),
			)
		}

		// Validate per-network exclusions
		for j, excl := range network.Exclusions {
			exclField := fmt.Sprintf(
				"%s.exclusions[%d]", prefix, j,
			)
			if _, _, err := net.ParseCIDR(excl); err != nil {
				// Try as single IP
				if ip := net.ParseIP(excl); ip == nil {
					result.addError(
						section, exclField,
						fmt.Sprintf(
							"invalid exclusion: %q"+
								" (must be CIDR or IP)",
							excl,
						),
					)
				}
			}
		}
	}
}
