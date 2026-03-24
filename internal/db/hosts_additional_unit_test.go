// Package db provides additional unit tests for host database operations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetHost ───────────────────────────────────────────────────────────────────

func TestGetHost_Unit(t *testing.T) {
	id := uuid.New()

	// Both error cases return before fetchHostPorts fires, so no additional
	// mock expectations are needed beyond the initial SELECT.

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)

		_, err := NewHostRepository(db).GetHost(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))

		_, err := NewHostRepository(db).GetHost(context.Background(), id)

		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── ListHosts ─────────────────────────────────────────────────────────────────

func TestListHosts_Unit(t *testing.T) {
	t.Run("count error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewHostRepository(db).ListHosts(context.Background(), &HostFilters{}, 0, 10)

		require.Error(t, err)
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT").
			WillReturnError(fmt.Errorf("list failed"))

		_, _, err := NewHostRepository(db).ListHosts(context.Background(), &HostFilters{}, 0, 10)

		require.Error(t, err)
	})
}

// ── GetByIP ───────────────────────────────────────────────────────────────────

func TestGetByIP_Unit(t *testing.T) {
	ip := IPAddr{IP: net.ParseIP("10.0.0.1")}

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("timeout"))

		_, err := NewHostRepository(db).GetByIP(context.Background(), ip)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get host")
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

		_, err := NewHostRepository(db).GetByIP(context.Background(), ip)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})
}

// ── GetActiveHosts ────────────────────────────────────────────────────────────

func TestGetActiveHosts_Unit(t *testing.T) {
	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("connection lost"))

		_, err := NewHostRepository(db).GetActiveHosts(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get active hosts")
	})

	t.Run("empty result returns nil slice without error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows([]string{
				"ip_address", "status", "last_seen", "open_ports", "total_ports_scanned",
			}))

		hosts, err := NewHostRepository(db).GetActiveHosts(context.Background())
		require.NoError(t, err)
		assert.Empty(t, hosts)
	})
}

// ── fetchHostPorts ────────────────────────────────────────────────────────────

func TestFetchHostPorts_Unit(t *testing.T) {
	hostID := uuid.New()
	now := time.Now().UTC()
	portCols := []string{"port", "protocol", "state", "service_name", "scanned_at"}

	t.Run("query error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT DISTINCT`).WillReturnError(fmt.Errorf("db error"))

		host := &Host{}
		err := NewHostRepository(db).fetchHostPorts(context.Background(), hostID, host)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query host ports")
	})

	t.Run("scan error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		// Return a row with wrong column count to force a scan error.
		mock.ExpectQuery(`SELECT DISTINCT`).
			WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

		host := &Host{}
		err := NewHostRepository(db).fetchHostPorts(context.Background(), hostID, host)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan port row")
	})

	t.Run("populates ports and increments TotalPorts", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT DISTINCT`).WillReturnRows(
			sqlmock.NewRows(portCols).
				AddRow(80, "tcp", "open", "http", now).
				AddRow(443, "tcp", "open", "https", now))

		host := &Host{}
		err := NewHostRepository(db).fetchHostPorts(context.Background(), hostID, host)
		require.NoError(t, err)
		assert.Equal(t, 2, host.TotalPorts)
		assert.Len(t, host.Ports, 2)
		assert.Equal(t, 80, host.Ports[0].Port)
		assert.Equal(t, "http", host.Ports[0].Service)
	})
}

// ── fetchHostScanCount ────────────────────────────────────────────────────────

func TestFetchHostScanCount_Unit(t *testing.T) {
	hostID := uuid.New()

	t.Run("populates ScanCount on success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

		host := &Host{}
		NewHostRepository(db).fetchHostScanCount(context.Background(), hostID, host)
		assert.Equal(t, 7, host.ScanCount)
	})

	t.Run("db error is non-fatal — ScanCount stays zero", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("timeout"))

		host := &Host{}
		// Should not panic or return an error — just log and continue.
		NewHostRepository(db).fetchHostScanCount(context.Background(), hostID, host)
		assert.Equal(t, 0, host.ScanCount)
	})
}

// ── UpdateHost ────────────────────────────────────────────────────────────────

func TestUpdateHost_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("begin tx error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx unavailable"))

		hostname := "srv"
		_, err := NewHostRepository(db).UpdateHost(context.Background(), id, UpdateHostInput{Hostname: &hostname})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()

		hostname := "srv"
		_, err := NewHostRepository(db).UpdateHost(context.Background(), id, UpdateHostInput{Hostname: &hostname})
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("empty input returns validation error without hitting DB", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()

		_, err := NewHostRepository(db).UpdateHost(context.Background(), id, UpdateHostInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no valid fields to update")
	})

	t.Run("update exec error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		hostname := "new-host"
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(`UPDATE hosts`).WillReturnError(fmt.Errorf("constraint violation"))
		mock.ExpectRollback()

		_, err := NewHostRepository(db).UpdateHost(context.Background(), id, UpdateHostInput{Hostname: &hostname})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update host")
	})

	t.Run("existence check error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		hostname := "x"
		_, err := NewHostRepository(db).UpdateHost(context.Background(), id, UpdateHostInput{Hostname: &hostname})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check host existence")
	})
}

// ── DeleteHost ────────────────────────────────────────────────────────────────

func TestDeleteHost_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("begin tx error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx error"))

		err := NewHostRepository(db).DeleteHost(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()

		err := NewHostRepository(db).DeleteHost(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("delete exec error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(`DELETE FROM hosts`).WillReturnError(fmt.Errorf("foreign key"))
		mock.ExpectRollback()

		err := NewHostRepository(db).DeleteHost(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete host")
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectExec(`DELETE FROM hosts`).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := NewHostRepository(db).DeleteHost(context.Background(), id)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("existence check error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := NewHostRepository(db).DeleteHost(context.Background(), id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check host existence")
	})
}
