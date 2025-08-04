package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/anstrom/scanorama/internal/config"
)

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
	syscall.Kill(os.Getpid(), syscall.SIGTERM)

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
