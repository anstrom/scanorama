// Package helpers provides testing utilities for database connections
// and test data management for Scanorama integration tests.
package helpers

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	_ "github.com/lib/pq"
)

// Constants for database configuration
const (
	defaultPostgreSQLPort = 5432
	dbConnectionTimeout   = 3 * time.Second
	retryDelay            = 500 * time.Millisecond
)

// TestDatabaseConfig holds the configuration for test database connection.
type TestDatabaseConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

// GetTestDatabaseConfig returns the test database configuration.
// It detects CI vs local environment and configures accordingly.
func GetTestDatabaseConfig() *TestDatabaseConfig {
	isCI := os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true"

	if isCI {
		// CI service container configuration
		return &TestDatabaseConfig{
			Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
			Port:     getEnvIntOrDefault("TEST_DB_PORT", defaultPostgreSQLPort),
			Database: getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
			Username: getEnvOrDefault("TEST_DB_USER", "scanorama_test_user"),
			Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password_123"),
			SSLMode:  getEnvOrDefault("TEST_DB_SSLMODE", "disable"),
		}
	} else {
		// Local docker container configuration
		return &TestDatabaseConfig{
			Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
			Port:     getEnvIntOrDefault("TEST_DB_PORT", defaultPostgreSQLPort),
			Database: getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
			Username: getEnvOrDefault("TEST_DB_USER", "test_user"),
			Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
			SSLMode:  getEnvOrDefault("TEST_DB_SSLMODE", "disable"),
		}
	}
}

// ConnectToTestDatabase connects to the test database.
// Expects database to be available via local docker or CI service container.
func ConnectToTestDatabase(ctx context.Context) (*db.DB, error) {
	config := GetTestDatabaseConfig()

	dbConfig := &db.Config{
		Host:            config.Host,
		Port:            config.Port,
		Database:        config.Database,
		Username:        config.Username,
		Password:        config.Password,
		SSLMode:         config.SSLMode,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: time.Minute,
	}

	database, err := db.Connect(ctx, dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test database at %s:%d/%s: %w",
			config.Host, config.Port, config.Database, err)
	}

	return database, nil
}

// SetupTestDatabase creates a test database connection and ensures schema is ready.
// Fails fast if database is not available.
func SetupTestDatabase(ctx context.Context) (*db.DB, func(), error) {
	database, err := ConnectToTestDatabase(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Database schema is set up by the container init script

	cleanup := func() {
		if database != nil {
			_ = database.Close()
		}
	}

	return database, cleanup, nil
}

// StartLocalTestDatabase starts a local PostgreSQL container for testing.
// This is used for local development when not in CI.
func StartLocalTestDatabase() error {
	isCI := os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true"
	if isCI {
		return nil // No need to start container in CI - service containers are used
	}

	// Check if container is already running
	if IsTestDatabaseAvailable() {
		return nil
	}

	// Start local test container using docker-compose
	// This should be handled by the test environment scripts
	return fmt.Errorf("local test database not available - run 'make test-db-up' or start test containers")
}

// IsTestDatabaseAvailable checks if the test database is available and accessible.
func IsTestDatabaseAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), dbConnectionTimeout)
	defer cancel()

	database, err := ConnectToTestDatabase(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = database.Close() }()

	// Test with a simple query
	var result int
	err = database.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	return err == nil && result == 1
}

// CreateTestDatabase is not needed with containerized setup.
func CreateTestDatabase() error {
	return nil
}

// WaitForTestDatabase waits for the test database to become available.
func WaitForTestDatabase(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		database, err := ConnectToTestDatabase(ctx)
		cancel()

		if err == nil {
			_ = database.Close()
			return nil
		}

		time.Sleep(retryDelay)
	}

	return fmt.Errorf("test database not available after %v timeout", timeout)
}

// getEnvOrDefault gets environment variable or returns default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault gets environment variable as int or returns default value.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
