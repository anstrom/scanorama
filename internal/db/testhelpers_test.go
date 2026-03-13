//go:build integration

package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const trueString = "true"

// getTestConfigs returns a prioritised list of database configurations to try.
// It prefers environment variables, then config files, then hardcoded defaults.
func getTestConfigs() []Config {
	isCI := os.Getenv("GITHUB_ACTIONS") == trueString
	isDebug := os.Getenv("DB_DEBUG") == trueString

	const defaultPort = 5432

	configs := []Config{
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

	if !hasEnvOverrides() {
		var fileConfig Config
		var err error
		if isCI {
			fileConfig, err = loadDBConfigFromFile("ci")
		} else {
			fileConfig, err = loadDBConfigFromFile("test")
		}
		if err == nil {
			configs = append([]Config{fileConfig}, configs...)
		}
	}

	if isDebug {
		for i, cfg := range configs {
			fmt.Printf("Database config %d: host=%s port=%d user=%s db=%s\n",
				i+1, cfg.Host, cfg.Port, cfg.Username, cfg.Database)
		}
	}

	return configs
}

// hasEnvOverrides reports whether any DB environment variables have been set.
func hasEnvOverrides() bool {
	for _, key := range []string{
		"TEST_DB_HOST", "TEST_DB_PORT", "TEST_DB_NAME",
		"TEST_DB_USER", "TEST_DB_PASSWORD",
	} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

// loadDBConfigFromFile loads a database configuration from
// test/fixtures/database.yml for the given environment label (e.g. "test", "ci").
func loadDBConfigFromFile(env string) (Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	candidates := []string{
		filepath.Join(wd, "..", "..", "test", "fixtures", "database.yml"),
		filepath.Join(wd, "..", "test", "fixtures", "database.yml"),
		filepath.Join(wd, "test", "fixtures", "database.yml"),
	}

	var configFile string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			configFile = p
			break
		}
	}
	if configFile == "" {
		return Config{}, fmt.Errorf("database.yml not found in any candidate path")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return Config{}, err
	}

	var configs map[string]Config
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return Config{}, err
	}

	cfg, ok := configs[env]
	if !ok {
		return Config{}, fmt.Errorf("environment %q not found in database.yml", env)
	}
	return cfg, nil
}

// getEnvOrDefault returns the value of key from the environment, or defaultValue
// if the variable is unset or empty.
func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if os.Getenv("DB_DEBUG") == trueString {
			fmt.Printf("Using environment variable %s=%s\n", key, val)
		}
		return val
	}
	return defaultValue
}

// getEnvIntOrDefault returns the integer value of key from the environment, or
// defaultValue if the variable is unset, empty, or not parseable as an int.
func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
			if os.Getenv("DB_DEBUG") == trueString {
				fmt.Printf("Using environment variable %s=%d\n", key, n)
			}
			return n
		}
	}
	if os.Getenv("DB_DEBUG") == trueString {
		fmt.Printf("Using default value for %s=%d\n", key, defaultValue)
	}
	return defaultValue
}

// waitForDB polls the given config until a connection succeeds or the context
// deadline is exceeded. A 10-second deadline is applied when the context has
// none of its own.
func waitForDB(ctx context.Context, config *Config) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		timeoutCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	isDebug := os.Getenv("DB_DEBUG") == trueString
	if isDebug {
		fmt.Printf("Waiting for database at %s:%d...\n", config.Host, config.Port)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for database at %s:%d: %w",
				config.Host, config.Port, timeoutCtx.Err())
		case <-ticker.C:
			db, err := Connect(timeoutCtx, config)
			if err == nil {
				_ = db.Close()
				if isDebug {
					fmt.Printf("Database at %s:%d is ready\n", config.Host, config.Port)
				}
				return nil
			}
			if isDebug {
				fmt.Printf("Database not yet ready: %v\n", err)
			}
		}
	}
}

// initializeSchema applies the initial schema migration if the scan_targets
// table does not yet exist.
func initializeSchema(db *DB) error {
	ctx := context.Background()

	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'scan_targets')").
		Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check for scan_targets table: %w", err)
	}
	if exists {
		return nil
	}

	_, currentFile, _, _ := runtime.Caller(0)
	schemaPath := filepath.Join(filepath.Dir(currentFile), "001_initial_schema.sql")

	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	if _, err := db.ExecContext(ctx, string(schemaBytes)); err != nil {
		return fmt.Errorf("failed to apply initial schema: %w", err)
	}
	return nil
}

// cleanupDB truncates all user tables in the public schema with CASCADE.
func cleanupDB(db *DB) error {
	ctx := context.Background()

	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration error: %w", err)
	}

	for _, table := range tables {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists); err != nil {
			return fmt.Errorf("failed to verify table %s: %w", table, err)
		}
		if !exists {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			return fmt.Errorf("failed to truncate table %s: %w", table, err)
		}
	}
	return nil
}

// tryDatabaseConnection attempts to reach a single database configuration.
// It returns the open *DB on success, or an error if the connection could not
// be established within the allowed window.
func tryDatabaseConnection(
	ctx context.Context, t *testing.T, config *Config,
	configIndex int, isDebug bool,
) (*DB, error) {
	t.Helper()

	if isDebug {
		t.Logf("Trying database config %d: host=%s port=%d user=%s db=%s",
			configIndex+1, config.Host, config.Port, config.Username, config.Database)
	}

	waitCtx := ctx
	var cancelFunc context.CancelFunc
	if os.Getenv("GITHUB_ACTIONS") == trueString {
		waitCtx, cancelFunc = context.WithTimeout(ctx, 30*time.Second)
		defer cancelFunc()
	}

	if err := waitForDB(waitCtx, config); err != nil {
		if isDebug {
			t.Logf("Config %d not reachable: %v", configIndex+1, err)
		}
		return nil, err
	}

	db, err := Connect(ctx, config)
	if err != nil {
		if isDebug {
			t.Logf("Connect failed for config %d: %v", configIndex+1, err)
		}
		return nil, err
	}

	if isDebug {
		t.Logf("Connected to %s", config.Database)
	}
	return db, nil
}

// findWorkingDatabase tries each config in turn and returns the first one that
// successfully connects, or nil if none are reachable.
func findWorkingDatabase(ctx context.Context, t *testing.T) *DB {
	t.Helper()
	isDebug := os.Getenv("DB_DEBUG") == trueString
	for i, cfg := range getTestConfigs() {
		if db, err := tryDatabaseConnection(ctx, t, &cfg, i, isDebug); err == nil {
			return db
		}
	}
	return nil
}

// setupTestDB opens a test database connection, applies the schema, and cleans
// up existing data. Tests that call this helper are integration tests: they are
// skipped automatically when no database is reachable or when -short is passed.
func setupTestDB(t *testing.T) (testDB *DB, cleanup func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	db := findWorkingDatabase(context.Background(), t)
	if db == nil {
		t.Skip("skipping test — no database available from any configuration")
		return nil, func() {}
	}

	if err := initializeSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("initializeSchema: %v", err)
	}

	if err := cleanupDB(db); err != nil {
		_ = db.Close()
		t.Fatalf("cleanupDB (before): %v", err)
	}

	return db, func() {
		if err := cleanupDB(db); err != nil {
			t.Logf("warning: cleanupDB (after): %v", err)
		}
		_ = db.Close()
	}
}
