package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// deviceCols are the columns returned by the devices SELECT.
var deviceCols = []string{"id", "name", "notes", "created_at", "updated_at"}

func makeDeviceRow(id uuid.UUID, name string) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows(deviceCols).AddRow(id, name, nil, now, now)
}

// ── NewDeviceRepository ───────────────────────────────────────────────────

func TestDeviceRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewDeviceRepository(db)
	require.NotNil(t, repo)
}

// ── CreateDevice ─────────────────────────────────────────────────────────

func TestDeviceRepository_CreateDevice_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery(`INSERT INTO devices`).
		WithArgs("My Router", nil).
		WillReturnRows(makeDeviceRow(id, "My Router"))

	device, err := NewDeviceRepository(db).CreateDevice(context.Background(), CreateDeviceInput{
		Name: "My Router",
	})

	require.NoError(t, err)
	assert.Equal(t, "My Router", device.Name)
	assert.Equal(t, id, device.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_CreateDevice_UniqueConflict(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`INSERT INTO devices`).
		WillReturnError(&pq.Error{Code: pqerror.UniqueViolation})

	_, err := NewDeviceRepository(db).CreateDevice(context.Background(), CreateDeviceInput{Name: "Router"})

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetDevice ─────────────────────────────────────────────────────────────

func TestDeviceRepository_GetDevice_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, notes, created_at, updated_at FROM devices WHERE id`).
		WithArgs(id).
		WillReturnRows(makeDeviceRow(id, "Switch"))

	device, err := NewDeviceRepository(db).GetDevice(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, id, device.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_GetDevice_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT id, name, notes, created_at, updated_at FROM devices WHERE id`).
		WillReturnRows(sqlmock.NewRows(deviceCols)) // empty

	_, err := NewDeviceRepository(db).GetDevice(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateDevice ─────────────────────────────────────────────────────────

func TestDeviceRepository_UpdateDevice_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	newName := "Updated Router"

	mock.ExpectQuery(`UPDATE devices SET`).
		WithArgs(id, &newName, (*string)(nil)).
		WillReturnRows(makeDeviceRow(id, newName))

	device, err := NewDeviceRepository(db).UpdateDevice(context.Background(), id, UpdateDeviceInput{
		Name: &newName,
	})

	require.NoError(t, err)
	assert.Equal(t, newName, device.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_UpdateDevice_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`UPDATE devices SET`).
		WillReturnRows(sqlmock.NewRows(deviceCols)) // empty = no match

	_, err := NewDeviceRepository(db).UpdateDevice(context.Background(), uuid.New(), UpdateDeviceInput{})

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteDevice ─────────────────────────────────────────────────────────

func TestDeviceRepository_DeleteDevice_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM devices`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).DeleteDevice(context.Background(), id)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_DeleteDevice_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`DELETE FROM devices`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	err := NewDeviceRepository(db).DeleteDevice(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── AttachHost ────────────────────────────────────────────────────────────

func TestDeviceRepository_AttachHost_Success(t *testing.T) {
	db, mock := newMockDB(t)
	deviceID, hostID := uuid.New(), uuid.New()

	mock.ExpectExec(`UPDATE hosts SET device_id`).
		WithArgs(deviceID, hostID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).AttachHost(context.Background(), deviceID, hostID)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_AttachHost_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`UPDATE hosts SET device_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewDeviceRepository(db).AttachHost(context.Background(), uuid.New(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DetachHost ────────────────────────────────────────────────────────────

func TestDeviceRepository_DetachHost_Success(t *testing.T) {
	db, mock := newMockDB(t)
	deviceID, hostID := uuid.New(), uuid.New()

	mock.ExpectExec(`UPDATE hosts SET device_id = NULL`).
		WithArgs(hostID, deviceID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).DetachHost(context.Background(), deviceID, hostID)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DismissSuggestion ────────────────────────────────────────────────────

func TestDeviceRepository_DismissSuggestion_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec(`UPDATE device_suggestions SET dismissed`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).DismissSuggestion(context.Background(), id)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_DismissSuggestion_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec(`UPDATE device_suggestions SET dismissed`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewDeviceRepository(db).DismissSuggestion(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListDevices ───────────────────────────────────────────────────────────

func TestDeviceRepository_ListDevices_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT d.id, d.name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "mac_count", "host_count"}))

	result, err := NewDeviceRepository(db).ListDevices(context.Background())

	require.NoError(t, err)
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_ListDevices_ReturnsRows(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery(`SELECT d.id, d.name`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "mac_count", "host_count"}).
			AddRow(id, "Router", 2, 3))

	result, err := NewDeviceRepository(db).ListDevices(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Router", result[0].Name)
	assert.Equal(t, 2, result[0].MACCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateMDNSName ────────────────────────────────────────────────────────

func TestDeviceRepository_UpdateMDNSName_Success(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectExec(`UPDATE hosts SET mdns_name`).
		WithArgs(hostID, "myphone.local").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).UpdateMDNSName(context.Background(), hostID, "myphone.local")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertSuggestion ─────────────────────────────────────────────────────

func TestDeviceRepository_UpsertSuggestion_Success(t *testing.T) {
	db, mock := newMockDB(t)
	hostID, deviceID := uuid.New(), uuid.New()
	reason := "MAC:stable"

	mock.ExpectExec(`INSERT INTO device_suggestions`).
		WithArgs(hostID, deviceID, 3, reason).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).UpsertSuggestion(context.Background(), hostID, deviceID, 3, reason)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── AcceptSuggestion ──────────────────────────────────────────────────────

func TestDeviceRepository_AcceptSuggestion_Success(t *testing.T) {
	db, mock := newMockDB(t)
	suggestionID := uuid.New()
	hostID, deviceID := uuid.New(), uuid.New()

	mock.ExpectQuery(`SELECT host_id, device_id FROM device_suggestions`).
		WithArgs(suggestionID).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "device_id"}).
			AddRow(hostID, deviceID))

	mock.ExpectExec(`UPDATE hosts SET device_id`).
		WithArgs(deviceID, hostID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectExec(`DELETE FROM device_suggestions`).
		WithArgs(suggestionID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewDeviceRepository(db).AcceptSuggestion(context.Background(), suggestionID)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_AcceptSuggestion_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT host_id, device_id FROM device_suggestions`).
		WillReturnRows(sqlmock.NewRows([]string{"host_id", "device_id"})) // empty

	err := NewDeviceRepository(db).AcceptSuggestion(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── AllDevicesWithSignals ─────────────────────────────────────────────────

func TestDeviceRepository_AllDevicesWithSignals_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	// Pass 1: no devices → early return, pass 2 is never executed.
	mock.ExpectQuery(`SELECT d.id, d.name, d.notes, d.created_at, d.updated_at`).
		WillReturnRows(sqlmock.NewRows(append(deviceCols, "mac_address")))

	result, err := NewDeviceRepository(db).AllDevicesWithSignals(context.Background())

	require.NoError(t, err)
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_AllDevicesWithSignals_WithData(t *testing.T) {
	db, mock := newMockDB(t)
	deviceID := uuid.New()
	now := time.Now().UTC()
	mac := "00:11:22:33:44:55"
	mdnsName := "router.local"

	// Pass 1: one device with one MAC.
	mock.ExpectQuery(`SELECT d.id, d.name, d.notes, d.created_at, d.updated_at`).
		WillReturnRows(sqlmock.NewRows(append(deviceCols, "mac_address")).
			AddRow(deviceID, "Router", nil, now, now, mac))

	// Pass 2: one name for the same device.
	mock.ExpectQuery(`SELECT device_id, name, source FROM device_known_names`).
		WillReturnRows(sqlmock.NewRows([]string{"device_id", "name", "source"}).
			AddRow(deviceID, mdnsName, "mdns"))

	result, err := NewDeviceRepository(db).AllDevicesWithSignals(context.Background())

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Router", result[0].Device.Name)
	assert.Equal(t, []string{mac}, result[0].KnownMACs)
	require.Len(t, result[0].KnownNames, 1)
	assert.Equal(t, mdnsName, result[0].KnownNames[0].Name)
	assert.Equal(t, "mdns", result[0].KnownNames[0].Source)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── FindDeviceByMAC ───────────────────────────────────────────────────────

func TestDeviceRepository_FindDeviceByMAC_Found(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery(`SELECT d.id, d.name, d.notes, d.created_at, d.updated_at`).
		WithArgs("00:11:22:33:44:55").
		WillReturnRows(makeDeviceRow(id, "Router"))

	device, err := NewDeviceRepository(db).FindDeviceByMAC(context.Background(), "00:11:22:33:44:55")

	require.NoError(t, err)
	require.NotNil(t, device)
	assert.Equal(t, id, device.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRepository_FindDeviceByMAC_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT d.id, d.name, d.notes, d.created_at, d.updated_at`).
		WillReturnRows(sqlmock.NewRows(deviceCols)) // empty

	device, err := NewDeviceRepository(db).FindDeviceByMAC(context.Background(), "00:ff:ff:ff:ff:ff")

	require.NoError(t, err)
	assert.Nil(t, device, "no match should return nil, nil")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── FindDevicesByName ─────────────────────────────────────────────────────

func TestDeviceRepository_FindDevicesByName_Results(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery(`SELECT d.id, d.name, d.notes, d.created_at, d.updated_at`).
		WithArgs("router.local", "mdns").
		WillReturnRows(makeDeviceRow(id, "Router"))

	devices, err := NewDeviceRepository(db).FindDevicesByName(context.Background(), "router.local", "mdns")

	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, id, devices[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetSuggestionsForDiscovery ────────────────────────────────────────────

func TestDeviceRepository_GetSuggestionsForDiscovery_Empty(t *testing.T) {
	db, _ := newMockDB(t)

	result, err := NewDeviceRepository(db).GetSuggestionsForDiscovery(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, result)
	// No DB calls expected — early return for empty slice.
}

func TestDeviceRepository_GetSuggestionsForDiscovery_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	hostID := uuid.New()

	mock.ExpectQuery(`SELECT id, host_id`).
		WillReturnError(fmt.Errorf("connection reset"))

	_, err := NewDeviceRepository(db).GetSuggestionsForDiscovery(context.Background(), []uuid.UUID{hostID})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
