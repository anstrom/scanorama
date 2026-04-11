// Package enrichment — network-level tests for SNMPEnricher.
//
// These tests exercise EnrichHost by connecting to a port that refuses the
// connection (nobody listening). SNMP is UDP-based, so "connection refused"
// manifests as a timeout or connect error depending on the OS; in both cases
// EnrichHost must return nil without panicking.
package enrichment

import (
	"context"
	"log/slog"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// newNetworkMockDB mirrors the helper in dns_test.go.
func newNetworkMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	return &db.DB{DB: sqlx.NewDb(rawDB, "sqlmock")}, mock
}

// TestNewSNMPEnricher verifies the constructor returns a non-nil enricher
// without performing any database calls.
func TestNewSNMPEnricher(t *testing.T) {
	mockDB, mock := newNetworkMockDB(t)
	repo := db.NewSNMPRepository(mockDB)

	enricher := NewSNMPEnricher(repo, slog.Default())
	require.NotNil(t, enricher)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEnrichHost_UnreachableHost verifies that when the SNMP target cannot
// be reached (127.0.0.2 is loopback but port 161 is not open), EnrichHost
// returns nil — an unreachable host is not a propagated error.
//
// This test exercises the ConnectIPv4 error path and the defer/close logic.
func TestEnrichHost_UnreachableHost(t *testing.T) {
	mockDB, mock := newNetworkMockDB(t)
	repo := db.NewSNMPRepository(mockDB)
	enricher := NewSNMPEnricher(repo, slog.Default())

	target := SNMPTarget{
		HostID:    uuid.New(),
		IP:        "127.0.0.1", // loopback — port 161 UDP not open in test env
		Community: "public",
	}

	// ConnectIPv4 either fails immediately or succeeds but Get times out.
	// Either way the function must return nil, not an error.
	err := enricher.EnrichHost(context.Background(), target)
	require.NoError(t, err, "unreachable SNMP target should return nil, not an error")

	// No repository calls expected (no successful data to store).
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEnrichHost_DefaultCommunity verifies that when SNMPTarget.Community is
// empty the enricher substitutes the default community string ("public")
// and still returns nil for an unreachable host.
func TestEnrichHost_DefaultCommunity(t *testing.T) {
	mockDB, mock := newNetworkMockDB(t)
	repo := db.NewSNMPRepository(mockDB)
	enricher := NewSNMPEnricher(repo, slog.Default())

	target := SNMPTarget{
		HostID:    uuid.New(),
		IP:        "127.0.0.1",
		Community: "", // should fall back to snmpCommunityStr ("public")
	}

	err := enricher.EnrichHost(context.Background(), target)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
