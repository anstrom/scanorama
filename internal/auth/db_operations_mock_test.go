// Package auth provides mock-based unit tests for the APIKeyRepository methods.
// These tests use go-sqlmock to simulate database interactions without requiring
// a live PostgreSQL instance, and complement the integration tests in
// db_operations_test.go.
package auth

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// newMockRepo creates an APIKeyRepository backed by a sqlmock database.
// It returns the repository, the mock controller, and a cleanup function.
func newMockRepo(t *testing.T) (*APIKeyRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := NewAPIKeyRepository(wrappedDB)
	cleanup := func() { _ = sqlDB.Close() }
	return repo, mock, cleanup
}

// apiKeyColumns lists the SELECT column order for most queries (no key_hash).
var apiKeyColumns = []string{
	"id", "name", "key_prefix", "created_at", "updated_at",
	"last_used_at", "expires_at", "is_active", "usage_count",
	"permissions", "created_by", "notes",
}

// apiKeyColumnsWithHash lists the SELECT column order for ValidateAPIKey
// (includes key_hash between key_prefix and created_at).
var apiKeyColumnsWithHash = []string{
	"id", "name", "key_prefix", "key_hash", "created_at", "updated_at",
	"last_used_at", "expires_at", "is_active", "usage_count",
	"permissions", "created_by", "notes",
}

// ---------------------------------------------------------------------------
// TestMockNewAPIKeyRepository
// ---------------------------------------------------------------------------

func TestMockNewAPIKeyRepository(t *testing.T) {
	repo, _, cleanup := newMockRepo(t)
	defer cleanup()

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.db)
}

// ---------------------------------------------------------------------------
// TestMockCreateAPIKey
// ---------------------------------------------------------------------------

func TestMockCreateAPIKey(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	generatedKey, err := GenerateAPIKey("Mock Test Key")
	require.NoError(t, err)

	now := time.Now().UTC()
	returnedID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	mock.ExpectQuery("INSERT INTO api_keys").
		WillReturnRows(
			sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
				AddRow(returnedID, now, now),
		)

	keyInfo, err := repo.CreateAPIKey(generatedKey)

	require.NoError(t, err)
	require.NotNil(t, keyInfo)
	assert.Equal(t, returnedID, keyInfo.ID)
	assert.Equal(t, generatedKey.KeyInfo.Name, keyInfo.Name)
	assert.Equal(t, generatedKey.KeyPrefix, keyInfo.KeyPrefix)
	assert.True(t, keyInfo.IsActive)
	assert.Equal(t, 0, keyInfo.UsageCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockCreateAPIKey_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	generatedKey, err := GenerateAPIKey("Mock Error Key")
	require.NoError(t, err)

	mock.ExpectQuery("INSERT INTO api_keys").
		WillReturnError(errors.New("connection refused"))

	keyInfo, err := repo.CreateAPIKey(generatedKey)

	assert.Error(t, err)
	assert.Nil(t, keyInfo)
	assert.Contains(t, err.Error(), "failed to store API key")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockListAPIKeys
// ---------------------------------------------------------------------------

func TestMockListAPIKeys_ShowAll(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow(
			"id-1", "Key One", "sk_aaaaaaaa...", now, now,
			nil, nil, true, 5, []byte(`{"read":true}`), nil, "first key",
		).
		AddRow("id-2", "Key Two", "sk_bbbbbbbb...", now, now, nil, nil, false, 0, []byte(`{}`), nil, "")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	keys, err := repo.ListAPIKeys(true, true)

	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Equal(t, "id-1", keys[0].ID)
	assert.Equal(t, "Key One", keys[0].Name)
	assert.True(t, keys[0].IsActive)
	assert.Equal(t, true, keys[0].Permissions["read"])
	assert.Equal(t, "id-2", keys[1].ID)
	assert.False(t, keys[1].IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockListAPIKeys_ActiveOnly(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow("id-1", "Active Key", "sk_aaaaaaaa...", now, now, nil, nil, true, 0, []byte(`{}`), nil, "")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	keys, err := repo.ListAPIKeys(false, false)

	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.True(t, keys[0].IsActive)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockListAPIKeys_Empty(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(apiKeyColumns))

	keys, err := repo.ListAPIKeys(true, true)

	require.NoError(t, err)
	assert.Empty(t, keys)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockListAPIKeys_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("query failed"))

	keys, err := repo.ListAPIKeys(false, false)

	assert.Error(t, err)
	assert.Nil(t, keys)
	assert.Contains(t, err.Error(), "failed to query API keys")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockListAPIKeys_ShowExpiredNotInactive(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	past := time.Now().UTC().Add(-24 * time.Hour)
	now := time.Now().UTC()
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow("id-exp", "Expired Key", "sk_cccccccc...", past, past, nil, &past, true, 0, []byte(`{}`), nil, "")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	keys, err := repo.ListAPIKeys(true, false)

	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "id-exp", keys[0].ID)
	assert.True(t, keys[0].ExpiresAt.Before(now))
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockListAPIKeys_NullPermissions(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	// permissions column is nil/empty — should default to an empty map.
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow("id-1", "Null Perms Key", "sk_aaaaaaaa...", now, now, nil, nil, true, 0, []byte(nil), nil, "")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	keys, err := repo.ListAPIKeys(true, true)

	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].Permissions)
	assert.Empty(t, keys[0].Permissions)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockFindAPIKeyByIdentifier
// ---------------------------------------------------------------------------

func TestMockFindAPIKeyByIdentifier_ByUUID_Found(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	id := "12345678-1234-1234-1234-123456789abc"
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow(id, "UUID Key", "sk_aaaaaaaa...", now, now, nil, nil, true, 0, []byte(`{}`), nil, "")

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	key, err := repo.FindAPIKeyByIdentifier(id)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, id, key.ID)
	assert.Equal(t, "UUID Key", key.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockFindAPIKeyByIdentifier_ByPrefix_Found(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow("id-prefix", "Prefix Key", "sk_abcd1234...", now, now, nil, nil, true, 3, []byte(`{}`), nil, "note")

	// "sk_abcd" is not a valid UUID so FindAPIKeyByIdentifier uses the prefix path.
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	key, err := repo.FindAPIKeyByIdentifier("sk_abcd")

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "id-prefix", key.ID)
	assert.Equal(t, 3, key.UsageCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockFindAPIKeyByIdentifier_NotFound_UUID(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	id := "00000000-0000-0000-0000-000000000000"
	// Empty rows triggers sql.ErrNoRows on QueryRow().Scan().
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(apiKeyColumns))

	key, err := repo.FindAPIKeyByIdentifier(id)

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "API key not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockFindAPIKeyByIdentifier_NotFound_Prefix(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(apiKeyColumns))

	key, err := repo.FindAPIKeyByIdentifier("sk_notexist")

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "API key not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockFindAPIKeyByIdentifier_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("db timeout"))

	key, err := repo.FindAPIKeyByIdentifier("some-prefix")

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "failed to query API key")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockUpdateAPIKey
// ---------------------------------------------------------------------------

func TestMockUpdateAPIKey_Success(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	id := "12345678-1234-1234-1234-123456789abc"
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Expect the UPDATE exec.
	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect the follow-up SELECT from FindAPIKeyByIdentifier (UUID path).
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow(id, "Updated Name", "sk_aaaaaaaa...", now, now, nil, nil, true, 0, []byte(`{}`), nil, "")
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	updates := map[string]interface{}{"name": "Updated Name"}
	key, err := repo.UpdateAPIKey(id, updates)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "Updated Name", key.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockUpdateAPIKey_MultipleFields(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	id := "12345678-1234-1234-1234-123456789abc"
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(48 * time.Hour)

	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 1))

	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow(id, "Multi Update", "sk_aaaaaaaa...", now, now, nil, &expiresAt, true, 0, []byte(`{}`), nil, "new note")
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	updates := map[string]interface{}{
		"name":       "Multi Update",
		"notes":      "new note",
		"expires_at": expiresAt,
	}
	key, err := repo.UpdateAPIKey(id, updates)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "Multi Update", key.Name)
	assert.Equal(t, "new note", key.Notes)
	assert.NotNil(t, key.ExpiresAt)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockUpdateAPIKey_UnsupportedField(t *testing.T) {
	repo, _, cleanup := newMockRepo(t)
	defer cleanup()

	// Returns early before any DB call.
	updates := map[string]interface{}{"is_active": false}
	key, err := repo.UpdateAPIKey("some-id", updates)

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "unsupported update field")
}

func TestMockUpdateAPIKey_NotFound(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	// UPDATE affects 0 rows.
	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 0))

	updates := map[string]interface{}{"name": "Ghost"}
	key, err := repo.UpdateAPIKey("12345678-1234-1234-1234-123456789abc", updates)

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "API key not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockUpdateAPIKey_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnError(errors.New("update failed"))

	updates := map[string]interface{}{"notes": "new note"}
	key, err := repo.UpdateAPIKey("12345678-1234-1234-1234-123456789abc", updates)

	assert.Error(t, err)
	assert.Nil(t, key)
	assert.Contains(t, err.Error(), "failed to update API key")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockUpdateAPIKey_EmptyUpdates_CallsFind(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	id := "12345678-1234-1234-1234-123456789abc"
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	// Empty updates → delegates directly to FindAPIKeyByIdentifier; no UPDATE.
	rows := sqlmock.NewRows(apiKeyColumns).
		AddRow(id, "Unchanged", "sk_aaaaaaaa...", now, now, nil, nil, true, 0, []byte(`{}`), nil, "")
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	key, err := repo.UpdateAPIKey(id, map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, "Unchanged", key.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockRevokeAPIKey
// ---------------------------------------------------------------------------

func TestMockRevokeAPIKey_Success(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.RevokeAPIKey("12345678-1234-1234-1234-123456789abc")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockRevokeAPIKey_ByPrefix(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.RevokeAPIKey("sk_abcd1234")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockRevokeAPIKey_NotFound(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	// 0 rows affected → not found.
	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.RevokeAPIKey("nonexistent-prefix")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockRevokeAPIKey_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnError(errors.New("exec error"))

	err := repo.RevokeAPIKey("some-prefix")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to revoke API key")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockValidateAPIKey
// ---------------------------------------------------------------------------

// TestMockValidateAPIKey_InvalidFormat checks the early-return path before any
// DB interaction takes place.
func TestMockValidateAPIKey_InvalidFormat(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	// No DB calls expected.
	result, err := repo.ValidateAPIKey("not-a-valid-key")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid API key format")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockValidateAPIKey_EmptyKey exercises the empty-string early return.
func TestMockValidateAPIKey_EmptyKey(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	result, err := repo.ValidateAPIKey("")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockValidateAPIKey_NoRows tests the path where the DB returns an empty
// result set — all active keys were checked and none matched.
func TestMockValidateAPIKey_NoRows(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(apiKeyColumnsWithHash))

	// Generate a validly-formatted key that passes IsValidAPIKeyFormat.
	gen, err := GenerateAPIKey("throwaway")
	require.NoError(t, err)

	result, err := repo.ValidateAPIKey(gen.Key)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid API key")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockValidateAPIKey_DBError covers the failure path when the SELECT itself
// returns an error.
func TestMockValidateAPIKey_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("connection lost"))

	gen, err := GenerateAPIKey("error key")
	require.NoError(t, err)

	result, err := repo.ValidateAPIKey(gen.Key)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to query API keys for validation")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockValidateAPIKey_Match generates a real key+hash pair so that the
// bcrypt comparison inside ValidateAPIKey succeeds. The goroutine-launched
// updateLastUsed will fire an UPDATE; we disable ordered matching so sqlmock
// does not treat any unmatched expectation as a fatal failure.
func TestMockValidateAPIKey_Match(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()

	// Allow expectations to be matched out of order so the background
	// updateLastUsed goroutine does not cause a spurious failure.
	mock.MatchExpectationsInOrder(false)

	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := NewAPIKeyRepository(wrappedDB)

	// Generate a real key so we can produce the correct bcrypt hash.
	gen, err := GenerateAPIKey("Validated Key")
	require.NoError(t, err)

	hash, err := HashAPIKey(gen.Key)
	require.NoError(t, err)

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	keyID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	rows := sqlmock.NewRows(apiKeyColumnsWithHash).
		AddRow(
			keyID, "Validated Key", gen.KeyPrefix, hash,
			now, now, // created_at, updated_at
			nil, nil, // last_used_at, expires_at
			true, 0, // is_active, usage_count
			[]byte(`{}`),
			nil, // created_by
			"",  // notes
		)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	// Absorb the background updateLastUsed UPDATE (best-effort — may or may
	// not arrive before the test exits).
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := repo.ValidateAPIKey(gen.Key)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, keyID, result.ID)
	assert.Equal(t, "Validated Key", result.Name)

	// Give the goroutine a moment to fire before the mock DB is closed.
	time.Sleep(50 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// TestMockGetAPIKeyStats
// ---------------------------------------------------------------------------

func TestMockGetAPIKeyStats_Success(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	recentUse := time.Date(2024, 5, 30, 9, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"total_keys", "active_keys", "revoked_keys", "expired_keys",
		"used_keys", "avg_usage_count", "most_recent_use",
	}).AddRow(
		10, 7, 2, 1,
		5,
		sql.NullFloat64{Float64: 3.5, Valid: true},
		sql.NullTime{Time: recentUse, Valid: true},
	)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	stats, err := repo.GetAPIKeyStats()

	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 10, stats["total_keys"])
	assert.Equal(t, 7, stats["active_keys"])
	assert.Equal(t, 2, stats["revoked_keys"])
	assert.Equal(t, 1, stats["expired_keys"])
	assert.Equal(t, 5, stats["used_keys"])
	assert.InDelta(t, 3.5, stats["avg_usage_count"], 0.001)
	assert.Equal(t, recentUse, stats["most_recent_use"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockGetAPIKeyStats_NullAvgAndTime(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	// avg_usage_count and most_recent_use are NULL (no keys have ever been used).
	rows := sqlmock.NewRows([]string{
		"total_keys", "active_keys", "revoked_keys", "expired_keys",
		"used_keys", "avg_usage_count", "most_recent_use",
	}).AddRow(
		0, 0, 0, 0, 0,
		sql.NullFloat64{Valid: false},
		sql.NullTime{Valid: false},
	)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	stats, err := repo.GetAPIKeyStats()

	require.NoError(t, err)
	assert.Equal(t, 0, stats["total_keys"])
	_, hasAvg := stats["avg_usage_count"]
	_, hasRecent := stats["most_recent_use"]
	assert.False(t, hasAvg, "avg_usage_count should not be present when NULL")
	assert.False(t, hasRecent, "most_recent_use should not be present when NULL")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockGetAPIKeyStats_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("stats query failed"))

	stats, err := repo.GetAPIKeyStats()

	assert.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "failed to get API key statistics")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockCleanupExpiredKeys
// ---------------------------------------------------------------------------

func TestMockCleanupExpiredKeys_SomeExpired(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 3))

	n, err := repo.CleanupExpiredKeys()

	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockCleanupExpiredKeys_NoneExpired(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := repo.CleanupExpiredKeys()

	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMockCleanupExpiredKeys_DBError(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	mock.ExpectExec("UPDATE").
		WillReturnError(errors.New("cleanup failed"))

	n, err := repo.CleanupExpiredKeys()

	assert.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Contains(t, err.Error(), "failed to cleanup expired API keys")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// TestMockCheckConfigAPIKeys
// ---------------------------------------------------------------------------

// TestMockCheckConfigAPIKeys_Empty ensures no DB call is made for an empty slice.
func TestMockCheckConfigAPIKeys_Empty(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	missing, err := repo.CheckConfigAPIKeys([]string{})

	require.NoError(t, err)
	assert.Empty(t, missing)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockCheckConfigAPIKeys_InvalidFormat verifies that keys failing
// IsValidAPIKeyFormat are reported as missing without any DB call —
// ValidateAPIKey returns "invalid API key format" before querying.
func TestMockCheckConfigAPIKeys_InvalidFormat(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	configKeys := []string{"bad-key-1", "also-bad"}

	missing, err := repo.CheckConfigAPIKeys(configKeys)

	require.NoError(t, err)
	assert.Equal(t, configKeys, missing)
	// No DB interactions expected.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockCheckConfigAPIKeys_ValidFormatButNotInDB tests keys that pass the
// format check but are not found in the database (empty SELECT result).
func TestMockCheckConfigAPIKeys_ValidFormatButNotInDB(t *testing.T) {
	repo, mock, cleanup := newMockRepo(t)
	defer cleanup()

	gen1, err := GenerateAPIKey("Config Key 1")
	require.NoError(t, err)
	gen2, err := GenerateAPIKey("Config Key 2")
	require.NoError(t, err)

	// Each ValidateAPIKey call issues a SELECT that returns 0 rows → "invalid API key".
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(apiKeyColumnsWithHash))
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(apiKeyColumnsWithHash))

	missing, err := repo.CheckConfigAPIKeys([]string{gen1.Key, gen2.Key})

	require.NoError(t, err)
	assert.Len(t, missing, 2)
	assert.Contains(t, missing, gen1.Key)
	assert.Contains(t, missing, gen2.Key)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestMockCheckConfigAPIKeys_MixedResults tests a mix of a valid (found in DB)
// key and a missing key. The found key uses a real bcrypt hash so the
// comparison in ValidateAPIKey succeeds.
func TestMockCheckConfigAPIKeys_MixedResults(t *testing.T) {
	// Use out-of-order matching to absorb the background updateLastUsed goroutine.
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()

	mock.MatchExpectationsInOrder(false)

	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := NewAPIKeyRepository(wrappedDB)

	// gen1 will be "found" — provide a row with its real hash.
	gen1, err := GenerateAPIKey("Found Key")
	require.NoError(t, err)
	hash1, err := HashAPIKey(gen1.Key)
	require.NoError(t, err)

	// gen2 will be "missing" — return empty rows.
	gen2, err := GenerateAPIKey("Missing Key")
	require.NoError(t, err)

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	foundRows := sqlmock.NewRows(apiKeyColumnsWithHash).
		AddRow(
			"id-found", "Found Key", gen1.KeyPrefix, hash1,
			now, now,
			nil, nil,
			true, 0,
			[]byte(`{}`),
			nil, "",
		)
	missingRows := sqlmock.NewRows(apiKeyColumnsWithHash)

	mock.ExpectQuery("SELECT").WillReturnRows(foundRows)
	mock.ExpectQuery("SELECT").WillReturnRows(missingRows)
	// Absorb the background updateLastUsed fired after gen1 validates successfully.
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))

	missing, err := repo.CheckConfigAPIKeys([]string{gen1.Key, gen2.Key})

	require.NoError(t, err)
	assert.Len(t, missing, 1)
	assert.Equal(t, gen2.Key, missing[0])

	// Give the goroutine a moment to complete before the mock DB is closed.
	time.Sleep(50 * time.Millisecond)
}
