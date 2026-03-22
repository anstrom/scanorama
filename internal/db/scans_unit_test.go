package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// scanColumns are the columns returned by the ListScans / processScanRow query.
var scanColumns = []string{
	"id", "name", "description", "targets", "scan_type", "ports",
	"profile_id", "status", "created_at", "started_at", "completed_at",
	"error_message",
}

// getScanColumns are the columns returned by the GetScan query (one extra: os_detection).
var getScanColumns = append(scanColumns, "os_detection")

// driverValues converts []interface{} to []driver.Value so it can be spread
// into sqlmock's AddRow(...driver.Value) variadic parameter.
func driverValues(vals []interface{}) []driver.Value {
	out := make([]driver.Value, len(vals))
	for i, v := range vals {
		out[i] = v
	}
	return out
}

func scanRow(id uuid.UUID, name, cidr, scanType, ports, status string, now time.Time) []driver.Value {
	return driverValues([]interface{}{
		id,
		name,
		sql.NullString{Valid: false},
		cidr,
		scanType,
		ports,
		nil, // profile_id
		status,
		now,
		nil, // started_at
		nil, // completed_at
		nil, // error_message
	})
}

// ── buildScanResponse ─────────────────────────────────────────────────────────

func TestBuildScanResponse(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()
	profile := "linux-server"

	t.Run("all fields set", func(t *testing.T) {
		scan := buildScanResponse(id, "My Scan", "a description",
			[]string{"10.0.0.0/8"}, "connect", "22,80", &profile, now)

		assert.Equal(t, id, scan.ID)
		assert.Equal(t, "My Scan", scan.Name)
		assert.Equal(t, "a description", scan.Description)
		assert.Equal(t, []string{"10.0.0.0/8"}, scan.Targets)
		assert.Equal(t, "connect", scan.ScanType)
		assert.Equal(t, "22,80", scan.Ports)
		assert.Equal(t, "pending", scan.Status)
		assert.Equal(t, now, scan.CreatedAt)
		assert.Equal(t, now, scan.UpdatedAt)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "linux-server", *scan.ProfileID)
	})

	t.Run("nil profile leaves ProfileID nil", func(t *testing.T) {
		scan := buildScanResponse(id, "No Profile", "", []string{"192.168.1.0/24"},
			"syn", "", nil, now)
		assert.Nil(t, scan.ProfileID)
	})

	t.Run("empty ports leaves PortsScanned nil", func(t *testing.T) {
		scan := buildScanResponse(id, "Scan", "", []string{"1.2.3.4/32"},
			"connect", "", nil, now)
		// buildScanResponse does not set PortsScanned; that is done by GetScan.
		assert.Nil(t, scan.PortsScanned)
	})
}

// ── getScanCount ──────────────────────────────────────────────────────────────

func TestGetScanCount(t *testing.T) {
	t.Run("returns count on success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		count, err := db.getScanCount(context.Background(), "", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("propagates db error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("db unavailable"))

		_, err := db.getScanCount(context.Background(), "", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan count")
	})

	t.Run("where clause is appended", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT.*WHERE sj\.status`).
			WithArgs("running").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		count, err := db.getScanCount(context.Background(), "WHERE sj.status = $1", []interface{}{"running"})
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})
}

// ── processScanRow ────────────────────────────────────────────────────────────

func TestProcessScanRow(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("minimal row — no timestamps, no description", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(scanColumns).AddRow(scanRow(id, "Scan", "10.0.0.0/8", "connect", "", "pending", now)...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()

		require.True(t, rows.Next())
		scan, err := processScanRow(rows)
		require.NoError(t, err)

		assert.Equal(t, id, scan.ID)
		assert.Equal(t, "Scan", scan.Name)
		assert.Empty(t, scan.Description)
		assert.Equal(t, []string{"10.0.0.0/8"}, scan.Targets)
		assert.Equal(t, "connect", scan.ScanType)
		assert.Nil(t, scan.DurationStr)
		assert.Nil(t, scan.PortsScanned)
		assert.Equal(t, now, scan.UpdatedAt) // falls back to CreatedAt
	})

	t.Run("with description", func(t *testing.T) {
		db, mock := newMockDB(t)
		row := driverValues([]interface{}{
			id, "Scan", sql.NullString{String: "my desc", Valid: true},
			"192.168.1.0/24", "syn", "22", nil, "pending",
			now, nil, nil, nil,
		})
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(scanColumns).AddRow(row...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processScanRow(rows)
		require.NoError(t, err)
		assert.Equal(t, "my desc", scan.Description)
	})

	t.Run("with non-empty ports sets PortsScanned", func(t *testing.T) {
		db, mock := newMockDB(t)
		row := driverValues([]interface{}{
			id, "Scan", sql.NullString{Valid: false},
			"10.0.0.0/8", "connect", "22,80,443", nil, "pending",
			now, nil, nil, nil,
		})
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(scanColumns).AddRow(row...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processScanRow(rows)
		require.NoError(t, err)
		require.NotNil(t, scan.PortsScanned)
		assert.Equal(t, "22,80,443", *scan.PortsScanned)
	})

	t.Run("with both timestamps computes DurationStr and sets UpdatedAt", func(t *testing.T) {
		db, mock := newMockDB(t)
		started := now.Add(-90 * time.Second)
		completed := now
		row := driverValues([]interface{}{
			id, "Scan", sql.NullString{Valid: false},
			"10.0.0.0/8", "connect", "", nil, "completed",
			now, started, completed, nil,
		})
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(scanColumns).AddRow(row...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processScanRow(rows)
		require.NoError(t, err)
		require.NotNil(t, scan.DurationStr)
		assert.Contains(t, *scan.DurationStr, "1m30s")
		assert.Equal(t, completed, scan.UpdatedAt)
	})

	t.Run("only started_at set — UpdatedAt uses started_at", func(t *testing.T) {
		db, mock := newMockDB(t)
		started := now.Add(-10 * time.Second)
		row := driverValues([]interface{}{
			id, "Scan", sql.NullString{Valid: false},
			"10.0.0.0/8", "connect", "", nil, "running",
			now, started, nil, nil,
		})
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(scanColumns).AddRow(row...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processScanRow(rows)
		require.NoError(t, err)
		assert.Nil(t, scan.DurationStr)
		assert.Equal(t, started, scan.UpdatedAt)
	})

	t.Run("with profile_id", func(t *testing.T) {
		db, mock := newMockDB(t)
		profile := "windows-server"
		row := driverValues([]interface{}{
			id, "Scan", sql.NullString{Valid: false},
			"10.0.0.0/8", "connect", "", &profile, "pending",
			now, nil, nil, nil,
		})
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(scanColumns).AddRow(row...))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processScanRow(rows)
		require.NoError(t, err)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "windows-server", *scan.ProfileID)
	})
}

// ── ListScans ─────────────────────────────────────────────────────────────────

func TestListScans_Unit(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("count query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("count failed"))

		_, _, err := db.ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan count")
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query failed"))

		_, _, err := db.ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list scans")
	})

	t.Run("empty result set", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows(scanColumns))

		scans, total, err := db.ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, scans)
	})

	t.Run("returns rows correctly", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(scanColumns).AddRow(
				scanRow(id, "My Scan", "10.0.0.0/8", "connect", "22", "pending", now)...))

		scans, total, err := db.ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, scans, 1)
		assert.Equal(t, id, scans[0].ID)
		assert.Equal(t, "My Scan", scans[0].Name)
	})

	t.Run("filters are applied to both queries", func(t *testing.T) {
		db, mock := newMockDB(t)
		profileID := "linux-server"

		mock.ExpectQuery(`SELECT COUNT.*sj\.status`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT.*sj\.status`).
			WillReturnRows(sqlmock.NewRows(scanColumns))

		_, _, err := db.ListScans(context.Background(), ScanFilters{
			Status:    "completed",
			ProfileID: &profileID,
		}, 0, 10)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── GetScan ───────────────────────────────────────────────────────────────────

func TestGetScan_Unit(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

		_, err := db.GetScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("connection reset"))

		_, err := db.GetScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan")
	})

	t.Run("success with minimal fields", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Scan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", nil, "pending",
				now, nil, nil, nil, false,
			))

		scan, err := db.GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, id, scan.ID)
		assert.Equal(t, "Scan", scan.Name)
		assert.Empty(t, scan.Description)
		assert.Nil(t, scan.ProfileID)
		assert.Nil(t, scan.PortsScanned)
		assert.Nil(t, scan.DurationStr)
		assert.Equal(t, now, scan.UpdatedAt)
		assert.Equal(t, map[string]interface{}{"os_detection": false}, scan.Options)
	})

	t.Run("success with description and profile", func(t *testing.T) {
		db, mock := newMockDB(t)
		profile := "windows-server"
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Full Scan", sql.NullString{String: "full desc", Valid: true},
				"192.168.1.0/24", "version", "80,443",
				&profile, "completed",
				now, nil, nil, nil, true,
			))

		scan, err := db.GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "full desc", scan.Description)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "windows-server", *scan.ProfileID)
		require.NotNil(t, scan.PortsScanned)
		assert.Equal(t, "80,443", *scan.PortsScanned)
		assert.Equal(t, map[string]interface{}{"os_detection": true}, scan.Options)
	})

	t.Run("both timestamps set — computes DurationStr", func(t *testing.T) {
		db, mock := newMockDB(t)
		started := now.Add(-2 * time.Minute)
		completed := now
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Scan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", nil, "completed",
				now, started, completed, nil, false,
			))

		scan, err := db.GetScan(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, scan.DurationStr)
		assert.Contains(t, *scan.DurationStr, "2m0s")
		assert.Equal(t, completed, scan.UpdatedAt)
	})

	t.Run("only started_at — UpdatedAt is started_at", func(t *testing.T) {
		db, mock := newMockDB(t)
		started := now.Add(-30 * time.Second)
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Scan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", nil, "running",
				now, started, nil, nil, false,
			))

		scan, err := db.GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Nil(t, scan.DurationStr)
		assert.Equal(t, started, scan.UpdatedAt)
	})
}

// ── CreateScan ────────────────────────────────────────────────────────────────

func TestCreateScan_Unit(t *testing.T) {
	t.Run("empty name returns validation error", func(t *testing.T) {
		db, _ := newMockDB(t)
		_, err := db.CreateScan(context.Background(), CreateScanInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("begin transaction error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx unavailable"))

		_, err := db.CreateScan(context.Background(), CreateScanInput{
			Name: "Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("existing CIDR reuses network", func(t *testing.T) {
		db, mock := newMockDB(t)
		networkID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := db.CreateScan(context.Background(), CreateScanInput{
			Name: "Reuse Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.NoError(t, err)
		assert.Equal(t, "Reuse Scan", scan.Name)
		assert.Equal(t, "connect", scan.ScanType)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("new CIDR creates network then job", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO networks`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := db.CreateScan(context.Background(), CreateScanInput{
			Name:        "New Scan",
			Description: "my desc",
			Targets:     []string{"192.168.1.0/24"},
			ScanType:    "syn",
			Ports:       "22,443",
		})
		require.NoError(t, err)
		assert.Equal(t, "New Scan", scan.Name)
		assert.Equal(t, "my desc", scan.Description)
		assert.Equal(t, "syn", scan.ScanType)
		assert.Equal(t, "22,443", scan.Ports)
		assert.Equal(t, "pending", scan.Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("name collision falls back to CIDR as network name", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).WillReturnError(sql.ErrNoRows)
		// First INSERT: name collision — no rows affected.
		mock.ExpectExec(`INSERT INTO networks`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// Fallback INSERT using CIDR as name.
		mock.ExpectExec(`INSERT INTO networks`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := db.CreateScan(context.Background(), CreateScanInput{
			Name: "Colliding Name", Targets: []string{"172.16.0.0/12"}, ScanType: "connect",
		})
		require.NoError(t, err)
		assert.NotNil(t, scan)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("CIDR lookup error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).WillReturnError(fmt.Errorf("network error"))
		mock.ExpectRollback()

		_, err := db.CreateScan(context.Background(), CreateScanInput{
			Name: "Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "look up network by CIDR")
	})

	t.Run("scan job insert error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		networkID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))
		mock.ExpectExec(`INSERT INTO scan_jobs`).WillReturnError(fmt.Errorf("job insert failed"))
		mock.ExpectRollback()

		_, err := db.CreateScan(context.Background(), CreateScanInput{
			Name: "Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create scan job")
	})

	t.Run("multiple targets create one job each", func(t *testing.T) {
		db, mock := newMockDB(t)
		net1 := uuid.New()
		net2 := uuid.New()

		mock.ExpectBegin()
		// Target 1: exists.
		mock.ExpectQuery(`SELECT id FROM networks`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(net1))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		// Target 2: new.
		mock.ExpectQuery(`SELECT id FROM networks`).WillReturnError(sql.ErrNoRows)
		mock.ExpectExec(`INSERT INTO networks`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := db.CreateScan(context.Background(), CreateScanInput{
			Name:     "Multi Scan",
			Targets:  []string{"10.0.0.0/8", "192.168.0.0/16"},
			ScanType: "connect",
		})
		require.NoError(t, err)
		// firstJobID is from target-1's job.
		assert.NotEqual(t, uuid.Nil, scan.ID)
		_ = net2
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("with profile_id", func(t *testing.T) {
		db, mock := newMockDB(t)
		networkID := uuid.New()
		profile := "linux-server"

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id FROM networks`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := db.CreateScan(context.Background(), CreateScanInput{
			Name:      "Profile Scan",
			Targets:   []string{"10.0.0.0/8"},
			ScanType:  "connect",
			ProfileID: &profile,
		})
		require.NoError(t, err)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "linux-server", *scan.ProfileID)
	})
}

// ── DeleteScan ────────────────────────────────────────────────────────────────

func TestDeleteScan_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`SELECT status`).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
		mock.ExpectExec(`DELETE FROM scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		require.NoError(t, db.DeleteScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()

		err := db.DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("running scan returns CodeConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`SELECT status`).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
		mock.ExpectRollback()

		err := db.DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeConflict))
	})

	t.Run("begin error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx error"))

		err := db.DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("existence check error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := db.DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check scan existence")
	})
}

// ── StartScan ─────────────────────────────────────────────────────────────────

func TestStartScan_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, db.StartScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows affected returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := db.StartScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := db.StartScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start scan")
	})
}

// ── CompleteScan ──────────────────────────────────────────────────────────────

func TestCompleteScan_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, db.CompleteScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows affected returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := db.CompleteScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := db.CompleteScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "complete scan")
	})
}

// ── StopScan ──────────────────────────────────────────────────────────────────

func TestStopScan_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success without error message", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, db.StopScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success with error message", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, db.StopScan(context.Background(), id, "nmap timed out"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty string error message is treated as nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, db.StopScan(context.Background(), id, ""))
	})

	t.Run("zero rows affected returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := db.StopScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := db.StopScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop scan")
	})
}

// ── UpdateScan ────────────────────────────────────────────────────────────────

func TestUpdateScan_Unit(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("begin tx error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx error"))

		name := "x"
		_, err := db.UpdateScan(context.Background(), id, UpdateScanInput{Name: &name})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()

		name := "x"
		_, err := db.UpdateScan(context.Background(), id, UpdateScanInput{Name: &name})
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("success — name and scan_type updated", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(`UPDATE networks`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		// updateScanJob is a no-op when Status and ProfileID are both nil.
		mock.ExpectCommit()
		// GetScan called after commit.
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Renamed", sql.NullString{Valid: false}, "10.0.0.0/8",
				"syn", "", nil, "pending", now, nil, nil, nil, false,
			))

		renamed := "Renamed"
		synType := "syn"
		scan, err := db.UpdateScan(context.Background(), id, UpdateScanInput{
			Name: &renamed, ScanType: &synType,
		})
		require.NoError(t, err)
		assert.Equal(t, "Renamed", scan.Name)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty input skips SQL and still calls GetScan", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		// Neither UPDATE should fire when all fields are nil.
		mock.ExpectCommit()
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Unchanged", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", nil, "pending", now, nil, nil, nil, false,
			))

		scan, err := db.UpdateScan(context.Background(), id, UpdateScanInput{})
		require.NoError(t, err)
		assert.Equal(t, "Unchanged", scan.Name)
	})
}

// ── getHostScansCount ─────────────────────────────────────────────────────────

func TestGetHostScansCount_Unit(t *testing.T) {
	hostID := uuid.New()

	t.Run("returns count", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

		count, err := db.getHostScansCount(context.Background(), hostID)
		require.NoError(t, err)
		assert.Equal(t, int64(7), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("db error"))

		_, err := db.getHostScansCount(context.Background(), hostID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host scans count")
	})
}

// ── processHostScanRow ────────────────────────────────────────────────────────

var hostScanColumns = []string{
	"id", "name", "description", "targets", "scan_type", "ports",
	"profile_id", "status", "started_at", "completed_at", "created_at",
	"schedule_id", "tags", "options",
}

func TestProcessHostScanRow(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("minimal row", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(hostScanColumns).AddRow(
				id, "HostScan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "22", sql.NullString{Valid: false}, "completed",
				sql.NullTime{Valid: false}, sql.NullTime{Valid: false}, now,
				sql.NullInt64{Valid: false}, []byte("[]"), "{}",
			))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processHostScanRow(rows)
		require.NoError(t, err)
		assert.Equal(t, id, scan.ID)
		assert.Equal(t, "HostScan", scan.Name)
		assert.Equal(t, []string{"10.0.0.0/8"}, scan.Targets)
		assert.Empty(t, scan.Tags)
		assert.Nil(t, scan.StartedAt)
		assert.Nil(t, scan.CompletedAt)
		assert.Nil(t, scan.ProfileID)
		assert.Nil(t, scan.ScheduleID)
	})

	t.Run("all optional fields populated", func(t *testing.T) {
		db, mock := newMockDB(t)
		started := now.Add(-time.Minute)
		completed := now
		profile := "linux-server"
		scheduleID := int64(42)

		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(hostScanColumns).AddRow(
				id, "Full", sql.NullString{String: "desc", Valid: true},
				"192.168.1.0/24", "version", "80,443",
				sql.NullString{String: profile, Valid: true}, "completed",
				sql.NullTime{Time: started, Valid: true},
				sql.NullTime{Time: completed, Valid: true},
				now,
				sql.NullInt64{Int64: scheduleID, Valid: true},
				[]byte(`["web","infra"]`), "{}",
			))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processHostScanRow(rows)
		require.NoError(t, err)
		assert.Equal(t, "desc", scan.Description)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "linux-server", *scan.ProfileID)
		require.NotNil(t, scan.StartedAt)
		assert.Equal(t, started, *scan.StartedAt)
		require.NotNil(t, scan.CompletedAt)
		assert.Equal(t, completed, *scan.CompletedAt)
		require.NotNil(t, scan.ScheduleID)
		assert.Equal(t, int64(42), *scan.ScheduleID)
		assert.Equal(t, []string{"web", "infra"}, scan.Tags)
	})

	t.Run("invalid tags JSON falls back to empty slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(hostScanColumns).AddRow(
				id, "Scan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", sql.NullString{Valid: false}, "pending",
				sql.NullTime{Valid: false}, sql.NullTime{Valid: false}, now,
				sql.NullInt64{Valid: false}, []byte("{invalid json}"), "{}",
			))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processHostScanRow(rows)
		require.NoError(t, err)
		assert.Equal(t, []string{}, scan.Tags)
	})

	t.Run("null tags JSON returns empty slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(hostScanColumns).AddRow(
				id, "Scan", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", sql.NullString{Valid: false}, "pending",
				sql.NullTime{Valid: false}, sql.NullTime{Valid: false}, now,
				sql.NullInt64{Valid: false}, []byte(nil), "{}",
			))

		rows, err := db.QueryContext(context.Background(), "SELECT 1")
		require.NoError(t, err)
		defer rows.Close()
		require.True(t, rows.Next())

		scan, err := processHostScanRow(rows)
		require.NoError(t, err)
		assert.Equal(t, []string{}, scan.Tags)
	})
}

// ── GetHostScans ──────────────────────────────────────────────────────────────

func TestGetHostScans_Unit(t *testing.T) {
	now := time.Now().UTC()
	hostID := uuid.New()
	scanID := uuid.New()

	t.Run("count error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("count error"))

		_, _, err := db.GetHostScans(context.Background(), hostID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host scans count")
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT DISTINCT`).WillReturnError(fmt.Errorf("list error"))

		_, _, err := db.GetHostScans(context.Background(), hostID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host scans")
	})

	t.Run("empty result", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT DISTINCT`).
			WillReturnRows(sqlmock.NewRows(hostScanColumns))

		scans, total, err := db.GetHostScans(context.Background(), hostID, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, scans)
	})

	t.Run("returns rows correctly", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT DISTINCT`).WillReturnRows(
			sqlmock.NewRows(hostScanColumns).AddRow(
				scanID, "HostScan", sql.NullString{Valid: false},
				"10.0.0.0/8", "connect", "22",
				sql.NullString{Valid: false}, "completed",
				sql.NullTime{Valid: false}, sql.NullTime{Valid: false}, now,
				sql.NullInt64{Valid: false}, []byte("[]"), "{}",
			))

		scans, total, err := db.GetHostScans(context.Background(), hostID, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, scans, 1)
		assert.Equal(t, scanID, scans[0].ID)
		assert.Equal(t, "HostScan", scans[0].Name)
	})
}
