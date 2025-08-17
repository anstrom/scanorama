// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// DatabaseOperation represents a function that operates on a database connection.
type DatabaseOperation func(*db.DB) error

// withDatabase executes the given operation with a database connection.
// It handles all database setup and cleanup, returning any errors that occur.
// This is the preferred method for operations that can handle errors gracefully.
func withDatabase(operation DatabaseOperation) error {
	// Load configuration
	cfg, err := config.Load(getConfigFilePath())
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	// Connect to database
	database, err := db.Connect(context.Background(), &cfg.Database)
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}

	// Ensure database is closed
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
		}
	}()

	// Execute the operation
	return operation(database)
}

// withDatabaseOrExit executes the given operation with a database connection.
// It handles all database setup and cleanup, and exits the program if any errors occur.
// This is suitable for CLI commands that should terminate on database errors.
func withDatabaseOrExit(operation func(*db.DB)) {
	err := withDatabase(func(database *db.DB) error {
		operation(database)
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// setupDatabaseForHostOperation validates an IP and sets up a database connection.
// This is a specialized helper for host-related operations.
func setupDatabaseForHostOperation(ip string) (*db.DB, error) {
	// Validate IP format
	if err := validateIP(ip); err != nil {
		return nil, fmt.Errorf("invalid IP address '%s': %v", ip, err)
	}

	// Setup database connection
	cfg, err := config.Load(getConfigFilePath())
	if err != nil {
		return nil, fmt.Errorf("error loading config: %v", err)
	}

	database, err := db.Connect(context.Background(), &cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	return database, nil
}

// withHostDatabaseOperation executes a host operation with proper database handling.
func withHostDatabaseOperation(ip string, operation func(*db.DB) error) {
	database, err := setupDatabaseForHostOperation(ip)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := operation(database); err != nil {
		if closeErr := database.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Close database connection on successful completion
	if closeErr := database.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", closeErr)
	}
}
