package db

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

const (
	trueString = "true"
)

// getTestConfig returns a single database configuration for testing.
func getTestConfig() Config {
	// Use 5432 as the default port (PostgreSQL standard)
	defaultPort := 5432

	// For CI, use postgres user and database name from environment
	if os.Getenv("GITHUB_ACTIONS") == trueString {
		return Config{
			Host:            getEnvOrDefault("TEST_DB_HOST", "localhost"),
			Port:            getEnvIntOrDefault("TEST_DB_PORT", defaultPort),
			Database:        getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
			Username:        getEnvOrDefault("TEST_DB_USER", "postgres"),
			Password:        getEnvOrDefault("TEST_DB_PASSWORD", "postgres"),
			SSLMode:         "disable",
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		}
	}

	// For local testing, use test database if available
	return Config{
		Host:            getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            getEnvIntOrDefault("TEST_DB_PORT", defaultPort),
		Database:        getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username:        getEnvOrDefault("TEST_DB_USER", "test_user"),
		Password:        getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}
}

// hasEnvOverrides checks if any database environment variables are set.
func hasEnvOverrides() bool {
	envVars := []string{"TEST_DB_HOST", "TEST_DB_PORT", "TEST_DB_NAME", "TEST_DB_USER", "TEST_DB_PASSWORD"}
	for _, envVar := range envVars {
		if _, exists := os.LookupEnv(envVar); exists {
			return true
		}
	}
	return false
}

// loadDBConfigFromFile loads database configuration from the test fixtures.
func loadDBConfigFromFile(env string) (Config, error) {
	// Find project root by looking for test directory
	wd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	// Look for database.yml in several possible locations
	possiblePaths := []string{
		filepath.Join(wd, "..", "..", "test", "fixtures", "database.yml"),
		filepath.Join(wd, "..", "test", "fixtures", "database.yml"),
		filepath.Join(wd, "test", "fixtures", "database.yml"),
	}

	var configFile string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			configFile = path
			break
		}
	}

	if configFile == "" {
		return Config{}, fmt.Errorf("database.yml not found")
	}

	data, err := os.ReadFile(configFile) //nolint:gosec // test file with controlled paths
	if err != nil {
		return Config{}, err
	}

	var configs map[string]Config
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return Config{}, err
	}

	config, ok := configs[env]
	if !ok {
		return Config{}, fmt.Errorf("environment %s not found in database.yml", env)
	}

	return config, nil
}

// getEnvOrDefault gets a string from environment or returns the default.
func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

// getEnvIntOrDefault gets an int from environment or returns the default.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}

	return defaultValue
}

// waitForDB waits for the database to become available with timeout.
func waitForDB(ctx context.Context, config *Config) error {
	// Use a much shorter timeout for testing to fail fast
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Try connecting immediately first
	db, err := Connect(timeoutCtx, config)
	if err == nil {
		_ = db.Close()
		return nil
	}

	// If immediate connection fails, return the error without retrying
	return fmt.Errorf("database not available at %s:%d: %w", config.Host, config.Port, err)
}

// initializeSchema applies the database schema if tables don't exist.
func initializeSchema(db *DB) error {
	ctx := context.Background()

	// Check if the main table exists
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'scan_targets')").Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if scan_targets table exists: %w", err)
	}

	if !exists {
		// Read and apply the schema from the main schema file
		// Get the current file's directory and build path to schema
		_, currentFile, _, _ := runtime.Caller(0)
		currentDir := filepath.Dir(currentFile)
		schemaPath := filepath.Join(currentDir, "001_initial_schema.sql")

		schemaBytes, err := os.ReadFile(schemaPath) //nolint:gosec // test file with controlled paths
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}

		_, err = db.ExecContext(ctx, string(schemaBytes))
		if err != nil {
			return fmt.Errorf("failed to initialize database schema: %w", err)
		}
	}

	return nil
}

// tryDatabaseConnection attempts to connect to a single database configuration.
func tryDatabaseConnection(
	ctx context.Context, _ *testing.T, config *Config,
	_ int,
) (*DB, error) {
	// Use a short timeout for fast failure
	connectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Try to connect directly
	db, err := Connect(connectCtx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s:%d/%s: %w",
			config.Host, config.Port, config.Database, err)
	}

	return db, nil
}

// findWorkingDatabase connects to the test database.
func findWorkingDatabase(ctx context.Context, t *testing.T) *DB {
	config := getTestConfig()
	db, err := tryDatabaseConnection(ctx, t, &config, 0)
	if err != nil {
		t.Logf("Database connection failed: %v", err)
		return nil
	}
	return db
}

// Shared database connection for all tests
var sharedTestDB *DB

func setupTestDB(t *testing.T) *DB {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping database tests in short mode")
	}

	// Use shared connection if already established
	if sharedTestDB != nil {
		return sharedTestDB
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := findWorkingDatabase(ctx, t)
	if db == nil {
		t.Skip("Skipping database test - no database available")
		return nil
	}

	// Initialize database schema if needed
	if err := initializeSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Store shared connection
	sharedTestDB = db
	return db
}

// Connect test cases.
func TestConnect(t *testing.T) {
	// Get available database configuration
	validConfig := getTestConfig()

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  validConfig,
			wantErr: false,
		},
		{
			name: "invalid port",
			config: Config{
				Host:            "localhost",
				Port:            0,
				Database:        "test_db",
				Username:        "test_user",
				Password:        "test_password",
				SSLMode:         "disable",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "empty host",
			config: Config{
				Host:            "",
				Port:            5432,
				Database:        "test_db",
				Username:        "test_user",
				Password:        "test_password",
				SSLMode:         "disable",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if testing.Short() && !tt.wantErr {
				t.Skip("Skipping database connection test in short mode")
			}

			ctx := context.Background()
			db, err := Connect(ctx, &tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("Connect() expected error but got nil")
				}
				return
			}

			// If we expect success but couldn't connect, handle gracefully
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") ||
					strings.Contains(err.Error(), "connect: network is unreachable") ||
					strings.Contains(err.Error(), "password authentication failed") ||
					strings.Contains(err.Error(), "database") && strings.Contains(err.Error(), "does not exist") {
					t.Skipf("Skipping test - database not available with this config: %v", err)
				} else {
					t.Errorf("Connect() unexpected error: %v", err)
				}
				return
			}

			if db != nil {
				if err := db.Close(); err != nil {
					t.Errorf("Failed to close database: %v", err)
				}
			}
		})
	}
}

func TestPing(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestRepositories(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	t.Run("ScanTargetRepository", func(t *testing.T) {
		repo := NewScanTargetRepository(db)

		// Create a new scan target
		description := "Test description"
		network := NetworkAddr{}
		_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
		network.IPNet = *ipnet

		target := &ScanTarget{
			ID:                  uuid.New(),
			Name:                "Test Target",
			Network:             network,
			Description:         &description,
			ScanIntervalSeconds: 3600,
			ScanPorts:           "22,80,443",
			ScanType:            "connect",
			Enabled:             true,
		}

		ctx := context.Background()
		err := repo.Create(ctx, target)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Get by ID
		retrieved, err := repo.GetByID(ctx, target.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.ID != target.ID {
			t.Errorf("GetByID() got ID = %v, want %v", retrieved.ID, target.ID)
		}

		if retrieved.Name != target.Name {
			t.Errorf("GetByID() got Name = %v, want %v", retrieved.Name, target.Name)
		}

		// Update the target
		target.Name = "Updated Target"
		err = repo.Update(ctx, target)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		// Verify the update
		updated, err := repo.GetByID(ctx, target.ID)
		if err != nil {
			t.Fatalf("GetByID() after update error = %v", err)
		}

		if updated.Name != "Updated Target" {
			t.Errorf("Update() failed, got Name = %v, want %v", updated.Name, "Updated Target")
		}

		// Get all targets
		allTargets, err := repo.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}

		if len(allTargets) != 1 {
			t.Errorf("GetAll() got %v targets, want 1", len(allTargets))
		}

		// Delete the target
		err = repo.Delete(ctx, target.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify it was deleted
		allTargets, err = repo.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() after delete error = %v", err)
		}

		if len(allTargets) != 0 {
			t.Errorf("Delete() failed, got %v targets, want 0", len(allTargets))
		}
	})
}

func TestQueryRow(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	ctx := context.Background()
	var result int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Errorf("QueryRow() error = %v", err)
	}

	if result != 1 {
		t.Errorf("QueryRow() = %v, want %v", result, 1)
	}
}

func TestQueryRows(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	rows, err := db.QueryContext(context.Background(), "SELECT generate_series(1, 3)")
	if err != nil {
		t.Errorf("Query() error = %v", err)
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Logf("Failed to close rows: %v", err)
		}
	}()

	var count int
	for rows.Next() {
		count++
		var val int
		if err := rows.Scan(&val); err != nil {
			t.Errorf("rows.Scan() error = %v", err)
		}
		if val != count {
			t.Errorf("Query() row %d = %v, want %v", count, val, count)
		}
	}

	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err() = %v", err)
	}

	if count != 3 {
		t.Errorf("Query() row count = %v, want %v", count, 3)
	}
}

func TestExecContext(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	result, err := db.ExecContext(context.Background(), "CREATE TEMPORARY TABLE test (id SERIAL PRIMARY KEY)")
	if err != nil {
		t.Errorf("ExecContext() error = %v", err)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Errorf("RowsAffected() error = %v", err)
	}

	if affected != 0 {
		t.Errorf("RowsAffected() = %v, want %v", affected, 0)
	}
}

func TestTransactions(t *testing.T) {
	db := setupTestDB(t)

	if db == nil {
		return // Test was skipped
	}

	ctx := context.Background()

	// Create a temporary table
	_, err := db.ExecContext(ctx, "CREATE TEMPORARY TABLE test_tx (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("Failed to create temp table: %v", err)
	}

	t.Run("Commit", func(t *testing.T) {
		tx, err := db.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx() error = %v", err)
		}

		// Insert in transaction
		_, err = tx.ExecContext(ctx, "INSERT INTO test_tx (value) VALUES ($1)", "test-commit")
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("Exec in tx error = %v", err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit() error = %v", err)
		}

		// Verify the insert was committed
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_tx WHERE value = $1", "test-commit").Scan(&count)
		if err != nil {
			t.Fatalf("QueryRow after commit error = %v", err)
		}

		if count != 1 {
			t.Errorf("After commit count = %v, want 1", count)
		}
	})

	t.Run("Rollback", func(t *testing.T) {
		tx, err := db.BeginTx(ctx)
		if err != nil {
			t.Fatalf("BeginTx() error = %v", err)
		}

		// Insert in transaction
		_, err = tx.ExecContext(ctx, "INSERT INTO test_tx (value) VALUES ($1)", "test-rollback")
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("Exec in tx error = %v", err)
		}

		// Rollback the transaction
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}

		// Verify the insert was rolled back
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_tx WHERE value = $1", "test-rollback").Scan(&count)
		if err != nil {
			t.Fatalf("QueryRow after rollback error = %v", err)
		}

		if count != 0 {
			t.Errorf("After rollback count = %v, want 0", count)
		}
	})
}
