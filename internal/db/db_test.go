package db

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/errors"
	"github.com/lib/pq"
)

// TestSanitizeDBError tests the error sanitization function without a live database.
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

// TestConnectionErrorSanitization tests that connection errors don't leak credentials.
// This test does not need a live database — it deliberately uses bad configs and
// asserts only on the shape of the returned error.
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
			ctx := t.Context()

			_, err := Connect(ctx, &tt.config)

			if err == nil {
				t.Skip("Expected connection error but succeeded — a local database may be running on this port")
			}

			errMsg := err.Error()

			if !strings.Contains(errMsg, tt.wantErrMsg) {
				t.Errorf("Expected error to contain %q, got: %s", tt.wantErrMsg, errMsg)
			}

			for _, forbidden := range tt.noContain {
				if strings.Contains(errMsg, forbidden) {
					t.Errorf("Error message contains sensitive data %q: %s", forbidden, errMsg)
				}
			}
		})
	}
}
