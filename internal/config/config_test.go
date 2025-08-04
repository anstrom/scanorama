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
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
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
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
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
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
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
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
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
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
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

func TestLoadWithEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		setup   func() (string, func())
		check   func(*Config) error
		wantErr bool
	}{
		{
			name: "override database config",
			env: map[string]string{
				"SCANORAMA_DB_HOST":     "env-host",
				"SCANORAMA_DB_PORT":     "5433",
				"SCANORAMA_DB_NAME":     "env-db",
				"SCANORAMA_DB_USER":     "env-user",
				"SCANORAMA_DB_PASSWORD": "env-pass",
			},
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  password: testpass
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			check: func(c *Config) error {
				if c == nil {
					t.Fatal("Config is nil")
				}
				// Set environment variables
				os.Setenv("SCANORAMA_DB_HOST", "env-host")
				os.Setenv("SCANORAMA_DB_PORT", "5433")
				os.Setenv("SCANORAMA_DB_NAME", "env-db")
				os.Setenv("SCANORAMA_DB_USER", "env-user")
				os.Setenv("SCANORAMA_DB_PASSWORD", "env-pass")

				cfg := getDatabaseConfigFromEnv()
				if got := cfg.Host; got != "env-host" {
					t.Errorf("Host = %v, want %v", got, "env-host")
				}
				if got := cfg.Port; got != 5433 {
					t.Errorf("Port = %v, want %v", got, 5433)
				}
				if got := cfg.Database; got != "env-db" {
					t.Errorf("Database = %v, want %v", got, "env-db")
				}
				if got := cfg.Username; got != "env-user" {
					t.Errorf("Username = %v, want %v", got, "env-user")
				}
				if got := cfg.Password; got != "env-pass" {
					t.Errorf("Password = %v, want %v", got, "env-pass")
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "invalid port in env",
			env: map[string]string{
				"SCANORAMA_DB_PORT": "invalid",
			},
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env
			origEnv := make(map[string]string)
			for k := range tt.env {
				if v, ok := os.LookupEnv(k); ok {
					origEnv[k] = v
				}
			}

			// Set test env
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			// Cleanup env after test
			defer func() {
				for k := range tt.env {
					if orig, ok := origEnv[k]; ok {
						os.Setenv(k, orig)
					} else {
						os.Unsetenv(k)
					}
				}
			}()

			path, cleanup := tt.setup()
			defer cleanup()

			cfg, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tt.check != nil {
				if err := tt.check(cfg); err != nil {
					t.Errorf("check failed: %v", err)
				}
			}
		})
	}
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
