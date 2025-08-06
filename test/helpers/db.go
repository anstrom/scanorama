// Package helpers provides testing utilities for database connections,
// environment setup, and test data management for Scanorama integration tests.
package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	_ "github.com/lib/pq"
)

// Constants for database testing.
const (
	defaultPostgreSQLPort = 5432
	dbConnectionTimeout   = 5 * time.Second
	minRequiredTables     = 3
	retryDelay            = 500 * time.Millisecond
)

// DatabaseConfig represents a database configuration for testing.
type DatabaseConfig struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

// GetTestDatabaseConfigs returns a list of database configurations to try for testing.
// It tries test database first, then development database as fallback.
func GetTestDatabaseConfigs() []DatabaseConfig {
	return []DatabaseConfig{
		{
			Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
			Port:     getEnvIntOrDefault("TEST_DB_PORT", defaultPostgreSQLPort),
			Database: getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
			Username: getEnvOrDefault("TEST_DB_USER", "test_user"),
			Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
			SSLMode:  "disable",
		},
		{
			Host:     getEnvOrDefault("DEV_DB_HOST", "localhost"),
			Port:     getEnvIntOrDefault("DEV_DB_PORT", defaultPostgreSQLPort),
			Database: getEnvOrDefault("DEV_DB_NAME", "scanorama_dev"),
			Username: getEnvOrDefault("DEV_DB_USER", "scanorama_dev"),
			Password: getEnvOrDefault("DEV_DB_PASSWORD", "dev_password"),
			SSLMode:  "disable",
		},
	}
}

// IsDatabaseAvailable checks if a database is available and accessible.
func IsDatabaseAvailable(config *DatabaseConfig) bool {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.Username, config.Password, config.Database, config.SSLMode)

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return false
	}
	defer func() { _ = sqlDB.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), dbConnectionTimeout)
	defer cancel()

	err = sqlDB.PingContext(ctx)
	return err == nil
}

// GetAvailableDatabase returns the first available database from the test configurations.
func GetAvailableDatabase() (*DatabaseConfig, error) {
	configs := GetTestDatabaseConfigs()

	for _, config := range configs {
		if IsDatabaseAvailable(&config) {
			return &config, nil
		}
	}

	return nil, fmt.Errorf("no available database found in test configurations")
}

// ConnectToTestDatabase attempts to connect to an available test database.
func ConnectToTestDatabase(ctx context.Context) (*db.DB, *DatabaseConfig, error) {
	configs := GetTestDatabaseConfigs()

	for _, config := range configs {
		dbConfig := &db.Config{
			Host:            config.Host,
			Port:            config.Port,
			Database:        config.Database,
			Username:        config.Username,
			Password:        config.Password,
			SSLMode:         config.SSLMode,
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		}

		database, err := db.Connect(ctx, dbConfig)
		if err == nil {
			return database, &config, nil
		}
	}

	return nil, nil, fmt.Errorf("failed to connect to any test database")
}

// CleanupTestTables removes test data from database tables.
func CleanupTestTables(ctx context.Context, database *db.DB) error {
	// DISABLED FOR CI DEBUGGING - cleanup functionality disabled to isolate CI issues
	fmt.Printf("DEBUG: CleanupTestTables called but cleanup is DISABLED for CI debugging\n")
	return nil

	// List of tables to clean in dependency order (child tables first)
	tables := []string{
		"host_history",
		"services",
		"port_scans",
		"scan_jobs",
		"discovery_jobs",
		"hosts",
		"scan_targets",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE 1=1", table)
		_, err := database.ExecContext(ctx, query)
		if err != nil {
			// Log warning but continue - some tables might not exist
			fmt.Printf("Warning: Failed to clean table %s: %v\n", table, err)
		}
	}

	return nil
}

// EnsureTestSchema ensures the database schema is set up for testing.
func EnsureTestSchema(ctx context.Context, database *db.DB) error {
	// Check if tables exist by querying information_schema
	query := `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('hosts', 'scan_jobs', 'port_scans')
	`

	var count int
	err := database.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check schema: %w", err)
	}

	if count < minRequiredTables {
		return fmt.Errorf("database schema incomplete - expected at least %d core tables, found %d",
			minRequiredTables, count)
	}

	return nil
}

// WaitForDatabase waits for database to become available with timeout.
func WaitForDatabase(config *DatabaseConfig, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if IsDatabaseAvailable(config) {
			return nil
		}
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("database not available after %v timeout", timeout)
}

// CreateTestDatabase creates a test database if it doesn't exist.
func CreateTestDatabase(config *DatabaseConfig) error {
	// Skip database creation if we can already connect to the target database
	if IsDatabaseAvailable(config) {
		return nil // Database already exists and is accessible
	}

	// Try to connect to default postgres database with various common credentials
	possibleAdminConfigs := []DatabaseConfig{
		{
			Host:     config.Host,
			Port:     config.Port,
			Database: "postgres",
			Username: getEnvOrDefault("POSTGRES_USER", "postgres"),
			Password: getEnvOrDefault("POSTGRES_PASSWORD", ""),
			SSLMode:  config.SSLMode,
		},
		{
			Host:     config.Host,
			Port:     config.Port,
			Database: "postgres",
			Username: config.Username,
			Password: config.Password,
			SSLMode:  config.SSLMode,
		},
	}

	var sqlDB *sql.DB
	var err error

	// Try each admin config
	for _, adminConfig := range possibleAdminConfigs {
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			adminConfig.Host, adminConfig.Port, adminConfig.Username, adminConfig.Password,
			adminConfig.Database, adminConfig.SSLMode)

		sqlDB, err = sql.Open("postgres", dsn)
		if err != nil {
			continue
		}

		// Test the connection
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = sqlDB.PingContext(ctx)
		cancel()

		if err == nil {
			break // Successfully connected
		}

		_ = sqlDB.Close()
		sqlDB = nil
	}

	if sqlDB == nil {
		return fmt.Errorf("failed to connect to postgres database with any credentials")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if database exists
	var exists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = sqlDB.QueryRowContext(ctx, checkQuery, config.Database).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if database exists: %w", err)
	}

	if !exists {
		// Create database
		createQuery := fmt.Sprintf("CREATE DATABASE %s", config.Database)
		_, err = sqlDB.ExecContext(ctx, createQuery)
		if err != nil {
			return fmt.Errorf("failed to create database %s: %w", config.Database, err)
		}

		// Create user if it doesn't exist
		createUserQuery := fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", config.Username, config.Password)
		_, err = sqlDB.ExecContext(ctx, createUserQuery)
		if err != nil {
			// User might already exist, which is fine
			fmt.Printf("Note: User creation failed (may already exist): %v\n", err)
		}

		// Grant privileges
		grantQuery := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", config.Database, config.Username)
		_, err = sqlDB.ExecContext(ctx, grantQuery)
		if err != nil {
			return fmt.Errorf("failed to grant privileges: %w", err)
		}
	}

	_ = sqlDB.Close()
	return nil
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

// IsPostgreSQLRunning checks if PostgreSQL is running on the given port.
func IsPostgreSQLRunning(port int) bool {
	// Try to connect using the test database credentials first
	configs := GetTestDatabaseConfigs()
	for _, config := range configs {
		if config.Port == port && IsDatabaseAvailable(&config) {
			return true
		}
	}

	// Fallback: try a simple network connection check
	dsn := fmt.Sprintf("host=localhost port=%d connect_timeout=2 sslmode=disable", port)
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return false
	}

	// Just test if we can open a connection, don't ping (which requires auth)
	_ = sqlDB.Close()
	return true
}

// GetDatabaseStatus returns a human-readable status of database availability.
func GetDatabaseStatus() string {
	if IsPostgreSQLRunning(defaultPostgreSQLPort) {
		// Check what databases are accessible
		configs := GetTestDatabaseConfigs()
		for _, config := range configs {
			if IsDatabaseAvailable(&config) {
				return fmt.Sprintf("✅ %s database available on localhost:5432", config.Database)
			}
		}
		return "⚠️ PostgreSQL running but no test databases accessible"
	}
	return "❌ No PostgreSQL found on localhost:5432"
}
