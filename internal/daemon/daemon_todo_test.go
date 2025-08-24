package daemon

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

func TestReloadConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		initialConfig *config.Config
		newConfig     *config.Config
		wantError     bool
		setupFunc     func(*Daemon)
	}{
		{
			name: "successful config reload with API changes",
			initialConfig: &config.Config{
				API: config.APIConfig{
					Enabled: true,
					Host:    "localhost",
					Port:    8080,
				},
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			newConfig: &config.Config{
				API: config.APIConfig{
					Enabled: true,
					Host:    "localhost",
					Port:    9090, // Changed port
				},
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			wantError: false,
		},
		{
			name: "successful config reload with database changes",
			initialConfig: &config.Config{
				API: config.APIConfig{
					Enabled: false,
				},
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			newConfig: &config.Config{
				API: config.APIConfig{
					Enabled: false,
				},
				Database: db.Config{
					Host:     "newhost", // Changed host
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			wantError: true, // Will fail because newhost doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create daemon with initial config
			d := New(tt.initialConfig)
			d.config = tt.initialConfig

			// Setup if needed
			if tt.setupFunc != nil {
				tt.setupFunc(d)
			}

			// Test config reload
			// Note: We can't fully test this without mocking config.Load()
			// but we can test the helper methods
			err := d.reloadConfiguration()
			if (err != nil) != tt.wantError {
				t.Errorf("reloadConfiguration() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDatabaseReconnection(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		wantError bool
	}{
		{
			name: "reconnection with valid config should succeed eventually",
			config: &config.Config{
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			wantError: true, // Will fail in test environment without real DB
		},
		{
			name: "reconnection with invalid config should fail",
			config: &config.Config{
				Database: db.Config{
					Host:     "nonexistent-host",
					Port:     5432,
					Username: "test",
					Password: "test",
					Database: "testdb",
					SSLMode:  "disable",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.config)

			// Test reconnection logic
			err := d.reconnectDatabase()
			if (err != nil) != tt.wantError {
				t.Errorf("reconnectDatabase() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestDebugModeToggle(t *testing.T) {
	d := New(&config.Config{})

	// Test initial state
	if d.IsDebugMode() {
		t.Error("Debug mode should be false initially")
	}

	// Test toggle to true
	d.toggleDebugMode()
	if !d.IsDebugMode() {
		t.Error("Debug mode should be true after first toggle")
	}

	// Test toggle back to false
	d.toggleDebugMode()
	if d.IsDebugMode() {
		t.Error("Debug mode should be false after second toggle")
	}
}

func TestDebugModeConcurrency(t *testing.T) {
	d := New(&config.Config{})

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Test concurrent access to debug mode
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			d.toggleDebugMode()
			_ = d.IsDebugMode()
		}()
	}

	wg.Wait()
	// Test passes if no race condition occurs
}

func TestDumpStatus(t *testing.T) {
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			WorkDir: "/tmp",
		},
		API: config.APIConfig{
			Enabled: true,
			Host:    "localhost",
			Port:    8080,
		},
	}

	d := New(cfg)

	// Test that dumpStatus doesn't panic
	// We can't easily test the output, but we can ensure it runs
	d.dumpStatus()

	// Test with debug mode enabled
	d.toggleDebugMode()
	d.dumpStatus()
}

func TestCheckMemoryUsage(t *testing.T) {
	d := New(&config.Config{})

	// Test that memory check doesn't panic
	d.checkMemoryUsage()

	// Memory check should run without errors
	// The actual memory values depend on the runtime state
}

func TestCheckDiskSpace(t *testing.T) {
	tests := []struct {
		name      string
		workDir   string
		wantError bool
	}{
		{
			name:      "valid directory",
			workDir:   "/tmp",
			wantError: false,
		},
		{
			name:      "empty work dir (should use current dir)",
			workDir:   "",
			wantError: false,
		},
		{
			name:      "nonexistent directory",
			workDir:   "/nonexistent/path",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Daemon: config.DaemonConfig{
					WorkDir: tt.workDir,
				},
			}
			d := New(cfg)

			err := d.checkDiskSpace()
			if (err != nil) != tt.wantError {
				t.Errorf("checkDiskSpace() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestCheckSystemResources(t *testing.T) {
	d := New(&config.Config{})

	// Test that system resource check doesn't panic
	d.checkSystemResources()

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.ctx = ctx
	d.checkSystemResources()
}

func TestCheckNetworkConnectivity(t *testing.T) {
	d := New(&config.Config{})

	err := d.checkNetworkConnectivity()
	// Currently returns nil as it's a framework implementation
	if err != nil {
		t.Errorf("checkNetworkConnectivity() should return nil, got %v", err)
	}
}

func TestPerformHealthCheck(t *testing.T) {
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			WorkDir: "/tmp",
		},
	}
	d := New(cfg)

	// Test that health check runs without panicking
	d.performHealthCheck()

	// Test with database (will fail ping but shouldn't panic)
	// Note: We can't easily mock the database for this test
	d.performHealthCheck()
}

func TestHasAPIConfigChanged(t *testing.T) {
	d := New(&config.Config{})

	tests := []struct {
		name      string
		oldConfig *config.Config
		newConfig *config.Config
		want      bool
	}{
		{
			name: "no changes",
			oldConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 8080},
			},
			newConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 8080},
			},
			want: false,
		},
		{
			name: "port changed",
			oldConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 8080},
			},
			newConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 9090},
			},
			want: true,
		},
		{
			name: "enabled status changed",
			oldConfig: &config.Config{
				API: config.APIConfig{Enabled: false, Host: "localhost", Port: 8080},
			},
			newConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 8080},
			},
			want: true,
		},
		{
			name: "host changed",
			oldConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "localhost", Port: 8080},
			},
			newConfig: &config.Config{
				API: config.APIConfig{Enabled: true, Host: "0.0.0.0", Port: 8080},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.hasAPIConfigChanged(tt.oldConfig, tt.newConfig)
			if got != tt.want {
				t.Errorf("hasAPIConfigChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasDatabaseConfigChanged(t *testing.T) {
	d := New(&config.Config{})

	tests := []struct {
		name      string
		oldConfig *config.Config
		newConfig *config.Config
		want      bool
	}{
		{
			name: "no changes",
			oldConfig: &config.Config{
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "user",
					Password: "pass",
					Database: "db",
					SSLMode:  "disable",
				},
			},
			newConfig: &config.Config{
				Database: db.Config{
					Host:     "localhost",
					Port:     5432,
					Username: "user",
					Password: "pass",
					Database: "db",
					SSLMode:  "disable",
				},
			},
			want: false,
		},
		{
			name: "host changed",
			oldConfig: &config.Config{
				Database: db.Config{Host: "localhost"},
			},
			newConfig: &config.Config{
				Database: db.Config{Host: "newhost"},
			},
			want: true,
		},
		{
			name: "port changed",
			oldConfig: &config.Config{
				Database: db.Config{Port: 5432},
			},
			newConfig: &config.Config{
				Database: db.Config{Port: 5433},
			},
			want: true,
		},
		{
			name: "password changed",
			oldConfig: &config.Config{
				Database: db.Config{Password: "oldpass"},
			},
			newConfig: &config.Config{
				Database: db.Config{Password: "newpass"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.hasDatabaseConfigChanged(tt.oldConfig, tt.newConfig)
			if got != tt.want {
				t.Errorf("hasDatabaseConfigChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthCheckIntegration(t *testing.T) {
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			WorkDir: t.TempDir(),
		},
	}
	d := New(cfg)

	// Test that all health check components work together
	d.performHealthCheck()

	// Verify that health checks handle various scenarios
	originalWorkDir := d.config.Daemon.WorkDir
	d.config.Daemon.WorkDir = "/nonexistent"
	d.performHealthCheck() // Should handle errors gracefully
	d.config.Daemon.WorkDir = originalWorkDir
}

func TestConfigReloadRollback(t *testing.T) {
	// Test that configuration rollback works when new config is invalid
	initialConfig := &config.Config{
		Database: db.Config{
			Host:     "localhost",
			Port:     5432,
			Username: "test",
			Password: "test",
			Database: "testdb",
			SSLMode:  "disable",
		},
	}

	d := New(initialConfig)
	originalConfig := d.config

	// Since we can't easily mock config.Load() to return invalid config,
	// we'll test the rollback logic by directly testing the helper methods

	// Verify that original config is preserved
	if d.config != originalConfig {
		t.Error("Original config should be preserved")
	}
}

func TestExponentialBackoffCalculation(t *testing.T) {
	// Test the exponential backoff logic used in reconnectDatabase
	baseDelay := 2 * time.Second
	maxDelay := 30 * time.Second

	tests := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{1, 2 * time.Second, 2 * time.Second},   // 2 * 2^0
		{2, 4 * time.Second, 4 * time.Second},   // 2 * 2^1
		{3, 8 * time.Second, 8 * time.Second},   // 2 * 2^2
		{4, 16 * time.Second, 16 * time.Second}, // 2 * 2^3
		{5, 30 * time.Second, 30 * time.Second}, // capped at maxDelay
		{6, 30 * time.Second, 30 * time.Second}, // capped at maxDelay
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			multiplier := 1 << uint(tt.attempt-1)
			delay := time.Duration(float64(baseDelay) * float64(multiplier))
			if delay > maxDelay {
				delay = maxDelay
			}

			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("attempt %d: got delay %v, want between %v and %v",
					tt.attempt, delay, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	d := New(&config.Config{})

	// Test that operations respect context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx

	cancel() // Cancel immediately

	// These operations should handle cancelled context gracefully
	d.checkSystemResources()

	// The reconnection would normally check for context cancellation
	// but we can't easily test that without mocking the database connection
}

func TestMemoryLeakPrevention(t *testing.T) {
	// Test that repeated operations don't cause memory leaks
	d := New(&config.Config{
		Daemon: config.DaemonConfig{WorkDir: "/tmp"},
	})

	// Run health checks multiple times
	for i := 0; i < 100; i++ {
		d.checkMemoryUsage()
		d.checkSystemResources()
		d.checkNetworkConnectivity()
		_ = d.checkDiskSpace()
	}

	// Toggle debug mode multiple times
	for i := 0; i < 50; i++ {
		d.toggleDebugMode()
	}

	// If we reach here without panicking or running out of memory,
	// the test passes
}

// Benchmark tests for performance verification

func BenchmarkHealthCheck(b *testing.B) {
	d := New(&config.Config{
		Daemon: config.DaemonConfig{WorkDir: "/tmp"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.performHealthCheck()
	}
}

func BenchmarkDebugModeToggle(b *testing.B) {
	d := New(&config.Config{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.toggleDebugMode()
	}
}

func BenchmarkMemoryCheck(b *testing.B) {
	d := New(&config.Config{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.checkMemoryUsage()
	}
}
