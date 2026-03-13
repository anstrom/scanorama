// Package handlers - admin configuration validation.
package handlers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// validateConfigUpdate validates a configuration update request.
func (h *AdminHandler) validateConfigUpdate(req *ConfigUpdateRequest) error {
	// First validate the request structure using validator
	if err := h.validator.Struct(req); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}

	if req.Section == "" {
		return fmt.Errorf("configuration section is required")
	}

	validSections := map[string]bool{
		configSectionAPI:      true,
		configSectionDatabase: true,
		configSectionScanning: true,
		configSectionLogging:  true,
		configSectionDaemon:   true,
	}

	if !validSections[req.Section] {
		return fmt.Errorf("invalid configuration section: %s", req.Section)
	}

	// Validate that the appropriate configuration section is provided and validate its content
	return h.validateConfigSection(req)
}

// validateConfigSection validates configuration sections (extracted to reduce complexity)
func (h *AdminHandler) validateConfigSection(req *ConfigUpdateRequest) error {
	switch req.Section {
	case configSectionAPI:
		return h.validateAPISection(req.Config.API)
	case configSectionDatabase:
		return h.validateDatabaseSection(req.Config.Database)
	case configSectionScanning:
		return h.validateScanningSection(req.Config.Scanning)
	case configSectionLogging:
		return h.validateLoggingSection(req.Config.Logging)
	case configSectionDaemon:
		return h.validateDaemonSection(req.Config.Daemon)
	default:
		return fmt.Errorf("unsupported configuration section: %s", req.Section)
	}
}

// validateAPISection validates API configuration section
func (h *AdminHandler) validateAPISection(config *APIConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("api configuration data is required for api section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("api configuration validation failed: %w", err)
	}
	if err := h.validateAPIConfig(config); err != nil {
		return fmt.Errorf("api configuration security validation failed: %w", err)
	}
	return nil
}

// validateDatabaseSection validates database configuration section
func (h *AdminHandler) validateDatabaseSection(config *DatabaseConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("database configuration data is required for database section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("database configuration validation failed: %w", err)
	}
	if err := h.validateDatabaseConfig(config); err != nil {
		return fmt.Errorf("database configuration security validation failed: %w", err)
	}
	return nil
}

// validateScanningSection validates scanning configuration section
func (h *AdminHandler) validateScanningSection(config *ScanningConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("scanning configuration data is required for scanning section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("scanning configuration validation failed: %w", err)
	}
	if err := h.validateScanningConfig(config); err != nil {
		return fmt.Errorf("scanning configuration security validation failed: %w", err)
	}
	return nil
}

// validateLoggingSection validates logging configuration section
func (h *AdminHandler) validateLoggingSection(config *LoggingConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("logging configuration data is required for logging section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("logging configuration validation failed: %w", err)
	}
	if err := h.validateLoggingConfig(config); err != nil {
		return fmt.Errorf("logging configuration security validation failed: %w", err)
	}
	return nil
}

// validateDaemonSection validates daemon configuration section
func (h *AdminHandler) validateDaemonSection(config *DaemonConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("daemon configuration data is required for daemon section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("daemon configuration validation failed: %w", err)
	}
	if err := h.validateDaemonConfig(config); err != nil {
		return fmt.Errorf("daemon configuration security validation failed: %w", err)
	}
	return nil
}

// validateAPIConfig validates API configuration with custom security checks.
func (h *AdminHandler) validateAPIConfig(config *APIConfigUpdate) error {
	if err := h.validateAPINetworkSettings(config); err != nil {
		return err
	}

	if err := h.validateAPITimeoutSettings(config); err != nil {
		return err
	}

	if err := h.validateAPICORSSettings(config); err != nil {
		return err
	}

	return nil
}

// validateAPINetworkSettings validates API network-related configuration
func (h *AdminHandler) validateAPINetworkSettings(config *APIConfigUpdate) error {
	if config.Host != nil {
		if err := validateHostField("host", *config.Host); err != nil {
			return err
		}
	}

	if config.Port != nil {
		if err := validatePortField("port", *config.Port); err != nil {
			return err
		}
	}

	return nil
}

// validateAPITimeoutSettings validates API timeout-related configuration
func (h *AdminHandler) validateAPITimeoutSettings(config *APIConfigUpdate) error {
	if config.ReadTimeout != nil {
		if err := validateDurationField("read_timeout", *config.ReadTimeout); err != nil {
			return err
		}
	}

	if config.WriteTimeout != nil {
		if err := validateDurationField("write_timeout", *config.WriteTimeout); err != nil {
			return err
		}
	}

	if config.IdleTimeout != nil {
		if err := validateDurationField("idle_timeout", *config.IdleTimeout); err != nil {
			return err
		}
	}

	if config.RequestTimeout != nil {
		if err := validateDurationField("request_timeout", *config.RequestTimeout); err != nil {
			return err
		}
	}

	if config.RateLimitWindow != nil {
		if err := validateDurationField("rate_limit_window", *config.RateLimitWindow); err != nil {
			return err
		}
	}

	return nil
}

// validateAPICORSSettings validates API CORS-related configuration
func (h *AdminHandler) validateAPICORSSettings(config *APIConfigUpdate) error {
	if config.CORSOrigins != nil {
		for i, origin := range config.CORSOrigins {
			fieldName := fmt.Sprintf("cors_origins[%d]", i)
			if err := validateStringField(fieldName, origin, maxAdminHostnameLength); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateDatabaseConfig validates database configuration with custom security checks.
func (h *AdminHandler) validateDatabaseConfig(config *DatabaseConfigUpdate) error {
	if config.Host != nil {
		if err := validateHostField("host", *config.Host); err != nil {
			return err
		}
	}

	if config.Port != nil {
		if err := validatePortField("port", *config.Port); err != nil {
			return err
		}
	}

	if config.Database != nil {
		if err := validateStringField("database", *config.Database, maxDatabaseNameLength); err != nil {
			return err
		}
	}

	if config.Username != nil {
		if err := validateStringField("username", *config.Username, maxUsernameLength); err != nil {
			return err
		}
	}

	if config.ConnMaxLifetime != nil {
		if err := validateDurationField("conn_max_lifetime", *config.ConnMaxLifetime); err != nil {
			return err
		}
	}

	if config.ConnMaxIdleTime != nil {
		if err := validateDurationField("conn_max_idle_time", *config.ConnMaxIdleTime); err != nil {
			return err
		}
	}

	return nil
}

// validateScanningConfig validates scanning configuration with custom security checks.
func (h *AdminHandler) validateScanningConfig(config *ScanningConfigUpdate) error {
	if config.DefaultInterval != nil {
		if err := validateDurationField("default_interval", *config.DefaultInterval); err != nil {
			return err
		}
	}

	if config.MaxScanTimeout != nil {
		if err := validateDurationField("max_scan_timeout", *config.MaxScanTimeout); err != nil {
			return err
		}
	}

	if config.DefaultPorts != nil {
		if err := validateStringField("default_ports", *config.DefaultPorts, maxAdminPortsStringLength); err != nil {
			return err
		}
		// Additional validation for port ranges could be added here
	}

	return nil
}

// validateLoggingConfig validates logging configuration with custom security checks.
func (h *AdminHandler) validateLoggingConfig(config *LoggingConfigUpdate) error {
	if config.Output != nil {
		if err := validatePathField("output", *config.Output); err != nil {
			return err
		}
	}

	return nil
}

// validateDaemonConfig validates daemon configuration with custom security checks.
func (h *AdminHandler) validateDaemonConfig(config *DaemonConfigUpdate) error {
	if config.PIDFile != nil {
		if err := validatePathField("pid_file", *config.PIDFile); err != nil {
			return err
		}
	}

	if config.WorkDir != nil {
		if err := validatePathField("work_dir", *config.WorkDir); err != nil {
			return err
		}
	}

	if config.User != nil {
		if err := validateStringField("user", *config.User, 32); err != nil {
			return err
		}
	}

	if config.Group != nil {
		if err := validateStringField("group", *config.Group, 32); err != nil {
			return err
		}
	}

	if config.ShutdownTimeout != nil {
		if err := validateDurationField("shutdown_timeout", *config.ShutdownTimeout); err != nil {
			return err
		}
	}

	return nil
}

// validateStringField validates string configuration fields with security constraints.
func validateStringField(field, value string, maxLength int) error {
	if len(value) > maxLength {
		return fmt.Errorf("%s too long: %d characters (max %d)", field, len(value), maxLength)
	}

	// Check for null bytes
	for i, char := range value {
		if char == 0 {
			return fmt.Errorf("%s contains null byte at position %d", field, i)
		}
	}

	// Check for control characters (except tabs and newlines)
	for i, char := range value {
		if char < 32 && char != 9 && char != 10 && char != 13 {
			return fmt.Errorf("%s contains control character at position %d", field, i)
		}
	}

	return nil
}

// validateHostField validates hostname or IP address fields.
func validateHostField(field, value string) error {
	if err := validateStringField(field, value, maxAdminHostnameLength); err != nil { // Max hostname length
		return err
	}

	// Basic hostname validation - allow empty for "listen on all interfaces"
	if value == "" {
		return nil
	}

	// Check for valid characters (basic validation)
	for i, char := range value {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '.' && char != '-' && char != ':' {
			return fmt.Errorf("%s contains invalid character at position %d", field, i)
		}
	}

	return nil
}

// validatePortField validates port number fields.
func validatePortField(field string, value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("%s out of range: %d (must be 1-65535)", field, value)
	}

	// Privileged ports (< 1024) are allowed but noted for security awareness

	return nil
}

// validateDurationField validates duration string fields.
func validateDurationField(field, value string) error {
	if err := validateStringField(field, value, maxDurationStringLength); err != nil {
		return err
	}

	if value == "" {
		return nil
	}

	// Try to parse as duration
	_, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s is not a valid duration: %w", field, err)
	}

	return nil
}

// validatePathField validates file path fields.
func validatePathField(field, value string) error {
	if err := validateStringField(field, value, maxPathLength); err != nil {
		return err
	}

	if value == "" {
		return nil
	}

	// Check for directory traversal patterns
	if strings.Contains(value, "..") {
		return fmt.Errorf("%s contains directory traversal: %s", field, value)
	}

	// Additional check for cleaned path differences
	cleanPath := filepath.Clean(value)
	if cleanPath != value && strings.Contains(cleanPath, "..") {
		return fmt.Errorf("%s contains directory traversal: %s", field, value)
	}

	return nil
}
