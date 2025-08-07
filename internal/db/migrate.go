package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

//go:embed *.sql
var migrationFiles embed.FS

// Migration represents a database migration.
type Migration struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	AppliedAt time.Time `db:"applied_at"`
	Checksum  string    `db:"checksum"`
}

// Migrator handles database migrations.
type Migrator struct {
	db *sqlx.DB
}

// NewMigrator creates a new migrator instance.
func NewMigrator(db *sqlx.DB) *Migrator {
	return &Migrator{db: db}
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist.
func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ DEFAULT NOW(),
			checksum VARCHAR(64) NOT NULL
		)`

	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

// getAppliedMigrations returns a list of already applied migrations.
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]Migration, error) {
	var migrations []Migration
	query := `SELECT id, name, applied_at, checksum FROM schema_migrations ORDER BY id`

	err := m.db.SelectContext(ctx, &migrations, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	applied := make(map[string]Migration)
	for _, migration := range migrations {
		applied[migration.Name] = migration
	}

	return applied, nil
}

// getMigrationFiles returns a sorted list of migration files.
func (m *Migrator) getMigrationFiles() ([]string, error) {
	var files []string

	err := fs.WalkDir(migrationFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, ".sql") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read migration files: %w", err)
	}

	sort.Strings(files)
	return files, nil
}

// calculateChecksum calculates a SHA-256 checksum for migration content.
func (m *Migrator) calculateChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// executeMigration executes a single migration file.
func (m *Migrator) executeMigration(ctx context.Context, filename string) error {
	content, err := migrationFiles.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", filename, err)
	}

	contentStr := string(content)
	checksum := m.calculateChecksum(contentStr)

	// Start transaction
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Execute the migration
	_, err = tx.ExecContext(ctx, contentStr)
	if err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", filename, err)
	}

	// Record the migration
	migrationName := strings.TrimSuffix(filepath.Base(filename), ".sql")
	insertQuery := `
		INSERT INTO schema_migrations (name, checksum)
		VALUES ($1, $2)`

	_, err = tx.ExecContext(ctx, insertQuery, migrationName, checksum)
	if err != nil {
		return fmt.Errorf("failed to record migration %s: %w", filename, err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration %s: %w", filename, err)
	}

	return nil
}

// Up runs all pending migrations.
func (m *Migrator) Up(ctx context.Context) error {
	// Ensure migrations table exists
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get migration files
	files, err := m.getMigrationFiles()
	if err != nil {
		return err
	}

	// Execute pending migrations
	for _, file := range files {
		migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")

		if _, exists := applied[migrationName]; exists {
			fmt.Printf("Migration %s already applied, skipping\n", migrationName)
			continue
		}

		fmt.Printf("Applying migration %s...\n", migrationName)
		if err := m.executeMigration(ctx, file); err != nil {
			return fmt.Errorf("migration %s failed: %w", migrationName, err)
		}
		fmt.Printf("Migration %s applied successfully\n", migrationName)
	}

	return nil
}

// Status shows the current migration status.
func (m *Migrator) Status(ctx context.Context) error {
	// Ensure migrations table exists
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get migration files
	files, err := m.getMigrationFiles()
	if err != nil {
		return err
	}

	fmt.Println("Migration Status:")
	fmt.Println("=================")

	for _, file := range files {
		migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")

		if migration, exists := applied[migrationName]; exists {
			fmt.Printf("✓ %s (applied at %s)\n", migrationName, migration.AppliedAt.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Printf("✗ %s (pending)\n", migrationName)
		}
	}

	return nil
}

// Reset drops all tables and re-runs migrations (USE WITH CAUTION).
func (m *Migrator) Reset(ctx context.Context) error {
	fmt.Println("WARNING: This will drop all tables and data!")
	fmt.Println("This operation cannot be undone.")

	// Drop all tables
	dropQueries := []string{
		"DROP TABLE IF EXISTS host_history CASCADE",
		"DROP TABLE IF EXISTS services CASCADE",
		"DROP TABLE IF EXISTS port_scans CASCADE",
		"DROP TABLE IF EXISTS hosts CASCADE",
		"DROP TABLE IF EXISTS scan_jobs CASCADE",
		"DROP TABLE IF EXISTS discovery_jobs CASCADE",
		"DROP TABLE IF EXISTS scheduled_jobs CASCADE",
		"DROP TABLE IF EXISTS scan_profiles CASCADE",
		"DROP TABLE IF EXISTS scan_targets CASCADE",
		"DROP TABLE IF EXISTS schema_migrations CASCADE",
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

	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, query := range dropQueries {
		_, err := tx.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to execute drop query: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reset: %w", err)
	}

	fmt.Println("All tables dropped successfully")

	// Re-run migrations
	return m.Up(ctx)
}

// ConnectAndMigrate is a convenience function to connect to database and run migrations.
func ConnectAndMigrate(ctx context.Context, config *Config) (*DB, error) {
	// Connect to database
	db, err := Connect(ctx, config)
	if err != nil {
		return nil, err
	}

	// Run migrations
	migrator := NewMigrator(db.DB)
	if err := migrator.Up(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return db, nil
}
