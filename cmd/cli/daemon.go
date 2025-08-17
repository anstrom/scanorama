// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"context"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/daemon"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
)

const (
	// Daemon operation constants.
	daemonStartupDelay     = 500 // milliseconds to wait for daemon startup
	daemonStopProgressStep = 5   // show progress every N seconds
	daemonStopTimeout      = 30  // seconds to wait before force kill
	statusLineLength       = 30  // characters for status separator line
)

var (
	daemonPidFile    string
	daemonLogFile    string
	daemonBackground bool
	daemonPort       int
)

// daemonCmd represents the daemon command.
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run scanorama as a background daemon",
	Long: `Run scanorama as a background daemon service that can execute
scheduled jobs, provide API endpoints, and perform continuous monitoring.
The daemon can be started, stopped, and monitored using subcommands.`,
	Example: `  scanorama daemon start
  scanorama daemon stop
  scanorama daemon status
  scanorama daemon restart`,
}

// daemonStartCmd represents the daemon start command.
var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the scanorama daemon",
	Long: `Start the scanorama daemon service in the background.
The daemon will process scheduled jobs and provide API endpoints.`,
	Example: `  scanorama daemon start
  scanorama daemon start --background
  scanorama daemon start --port 8080 --log-file /var/log/scanorama.log`,
	Run: runDaemonStart,
}

// daemonStopCmd represents the daemon stop command.
var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running scanorama daemon",
	Long: `Stop the currently running scanorama daemon service.
This will gracefully shut down the daemon and stop all background jobs.`,
	Example: `  scanorama daemon stop
  scanorama daemon stop --pid-file /var/run/scanorama.pid`,
	Run: runDaemonStop,
}

// daemonStatusCmd represents the daemon status command.
var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the scanorama daemon",
	Long: `Check whether the scanorama daemon is currently running
and display information about its status and configuration.`,
	Example: `  scanorama daemon status
  scanorama daemon status --pid-file /var/run/scanorama.pid`,
	Run: runDaemonStatus,
}

// daemonRestartCmd represents the daemon restart command.
var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the scanorama daemon",
	Long: `Stop the currently running daemon (if any) and start a new instance.
This is equivalent to running 'daemon stop' followed by 'daemon start'.`,
	Example: `  scanorama daemon restart
  scanorama daemon restart --background`,
	Run: runDaemonRestart,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonRestartCmd)

	// Persistent flags for all daemon commands
	daemonCmd.PersistentFlags().StringVar(&daemonPidFile, "pid-file", "/tmp/scanorama.pid", "Path to PID file")

	// Flags for start command
	daemonStartCmd.Flags().StringVar(&daemonLogFile, "log-file", "", "Path to log file (default: stdout)")
	daemonStartCmd.Flags().BoolVar(&daemonBackground, "background", true, "Run in background (detach from terminal)")
	daemonStartCmd.Flags().IntVar(&daemonPort, "port", 8080, "Port for API server")

	// Flags for restart command (inherit from start)
	daemonRestartCmd.Flags().StringVar(&daemonLogFile, "log-file", "", "Path to log file (default: stdout)")
	daemonRestartCmd.Flags().BoolVar(&daemonBackground, "background", true, "Run in background (detach from terminal)")
	daemonRestartCmd.Flags().IntVar(&daemonPort, "port", 8080, "Port for API server")

	// Add detailed descriptions
	daemonCmd.PersistentFlags().Lookup("pid-file").Usage = "File to store daemon process ID"
	daemonStartCmd.Flags().Lookup("log-file").Usage = "File to write daemon logs (empty = stdout)"
	daemonStartCmd.Flags().Lookup("background").Usage = "Detach from terminal and run in background"
	daemonStartCmd.Flags().Lookup("port").Usage = "Port number for daemon API server"
}

func runDaemonStart(_ *cobra.Command, _ []string) {
	// Check if daemon is already running
	if isDaemonRunning() {
		fmt.Fprintf(os.Stderr, "Daemon is already running (PID file: %s)\n", daemonPidFile)
		fmt.Fprintf(os.Stderr, "Use 'scanorama daemon stop' to stop it first, or 'daemon restart' to restart\n")
		os.Exit(1)
	}

	// Setup configuration
	cfg, err := config.Load(getConfigFilePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Test database connection
	database, err := db.Connect(context.Background(), &cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	// Ping database to ensure it's working
	if err := database.Ping(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Database connection test failed: %v\n", err)
		if closeErr := database.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
		}
		os.Exit(1)
	}

	// Close database connection after testing - daemon will create its own connections
	if closeErr := database.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
	}

	if verbose {
		fmt.Printf("Starting daemon with configuration:\n")
		fmt.Printf("  PID file: %s\n", daemonPidFile)
		fmt.Printf("  Log file: %s\n", getLogFileDesc())
		fmt.Printf("  Background: %t\n", daemonBackground)
		fmt.Printf("  API port: %d\n", daemonPort)
	}

	// Create and start daemon
	d := daemon.New(cfg)

	fmt.Printf("Starting scanorama daemon...\n")
	if daemonBackground {
		fmt.Printf("Daemon will run in background (PID file: %s)\n", daemonPidFile)
	}

	err = d.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	if !daemonBackground {
		// Running in foreground, this won't return until daemon stops
		fmt.Println("Daemon started successfully (running in foreground)")
	} else {
		// Give it a moment to start up
		time.Sleep(daemonStartupDelay * time.Millisecond)
		if isDaemonRunning() {
			fmt.Println("Daemon started successfully")
		} else {
			fmt.Fprintf(os.Stderr, "Daemon failed to start properly\n")
			os.Exit(1)
		}
	}
}

func runDaemonStop(_ *cobra.Command, _ []string) {
	if !isDaemonRunning() {
		fmt.Printf("Daemon is not running (no PID file found at %s)\n", daemonPidFile)
		return
	}

	pid, err := readPIDFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading PID file: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("Stopping daemon with PID %d...\n", pid)
	}

	// Send SIGTERM to daemon
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding daemon process: %v\n", err)
		os.Exit(1)
	}

	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending stop signal to daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait for daemon to stop (up to configured timeout)
	fmt.Printf("Stopping daemon (PID %d)...\n", pid)
	for i := 0; i < daemonStopTimeout; i++ {
		if !isDaemonRunning() {
			fmt.Println("Daemon stopped successfully")
			return
		}
		time.Sleep(1 * time.Second)
		if i%daemonStopProgressStep == (daemonStopProgressStep - 1) {
			fmt.Printf("Waiting for daemon to stop... (%d seconds)\n", i+1)
		}
	}

	// If still running after 30 seconds, force kill
	fmt.Printf("Daemon did not stop gracefully, sending SIGKILL...\n")
	err = process.Signal(syscall.SIGKILL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error force-killing daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait a bit more
	time.Sleep(2 * time.Second)
	if !isDaemonRunning() {
		fmt.Println("Daemon force-stopped")
	} else {
		fmt.Fprintf(os.Stderr, "Failed to stop daemon\n")
		os.Exit(1)
	}
}

func runDaemonStatus(_ *cobra.Command, _ []string) {
	fmt.Printf("Scanorama Daemon Status\n")
	fmt.Println(strings.Repeat("=", statusLineLength))

	if !isDaemonRunning() {
		fmt.Printf("Status: Not running\n")
		fmt.Printf("PID file: %s (not found)\n", daemonPidFile)
		return
	}

	pid, err := readPIDFile()
	if err != nil {
		fmt.Printf("Status: Unknown (error reading PID file: %v)\n", err)
		return
	}

	// Check if process is actually running
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Status: Not running (process not found)\n")
		fmt.Printf("PID file: %s (stale)\n", daemonPidFile)
		return
	}

	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		fmt.Printf("Status: Not running (process not responding)\n")
		fmt.Printf("PID file: %s (stale)\n", daemonPidFile)
		return
	}

	fmt.Printf("Status: Running\n")
	fmt.Printf("PID: %d\n", pid)
	fmt.Printf("PID file: %s\n", daemonPidFile)

	// Get additional info if available
	if info, err := os.Stat(daemonPidFile); err == nil {
		fmt.Printf("Started: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
		fmt.Printf("Uptime: %s\n", formatDuration(time.Since(info.ModTime())))
	}

	// Try to get daemon info via API (if available)
	// This would require the daemon to expose a status endpoint
	fmt.Printf("\nTo view daemon logs: tail -f %s\n", getLogFileDesc())
	fmt.Printf("To stop daemon: scanorama daemon stop\n")
}

func runDaemonRestart(cmd *cobra.Command, args []string) {
	fmt.Println("Restarting scanorama daemon...")

	// Stop existing daemon if running
	if isDaemonRunning() {
		fmt.Println("Stopping existing daemon...")
		runDaemonStop(cmd, args)

		// Wait a moment for clean shutdown
		time.Sleep(1 * time.Second)
	}

	// Start new daemon
	fmt.Println("Starting new daemon...")
	runDaemonStart(cmd, args)
}

func isDaemonRunning() bool {
	if _, err := os.Stat(daemonPidFile); os.IsNotExist(err) {
		return false
	}

	pid, err := readPIDFile()
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Send signal 0 to check if process is alive
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func readPIDFile() (int, error) {
	// #nosec G304 - daemonPidFile is a controlled path from command line flags
	data, err := os.ReadFile(daemonPidFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %v", err)
	}

	return pid, nil
}

func getLogFileDesc() string {
	if daemonLogFile == "" {
		return "stdout"
	}
	return daemonLogFile
}
