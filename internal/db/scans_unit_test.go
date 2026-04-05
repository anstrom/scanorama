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

// ── updateScanJob (direct) ────────────────────────────────────────────────────

func TestUpdateScanJob_Unit(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	t.Run("no fields — no-op returns nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		// No SQL should fire.
		err := updateScanJob(ctx, db.DB, id, UpdateScanInput{})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("status only", func(t *testing.T) {
		db, mock := newMockDB(t)
		status := "completed"
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := updateScanJob(ctx, db.DB, id, UpdateScanInput{Status: &status})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("profile_id only", func(t *testing.T) {
		db, mock := newMockDB(t)
		profile := "linux-server"
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := updateScanJob(ctx, db.DB, id, UpdateScanInput{ProfileID: &profile})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("status and profile_id together", func(t *testing.T) {
		db, mock := newMockDB(t)
		status := "failed"
		profile := "windows-server"
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := updateScanJob(ctx, db.DB, id, UpdateScanInput{
			Status: &status, ProfileID: &profile,
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		status := "completed"
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnError(fmt.Errorf("connection lost"))

		err := updateScanJob(ctx, db.DB, id, UpdateScanInput{Status: &status})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update scan job")
	})
}

// ── GetScanResults ────────────────────────────────────────────────────────────

var scanResultColumns = []string{
	"id", "job_id", "host_id", "host_ip",
	"port", "protocol", "state", "service_name",
	"scanned_at", "os_name", "os_family", "os_version", "os_confidence",
}

func TestGetScanResults_Unit(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()
	now := time.Now().UTC()

	t.Run("count query error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewScanRepository(db).GetScanResults(ctx, scanID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan results count")
	})

	t.Run("list query error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).
			WillReturnError(fmt.Errorf("query failed"))

		_, _, err := NewScanRepository(db).GetScanResults(ctx, scanID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query scan results")
	})

	t.Run("empty result returns zero total and nil slice", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows(scanResultColumns))

		results, total, err := NewScanRepository(db).GetScanResults(ctx, scanID, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, results)
	})

	t.Run("returns rows correctly — service_name nil becomes empty string", func(t *testing.T) {
		db, mock := newMockDB(t)
		resultID := uuid.New()
		hostID := uuid.New()

		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(scanResultColumns).AddRow(
				resultID, scanID, hostID, "10.0.0.1",
				80, "tcp", "open", nil,
				now, "Linux", "linux", "5.15", 90,
			))

		results, total, err := NewScanRepository(db).GetScanResults(ctx, scanID, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, results, 1)
		assert.Equal(t, resultID, results[0].ID)
		assert.Equal(t, scanID, results[0].ScanID)
		assert.Equal(t, "10.0.0.1", results[0].HostIP)
		assert.Equal(t, 80, results[0].Port)
		assert.Equal(t, "tcp", results[0].Protocol)
		assert.Equal(t, "open", results[0].State)
		assert.Equal(t, "", results[0].Service) // nil → ""
		assert.Equal(t, "Linux", results[0].OSName)
		assert.Equal(t, 90, *results[0].OSConfidence)
	})

	t.Run("returns rows correctly — service_name populated", func(t *testing.T) {
		db, mock := newMockDB(t)
		svc := "http"
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(scanResultColumns).AddRow(
				uuid.New(), scanID, uuid.New(), "192.168.1.1",
				443, "tcp", "open", &svc,
				now, "", "", "", nil,
			))

		results, _, err := NewScanRepository(db).GetScanResults(ctx, scanID, 0, 10)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "http", results[0].Service)
		assert.Nil(t, results[0].OSConfidence)
	})
}

// ── GetScanSummary ────────────────────────────────────────────────────────────

func TestGetScanSummary_Unit(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()

	t.Run("no port_scans rows returns zero-value summary without error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).
			WillReturnError(sql.ErrNoRows)

		summary, err := NewScanRepository(db).GetScanSummary(ctx, scanID)
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, scanID, summary.ScanID)
		assert.Equal(t, 0, summary.TotalHosts)
		assert.Equal(t, 0, summary.OpenPorts)
		assert.Equal(t, int64(0), summary.Duration)
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).
			WillReturnError(fmt.Errorf("connection reset"))

		_, err := NewScanRepository(db).GetScanSummary(ctx, scanID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan summary")
	})

	t.Run("returns aggregated stats — nil duration_seconds stays zero", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows([]string{
				"total_hosts", "total_ports", "open_ports", "closed_ports", "duration_seconds",
			}).AddRow(3, 12, 5, 7, nil))

		summary, err := NewScanRepository(db).GetScanSummary(ctx, scanID)
		require.NoError(t, err)
		assert.Equal(t, 3, summary.TotalHosts)
		assert.Equal(t, 12, summary.TotalPorts)
		assert.Equal(t, 5, summary.OpenPorts)
		assert.Equal(t, 7, summary.ClosedPorts)
		assert.Equal(t, int64(0), summary.Duration)
	})

	t.Run("returns aggregated stats — duration_seconds populated", func(t *testing.T) {
		db, mock := newMockDB(t)
		dur := 42
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows([]string{
				"total_hosts", "total_ports", "open_ports", "closed_ports", "duration_seconds",
			}).AddRow(1, 4, 2, 2, &dur))

		summary, err := NewScanRepository(db).GetScanSummary(ctx, scanID)
		require.NoError(t, err)
		assert.Equal(t, int64(42), summary.Duration)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// scanColumns are the columns returned by the ListScans / processScanRow query.
var scanColumns = []string{
	"id", "name", "description", "network_cidr", "scan_type", "ports",
	"profile_id", "status", "created_at", "started_at", "completed_at",
	"error_message", "execution_details",
}

// getScanColumns are the columns returned by GetScan: same as scanColumns but
// with os_detection inserted before execution_details.
var getScanColumns = []string{
	"id", "name", "description", "network_cidr", "scan_type", "ports",
	"profile_id", "status", "created_at", "started_at", "completed_at",
	"error_message", "os_detection", "execution_details",
}

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
		nil, // execution_details
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

// ── isHostTarget / allHostTargets ────────────────────────────────────────────

func TestIsHostTarget(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"10.0.0.1", true},        // bare IPv4
		{"192.168.1.100", true},   // bare IPv4
		{"10.0.0.1/32", true},     // explicit /32
		{"::1", true},             // bare IPv6 loopback
		{"2001:db8::1/128", true}, // explicit /128
		{"10.0.0.0/8", false},     // network CIDR /8
		{"192.168.1.0/24", false}, // /24 network
		{"172.16.0.0/12", false},  // /12 network
		{"10.0.0.0/31", false},    // /31 point-to-point (two hosts, not one)
		{"notanip", false},        // garbage — not a valid IP or CIDR
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.want, isHostTarget(tc.input))
		})
	}
}

func TestAllHostTargets(t *testing.T) {
	t.Run("all bare IPs returns true", func(t *testing.T) {
		assert.True(t, allHostTargets([]string{"10.0.0.1", "192.168.1.5"}))
	})
	t.Run("all /32 CIDRs returns true", func(t *testing.T) {
		assert.True(t, allHostTargets([]string{"10.0.0.1/32", "10.0.0.2/32"}))
	})
	t.Run("mixed bare IP and /32 returns true", func(t *testing.T) {
		assert.True(t, allHostTargets([]string{"10.0.0.1", "10.0.0.2/32"}))
	})
	t.Run("any network CIDR makes it false", func(t *testing.T) {
		assert.False(t, allHostTargets([]string{"10.0.0.1", "10.0.0.0/24"}))
	})
	t.Run("single network CIDR returns false", func(t *testing.T) {
		assert.False(t, allHostTargets([]string{"10.0.0.0/8"}))
	})
	t.Run("empty slice is vacuously true", func(t *testing.T) {
		assert.True(t, allHostTargets([]string{}))
	})
}

// ── getScanCount ──────────────────────────────────────────────────────────────

func TestGetScanCount(t *testing.T) {
	t.Run("returns count on success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

		count, err := NewScanRepository(db).getScanCount(context.Background(), "", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(42), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("propagates db error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("db unavailable"))

		_, err := NewScanRepository(db).getScanCount(context.Background(), "", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan count")
	})

	t.Run("where clause is appended", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT.*WHERE sj\.status`).
			WithArgs("running").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

		count, err := NewScanRepository(db).getScanCount(
			context.Background(), "WHERE sj.status = $1", []interface{}{"running"})
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
			nil, // execution_details
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
			nil, // execution_details
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
			nil, // execution_details
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
			nil, // execution_details
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
			nil, // execution_details
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

		_, _, err := NewScanRepository(db).ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get scan count")
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("query failed"))

		_, _, err := NewScanRepository(db).ListScans(context.Background(), ScanFilters{}, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list scans")
	})

	t.Run("empty result set", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows(scanColumns))

		scans, total, err := NewScanRepository(db).ListScans(context.Background(), ScanFilters{}, 0, 10)
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

		scans, total, err := NewScanRepository(db).ListScans(context.Background(), ScanFilters{}, 0, 10)
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

		_, _, err := NewScanRepository(db).ListScans(context.Background(), ScanFilters{
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

		_, err := NewScanRepository(db).GetScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("connection reset"))

		_, err := NewScanRepository(db).GetScan(context.Background(), id)
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
				sql.NullString{Valid: false},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
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
				sql.NullString{Valid: false},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
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
				sql.NullString{Valid: false},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
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
				sql.NullString{Valid: false},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Nil(t, scan.DurationStr)
		assert.Equal(t, started, scan.UpdatedAt)
	})

	t.Run("stored scan_targets override network CIDR", func(t *testing.T) {
		db, mock := newMockDB(t)
		execDetails := `{"os_detection": false, "scan_targets": ["10.0.0.1", "10.0.0.2"]}`
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Multi-target", sql.NullString{Valid: false}, "10.0.0.1/32",
				"connect", "80,443", nil, "pending",
				now, nil, nil, nil, false,
				sql.NullString{String: execDetails, Valid: true},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, scan.Targets)
	})

	t.Run("invalid scan_targets JSON falls back to network CIDR", func(t *testing.T) {
		db, mock := newMockDB(t)
		execDetails := `{"os_detection": false, "scan_targets": "not-an-array"}`
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Fallback", sql.NullString{Valid: false}, "10.0.0.1/32",
				"connect", "", nil, "pending",
				now, nil, nil, nil, false,
				sql.NullString{String: execDetails, Valid: true},
			))

		scan, err := NewScanRepository(db).GetScan(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, []string{"10.0.0.1/32"}, scan.Targets)
	})
}

// ── CreateScan ────────────────────────────────────────────────────────────────

func TestCreateScan_Unit(t *testing.T) {
	t.Run("empty name returns validation error", func(t *testing.T) {
		db, _ := newMockDB(t)
		_, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("begin transaction error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx unavailable"))

		_, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
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

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name: "Reuse Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.NoError(t, err)
		assert.Equal(t, "Reuse Scan", scan.Name)
		assert.Equal(t, "connect", scan.ScanType)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("bare-IP targets create one job with no network row", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// Bare IPs are host targets — no network lookup or INSERT into networks.
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name:     "Host-target Scan",
			Targets:  []string{"10.0.0.1", "10.0.0.2"},
			ScanType: "connect",
			Ports:    "80,443",
		})
		require.NoError(t, err)
		// The response must carry the full original target list.
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, scan.Targets)
		assert.Equal(t, "pending", scan.Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("/32 CIDR target creates one job with no network row", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// A /32 CIDR denotes a single host — no network row should be created.
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name:     "Single-host CIDR Scan",
			Targets:  []string{"192.168.1.100/32"},
			ScanType: "connect",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"192.168.1.100/32"}, scan.Targets)
		assert.Equal(t, "pending", scan.Status)
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

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
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

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
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

		_, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
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

		_, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name: "Scan", Targets: []string{"10.0.0.0/8"}, ScanType: "connect",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create scan job")
	})

	t.Run("multiple targets create exactly one job using first target's network", func(t *testing.T) {
		db, mock := newMockDB(t)
		net1 := uuid.New()

		mock.ExpectBegin()
		// Only the FIRST target is looked up — no second SELECT.
		mock.ExpectQuery(`SELECT id FROM networks`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(net1))
		// Only ONE scan_jobs INSERT regardless of target count.
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name:     "Multi Scan",
			Targets:  []string{"10.0.0.0/8", "192.168.0.0/16"},
			ScanType: "connect",
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, scan.ID)
		// The full target list must be preserved in the response.
		assert.Equal(t, []string{"10.0.0.0/8", "192.168.0.0/16"}, scan.Targets)
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

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name:      "Profile Scan",
			Targets:   []string{"10.0.0.0/8"},
			ScanType:  "connect",
			ProfileID: &profile,
		})
		require.NoError(t, err)
		require.NotNil(t, scan.ProfileID)
		assert.Equal(t, "linux-server", *scan.ProfileID)
	})

	t.Run("pre-supplied NetworkID skips network lookup and insert", func(t *testing.T) {
		db, mock := newMockDB(t)
		netID := uuid.New()

		mock.ExpectBegin()
		// No SELECT or INSERT into networks — networkID is provided by the caller.
		mock.ExpectExec(`INSERT INTO scan_jobs`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		scan, err := NewScanRepository(db).CreateScan(context.Background(), CreateScanInput{
			Name:      "Network Scan",
			Targets:   []string{"10.0.0.1", "10.0.0.2"},
			ScanType:  "connect",
			Ports:     "1-1024",
			NetworkID: &netID,
		})
		require.NoError(t, err)
		assert.Equal(t, "Network Scan", scan.Name)
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, scan.Targets)
		assert.Equal(t, "pending", scan.Status)
		// All mock expectations must be satisfied — in particular, no network
		// SELECT or INSERT should have been issued.
		require.NoError(t, mock.ExpectationsWereMet())
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

		require.NoError(t, NewScanRepository(db).DeleteScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()

		err := NewScanRepository(db).DeleteScan(context.Background(), id)
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

		err := NewScanRepository(db).DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeConflict))
	})

	t.Run("begin error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx error"))

		err := NewScanRepository(db).DeleteScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("existence check error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := NewScanRepository(db).DeleteScan(context.Background(), id)
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

		require.NoError(t, NewScanRepository(db).StartScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows + scan exists in wrong state returns CodeConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT status FROM scan_jobs WHERE id`).
			WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))

		err := NewScanRepository(db).StartScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsConflict(err), "expected conflict, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows + scan not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT status FROM scan_jobs WHERE id`).
			WillReturnError(sql.ErrNoRows)

		err := NewScanRepository(db).StartScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err), "expected not-found, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows + secondary SELECT fails with generic error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT status FROM scan_jobs WHERE id`).
			WillReturnError(fmt.Errorf("connection reset"))

		err := NewScanRepository(db).StartScan(context.Background(), id)
		require.Error(t, err)
		assert.False(t, errors.IsConflict(err), "generic SELECT error should not be conflict")
		assert.False(t, errors.IsNotFound(err), "generic SELECT error should not be not-found")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rows affected error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewErrorResult(fmt.Errorf("rows affected failed")))

		err := NewScanRepository(db).StartScan(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rows affected")
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := NewScanRepository(db).StartScan(context.Background(), id)
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

		require.NoError(t, NewScanRepository(db).CompleteScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero rows affected returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewScanRepository(db).CompleteScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := NewScanRepository(db).CompleteScan(context.Background(), id)
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

		require.NoError(t, NewScanRepository(db).StopScan(context.Background(), id))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success with error message", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, NewScanRepository(db).StopScan(context.Background(), id, "nmap timed out"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty string error message is treated as nil", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, NewScanRepository(db).StopScan(context.Background(), id, ""))
	})

	t.Run("zero rows affected returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewScanRepository(db).StopScan(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE scan_jobs`).WillReturnError(fmt.Errorf("db error"))

		err := NewScanRepository(db).StopScan(context.Background(), id)
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
		_, err := NewScanRepository(db).UpdateScan(context.Background(), id, UpdateScanInput{Name: &name})
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
		_, err := NewScanRepository(db).UpdateScan(context.Background(), id, UpdateScanInput{Name: &name})
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
		// updateHostOnlyScanDetails is a no-op for network-backed scans
		// (WHERE network_id IS NULL matches nothing) but the exec is still issued.
		mock.ExpectExec(`UPDATE scan_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// updateScanJob is a no-op when Status and ProfileID are both nil.
		mock.ExpectCommit()
		// GetScan called after commit.
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Renamed", sql.NullString{Valid: false}, "10.0.0.0/8",
				"syn", "", nil, "pending", now, nil, nil, nil, false,
				sql.NullString{Valid: false},
			))

		renamed := "Renamed"
		synType := "syn"
		scan, err := NewScanRepository(db).UpdateScan(context.Background(), id, UpdateScanInput{
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
		// Neither updateScanNetwork nor updateHostOnlyScanDetails fire when
		// all metadata fields are nil, and updateScanJob is also a no-op.
		mock.ExpectCommit()
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(getScanColumns).AddRow(
				id, "Unchanged", sql.NullString{Valid: false}, "10.0.0.0/8",
				"connect", "", nil, "pending", now, nil, nil, nil, false,
				sql.NullString{Valid: false},
			))

		scan, err := NewScanRepository(db).UpdateScan(context.Background(), id, UpdateScanInput{})
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

		count, err := NewHostRepository(db).getHostScansCount(context.Background(), hostID)
		require.NoError(t, err)
		assert.Equal(t, int64(7), count)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("db error"))

		_, err := NewHostRepository(db).getHostScansCount(context.Background(), hostID)
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

		_, _, err := NewHostRepository(db).GetHostScans(context.Background(), hostID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host scans count")
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT DISTINCT`).WillReturnError(fmt.Errorf("list error"))

		_, _, err := NewHostRepository(db).GetHostScans(context.Background(), hostID, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host scans")
	})

	t.Run("empty result", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT DISTINCT`).
			WillReturnRows(sqlmock.NewRows(hostScanColumns))

		scans, total, err := NewHostRepository(db).GetHostScans(context.Background(), hostID, 0, 10)
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

		scans, total, err := NewHostRepository(db).GetHostScans(context.Background(), hostID, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, scans, 1)
		assert.Equal(t, scanID, scans[0].ID)
		assert.Equal(t, "HostScan", scans[0].Name)
	})
}
