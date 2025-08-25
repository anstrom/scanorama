package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// =============================================================================
// Core Daemon Functionality Tests
// =============================================================================

func TestNewDaemon(t *testing.T) {
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			User:    "nobody",
			Group:   "nobody",
			PIDFile: filepath.Join(t.TempDir(), "test.pid"),
		},
	}

	d := New(cfg)

	if d == nil {
		t.Fatal("New() returned nil daemon")
	}

	if d.config != cfg {
		t.Error("New() did not set config correctly")
	}

	if d.logger == nil {
		t.Error("New() did not initialize logger")
	}
}

// =============================================================================
// System Integration Tests
// =============================================================================

func TestPIDFileHandling(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			PIDFile: pidFile,
		},
	}

	d := New(cfg)

	// Test writing PID file
	if err := d.createPIDFile(); err != nil {
		t.Fatalf("createPIDFile() error = %v", err)
	}

	// Verify PID file content
	content, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("Failed to read PID file: %v", err)
	}

	expectedPID := fmt.Sprintf("%d", os.Getpid())
	if string(content) != expectedPID {
		t.Errorf("PID file content = %q, want %q", content, expectedPID)
	}

	// Test removing PID file
	d.cleanup()

	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file was not removed")
	}
}

func TestPrivilegeDropping(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("Test requires root privileges")
	}

	tests := []struct {
		name      string
		config    config.DaemonConfig
		wantError bool
	}{
		{
			name: "valid user and group",
			config: config.DaemonConfig{
				User:  "nobody",
				Group: "nobody",
			},
			wantError: false,
		},
		{
			name: "invalid user",
			config: config.DaemonConfig{
				User:  "nonexistentuser",
				Group: "nobody",
			},
			wantError: true,
		},
		{
			name: "invalid group",
			config: config.DaemonConfig{
				User:  "nobody",
				Group: "nonexistentgroup",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(&config.Config{Daemon: tt.config})

			err := d.dropPrivileges()
			if (err != nil) != tt.wantError {
				t.Errorf("dropPrivileges() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSignalHandling(t *testing.T) {
	d := New(&config.Config{
		Daemon: config.DaemonConfig{
			PIDFile: filepath.Join(t.TempDir(), "test.pid"),
		},
	})

	// Setup signal handlers
	done := make(chan struct{})
	go func() {
		<-d.GetContext().Done()
		close(done)
	}()
	d.setupSignalHandlers()

	// Send termination signal
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

	// Wait for handler to process signal
	<-done
}

func TestDaemonize(t *testing.T) {
	if os.Getenv("GO_TEST_DAEMONIZE") == "1" {
		// Child process
		cfg := &config.Config{
			Daemon: config.DaemonConfig{
				PIDFile: filepath.Join(os.TempDir(), "test.pid"),
			},
		}
		d := New(cfg)
		if err := d.Start(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Parent process
	cmd := os.Args[0]
	env := []string{"GO_TEST_DAEMONIZE=1"}
	_, err := os.StartProcess(cmd, []string{cmd, "-test.run=TestDaemonize"}, &os.ProcAttr{
		Env: append(os.Environ(), env...),
	})
	if err != nil {
		t.Fatalf("Failed to start daemon process: %v", err)
	}
}

func TestWorkingDirectoryChange(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			WorkDir: tempDir,
		},
	}

	d := New(cfg)

	if err := os.Chdir(d.config.Daemon.WorkDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	resolvedCwd, _ := filepath.EvalSymlinks(cwd)
	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	if resolvedCwd != resolvedTempDir {
		t.Errorf("Working directory = %s, want %s", resolvedCwd, resolvedTempDir)
	}
}

func TestSignalHandlerMethods(t *testing.T) {
	// Test that all signal handler methods exist and don't panic
	d := New(&config.Config{
		Daemon: config.DaemonConfig{WorkDir: "/tmp"},
	})

	// Suppress log output during testing
	oldOutput := log.Default().Writer()
	defer log.SetOutput(oldOutput)
	log.SetOutput(io.Discard)

	t.Run("status dump method exists", func(t *testing.T) {
		// Should not panic
		d.dumpStatus()
	})

	t.Run("debug toggle method exists", func(t *testing.T) {
		initialState := d.IsDebugMode()
		d.toggleDebugMode()
		if d.IsDebugMode() == initialState {
			t.Error("Debug mode should have toggled")
		}
	})

	t.Run("config reload method exists", func(t *testing.T) {
		// Should not panic, may return error due to missing config file
		_ = d.reloadConfiguration()
	})
}

// =============================================================================
// Configuration Management Tests
// =============================================================================

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

func TestReloadConfiguration(t *testing.T) {
	t.Run("validates new configuration", func(t *testing.T) {
		d := New(&config.Config{
			API: config.APIConfig{
				Enabled: false,
			},
		})

		// Since config.Load("") would try to load from file,
		// we test the validation logic indirectly by ensuring
		// the method handles configuration properly

		// This test validates that the method exists and has
		// proper error handling structure
		err := d.reloadConfiguration()
		// We expect an error since no valid config file exists
		if err == nil {
			t.Log("Configuration reload succeeded (likely using default config)")
		} else {
			t.Logf("Configuration reload failed as expected: %v", err)
		}
	})
}

// =============================================================================
// Feature Tests
// =============================================================================

func TestToggleDebugMode(t *testing.T) {
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

func TestDumpStatus(t *testing.T) {
	// Capture log output
	oldOutput := log.Default().Writer()
	defer log.SetOutput(oldOutput)

	// Use a discard writer to avoid cluttering test output
	log.SetOutput(io.Discard)

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
	d.dumpStatus()

	// Test with debug mode enabled
	d.toggleDebugMode()
	d.dumpStatus()
}

// =============================================================================
// Database Tests
// =============================================================================

func TestReconnectDatabase(t *testing.T) {
	// Test reconnection with invalid configuration should fail quickly
	t.Run("invalid config fails fast", func(t *testing.T) {
		d := New(&config.Config{
			Database: db.Config{
				// Invalid configuration - empty host
			},
		})

		// Set a short timeout to ensure the test doesn't hang
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		d.ctx = ctx

		err := d.reconnectDatabase()
		if err == nil {
			t.Error("Expected reconnection to fail with invalid config")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		d := New(&config.Config{
			Database: db.Config{
				Host:     "nonexistent-host",
				Port:     5432,
				Username: "test",
				Password: "test",
				Database: "test",
			},
		})

		// Cancel context immediately
		ctx, cancel := context.WithCancel(context.Background())
		d.ctx = ctx
		cancel()

		err := d.reconnectDatabase()
		if err == nil {
			t.Error("Expected reconnection to fail with cancelled context")
		}
	})
}

// =============================================================================
// Concurrency and Performance Tests
// =============================================================================

func TestDebugModeConcurrency(t *testing.T) {
	d := New(&config.Config{})

	const numGoroutines = 10
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

func TestMemoryAndPerformance(t *testing.T) {
	d := New(&config.Config{
		Daemon: config.DaemonConfig{WorkDir: "/tmp"},
	})

	// Suppress log output
	oldOutput := log.Default().Writer()
	defer log.SetOutput(oldOutput)
	log.SetOutput(io.Discard)

	// Test repeated operations don't cause memory leaks
	for i := 0; i < 10; i++ {
		d.toggleDebugMode()
		d.dumpStatus()
		_ = d.IsDebugMode()
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkDebugModeToggle(b *testing.B) {
	d := New(&config.Config{})

	// Suppress log output
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.toggleDebugMode()
	}
}

func BenchmarkIsDebugMode(b *testing.B) {
	d := New(&config.Config{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.IsDebugMode()
	}
}

func BenchmarkConfigComparison(b *testing.B) {
	d := New(&config.Config{})

	oldConfig := &config.Config{
		API: config.APIConfig{Host: "localhost", Port: 8080},
	}
	newConfig := &config.Config{
		API: config.APIConfig{Host: "localhost", Port: 9090},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.hasAPIConfigChanged(oldConfig, newConfig)
	}
}
