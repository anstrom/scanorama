package services

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

func newResolverMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

func resolverHost(osFamily *string) *db.Host {
	return &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.1")},
		OSFamily:  osFamily,
		LastSeen:  time.Now(),
	}
}

func osPtr(s string) *string { return &s }

func TestPortListResolver_UsesSettingsBaseWhenPresent(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).
			AddRow(`"22,443,8080"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).
			AddRow("256"))
	// No OS family — no port_definitions query
	// Fleet — empty
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, "22,443,8080", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackToHardcodedDefaultWhenSettingMissing(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, "22,80,135,443,445,3389", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_ReturnsRangeBaseUnchanged(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.refresh.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).
			AddRow(`"1-1024"`))
	// Range base → readLimit and OS/fleet queries are all skipped

	result := r.Resolve(context.Background(), "refresh", resolverHost(nil))
	assert.Equal(t, "1-1024", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_MergesOSPortsWhenOSFamilyKnown(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())
	host := resolverHost(osPtr("linux"))

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(111).AddRow(2049))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result := r.Resolve(context.Background(), "os_detection", host)
	assert.Equal(t, "22,111,2049", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_MergesFleetTopPorts(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.identity_enrichment.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	// No OS family
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80).AddRow(443))

	result := r.Resolve(context.Background(), "identity_enrichment", resolverHost(nil))
	assert.Equal(t, "22,80,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_DeduplicatesOverlappingPorts(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())
	host := resolverHost(osPtr("linux"))

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22,80"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80).AddRow(443))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(22))

	result := r.Resolve(context.Background(), "os_detection", host)
	assert.Equal(t, "22,80,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_AugmentationCappedAtLimit(t *testing.T) {
	// Base has 2 ports; limit is 3; fleet offers 3 candidates.
	// Only 1 augmentation slot remains after base is reserved.
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22,443"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("3"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80).AddRow(8080).AddRow(8443))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	// base [22, 443] always kept; augmentation fills 1 remaining slot (80 is first)
	assert.Equal(t, "22,80,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_BasePortsPreservedBeyondLimit(t *testing.T) {
	// Base has 6 ports (> limit=3). All base ports are always kept regardless.
	// This verifies that critical high-numbered ports like 3389 survive the cap.
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22,80,443,3389,8080,8443"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("3"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(9000))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	// All 6 base ports kept even though limit=3; fleet not added (base exceeds limit)
	assert.Equal(t, "22,80,443,3389,8080,8443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_ZeroLimitSettingPromotedToDefault(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("0"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, "22,80", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── Error-path tests ──────────────────────────────────────────────────────────

func TestPortListResolver_FallsBackOnBasePortsDBError(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnError(fmt.Errorf("conn error"))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, osDetectionPorts, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackOnUnknownStage(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	// Unknown stage → stageDefaultPorts fallback = "1-1024" (a range).
	// readBasePorts returns "1-1024" after ErrNoRows from DB.
	// Resolve detects the range and returns immediately — no further queries.
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.unknown_stage.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	result := r.Resolve(context.Background(), "unknown_stage", resolverHost(nil))
	assert.Equal(t, "1-1024", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackOnLimitDBError(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	// limit falls back to 256; both ports fit
	assert.Equal(t, "22,80", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackOnNonIntegerLimit(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"not-a-number"`))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, "22,80", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackOnNegativeLimit(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("-5"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	assert.Equal(t, "22,80", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_SkipsOSPortsOnQueryError(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())
	host := resolverHost(osPtr("linux"))

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnError(fmt.Errorf("db error"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(443))

	result := r.Resolve(context.Background(), "os_detection", host)
	// OS source entirely skipped; fleet provides 443
	assert.Equal(t, "22,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_SkipsOSPortsOnIterationError(t *testing.T) {
	// RowError(1, err): first row (111) scans successfully, second Next() call
	// returns the error → rows.Err() fires → accumulated ports discarded.
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())
	host := resolverHost(osPtr("linux"))

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).
			AddRow(111).AddRow(999).RowError(1, fmt.Errorf("cursor error")))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(443))

	result := r.Resolve(context.Background(), "os_detection", host)
	// port 111 was accumulated but rows.Err() discards the whole source — only fleet's 443 added
	assert.Equal(t, "22,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_SkipsFleetPortsOnQueryError(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnError(fmt.Errorf("db error"))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	// Fleet skipped entirely; only base port remains
	assert.Equal(t, "22", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_SkipsFleetPortsOnIterationError(t *testing.T) {
	// RowError(1, err): port 443 scans successfully, second Next() fires the error
	// → rows.Err() → accumulated ports discarded.
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database, discardLogger())

	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`"22"`))
	mock.ExpectQuery(`SELECT value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("256"))
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(443).AddRow(80).RowError(1, fmt.Errorf("cursor error")))

	result := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	// port 443 was accumulated but rows.Err() discards the whole fleet source
	assert.Equal(t, "22", result)
	require.NoError(t, mock.ExpectationsWereMet())
}
