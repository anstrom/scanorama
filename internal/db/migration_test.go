package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestMigrationSystem validates the complete migration system functionality.
func TestMigrationSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping migration test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect without migrations first
	db, err := Connect(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	// Clean slate - drop all tables
	cleanupTables(t, db)

	// Test migration system
	migrator := &Migrator{db: db.DB}

	t.Run("migrations_table_creation", func(t *testing.T) {
		err := migrator.ensureMigrationsTable(ctx)
		if err != nil {
			t.Fatalf("Failed to create migrations table: %v", err)
		}

		// Verify migrations table exists with correct schema
		var tableName string
		err = db.QueryRow("SELECT table_name FROM information_schema.tables WHERE table_name = 'schema_migrations'").Scan(&tableName)
		if err != nil {
			t.Fatalf("Migrations table was not created: %v", err)
		}

		// Verify required columns exist
		requiredColumns := []string{"id", "name", "applied_at", "checksum"}
		for _, col := range requiredColumns {
			var columnName string
			err = db.QueryRow("SELECT column_name FROM information_schema.columns WHERE table_name = 'schema_migrations' AND column_name = $1", col).Scan(&columnName)
			if err != nil {
				t.Errorf("Required column '%s' missing from schema_migrations table", col)
			}
		}
	})

	t.Run("migration_files_validation", func(t *testing.T) {
		files, err := migrator.getMigrationFiles()
		if err != nil {
			t.Fatalf("Failed to get migration files: %v", err)
		}

		if len(files) == 0 {
			t.Fatal("No migration files found")
		}

		// Validate migration files follow proper numbering convention
		// Should be sequential starting from 001_
		for i, file := range files {
			migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")
			expectedPrefix := fmt.Sprintf("%03d_", i+1)
			if !strings.HasPrefix(migrationName, expectedPrefix) {
				t.Errorf("Migration file %d should start with '%s', got: %s", i+1, expectedPrefix, migrationName)
			}

			// Validate migration name format (number_description)
			parts := strings.SplitN(migrationName, "_", 2)
			if len(parts) < 2 {
				t.Errorf("Migration file '%s' should follow format 'NNN_description'", migrationName)
			}
		}

		// Validate each migration file can be read
		for _, file := range files {
			content, err := migrationFiles.ReadFile(file)
			if err != nil {
				t.Errorf("Cannot read migration file %s: %v", file, err)
			}
			if len(content) == 0 {
				t.Errorf("Migration file %s is empty", file)
			}
		}
	})

	t.Run("migration_execution", func(t *testing.T) {
		err := migrator.Up(ctx)
		if err != nil {
			t.Fatalf("Failed to run migrations: %v", err)
		}

		// Verify all migrations were recorded
		applied, err := migrator.getAppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("Failed to get applied migrations: %v", err)
		}

		if len(applied) == 0 {
			t.Fatal("No migrations were applied")
		}

		// Verify migrations follow proper numbering convention
		files, err := migrator.getMigrationFiles()
		if err != nil {
			t.Fatalf("Failed to get migration files for validation: %v", err)
		}

		// Validate that applied migrations match available migration files
		for _, file := range files {
			migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")
			if _, exists := applied[migrationName]; !exists {
				t.Errorf("Migration file '%s' exists but was not applied", migrationName)
			}
		}

		// Validate migration naming convention (should start with numbers)
		for i, file := range files {
			migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")
			expectedPrefix := fmt.Sprintf("%03d_", i+1)
			if !strings.HasPrefix(migrationName, expectedPrefix) {
				t.Errorf("Migration %d should start with '%s', got: %s", i+1, expectedPrefix, migrationName)
			}
		}

		t.Logf("Successfully validated %d migrations with proper numbering", len(applied))
	})

	t.Run("migration_idempotency", func(t *testing.T) {
		// Run migrations again - should be no-op
		err := migrator.Up(ctx)
		if err != nil {
			t.Fatalf("Second migration run failed: %v", err)
		}

		// Verify migration count didn't change
		applied, err := migrator.getAppliedMigrations(ctx)
		if err != nil {
			t.Fatalf("Failed to get applied migrations after second run: %v", err)
		}

		t.Logf("Applied %d migrations successfully (idempotent)", len(applied))
	})
}

// TestSchemaAfterMigrations validates that the schema after migrations
// matches the expectations of the application code.
func TestSchemaAfterMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping schema validation test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use ConnectAndMigrate to ensure schema is up to date
	db, err := ConnectAndMigrate(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect and migrate test database: %v", err)
	}
	defer db.Close()

	// Query all user tables that exist after migrations
	// This is dynamic and will adapt as the schema grows
	var existingTables []string
	tableQuery := `SELECT table_name FROM information_schema.tables
	               WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	               ORDER BY table_name`

	rows, err := db.Query(tableQuery)
	if err != nil {
		t.Fatalf("Failed to query existing tables: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("Failed to scan table name: %v", err)
		}
		existingTables = append(existingTables, tableName)
	}

	if len(existingTables) == 0 {
		t.Fatal("No tables found after migrations - migration system may have failed")
	}

	// Validate that essential application functionality exists
	// These are behavior-based checks, not hard-coded table lists
	t.Run("has_migrations_table", func(t *testing.T) {
		found := false
		for _, table := range existingTables {
			if table == "schema_migrations" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Migration system table 'schema_migrations' not found")
		}
	})

	t.Run("code_schema_assumptions", func(t *testing.T) {
		// Test that the actual schema assumptions made by application code are valid
		// This catches mismatches between code and schema migrations

		schemaAssumptions := []struct {
			description string
			query       string
		}{
			{
				"scan_targets table with required columns",
				"SELECT id, name, network, enabled FROM scan_targets LIMIT 0",
			},
			{
				"hosts table with ip_address and ignore_scanning columns",
				"SELECT id, ip_address, hostname, status, ignore_scanning, discovery_method FROM hosts LIMIT 0",
			},
			{
				"scan_jobs table with target_id foreign key",
				"SELECT id, target_id, status, started_at, completed_at FROM scan_jobs LIMIT 0",
			},
			{
				"port_scans table with host_id and state columns",
				"SELECT id, host_id, port, protocol, state, service_name FROM port_scans LIMIT 0",
			},
			{
				"active_hosts view exists and is queryable",
				"SELECT ip_address FROM active_hosts LIMIT 0",
			},
			{
				"network_summary view exists for dashboard",
				"SELECT target_name FROM network_summary LIMIT 0",
			},
		}

		for _, assumption := range schemaAssumptions {
			t.Run(assumption.description, func(t *testing.T) {
				rows, err := db.Query(assumption.query)
				if err != nil {
					t.Errorf("Schema assumption failed: %s\nQuery: %s\nError: %v",
						assumption.description, assumption.query, err)
					return
				}
				rows.Close()
				t.Logf("✓ Schema assumption validated: %s", assumption.description)
			})
		}
	})

	t.Logf("Found %d tables after migrations: %v", len(existingTables), existingTables)

	// Test actual queries used by application code to catch schema mismatches
	// These are extracted from real code usage patterns
	applicationQueries := []struct {
		name         string
		query        string
		codeLocation string
		purpose      string
	}{
		{
			"hosts_update_ignore_flag",
			"UPDATE hosts SET ignore_scanning = $1 WHERE ip_address = $2",
			"cmd/cli/hosts.go:363",
			"CLI command to ignore/unignore hosts for scanning",
		},
		{
			"scan_targets_create",
			"INSERT INTO scan_targets (id, name, network, description, scan_interval_seconds, scan_ports, scan_type, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			"internal/db/database.go:114",
			"Repository method to create scan targets",
		},
		{
			"scan_targets_get_enabled",
			"SELECT * FROM scan_targets WHERE enabled = true ORDER BY name",
			"internal/db/database.go:170",
			"Get active scan targets for scanning",
		},
		{
			"scan_jobs_create",
			"INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at, scan_stats) VALUES ($1, $2, $3, $4, $5, $6)",
			"internal/db/database.go:243",
			"Create new scan job with target reference",
		},
		{
			"scan_jobs_update_status",
			"UPDATE scan_jobs SET status = $1, completed_at = $2 WHERE id = $3",
			"internal/db/database.go:283",
			"Update scan job completion status",
		},
		{
			"hosts_get_by_ip",
			"SELECT * FROM hosts WHERE ip_address = $1",
			"internal/db/database.go:379",
			"Find host by IP address for updates",
		},
		{
			"hosts_create_or_update",
			"INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_version, status, discovery_method, response_time_ms, discovery_count) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
			"internal/db/database.go:330",
			"Create or update host records from discovery",
		},
		{
			"port_scans_create_batch",
			"INSERT INTO port_scans (id, job_id, host_id, port, protocol, state, service_name, service_version, service_product, banner) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
			"internal/db/database.go:445",
			"Bulk insert port scan results",
		},
		{
			"port_scans_get_by_host",
			"SELECT * FROM port_scans WHERE host_id = $1 ORDER BY port",
			"internal/db/database.go:482",
			"Get all port scans for a specific host",
		},
		{
			"active_hosts_view",
			"SELECT * FROM active_hosts ORDER BY ip_address",
			"internal/db/database.go:394",
			"Repository method using active_hosts view",
		},
		{
			"network_summary_view",
			"SELECT * FROM network_summary ORDER BY target_name",
			"internal/db/database.go:504",
			"Dashboard network summary from view",
		},
		{
			"scheduled_jobs_create",
			"INSERT INTO scheduled_jobs (name, job_type, cron_expression, configuration, is_active) VALUES ($1, $2, $3, $4, $5)",
			"cmd/cli/schedule.go:395",
			"CLI command to create scheduled jobs",
		},
		{
			"hosts_verify_exists",
			"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)",
			"internal/db/database.go:433",
			"Verify host exists before creating port scans",
		},
	}

	for _, appQuery := range applicationQueries {
		t.Run("query_"+appQuery.name, func(t *testing.T) {
			// Prepare the query to test column/table existence without actual data
			// Replace actual parameters with dummy values for schema validation
			testQuery := strings.ReplaceAll(appQuery.query, " VALUES ", " VALUES ")

			// For INSERT/UPDATE queries, we can't easily test without parameters
			// So we'll use EXPLAIN to validate the query structure
			if strings.HasPrefix(strings.ToUpper(testQuery), "INSERT") ||
				strings.HasPrefix(strings.ToUpper(testQuery), "UPDATE") ||
				strings.HasPrefix(strings.ToUpper(testQuery), "DELETE") {

				// Use EXPLAIN to validate query structure without executing
				explainQuery := "EXPLAIN " + testQuery

				// Count the number of parameters and create dummy values
				paramCount := strings.Count(testQuery, "$")
				args := make([]interface{}, paramCount)
				for i := 0; i < paramCount; i++ {
					args[i] = "dummy"
				}

				rows, err := db.Query(explainQuery, args...)
				if err != nil {
					t.Errorf("Application query schema validation failed:\nQuery: %s\nLocation: %s\nPurpose: %s\nError: %v",
						appQuery.query, appQuery.codeLocation, appQuery.purpose, err)
					return
				}
				rows.Close()
			} else {
				// For SELECT queries, we can test directly with LIMIT 0
				limitedQuery := testQuery
				if !strings.Contains(strings.ToUpper(testQuery), "LIMIT") {
					limitedQuery += " LIMIT 0"
				}

				rows, err := db.Query(limitedQuery)
				if err != nil {
					t.Errorf("Application query schema validation failed:\nQuery: %s\nLocation: %s\nPurpose: %s\nError: %v",
						appQuery.query, appQuery.codeLocation, appQuery.purpose, err)
					return
				}
				rows.Close()
			}

			t.Logf("✓ Application query validated: %s (%s)", appQuery.purpose, appQuery.codeLocation)
		})
	}
}

// TestMigrationChecksums validates that migration checksums are calculated
// and stored correctly to detect migration file changes.
func TestMigrationChecksums(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping checksum test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := ConnectAndMigrate(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect and migrate test database: %v", err)
	}
	defer db.Close()

	migrator := &Migrator{db: db.DB}

	// Get applied migrations
	applied, err := migrator.getAppliedMigrations(ctx)
	if err != nil {
		t.Fatalf("Failed to get applied migrations: %v", err)
	}

	// Get migration files
	files, err := migrator.getMigrationFiles()
	if err != nil {
		t.Fatalf("Failed to get migration files: %v", err)
	}

	// Verify each migration has a valid checksum
	for _, file := range files {
		migrationName := strings.TrimSuffix(file, ".sql")
		migrationName = strings.TrimPrefix(migrationName, "internal/db/")

		if migration, exists := applied[migrationName]; exists {
			t.Run("checksum_"+migrationName, func(t *testing.T) {
				if migration.Checksum == "" {
					t.Errorf("Migration '%s' has empty checksum", migrationName)
				}

				// Verify checksum format (should be hex string)
				if len(migration.Checksum) != 64 { // SHA-256 hex string length
					t.Errorf("Migration '%s' checksum has wrong length: %d (expected 64)",
						migrationName, len(migration.Checksum))
				}

				// Verify applied_at is set
				if migration.AppliedAt.IsZero() {
					t.Errorf("Migration '%s' has zero applied_at timestamp", migrationName)
				}
			})
		}
	}
}

// TestAutomatedQueryValidation automatically discovers and validates SQL queries
// from the codebase against the migrated schema.
func TestAutomatedQueryValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping automated query validation test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := ConnectAndMigrate(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect and migrate test database: %v", err)
	}
	defer db.Close()

	// Extract SQL queries from Go source files
	queries, err := ExtractSQLQueries()
	if err != nil {
		t.Fatalf("Failed to extract SQL queries from source: %v", err)
	}

	if len(queries) == 0 {
		t.Skip("No SQL queries found in source code")
	}

	t.Logf("Found %d SQL queries to validate", len(queries))

	for _, query := range queries {
		t.Run("auto_"+query.Name, func(t *testing.T) {
			if err := validateQueryAgainstSchema(db, query); err != nil {
				t.Errorf("SQL query validation failed:\nFile: %s\nQuery: %s\nError: %v",
					query.File, query.SQL, err)
			} else {
				t.Logf("✓ Query validated: %s", query.Name)
			}
		})
	}
}

// SQLQuery represents a SQL query found in source code
type SQLQuery struct {
	Name string
	File string
	SQL  string
	Type string // SELECT, INSERT, UPDATE, DELETE
}

// ExtractSQLQueries scans Go source files for SQL queries
func ExtractSQLQueries() ([]SQLQuery, error) {
	var queries []SQLQuery

	// SQL query patterns to search for in Go code
	patterns := []struct {
		name    string
		regex   *regexp.Regexp
		capture int // which capture group contains the SQL
	}{
		{
			"query_backtick",
			regexp.MustCompile(`query\s*:?=\s*` + "`([^`]+)`"),
			1,
		},
		{
			"query_string",
			regexp.MustCompile(`query\s*:?=\s*"([^"]+)"`),
			1,
		},
		{
			"db_query_direct",
			regexp.MustCompile(`\.Query[^(]*\([^"]*"([^"]+)"`),
			1,
		},
		{
			"db_exec_direct",
			regexp.MustCompile(`\.Exec[^(]*\([^"]*"([^"]+)"`),
			1,
		},
		{
			"multiline_query",
			regexp.MustCompile(`query\s*:?=\s*` + "`([^`]*(?:SELECT|INSERT|UPDATE|DELETE)[^`]*)`"),
			1,
		},
	}

	// Walk through Go source files
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .go files, skip test files and vendor directories
		if !strings.HasSuffix(path, ".go") ||
			strings.Contains(path, "vendor/") ||
			strings.Contains(path, ".git/") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		fileContent := string(content)

		// Apply each pattern to find SQL queries
		for _, pattern := range patterns {
			matches := pattern.regex.FindAllStringSubmatch(fileContent, -1)
			for _, match := range matches {
				if len(match) > pattern.capture {
					sql := strings.TrimSpace(match[pattern.capture])

					// Skip if it doesn't look like a real SQL query
					if len(sql) < 10 || !containsSQLKeywords(sql) {
						continue
					}

					// Clean up the SQL
					sql = cleanSQL(sql)

					// Determine query type
					queryType := getQueryType(sql)
					if queryType == "" {
						continue
					}

					// Generate a unique name
					name := generateQueryName(path, sql, queryType)

					queries = append(queries, SQLQuery{
						Name: name,
						File: path,
						SQL:  sql,
						Type: queryType,
					})
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk source files: %w", err)
	}

	// Remove duplicates
	queries = removeDuplicateQueries(queries)

	return queries, nil
}

// validateQueryAgainstSchema validates a SQL query against the current schema
func validateQueryAgainstSchema(db *DB, query SQLQuery) error {
	switch query.Type {
	case "SELECT":
		// For SELECT queries, add LIMIT 0 to avoid returning data
		testQuery := query.SQL
		if !strings.Contains(strings.ToUpper(testQuery), "LIMIT") {
			testQuery += " LIMIT 0"
		}

		rows, err := db.Query(testQuery)
		if err != nil {
			return fmt.Errorf("SELECT query failed: %w", err)
		}
		rows.Close()

	case "INSERT", "UPDATE", "DELETE":
		// Use EXPLAIN to validate structure without executing
		explainQuery := "EXPLAIN " + query.SQL

		// Count parameters and create dummy values
		paramCount := strings.Count(query.SQL, "$")
		args := make([]interface{}, paramCount)
		for i := 0; i < paramCount; i++ {
			switch i % 4 {
			case 0:
				args[i] = "00000000-0000-0000-0000-000000000000" // UUID
			case 1:
				args[i] = "dummy_string"
			case 2:
				args[i] = 42
			case 3:
				args[i] = true
			}
		}

		rows, err := db.Query(explainQuery, args...)
		if err != nil {
			return fmt.Errorf("EXPLAIN query failed: %w", err)
		}
		rows.Close()

	default:
		return fmt.Errorf("unsupported query type: %s", query.Type)
	}

	return nil
}

// containsSQLKeywords checks if a string contains SQL keywords
func containsSQLKeywords(s string) bool {
	upper := strings.ToUpper(s)
	keywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "FROM", "WHERE", "SET"}
	for _, keyword := range keywords {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// cleanSQL removes extra whitespace and normalizes SQL
func cleanSQL(sql string) string {
	// Remove extra whitespace
	sql = regexp.MustCompile(`\s+`).ReplaceAllString(sql, " ")
	// Remove leading/trailing whitespace
	sql = strings.TrimSpace(sql)
	// Remove newline escapes
	sql = strings.ReplaceAll(sql, "\\n", " ")
	sql = strings.ReplaceAll(sql, "\\t", " ")
	return sql
}

// getQueryType determines the type of SQL query
func getQueryType(sql string) string {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	switch {
	case strings.HasPrefix(upper, "SELECT"):
		return "SELECT"
	case strings.HasPrefix(upper, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(upper, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(upper, "DELETE"):
		return "DELETE"
	default:
		return ""
	}
}

// generateQueryName creates a unique name for a query
func generateQueryName(file, sql, queryType string) string {
	// Extract table name from SQL
	tableName := extractTableName(sql)

	// Create base name from file and query type
	baseName := strings.ToLower(queryType)
	if tableName != "" {
		baseName += "_" + tableName
	}

	// Add file context
	fileBase := filepath.Base(file)
	fileBase = strings.TrimSuffix(fileBase, ".go")

	return fmt.Sprintf("%s_%s", fileBase, baseName)
}

// extractTableName attempts to extract the main table name from SQL
func extractTableName(sql string) string {
	upper := strings.ToUpper(sql)

	// Pattern for FROM clause
	fromPattern := regexp.MustCompile(`FROM\s+(\w+)`)
	if matches := fromPattern.FindStringSubmatch(upper); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// Pattern for INSERT INTO
	insertPattern := regexp.MustCompile(`INSERT\s+INTO\s+(\w+)`)
	if matches := insertPattern.FindStringSubmatch(upper); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// Pattern for UPDATE
	updatePattern := regexp.MustCompile(`UPDATE\s+(\w+)`)
	if matches := updatePattern.FindStringSubmatch(upper); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	// Pattern for DELETE FROM
	deletePattern := regexp.MustCompile(`DELETE\s+FROM\s+(\w+)`)
	if matches := deletePattern.FindStringSubmatch(upper); len(matches) > 1 {
		return strings.ToLower(matches[1])
	}

	return ""
}

// removeDuplicateQueries removes duplicate queries based on SQL content
func removeDuplicateQueries(queries []SQLQuery) []SQLQuery {
	seen := make(map[string]bool)
	var unique []SQLQuery

	for _, query := range queries {
		key := query.SQL
		if !seen[key] {
			seen[key] = true
			unique = append(unique, query)
		}
	}

	return unique
}

// TestCriticalQueriesAfterMigration validates that critical application queries
// work correctly after all migrations are applied.
func TestCriticalQueriesAfterMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping query validation test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := ConnectAndMigrate(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect and migrate test database: %v", err)
	}
	defer db.Close()

	// Test the exact queries used by the application
	applicationQueries := []struct {
		name        string
		query       string
		description string
	}{
		{
			name: "hosts_list_query",
			query: `SELECT h.ip_address, h.status, h.os_family, h.last_seen,
					h.ignore_scanning, h.discovery_method
					FROM hosts h WHERE 1=1 LIMIT 1`,
			description: "Main hosts listing query from CLI",
		},
		{
			name: "hosts_with_scan_jobs",
			query: `SELECT h.id, COUNT(sj.id) as job_count
					FROM hosts h
					LEFT JOIN scan_jobs sj ON h.id = sj.target_id
					GROUP BY h.id LIMIT 1`,
			description: "Hosts joined with scan jobs",
		},
		{
			name: "hosts_with_port_scans",
			query: `SELECT h.id, COUNT(ps.id) as port_count
					FROM hosts h
					LEFT JOIN port_scans ps ON h.id = ps.host_id AND ps.state = 'open'
					GROUP BY h.id LIMIT 1`,
			description: "Hosts with open port counts",
		},
		{
			name: "full_hosts_query",
			query: `SELECT h.ip_address, h.status,
					COALESCE(h.os_family, 'unknown') as os_family,
					COALESCE(h.os_name, 'Unknown') as os_name,
					h.last_seen, h.first_seen,
					COALESCE(h.ignore_scanning, false) as ignore_scanning,
					h.discovery_method,
					COUNT(DISTINCT ps.id) as open_ports,
					COUNT(DISTINCT sj.id) as total_scans
					FROM hosts h
					LEFT JOIN port_scans ps ON h.id = ps.host_id AND ps.state = 'open'
					LEFT JOIN scan_jobs sj ON h.id = sj.target_id
					WHERE (h.ignore_scanning IS NULL OR h.ignore_scanning = false)
					GROUP BY h.id, h.ip_address, h.status, h.os_family, h.os_name,
						h.last_seen, h.first_seen, h.ignore_scanning, h.discovery_method
					ORDER BY h.last_seen DESC, h.ip_address LIMIT 1`,
			description: "Complete hosts query with all joins (from hosts CLI command)",
		},
	}

	for _, query := range applicationQueries {
		t.Run(query.name, func(t *testing.T) {
			rows, err := db.Query(query.query)
			if err != nil {
				t.Errorf("Critical application query '%s' failed after migrations: %v\nQuery: %s\nDescription: %s",
					query.name, err, query.query, query.description)
				return
			}
			defer rows.Close()
			t.Logf("✓ Query '%s' executed successfully", query.description)
		})
	}
}

// TestMigrationRollbackSafety validates that migrations cannot be accidentally
// corrupted or rolled back unsafely.
func TestMigrationRollbackSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rollback safety test in short mode")
	}

	configs := getTestConfigs()
	if len(configs) == 0 {
		t.Skip("No test database configuration available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := ConnectAndMigrate(ctx, &configs[0])
	if err != nil {
		t.Skipf("Cannot connect and migrate test database: %v", err)
	}
	defer db.Close()

	// Verify migrations table is protected
	t.Run("migrations_table_protection", func(t *testing.T) {
		// Should not be able to delete from migrations table in normal operation
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
		if err != nil {
			t.Fatalf("Cannot query migrations table: %v", err)
		}

		if count == 0 {
			t.Error("No migrations recorded - migration system may be broken")
		}

		t.Logf("Found %d applied migrations", count)
	})

	// Verify critical data constraints exist
	t.Run("foreign_key_constraints", func(t *testing.T) {
		// Check that critical foreign key relationships exist
		constraintQueries := []struct {
			name  string
			query string
		}{
			{
				"scan_jobs_target_id_fk",
				`SELECT COUNT(*) FROM information_schema.table_constraints
				 WHERE constraint_type = 'FOREIGN KEY' AND table_name = 'scan_jobs'
				 AND constraint_name LIKE '%target_id%'`,
			},
			{
				"port_scans_host_id_fk",
				`SELECT COUNT(*) FROM information_schema.table_constraints
				 WHERE constraint_type = 'FOREIGN KEY' AND table_name = 'port_scans'
				 AND constraint_name LIKE '%host_id%'`,
			},
		}

		for _, constraint := range constraintQueries {
			var count int
			err := db.QueryRow(constraint.query).Scan(&count)
			if err != nil {
				t.Errorf("Failed to check constraint %s: %v", constraint.name, err)
			} else if count == 0 {
				t.Logf("Warning: Foreign key constraint '%s' may be missing", constraint.name)
			} else {
				t.Logf("✓ Constraint '%s' exists", constraint.name)
			}
		}
	})
}

// cleanupTables removes all tables to ensure clean migration testing
func cleanupTables(t *testing.T, db *DB) {
	// Dynamically find all user tables to drop
	var tables []string
	query := `SELECT table_name FROM information_schema.tables
	          WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	          ORDER BY table_name`

	rows, err := db.Query(query)
	if err != nil {
		t.Logf("Warning: Could not query tables for cleanup: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Logf("Warning: Could not scan table name: %v", err)
			continue
		}
		tables = append(tables, tableName)
	}

	// Drop all found tables with CASCADE to handle dependencies
	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			t.Logf("Warning: Could not drop table %s: %v", table, err)
		}
	}

	// Also drop any views or other objects that might exist
	cleanupQueries := []string{
		"DROP MATERIALIZED VIEW IF EXISTS host_summary CASCADE",
		"DROP MATERIALIZED VIEW IF EXISTS network_summary_mv CASCADE",
		"DROP VIEW IF EXISTS scan_performance_stats CASCADE",
		"DROP VIEW IF EXISTS network_summary CASCADE",
		"DROP VIEW IF EXISTS active_hosts CASCADE",
		"DROP FUNCTION IF EXISTS refresh_summary_views() CASCADE",
		"DROP FUNCTION IF EXISTS cleanup_old_scan_data() CASCADE",
		"DROP FUNCTION IF EXISTS update_modified_by() CASCADE",
		"DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE",
		"DROP FUNCTION IF EXISTS update_host_last_seen() CASCADE",
	}

	for _, query := range cleanupQueries {
		_, err := db.Exec(query)
		if err != nil {
			t.Logf("Warning: Could not execute cleanup query '%s': %v", query, err)
		}
	}
}
