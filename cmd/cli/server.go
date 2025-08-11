// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements the server command with lifecycle management.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/api"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
)

// Timeout constants.
const (
	serverStartupTimeout     = 30 * time.Second
	serverShutdownTimeout    = 10 * time.Second
	serverHealthCheckRetries = 5
	serverHealthCheckDelay   = 1 * time.Second
	defaultLogLines          = 50
	databaseTimeout          = 5 * time.Second
	startupSleep             = 100 * time.Millisecond
)

// File permission constants.
const (
	dirPermissions  = 0750
	filePermissions = 0600
)

// Default file paths.
const (
	defaultPIDFile = "scanorama.pid"
	defaultLogFile = "scanorama.log"
)

// Server command flags.
var (
	serverForeground bool
	serverPIDFile    string
	serverLogFile    string
	serverHost       string
	serverPort       int
)

// serverCmd represents the server command and its subcommands.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server lifecycle management",
	Long: `Manage the Scanorama API server lifecycle.

The server command provides subcommands for starting, stopping,
and monitoring the Scanorama API server process.`,
	Example: `  scanorama server start
  scanorama server start --background
  scanorama server stop
  scanorama server status
  scanorama server restart
  scanorama server logs --follow`,
}

// serverStartCmd represents the server start command.
var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the API server",
	Long: `Start the Scanorama API server.

By default, the server runs in background mode. Use --foreground
to run in the current terminal.`,
	Example: `  scanorama server start
  scanorama server start --foreground
  scanorama server start --host 0.0.0.0 --port 8080`,
	RunE: runServerStart,
}

// serverStopCmd represents the server stop command.
var serverStopCmd = &cobra.Command{
	Use:     "stop",
	Short:   "Stop the API server",
	Long:    "Stop the running Scanorama API server.",
	Example: `  scanorama server stop`,
	RunE:    runServerStop,
}

// serverRestartCmd represents the server restart command.
var serverRestartCmd = &cobra.Command{
	Use:     "restart",
	Short:   "Restart the API server",
	Long:    "Stop and start the Scanorama API server.",
	Example: `  scanorama server restart`,
	RunE:    runServerRestart,
}

// serverStatusCmd represents the server status command.
var serverStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show server status",
	Long:    "Display the current status of the Scanorama API server.",
	Example: `  scanorama server status`,
	RunE:    runServerStatus,
}

// serverLogsCmd represents the server logs command.
var serverLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show server logs",
	Long:  "Display logs from the Scanorama API server.",
	Example: `  scanorama server logs
  scanorama server logs --follow
  scanorama server logs --lines 100`,
	RunE: runServerLogs,
}

func init() {
	// Add server command to root
	rootCmd.AddCommand(serverCmd)

	// Add subcommands
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverLogsCmd)

	// Server start flags
	serverStartCmd.Flags().BoolVar(&serverForeground, "foreground", false, "Run server in foreground mode")
	serverStartCmd.Flags().StringVar(&serverHost, "host", "", "Override server host")
	serverStartCmd.Flags().IntVar(&serverPort, "port", 0, "Override server port")
	serverStartCmd.Flags().StringVar(&serverPIDFile, "pid-file", defaultPIDFile, "PID file path")
	serverStartCmd.Flags().StringVar(&serverLogFile, "log-file", defaultLogFile, "Log file path")

	// Server stop flags
	serverStopCmd.Flags().StringVar(&serverPIDFile, "pid-file", defaultPIDFile, "PID file path")

	// Server restart flags
	serverRestartCmd.Flags().StringVar(&serverPIDFile, "pid-file", defaultPIDFile, "PID file path")
	serverRestartCmd.Flags().StringVar(&serverLogFile, "log-file", defaultLogFile, "Log file path")

	// Server status flags
	serverStatusCmd.Flags().StringVar(&serverPIDFile, "pid-file", defaultPIDFile, "PID file path")
	serverStatusCmd.Flags().StringVar(&serverLogFile, "log-file", defaultLogFile, "Log file path")

	// Server logs flags
	serverLogsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	serverLogsCmd.Flags().IntP("lines", "n", defaultLogLines, "Number of lines to show")
	serverLogsCmd.Flags().StringVar(&serverLogFile, "log-file", defaultLogFile, "Log file path")
}

// pidFileExists checks if a PID file exists and if the process is running.
// Returns the PID and whether the process is actually running.
func pidFileExists(pidFile string) (int, bool) {
	data, err := os.ReadFile(pidFile) //nolint:gosec // pidFile is controlled, not user input
	if err != nil {
		return 0, false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, false
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}

	// On Unix, Signal(0) tests if we can send a signal to the process
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}

	return pid, true
}

// writePIDFile writes the process ID to a file.
func writePIDFile(pidFile string, pid int) error {
	dir := filepath.Dir(pidFile)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create PID file directory: %w", err)
	}

	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), filePermissions)
}

// removePIDFile removes the PID file.
func removePIDFile(pidFile string) error {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(pidFile)
}

// setupLogFile ensures the log file directory exists.
func setupLogFile(logFile string) error {
	dir := filepath.Dir(logFile)
	if dir != "." {
		if err := os.MkdirAll(dir, dirPermissions); err != nil {
			return fmt.Errorf("failed to create log directory: %w", err)
		}
	}
	return nil
}

// checkServerLiveness performs a quick liveness check against the server.
func checkServerLiveness(address string) error {
	url := fmt.Sprintf("http://%s/api/v1/liveness", address)

	for i := 0; i < serverHealthCheckRetries; i++ {
		resp, err := http.Get(url) //nolint:gosec // URL is constructed from config, not user input
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		time.Sleep(serverHealthCheckDelay)
	}

	return fmt.Errorf("server liveness check failed after %d retries", serverHealthCheckRetries)
}

// checkServerHealth performs a comprehensive health check against the server.
func checkServerHealth(address string) error {
	url := fmt.Sprintf("http://%s/api/v1/health", address)

	for i := 0; i < serverHealthCheckRetries; i++ {
		resp, err := http.Get(url) //nolint:gosec // URL is constructed from config, not user input
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		time.Sleep(serverHealthCheckDelay)
	}

	return fmt.Errorf("server health check failed after %d retries", serverHealthCheckRetries)
}

// runServerInBackground starts the server process in background.
func runServerInBackground() error {
	// Check if server is already running
	if pid, running := pidFileExists(serverPIDFile); running {
		return fmt.Errorf("server is already running (PID %d)", pid)
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	args := buildServerArgs()

	if err := setupLogFile(serverLogFile); err != nil {
		return err
	}

	cmd, err := startBackgroundProcess(args)
	if err != nil {
		return err
	}

	return verifyServerStartup(cfg, cmd.Process.Pid)
}

// buildServerArgs constructs arguments for background server process.
func buildServerArgs() []string {
	args := []string{"server", "start", "--foreground"}
	if cfgFile != "" {
		args = append(args, "--config", cfgFile)
	}
	if verbose {
		args = append(args, "--verbose")
	}
	if serverHost != "" {
		args = append(args, "--host", serverHost)
	}
	if serverPort > 0 {
		args = append(args, "--port", strconv.Itoa(serverPort))
	}
	return args
}

// startBackgroundProcess starts the detached background process.
func startBackgroundProcess(args []string) (*exec.Cmd, error) {
	cmd := exec.Command(os.Args[0], args...) //nolint:gosec // args are controlled, not user input
	cmd.Env = os.Environ()

	//nolint:gosec // serverLogFile is controlled, not user input
	logFile, err := os.OpenFile(serverLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		_ = logFile.Close() // Close in parent process
	}()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach from parent process group on Unix systems
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server process: %w", err)
	}

	// Write PID file
	if err := writePIDFile(serverPIDFile, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("Server starting in background (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("PID file: %s\n", serverPIDFile)
	fmt.Printf("Log file: %s\n", serverLogFile)

	return cmd, nil
}

// verifyServerStartup checks if the server started successfully.
func verifyServerStartup(cfg *config.Config, pid int) error {
	fmt.Print("Waiting for server to start...")
	time.Sleep(2 * time.Second)

	// First check if the process is still running
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf(" failed\n")
		return fmt.Errorf("process with PID %d not found: %w", pid, err)
	}

	// On Unix systems, we can send signal 0 to check if process exists
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Printf(" failed\n")
		return fmt.Errorf("process with PID %d is not running: %w", pid, err)
	}

	// Then check if the server is responding to HTTP requests
	serverAddr := getServerAddress(cfg)
	if err := checkServerLiveness(serverAddr); err != nil {
		fmt.Printf(" failed\n")
		return fmt.Errorf("server failed to start properly: %w", err)
	}

	fmt.Printf(" done\n")
	fmt.Printf("Server is running at: http://%s (PID: %d)\n", serverAddr, pid)
	return nil
}

// getServerAddress determines the server address from config and flags.
func getServerAddress(cfg *config.Config) string {
	if serverHost != "" || serverPort > 0 {
		host := serverHost
		port := serverPort
		if host == "" {
			host = cfg.API.Host
		}
		if port == 0 {
			port = cfg.API.Port
		}
		return fmt.Sprintf("%s:%d", host, port)
	}
	return cfg.GetAPIAddress()
}

// runServerInForeground starts the server in foreground mode.
func runServerInForeground() error {
	logger := logging.Default()

	cfg, database, err := setupServerEnvironment(logger)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			logger.Error("Failed to close database connection", "error", closeErr)
		}
	}()

	apiServer, err := createAndStartServer(cfg, database, logger)
	if err != nil {
		return err
	}

	return waitForShutdown(apiServer, logger)
}

// setupServerEnvironment loads config, validates settings, and connects to database.
func setupServerEnvironment(logger *logging.Logger) (*config.Config, *db.DB, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading config: %w", err)
	}

	// Override API configuration from command line flags
	if serverHost != "" {
		cfg.API.Host = serverHost
	}
	if serverPort > 0 {
		cfg.API.Port = serverPort
	}

	// Validate that API is enabled
	if !cfg.API.Enabled {
		return nil, nil, fmt.Errorf("API server is disabled in configuration\n" +
			"Enable it by setting 'api.enabled: true' in config")
	}

	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Setup database
	logger.Info("Connecting to database...")
	database, err := db.ConnectAndMigrate(context.Background(), &cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("database connection failed: %w", err)
	}

	// Test database connection
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	if err := database.Ping(ctx); err != nil {
		return nil, nil, fmt.Errorf("database ping failed: %w", err)
	}
	logger.Info("Database connection successful")

	return cfg, database, nil
}

// createAndStartServer creates the API server and starts it.
func createAndStartServer(cfg *config.Config, database *db.DB, logger *logging.Logger) (*api.Server, error) {
	// Log version information
	fmt.Printf("Starting Scanorama API Server %s\n", getVersion())
	logger.Info("Starting Scanorama API Server",
		"version", version,
		"commit", commit,
		"build_time", buildTime,
		"address", cfg.GetAPIAddress())

	apiServer, err := api.New(cfg, database)
	if err != nil {
		return nil, fmt.Errorf("failed to create API server: %w", err)
	}

	// Write PID file in foreground mode too (for consistency)
	if !serverForeground {
		if err := writePIDFile(serverPIDFile, os.Getpid()); err != nil {
			logger.Warn("Failed to write PID file", "error", err)
		} else {
			defer func() {
				_ = removePIDFile(serverPIDFile)
			}()
		}
	}

	// Start server in background goroutine
	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrChan := make(chan error, 1)
	go func() {
		if err := apiServer.Start(serverCtx); err != nil {
			serverErrChan <- err
		}
	}()

	time.Sleep(startupSleep)

	fmt.Printf("API server started successfully on %s\n", cfg.GetAPIAddress())
	fmt.Printf("Health check: http://%s/api/v1/health\n", cfg.GetAPIAddress())
	fmt.Printf("Liveness check: http://%s/api/v1/liveness\n", cfg.GetAPIAddress())
	fmt.Printf("API documentation: http://%s/swagger/\n", cfg.GetAPIAddress())

	return apiServer, nil
}

// waitForShutdown handles graceful shutdown of the server.
func waitForShutdown(apiServer *api.Server, logger *logging.Logger) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	serverCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErrChan := make(chan error, 1)
	go func() {
		if err := apiServer.Start(serverCtx); err != nil {
			serverErrChan <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
		fmt.Printf("\nReceived %s signal, shutting down gracefully...\n", sig.String())

	case err := <-serverErrChan:
		logger.Error("API server error", "error", err)
		return fmt.Errorf("API server error: %w", err)
	}

	cancel()

	// Perform graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer shutdownCancel()

	shutdownComplete := make(chan error, 1)
	go func() {
		shutdownComplete <- apiServer.Stop()
	}()

	select {
	case err := <-shutdownComplete:
		if err != nil {
			logger.Error("Server shutdown error", "error", err)
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		fmt.Println("Server stopped successfully")

	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing stop")
		fmt.Println("Shutdown timeout exceeded, server may not have stopped cleanly")
		return fmt.Errorf("shutdown timeout exceeded")
	}

	return nil
}

// runServerStart handles the server start command.
func runServerStart(cmd *cobra.Command, args []string) error {
	if serverForeground {
		// Run in foreground mode
		return runServerInForeground()
	} else {
		// Run in background mode (default)
		return runServerInBackground()
	}
}

// runServerStop handles the server stop command.
func runServerStop(cmd *cobra.Command, args []string) error {
	pid, running := pidFileExists(serverPIDFile)
	if !running {
		fmt.Println("Server is not running")
		return nil
	}

	fmt.Printf("Stopping server (PID %d)...\n", pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send shutdown signal: %w", err)
	}

	// Wait for graceful shutdown
	fmt.Print("Waiting for graceful shutdown...")
	for i := 0; i < 30; i++ { // Wait up to 30 seconds
		if _, running := pidFileExists(serverPIDFile); !running {
			break
		}
		time.Sleep(1 * time.Second)
		fmt.Print(".")
	}

	// Check if process is still running
	if _, running := pidFileExists(serverPIDFile); running {
		fmt.Printf("\nGraceful shutdown timeout, forcing stop...\n")
		if err := process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("failed to force stop: %w", err)
		}
		time.Sleep(1 * time.Second)
	}

	// Clean up PID file
	if err := removePIDFile(serverPIDFile); err != nil {
		fmt.Printf("Warning: failed to remove PID file: %v\n", err)
	}

	fmt.Printf(" done\n")
	fmt.Println("Server stopped successfully")
	return nil
}

// runServerRestart handles the server restart command.
func runServerRestart(cmd *cobra.Command, args []string) error {
	// Stop the server first
	if pid, running := pidFileExists(serverPIDFile); running {
		fmt.Printf("Stopping server (PID %d)...\n", pid)
		if err := runServerStop(cmd, args); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}
		// Give it a moment to fully stop
		time.Sleep(2 * time.Second)
	}

	// Start the server
	fmt.Println("Starting server...")
	return runServerStart(cmd, args)
}

// runServerStatus handles the server status command.
func runServerStatus(cmd *cobra.Command, args []string) error {
	// Load config to get server address
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if server is responding via API
	fmt.Printf("Address: %s\n", cfg.GetAPIAddress())

	// First check liveness (quick check)
	err = checkServerLiveness(cfg.GetAPIAddress())
	if err != nil {
		fmt.Printf("Status: Server is not running (%v)\n", err)

		// Check for stale PID file
		if pid, _ := pidFileExists(serverPIDFile); pid > 0 {
			fmt.Printf("Stale PID file found: %s (PID %d)\n", serverPIDFile, pid)
		}
		return nil
	}

	fmt.Println("Status: Server is running")

	// Now check health (deeper dependency checks)
	err = checkServerHealth(cfg.GetAPIAddress())
	if err != nil {
		fmt.Printf("Health: Unhealthy (%v)\n", err)
	} else {
		fmt.Println("Health: Healthy")
	}

	// Show PID info if available
	if pid, running := pidFileExists(serverPIDFile); running {
		fmt.Printf("PID: %d\n", pid)
		fmt.Printf("PID file: %s\n", serverPIDFile)
		fmt.Printf("Log file: %s\n", serverLogFile)

		// Show process start time from PID file
		if stat, err := os.Stat(serverPIDFile); err == nil {
			fmt.Printf("Started: %s\n", stat.ModTime().Format("2006-01-02 15:04:05"))
		}
	} else {
		fmt.Println("Warning: Server is running but no PID file found")
	}

	return nil
}

// runServerLogs handles the server logs command.
func runServerLogs(cmd *cobra.Command, args []string) error {
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")

	// Check if log file exists
	if _, err := os.Stat(serverLogFile); os.IsNotExist(err) {
		return fmt.Errorf("log file does not exist: %s", serverLogFile)
	}

	if follow {
		// Use tail -f equivalent
		return tailLog(serverLogFile)
	} else {
		// Show last N lines
		return showLastLines(serverLogFile, lines)
	}
}

// showLastLines displays the last N lines of a file.
func showLastLines(filename string, lines int) error {
	file, err := os.Open(filename) //nolint:gosec // filename is controlled, not user input
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Read file in chunks from the end
	const bufSize = 8192
	var allLines []string

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	fileSize := stat.Size()
	offset := fileSize

	for len(allLines) < lines && offset > 0 {
		readSize := int64(bufSize)
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize

		buf := make([]byte, readSize)
		_, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return err
		}

		chunk := string(buf)
		chunkLines := strings.Split(chunk, "\n")
		allLines = append(chunkLines, allLines...)
	}

	// Show last N lines
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	for i := start; i < len(allLines); i++ {
		if strings.TrimSpace(allLines[i]) != "" {
			fmt.Println(allLines[i])
		}
	}

	return nil
}

// tailLog follows log file output (equivalent to tail -f).
func tailLog(filename string) error {
	file, err := os.Open(filename) //nolint:gosec // filename is controlled, not user input
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Seek to end of file
	_, err = file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	fmt.Printf("Following log file: %s (Ctrl+C to stop)\n", filename)

	// Follow file
	for {
		line := make([]byte, 1024)
		n, err := file.Read(line)
		if err == io.EOF {
			time.Sleep(startupSleep)
			continue
		}
		if err != nil {
			return err
		}
		fmt.Print(string(line[:n]))
	}
}
