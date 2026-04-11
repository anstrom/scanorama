// Package db — unit tests for DNSRepository using sqlmock.
// These run without a live database and exercise SQL-level behavior.
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dnsCols are the column names returned by ListDNSRecords.
var dnsCols = []string{"id", "host_id", "record_type", "value", "ttl", "resolved_at"}

// ── NewDNSRepository ──────────────────────────────────────────────────────────

func TestDNSRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewDNSRepository(db)
	require.NotNil(t, repo)
}

// ── UpsertDNSRecords ──────────────────────────────────────────────────────────

func TestDNSRepository_UpsertDNSRecords_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, []DNSRecord{})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_MultipleRecords(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	records := []DNSRecord{
		{ID: uuid.New(), RecordType: "PTR", Value: "host.example.com"},
		{ID: uuid.New(), RecordType: "A", Value: "192.0.2.1"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO host_dns_records").
		WithArgs(records[0].ID, hostID, "PTR", "host.example.com", (*int)(nil)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO host_dns_records").
		WithArgs(records[1].ID, hostID, "A", "192.0.2.1", (*int)(nil)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, records)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_AssignsID(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	// Pass a record with uuid.Nil — the repository should assign a new ID.
	records := []DNSRecord{
		{ID: uuid.Nil, RecordType: "TXT", Value: "v=spf1 include:example.com ~all"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// Match any UUID in the first argument position.
	mock.ExpectExec("INSERT INTO host_dns_records").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, records)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, records[0].ID, "repository should assign a UUID when ID is nil")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_BeginError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectBegin().WillReturnError(fmt.Errorf("connection refused"))

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), uuid.New(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_DeleteError(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnError(fmt.Errorf("table missing"))

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete existing")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_InsertError(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	records := []DNSRecord{
		{ID: uuid.New(), RecordType: "A", Value: "10.0.0.1"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO host_dns_records").
		WillReturnError(fmt.Errorf("constraint violation"))

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, records)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_UpsertDNSRecords_CommitError(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM host_dns_records").
		WithArgs(hostID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	err := NewDNSRepository(db).UpsertDNSRecords(context.Background(), hostID, []DNSRecord{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "commit")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListDNSRecords ────────────────────────────────────────────────────────────

func TestDNSRepository_ListDNSRecords_Found(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	now := time.Now().UTC()
	id1 := uuid.New()
	id2 := uuid.New()

	mock.ExpectQuery("SELECT .+ FROM host_dns_records").
		WithArgs(hostID).
		WillReturnRows(
			sqlmock.NewRows(dnsCols).
				AddRow(id1, hostID, "PTR", "host.example.com", nil, now).
				AddRow(id2, hostID, "A", "192.0.2.1", nil, now),
		)

	records, err := NewDNSRepository(db).ListDNSRecords(context.Background(), hostID)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "PTR", records[0].RecordType)
	assert.Equal(t, "host.example.com", records[0].Value)
	assert.Equal(t, "A", records[1].RecordType)
	assert.Equal(t, "192.0.2.1", records[1].Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_ListDNSRecords_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery("SELECT .+ FROM host_dns_records").
		WithArgs(hostID).
		WillReturnRows(sqlmock.NewRows(dnsCols))

	records, err := NewDNSRepository(db).ListDNSRecords(context.Background(), hostID)
	require.NoError(t, err)
	require.NotNil(t, records, "ListDNSRecords must return a non-nil slice for zero rows")
	assert.Empty(t, records)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDNSRepository_ListDNSRecords_QueryError(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery("SELECT .+ FROM host_dns_records").
		WithArgs(hostID).
		WillReturnError(fmt.Errorf("connection lost"))

	records, err := NewDNSRepository(db).ListDNSRecords(context.Background(), hostID)
	require.Error(t, err)
	assert.Nil(t, records)
	require.NoError(t, mock.ExpectationsWereMet())
}
