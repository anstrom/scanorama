package config

import (
	"context"
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
				expectedErr := "config file not found: stat /nonexistent/config.yaml: no such file or directory"
				if err == nil || err.Error() != expectedErr {
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

// TestConfigHotReload tests the hot-reload functionality behavior
func TestConfigHotReload(t *testing.T) {
	t.Run("config_loads_with_hot_reload_capabilities", func(t *testing.T) {
		content := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
scanning:
  worker_pool_size: 4
`
		path, cleanup := createTestConfigFile(t, content)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Test that hot-reload channel is available
		reloadChan := cfg.ReloadChannel()
		if reloadChan == nil {
			t.Error("ReloadChannel() should return a non-nil channel")
		}

		// Test initial configuration values
		if cfg.Database.Host != "localhost" {
			t.Errorf("Expected database host 'localhost', got %s", cfg.Database.Host)
		}
		if cfg.Scanning.WorkerPoolSize != 4 {
			t.Errorf("Expected worker pool size 4, got %d", cfg.Scanning.WorkerPoolSize)
		}
	})

	t.Run("config_detects_file_changes", func(t *testing.T) {
		initialContent := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
scanning:
  worker_pool_size: 4
`
		path, cleanup := createTestConfigFile(t, initialContent)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Modify the config file
		updatedContent := `
database:
  host: updated-host
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
scanning:
  worker_pool_size: 8
`
		time.Sleep(100 * time.Millisecond) // Ensure different modification time
		if err := os.WriteFile(path, []byte(updatedContent), 0o600); err != nil {
			t.Fatalf("Failed to update config file: %v", err)
		}

		// Test that checkForChanges detects the modification
		err = cfg.checkForChanges()
		if err != nil {
			t.Errorf("checkForChanges() failed: %v", err)
		}

		// Test that reload channel receives signal
		select {
		case <-cfg.ReloadChannel():
			// Expected behavior - change detected
		default:
			t.Error("ReloadChannel should receive signal after file modification")
		}
	})

	t.Run("config_reloads_successfully", func(t *testing.T) {
		initialContent := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
scanning:
  worker_pool_size: 4
`
		path, cleanup := createTestConfigFile(t, initialContent)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Verify initial values
		if cfg.Database.Host != "localhost" {
			t.Fatalf("Expected initial host 'localhost', got %s", cfg.Database.Host)
		}
		if cfg.Scanning.WorkerPoolSize != 4 {
			t.Fatalf("Expected initial worker pool size 4, got %d", cfg.Scanning.WorkerPoolSize)
		}

		// Update the config file
		updatedContent := `
database:
  host: updated-host
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
scanning:
  worker_pool_size: 8
api:
  port: 9090
`
		time.Sleep(100 * time.Millisecond) // Ensure different modification time
		if err := os.WriteFile(path, []byte(updatedContent), 0o600); err != nil {
			t.Fatalf("Failed to update config file: %v", err)
		}

		// Reload the configuration
		err = cfg.Reload()
		if err != nil {
			t.Fatalf("Reload() failed: %v", err)
		}

		// Verify that values have been updated
		if cfg.Database.Host != "updated-host" {
			t.Errorf("Expected reloaded host 'updated-host', got %s", cfg.Database.Host)
		}
		if cfg.Scanning.WorkerPoolSize != 8 {
			t.Errorf("Expected reloaded worker pool size 8, got %d", cfg.Scanning.WorkerPoolSize)
		}
		if cfg.API.Port != 9090 {
			t.Errorf("Expected reloaded API port 9090, got %d", cfg.API.Port)
		}
	})

	t.Run("config_handles_invalid_reload_gracefully", func(t *testing.T) {
		validContent := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
`
		path, cleanup := createTestConfigFile(t, validContent)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Store original values
		originalHost := cfg.Database.Host

		// Write invalid content
		invalidContent := `
database:
  host: localhost
  port: invalid-port
  database: testdb
`
		time.Sleep(100 * time.Millisecond)
		if err := os.WriteFile(path, []byte(invalidContent), 0o600); err != nil {
			t.Fatalf("Failed to write invalid config: %v", err)
		}

		// Attempt to reload - should fail
		err = cfg.Reload()
		if err == nil {
			t.Error("Reload() should fail with invalid configuration")
		}

		// Original configuration should remain unchanged
		if cfg.Database.Host != originalHost {
			t.Errorf("Configuration should not change after failed reload, got %s", cfg.Database.Host)
		}
	})

	t.Run("config_reload_preserves_hot_reload_state", func(t *testing.T) {
		content := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
`
		path, cleanup := createTestConfigFile(t, content)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Get original channel reference
		originalChan := cfg.ReloadChannel()

		// Update and reload
		updatedContent := `
database:
  host: updated-host
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
`
		time.Sleep(100 * time.Millisecond)
		if err := os.WriteFile(path, []byte(updatedContent), 0o600); err != nil {
			t.Fatalf("Failed to update config file: %v", err)
		}

		err = cfg.Reload()
		if err != nil {
			t.Fatalf("Reload() failed: %v", err)
		}

		// Reload channel should still be the same (hot-reload state preserved)
		newChan := cfg.ReloadChannel()
		if originalChan != newChan {
			t.Error("ReloadChannel should remain the same after config reload")
		}

		// Should still be able to detect further changes
		err = cfg.checkForChanges()
		if err != nil {
			t.Errorf("checkForChanges() should still work after reload: %v", err)
		}
	})

	t.Run("config_without_file_path_handles_reload_gracefully", func(t *testing.T) {
		cfg := Default()
		// cfg.filePath is empty since it wasn't loaded from a file

		err := cfg.Reload()
		if err == nil {
			t.Error("Reload() should fail when no file path is set")
		}

		// Should return a valid error message
		if err != nil && err.Error() == "" {
			t.Error("Error should have a descriptive message")
		}
	})

	t.Run("watch_for_reload_respects_context_cancellation", func(t *testing.T) {
		content := `
database:
  host: localhost
  port: 5432
  database: testdb
  username: testuser
  password: testpass
  ssl_mode: disable
`
		path, cleanup := createTestConfigFile(t, content)
		defer cleanup()

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Start watching - should return when context is cancelled
		start := time.Now()
		err = cfg.WatchForReload(ctx)
		elapsed := time.Since(start)

		// Should have returned due to context cancellation
		if err != context.DeadlineExceeded {
			t.Errorf("Expected context.DeadlineExceeded, got %v", err)
		}

		// Should have taken approximately the timeout duration
		if elapsed < 50*time.Millisecond || elapsed > 200*time.Millisecond {
			t.Errorf("WatchForReload should respect context timeout, took %v", elapsed)
		}
	})

	t.Run("watch_for_reload_without_file_path_fails_gracefully", func(t *testing.T) {
		cfg := Default()
		// cfg.filePath is empty since it wasn't loaded from a file

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := cfg.WatchForReload(ctx)
		if err == nil {
			t.Error("WatchForReload() should fail when no file path is set")
		}

		// Should return immediately with descriptive error, not wait for context
		if err != nil && err.Error() == "" {
			t.Error("Error should have a descriptive message")
		}
	})
}

func TestValidateHelpersAndSave(t *testing.T) {
	t.Run("validateConfigPath rejects traversal and bad ext", func(t *testing.T) {
		if err := validateConfigPath("../etc/passwd"); err == nil {
			t.Error("expected error for path traversal")
		}
		if err := validateConfigPath("config.exe"); err == nil {
			t.Error("expected error for unsupported extension")
		}
		if err := validateConfigPath("config.yaml"); err != nil {
			t.Errorf("unexpected error for valid path: %v", err)
		}
	})

	t.Run("validateConfigPermissions detects insecure perms", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "cfg.yaml")
		if err := os.WriteFile(p, []byte("a: b"), 0o644); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateConfigPermissions(fi); err == nil {
			t.Error("expected error for world-readable file")
		}
		if err := os.Chmod(p, 0o600); err != nil {
			t.Fatal(err)
		}
		fi, _ = os.Stat(p)
		if err := validateConfigPermissions(fi); err != nil {
			t.Errorf("unexpected error for secure perms: %v", err)
		}
	})

	t.Run("validateConfigContent edge cases", func(t *testing.T) {
		if err := validateConfigContent([]byte{}); err == nil {
			t.Error("expected error for empty content")
		}
		big := make([]byte, maxContentSize+1)
		if err := validateConfigContent(big); err == nil {
			t.Error("expected error for oversized content")
		}
		// 2% null bytes triggers binary heuristic
		data := make([]byte, 200)
		for i := 0; i < 10; i++ { // 10/200 = 5%
			data[i] = 0
		}
		if err := validateConfigContent(data); err == nil {
			t.Error("expected error for binary-like content")
		}
	})

	t.Run("safeJSONUnmarshal unknown fields cause error", func(t *testing.T) {
		var out struct {
			A int `json:"a"`
		}
		err := safeJSONUnmarshal([]byte(`{"a":1,"b":2}`), &out)
		if err == nil {
			t.Error("expected error for unknown field")
		}
	})

	t.Run("safeYAMLUnmarshal malformed yaml returns error", func(t *testing.T) {
		var out struct {
			A int `yaml:"a"`
		}
		if err := safeYAMLUnmarshal([]byte("a: [1,2"), &out); err == nil {
			t.Error("expected YAML decode error")
		}
	})

	t.Run("Save writes file successfully", func(t *testing.T) {
		cfg := Default()
		dir := t.TempDir()
		p := filepath.Join(dir, "out.yaml")
		if err := cfg.Save(p); err != nil {
			t.Fatalf("Save() error: %v", err)
		}
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected file to exist: %v", err)
		}
	})
}

func TestAccessorsAndDefaults(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	// Accessors
	_ = cfg.GetDatabaseConfig()
	_ = cfg.IsDaemonMode()
	_ = cfg.IsAPIEnabled()
	_ = cfg.GetLogOutput()
	_ = cfg.GetAPIAddress()
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
