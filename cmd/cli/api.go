// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements the API server command for running a standalone API server
// without the full daemon functionality.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/api"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
)

const (
	// API server operation constants.
	apiServerStartupDelay = 500 // milliseconds to wait for server startup
	apiShutdownTimeout    = 30  // seconds to wait for graceful shutdown
)

var (
	apiHost string
	apiPort int
)

// apiCmd represents the api command.
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run scanorama API server",
	Long: `Run the scanorama REST API server as a standalone service.
This provides HTTP endpoints for managing scans, hosts, discovery jobs,
and accessing scan results without running the full daemon service.

The API server provides:
  - REST endpoints for CRUD operations
  - Real-time WebSocket connections for scan updates
  - Prometheus metrics endpoint
  - Health check and status endpoints`,
	Example: `  scanorama api
  scanorama api --host 0.0.0.0 --port 8080
  scanorama api --config /etc/scanorama/config.yaml`,
	RunE: runAPIServer,
}

func init() {
	rootCmd.AddCommand(apiCmd)

	// API-specific flags
	apiCmd.Flags().StringVar(&apiHost, "host", "", "API server host address (overrides config)")
	apiCmd.Flags().IntVar(&apiPort, "port", 0, "API server port (overrides config)")

	// Add detailed descriptions
	apiCmd.Flags().Lookup("host").Usage = "Host address to bind API server (empty = use config value)"
	apiCmd.Flags().Lookup("port").Usage = "Port number for API server (0 = use config value)"
}

// loadAndValidateConfig loads and validates the API configuration.
func loadAndValidateConfig() (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	// Override API configuration from command line flags
	if apiHost != "" {
		cfg.API.Host = apiHost
	}
	if apiPort > 0 {
		cfg.API.Port = apiPort
	}

	// Validate that API is enabled
	if !cfg.API.Enabled {
		return nil, fmt.Errorf("API server is disabled in configuration\n" +
			"Enable it by setting 'api.enabled: true' in config or use daemon mode")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// setupDatabase initializes and tests the database connection.
func setupDatabase(cfg *config.Config, logger *slog.Logger) (*db.DB, error) {
	logger.Info("Connecting to database...")
	database, err := db.ConnectAndMigrate(context.Background(), &cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	// Test database connection
	if err := database.Ping(context.Background()); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	logger.Info("Database connection established")
	return database, nil
}

// startAPIServer starts the API server and checks if it started successfully.
func startAPIServer(ctx context.Context, apiServer *api.Server, serverErrChan chan error) error {
	go func() {
		if err := apiServer.Start(ctx); err != nil {
			serverErrChan <- err
		}
	}()

	// Wait for startup
	time.Sleep(apiServerStartupDelay * time.Millisecond)

	// Check if server started successfully
	if !apiServer.IsRunning() {
		select {
		case err := <-serverErrChan:
			return fmt.Errorf("API server failed to start: %w", err)
		default:
			return fmt.Errorf("API server failed to start (unknown error)")
		}
	}

	return nil
}

// printStartupInfo prints verbose startup information.
func printStartupInfo(cfg *config.Config) {
	if verbose {
		fmt.Printf("API server configuration:\n")
		fmt.Printf("  Address: %s\n", cfg.GetAPIAddress())
		fmt.Printf("  Auth enabled: %t\n", cfg.API.AuthEnabled)
		fmt.Printf("  CORS enabled: %t\n", cfg.API.EnableCORS)
		fmt.Printf("  Rate limiting: %t\n", cfg.API.RateLimitEnabled)
		if cfg.API.AuthEnabled {
			fmt.Printf("  API keys configured: %d\n", len(cfg.API.APIKeys))
		}
	}
}

// printAPIEndpoints prints available API endpoints.
func printAPIEndpoints() {
	if verbose {
		fmt.Printf("Available endpoints:\n")
		fmt.Printf("  GET  /api/v1/health       - Health check\n")
		fmt.Printf("  GET  /api/v1/status       - System status\n")
		fmt.Printf("  GET  /api/v1/scans        - List scans\n")
		fmt.Printf("  POST /api/v1/scans        - Create scan\n")
		fmt.Printf("  GET  /api/v1/hosts        - List hosts\n")
		fmt.Printf("  GET  /api/v1/discovery    - List discovery jobs\n")
		fmt.Printf("  GET  /api/v1/profiles     - List scan profiles\n")
		fmt.Printf("  GET  /api/v1/schedules    - List schedules\n")
		fmt.Printf("  GET  /api/v1/metrics      - Prometheus metrics\n")
		fmt.Printf("  GET  /api/v1/ws/scans     - WebSocket: scan updates\n")
		fmt.Printf("  GET  /api/v1/ws/discovery - WebSocket: discovery updates\n")
	}
}

// gracefulShutdown handles graceful shutdown of the API server.
func gracefulShutdown(apiServer *api.Server) error {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), apiShutdownTimeout*time.Second)
	defer shutdownCancel()

	shutdownComplete := make(chan error, 1)
	go func() {
		shutdownComplete <- apiServer.Stop()
	}()

	select {
	case err := <-shutdownComplete:
		if err != nil {
			return fmt.Errorf("error during API server shutdown: %w", err)
		}
		fmt.Println("API server stopped gracefully")
		return nil

	case <-shutdownCtx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

func runAPIServer(cmd *cobra.Command, args []string) error {
	logger := logging.Default()

	if verbose {
		fmt.Println("Starting scanorama API server...")
	}

	// Load and validate configuration
	cfg, err := loadAndValidateConfig()
	if err != nil {
		return err
	}

	printStartupInfo(cfg)

	// Setup database
	database, err := setupDatabase(cfg, logger.Logger)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
		}
	}()

	// Create API server
	apiServer, err := api.New(cfg, database)
	if err != nil {
		return fmt.Errorf("failed to create API server: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start API server
	serverErrChan := make(chan error, 1)
	if err := startAPIServer(ctx, apiServer, serverErrChan); err != nil {
		return err
	}

	fmt.Printf("API server started successfully on %s\n", cfg.GetAPIAddress())
	fmt.Printf("Health check: http://%s/api/v1/health\n", cfg.GetAPIAddress())
	fmt.Printf("API documentation: http://%s/\n", cfg.GetAPIAddress())

	printAPIEndpoints()

	logger.Info("API server started", "address", cfg.GetAPIAddress())

	// Wait for shutdown signal or server error
	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
		fmt.Printf("\nReceived %s signal, shutting down gracefully...\n", sig.String())

	case err := <-serverErrChan:
		logger.Error("API server error", "error", err)
		fmt.Fprintf(os.Stderr, "API server error: %v\n", err)
	}

	// Cancel context to signal shutdown
	cancel()

	// Perform graceful shutdown
	return gracefulShutdown(apiServer)
}
