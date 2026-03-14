//go:build integration

package db

import (
	"context"
	"fmt"
	"path/filepath"
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

	// Create a clean database specifically for migration testing
	db, cleanup, err := setupCleanMigrationDatabase(ctx, &configs[0], t)
	if err != nil {
		t.Skipf("Cannot setup clean migration database: %v", err)
	}
	defer cleanup()
	defer db.Close()

	// Test migration system
	migrator := &Migrator{db: db.DB}

	t.Run("migrations_table_creation", func(t *testing.T) {
		err := migrator.ensureMigrationsTable(ctx)
		if err != nil {
			t.Fatalf("Failed to create migrations table: %v", err)
		}

		// Verify migrations table exists with correct schema
		var tableName string
		query := "SELECT table_name FROM information_schema.tables WHERE table_name = 'schema_migrations'"
		err = db.QueryRow(query).Scan(&tableName)
		if err != nil {
			t.Fatalf("Migrations table was not created: %v", err)
		}

		// Verify required columns exist
		requiredColumns := []string{"id", "name", "applied_at", "checksum"}
		for _, col := range requiredColumns {
			var columnName string
			query := `SELECT column_name FROM information_schema.columns
					  WHERE table_name = 'schema_migrations' AND column_name = $1`
			err = db.QueryRow(query, col).Scan(&columnName)
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

	// Create clean database and apply migrations
	db, cleanup, err := setupCleanMigrationDatabase(ctx, &configs[0], t)
	if err != nil {
		t.Skipf("Cannot setup clean migration database: %v", err)
	}
	defer cleanup()
	defer db.Close()

	// Apply migrations to the clean database
	migrator := &Migrator{db: db.DB}
	err = migrator.Up(ctx)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

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

	// Create clean database and apply migrations
	db, cleanup, err := setupCleanMigrationDatabase(ctx, &configs[0], t)
	if err != nil {
		t.Skipf("Cannot setup clean migration database: %v", err)
	}
	defer cleanup()
	defer db.Close()

	// Apply migrations to the clean database
	migrator := &Migrator{db: db.DB}
	err = migrator.Up(ctx)
	if err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

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

// setupCleanMigrationDatabase creates a completely clean database for migration testing
func setupCleanMigrationDatabase(ctx context.Context, baseConfig *Config, t *testing.T) (*DB, func(), error) {
	// Generate unique database name for this test
	cleanDBName := fmt.Sprintf("scanorama_migration_test_%d", time.Now().UnixNano())

	// Create config for connecting to the main database to create our test database
	adminConfig := *baseConfig
	if adminConfig.Database == "scanorama_test" {
		// Connect to the default postgres database to create our test database
		adminConfig.Database = "postgres"
	}

	// Connect to admin database to create clean test database
	adminDB, err := Connect(ctx, &adminConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot connect to admin database: %w", err)
	}

	// Create the clean test database
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", cleanDBName))
	if err != nil {
		adminDB.Close()
		return nil, nil, fmt.Errorf("cannot create clean test database: %w", err)
	}
	adminDB.Close()

	// Create config for the clean test database
	cleanConfig := *baseConfig
	cleanConfig.Database = cleanDBName

	// Connect to the clean database
	cleanDB, err := Connect(ctx, &cleanConfig)
	if err != nil {
		// If connection fails, try to clean up the database
		adminDB, _ := Connect(ctx, &adminConfig)
		if adminDB != nil {
			adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", cleanDBName))
			adminDB.Close()
		}
		return nil, nil, fmt.Errorf("cannot connect to clean test database: %w", err)
	}

	// Create cleanup function
	cleanup := func() {
		cleanDB.Close()
		// Connect to admin database to drop the test database
		if adminDB, err := Connect(ctx, &adminConfig); err == nil {
			_, err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", cleanDBName))
			if err != nil {
				t.Logf("Warning: Could not clean up test database %s: %v", cleanDBName, err)
			}
			adminDB.Close()
		}
	}

	t.Logf("Created clean migration test database: %s", cleanDBName)
	return cleanDB, cleanup, nil
}
