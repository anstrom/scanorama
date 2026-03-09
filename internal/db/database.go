// Package db provides database connectivity and data models for scanorama.
// It handles database migrations, host management, scan results storage,
// and provides the core data access layer for the application.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// sanitizeDBError converts raw database errors into safe, sanitized errors
// that don't expose internal SQL details or credentials to API clients.
// The original error is preserved in the Cause field for internal debugging.
func sanitizeDBError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Handle specific known database errors.
	if err == sql.ErrNoRows {
		dbErr := errors.NewDatabaseError(errors.CodeNotFound, "Resource not found")
		dbErr.Operation = operation
		dbErr.Cause = err
		return dbErr
	}

	// Check for PostgreSQL-specific errors.
	if pqErr, ok := err.(*pq.Error); ok {
		var dbErr *errors.DatabaseError
		switch pqErr.Code {
		case "23505": // unique_violation
			dbErr = errors.NewDatabaseError(errors.CodeConflict, "Resource already exists")
		case "23503": // foreign_key_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Referenced resource does not exist")
		case "23502": // not_null_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Required field is missing")
		case "23514": // check_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Data validation failed")
		case "57014": // query_canceled
			dbErr = errors.NewDatabaseError(errors.CodeCanceled, "Database operation was canceled")
		case "57P01": // admin_shutdown
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseConnection, "Database connection lost")
		case "08000", "08003", "08006": // connection errors
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseConnection, "Database connection error")
		default:
			// Unknown PostgreSQL error - use generic sanitized error.
			msg := fmt.Sprintf("Database operation failed: %s", operation)
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseQuery, msg)
		}
		// Preserve original error for internal logging.
		dbErr.Operation = operation
		dbErr.Cause = err
		return dbErr
	}

	// For all other errors, return a generic sanitized error without details.
	dbErr := errors.NewDatabaseError(errors.CodeDatabaseQuery, fmt.Sprintf("Database operation failed: %s", operation))
	dbErr.Operation = operation
	// Store the original error as Cause for internal logging, but it won't be exposed to API.
	dbErr.Cause = err
	return dbErr
}

const (
	// Default database configuration values.
	defaultPostgresPort    = 5432
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 5
	defaultConnMaxIdleTime = 5
)

// DB wraps sqlx.DB with additional functionality.
type DB struct {
	*sqlx.DB
}

// Config holds database configuration.
type Config struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Database        string        `yaml:"database" json:"database"`
	Username        string        `yaml:"username" json:"username"`
	Password        string        `yaml:"password" json:"password"` //nolint:gosec // G117: config field
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
}

// DefaultConfig returns the default database configuration.
// Database name, username, and password must be explicitly configured.
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            defaultPostgresPort,
		Database:        "", // Must be configured.
		Username:        "", // Must be configured.
		Password:        "", // Must be configured.
		SSLMode:         "disable",
		MaxOpenConns:    defaultMaxOpenConns,
		MaxIdleConns:    defaultMaxIdleConns,
		ConnMaxLifetime: defaultConnMaxLifetime * time.Minute,
		ConnMaxIdleTime: defaultConnMaxIdleTime * time.Minute,
	}
}

// Connect establishes a connection to PostgreSQL.
// Returns sanitized errors that don't leak credentials or DSN details.
func Connect(ctx context.Context, config *Config) (*DB, error) {
	// Build DSN - PostgreSQL lib/pq handles special characters in values correctly
	// when using key=value format (values with spaces/special chars are auto-escaped).
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.Database,
		config.Username, config.Password, config.SSLMode,
	)

	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		// Return sanitized error without DSN to prevent credential leakage in logs.
		return nil, errors.ErrDatabaseConnection(err)
	}

	// Configure connection pool.
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection.
	if err := db.PingContext(ctx); err != nil {
		// Close the connection before returning error.
		if closeErr := db.Close(); closeErr != nil {
			// Don't log raw error - it might contain connection details.
			log.Printf("Failed to close database connection after ping failure")
		}
		return nil, errors.WrapDatabaseError(errors.CodeDatabaseConnection, "Failed to verify database connection", err)
	}

	// Log success without credentials - only safe connection details.
	log.Printf("Successfully connected to database at %s:%d/%s", config.Host, config.Port, config.Database)
	return &DB{DB: db}, nil
}

// Repository provides database operations.
type Repository struct {
	db *DB
}

// NewRepository creates a new repository instance.
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// filterCondition represents a single filter condition.
type filterCondition struct {
	column string
	value  interface{}
}

// buildWhereClause creates a WHERE clause and args from a slice of conditions.
func buildWhereClause(conditions []filterCondition) (whereClause string, args []interface{}) {
	if len(conditions) == 0 {
		return "", nil
	}

	clauses := make([]string, 0, len(conditions))
	for i, condition := range conditions {
		clauses = append(clauses, fmt.Sprintf("%s = $%d", condition.column, i+1))
		args = append(args, condition.value)
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

// buildUpdateQuery creates SQL SET clause and args from field mappings.
func buildUpdateQuery(data map[string]interface{}, fieldMappings map[string]string) (
	setParts []string, args []interface{}) {
	argIndex := 1

	for requestField, dbField := range fieldMappings {
		if value, exists := data[requestField]; exists && value != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", dbField, argIndex))
			args = append(args, value)
			argIndex++
		}
	}

	return setParts, args
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Ping tests the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.PingContext(ctx)
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return db.BeginTxx(ctx, nil)
}
