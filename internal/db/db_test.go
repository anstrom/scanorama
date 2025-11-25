package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/errors"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

const (
	trueString = "true"
)

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

// cleanupDB truncates all user tables dynamically.
func cleanupDB(db *DB) error {
	ctx := context.Background()

	// Dynamically discover all user tables
	var tables []string
	query := `SELECT table_name FROM information_schema.tables
	          WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	          ORDER BY table_name`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query existing tables: %w", err)
	}
	defer rows.Close()

	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration error: %w", err)
	}

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Truncate all found tables with CASCADE to handle dependencies
	for _, table := range tables {
		// Check if table still exists before truncating (in case it was dropped by another operation)
		var exists bool
		checkQuery := `SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`
		err := db.QueryRowContext(ctx, checkQuery, table).Scan(&exists)
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
				// Check if it's our sanitized database connection error
				if strings.Contains(err.Error(), "DATABASE_CONNECTION") ||
					strings.Contains(err.Error(), "Failed to connect to database") ||
					strings.Contains(err.Error(), "Failed to verify database connection") {
					t.Skipf("Skipping test - database not available with this config: %v", err)
				} else if strings.Contains(err.Error(), "connection refused") ||
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

// TestSanitizeDBError tests the error sanitization function.
func TestSanitizeDBError(t *testing.T) {
	tests := []struct {
		name           string
		operation      string
		inputErr       error
		wantCode       errors.ErrorCode
		wantContains   string
		wantNotContain string
	}{
		{
			name:         "nil error returns nil",
			operation:    "test operation",
			inputErr:     nil,
			wantCode:     "",
			wantContains: "",
		},
		{
			name:         "sql.ErrNoRows returns NotFound",
			operation:    "get record",
			inputErr:     sql.ErrNoRows,
			wantCode:     errors.CodeNotFound,
			wantContains: "Resource not found",
		},
		{
			name:           "unique_violation (23505) returns Conflict",
			operation:      "create scan target",
			inputErr:       &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"},
			wantCode:       errors.CodeConflict,
			wantContains:   "Resource already exists",
			wantNotContain: "duplicate key",
		},
		{
			name:      "foreign_key_violation (23503) returns Validation",
			operation: "create record",
			inputErr: &pq.Error{
				Code:    "23503",
				Message: "insert or update on table violates foreign key constraint",
			},
			wantCode:       errors.CodeValidation,
			wantContains:   "Referenced resource does not exist",
			wantNotContain: "foreign key constraint",
		},
		{
			name:           "not_null_violation (23502) returns Validation",
			operation:      "insert record",
			inputErr:       &pq.Error{Code: "23502", Message: "null value in column violates not-null constraint"},
			wantCode:       errors.CodeValidation,
			wantContains:   "Required field is missing",
			wantNotContain: "null value",
		},
		{
			name:           "check_violation (23514) returns Validation",
			operation:      "update record",
			inputErr:       &pq.Error{Code: "23514", Message: "new row violates check constraint"},
			wantCode:       errors.CodeValidation,
			wantContains:   "Data validation failed",
			wantNotContain: "check constraint",
		},
		{
			name:         "query_canceled (57014) returns Canceled",
			operation:    "query",
			inputErr:     &pq.Error{Code: "57014", Message: "canceling statement due to user request"},
			wantCode:     errors.CodeCanceled,
			wantContains: "Database operation was canceled",
		},
		{
			name:         "admin_shutdown (57P01) returns DatabaseConnection",
			operation:    "query",
			inputErr:     &pq.Error{Code: "57P01", Message: "terminating connection due to administrator command"},
			wantCode:     errors.CodeDatabaseConnection,
			wantContains: "Database connection lost",
		},
		{
			name:         "connection error (08000) returns DatabaseConnection",
			operation:    "connect",
			inputErr:     &pq.Error{Code: "08000", Message: "connection exception"},
			wantCode:     errors.CodeDatabaseConnection,
			wantContains: "Database connection error",
		},
		{
			name:           "generic error is sanitized",
			operation:      "complex query",
			inputErr:       fmt.Errorf("pq: syntax error at or near SELECT"),
			wantCode:       errors.CodeDatabaseQuery,
			wantContains:   "Database operation failed: complex query",
			wantNotContain: "syntax error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDBError(tt.operation, tt.inputErr)

			if tt.inputErr == nil {
				if result != nil {
					t.Errorf("Expected nil for nil input, got: %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("Expected error but got nil")
			}

			// Check error code
			if tt.wantCode != "" {
				if !errors.IsCode(result, tt.wantCode) {
					t.Errorf("Expected error code %s, got code %s", tt.wantCode, errors.GetCode(result))
				}
			}

			// Check error message contains expected text
			errMsg := result.Error()
			if tt.wantContains != "" && !strings.Contains(errMsg, tt.wantContains) {
				t.Errorf("Expected error to contain %q, got: %s", tt.wantContains, errMsg)
			}

			// Check that sensitive details are NOT in the error
			if tt.wantNotContain != "" && strings.Contains(errMsg, tt.wantNotContain) {
				t.Errorf("Error message should NOT contain %q, but got: %s", tt.wantNotContain, errMsg)
			}
		})
	}
}

// TestRepositoryErrorSanitization tests that repository methods return sanitized errors.
func TestRepositoryErrorSanitization(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		return // Test was skipped
	}

	ctx := context.Background()
	repo := NewScanTargetRepository(db)

	t.Run("Create with duplicate returns sanitized conflict error", func(t *testing.T) {
		// Create a scan target
		network := NetworkAddr{}
		_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
		network.IPNet = *ipnet

		target1 := &ScanTarget{
			ID:      uuid.New(),
			Name:    "duplicate-test-target",
			Network: network,
		}

		err := repo.Create(ctx, target1)
		if err != nil {
			t.Skipf("Could not create initial target: %v", err)
		}

		// Try to create another with the same name
		target2 := &ScanTarget{
			ID:      uuid.New(),
			Name:    "duplicate-test-target",
			Network: network,
		}

		err = repo.Create(ctx, target2)
		if err == nil {
			t.Fatal("Expected conflict error for duplicate name")
		}

		// Check that error is sanitized
		errMsg := err.Error()
		if !strings.Contains(errMsg, "CONFLICT") && !strings.Contains(errMsg, "Resource already exists") {
			t.Errorf("Expected sanitized conflict error, got: %s", errMsg)
		}

		// Verify SQL details are NOT exposed
		if strings.Contains(strings.ToLower(errMsg), "constraint") ||
			strings.Contains(strings.ToLower(errMsg), "duplicate key") ||
			strings.Contains(strings.ToLower(errMsg), "unique_violation") {
			t.Errorf("Error message contains SQL details that should be sanitized: %s", errMsg)
		}
	})

	t.Run("GetByID with non-existent ID returns sanitized not found error", func(t *testing.T) {
		nonExistentID := uuid.New()
		target, err := repo.GetByID(ctx, nonExistentID)

		if err == nil {
			t.Fatal("Expected not found error")
		}

		if target != nil {
			t.Error("Expected nil target for not found error")
		}

		// Check that error is sanitized
		errMsg := err.Error()
		if !strings.Contains(errMsg, "NOT_FOUND") && !strings.Contains(errMsg, "Resource not found") {
			t.Errorf("Expected sanitized not found error, got: %s", errMsg)
		}
	})

	t.Run("Delete non-existent returns sanitized not found error", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := repo.Delete(ctx, nonExistentID)

		if err == nil {
			t.Fatal("Expected not found error")
		}

		// Check that error is sanitized
		errMsg := err.Error()
		if !strings.Contains(errMsg, "NOT_FOUND") && !strings.Contains(errMsg, "not found") {
			t.Errorf("Expected sanitized not found error, got: %s", errMsg)
		}
	})
}

// TestConnectionErrorSanitization tests that connection errors don't leak credentials.
func TestConnectionErrorSanitization(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		wantErrMsg string
		noContain  []string
	}{
		{
			name: "invalid host connection error is sanitized",
			config: Config{
				Host:            "invalid-nonexistent-host-12345",
				Port:            5432,
				Database:        "testdb",
				Username:        "testuser",
				Password:        "supersecretpassword123",
				SSLMode:         "disable",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
			wantErrMsg: "DATABASE_CONNECTION",
			noContain:  []string{"supersecretpassword123", "testuser", "testdb"},
		},
		{
			name: "invalid port connection error is sanitized",
			config: Config{
				Host:            "localhost",
				Port:            1,
				Database:        "testdb",
				Username:        "testuser",
				Password:        "mypassword",
				SSLMode:         "disable",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
			wantErrMsg: "DATABASE_CONNECTION",
			noContain:  []string{"mypassword", "testuser"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, err := Connect(ctx, &tt.config)

			if err == nil {
				t.Skip("Expected connection error but succeeded - database might be available")
			}

			errMsg := err.Error()

			// Check that error contains expected code
			if !strings.Contains(errMsg, tt.wantErrMsg) {
				t.Errorf("Expected error to contain %q, got: %s", tt.wantErrMsg, errMsg)
			}

			// Check that credentials are NOT in the error message
			for _, forbidden := range tt.noContain {
				if strings.Contains(errMsg, forbidden) {
					t.Errorf("Error message contains sensitive data %q: %s", forbidden, errMsg)
				}
			}
		})
	}
}

// TestSanitizeDBErrorPreservesCause verifies that sanitizeDBError preserves
// the original error in the Cause field for internal debugging.
func TestSanitizeDBErrorPreservesCause(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		inputErr  error
	}{
		{
			name:      "PostgreSQL unique violation preserves cause",
			operation: "create record",
			inputErr:  &pq.Error{Code: "23505", Message: "duplicate key value"},
		},
		{
			name:      "PostgreSQL foreign key violation preserves cause",
			operation: "insert record",
			inputErr: &pq.Error{
				Code:    "23503",
				Message: "foreign key constraint violation",
			},
		},
		{
			name:      "sql.ErrNoRows preserves cause",
			operation: "get record",
			inputErr:  sql.ErrNoRows,
		},
		{
			name:      "generic error preserves cause",
			operation: "query",
			inputErr:  fmt.Errorf("connection timeout"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeDBError(tt.operation, tt.inputErr)

			if result == nil {
				t.Fatal("Expected error but got nil")
			}

			// Verify it's a DatabaseError
			dbErr, ok := result.(*errors.DatabaseError)
			if !ok {
				t.Fatalf("Expected *errors.DatabaseError, got %T", result)
			}

			// Verify Operation is set
			if dbErr.Operation != tt.operation {
				t.Errorf("Expected operation %q, got %q", tt.operation, dbErr.Operation)
			}

			// Verify Cause is preserved
			if dbErr.Cause == nil {
				t.Error("Expected Cause to be preserved, but it was nil")
			}

			// Verify we can unwrap to get the original error
			unwrapped := dbErr.Unwrap()
			if unwrapped == nil {
				t.Error("Expected to unwrap original error, but got nil")
			}

			// For PostgreSQL errors, verify we can still access the original pq.Error
			if pqErr, ok := tt.inputErr.(*pq.Error); ok {
				unwrappedPQ, ok := unwrapped.(*pq.Error)
				if !ok {
					t.Errorf("Expected unwrapped error to be *pq.Error, got %T", unwrapped)
				} else if unwrappedPQ.Code != pqErr.Code {
					t.Errorf("Expected error code %s, got %s", pqErr.Code, unwrappedPQ.Code)
				}
			}

			// Verify the error message is sanitized (doesn't contain SQL details)
			errMsg := result.Error()
			if strings.Contains(strings.ToLower(errMsg), "duplicate key") ||
				strings.Contains(strings.ToLower(errMsg), "constraint") ||
				strings.Contains(strings.ToLower(errMsg), "foreign key") {
				t.Errorf("Error message not sanitized, contains SQL details: %s", errMsg)
			}
		})
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
