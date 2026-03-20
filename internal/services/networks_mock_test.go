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

// =============================================================================
// GetNetworkByName
// =============================================================================

func TestNetworkService_GetNetworkByName_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WithArgs("corp-lan").
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			id, "corp-lan", "10.0.0.0/8", nil, "ping",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	// getNetworkExclusions for this network (empty result)
	mock.ExpectQuery(`SELECT`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	result, err := svc.GetNetworkByName(context.Background(), "corp-lan")
	require.NoError(t, err)
	assert.Equal(t, "corp-lan", result.Network.Name)
	assert.Empty(t, result.Exclusions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkByName_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WithArgs("missing").
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkByName(context.Background(), "missing")
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkByName_WithExclusions(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	exID := uuid.New()
	now := time.Now().UTC()
	reason := "reserved"

	mock.ExpectQuery(`SELECT`).
		WithArgs("corp-lan").
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			netID, "corp-lan", "10.0.0.0/8", nil, "ping",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns).AddRow(
			exID, netID, "10.255.0.0/16", &reason, true, now, now, nil,
		))

	result, err := svc.GetNetworkByName(context.Background(), "corp-lan")
	require.NoError(t, err)
	require.Len(t, result.Exclusions, 1)
	assert.Equal(t, "10.255.0.0/16", result.Exclusions[0].ExcludedCIDR)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GetActiveNetworks
// =============================================================================

func TestNetworkService_GetActiveNetworks_Empty(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns))

	results, err := svc.GetActiveNetworks(context.Background())
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetActiveNetworks_MultipleNetworks(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns).
			AddRow(id1, "net-a", "10.0.0.0/8", nil, "ping", true, true, nil, nil, 0, 0, now, now, nil).
			AddRow(id2, "net-b", "192.168.0.0/16", nil, "arp", true, false, nil, nil, 0, 0, now, now, nil),
		)

	// getNetworkExclusions for net-a
	mock.ExpectQuery(`SELECT`).
		WithArgs(id1).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	// getNetworkExclusions for net-b
	mock.ExpectQuery(`SELECT`).
		WithArgs(id2).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	results, err := svc.GetActiveNetworks(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "net-a", results[0].Network.Name)
	assert.Equal(t, "net-b", results[1].Network.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetActiveNetworks_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetActiveNetworks(context.Background())
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// getNetworkExclusions failure is non-fatal; the network is still returned with empty exclusions.
func TestNetworkService_GetActiveNetworks_ExclusionFetchFailureContinues(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id1 := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns).
			AddRow(id1, "net-a", "10.0.0.0/8", nil, "ping", true, true, nil, nil, 0, 0, now, now, nil),
		)

	mock.ExpectQuery(`SELECT`).
		WithArgs(id1).
		WillReturnError(sql.ErrConnDone)

	results, err := svc.GetActiveNetworks(context.Background())
	require.NoError(t, err, "GetActiveNetworks should not propagate exclusion fetch errors")
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Exclusions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GetGlobalExclusions
// =============================================================================

func TestNetworkService_GetGlobalExclusions_Empty(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	exclusions, err := svc.GetGlobalExclusions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, exclusions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetGlobalExclusions_WithRows(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	exID := uuid.New()
	now := time.Now().UTC()
	reason := "test range"

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(exclusionColumns).AddRow(
			exID, nil, "10.255.255.0/24", &reason, true, now, now, nil,
		))

	exclusions, err := svc.GetGlobalExclusions(context.Background())
	require.NoError(t, err)
	require.Len(t, exclusions, 1)
	assert.Equal(t, "10.255.255.0/24", exclusions[0].ExcludedCIDR)
	assert.Nil(t, exclusions[0].NetworkID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetGlobalExclusions_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetGlobalExclusions(context.Background())
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GetNetworkExclusions  (delegates to getNetworkExclusions with non-nil ID)
// =============================================================================

func TestNetworkService_GetNetworkExclusions_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	exID := uuid.New()
	now := time.Now().UTC()
	reason := "link-local"

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns).AddRow(
			exID, netID, "169.254.0.0/16", &reason, true, now, now, nil,
		))

	exclusions, err := svc.GetNetworkExclusions(context.Background(), netID)
	require.NoError(t, err)
	require.Len(t, exclusions, 1)
	assert.Equal(t, "169.254.0.0/16", exclusions[0].ExcludedCIDR)
	assert.Equal(t, netID, *exclusions[0].NetworkID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkExclusions_Empty(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	exclusions, err := svc.GetNetworkExclusions(context.Background(), netID)
	require.NoError(t, err)
	assert.Empty(t, exclusions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkExclusions_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkExclusions(context.Background(), netID)
	assert.Error(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// UpdateNetworkDiscoveryTime
// =============================================================================

func TestNetworkService_UpdateNetworkDiscoveryTime_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectExec(`UPDATE networks`).
		WithArgs(netID, 42, 17).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := svc.UpdateNetworkDiscoveryTime(context.Background(), netID, 42, 17)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_UpdateNetworkDiscoveryTime_ZeroCounts(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectExec(`UPDATE networks`).
		WithArgs(netID, 0, 0).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := svc.UpdateNetworkDiscoveryTime(context.Background(), netID, 0, 0)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_UpdateNetworkDiscoveryTime_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectExec(`UPDATE networks`).
		WithArgs(netID, 10, 5).
		WillReturnError(sql.ErrConnDone)

	err := svc.UpdateNetworkDiscoveryTime(context.Background(), netID, 10, 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update network discovery time")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GetNetworkStats
// =============================================================================

var networkStatsColumns = []string{
	"total_networks", "active_networks", "scan_enabled_networks",
}

var exclusionStatsColumns = []string{
	"total_exclusions", "global_exclusions", "network_exclusions",
}

func TestNetworkService_GetNetworkStats_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkStatsColumns).AddRow(5, 3, 2))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1024))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(512))

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(exclusionStatsColumns).AddRow(10, 4, 6))

	stats, err := svc.GetNetworkStats(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stats)

	networks := stats["networks"].(map[string]interface{})
	assert.Equal(t, 5, networks["total"])
	assert.Equal(t, 3, networks["active"])
	assert.Equal(t, 2, networks["scan_enabled"])

	hosts := stats["hosts"].(map[string]interface{})
	assert.Equal(t, 1024, hosts["total"])
	assert.Equal(t, 512, hosts["active"])

	exclusions := stats["exclusions"].(map[string]interface{})
	assert.Equal(t, 10, exclusions["total"])
	assert.Equal(t, 4, exclusions["global"])
	assert.Equal(t, 6, exclusions["network"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkStats_NetworkQueryError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkStats(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkStats_TotalHostQueryError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkStatsColumns).AddRow(1, 1, 1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkStats(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get total host count")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkStats_ActiveHostQueryError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkStatsColumns).AddRow(1, 1, 1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts WHERE`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkStats(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get active host count")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkStats_ExclusionQueryError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkStatsColumns).AddRow(1, 1, 1))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

	mock.ExpectQuery(`SELECT`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GetNetworkStats(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get exclusion stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GetNetworkStats_ZeroValues(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkStatsColumns).AddRow(0, 0, 0))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM hosts WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(exclusionStatsColumns).AddRow(0, 0, 0))

	stats, err := svc.GetNetworkStats(context.Background())
	require.NoError(t, err)

	networks := stats["networks"].(map[string]interface{})
	assert.Equal(t, 0, networks["total"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// ListNetworks
// =============================================================================

func TestNetworkService_ListNetworks_All(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id1, id2 := uuid.New(), uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns).
			AddRow(id1, "alpha", "10.0.0.0/8", nil, "ping", true, true, nil, nil, 0, 0, now, now, nil).
			AddRow(id2, "beta", "192.168.0.0/16", nil, "arp", false, false, nil, nil, 0, 0, now, now, nil),
		)

	networks, err := svc.ListNetworks(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, networks, 2)
	assert.Equal(t, "alpha", networks[0].Name)
	assert.Equal(t, "beta", networks[1].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_ListNetworks_ActiveOnly(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id1 := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns).
			AddRow(id1, "alpha", "10.0.0.0/8", nil, "ping", true, true, nil, nil, 5, 3, now, now, nil),
		)

	networks, err := svc.ListNetworks(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, networks, 1)
	assert.True(t, networks[0].IsActive)
	assert.Equal(t, 5, networks[0].HostCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_ListNetworks_Empty(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(networkColumns))

	networks, err := svc.ListNetworks(context.Background(), false)
	require.NoError(t, err)
	assert.Empty(t, networks)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_ListNetworks_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT`).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.ListNetworks(context.Background(), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list networks")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// UpdateNetwork — success and not-found / db-error cases
// =============================================================================

func TestNetworkService_UpdateNetwork_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`UPDATE networks`).
		WithArgs(id, "net-updated", "10.0.0.0/8", "new desc", "tcp", true).
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			id, "net-updated", "10.0.0.0/8", "new desc", "tcp",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	network, err := svc.UpdateNetwork(
		context.Background(),
		id, "net-updated", "10.0.0.0/8", "new desc", "tcp", true,
	)
	require.NoError(t, err)
	assert.Equal(t, "net-updated", network.Name)
	assert.Equal(t, id, network.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_UpdateNetwork_NotFound(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectQuery(`UPDATE networks`).
		WithArgs(id, "net-1", "10.0.0.0/8", "desc", "ping", true).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.UpdateNetwork(
		context.Background(),
		id, "net-1", "10.0.0.0/8", "desc", "ping", true,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), id.String())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_UpdateNetwork_DBError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	id := uuid.New()

	mock.ExpectQuery(`UPDATE networks`).
		WithArgs(id, "net-1", "10.0.0.0/8", "desc", "ping", true).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.UpdateNetwork(
		context.Background(),
		id, "net-1", "10.0.0.0/8", "desc", "ping", true,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update network")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// GenerateTargetsForNetwork
// =============================================================================

func TestNetworkService_GenerateTargetsForNetwork_Success(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	now := time.Now().UTC()

	// GetNetworkByID → GetContext for the network row
	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			netID, "corp", "10.0.0.0/8", nil, "ping",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	// GetNetworkByID → getNetworkExclusions
	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	// generate_host_ips_with_exclusions
	mock.ExpectQuery(`SELECT`).
		WithArgs("10.0.0.0/8", netID, 10).
		WillReturnRows(sqlmock.NewRows([]string{"ip_address"}).
			AddRow("10.0.0.1").
			AddRow("10.0.0.2"),
		)

	targets, err := svc.GenerateTargetsForNetwork(context.Background(), netID, 10)
	require.NoError(t, err)
	assert.Len(t, targets, 2)
	assert.Equal(t, "10.0.0.1", targets[0])
	assert.Equal(t, "10.0.0.2", targets[1])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GenerateTargetsForNetwork_GetNetworkError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.GenerateTargetsForNetwork(context.Background(), netID, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get network")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GenerateTargetsForNetwork_QueryError(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			netID, "corp", "10.0.0.0/8", nil, "ping",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	mock.ExpectQuery(`SELECT`).
		WithArgs("10.0.0.0/8", netID, 5).
		WillReturnError(sql.ErrConnDone)

	_, err := svc.GenerateTargetsForNetwork(context.Background(), netID, 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate targets")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNetworkService_GenerateTargetsForNetwork_EmptyResult(t *testing.T) {
	svc, mock, cleanup := newMockService(t)
	defer cleanup()

	netID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(networkColumns).AddRow(
			netID, "corp", "10.0.0.0/8", nil, "ping",
			true, true, nil, nil, 0, 0, now, now, nil,
		))

	mock.ExpectQuery(`SELECT`).
		WithArgs(netID).
		WillReturnRows(sqlmock.NewRows(exclusionColumns))

	mock.ExpectQuery(`SELECT`).
		WithArgs("10.0.0.0/8", netID, 100).
		WillReturnRows(sqlmock.NewRows([]string{"ip_address"}))

	targets, err := svc.GenerateTargetsForNetwork(context.Background(), netID, 100)
	require.NoError(t, err)
	assert.Empty(t, targets)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// exclusionColumns is the ordered list of columns returned by exclusion SELECT queries.
var exclusionColumns = []string{
	"id", "network_id", "excluded_cidr", "reason", "enabled",
	"created_at", "updated_at", "created_by",
}
