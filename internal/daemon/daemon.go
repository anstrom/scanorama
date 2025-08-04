package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// Daemon represents the main daemon process
type Daemon struct {
	config   *config.Config
	database *db.DB
	pidFile  string
	logger   *log.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

// New creates a new daemon instance
func New(cfg *config.Config) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())

	return &Daemon{
		config:  cfg,
		pidFile: cfg.Daemon.PIDFile,
		logger:  log.New(os.Stdout, "[daemon] ", log.LstdFlags|log.Lshortfile),
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
}

// Start starts the daemon
func (d *Daemon) Start() error {
	d.logger.Println("Starting scanorama daemon...")

	// Validate configuration
	if err := d.config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create working directory if needed
	if d.config.Daemon.WorkDir != "" {
		if err := os.MkdirAll(d.config.Daemon.WorkDir, 0755); err != nil {
			return fmt.Errorf("failed to create working directory: %w", err)
		}
		if err := os.Chdir(d.config.Daemon.WorkDir); err != nil {
			return fmt.Errorf("failed to change to working directory: %w", err)
		}
	}

	// Fork to background if daemon mode is enabled
	if d.config.Daemon.Daemonize {
		if err := d.fork(); err != nil {
			return fmt.Errorf("failed to fork daemon: %w", err)
		}
	}

	// Drop privileges if configured
	if err := d.dropPrivileges(); err != nil {
		return fmt.Errorf("failed to drop privileges: %w", err)
	}

	// Create PID file
	if err := d.createPIDFile(); err != nil {
		return fmt.Errorf("failed to create PID file: %w", err)
	}

	// Setup signal handling
	d.setupSignalHandlers()

	// Initialize database connection
	if err := d.initDatabase(); err != nil {
		d.cleanup()
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Start the main daemon loop
	d.logger.Println("Daemon started successfully")
	return d.run()
}

// Stop stops the daemon gracefully
func (d *Daemon) Stop() error {
	d.logger.Println("Stopping daemon...")

	// Cancel context to signal shutdown
	d.cancel()

	// Wait for graceful shutdown with timeout
	select {
	case <-d.done:
		d.logger.Println("Daemon stopped gracefully")
	case <-time.After(d.config.Daemon.ShutdownTimeout):
		d.logger.Println("Shutdown timeout reached, forcing exit")
	}

	d.cleanup()
	return nil
}

// fork creates a background process
func (d *Daemon) fork() error {
	// Check if already running as daemon
	if os.Getppid() == 1 {
		return nil // Already a daemon
	}

	// Fork the process
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Prepare arguments (exclude daemon flag to prevent infinite forking)
	args := []string{executable}
	for _, arg := range os.Args[1:] {
		if arg != "--daemon" && arg != "-d" {
			args = append(args, arg)
		}
	}

	// Create process attributes
	procAttr := &os.ProcAttr{
		Dir:   d.config.Daemon.WorkDir,
		Env:   os.Environ(),
		Files: []*os.File{nil, nil, nil}, // Detach from terminal
	}

	// Start the child process
	process, err := os.StartProcess(executable, args, procAttr)
	if err != nil {
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	d.logger.Printf("Daemon forked with PID %d", process.Pid)

	// Exit parent process
	os.Exit(0)
	return nil
}

// dropPrivileges drops root privileges if configured
func (d *Daemon) dropPrivileges() error {
	if d.config.Daemon.User == "" && d.config.Daemon.Group == "" {
		return nil // No privilege dropping configured
	}

	// Note: Privilege dropping requires root privileges
	// This is a simplified implementation
	if os.Getuid() != 0 {
		d.logger.Println("Not running as root, skipping privilege drop")
		return nil
	}

	// Change group first
	if d.config.Daemon.Group != "" {
		grp, err := user.LookupGroup(d.config.Daemon.Group)
		if err != nil {
			return fmt.Errorf("failed to lookup group %s: %w", d.config.Daemon.Group, err)
		}
		gid, err := strconv.Atoi(grp.Gid)
		if err != nil {
			return fmt.Errorf("invalid group ID: %w", err)
		}
		if err := syscall.Setgid(gid); err != nil {
			return fmt.Errorf("failed to set GID to %d: %w", gid, err)
		}
		d.logger.Printf("Changed group to %s (GID: %d)", d.config.Daemon.Group, gid)
	}

	// Change user
	if d.config.Daemon.User != "" {
		usr, err := user.Lookup(d.config.Daemon.User)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s: %w", d.config.Daemon.User, err)
		}

		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return fmt.Errorf("invalid user ID: %w", err)
		}

		if err := syscall.Setuid(uid); err != nil {
			return fmt.Errorf("failed to setuid to %d: %w", uid, err)
		}
		d.logger.Printf("Changed user to %s (UID: %d)", d.config.Daemon.User, uid)
	}

	return nil
}

// createPIDFile creates the PID file
func (d *Daemon) createPIDFile() error {
	if d.pidFile == "" {
		return nil // No PID file configured
	}

	// Ensure directory exists
	dir := filepath.Dir(d.pidFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	// Check if PID file already exists
	if _, err := os.Stat(d.pidFile); err == nil {
		// Read existing PID
		data, err := os.ReadFile(d.pidFile)
		if err != nil {
			return fmt.Errorf("failed to read existing PID file: %w", err)
		}

		pid, err := strconv.Atoi(string(data))
		if err == nil {
			// Check if process is still running
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("daemon already running with PID %d", pid)
				}
			}
		}

		// Remove stale PID file
		os.Remove(d.pidFile)
	}

	// Write current PID
	pid := os.Getpid()
	if err := os.WriteFile(d.pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	d.logger.Printf("Created PID file: %s (PID: %d)", d.pidFile, pid)
	return nil
}

// setupSignalHandlers sets up signal handling for graceful shutdown
func (d *Daemon) setupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)

	// Register signal handlers
	signal.Notify(sigChan,
		syscall.SIGTERM, // Termination signal
		syscall.SIGINT,  // Interrupt signal (Ctrl+C)
		syscall.SIGHUP,  // Hangup signal (reload config)
		syscall.SIGUSR1, // User signal 1 (custom action)
		syscall.SIGUSR2, // User signal 2 (custom action)
	)

	go func() {
		for sig := range sigChan {
			d.logger.Printf("Received signal: %v", sig)

			switch sig {
			case syscall.SIGTERM, syscall.SIGINT:
				d.logger.Println("Initiating graceful shutdown...")
				d.cancel()
				return
			case syscall.SIGHUP:
				d.logger.Println("Received SIGHUP - configuration reload not implemented")
				// TODO: Implement configuration reload
			case syscall.SIGUSR1:
				d.logger.Println("Received SIGUSR1 - custom action not implemented")
				// TODO: Implement custom action (e.g., status dump)
			case syscall.SIGUSR2:
				d.logger.Println("Received SIGUSR2 - custom action not implemented")
				// TODO: Implement custom action (e.g., toggle debug mode)
			}
		}
	}()
}

// initDatabase initializes the database connection
func (d *Daemon) initDatabase() error {
	d.logger.Println("Connecting to database...")

	// Connect to database with migration
	database, err := db.ConnectAndMigrate(d.ctx, d.config.GetDatabaseConfig())
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	d.database = database
	d.logger.Println("Database connection established")
	return nil
}

// run executes the main daemon loop
func (d *Daemon) run() error {
	d.logger.Println("Entering main daemon loop...")

	// Main daemon loop
	for {
		select {
		case <-d.ctx.Done():
			d.logger.Println("Shutdown signal received")
			close(d.done)
			return nil

		case <-time.After(10 * time.Second):
			// Periodic health check or maintenance task
			d.performHealthCheck()
		}
	}
}

// performHealthCheck performs periodic health checks
func (d *Daemon) performHealthCheck() {
	// Check database connection
	if d.database != nil {
		if err := d.database.Ping(d.ctx); err != nil {
			d.logger.Printf("Database health check failed: %v", err)
			// TODO: Implement reconnection logic
		}
	}

	// TODO: Add more health checks
	// - Check scanning workers status
	// - Check API server status
	// - Monitor resource usage
}

// cleanup performs cleanup tasks
func (d *Daemon) cleanup() {
	d.logger.Println("Performing cleanup...")

	// Close database connection
	if d.database != nil {
		if err := d.database.Close(); err != nil {
			d.logger.Printf("Error closing database: %v", err)
		}
	}

	// Remove PID file
	if d.pidFile != "" {
		if err := os.Remove(d.pidFile); err != nil {
			d.logger.Printf("Error removing PID file: %v", err)
		} else {
			d.logger.Printf("Removed PID file: %s", d.pidFile)
		}
	}

	d.logger.Println("Cleanup completed")
}

// GetPID returns the daemon's PID
func (d *Daemon) GetPID() int {
	return os.Getpid()
}

// IsRunning checks if the daemon is running
func (d *Daemon) IsRunning() bool {
	select {
	case <-d.ctx.Done():
		return false
	default:
		return true
	}
}

// GetContext returns the daemon's context
func (d *Daemon) GetContext() context.Context {
	return d.ctx
}

// GetDatabase returns the database connection
func (d *Daemon) GetDatabase() *db.DB {
	return d.database
}

// GetConfig returns the daemon configuration
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}
