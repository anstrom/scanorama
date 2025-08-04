package daemon

import (
	"io/ioutil"
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

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test writing PID file
	if err := d.writePIDFile(); err != nil {
		t.Fatalf("writePIDFile() error = %v", err)
	}

	// Verify PID file content
	content, err := ioutil.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("Failed to read PID file: %v", err)
	}

	expectedPID := []byte(string(os.Getpid()) + "\n")
	if string(content) != string(expectedPID) {
		t.Errorf("PID file content = %q, want %q", content, expectedPID)
	}

	// Test removing PID file
	if err := d.removePIDFile(); err != nil {
		t.Fatalf("removePIDFile() error = %v", err)
	}

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
			d, err := New(&config.Config{Daemon: tt.config})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			err = d.dropPrivileges()
			if (err != nil) != tt.wantError {
				t.Errorf("dropPrivileges() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSignalHandling(t *testing.T) {
	d, err := New(&config.Config{
		Daemon: config.DaemonConfig{
			PIDFile: filepath.Join(t.TempDir(), "test.pid"),
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Start signal handler
	done := make(chan struct{})
	go func() {
		d.handleSignals()
		close(done)
	}()

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
		d, _ := New(cfg)
		d.Daemonize()
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
			WorkingDir: tempDir,
		},
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := d.changeWorkingDirectory(); err != nil {
		t.Fatalf("changeWorkingDirectory() error = %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	if cwd != tempDir {
		t.Errorf("Working directory = %s, want %s", cwd, tempDir)
	}
}
