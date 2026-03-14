// Package services provides sqlmock-based unit tests for NetworkService DB methods.
// These tests exercise the SQL-dependent methods without requiring a live PostgreSQL
// instance by using github.com/DATA-DOG/go-sqlmock together with sqlx.
package services

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// newMockService creates a NetworkService backed by a go-sqlmock database.
// The returned mock allows callers to set expectations; the cleanup function
// closes the underlying connection.
func newMockService(t *testing.T) (*NetworkService, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err, "sqlmock.New should succeed")

	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	svc := NewNetworkService(wrappedDB)

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return svc, mock, cleanup
}

// networkColumns is the ordered list of columns returned by network SELECT queries.
var networkColumns = []string{
	"id", "name", "cidr", "description", "discovery_method",
	"is_active", "scan_enabled", "last_discovery", "last_scan",
	"host_count", "active_host_count", "created_at", "updated_at", "created_by",
}

// =============================================================================
// DeleteNetwork
// =============================================================================

func TestNetworkService_DeleteNetwork_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectExec(`DELETE FROM networks WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := svc.DeleteNetwork(context.Background(), id)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_DeleteNetwork_NotFound(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectExec(`DELETE FROM networks WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 0)) // zero rows affected → not found

	err := svc.DeleteNetwork(context.Background(), id)
	assert.Error(t, err, "should error when no row is deleted")
	assert.Contains(t, err.Error(), id.String())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_DeleteNetwork_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectExec(`DELETE FROM networks WHERE id = \$1`).
		WithArgs(id).
		WillReturnError(sql.ErrConnDone)

	err := svc.DeleteNetwork(context.Background(), id)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// CreateNetwork — validation-only paths (no DB call needed)
// =============================================================================

func TestNetworkService_CreateNetwork_InvalidCIDR(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	_, err := svc.CreateNetwork(
		context.Background(),
		"net-1", "not-a-cidr", "desc", "ping", true, true,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CIDR")
	// No DB expectations should have been set or triggered.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_CreateNetwork_InvalidMethod(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	_, err := svc.CreateNetwork(
		context.Background(),
		"net-1", "10.0.0.0/8", "desc", "invalid-method", true, true,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid discovery method")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_CreateNetwork_ValidMethodsAccepted(t *testing.T) {
	validMethods := []string{"ping", "tcp", "arp", "icmp"}

	for _, method := range validMethods {
		t.Run(method, func(t *testing.T) {
			svc, mock, cleanup := newMockService(t)
			defer cleanup()

			id := uuid.New()
			now := time.Now().UTC()

			// Expect the INSERT … RETURNING query.
			mock.ExpectQuery(`INSERT INTO networks`).
				WithArgs(
					"net-1",
					"10.0.0.0/8",
					"desc",
					method,
					true,
					true,
				).
				WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
					id, "net-1", "10.0.0.0/8", "desc", method,
					true, true, nil, nil, 0, 0, now, now, nil,
				))

			network, err := svc.CreateNetwork(
				context.Background(),
				"net-1", "10.0.0.0/8", "desc", method, true, true,
			)

			assert.NoError(t, err)
			assert.NotNil(t, network)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// =============================================================================
// UpdateNetwork — validation-only paths
// =============================================================================

func TestNetworkService_UpdateNetwork_InvalidCIDR(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	_, err := svc.UpdateNetwork(
		context.Background(),
		uuid.New(), "net-1", "bad-cidr", "desc", "ping", true,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CIDR")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_UpdateNetwork_InvalidMethod(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	_, err := svc.UpdateNetwork(
		context.Background(),
		uuid.New(), "net-1", "192.168.0.0/24", "desc", "nmap", true,
	)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid discovery method")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GetNetworkByID — not-found path
// =============================================================================

func TestNetworkService_GetNetworkByID_NotFound(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	// sqlx.GetContext executes a SELECT and expects exactly one row.
	// Returning ErrNoRows causes GetContext to surface sql.ErrNoRows which the
	// service translates to a "not found" error.
	mock.ExpectQuery(`SELECT`).
		WithArgs(id).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.GetNetworkByID(context.Background(), id)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), id.String())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkByID_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(id).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkByID(context.Background(), id)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// normalizeCIDR — comprehensive edge cases beyond what networks_test.go has
// =============================================================================

func TestNormalizeCIDR_AdditionalCases(t *testing.T) {
	svc := &NetworkService{}

	tests := []struct {
		name        string
		input       string
		wantOutput  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "loopback IP becomes /32",
			input:      "127.0.0.1",
			wantOutput: "127.0.0.1/32",
		},
		{
			name:       "zero CIDR prefix",
			input:      "0.0.0.0/0",
			wantOutput: "0.0.0.0/0",
		},
		{
			name:       "host CIDR /32 remains unchanged",
			input:      "10.0.0.1/32",
			wantOutput: "10.0.0.1/32",
		},
		{
			name:       "IPv6 loopback becomes /128",
			input:      "::1",
			wantOutput: "::1/128",
		},
		{
			name:       "IPv6 CIDR /128 unchanged",
			input:      "::1/128",
			wantOutput: "::1/128",
		},
		{
			name:        "hostname is rejected",
			input:       "example.com",
			wantErr:     true,
			errContains: "invalid CIDR or IP address",
		},
		{
			name:       "CIDR with host bits set is still valid CIDR",
			input:      "192.168.1.5/24",
			wantOutput: "192.168.1.5/24", // Go's net.ParseCIDR accepts this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.normalizeCIDR(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOutput, got)
			}
		})
	}
}

// =============================================================================
// AddExclusion — normalizeCIDR validation path (no DB call on bad CIDR)
// =============================================================================

func TestNetworkService_AddExclusion_InvalidCIDR(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	_, err := svc.AddExclusion(context.Background(), &netID, "not-valid", "reason")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CIDR or IP address")
	// No DB call should have been made.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_AddExclusion_ValidIPNoDBCallOnInsertError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectQuery(`INSERT INTO network_exclusions`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.AddExclusion(context.Background(), &netID, "192.168.1.0/24", "test reason")

	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// RemoveExclusion
// =============================================================================

func TestNetworkService_RemoveExclusion_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	exclusionID := uuid.New()

	mock.ExpectExec(`DELETE FROM network_exclusions WHERE id = \$1`).
		WithArgs(exclusionID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := svc.RemoveExclusion(context.Background(), exclusionID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_RemoveExclusion_NotFound(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	exclusionID := uuid.New()

	mock.ExpectExec(`DELETE FROM network_exclusions WHERE id = \$1`).
		WithArgs(exclusionID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := svc.RemoveExclusion(context.Background(), exclusionID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_RemoveExclusion_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	exclusionID := uuid.New()

	mock.ExpectExec(`DELETE FROM network_exclusions WHERE id = \$1`).
		WithArgs(exclusionID).
		WillReturnError(sql.ErrConnDone)

	err := svc.RemoveExclusion(context.Background(), exclusionID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
