// Package db — unit tests for SettingsRepository using sqlmock.
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

var settingCols = []string{"key", "value", "description", "type", "updated_at"}

func makeSettingRow(key, value, desc, typ string) *sqlmock.Rows {
	return sqlmock.NewRows(settingCols).
		AddRow(key, value, desc, typ, time.Now().UTC())
}

// ── NewSettingsRepository ──────────────────────────────────────────────────────

func TestSettingsRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewSettingsRepository(db)
	require.NotNil(t, repo)
}

// ── ListSettings ───────────────────────────────────────────────────────────────

func TestSettingsRepository_ListSettings_Empty(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	settings, err := repo.ListSettings(context.Background())

	require.NoError(t, err)
	assert.Empty(t, settings)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_ListSettings_Multiple(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows(settingCols).
		AddRow("scan.max_concurrent", "5", "Max concurrent scans", "int", time.Now().UTC()).
		AddRow("scan.default_timing", "3", "Default timing", "int", time.Now().UTC())
	mock.ExpectQuery("SELECT key").WillReturnRows(rows)

	repo := NewSettingsRepository(db)
	settings, err := repo.ListSettings(context.Background())

	require.NoError(t, err)
	require.Len(t, settings, 2)
	assert.Equal(t, "scan.max_concurrent", settings[0].Key)
	assert.Equal(t, "5", settings[0].Value)
	assert.Equal(t, "scan.default_timing", settings[1].Key)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_ListSettings_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").WillReturnError(fmt.Errorf("connection reset"))

	repo := NewSettingsRepository(db)
	_, err := repo.ListSettings(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_ListSettings_ScanError(t *testing.T) {
	db, mock := newMockDB(t)
	// Return a row with wrong column count to trigger a scan error.
	rows := sqlmock.NewRows([]string{"key"}).AddRow("only-key")
	mock.ExpectQuery("SELECT key").WillReturnRows(rows)

	repo := NewSettingsRepository(db)
	_, err := repo.ListSettings(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetSetting ─────────────────────────────────────────────────────────────────

func TestSettingsRepository_GetSetting_Found(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.max_concurrent").
		WillReturnRows(makeSettingRow("scan.max_concurrent", "5", "Max concurrent scans", "int"))

	repo := NewSettingsRepository(db)
	s, err := repo.GetSetting(context.Background(), "scan.max_concurrent")

	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "scan.max_concurrent", s.Key)
	assert.Equal(t, "5", s.Value)
	assert.Equal(t, "int", s.Type)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetSetting_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	_, err := repo.GetSetting(context.Background(), "nonexistent")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetSetting_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.max_concurrent").
		WillReturnError(fmt.Errorf("connection refused"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetSetting(context.Background(), "scan.max_concurrent")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetStringSetting ───────────────────────────────────────────────────────────

func TestSettingsRepository_GetStringSetting_ReturnsUnquotedValue(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.label").
		WillReturnRows(makeSettingRow("scan.label", `"fast"`, "", "string"))

	repo := NewSettingsRepository(db)
	val, err := repo.GetStringSetting(context.Background(), "scan.label")

	require.NoError(t, err)
	assert.Equal(t, "fast", val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_ReturnsErrNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("missing.key").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSetting(context.Background(), "missing.key")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_ReturnsErrorOnWrongType(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.label").
		WillReturnRows(makeSettingRow("scan.label", `123`, "", "int"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSetting(context.Background(), "scan.label")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a JSON string")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSetting_PropagatesDBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.label").
		WillReturnError(fmt.Errorf("conn error"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSetting(context.Background(), "scan.label")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetIntSetting ──────────────────────────────────────────────────────────────

func TestSettingsRepository_GetIntSetting_ReturnsInteger(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.max_concurrent").
		WillReturnRows(makeSettingRow("scan.max_concurrent", `5`, "", "int"))

	repo := NewSettingsRepository(db)
	val, err := repo.GetIntSetting(context.Background(), "scan.max_concurrent")

	require.NoError(t, err)
	assert.Equal(t, 5, val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetIntSetting_ReturnsErrNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("missing.key").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	_, err := repo.GetIntSetting(context.Background(), "missing.key")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetIntSetting_ReturnsErrorOnWrongType(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.max_concurrent").
		WillReturnRows(makeSettingRow("scan.max_concurrent", `"not-a-number"`, "", "string"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetIntSetting(context.Background(), "scan.max_concurrent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a JSON integer")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetIntSetting_PropagatesDBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("scan.max_concurrent").
		WillReturnError(fmt.Errorf("conn error"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetIntSetting(context.Background(), "scan.max_concurrent")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetBoolSetting ─────────────────────────────────────────────────────────────

func TestSettingsRepository_GetBoolSetting_ReturnsBoolean(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("notifications.scan_complete").
		WillReturnRows(makeSettingRow("notifications.scan_complete", `true`, "", "bool"))

	repo := NewSettingsRepository(db)
	val, err := repo.GetBoolSetting(context.Background(), "notifications.scan_complete")

	require.NoError(t, err)
	assert.True(t, val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetBoolSetting_ReturnsErrNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("missing.key").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	_, err := repo.GetBoolSetting(context.Background(), "missing.key")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetBoolSetting_ReturnsErrorOnWrongType(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("notifications.scan_complete").
		WillReturnRows(makeSettingRow("notifications.scan_complete", `"yes"`, "", "string"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetBoolSetting(context.Background(), "notifications.scan_complete")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a JSON boolean")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetBoolSetting_PropagatesDBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("notifications.scan_complete").
		WillReturnError(fmt.Errorf("conn error"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetBoolSetting(context.Background(), "notifications.scan_complete")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetStringSliceSetting ──────────────────────────────────────────────────────

func TestSettingsRepository_GetStringSliceSetting_ReturnsSlice(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("discovery.methods").
		WillReturnRows(makeSettingRow("discovery.methods", `["icmp","arp"]`, "", "string[]"))

	repo := NewSettingsRepository(db)
	val, err := repo.GetStringSliceSetting(context.Background(), "discovery.methods")

	require.NoError(t, err)
	assert.Equal(t, []string{"icmp", "arp"}, val)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSliceSetting_ReturnsErrNotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("missing.key").
		WillReturnRows(sqlmock.NewRows(settingCols))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSliceSetting(context.Background(), "missing.key")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSliceSetting_ReturnsErrorOnWrongType(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("discovery.methods").
		WillReturnRows(makeSettingRow("discovery.methods", `"single"`, "", "string"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSliceSetting(context.Background(), "discovery.methods")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a JSON string array")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_GetStringSliceSetting_PropagatesDBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT key").
		WithArgs("discovery.methods").
		WillReturnError(fmt.Errorf("conn error"))

	repo := NewSettingsRepository(db)
	_, err := repo.GetStringSliceSetting(context.Background(), "discovery.methods")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── SetSetting ─────────────────────────────────────────────────────────────────

func TestSettingsRepository_SetSetting_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE settings").
		WithArgs("scan.max_concurrent", "10").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := NewSettingsRepository(db)
	err := repo.SetSetting(context.Background(), "scan.max_concurrent", "10")

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_SetSetting_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE settings").
		WithArgs("nonexistent", "true").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	repo := NewSettingsRepository(db)
	err := repo.SetSetting(context.Background(), "nonexistent", "true")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsRepository_SetSetting_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("UPDATE settings").
		WillReturnError(fmt.Errorf("connection reset"))

	repo := NewSettingsRepository(db)
	err := repo.SetSetting(context.Background(), "scan.max_concurrent", "5")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
