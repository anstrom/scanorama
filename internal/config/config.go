// Package config provides configuration management for scanorama.
// It handles loading configuration from files, environment variables,
// and provides default values for various components.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Default timeout and retry values.
	defaultShutdownTimeoutSec = 30
	defaultRetryDelaySec      = 30
	defaultRequestTimeoutSec  = 30
	defaultMaxRetries         = 3
	defaultBackoffMultiplier  = 2.0

	// Default scanning configuration values.
	defaultWorkerPoolSize       = 10
	defaultScanTimeoutMin       = 10
	defaultMaxConcurrentTargets = 100
	defaultRequestsPerSecond    = 100
	defaultBurstSize            = 200

	// Default API configuration.
	defaultAPIPort          = 8080
	defaultMaxRequestSizeMB = 1024
	bytesPerMB              = 1024

	// Default logging configuration.
	defaultMaxSizeMB  = 100
	defaultMaxBackups = 5
	defaultMaxAgeDays = 30

	// Security validation constants.
	maxConfigSize   = 10 * 1024 * 1024 // Maximum config file size (10MB)
	maxContentSize  = 5 * 1024 * 1024  // Maximum config content size (5MB)
	maxPathLength   = 4096             // Maximum file path length
	permissionsMask = 0o777            // File permissions mask for validation
)

// Default configuration values.
const (
	DefaultPostgresPort    = 5432
	DefaultMaxOpenConns    = 25
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = 5 * time.Minute
	DefaultConnMaxIdleTime = 5 * time.Minute
	DefaultDirPermissions  = 0o750
	DefaultFilePermissions = 0o600
)

// Config represents the application configuration.
type Config struct {
	// Daemon configuration
	Daemon DaemonConfig `yaml:"daemon" json:"daemon"`

	// Database configuration
	Database db.Config `yaml:"database" json:"database"`

	// Scanning configuration
	Scanning ScanningConfig `yaml:"scanning" json:"scanning"`

	// API configuration
	API APIConfig `yaml:"api" json:"api"`

	// Discovery configuration
	Discovery DiscoveryConfig `yaml:"discovery" json:"discovery"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging" json:"logging"`
}

// DaemonConfig holds daemon-specific settings.
type DaemonConfig struct {
	// PID file location
	PIDFile string `yaml:"pid_file" json:"pid_file"`

	// Working directory
	WorkDir string `yaml:"work_dir" json:"work_dir"`

	// User to run as (for privilege dropping)
	User string `yaml:"user" json:"user"`

	// Group to run as
	Group string `yaml:"group" json:"group"`

	// Enable daemon mode (fork to background)
	Daemonize bool `yaml:"daemonize" json:"daemonize"`

	// Graceful shutdown timeout
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
}

// ScanningConfig holds scanning-related settings.
type ScanningConfig struct {
	// Number of concurrent scanning workers
	WorkerPoolSize int `yaml:"worker_pool_size" json:"worker_pool_size"`

	// Default scan interval for targets
	DefaultInterval time.Duration `yaml:"default_interval" json:"default_interval"`

	// Maximum scan timeout per target
	MaxScanTimeout time.Duration `yaml:"max_scan_timeout" json:"max_scan_timeout"`

	// Default ports to scan
	DefaultPorts string `yaml:"default_ports" json:"default_ports"`

	// Default scan type
	DefaultScanType string `yaml:"default_scan_type" json:"default_scan_type"`

	// Maximum concurrent targets per job
	MaxConcurrentTargets int `yaml:"max_concurrent_targets" json:"max_concurrent_targets"`

	// Enable service detection
	EnableServiceDetection bool `yaml:"enable_service_detection" json:"enable_service_detection"`

	// Enable OS detection
	EnableOSDetection bool `yaml:"enable_os_detection" json:"enable_os_detection"`

	// Retry configuration
	Retry RetryConfig `yaml:"retry" json:"retry"`

	// Rate limiting
	RateLimit RateLimitConfig `yaml:"rate_limit" json:"rate_limit"`
}

// RetryConfig holds retry settings for failed scans.
type RetryConfig struct {
	// Maximum number of retries
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// Delay between retries
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`

	// Exponential backoff multiplier
	BackoffMultiplier float64 `yaml:"backoff_multiplier" json:"backoff_multiplier"`
}

// RateLimitConfig holds rate limiting settings.
type RateLimitConfig struct {
	// Enable rate limiting
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Requests per second
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second"`

	// Burst size
	BurstSize int `yaml:"burst_size" json:"burst_size"`
}

// APIConfig holds API server settings.
type APIConfig struct {
	// Enable API server
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Listen host
	Host string `yaml:"host" json:"host"`

	// Listen port
	Port int `yaml:"port" json:"port"`

	// HTTP timeouts
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout" json:"idle_timeout"`

	// Maximum header size
	MaxHeaderBytes int `yaml:"max_header_bytes" json:"max_header_bytes"`

	// Enable TLS
	TLS TLSConfig `yaml:"tls" json:"tls"`

	// Authentication settings
	AuthEnabled bool     `yaml:"auth_enabled" json:"auth_enabled"`
	APIKeys     []string `yaml:"api_keys" json:"api_keys"`

	// CORS settings
	EnableCORS  bool     `yaml:"enable_cors" json:"enable_cors"`
	CORSOrigins []string `yaml:"cors_origins" json:"cors_origins"`

	// Rate limiting
	RateLimitEnabled  bool          `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	RateLimitRequests int           `yaml:"rate_limit_requests" json:"rate_limit_requests"`
	RateLimitWindow   time.Duration `yaml:"rate_limit_window" json:"rate_limit_window"`

	// Request timeout (deprecated, use ReadTimeout)
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`

	// Maximum request size
	MaxRequestSize int64 `yaml:"max_request_size" json:"max_request_size"`
}

// TLSConfig holds TLS settings.
type TLSConfig struct {
	// Enable TLS
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Certificate file path
	CertFile string `yaml:"cert_file" json:"cert_file"`

	// Private key file path
	KeyFile string `yaml:"key_file" json:"key_file"`

	// CA certificate file (for client authentication)
	CAFile string `yaml:"ca_file" json:"ca_file"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	// Enable CORS
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Allowed origins
	AllowedOrigins []string `yaml:"allowed_origins" json:"allowed_origins"`

	// Allowed methods
	AllowedMethods []string `yaml:"allowed_methods" json:"allowed_methods"`

	// Allowed headers
	AllowedHeaders []string `yaml:"allowed_headers" json:"allowed_headers"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	// Log level (debug, info, warn, error)
	Level string `yaml:"level" json:"level"`

	// Log format (text, json)
	Format string `yaml:"format" json:"format"`

	// Log output (stdout, stderr, file path)
	Output string `yaml:"output" json:"output"`

	// Log file rotation
	Rotation RotationConfig `yaml:"rotation" json:"rotation"`

	// Enable structured logging
	Structured bool `yaml:"structured" json:"structured"`

	// Enable request logging for API
	RequestLogging bool `yaml:"request_logging" json:"request_logging"`
}

// RotationConfig holds log rotation settings.
type RotationConfig struct {
	// Enable log rotation
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Maximum file size in MB
	MaxSizeMB int `yaml:"max_size_mb" json:"max_size_mb"`

	// Maximum number of backup files
	MaxBackups int `yaml:"max_backups" json:"max_backups"`

	// Maximum age in days
	MaxAgeDays int `yaml:"max_age_days" json:"max_age_days"`

	// Compress rotated files
	Compress bool `yaml:"compress" json:"compress"`
}

// DiscoveryConfig contains discovery engine configuration.
type DiscoveryConfig struct {
	// Predefined networks to discover
	Networks []NetworkConfig `yaml:"networks" json:"networks"`

	// Global exclusions applied to all networks
	GlobalExclusions []string `yaml:"global_exclusions" json:"global_exclusions"`

	// Default discovery settings
	Defaults DiscoveryDefaults `yaml:"defaults" json:"defaults"`

	// Enable automatic network seeding from config
	AutoSeed bool `yaml:"auto_seed" json:"auto_seed"`
}

// NetworkConfig defines a network to be discovered.
type NetworkConfig struct {
	// Network name (must be unique)
	Name string `yaml:"name" json:"name"`

	// CIDR notation (e.g., "192.168.1.0/24")
	CIDR string `yaml:"cidr" json:"cidr"`

	// Discovery method (ping, tcp, arp)
	Method string `yaml:"method" json:"method"`

	// Cron schedule for automatic discovery (optional)
	Schedule string `yaml:"schedule" json:"schedule"`

	// Description of the network
	Description string `yaml:"description" json:"description"`

	// Network-specific exclusions
	Exclusions []string `yaml:"exclusions" json:"exclusions"`

	// Enable/disable this network
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Custom ports for TCP discovery
	Ports string `yaml:"ports" json:"ports"`
}

// DiscoveryDefaults contains default discovery settings.
type DiscoveryDefaults struct {
	// Default discovery method
	Method string `yaml:"method" json:"method"`

	// Default timeout for discovery operations
	Timeout string `yaml:"timeout" json:"timeout"`

	// Default schedule for networks without explicit schedule
	Schedule string `yaml:"schedule" json:"schedule"`

	// Default ports for TCP discovery
	Ports string `yaml:"ports" json:"ports"`
}

// Default returns the default configuration with database credentials
// loaded from environment variables if available.
func Default() *Config {
	return &Config{
		Daemon:    defaultDaemonConfig(),
		Database:  getDatabaseConfigFromEnv(),
		Scanning:  defaultScanningConfig(),
		API:       defaultAPIConfig(),
		Discovery: defaultDiscoveryConfig(),
		Logging:   defaultLoggingConfig(),
	}
}

// defaultDaemonConfig returns the default daemon configuration.
func defaultDaemonConfig() DaemonConfig {
	return DaemonConfig{
		PIDFile:         getEnvString("SCANORAMA_PID_FILE", "/var/run/scanorama.pid"),
		WorkDir:         getEnvString("SCANORAMA_WORK_DIR", "/var/lib/scanorama"),
		User:            getEnvString("SCANORAMA_USER", ""),
		Group:           getEnvString("SCANORAMA_GROUP", ""),
		Daemonize:       false,
		ShutdownTimeout: defaultShutdownTimeoutSec * time.Second,
	}
}

// defaultScanningConfig returns the default scanning configuration.
func defaultScanningConfig() ScanningConfig {
	return ScanningConfig{
		WorkerPoolSize:         defaultWorkerPoolSize,
		DefaultInterval:        1 * time.Hour,
		MaxScanTimeout:         defaultScanTimeoutMin * time.Minute,
		DefaultPorts:           "22,80,443,8080,8443",
		DefaultScanType:        "connect",
		MaxConcurrentTargets:   defaultMaxConcurrentTargets,
		EnableServiceDetection: true,
		EnableOSDetection:      false,
		Retry: RetryConfig{
			MaxRetries:        defaultMaxRetries,
			RetryDelay:        defaultRetryDelaySec * time.Second,
			BackoffMultiplier: defaultBackoffMultiplier,
		},
		RateLimit: RateLimitConfig{
			Enabled:           true,
			RequestsPerSecond: defaultRequestsPerSecond,
			BurstSize:         defaultBurstSize,
		},
	}
}

// defaultAPIConfig returns the default API configuration.
func defaultAPIConfig() APIConfig {
	return APIConfig{
		Enabled:        true,
		Host:           "127.0.0.1",
		Port:           defaultAPIPort,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
		TLS: TLSConfig{
			Enabled:  false,
			CertFile: "",
			KeyFile:  "",
			CAFile:   "",
		},
		AuthEnabled:       false,
		APIKeys:           []string{},
		EnableCORS:        true,
		CORSOrigins:       []string{"*"},
		RateLimitEnabled:  true,
		RateLimitRequests: 100,
		RateLimitWindow:   time.Minute,
		RequestTimeout:    defaultRequestTimeoutSec * time.Second,
		MaxRequestSize:    defaultMaxRequestSizeMB * bytesPerMB, // 1MB
	}
}

// defaultDiscoveryConfig returns the default discovery configuration.
func defaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Networks:         []NetworkConfig{},
		GlobalExclusions: []string{},
		Defaults: DiscoveryDefaults{
			Method:   "ping",
			Timeout:  "30s",
			Schedule: "0 */12 * * *", // Twice daily
			Ports:    "22,80,443,8080,8443,3389,5432,6379",
		},
		AutoSeed: true,
	}
}

// defaultLoggingConfig returns the default logging configuration.
func defaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
		Rotation: RotationConfig{
			Enabled:    false,
			MaxSizeMB:  defaultMaxSizeMB,
			MaxBackups: defaultMaxBackups,
			MaxAgeDays: defaultMaxAgeDays,
			Compress:   true,
		},
		Structured:     false,
		RequestLogging: true,
	}
}

// getEnvString gets a string value from environment variable with fallback.
func getEnvString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt gets an integer value from environment variable with fallback.
func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

// getEnvDuration gets a duration value from environment variable with fallback.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

// getDatabaseConfigFromEnv creates database config from environment variables.
func getDatabaseConfigFromEnv() db.Config {
	return db.Config{
		Host:            getEnvString("SCANORAMA_DB_HOST", "localhost"),
		Port:            getEnvInt("SCANORAMA_DB_PORT", DefaultPostgresPort),
		Database:        getEnvString("SCANORAMA_DB_NAME", ""),
		Username:        getEnvString("SCANORAMA_DB_USER", ""),
		Password:        getEnvString("SCANORAMA_DB_PASSWORD", ""),
		SSLMode:         getEnvString("SCANORAMA_DB_SSLMODE", "disable"),
		MaxOpenConns:    getEnvInt("SCANORAMA_DB_MAX_OPEN_CONNS", DefaultMaxOpenConns),
		MaxIdleConns:    getEnvInt("SCANORAMA_DB_MAX_IDLE_CONNS", DefaultMaxIdleConns),
		ConnMaxLifetime: getEnvDuration("SCANORAMA_DB_CONN_MAX_LIFETIME", DefaultConnMaxLifetime),
		ConnMaxIdleTime: getEnvDuration("SCANORAMA_DB_CONN_MAX_IDLE_TIME", DefaultConnMaxIdleTime),
	}
}

// Load loads configuration from a file.
func Load(path string) (*Config, error) {
	// Validate path for security
	if err := validateConfigPath(path); err != nil {
		return nil, fmt.Errorf("invalid config path: %w", err)
	}

	// Start with defaults (includes environment variables)
	config := Default()

	// Check if file exists and get file info for security validation
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to access config file: %w", err)
	}

	// Validate file size (max 10MB to prevent DoS)
	const maxConfigSize = 10 * 1024 * 1024
	if fileInfo.Size() > maxConfigSize {
		return nil, fmt.Errorf("config file too large: %d bytes (max %d bytes)", fileInfo.Size(), maxConfigSize)
	}

	// Validate file permissions for security
	if err := validateConfigPermissions(fileInfo); err != nil {
		return nil, fmt.Errorf("insecure config file permissions: %w", err)
	}

	// Read file with size limit
	data, err := os.ReadFile(path) //nolint:gosec // path and permissions are validated
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Validate content before parsing
	if err := validateConfigContent(data); err != nil {
		return nil, fmt.Errorf("invalid config content: %w", err)
	}

	// Parse based on file extension with strict options
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := safeYAMLUnmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".json":
		if err := safeJSONUnmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		// Default to YAML with strict parsing
		if err := safeYAMLUnmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config (assumed YAML): %w", err)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Save saves configuration to a file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DefaultDirPermissions); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, DefaultFilePermissions); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// validateConfigPath validates that the config path is safe to use.
func validateConfigPath(path string) error {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check for directory traversal patterns
	if filepath.IsAbs(cleanPath) {
		// For absolute paths, ensure they don't contain .. components
		if filepath.Dir(cleanPath) != filepath.Dir(path) {
			return fmt.Errorf("path contains directory traversal")
		}
	} else {
		// For relative paths, ensure they don't escape the current directory
		if cleanPath != "" && cleanPath[0] == '.' && len(cleanPath) > 1 && cleanPath[1] == '.' {
			return fmt.Errorf("path contains directory traversal")
		}
	}

	// Additional security checks
	if len(path) > maxPathLength {
		return fmt.Errorf("path too long: %d characters (max %d)", len(path), maxPathLength)
	}

	// Check for null bytes (path injection)
	for i, char := range path {
		if char == 0 {
			return fmt.Errorf("null byte in path at position %d", i)
		}
	}

	// Validate file extension
	ext := filepath.Ext(cleanPath)
	allowedExtensions := map[string]bool{
		".yaml": true,
		".yml":  true,
		".json": true,
		"":      true, // Allow no extension for default config files
	}
	if !allowedExtensions[ext] {
		return fmt.Errorf("unsupported config file extension: %s", ext)
	}

	return nil
}

// validateConfigPermissions validates that config file has secure permissions
func validateConfigPermissions(fileInfo os.FileInfo) error {
	mode := fileInfo.Mode()

	// Config files should not be world-readable or writable
	if mode&0o044 != 0 {
		return fmt.Errorf("config file has insecure permissions %o: should not be world-readable", mode&permissionsMask)
	}

	// Config files should not be group-writable unless specifically needed
	if mode&0o020 != 0 {
		return fmt.Errorf("config file has insecure permissions %o: should not be group-writable", mode&permissionsMask)
	}

	return nil
}

// validateConfigContent performs basic validation on config file content
func validateConfigContent(data []byte) error {
	// Check for minimum content
	if len(data) == 0 {
		return fmt.Errorf("config file is empty")
	}

	// Check for extremely large content
	if len(data) > maxContentSize {
		return fmt.Errorf("config content too large: %d bytes (max %d)", len(data), maxContentSize)
	}

	// Check for binary content (basic heuristic)
	nullCount := 0
	for _, b := range data {
		if b == 0 {
			nullCount++
		}
	}
	if nullCount > 0 && len(data) > 0 && float64(nullCount)/float64(len(data)) > 0.01 {
		return fmt.Errorf("config file appears to contain binary data")
	}

	return nil
}

// safeYAMLUnmarshal performs secure YAML unmarshaling with restrictions
func safeYAMLUnmarshal(data []byte, dest interface{}) error {
	// Use secure unmarshaling while allowing field name flexibility for compatibility
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	// Note: KnownFields(true) is disabled to allow field name flexibility
	// Security is maintained through content validation and size limits

	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("YAML decode error: %w", err)
	}

	return nil
}

// safeJSONUnmarshal performs secure JSON unmarshaling with restrictions
func safeJSONUnmarshal(data []byte, dest interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber() // Prevent float precision issues

	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("JSON decode error: %w", err)
	}

	return nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := c.validateDatabase(); err != nil {
		return err
	}
	if err := c.validateScanning(); err != nil {
		return err
	}
	if err := c.validateAPI(); err != nil {
		return err
	}
	if err := c.validateTLS(); err != nil {
		return err
	}
	if err := c.validateLogging(); err != nil {
		return err
	}
	return nil
}

// validateDatabase validates the database configuration.
func (c *Config) validateDatabase() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required (set SCANORAMA_DB_HOST or configure in file)")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required (set SCANORAMA_DB_NAME or configure in file)")
	}
	if c.Database.Username == "" {
		return fmt.Errorf("database username is required (set SCANORAMA_DB_USER or configure in file)")
	}
	return nil
}

// validateScanning validates the scanning configuration.
func (c *Config) validateScanning() error {
	if c.Scanning.WorkerPoolSize <= 0 {
		return fmt.Errorf("worker pool size must be positive")
	}
	if c.Scanning.MaxConcurrentTargets <= 0 {
		return fmt.Errorf("max concurrent targets must be positive")
	}
	if c.Scanning.DefaultInterval <= 0 {
		return fmt.Errorf("default scan interval must be positive")
	}

	// Validate scan type
	validScanTypes := map[string]bool{
		"connect": true,
		"syn":     true,
		"version": true,
	}
	if !validScanTypes[c.Scanning.DefaultScanType] {
		return fmt.Errorf("invalid default scan type: %s", c.Scanning.DefaultScanType)
	}
	return nil
}

// validateAPI validates the API configuration.
func (c *Config) validateAPI() error {
	if !c.API.Enabled {
		return nil
	}

	if c.API.Port <= 0 || c.API.Port > 65535 {
		return fmt.Errorf("API port must be between 1 and 65535")
	}
	if c.API.Host == "" {
		return fmt.Errorf("API host address is required when API is enabled")
	}

	// Validate timeouts
	if c.API.ReadTimeout <= 0 {
		return fmt.Errorf("API read timeout must be positive")
	}
	if c.API.WriteTimeout <= 0 {
		return fmt.Errorf("API write timeout must be positive")
	}
	if c.API.IdleTimeout <= 0 {
		return fmt.Errorf("API idle timeout must be positive")
	}

	// Validate max header bytes
	if c.API.MaxHeaderBytes <= 0 {
		return fmt.Errorf("API max header bytes must be positive")
	}

	// Validate rate limiting
	if err := c.validateAPIRateLimiting(); err != nil {
		return err
	}

	// Validate authentication
	if c.API.AuthEnabled && len(c.API.APIKeys) == 0 {
		return fmt.Errorf("at least one API key must be provided when authentication is enabled")
	}

	return nil
}

// validateAPIRateLimiting validates the API rate limiting configuration.
func (c *Config) validateAPIRateLimiting() error {
	if !c.API.RateLimitEnabled {
		return nil
	}
	if c.API.RateLimitRequests <= 0 {
		return fmt.Errorf("rate limit requests must be positive when rate limiting is enabled")
	}
	if c.API.RateLimitWindow <= 0 {
		return fmt.Errorf("rate limit window must be positive when rate limiting is enabled")
	}
	return nil
}

// validateTLS validates the TLS configuration.
func (c *Config) validateTLS() error {
	if c.API.TLS.Enabled {
		if c.API.TLS.CertFile == "" {
			return fmt.Errorf("TLS certificate file is required when TLS is enabled")
		}
		if c.API.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}
	return nil
}

// validateLogging validates the logging configuration.
func (c *Config) validateLogging() error {
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"text": true,
		"json": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}
	return nil
}

// GetDatabaseConfig returns the database configuration.
func (c *Config) GetDatabaseConfig() db.Config {
	return c.Database
}

// IsDaemonMode returns true if running in daemon mode.
func (c *Config) IsDaemonMode() bool {
	return c.Daemon.Daemonize
}

// GetAPIAddress returns the full API address.
func (c *Config) GetAPIAddress() string {
	return fmt.Sprintf("%s:%d", c.API.Host, c.API.Port)
}

// IsAPIEnabled returns true if API server is enabled.
func (c *Config) IsAPIEnabled() bool {
	return c.API.Enabled
}

// GetLogOutput returns the log output destination.
func (c *Config) GetLogOutput() string {
	return c.Logging.Output
}
