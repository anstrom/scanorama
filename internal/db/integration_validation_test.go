package db

import (
	"context"
	"testing"
	"time"
)

// TestDatabaseConfigDefaults ensures that default database configuration
// values are always compatible with the PostgreSQL driver.
func TestDatabaseConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Validate SSL mode is supported by PostgreSQL driver
	supportedSSLModes := []string{"disable", "require", "verify-ca", "verify-full"}
	validSSLMode := false
	for _, mode := range supportedSSLModes {
		if cfg.SSLMode == mode {
			validSSLMode = true
			break
		}
	}

	if !validSSLMode {
		t.Errorf("Default SSL mode '%s' is not supported by PostgreSQL driver. Supported modes: %v",
			cfg.SSLMode, supportedSSLModes)
	}

	// Validate other required fields have reasonable defaults
	if cfg.Host == "" {
		t.Error("Default host should not be empty")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		t.Errorf("Default port %d is not a valid port number", cfg.Port)
	}
	if cfg.MaxOpenConns <= 0 {
		t.Errorf("Default MaxOpenConns %d should be positive", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns < 0 {
		t.Errorf("Default MaxIdleConns %d should not be negative", cfg.MaxIdleConns)
	}
}

// TestQueryColumnConsistency validates that critical queries reference
// columns that should exist in the database schema.
func TestQueryColumnConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	// Try to connect to any available test database
	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := Connect(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	// Test queries that are critical to application functionality
	criticalQueries := []struct {
		name        string
		query       string
		description string
	}{
		{
			name: "hosts_list",
			query: `SELECT h.id, h.ip_address, h.status, h.os_family, h.last_seen,
					h.ignore_scanning, h.discovery_method
					FROM hosts h LIMIT 1`,
			description: "Main hosts query used by CLI",
		},
		{
			name: "hosts_with_scans",
			query: `SELECT h.id, COUNT(sj.id) as scan_count
					FROM hosts h
					LEFT JOIN scan_jobs sj ON h.id = sj.target_id
					GROUP BY h.id LIMIT 1`,
			description: "Hosts with scan job counts",
		},
		{
			name: "hosts_with_ports",
			query: `SELECT h.id, COUNT(ps.id) as port_count
					FROM hosts h
					LEFT JOIN port_scans ps ON h.id = ps.host_id
					GROUP BY h.id LIMIT 1`,
			description: "Hosts with port scan counts",
		},
	}

	for _, query := range criticalQueries {
		t.Run(query.name, func(t *testing.T) {
			rows, err := db.Query(query.query)
			if err != nil {
				t.Errorf("Critical query '%s' failed - likely due to column name mismatch: %v\nQuery: %s",
					query.description, err, query.query)
				return
			}
			defer rows.Close()
			if err := rows.Err(); err != nil {
				t.Errorf("Rows iteration error: %v", err)
				return
			}
			t.Logf("✓ Query '%s' executed successfully", query.description)
		})
	}
}

// TestConfigurationLoading ensures that configuration can be loaded
// with default values without external dependencies.
func TestConfigurationLoading(t *testing.T) {
	// Test that we can create a default config without panics or invalid values
	cfg := DefaultConfig()

	// Validate all required string fields have values
	if cfg.Host == "" {
		t.Error("Default host should not be empty")
	}
	if cfg.SSLMode == "" {
		t.Error("Default SSL mode should not be empty")
	}

	// Validate numeric fields are within reasonable ranges
	if cfg.Port < 1 || cfg.Port > 65535 {
		t.Errorf("Default port %d is not valid", cfg.Port)
	}
	if cfg.MaxOpenConns < 1 {
		t.Errorf("Default MaxOpenConns %d should be at least 1", cfg.MaxOpenConns)
	}

	// Validate timeout fields are positive
	if cfg.ConnMaxLifetime <= 0 {
		t.Errorf("Default ConnMaxLifetime %v should be positive", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime <= 0 {
		t.Errorf("Default ConnMaxIdleTime %v should be positive", cfg.ConnMaxIdleTime)
	}
}

// TestTableSchemaAssumptions validates that our code assumptions about
// database schema are correct. This catches schema drift.
func TestTableSchemaAssumptions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := Connect(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	// Check critical columns that code depends on
	schemaChecks := []struct {
		table  string
		column string
		reason string
	}{
		{"hosts", "ignore_scanning", "Used in hosts filtering queries"},
		{"scan_jobs", "target_id", "Used to join with hosts table"},
		{"hosts", "ip_address", "Primary identifier for hosts"},
		{"hosts", "status", "Used for filtering active hosts"},
		{"port_scans", "host_id", "Used to join with hosts table"},
		{"port_scans", "state", "Used to filter open ports"},
	}

	for _, check := range schemaChecks {
		t.Run(check.table+"_"+check.column, func(t *testing.T) {
			query := `SELECT column_name FROM information_schema.columns
					  WHERE table_name = $1 AND column_name = $2`

			var columnName string
			err := db.QueryRow(query, check.table, check.column).Scan(&columnName)
			if err != nil {
				t.Errorf("Expected column '%s.%s' not found: %s\nReason: %s",
					check.table, check.column, err, check.reason)
			}
		})
	}
}
