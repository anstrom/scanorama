// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/logging"
)

const (
	// Default configuration constants.
	defaultDatabasePort         = 5432 // PostgreSQL default port
	defaultMaxConcurrentTargets = 100  // default max concurrent scan targets
)

var (
	cfgFile string
	verbose bool
)

// Build information - these will be set by ldflags during build.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "scanorama",
	Short: "Advanced Network Scanner",
	Long: `Scanorama is an advanced network scanning and discovery tool designed for
continuous network monitoring with OS-aware scanning capabilities, automated
scheduling, and robust database persistence.`,
	Version: getVersion(),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Bind flags to viper
	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to bind verbose flag: %v\n", err)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	// Read in environment variables that match
	viper.AutomaticEnv()
	viper.SetEnvPrefix("SCANORAMA")

	// Set defaults for common configuration
	setConfigDefaults()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}

	// Initialize structured logging after config is loaded
	initLogging()
}

// setConfigDefaults sets default values for configuration.
func setConfigDefaults() {
	// Database configuration
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", defaultDatabasePort)
	viper.SetDefault("database.database", "scanorama")
	viper.SetDefault("database.username", "scanorama")
	viper.SetDefault("database.ssl_mode", "require")

	// Scanning configuration
	viper.SetDefault("scanning.worker_pool_size", 10)
	viper.SetDefault("scanning.default_scan_type", "connect")
	viper.SetDefault("scanning.default_ports", "22,80,443,8080,8443")
	viper.SetDefault("scanning.max_concurrent_targets", defaultMaxConcurrentTargets)

	// Logging configuration
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "text")
	viper.SetDefault("logging.output", "stdout")
	viper.SetDefault("logging.structured", false)
	viper.SetDefault("logging.request_logging", true)
}

// getVersion returns the version string.
func getVersion() string {
	return fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildTime)
}

// SetVersion sets the version information (called from main).
func SetVersion(v, c, bt string) {
	version = v
	commit = c
	buildTime = bt
	rootCmd.Version = getVersion()
}

// initLogging initializes structured logging based on configuration.
func initLogging() {
	// Try to load full config for logging settings
	cfg, err := config.Load(viper.ConfigFileUsed())
	if err != nil {
		// If config loading fails, use default logging
		logger := logging.NewDefault()
		logging.SetDefault(logger)
		return
	}

	// Convert config logging to our logging config
	logConfig := logging.Config{
		Level:     logging.LogLevel(cfg.Logging.Level),
		Format:    logging.LogFormat(cfg.Logging.Format),
		Output:    cfg.Logging.Output,
		AddSource: cfg.Logging.Level == "debug",
	}

	// Create logger
	logger, err := logging.New(logConfig)
	if err != nil {
		// Fall back to default if creation fails
		logger = logging.NewDefault()
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logging: %v\n", err)
	}

	// Set as default logger
	logging.SetDefault(logger)

	// Log initialization if verbose
	if verbose {
		logging.Info("Structured logging initialized", "level", cfg.Logging.Level, "format", cfg.Logging.Format)
	}
}
