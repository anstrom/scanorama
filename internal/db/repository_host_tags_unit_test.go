// Package db — unit tests for HostRepository tag methods using sqlmock.
// These run without a live database and complement the integration tests in
// tags_groups_repository_test.go.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetAllTags ─────────────────────────────────────────────────────────────

func TestHostRepository_GetAllTags_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnRows(sqlmock.NewRows([]string{"tag"}))

	tags, err := NewHostRepository(db).GetAllTags(context.Background())

	require.NoError(t, err)
	assert.Empty(t, tags)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_GetAllTags_ReturnsSortedList(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnRows(sqlmock.NewRows([]string{"tag"}).
			AddRow("db").
			AddRow("prod").
			AddRow("web"))

	tags, err := NewHostRepository(db).GetAllTags(context.Background())

	require.NoError(t, err)
	require.Len(t, tags, 3)
	assert.Equal(t, "db", tags[0])
	assert.Equal(t, "prod", tags[1])
	assert.Equal(t, "web", tags[2])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_GetAllTags_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnError(fmt.Errorf("connection reset"))

	tags, err := NewHostRepository(db).GetAllTags(context.Background())

	require.Error(t, err)
	assert.Nil(t, tags)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateHostTags ─────────────────────────────────────────────────────────

func TestHostRepository_UpdateHostTags_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec("UPDATE hosts SET tags").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewHostRepository(db).UpdateHostTags(context.Background(), id, []string{"prod", "web"})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_UpdateHostTags_ClearsAllTags(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	// Replacing with empty slice should still execute — it clears all tags.
	mock.ExpectExec("UPDATE hosts SET tags").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewHostRepository(db).UpdateHostTags(context.Background(), id, []string{})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_UpdateHostTags_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts SET tags").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewHostRepository(db).UpdateHostTags(context.Background(), uuid.New(), []string{"prod"})

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_UpdateHostTags_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts SET tags").
		WillReturnError(fmt.Errorf("connection lost"))

	err := NewHostRepository(db).UpdateHostTags(context.Background(), uuid.New(), []string{"prod"})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── AddHostTags ────────────────────────────────────────────────────────────

func TestHostRepository_AddHostTags_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewHostRepository(db).AddHostTags(context.Background(), id, []string{"new-tag"})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_AddHostTags_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewHostRepository(db).AddHostTags(context.Background(), uuid.New(), []string{"tag"})

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_AddHostTags_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnError(fmt.Errorf("timeout"))

	err := NewHostRepository(db).AddHostTags(context.Background(), uuid.New(), []string{"tag"})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── RemoveHostTags ─────────────────────────────────────────────────────────

func TestHostRepository_RemoveHostTags_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewHostRepository(db).RemoveHostTags(context.Background(), id, []string{"old-tag"})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_RemoveHostTags_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewHostRepository(db).RemoveHostTags(context.Background(), uuid.New(), []string{"tag"})

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_RemoveHostTags_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnError(fmt.Errorf("connection reset"))

	err := NewHostRepository(db).RemoveHostTags(context.Background(), uuid.New(), []string{"tag"})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── BulkUpdateTags ─────────────────────────────────────────────────────────

func TestHostRepository_BulkUpdateTags_EmptyIDs_NoOp(t *testing.T) {
	db, mock := newMockDB(t)

	// No DB calls expected when ids slice is empty.
	err := NewHostRepository(db).BulkUpdateTags(context.Background(), nil, []string{"tag"}, "add")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_BulkUpdateTags_InvalidAction(t *testing.T) {
	db, mock := newMockDB(t)

	err := NewHostRepository(db).BulkUpdateTags(
		context.Background(),
		[]uuid.UUID{uuid.New()},
		[]string{"tag"},
		"nuke",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown bulk tag action")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_BulkUpdateTags_AddAction(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(2, 2))

	err := NewHostRepository(db).BulkUpdateTags(
		context.Background(),
		[]uuid.UUID{uuid.New(), uuid.New()},
		[]string{"new-tag"},
		"add",
	)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_BulkUpdateTags_RemoveAction(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(3, 3))

	err := NewHostRepository(db).BulkUpdateTags(
		context.Background(),
		[]uuid.UUID{uuid.New(), uuid.New(), uuid.New()},
		[]string{"old-tag"},
		"remove",
	)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_BulkUpdateTags_SetAction(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewHostRepository(db).BulkUpdateTags(
		context.Background(),
		[]uuid.UUID{uuid.New()},
		[]string{"only-this"},
		"set",
	)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_BulkUpdateTags_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE hosts").
		WillReturnError(fmt.Errorf("timeout"))

	err := NewHostRepository(db).BulkUpdateTags(
		context.Background(),
		[]uuid.UUID{uuid.New()},
		[]string{"tag"},
		"set",
	)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetHostGroups ──────────────────────────────────────────────────────────

func TestHostRepository_GetHostGroups_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT hg.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color"}))

	groups, err := NewHostRepository(db).GetHostGroups(context.Background(), uuid.New())

	require.NoError(t, err)
	// Must return a non-nil slice so callers can range without nil checks.
	assert.NotNil(t, groups)
	assert.Empty(t, groups)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_GetHostGroups_WithGroups(t *testing.T) {
	db, mock := newMockDB(t)
	g1, g2 := uuid.New(), uuid.New()

	mock.ExpectQuery("SELECT hg.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color"}).
			AddRow(g1, "infra", "#10b981").
			AddRow(g2, "prod", ""))

	groups, err := NewHostRepository(db).GetHostGroups(context.Background(), uuid.New())

	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, g1, groups[0].ID)
	assert.Equal(t, "infra", groups[0].Name)
	assert.Equal(t, "#10b981", groups[0].Color)
	assert.Equal(t, g2, groups[1].ID)
	assert.Equal(t, "prod", groups[1].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_GetHostGroups_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT hg.id").
		WillReturnError(fmt.Errorf("connection lost"))

	groups, err := NewHostRepository(db).GetHostGroups(context.Background(), uuid.New())

	require.Error(t, err)
	assert.Nil(t, groups)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetHost tags integration (unit) ───────────────────────────────────────
// Verifies that GetHost correctly populates host.Tags from the tags column.

func TestHostRepository_GetHost_PopulatesTags(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	// Main host SELECT — includes the tags column and knowledge_score column.
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(getHostColumns).
			AddRow(
				id, "10.0.0.5",
				nil, nil, nil, // hostname, mac, vendor
				nil, nil, nil, // os_family, os_name, os_version
				nil, nil, nil, // os_confidence, os_detected_at, os_method
				nil,                // os_details
				nil,                // discovery_method
				nil, nil, nil, nil, // response_time fields
				false,          // ignore_scanning
				now, now, "up", // first_seen, last_seen, status
				nil, nil, 0, // status_changed_at, previous_status, timeout_count
				pq.StringArray{"prod", "web"}, // tags
				0,                             // knowledge_score
			))

	// fetchHostPorts — no ports.
	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnRows(sqlmock.NewRows([]string{"port", "protocol", "state", "service_name", "scanned_at"}))

	// fetchHostScanCount.
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// GetHostGroups — no groups.
	mock.ExpectQuery("SELECT hg.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color"}))

	host, err := NewHostRepository(db).GetHost(context.Background(), id)

	require.NoError(t, err)
	require.NotNil(t, host)
	require.Len(t, host.Tags, 2)
	assert.Equal(t, "prod", host.Tags[0])
	assert.Equal(t, "web", host.Tags[1])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestHostRepository_GetHost_EmptyTags(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(getHostColumns).
			AddRow(
				id, "10.0.0.6",
				nil, nil, nil,
				nil, nil, nil,
				nil, nil, nil,
				nil, nil,
				nil, nil, nil, nil,
				false,
				now, now, "up",
				nil, nil, 0,
				pq.StringArray{},
				0, // knowledge_score
			))

	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnRows(sqlmock.NewRows([]string{"port", "protocol", "state", "service_name", "scanned_at"}))

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery("SELECT hg.id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "color"}))

	host, err := NewHostRepository(db).GetHost(context.Background(), id)

	require.NoError(t, err)
	// An empty pq.StringArray scans to an empty (non-nil) slice.
	assert.NotNil(t, host.Tags)
	assert.Empty(t, host.Tags)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListHosts tags (unit) ──────────────────────────────────────────────────
// Verifies that scanHostRows correctly populates host.Tags.

func TestHostRepository_ScanHostRows_PopulatesTags(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	// The listHosts query selects tags as one of the columns before the
	// aggregate open_ports / total_ports_scanned / scan_count columns.
	listCols := append(append([]string{}, getHostColumns...), "open_ports", "total_ports_scanned", "scan_count")

	// getHostCount runs first in ListHosts.
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(listCols).
			AddRow(
				id, "10.0.0.7",
				nil, nil, nil,
				nil, nil, nil,
				nil, nil, nil,
				nil, nil,
				nil, nil, nil, nil,
				false,
				now, now, "up",
				nil, nil, 0,
				pq.StringArray{"staging"},
				0,                                    // knowledge_score
				sql.NullInt64{Int64: 2, Valid: true}, // open_ports
				int64(3),                             // total_ports_scanned
				sql.NullInt64{Int64: 1, Valid: true}, // scan_count
			))

	filters := &HostFilters{}
	hosts, total, err := NewHostRepository(db).ListHosts(context.Background(), filters, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, hosts, 1)
	require.Len(t, hosts[0].Tags, 1)
	assert.Equal(t, "staging", hosts[0].Tags[0])
	require.NoError(t, mock.ExpectationsWereMet())
}
