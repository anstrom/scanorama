package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (string, func())
		wantErr bool
	}{
		{
			name: "valid yaml config",
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
daemon:
  user: nobody
  group: nobody
  pid_file: /var/run/scanorama.pid
scanning:
  worker_pool_size: 4
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					_ = os.Remove(path)
				}
			},
			wantErr: false,
		},
		{
			name: "valid json config",
			setup: func() (string, func()) {
				content := []byte(`{
					"database": {
						"host": "localhost",
						"port": 5432,
						"database": "testdb",
						"username": "testuser",
						"password": "testpass",
						"ssl_mode": "disable"
					},
					"daemon": {
						"user": "nobody",
						"group": "nobody",
						"pid_file": "/var/run/scanorama.pid"
					},
					"scanning": {
						"worker_pool_size": 4
					}
				}`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					_ = os.Remove(path)
				}
			},
			wantErr: false,
		},
		{
			name: "invalid yaml syntax",
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: invalid
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					_ = os.Remove(path)
				}
			},
			wantErr: true,
		},
		{
			name: "invalid json syntax",
			setup: func() (string, func()) {
				content := []byte(`{
					"database": {
						"host": "localhost",
						"port": "invalid"
					},
				}`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					_ = os.Remove(path)
				}
			},
			wantErr: true,
		},
		{
			name: "nonexistent file",
			setup: func() (string, func()) {
				return "/nonexistent/config.yaml", func() {}
			},
			wantErr: true,
		},
		{
			name: "unsupported extension",
			setup: func() (string, func()) {
				content := []byte(`config data`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.txt")
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					_ = os.Remove(path)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup := tt.setup()
			defer cleanup()

			_, err := Load(path)
			if tt.name == "nonexistent file" {
				if err == nil || err.Error() != "config file not found: stat /nonexistent/config.yaml: no such file or directory" {
					t.Errorf("Load() expected specific error message, got %v", err)
				}
			} else if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// setUpEnvironment sets up test environment variables and returns a cleanup function.
func setUpEnvironment(env map[string]string) func() {
	origEnv := make(map[string]string)
	for k := range env {
		if v, ok := os.LookupEnv(k); ok {
			origEnv[k] = v
		}
	}

	for k, v := range env {
		_ = os.Setenv(k, v)
	}

	return func() {
		for k := range env {
			if orig, ok := origEnv[k]; ok {
				_ = os.Setenv(k, orig)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

// createTestConfigFile creates a temporary config file with given content.
func createTestConfigFile(t *testing.T, content string) (path string, cleanup func()) {
	dir := t.TempDir()
	path = filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, func() {
		_ = os.Remove(path)
	}
}

// validateDatabaseConfig validates database configuration from environment.
func validateDatabaseConfig(t *testing.T) {
	cfg := getDatabaseConfigFromEnv()

	expected := map[string]interface{}{
		"env-host": cfg.Host,
		"env-db":   cfg.Database,
		"env-user": cfg.Username,
		"env-pass": cfg.Password,
	}

	for want, got := range expected {
		if got != want {
			t.Errorf("Expected %v, got %v", want, got)
		}
	}

	if cfg.Port != 5433 {
		t.Errorf("Port = %v, want %v", cfg.Port, 5433)
	}
}

func TestLoadWithEnv(t *testing.T) {
	t.Run("override database config", func(t *testing.T) {
		env := map[string]string{
			"SCANORAMA_DB_HOST":     "env-host",
			"SCANORAMA_DB_PORT":     "5433",
			"SCANORAMA_DB_NAME":     "env-db",
			"SCANORAMA_DB_USER":     "env-user",
			"SCANORAMA_DB_PASSWORD": "env-pass",
		}

		cleanup := setUpEnvironment(env)
		defer cleanup()

		content := `
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  password: testpass
`
		path, fileCleanup := createTestConfigFile(t, content)
		defer fileCleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Errorf("Load() error = %v, wantErr false", err)
			return
		}

		if cfg == nil {
			t.Fatal("Config is nil")
		}

		validateDatabaseConfig(t)
	})

	t.Run("invalid port in env", func(t *testing.T) {
		env := map[string]string{
			"SCANORAMA_DB_PORT": "invalid",
		}

		cleanup := setUpEnvironment(env)
		defer cleanup()

		content := `
database:
  host: localhost
  port: 5432
`
		path, fileCleanup := createTestConfigFile(t, content)
		defer fileCleanup()

		_, err := Load(path)
		if err == nil {
			t.Errorf("Load() error = nil, wantErr true")
		}
	})
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Database: db.Config{
					Host:            "localhost",
					Port:            5432,
					Database:        "testdb",
					Username:        "testuser",
					Password:        "testpass",
					SSLMode:         "disable",
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: 5 * time.Minute,
					ConnMaxIdleTime: 5 * time.Minute,
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
				Daemon: DaemonConfig{
					User:    "nobody",
					Group:   "nobody",
					PIDFile: "/var/run/scanorama.pid",
				},
				Scanning: ScanningConfig{
					WorkerPoolSize:         4,
					MaxConcurrentTargets:   10,
					DefaultInterval:        time.Hour,
					MaxScanTimeout:         10 * time.Minute,
					DefaultPorts:           "22,80,443",
					DefaultScanType:        "connect",
					EnableServiceDetection: true,
					Retry: RetryConfig{
						MaxRetries:        3,
						RetryDelay:        time.Second * 30,
						BackoffMultiplier: 2.0,
					},
					RateLimit: RateLimitConfig{
						Enabled:           true,
						RequestsPerSecond: 100,
						BurstSize:         200,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing database host",
			config: &Config{
				Database: db.Config{
					Port:            5432,
					Database:        "testdb",
					Username:        "testuser",
					Password:        "testpass",
					SSLMode:         "disable",
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: 5 * time.Minute,
					ConnMaxIdleTime: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid database port",
			config: &Config{
				Database: db.Config{
					Host:            "localhost",
					Port:            0,
					Database:        "testdb",
					Username:        "testuser",
					Password:        "testpass",
					SSLMode:         "disable",
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: 5 * time.Minute,
					ConnMaxIdleTime: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "missing database name",
			config: &Config{
				Database: db.Config{
					Host:            "localhost",
					Port:            5432,
					Database:        "testdb",
					Username:        "testuser",
					SSLMode:         "disable",
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: 5 * time.Minute,
					ConnMaxIdleTime: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
		{
			name: "missing database user",
			config: &Config{
				Database: db.Config{
					Host:            "localhost",
					Port:            5432,
					Username:        "testuser",
					Password:        "testpass",
					SSLMode:         "disable",
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: 5 * time.Minute,
					ConnMaxIdleTime: 5 * time.Minute,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
