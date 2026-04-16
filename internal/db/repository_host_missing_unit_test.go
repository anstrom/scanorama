package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── CreateOrUpdate ─────────────────────────────────────────────────────────

func TestCreateOrUpdate_InsertPopulatesTimestamps(t *testing.T) {
	db, mock := newMockDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	host := &Host{
		IPAddress: IPAddr{IP: net.ParseIP("10.1.2.3")},
		Status:    "up",
	}

	mock.ExpectQuery(`INSERT INTO hosts`).
		WillReturnRows(
			sqlmock.NewRows([]string{"first_seen", "last_seen"}).
				AddRow(now, now),
		)

	require.NoError(t, NewHostRepository(db).CreateOrUpdate(context.Background(), host))

	assert.False(t, host.ID == uuid.Nil, "ID should be assigned when nil on input")
	assert.WithinDuration(t, now, host.FirstSeen, time.Second)
	assert.WithinDuration(t, now, host.LastSeen, time.Second)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateOrUpdate_PreservesExistingID(t *testing.T) {
	db, mock := newMockDB(t)

	id := uuid.New()
	host := &Host{ID: id, IPAddress: IPAddr{IP: net.ParseIP("10.1.2.4")}, Status: "up"}
	now := time.Now().UTC()

	mock.ExpectQuery(`INSERT INTO hosts`).
		WillReturnRows(sqlmock.NewRows([]string{"first_seen", "last_seen"}).AddRow(now, now))

	require.NoError(t, NewHostRepository(db).CreateOrUpdate(context.Background(), host))
	assert.Equal(t, id, host.ID, "pre-set ID must not be replaced")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateOrUpdate_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`INSERT INTO hosts`).WillReturnError(errors.New("connection reset"))

	err := NewHostRepository(db).CreateOrUpdate(context.Background(), &Host{
		IPAddress: IPAddr{IP: net.ParseIP("10.1.2.5")},
		Status:    "up",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create or update host")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── BulkDeleteHosts ───────────────────────────────────────────────────────

func TestBulkDeleteHosts_EmptyIDsIsNoOp(t *testing.T) {
	db, mock := newMockDB(t)

	n, err := NewHostRepository(db).BulkDeleteHosts(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	require.NoError(t, mock.ExpectationsWereMet()) // no queries issued
}

func TestBulkDeleteHosts_DeletesAndReturnsCount(t *testing.T) {
	db, mock := newMockDB(t)

	id1, id2 := uuid.New(), uuid.New()
	mock.ExpectExec(`DELETE FROM hosts WHERE id = ANY`).
		WillReturnResult(sqlmock.NewResult(0, 2))

	n, err := NewHostRepository(db).BulkDeleteHosts(context.Background(), []uuid.UUID{id1, id2})

	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkDeleteHosts_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`DELETE FROM hosts WHERE id = ANY`).
		WillReturnError(errors.New("connection reset"))

	_, err := NewHostRepository(db).BulkDeleteHosts(context.Background(), []uuid.UUID{uuid.New()})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bulk delete hosts")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetHostNetworks ───────────────────────────────────────────────────────

// networkColumns matches the SELECT column list in GetHostNetworks.
var networkColumns = []string{
	"id", "name", "cidr", "description", "discovery_method",
	"is_active", "scan_enabled", "scan_interval_seconds",
	"scan_ports", "scan_type",
	"last_discovery", "last_scan",
	"host_count", "active_host_count",
	"created_at", "updated_at", "created_by", "modified_by",
}

func networkRow(id uuid.UUID, name, cidr string) []driver.Value {
	now := time.Now().UTC()
	return []driver.Value{
		id, name, cidr, nil, "tcp",
		true, true, 0,
		"", "",
		nil, nil,
		0, 0,
		now, now, nil, nil,
	}
}

func TestGetHostNetworks_EmptyResult(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery(`SELECT.*FROM host_network_memberships`).
		WillReturnRows(sqlmock.NewRows(networkColumns))

	networks, err := NewHostRepository(db).GetHostNetworks(context.Background(), hostID)

	require.NoError(t, err)
	assert.Empty(t, networks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetHostNetworks_ReturnsNetworks(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()
	netID := uuid.New()

	rows := sqlmock.NewRows(networkColumns).AddRow(networkRow(netID, "corp", "10.0.0.0/8")...)

	mock.ExpectQuery(`SELECT.*FROM host_network_memberships`).WillReturnRows(rows)

	networks, err := NewHostRepository(db).GetHostNetworks(context.Background(), hostID)

	require.NoError(t, err)
	require.Len(t, networks, 1)
	assert.Equal(t, netID, networks[0].ID)
	assert.Equal(t, "corp", networks[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetHostNetworks_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT.*FROM host_network_memberships`).
		WillReturnError(errors.New("connection reset"))

	_, err := NewHostRepository(db).GetHostNetworks(context.Background(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get host networks")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── RecalculateKnowledgeScore ─────────────────────────────────────────────

func TestRecalculateKnowledgeScore_ExecutesUpdate(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectExec(`UPDATE hosts`).
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, NewHostRepository(db).RecalculateKnowledgeScore(context.Background(), hostID))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecalculateKnowledgeScore_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`UPDATE hosts`).
		WillReturnError(&pq.Error{Code: "53300", Message: "too many connections"})

	err := NewHostRepository(db).RecalculateKnowledgeScore(context.Background(), uuid.New())

	require.Error(t, err)
	assert.Contains(t, fmt.Sprintf("%v", err), "recalculate knowledge score")
	require.NoError(t, mock.ExpectationsWereMet())
}
