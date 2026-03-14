package config

import (
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

// validFullConfig returns a fully populated, valid Config for use in tests.
func validFullConfig() *Config {
	return &Config{
		Daemon: DaemonConfig{
			PIDFile:         "/var/run/scanorama.pid",
			WorkDir:         "/var/lib/scanorama",
			User:            "nobody",
			Group:           "nobody",
			Daemonize:       false,
			ShutdownTimeout: 30 * time.Second,
		},
		Database: db.Config{
			Host:            "localhost",
			Port:            5432,
			Database:        "scanorama",
			Username:        "scanorama",
			Password:        "secret",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
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
			Enabled:           true,
			Host:              "127.0.0.1",
			Port:              8080,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1 << 20,
			TLS:               TLSConfig{Enabled: false},
			AuthEnabled:       false,
			APIKeys:           []string{},
			EnableCORS:        true,
			CORSOrigins:       []string{"*"},
			RateLimitEnabled:  true,
			RateLimitRequests: 100,
			RateLimitWindow:   time.Minute,
			RequestTimeout:    30 * time.Second,
			MaxRequestSize:    1024 * 1024,
		},
		Discovery: DiscoveryConfig{
			Networks:         []NetworkConfig{},
			GlobalExclusions: []string{},
			Defaults: DiscoveryDefaults{
				Method:   "ping",
				Timeout:  "30s",
				Schedule: "0 */12 * * *",
				Ports:    "22,80,443",
			},
			AutoSeed: true,
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

func TestValidateConfig_ValidFullConfig(t *testing.T) {
	cfg := validFullConfig()
	result := ValidateConfig(cfg)

	if result.HasErrors() {
		t.Errorf("expected valid full config to pass validation, got errors: %s", result.Error())
	}
	if !result.IsValid() {
		t.Errorf("expected IsValid()=true for valid config")
	}
	if result.AsError() != nil {
		t.Errorf("expected AsError()=nil for valid config, got: %v", result.AsError())
	}
}

func TestValidateConfig_NilConfig(t *testing.T) {
	result := ValidateConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil config")
	}
	if result.IsValid() {
		t.Fatal("expected IsValid()=false for nil config")
	}
}

// --- Database validation ---

func TestValidateDatabaseConfig_MissingHost(t *testing.T) {
	cfg := &db.Config{
		Host:     "",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "host")
}

func TestValidateDatabaseConfig_MissingDatabase(t *testing.T) {
	cfg := &db.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "disable",
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "database")
}

func TestValidateDatabaseConfig_MissingUsername(t *testing.T) {
	cfg := &db.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "",
		Password: "testpass",
		SSLMode:  "disable",
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "username")
}

func TestValidateDatabaseConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"too large port", 70000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &db.Config{
				Host:     "localhost",
				Port:     tt.port,
				Database: "testdb",
				Username: "testuser",
				Password: "testpass",
				SSLMode:  "disable",
			}
			result := ValidateDatabaseConfig(cfg)
			assertHasError(t, result, "port")
		})
	}
}

func TestValidateDatabaseConfig_EmptyPasswordWarning(t *testing.T) {
	cfg := &db.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		Password: "",
		SSLMode:  "disable",
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasWarning(t, result, "password")
}

func TestValidateDatabaseConfig_InvalidSSLMode(t *testing.T) {
	cfg := &db.Config{
		Host:     "localhost",
		Port:     5432,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
		SSLMode:  "bogus",
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "ssl_mode")
}

func TestValidateDatabaseConfig_NegativeConnPool(t *testing.T) {
	cfg := &db.Config{
		Host:         "localhost",
		Port:         5432,
		Database:     "testdb",
		Username:     "testuser",
		Password:     "testpass",
		SSLMode:      "disable",
		MaxOpenConns: -1,
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "max_open_conns")
}

func TestValidateDatabaseConfig_NegativeConnLifetime(t *testing.T) {
	cfg := &db.Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "testdb",
		Username:        "testuser",
		Password:        "testpass",
		SSLMode:         "disable",
		ConnMaxLifetime: -1 * time.Second,
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "conn_max_lifetime")
}

func TestValidateDatabaseConfig_NegativeConnIdleTime(t *testing.T) {
	cfg := &db.Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "testdb",
		Username:        "testuser",
		Password:        "testpass",
		SSLMode:         "disable",
		ConnMaxIdleTime: -1 * time.Second,
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasError(t, result, "conn_max_idle_time")
}

func TestValidateDatabaseConfig_IdleExceedsOpenWarning(t *testing.T) {
	cfg := &db.Config{
		Host:         "localhost",
		Port:         5432,
		Database:     "testdb",
		Username:     "testuser",
		Password:     "testpass",
		SSLMode:      "disable",
		MaxOpenConns: 5,
		MaxIdleConns: 10,
	}
	result := ValidateDatabaseConfig(cfg)
	assertHasWarning(t, result, "max_idle_conns")
}

func TestValidateDatabaseConfig_CollectsMultipleErrors(t *testing.T) {
	cfg := &db.Config{
		Host:     "",
		Port:     0,
		Database: "",
		Username: "",
		Password: "",
		SSLMode:  "bogus",
	}
	result := ValidateDatabaseConfig(cfg)
	if len(result.Errors) < 4 {
		t.Errorf("expected at least 4 errors for completely invalid database config, got %d: %s",
			len(result.Errors), result.Error())
	}
}

func TestValidateDatabaseConfig_Nil(t *testing.T) {
	result := ValidateDatabaseConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil database config")
	}
}

// --- Logging validation ---

func TestValidateLoggingConfig_InvalidLevel(t *testing.T) {
	// Note: the validator lowercases before checking, so "INFO" -> "info" is valid.
	// "Warning" -> "warning" is invalid (valid values are warn, not warning).
	tests := []string{"trace", "fatal", "Warning", "", "verbose"}
	for _, lvl := range tests {
		t.Run("level_"+lvl, func(t *testing.T) {
			cfg := &LoggingConfig{
				Level:  lvl,
				Format: "text",
				Output: "stdout",
			}
			result := ValidateLoggingConfig(cfg)
			assertHasError(t, result, "level")
		})
	}
}

func TestValidateLoggingConfig_ValidLevels(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error"} {
		t.Run("level_"+lvl, func(t *testing.T) {
			cfg := &LoggingConfig{
				Level:  lvl,
				Format: "text",
				Output: "stdout",
			}
			result := ValidateLoggingConfig(cfg)
			assertNoError(t, result, "level")
		})
	}
}

func TestValidateLoggingConfig_InvalidFormat(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "xml",
		Output: "stdout",
	}
	result := ValidateLoggingConfig(cfg)
	assertHasError(t, result, "format")
}

func TestValidateLoggingConfig_EmptyOutputWarning(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "",
	}
	result := ValidateLoggingConfig(cfg)
	assertHasWarning(t, result, "output")
}

func TestValidateLoggingConfig_RotationWithStdout(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
		Rotation: RotationConfig{
			Enabled:    true,
			MaxSizeMB:  100,
			MaxBackups: 5,
			MaxAgeDays: 30,
		},
	}
	result := ValidateLoggingConfig(cfg)
	assertHasWarning(t, result, "rotation.enabled")
}

func TestValidateLoggingConfig_RotationInvalidMaxSize(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "/var/log/scanorama.log",
		Rotation: RotationConfig{
			Enabled:    true,
			MaxSizeMB:  0,
			MaxBackups: 5,
			MaxAgeDays: 30,
		},
	}
	result := ValidateLoggingConfig(cfg)
	assertHasError(t, result, "rotation.max_size_mb")
}

func TestValidateLoggingConfig_RotationNegativeBackups(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "/var/log/scanorama.log",
		Rotation: RotationConfig{
			Enabled:    true,
			MaxSizeMB:  100,
			MaxBackups: -1,
			MaxAgeDays: 30,
		},
	}
	result := ValidateLoggingConfig(cfg)
	assertHasError(t, result, "rotation.max_backups")
}

func TestValidateLoggingConfig_RotationNegativeAge(t *testing.T) {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "/var/log/scanorama.log",
		Rotation: RotationConfig{
			Enabled:    true,
			MaxSizeMB:  100,
			MaxBackups: 5,
			MaxAgeDays: -1,
		},
	}
	result := ValidateLoggingConfig(cfg)
	assertHasError(t, result, "rotation.max_age_days")
}

func TestValidateLoggingConfig_Nil(t *testing.T) {
	result := ValidateLoggingConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil logging config")
	}
}

func TestValidateLoggingConfig_DirectoryTraversal(t *testing.T) {
	// Use a relative path so filepath.Clean preserves the ".." components
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "../../etc/scanorama.log",
	}
	result := ValidateLoggingConfig(cfg)
	assertHasError(t, result, "output")
}

// --- Scanning validation ---

func TestValidateScanningConfig_InvalidScanType(t *testing.T) {
	// Note: the validator lowercases before checking, so "CONNECT" -> "connect" is valid.
	tests := []string{"udp", "stealth", "", "bogus"}
	for _, st := range tests {
		t.Run("type_"+st, func(t *testing.T) {
			cfg := validScanningConfig()
			cfg.DefaultScanType = st
			result := ValidateScanningConfig(cfg)
			assertHasError(t, result, "default_scan_type")
		})
	}
}

func TestValidateScanningConfig_ValidScanTypes(t *testing.T) {
	for _, st := range []string{"connect", "syn", "version"} {
		t.Run("type_"+st, func(t *testing.T) {
			cfg := validScanningConfig()
			cfg.DefaultScanType = st
			result := ValidateScanningConfig(cfg)
			assertNoError(t, result, "default_scan_type")
		})
	}
}

func TestValidateScanningConfig_ZeroWorkerPool(t *testing.T) {
	cfg := validScanningConfig()
	cfg.WorkerPoolSize = 0
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "worker_pool_size")
}

func TestValidateScanningConfig_NegativeWorkerPool(t *testing.T) {
	cfg := validScanningConfig()
	cfg.WorkerPoolSize = -5
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "worker_pool_size")
}

func TestValidateScanningConfig_ZeroConcurrentTargets(t *testing.T) {
	cfg := validScanningConfig()
	cfg.MaxConcurrentTargets = 0
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "max_concurrent_targets")
}

func TestValidateScanningConfig_NegativeDefaultInterval(t *testing.T) {
	cfg := validScanningConfig()
	cfg.DefaultInterval = -1 * time.Hour
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "default_interval")
}

func TestValidateScanningConfig_NegativeMaxScanTimeout(t *testing.T) {
	cfg := validScanningConfig()
	cfg.MaxScanTimeout = -1 * time.Minute
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "max_scan_timeout")
}

func TestValidateScanningConfig_ZeroMaxScanTimeoutWarning(t *testing.T) {
	cfg := validScanningConfig()
	cfg.MaxScanTimeout = 0
	result := ValidateScanningConfig(cfg)
	assertHasWarning(t, result, "max_scan_timeout")
}

func TestValidateScanningConfig_NegativeRetryDelay(t *testing.T) {
	cfg := validScanningConfig()
	cfg.Retry.RetryDelay = -1 * time.Second
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "retry.retry_delay")
}

func TestValidateScanningConfig_NegativeRetries(t *testing.T) {
	cfg := validScanningConfig()
	cfg.Retry.MaxRetries = -1
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "retry.max_retries")
}

func TestValidateScanningConfig_NegativeBackoffMultiplier(t *testing.T) {
	cfg := validScanningConfig()
	cfg.Retry.BackoffMultiplier = -1.0
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "retry.backoff_multiplier")
}

func TestValidateScanningConfig_SmallBackoffWarning(t *testing.T) {
	cfg := validScanningConfig()
	cfg.Retry.BackoffMultiplier = 0.5
	result := ValidateScanningConfig(cfg)
	assertHasWarning(t, result, "retry.backoff_multiplier")
}

func TestValidateScanningConfig_RateLimitEnabledZeroRPS(t *testing.T) {
	cfg := validScanningConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.RequestsPerSecond = 0
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "rate_limit.requests_per_second")
}

func TestValidateScanningConfig_RateLimitEnabledZeroBurst(t *testing.T) {
	cfg := validScanningConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.BurstSize = 0
	result := ValidateScanningConfig(cfg)
	assertHasError(t, result, "rate_limit.burst_size")
}

func TestValidateScanningConfig_LargeWorkerPoolWarning(t *testing.T) {
	cfg := validScanningConfig()
	cfg.WorkerPoolSize = 5000
	result := ValidateScanningConfig(cfg)
	assertHasWarning(t, result, "worker_pool_size")
}

func TestValidateScanningConfig_Nil(t *testing.T) {
	result := ValidateScanningConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil scanning config")
	}
}

// --- API validation ---

func TestValidateAPIConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 70000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAPIConfig()
			cfg.Port = tt.port
			result := ValidateAPIConfig(cfg)
			assertHasError(t, result, "port")
		})
	}
}

func TestValidateAPIConfig_PrivilegedPortWarning(t *testing.T) {
	cfg := validAPIConfig()
	cfg.Port = 443
	result := ValidateAPIConfig(cfg)
	assertHasWarning(t, result, "port")
}

func TestValidateAPIConfig_ValidPort(t *testing.T) {
	cfg := validAPIConfig()
	cfg.Port = 8080
	result := ValidateAPIConfig(cfg)
	assertNoError(t, result, "port")
}

func TestValidateAPIConfig_EmptyHost(t *testing.T) {
	cfg := validAPIConfig()
	cfg.Host = ""
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "host")
}

func TestValidateAPIConfig_NegativeReadTimeout(t *testing.T) {
	cfg := validAPIConfig()
	cfg.ReadTimeout = -1 * time.Second
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "read_timeout")
}

func TestValidateAPIConfig_NegativeWriteTimeout(t *testing.T) {
	cfg := validAPIConfig()
	cfg.WriteTimeout = -1 * time.Second
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "write_timeout")
}

func TestValidateAPIConfig_NegativeIdleTimeout(t *testing.T) {
	cfg := validAPIConfig()
	cfg.IdleTimeout = -1 * time.Second
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "idle_timeout")
}

func TestValidateAPIConfig_NegativeRequestTimeout(t *testing.T) {
	cfg := validAPIConfig()
	cfg.RequestTimeout = -1 * time.Second
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "request_timeout")
}

func TestValidateAPIConfig_ZeroMaxHeaderBytes(t *testing.T) {
	cfg := validAPIConfig()
	cfg.MaxHeaderBytes = 0
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "max_header_bytes")
}

func TestValidateAPIConfig_NegativeMaxRequestSize(t *testing.T) {
	cfg := validAPIConfig()
	cfg.MaxRequestSize = -1
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "max_request_size")
}

func TestValidateAPIConfig_TLSEnabledNoCert(t *testing.T) {
	cfg := validAPIConfig()
	cfg.TLS.Enabled = true
	cfg.TLS.CertFile = ""
	cfg.TLS.KeyFile = "/path/to/key.pem"
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "tls.cert_file")
}

func TestValidateAPIConfig_TLSEnabledNoKey(t *testing.T) {
	cfg := validAPIConfig()
	cfg.TLS.Enabled = true
	cfg.TLS.CertFile = "/path/to/cert.pem"
	cfg.TLS.KeyFile = ""
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "tls.key_file")
}

func TestValidateAPIConfig_TLSEnabledBothMissing(t *testing.T) {
	cfg := validAPIConfig()
	cfg.TLS.Enabled = true
	cfg.TLS.CertFile = ""
	cfg.TLS.KeyFile = ""
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "tls.cert_file")
	assertHasError(t, result, "tls.key_file")
}

func TestValidateAPIConfig_TLSDisabledNoCertOK(t *testing.T) {
	cfg := validAPIConfig()
	cfg.TLS.Enabled = false
	cfg.TLS.CertFile = ""
	cfg.TLS.KeyFile = ""
	result := ValidateAPIConfig(cfg)
	assertNoError(t, result, "tls.cert_file")
	assertNoError(t, result, "tls.key_file")
}

func TestValidateAPIConfig_AuthEnabledNoKeys(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = true
	cfg.APIKeys = []string{}
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "api_keys")
}

func TestValidateAPIConfig_AuthEnabledWithKeys(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = true
	cfg.APIKeys = []string{"a-very-long-secret-key-1234567890"}
	result := ValidateAPIConfig(cfg)
	assertNoError(t, result, "api_keys")
}

func TestValidateAPIConfig_AuthEnabledEmptyKey(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = true
	cfg.APIKeys = []string{""}
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "api_keys[0]")
}

func TestValidateAPIConfig_AuthEnabledShortKeyWarning(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = true
	cfg.APIKeys = []string{"short"}
	result := ValidateAPIConfig(cfg)
	assertHasWarning(t, result, "api_keys[0]")
}

func TestValidateAPIConfig_RateLimitEnabledZeroRequests(t *testing.T) {
	cfg := validAPIConfig()
	cfg.RateLimitEnabled = true
	cfg.RateLimitRequests = 0
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "rate_limit_requests")
}

func TestValidateAPIConfig_RateLimitEnabledZeroWindow(t *testing.T) {
	cfg := validAPIConfig()
	cfg.RateLimitEnabled = true
	cfg.RateLimitWindow = 0
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "rate_limit_window")
}

func TestValidateAPIConfig_DisabledAPISkipsValidation(t *testing.T) {
	cfg := &APIConfig{
		Enabled: false,
		Port:    0, // would be invalid if API were enabled
		Host:    "",
	}
	result := ValidateAPIConfig(cfg)
	if result.HasErrors() {
		t.Errorf("expected no errors for disabled API, got: %s", result.Error())
	}
}

func TestValidateAPIConfig_DisabledAPITLSWarning(t *testing.T) {
	cfg := &APIConfig{
		Enabled: false,
		TLS:     TLSConfig{Enabled: true},
	}
	result := ValidateAPIConfig(cfg)
	assertHasWarning(t, result, "tls.enabled")
}

func TestValidateAPIConfig_NoAuthNoTLSWarning(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = false
	cfg.TLS.Enabled = false
	result := ValidateAPIConfig(cfg)
	assertHasWarning(t, result, "auth_enabled")
}

func TestValidateAPIConfig_WildcardCORSWithAuthWarning(t *testing.T) {
	cfg := validAPIConfig()
	cfg.AuthEnabled = true
	cfg.APIKeys = []string{"a-very-long-secret-key-1234567890"}
	cfg.EnableCORS = true
	cfg.CORSOrigins = []string{"*"}
	cfg.TLS.Enabled = true
	cfg.TLS.CertFile = "/path/to/cert.pem"
	cfg.TLS.KeyFile = "/path/to/key.pem"
	result := ValidateAPIConfig(cfg)
	assertHasWarning(t, result, "cors_origins")
}

func TestValidateAPIConfig_Nil(t *testing.T) {
	result := ValidateAPIConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil API config")
	}
}

func TestValidateAPIConfig_TLSDirectoryTraversal(t *testing.T) {
	// Use a relative path so filepath.Clean preserves the ".." components
	cfg := validAPIConfig()
	cfg.TLS.Enabled = true
	cfg.TLS.CertFile = "../../etc/shadow"
	cfg.TLS.KeyFile = "/etc/ssl/key.pem"
	result := ValidateAPIConfig(cfg)
	assertHasError(t, result, "tls.cert_file")
}

// --- Daemon validation ---

func TestValidateDaemonConfig_NegativeShutdownTimeout(t *testing.T) {
	cfg := &DaemonConfig{
		ShutdownTimeout: -5 * time.Second,
	}
	result := ValidateDaemonConfig(cfg)
	assertHasError(t, result, "shutdown_timeout")
}

func TestValidateDaemonConfig_ZeroShutdownTimeoutWarning(t *testing.T) {
	cfg := &DaemonConfig{
		ShutdownTimeout: 0,
	}
	result := ValidateDaemonConfig(cfg)
	assertHasWarning(t, result, "shutdown_timeout")
}

func TestValidateDaemonConfig_DirectoryTraversalPIDFile(t *testing.T) {
	// Use a relative path so filepath.Clean preserves the ".." components
	cfg := &DaemonConfig{
		PIDFile:         "../../etc/scanorama.pid",
		ShutdownTimeout: 30 * time.Second,
	}
	result := ValidateDaemonConfig(cfg)
	assertHasError(t, result, "pid_file")
}

func TestValidateDaemonConfig_DirectoryTraversalWorkDir(t *testing.T) {
	// Use a relative path so filepath.Clean preserves the ".." components
	cfg := &DaemonConfig{
		WorkDir:         "../../etc",
		ShutdownTimeout: 30 * time.Second,
	}
	result := ValidateDaemonConfig(cfg)
	assertHasError(t, result, "work_dir")
}

func TestValidateDaemonConfig_DaemonizeNoPIDWarning(t *testing.T) {
	cfg := &DaemonConfig{
		Daemonize:       true,
		PIDFile:         "",
		ShutdownTimeout: 30 * time.Second,
	}
	result := ValidateDaemonConfig(cfg)
	assertHasWarning(t, result, "pid_file")
}

func TestValidateDaemonConfig_Nil(t *testing.T) {
	result := ValidateDaemonConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil daemon config")
	}
}

func TestValidateDaemonConfig_Valid(t *testing.T) {
	cfg := &DaemonConfig{
		PIDFile:         "/var/run/scanorama.pid",
		WorkDir:         "/var/lib/scanorama",
		User:            "nobody",
		Group:           "nobody",
		Daemonize:       false,
		ShutdownTimeout: 30 * time.Second,
	}
	result := ValidateDaemonConfig(cfg)
	if result.HasErrors() {
		t.Errorf("expected no errors for valid daemon config, got: %s", result.Error())
	}
}

// --- Discovery validation ---

func TestValidateDiscoveryConfig_InvalidDefaultMethod(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{
			Method:  "bogus",
			Timeout: "30s",
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "defaults.method")
}

func TestValidateDiscoveryConfig_InvalidDefaultTimeout(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{
			Method:  "ping",
			Timeout: "not-a-duration",
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "defaults.timeout")
}

func TestValidateDiscoveryConfig_DuplicateNetworkNames(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{Name: "net1", CIDR: "192.168.1.0/24", Enabled: true},
			{Name: "net1", CIDR: "10.0.0.0/8", Enabled: true},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[1].name")
}

func TestValidateDiscoveryConfig_InvalidCIDR(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{Name: "bad", CIDR: "not-a-cidr"},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[0].cidr")
}

func TestValidateDiscoveryConfig_EmptyNetworkName(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{Name: "", CIDR: "192.168.1.0/24"},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[0].name")
}

func TestValidateDiscoveryConfig_EmptyNetworkCIDR(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{Name: "test", CIDR: ""},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[0].cidr")
}

func TestValidateDiscoveryConfig_InvalidNetworkMethod(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{Name: "test", CIDR: "192.168.1.0/24", Method: "snmp"},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[0].method")
}

func TestValidateDiscoveryConfig_InvalidExclusion(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{
				Name:       "test",
				CIDR:       "192.168.1.0/24",
				Exclusions: []string{"not-valid"},
			},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "networks[0].exclusions[0]")
}

func TestValidateDiscoveryConfig_ValidExclusionIP(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{
				Name:       "test",
				CIDR:       "192.168.1.0/24",
				Exclusions: []string{"192.168.1.1"},
			},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertNoError(t, result, "networks[0].exclusions[0]")
}

func TestValidateDiscoveryConfig_ValidExclusionCIDR(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		Networks: []NetworkConfig{
			{
				Name:       "test",
				CIDR:       "192.168.1.0/24",
				Exclusions: []string{"192.168.1.0/28"},
			},
		},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertNoError(t, result, "networks[0].exclusions[0]")
}

func TestValidateDiscoveryConfig_InvalidGlobalExclusion(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults:         DiscoveryDefaults{Method: "ping", Timeout: "30s"},
		GlobalExclusions: []string{"not-valid"},
	}
	result := ValidateDiscoveryConfig(cfg)
	assertHasError(t, result, "global_exclusions[0]")
}

func TestValidateDiscoveryConfig_Nil(t *testing.T) {
	result := ValidateDiscoveryConfig(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil discovery config")
	}
}

func TestValidateDiscoveryConfig_EmptyValid(t *testing.T) {
	cfg := &DiscoveryConfig{
		Defaults: DiscoveryDefaults{Method: "ping", Timeout: "30s"},
	}
	result := ValidateDiscoveryConfig(cfg)
	if result.HasErrors() {
		t.Errorf("expected no errors for empty discovery config, got: %s", result.Error())
	}
}

// --- ValidateConfig (full integration) ---

func TestValidateConfig_CollectsAllSectionErrors(t *testing.T) {
	cfg := &Config{
		Database: db.Config{
			Host:     "", // error
			Port:     0,  // error
			Database: "", // error
			Username: "", // error
		},
		Scanning: ScanningConfig{
			WorkerPoolSize:       0,  // error
			MaxConcurrentTargets: 0,  // error
			DefaultScanType:      "", // error
		},
		API: APIConfig{
			Enabled: true,
			Port:    0,  // error
			Host:    "", // error
		},
		Logging: LoggingConfig{
			Level:  "bogus", // error
			Format: "xml",   // error
		},
	}
	result := ValidateConfig(cfg)
	if !result.HasErrors() {
		t.Fatal("expected errors from multi-section invalid config")
	}
	// Should have errors from multiple sections
	sections := make(map[string]bool)
	for _, e := range result.Errors {
		sections[e.Section] = true
	}
	for _, expected := range []string{"database", "scanning", "api", "logging"} {
		if !sections[expected] {
			t.Errorf("expected errors from section %q, but found none", expected)
		}
	}
}

func TestValidateConfig_DefaultConfigPasses(t *testing.T) {
	cfg := Default()
	// Default() doesn't set database credentials from env by default,
	// so we need to fill them in for a valid config
	cfg.Database.Host = "localhost"
	cfg.Database.Database = "scanorama"
	cfg.Database.Username = "scanorama"
	cfg.Database.Password = "secret"

	result := ValidateConfig(cfg)
	if result.HasErrors() {
		t.Errorf("expected config with defaults applied to pass validation, got errors: %s", result.Error())
	}
}

// --- ValidateAndNormalize ---

func TestValidateAndNormalize_NilConfig(t *testing.T) {
	result := ValidateAndNormalize(nil)
	if !result.HasErrors() {
		t.Fatal("expected error for nil config")
	}
}

func TestValidateAndNormalize_NormalizesValues(t *testing.T) {
	cfg := validFullConfig()
	cfg.Scanning.DefaultScanType = "  CONNECT  "
	cfg.Logging.Level = "  INFO  "
	cfg.Logging.Format = "  JSON  "
	cfg.Daemon.PIDFile = "/var/run/../run/scanorama.pid"
	cfg.Discovery.Defaults.Method = "  PING  "

	result := ValidateAndNormalize(cfg)
	if result.HasErrors() {
		t.Errorf("expected normalization to fix issues, got errors: %s", result.Error())
	}

	if cfg.Scanning.DefaultScanType != "connect" {
		t.Errorf("expected scan type to be normalized to 'connect', got %q", cfg.Scanning.DefaultScanType)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level to be normalized to 'info', got %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format to be normalized to 'json', got %q", cfg.Logging.Format)
	}
	if cfg.Discovery.Defaults.Method != "ping" {
		t.Errorf("expected discovery method to be normalized to 'ping', got %q", cfg.Discovery.Defaults.Method)
	}
}

func TestValidateAndNormalize_CleansFilePaths(t *testing.T) {
	cfg := validFullConfig()
	cfg.Daemon.PIDFile = "/var/run/./scanorama.pid"
	cfg.Daemon.WorkDir = "/var/lib/./scanorama/"
	cfg.API.TLS.Enabled = true
	cfg.API.TLS.CertFile = "/etc/ssl/./certs/cert.pem"
	cfg.API.TLS.KeyFile = "/etc/ssl/./private/key.pem"
	cfg.API.AuthEnabled = true
	cfg.API.APIKeys = []string{"a-very-long-secret-key-1234567890"}

	_ = ValidateAndNormalize(cfg)

	if cfg.Daemon.PIDFile != "/var/run/scanorama.pid" {
		t.Errorf("expected PIDFile to be cleaned, got %q", cfg.Daemon.PIDFile)
	}
	if cfg.API.TLS.CertFile != "/etc/ssl/certs/cert.pem" {
		t.Errorf("expected CertFile to be cleaned, got %q", cfg.API.TLS.CertFile)
	}
	if cfg.API.TLS.KeyFile != "/etc/ssl/private/key.pem" {
		t.Errorf("expected KeyFile to be cleaned, got %q", cfg.API.TLS.KeyFile)
	}
}

func TestValidateAndNormalize_NormalizesNetworks(t *testing.T) {
	cfg := validFullConfig()
	cfg.Discovery.Networks = []NetworkConfig{
		{
			Name:   "  test-net  ",
			CIDR:   " 192.168.1.0/24 ",
			Method: " TCP ",
		},
	}

	_ = ValidateAndNormalize(cfg)

	if cfg.Discovery.Networks[0].Name != "test-net" {
		t.Errorf("expected network name trimmed, got %q", cfg.Discovery.Networks[0].Name)
	}
	if cfg.Discovery.Networks[0].CIDR != "192.168.1.0/24" {
		t.Errorf("expected CIDR trimmed, got %q", cfg.Discovery.Networks[0].CIDR)
	}
	if cfg.Discovery.Networks[0].Method != "tcp" {
		t.Errorf("expected method lowercased, got %q", cfg.Discovery.Networks[0].Method)
	}
}

// --- ValidationResult type ---

func TestValidationResult_AllIssues(t *testing.T) {
	result := &ValidationResult{}
	result.addError("db", "host", "missing")
	result.addWarning("db", "password", "empty")
	result.addError("api", "port", "invalid")

	all := result.AllIssues()
	if len(all) != 3 {
		t.Errorf("expected 3 total issues, got %d", len(all))
	}
}

func TestValidationResult_Merge(t *testing.T) {
	r1 := &ValidationResult{}
	r1.addError("db", "host", "missing")
	r1.addWarning("db", "password", "empty")

	r2 := &ValidationResult{}
	r2.addError("api", "port", "invalid")

	r1.merge(r2)

	if len(r1.Errors) != 2 {
		t.Errorf("expected 2 errors after merge, got %d", len(r1.Errors))
	}
	if len(r1.Warnings) != 1 {
		t.Errorf("expected 1 warning after merge, got %d", len(r1.Warnings))
	}
}

func TestValidationResult_MergeNil(t *testing.T) {
	r := &ValidationResult{}
	r.addError("db", "host", "missing")
	r.merge(nil)
	if len(r.Errors) != 1 {
		t.Errorf("expected 1 error after merging nil, got %d", len(r.Errors))
	}
}

func TestValidationResult_ErrorString(t *testing.T) {
	r := &ValidationResult{}
	if r.Error() != "" {
		t.Errorf("expected empty error string for valid result, got %q", r.Error())
	}

	r.addError("db", "host", "missing")
	errStr := r.Error()
	if errStr == "" {
		t.Fatal("expected non-empty error string")
	}
	if !strings.Contains(errStr, "host") {
		t.Errorf("expected error string to mention 'host', got %q", errStr)
	}
}

func TestValidationResult_AsError(t *testing.T) {
	r := &ValidationResult{}
	if r.AsError() != nil {
		t.Fatal("expected nil error for valid result")
	}

	r.addError("db", "host", "missing")
	err := r.AsError()
	if err == nil {
		t.Fatal("expected non-nil error for invalid result")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected error to mention 'validation failed', got %q", err.Error())
	}
}

func TestValidationIssue_ErrorFormat(t *testing.T) {
	issue := ValidationIssue{
		Section:  "database",
		Field:    "host",
		Message:  "is required",
		Severity: SeverityError,
	}
	expected := "[ERROR] database.host: is required"
	if issue.Error() != expected {
		t.Errorf("expected %q, got %q", expected, issue.Error())
	}

	warning := ValidationIssue{
		Section:  "api",
		Field:    "",
		Message:  "something",
		Severity: SeverityWarning,
	}
	expected = "[WARNING] api: something"
	if warning.Error() != expected {
		t.Errorf("expected %q, got %q", expected, warning.Error())
	}
}

func TestValidationResult_HasWarningsNoErrors(t *testing.T) {
	r := &ValidationResult{}
	r.addWarning("db", "password", "empty")

	if r.HasErrors() {
		t.Fatal("should not have errors")
	}
	if !r.HasWarnings() {
		t.Fatal("should have warnings")
	}
	if !r.IsValid() {
		t.Fatal("should be valid (warnings only)")
	}
}

// --- helper functions ---

func validScanningConfig() *ScanningConfig {
	return &ScanningConfig{
		WorkerPoolSize:         10,
		DefaultInterval:        1 * time.Hour,
		MaxScanTimeout:         10 * time.Minute,
		DefaultPorts:           "22,80,443",
		DefaultScanType:        "connect",
		MaxConcurrentTargets:   100,
		EnableServiceDetection: true,
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
	}
}

func validAPIConfig() *APIConfig {
	return &APIConfig{
		Enabled:           true,
		Host:              "127.0.0.1",
		Port:              8080,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLS:               TLSConfig{Enabled: false},
		AuthEnabled:       false,
		APIKeys:           []string{},
		EnableCORS:        false,
		RateLimitEnabled:  true,
		RateLimitRequests: 100,
		RateLimitWindow:   time.Minute,
		RequestTimeout:    30 * time.Second,
		MaxRequestSize:    1024 * 1024,
	}
}

// assertHasError checks that the result contains at least one error whose Field matches the given substring.
func assertHasError(t *testing.T, result *ValidationResult, fieldSubstring string) {
	t.Helper()
	for _, e := range result.Errors {
		if strings.Contains(e.Field, fieldSubstring) {
			return
		}
	}
	t.Errorf(
		"expected validation error with field containing %q, but found none in errors: %v",
		fieldSubstring, result.Errors,
	)
}

// assertNoError checks that the result contains no errors whose Field matches the given substring.
func assertNoError(t *testing.T, result *ValidationResult, fieldSubstring string) {
	t.Helper()
	for _, e := range result.Errors {
		if strings.Contains(e.Field, fieldSubstring) {
			t.Errorf("expected no validation error with field containing %q, but found: %v", fieldSubstring, e)
			return
		}
	}
}

// assertHasWarning checks that the result contains at least one warning whose Field matches the given substring.
func assertHasWarning(t *testing.T, result *ValidationResult, fieldSubstring string) {
	t.Helper()
	for _, w := range result.Warnings {
		if strings.Contains(w.Field, fieldSubstring) {
			return
		}
	}
	t.Errorf(
		"expected validation warning with field containing %q, but found none in warnings: %v",
		fieldSubstring, result.Warnings,
	)
}
