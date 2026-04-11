// Package db — unit tests for SNMPRepository using sqlmock.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

var snmpDataCols = []string{
	"host_id", "sys_name", "sys_descr", "sys_location", "sys_contact",
	"sys_uptime", "if_count", "interfaces", "community", "collected_at",
}

// ── NewSNMPRepository ─────────────────────────────────────────────────────────

func TestSNMPRepository_New(t *testing.T) {
	mockDB, _ := newMockDB(t)
	repo := NewSNMPRepository(mockDB)
	require.NotNil(t, repo)
}

// ── UpsertSNMPData ────────────────────────────────────────────────────────────

func TestSNMPRepository_UpsertSNMPData_OK(t *testing.T) {
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectExec(`INSERT INTO host_snmp_data`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	sysName := "router-01"
	d := &HostSNMPData{
		HostID:  hostID,
		SysName: &sysName,
	}

	err := NewSNMPRepository(mockDB).UpsertSNMPData(context.Background(), d)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_UpsertSNMPData_NilHostID(t *testing.T) {
	mockDB, mock := newMockDB(t)

	d := &HostSNMPData{HostID: uuid.Nil}

	err := NewSNMPRepository(mockDB).UpsertSNMPData(context.Background(), d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host ID is required")

	// DB should never be touched.
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_UpsertSNMPData_DBError(t *testing.T) {
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectExec(`INSERT INTO host_snmp_data`).
		WillReturnError(fmt.Errorf("connection refused"))

	d := &HostSNMPData{HostID: hostID}

	err := NewSNMPRepository(mockDB).UpsertSNMPData(context.Background(), d)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_UpsertSNMPData_NilInterfaces(t *testing.T) {
	// When Interfaces is nil the repository should default it to "[]".
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectExec(`INSERT INTO host_snmp_data`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	d := &HostSNMPData{
		HostID:     hostID,
		Interfaces: nil,
	}

	err := NewSNMPRepository(mockDB).UpsertSNMPData(context.Background(), d)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetSNMPData ───────────────────────────────────────────────────────────────

func TestSNMPRepository_GetSNMPData_Found(t *testing.T) {
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()
	now := time.Now().UTC()

	sysName := "router-01"
	sysDescr := "Linux"
	sysLoc := "DC1"
	sysCont := "ops@example.com"
	var uptime int64 = 99999
	ifCount := 2
	community := "public"

	rows := sqlmock.NewRows(snmpDataCols).AddRow(
		hostID.String(),
		&sysName, &sysDescr, &sysLoc, &sysCont,
		&uptime, &ifCount,
		[]byte("[]"),
		&community,
		now,
	)
	mock.ExpectQuery(`SELECT host_id`).
		WithArgs(hostID).
		WillReturnRows(rows)

	d, err := NewSNMPRepository(mockDB).GetSNMPData(context.Background(), hostID)
	require.NoError(t, err)
	require.NotNil(t, d)

	assert.Equal(t, hostID, d.HostID)
	require.NotNil(t, d.SysName)
	assert.Equal(t, "router-01", *d.SysName)
	require.NotNil(t, d.IfCount)
	assert.Equal(t, 2, *d.IfCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_GetSNMPData_NotFound(t *testing.T) {
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery(`SELECT host_id`).
		WithArgs(hostID).
		WillReturnError(sql.ErrNoRows)

	_, err := NewSNMPRepository(mockDB).GetSNMPData(context.Background(), hostID)
	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_GetSNMPData_DBError(t *testing.T) {
	mockDB, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery(`SELECT host_id`).
		WithArgs(hostID).
		WillReturnError(fmt.Errorf("timeout"))

	_, err := NewSNMPRepository(mockDB).GetSNMPData(context.Background(), hostID)
	require.Error(t, err)
	assert.False(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListHostsWithSNMP ─────────────────────────────────────────────────────────

func TestSNMPRepository_ListHostsWithSNMP_OK(t *testing.T) {
	mockDB, mock := newMockDB(t)
	id1 := uuid.New()
	id2 := uuid.New()

	rows := sqlmock.NewRows([]string{"host_id"}).
		AddRow(id1.String()).
		AddRow(id2.String())

	mock.ExpectQuery(`SELECT host_id FROM host_snmp_data`).
		WillReturnRows(rows)

	ids, err := NewSNMPRepository(mockDB).ListHostsWithSNMP(context.Background())
	require.NoError(t, err)
	require.Len(t, ids, 2)
	assert.Equal(t, id1, ids[0])
	assert.Equal(t, id2, ids[1])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_ListHostsWithSNMP_Empty(t *testing.T) {
	mockDB, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT host_id FROM host_snmp_data`).
		WillReturnRows(sqlmock.NewRows([]string{"host_id"}))

	ids, err := NewSNMPRepository(mockDB).ListHostsWithSNMP(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPRepository_ListHostsWithSNMP_QueryError(t *testing.T) {
	mockDB, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT host_id FROM host_snmp_data`).
		WillReturnError(fmt.Errorf("query error"))

	_, err := NewSNMPRepository(mockDB).ListHostsWithSNMP(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
