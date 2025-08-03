package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/anstrom/scanorama/internal/db"
)

// Config represents the complete daemon configuration
type Config struct {
	// Daemon configuration
	Daemon DaemonConfig `yaml:"daemon" json:"daemon"`

	// Database configuration
	Database db.Config `yaml:"database" json:"database"`

	// Scanning configuration
	Scanning ScanningConfig `yaml:"scanning" json:"scanning"`

	// API configuration
	API APIConfig `yaml:"api" json:"api"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging" json:"logging"`
}

// DaemonConfig holds daemon-specific settings
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

// ScanningConfig holds scanning-related settings
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

// RetryConfig holds retry settings for failed scans
type RetryConfig struct {
	// Maximum number of retries
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// Delay between retries
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`

	// Exponential backoff multiplier
	BackoffMultiplier float64 `yaml:"backoff_multiplier" json:"backoff_multiplier"`
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	// Enable rate limiting
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Requests per second
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second"`

	// Burst size
	BurstSize int `yaml:"burst_size" json:"burst_size"`
}

// APIConfig holds API server settings
type APIConfig struct {
	// Enable API server
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Listen address
	ListenAddr string `yaml:"listen_addr" json:"listen_addr"`

	// Listen port
	Port int `yaml:"port" json:"port"`

	// Enable TLS
	TLS TLSConfig `yaml:"tls" json:"tls"`

	// API key for authentication
	APIKey string `yaml:"api_key" json:"api_key"`

	// CORS settings
	CORS CORSConfig `yaml:"cors" json:"cors"`

	// Request timeout
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`

	// Maximum request size
	MaxRequestSize int64 `yaml:"max_request_size" json:"max_request_size"`
}

// TLSConfig holds TLS settings
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

// CORSConfig holds CORS settings
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

// LoggingConfig holds logging settings
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

// RotationConfig holds log rotation settings
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

// Default returns a configuration with sensible defaults
func Default() *Config {
	return &Config{
		Daemon: DaemonConfig{
			PIDFile:         "/var/run/scanorama.pid",
			WorkDir:         "/var/lib/scanorama",
			User:            "",
			Group:           "",
			Daemonize:       false,
			ShutdownTimeout: 30 * time.Second,
		},
		Database: db.DefaultConfig(),
		Scanning: ScanningConfig{
			WorkerPoolSize:         10,
			DefaultInterval:        1 * time.Hour,
			MaxScanTimeout:         10 * time.Minute,
			DefaultPorts:           "22,80,443,8080,8443",
			DefaultScanType:        "connect",
			MaxConcurrentTargets:   100,
			EnableServiceDetection: true,
			EnableOSDetection:      false,
			Retry: RetryConfig{
				MaxRetries:        3,
				RetryDelay:        30 * time.Second,
				BackoffMultiplier: 2.0,
			},
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerSecond: 100,
				BurstSize:         200,
			},
		},
		API: APIConfig{
			Enabled:    true,
			ListenAddr: "127.0.0.1",
			Port:       8080,
			TLS: TLSConfig{
				Enabled:  false,
				CertFile: "",
				KeyFile:  "",
				CAFile:   "",
			},
			APIKey: "",
			CORS: CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Content-Type", "Authorization"},
			},
			RequestTimeout: 30 * time.Second,
			MaxRequestSize: 1024 * 1024, // 1MB
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
			Rotation: RotationConfig{
				Enabled:    false,
				MaxSizeMB:  100,
				MaxBackups: 5,
				MaxAgeDays: 30,
				Compress:   true,
			},
			Structured:     false,
			RequestLogging: true,
		},
	}
}

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	// Start with defaults
	config := Default()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config, nil // Return defaults if no config file
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse based on file extension
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".json":
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	default:
		// Default to YAML
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config (assumed YAML): %w", err)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate database configuration
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Database.Username == "" {
		return fmt.Errorf("database username is required")
	}

	// Validate scanning configuration
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

	// Validate API configuration
	if c.API.Enabled {
		if c.API.Port <= 0 || c.API.Port > 65535 {
			return fmt.Errorf("API port must be between 1 and 65535")
		}
		if c.API.ListenAddr == "" {
			return fmt.Errorf("API listen address is required when API is enabled")
		}
	}

	// Validate TLS configuration
	if c.API.TLS.Enabled {
		if c.API.TLS.CertFile == "" {
			return fmt.Errorf("TLS certificate file is required when TLS is enabled")
		}
		if c.API.TLS.KeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}

	// Validate logging configuration
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

// GetDatabaseConfig returns the database configuration
func (c *Config) GetDatabaseConfig() db.Config {
	return c.Database
}

// IsDaemonMode returns true if running in daemon mode
func (c *Config) IsDaemonMode() bool {
	return c.Daemon.Daemonize
}

// GetAPIAddress returns the full API address
func (c *Config) GetAPIAddress() string {
	return fmt.Sprintf("%s:%d", c.API.ListenAddr, c.API.Port)
}

// IsAPIEnabled returns true if API server is enabled
func (c *Config) IsAPIEnabled() bool {
	return c.API.Enabled
}

// GetLogOutput returns the log output destination
func (c *Config) GetLogOutput() string {
	return c.Logging.Output
}
