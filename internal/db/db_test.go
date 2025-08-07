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

// test tables in order of dependency (children first, parents last).
var testTables = []string{
	"host_history",
	"services",
	"port_scans",
	"hosts",
	"discovery_jobs",
	"scheduled_jobs",
	"scan_jobs",
	"scan_profiles",
	"scan_targets",
}

// It prioritizes environment variables, then tries config files, then defaults.
func getTestConfigs() []Config {
	// Check if running in CI environment or debug mode
	isCI := os.Getenv("GITHUB_ACTIONS") == trueString
	isDebug := os.Getenv("DB_DEBUG") == trueString

	// Use 5432 as the default port (PostgreSQL standard)
	defaultPort := 5432

	configs := []Config{
		// Try test database first
		{
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
		},
		// Fall back to development database
		{
			Host:            getEnvOrDefault("DEV_DB_HOST", "localhost"),
			Port:            getEnvIntOrDefault("DEV_DB_PORT", defaultPort),
			Database:        getEnvOrDefault("DEV_DB_NAME", "scanorama_dev"),
			Username:        getEnvOrDefault("DEV_DB_USER", "scanorama_dev"),
			Password:        getEnvOrDefault("DEV_DB_PASSWORD", "dev_password"),
			SSLMode:         "disable",
			MaxOpenConns:    2,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		},
	}

	// Try to load from config file only if no environment overrides are set
	if !hasEnvOverrides() {
		var fileConfig Config
		var err error

		if isCI {
			fileConfig, err = loadDBConfigFromFile("ci")
		} else {
			fileConfig, err = loadDBConfigFromFile("test")
		}

		if err == nil {
			// Use file config as first preference if no environment variables are set
			configs = append([]Config{fileConfig}, configs...)
		}
	}

	if isDebug {
		for i, config := range configs {
			fmt.Printf("Database config %d: host=%s port=%d user=%s db=%s\n",
				i+1, config.Host, config.Port, config.Username, config.Database)
		}
	}

	return configs
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
		// Log for debugging
		if os.Getenv("DB_DEBUG") == trueString {
			fmt.Printf("Using environment variable %s=%s\n", key, val)
		}
		return val
	}
	return defaultValue
}

// getEnvIntOrDefault gets an int from environment or returns the default.
func getEnvIntOrDefault(key string, defaultValue int) int {
	isDebug := os.Getenv("DB_DEBUG") == trueString
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

// waitForDB waits for the database to become available with timeout.
func waitForDB(ctx context.Context, config *Config) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Use the context timeout if already set, otherwise default to 10 seconds
	timeoutCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	isDebug := os.Getenv("DB_DEBUG") == trueString
	if isDebug {
		fmt.Printf("Attempting to connect to database at %s:%d...\n", config.Host, config.Port)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for database at %s:%d: %w", config.Host, config.Port, timeoutCtx.Err())
		case <-ticker.C:
			db, err := Connect(timeoutCtx, config)
			if err == nil {
				if isDebug {
					fmt.Printf("Successfully connected to database at %s:%d\n", config.Host, config.Port)
				}
				_ = db.Close()
				return nil
			} else {
				// Log the error for debugging
				if isDebug {
					fmt.Printf("Database connection attempt failed: %v\n", err)
				}
				// Add a small delay between connection attempts
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
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

// cleanupDB truncates all test tables in the correct order.
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

// tryDatabaseConnection attempts to connect to a single database configuration.
func tryDatabaseConnection(
	ctx context.Context, t *testing.T, config *Config,
	configIndex int, isDebug bool,
) (*DB, error) {
	if isDebug {
		t.Logf("Trying database config %d: host=%s port=%d user=%s db=%s",
			configIndex+1, config.Host, config.Port, config.Username, config.Database)
	}

	// Wait for database with longer timeout in CI
	waitCtx := ctx
	var cancelFunc context.CancelFunc
	if os.Getenv("GITHUB_ACTIONS") == trueString {
		waitCtx, cancelFunc = context.WithTimeout(ctx, 30*time.Second)
	}
	defer func() {
		if cancelFunc != nil {
			cancelFunc()
		}
	}()

	// Try to connect to this database
	if err := waitForDB(waitCtx, config); err != nil {
		if isDebug {
			t.Logf("Database config %d not available: %v", configIndex+1, err)
		}
		return nil, err
	}

	db, err := Connect(ctx, config)
	if err != nil {
		if isDebug {
			t.Logf("Failed to connect with config %d: %v", configIndex+1, err)
		}
		return nil, err
	}

	if isDebug {
		t.Logf("Successfully connected to database: %s", config.Database)
	}
	return db, nil
}

// findWorkingDatabase tries multiple database configurations until one works.
func findWorkingDatabase(ctx context.Context, t *testing.T) *DB {
	configs := getTestConfigs()
	isDebug := os.Getenv("DB_DEBUG") == trueString

	for i, config := range configs {
		if db, err := tryDatabaseConnection(ctx, t, &config, i, isDebug); err == nil {
			return db
		}
	}

	return nil
}

func setupTestDB(t *testing.T) (testDB *DB, cleanup func()) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping database tests in short mode")
	}

	ctx := context.Background()
	db := findWorkingDatabase(ctx, t)

	if db == nil {
		t.Skipf("Skipping test - no database available from any configuration")
		return nil, func() {}
	}

	// Initialize database schema if needed
	if err := initializeSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("Failed to initialize database schema: %v", err)
	}

	// Clean up database before test
	if err := cleanupDB(db); err != nil {
		_ = db.Close()
		t.Fatalf("Failed to clean up database: %v", err)
	}

	return db, func() {
		// Clean up database after test
		if err := cleanupDB(db); err != nil {
			t.Logf("Warning: Failed to clean up database: %v", err)
		}
		_ = db.Close()
	}
}

// Connect test cases.
func TestConnect(t *testing.T) {
	// Get available database configuration
	configs := getTestConfigs()

	// Use the first config as the valid config for testing
	validConfig := configs[0]

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
