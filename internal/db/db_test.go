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

// test tables in order of dependency (children first, parents last)
var testTables = []string{
	"host_history",
	"services",
	"port_scans",
	"hosts",
	"scan_jobs",
	"scan_targets",
}

// getTestConfig returns database configuration for tests
// It tries to read from the test configuration file first,
// then environment variables, then falls back to defaults
func getTestConfig() Config {
	// Check if running in CI environment or debug mode
	isCI := os.Getenv("GITHUB_ACTIONS") == "true"
	isDebug := os.Getenv("DB_DEBUG") == "true"

	// Try to load from config file
	var err error
	var config Config

	if isCI {
		config, err = loadDBConfigFromFile("ci")
	} else {
		config, err = loadDBConfigFromFile("test")
	}

	if err == nil {
		if isDebug {
			fmt.Printf("Using database config from file: host=%s port=%d\n",
				config.Host, config.Port)
		}
		return config
	}

	// Use 5432 as the default port (PostgreSQL standard)
	defaultPort := 5432

	// Fall back to environment variables and defaults
	return Config{
		Host:            getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            getEnvIntOrDefault("TEST_DB_PORT", defaultPort),
		Database:        getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username:        getEnvOrDefault("TEST_DB_USER", "test_user"),
		Password:        getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}
}

// loadDBConfigFromFile loads database configuration from the test fixtures
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

	data, err := os.ReadFile(configFile)
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

// getEnvOrDefault gets a string from environment or returns the default
func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		// Log for debugging
		if os.Getenv("DB_DEBUG") == "true" {
			fmt.Printf("Using environment variable %s=%s\n", key, val)
		}
		return val
	}
	return defaultValue
}

// getEnvIntOrDefault gets an int from environment or returns the default
func getEnvIntOrDefault(key string, defaultValue int) int {
	isDebug := os.Getenv("DB_DEBUG") == "true"
	if val, ok := os.LookupEnv(key); ok && val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			// Log for debugging
			if isDebug {
				fmt.Printf("Using environment variable %s=%d\n", key, i)
			}
			return i
		}
	}
	if isDebug {
		fmt.Printf("Using default value for %s=%d\n", key, defaultValue)
	}
	return defaultValue
}

// waitForDB waits for the database to become available with timeout
func waitForDB(ctx context.Context, config Config) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	isDebug := os.Getenv("DB_DEBUG") == "true"
	if isDebug {
		fmt.Printf("Attempting to connect to database at %s:%d...\n", config.Host, config.Port)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for database: %w", timeoutCtx.Err())
		case <-ticker.C:
			db, err := Connect(timeoutCtx, config)
			if err == nil {
				fmt.Printf("Successfully connected to database at %s:%d\n", config.Host, config.Port)
				db.Close()
				return nil
			} else {
				// Log the error for debugging
				fmt.Printf("Database connection attempt failed: %v\n", err)
				// Add a small delay between connection attempts
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

// initializeSchema applies the database schema if tables don't exist
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

		schemaBytes, err := os.ReadFile(schemaPath)
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

// cleanupDB truncates all test tables in the correct order
func cleanupDB(db *DB) error {
	ctx := context.Background()
	for _, table := range testTables {
		// Check if table exists before attempting to truncate
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)",
			table).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check if table %s exists: %w", table, err)
		}

		if exists {
			_, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
			if err != nil {
				return fmt.Errorf("failed to truncate table %s: %w", table, err)
			}
		}
	}
	return nil
}

// setupTestDB creates a connection to the test database
// It returns a cleanup function that should be deferred
func setupTestDB(t *testing.T) (*DB, func()) {
	if testing.Short() {
		t.Skip("Skipping database tests in short mode")
	}

	isDebug := os.Getenv("DB_DEBUG") == "true"

	config := getTestConfig()
	ctx := context.Background()

	if isDebug {
		t.Logf("Using database config: host=%s port=%d user=%s db=%s",
			config.Host, config.Port, config.Username, config.Database)
	}

	// Try to connect to the database
	// Try to connect quickly to avoid long test times when database is not available
	if err := waitForDB(ctx, config); err != nil {
		t.Skipf("Skipping test - database not available: %v", err)
		return nil, func() {}
	}

	db, err := Connect(ctx, config)
	if err != nil {
		t.Skipf("Skipping test - failed to connect to database: %v", err)
		return nil, func() {}
	}

	// Initialize database schema if needed
	if err := initializeSchema(db); err != nil {
		db.Close()
		t.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Clean up database before test
	if err := cleanupDB(db); err != nil {
		db.Close()
		t.Fatalf("Failed to clean up database: %v", err)
	}

	return db, func() {
		// Clean up database after test
		if err := cleanupDB(db); err != nil {
			t.Errorf("Failed to clean up database after test: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
	}
}

// Connect test cases
func TestConnect(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Host:            "localhost",
				Port:            5432,
				Database:        "scanorama_test",
				Username:        "test_user",
				Password:        "test_password",
				SSLMode:         "disable",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
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
			db, err := Connect(ctx, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("Connect() expected error but got nil")
				}
				return
			}

			// If we expect success but couldn't connect, skip rather than fail
			// (but only for connection refused errors)
			if err != nil {
				if strings.Contains(err.Error(), "connection refused") ||
					strings.Contains(err.Error(), "connect: network is unreachable") {
					t.Skipf("Skipping test - database not available: %v", err)
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
	db, cleanup := setupTestDB(t)
	defer cleanup()

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
	db, cleanup := setupTestDB(t)
	defer cleanup()

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
	db, cleanup := setupTestDB(t)
	defer cleanup()

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

func TestQuery(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		return // Test was skipped
	}

	rows, err := db.QueryContext(context.Background(), "SELECT generate_series(1, 3)")
	if err != nil {
		t.Errorf("Query() error = %v", err)
		return
	}
	defer rows.Close()

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

func TestExec(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

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

func TestTransaction(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

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
			tx.Rollback()
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
			tx.Rollback()
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
