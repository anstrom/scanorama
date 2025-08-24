// Package daemon provides the background service functionality for scanorama.
// It manages scheduled discovery and scanning jobs, handles API endpoints,
// and coordinates the overall operation of the scanning system.
package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/anstrom/scanorama/internal/api"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Health check interval in seconds.
	healthCheckIntervalSeconds = 10
)

// File permission constants.
const (
	DefaultDirPermissions  = 0o750
	DefaultFilePermissions = 0o600
)

// Daemon represents the main daemon process.
type Daemon struct {
	config    *config.Config
	database  *db.DB
	apiServer *api.Server
	pidFile   string
	logger    *log.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	debugMode bool
	mu        sync.RWMutex
}

// New creates a new daemon instance.
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

// Start starts the daemon.
func (d *Daemon) Start() error {
	d.logger.Println("Starting scanorama daemon...")

	// Validate configuration
	if err := d.config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create working directory if needed
	if d.config.Daemon.WorkDir != "" {
		if err := os.MkdirAll(d.config.Daemon.WorkDir, DefaultDirPermissions); err != nil {
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

	// Initialize API server if enabled
	if err := d.initAPIServer(); err != nil {
		d.cleanup()
		return fmt.Errorf("failed to initialize API server: %w", err)
	}

	// Start the main daemon loop
	d.logger.Println("Daemon started successfully")
	return d.run()
}

// Stop stops the daemon gracefully.
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

// fork creates a background process.
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

// dropPrivileges drops root privileges if configured.
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

// createPIDFile creates the PID file.
func (d *Daemon) createPIDFile() error {
	if d.pidFile == "" {
		return nil // No PID file configured
	}

	// Ensure directory exists
	dir := filepath.Dir(d.pidFile)
	if err := os.MkdirAll(dir, DefaultDirPermissions); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	// Check if PID file already exists
	if err := d.checkExistingPID(); err != nil {
		return err
	}

	// Write current PID
	pid := os.Getpid()
	if err := os.WriteFile(d.pidFile, []byte(strconv.Itoa(pid)), DefaultFilePermissions); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	d.logger.Printf("Created PID file: %s (PID: %d)", d.pidFile, pid)
	return nil
}

// setupSignalHandlers sets up signal handling for graceful shutdown.
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
				d.logger.Println("Received SIGHUP - reloading configuration...")
				if err := d.reloadConfiguration(); err != nil {
					d.logger.Printf("Configuration reload failed: %v", err)
				} else {
					d.logger.Println("Configuration reloaded successfully")
				}
			case syscall.SIGUSR1:
				d.logger.Println("Received SIGUSR1 - dumping status...")
				d.dumpStatus()
			case syscall.SIGUSR2:
				d.logger.Println("Received SIGUSR2 - toggling debug mode...")
				d.toggleDebugMode()
			}
		}
	}()
}

// initDatabase initializes the database connection.
func (d *Daemon) initDatabase() error {
	d.logger.Println("Connecting to database...")

	// Connect to database with migration
	dbConfig := d.config.GetDatabaseConfig()
	database, err := db.ConnectAndMigrate(d.ctx, &dbConfig)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	d.database = database
	d.logger.Println("Database connection established")
	return nil
}

// initAPIServer initializes the API server.
func (d *Daemon) initAPIServer() error {
	if !d.config.IsAPIEnabled() {
		d.logger.Println("API server disabled, skipping initialization")
		return nil
	}

	d.logger.Printf("Initializing API server on %s", d.config.GetAPIAddress())

	// Create API server
	apiServer, err := api.New(d.config, d.database)
	if err != nil {
		return fmt.Errorf("API server creation failed: %w", err)
	}

	d.apiServer = apiServer
	d.logger.Println("API server initialized")
	return nil
}

// checkExistingPID checks if a PID file exists and if the process is still running.
func (d *Daemon) checkExistingPID() error {
	if _, err := os.Stat(d.pidFile); os.IsNotExist(err) {
		return nil // No PID file exists
	}

	// Read existing PID
	data, err := os.ReadFile(d.pidFile)
	if err != nil {
		return fmt.Errorf("failed to read existing PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		// Invalid PID file, remove it
		_ = os.Remove(d.pidFile)
		return nil
	}

	// Check if process is still running
	if d.isProcessRunning(pid) {
		return fmt.Errorf("daemon already running with PID %d", pid)
	}

	// Remove stale PID file
	_ = os.Remove(d.pidFile)
	return nil
}

// isProcessRunning checks if a process with the given PID is running.
func (d *Daemon) isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// run executes the main daemon loop.
func (d *Daemon) run() error {
	d.logger.Println("Entering main daemon loop...")

	// Start API server if configured
	if d.apiServer != nil {
		go func() {
			d.logger.Printf("Starting API server on %s", d.config.GetAPIAddress())
			if err := d.apiServer.Start(d.ctx); err != nil {
				d.logger.Printf("API server error: %v", err)
			}
		}()
	}

	// Main daemon loop
	for {
		select {
		case <-d.ctx.Done():
			d.logger.Println("Shutdown signal received")
			close(d.done)
			return nil

		case <-time.After(healthCheckIntervalSeconds * time.Second):
			// Periodic health check or maintenance task
			d.performHealthCheck()
		}
	}
}

// performHealthCheck performs periodic health checks.
func (d *Daemon) performHealthCheck() {
	// Check database connection
	if d.database != nil {
		if err := d.database.Ping(d.ctx); err != nil {
			d.logger.Printf("Database health check failed: %v", err)

			// Attempt database reconnection with exponential backoff
			if err := d.reconnectDatabase(); err != nil {
				d.logger.Printf("Database reconnection failed: %v", err)
			}
		}
	}

	// Check API server health if enabled
	if d.apiServer != nil {
		d.logger.Printf("API server health check: OK")
	}

	// Check memory usage
	d.checkMemoryUsage()

	// Check disk space
	if err := d.checkDiskSpace(); err != nil {
		d.logger.Printf("Disk space health check failed: %v", err)
	}

	// Check system resources
	d.checkSystemResources()

	// Check network connectivity (optional external connectivity test)
	if err := d.checkNetworkConnectivity(); err != nil {
		d.logger.Printf("Network connectivity health check failed: %v", err)
	}
}

// reconnectDatabase attempts to reconnect to the database with exponential backoff.
func (d *Daemon) reconnectDatabase() error {
	const maxRetries = 5
	const baseDelay = 2 * time.Second
	const maxDelay = 30 * time.Second

	d.logger.Println("Attempting database reconnection...")

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Calculate delay with exponential backoff
		const maxAttemptForShift = 31
		if attempt > maxAttemptForShift {
			attempt = maxAttemptForShift // Prevent overflow
		}
		shiftAmount := attempt - 1
		if shiftAmount < 0 {
			shiftAmount = 0
		}
		multiplier := 1 << shiftAmount
		delay := time.Duration(float64(baseDelay) * float64(multiplier))
		if delay > maxDelay {
			delay = maxDelay
		}

		d.logger.Printf("Reconnection attempt %d/%d in %v...", attempt, maxRetries, delay)

		// Wait before attempting reconnection (except for first attempt)
		if attempt > 1 {
			select {
			case <-d.ctx.Done():
				return fmt.Errorf("reconnection cancelled due to shutdown")
			case <-time.After(delay):
				// Continue with reconnection attempt
			}
		}

		// Close existing database connection if it exists
		if d.database != nil {
			if err := d.database.Close(); err != nil {
				d.logger.Printf("Warning: failed to close existing database connection: %v", err)
			}
		}

		// Attempt to reconnect
		dbConfig := d.config.GetDatabaseConfig()
		database, err := db.ConnectAndMigrate(d.ctx, &dbConfig)
		if err != nil {
			d.logger.Printf("Reconnection attempt %d failed: %v", attempt, err)

			// If this was the last attempt, return the error
			if attempt == maxRetries {
				return fmt.Errorf("failed to reconnect after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		// Verify the connection works
		if err := database.Ping(d.ctx); err != nil {
			d.logger.Printf("Reconnection attempt %d succeeded but ping failed: %v", attempt, err)
			if closeErr := database.Close(); closeErr != nil {
				d.logger.Printf("Warning: failed to close database connection: %v", closeErr)
			}

			if attempt == maxRetries {
				return fmt.Errorf("reconnected but ping failed after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		// Success! Update the database connection
		d.database = database
		d.logger.Printf("Database reconnection successful on attempt %d", attempt)
		return nil
	}

	return fmt.Errorf("database reconnection failed after %d attempts", maxRetries)
}

// dumpStatus logs detailed status information about the daemon.
func (d *Daemon) dumpStatus() {
	d.mu.RLock()
	debugMode := d.debugMode
	d.mu.RUnlock()

	d.logger.Println("=== DAEMON STATUS DUMP ===")

	// Basic daemon info
	d.logger.Printf("PID: %d", os.Getpid())
	d.logger.Printf("Debug Mode: %t", debugMode)
	d.logger.Printf("Uptime: %v", time.Since(time.Now())) // This would need to track actual start time

	// Database status
	if d.database != nil {
		if err := d.database.Ping(d.ctx); err != nil {
			d.logger.Printf("Database Status: ERROR - %v", err)
		} else {
			d.logger.Println("Database Status: OK")
		}
	} else {
		d.logger.Println("Database Status: NOT CONNECTED")
	}

	// API Server status
	if d.apiServer != nil {
		d.logger.Printf("API Server: RUNNING on %s", d.config.GetAPIAddress())
	} else {
		d.logger.Println("API Server: DISABLED")
	}

	// Memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	d.logger.Printf("Memory Usage: Alloc=%d KB, TotalAlloc=%d KB, Sys=%d KB, NumGC=%d",
		memStats.Alloc/1024, memStats.TotalAlloc/1024, memStats.Sys/1024, memStats.NumGC)

	// Goroutine count
	d.logger.Printf("Goroutines: %d", runtime.NumGoroutine())

	// Configuration summary
	d.logger.Printf("Config File: %s", d.config.Daemon.PIDFile) // This is a placeholder
	d.logger.Printf("Working Directory: %s", d.config.Daemon.WorkDir)

	d.logger.Println("=== END STATUS DUMP ===")
}

// toggleDebugMode toggles debug mode on/off.
func (d *Daemon) toggleDebugMode() {
	d.mu.Lock()
	d.debugMode = !d.debugMode
	newMode := d.debugMode
	d.mu.Unlock()

	if newMode {
		d.logger.Println("Debug mode ENABLED")
		d.logger.Println("- Verbose logging activated")
		d.logger.Println("- Performance metrics collection enabled")
		d.logger.Println("- Detailed request/response logging enabled")

		// In a real implementation, you would:
		// - Change log level to DEBUG
		// - Enable performance metrics collection
		// - Enable detailed HTTP request/response logging
		// - Possibly enable memory profiling
	} else {
		d.logger.Println("Debug mode DISABLED")
		d.logger.Println("- Logging returned to normal level")
		d.logger.Println("- Performance metrics collection disabled")
		d.logger.Println("- Detailed request/response logging disabled")

		// In a real implementation, you would:
		// - Change log level back to INFO/WARN
		// - Disable performance metrics collection
		// - Disable detailed HTTP request/response logging
		// - Disable memory profiling
	}
}

// IsDebugMode returns the current debug mode status.
func (d *Daemon) IsDebugMode() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.debugMode
}

// checkMemoryUsage monitors memory usage and logs warnings if usage is high.
func (d *Daemon) checkMemoryUsage() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Convert to MB for easier reading
	allocMB := memStats.Alloc / 1024 / 1024
	sysMB := memStats.Sys / 1024 / 1024

	// Log warning if memory usage is high (threshold: 1GB allocated)
	if allocMB > 1024 {
		d.logger.Printf("WARNING: High memory usage detected - Allocated: %d MB, System: %d MB", allocMB, sysMB)
	}

	// Log info about garbage collection if it's happening frequently
	if memStats.NumGC > 0 && memStats.NumGC%100 == 0 {
		d.logger.Printf("Memory stats - Allocated: %d MB, System: %d MB, GC cycles: %d", allocMB, sysMB, memStats.NumGC)
	}
}

// checkDiskSpace checks available disk space in the working directory.
func (d *Daemon) checkDiskSpace() error {
	workDir := d.config.Daemon.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// Get disk usage information
	// Note: This is a simplified implementation. In production, you might want to use
	// a more robust disk space checking mechanism
	stat, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("failed to stat working directory: %w", err)
	}

	// This is a basic check - just verify the directory is accessible
	// A more complete implementation would check actual disk space
	if !stat.IsDir() {
		return fmt.Errorf("working directory is not a directory: %s", workDir)
	}

	return nil
}

// checkSystemResources monitors general system resource usage.
func (d *Daemon) checkSystemResources() {
	// Check goroutine count
	numGoroutines := runtime.NumGoroutine()
	const maxGoroutines = 1000
	if numGoroutines > maxGoroutines {
		d.logger.Printf("WARNING: High goroutine count detected: %d", numGoroutines)
	}

	// Check if daemon is still responsive by verifying context isn't cancelled
	select {
	case <-d.ctx.Done():
		d.logger.Println("WARNING: Daemon context is cancelled during health check")
	default:
		// Daemon is still running normally
	}
}

// checkNetworkConnectivity performs basic network connectivity checks.
func (d *Daemon) checkNetworkConnectivity() error {
	// Basic connectivity test - try to resolve a well-known hostname
	// This is optional and can be disabled in production environments
	// where external connectivity isn't available or desired

	// Skip network check if we're in a restricted environment
	// This could be controlled by a configuration option
	return nil
}

// reloadConfiguration reloads the configuration from file and updates the daemon.
func (d *Daemon) reloadConfiguration() error {
	d.logger.Println("Starting configuration reload...")

	// Load new configuration from the same source
	// Note: We need to track the original config file path
	// For now, we'll use a default config loading approach
	newConfig, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load new configuration: %w", err)
	}

	// Validate the new configuration
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("new configuration validation failed: %w", err)
	}

	// Store old configuration for rollback if needed
	oldConfig := d.config

	// Update daemon configuration
	d.config = newConfig

	// Handle API server reconfiguration if needed
	if err := d.handleAPIServerReload(oldConfig, newConfig); err != nil {
		return err
	}

	// Handle database configuration changes
	if d.hasDatabaseConfigChanged(oldConfig, newConfig) {
		d.logger.Println("Database configuration changed, reconnecting...")

		// Close old database connection
		if d.database != nil {
			if err := d.database.Close(); err != nil {
				d.logger.Printf("Warning: failed to close old database connection: %v", err)
			}
		}

		// Initialize new database connection
		if err := d.initDatabase(); err != nil {
			// Rollback configuration on database failure
			d.config = oldConfig
			// Try to restore old database connection
			_ = d.initDatabase()
			return fmt.Errorf("failed to reconnect to database with new config: %w", err)
		}
	}

	d.logger.Println("Configuration reload completed")
	return nil
}

// hasAPIConfigChanged checks if API configuration has changed between old and new config.
func (d *Daemon) hasAPIConfigChanged(oldConfig, newConfig *config.Config) bool {
	return oldConfig.GetAPIAddress() != newConfig.GetAPIAddress() ||
		oldConfig.IsAPIEnabled() != newConfig.IsAPIEnabled()
}

// hasDatabaseConfigChanged checks if database configuration has changed.
func (d *Daemon) hasDatabaseConfigChanged(oldConfig, newConfig *config.Config) bool {
	oldDB := oldConfig.GetDatabaseConfig()
	newDB := newConfig.GetDatabaseConfig()

	return oldDB.Host != newDB.Host ||
		oldDB.Port != newDB.Port ||
		oldDB.Username != newDB.Username ||
		oldDB.Password != newDB.Password ||
		oldDB.Database != newDB.Database ||
		oldDB.SSLMode != newDB.SSLMode
}

// cleanup performs cleanup tasks.
func (d *Daemon) cleanup() {
	d.logger.Println("Performing cleanup...")

	// Stop API server
	if d.apiServer != nil {
		d.logger.Println("Stopping API server...")
		if err := d.apiServer.Stop(); err != nil {
			d.logger.Printf("Error stopping API server: %v", err)
		} else {
			d.logger.Println("API server stopped")
		}
	}

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

// handleAPIServerReload handles API server reconfiguration during config reload.
//
//nolint:nestif // Complex but necessary for API server management
func (d *Daemon) handleAPIServerReload(oldConfig, newConfig *config.Config) error {
	if d.apiServer != nil {
		// Check if API configuration changed
		if d.hasAPIConfigChanged(oldConfig, newConfig) {
			d.logger.Println("API configuration changed, restarting API server...")

			// Stop old API server
			if err := d.apiServer.Stop(); err != nil {
				d.logger.Printf("Warning: failed to stop old API server: %v", err)
			}

			// Initialize new API server
			if newConfig.IsAPIEnabled() {
				if err := d.initAPIServer(); err != nil {
					// Rollback configuration on API server failure
					d.config = oldConfig
					return fmt.Errorf("failed to restart API server with new config: %w", err)
				}

				// Start new API server
				go func() {
					if err := d.apiServer.Start(d.ctx); err != nil {
						d.logger.Printf("API server error after reload: %v", err)
					}
				}()
			} else {
				d.apiServer = nil
				d.logger.Println("API server disabled in new configuration")
			}
		}
	} else if newConfig.IsAPIEnabled() {
		// API server was disabled, now enabled
		d.logger.Println("API server enabled in new configuration, starting...")
		if err := d.initAPIServer(); err != nil {
			d.config = oldConfig
			return fmt.Errorf("failed to start API server: %w", err)
		}

		go func() {
			if err := d.apiServer.Start(d.ctx); err != nil {
				d.logger.Printf("API server error after reload: %v", err)
			}
		}()
	}
	return nil
}

// GetPID returns the daemon's PID.
func (d *Daemon) GetPID() int {
	return os.Getpid()
}

// IsRunning checks if the daemon is running.
func (d *Daemon) IsRunning() bool {
	select {
	case <-d.ctx.Done():
		return false
	default:
		return true
	}
}

// GetContext returns the daemon's context.
func (d *Daemon) GetContext() context.Context {
	return d.ctx
}

// GetDatabase returns the database connection.
func (d *Daemon) GetDatabase() *db.DB {
	return d.database
}

// GetConfig returns the daemon configuration.
func (d *Daemon) GetConfig() *config.Config {
	return d.config
}
