// Package db provides unit tests for scan profile database operations.
package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetProfile ────────────────────────────────────────────────────────────────

func TestGetProfile_Unit(t *testing.T) {
	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)

		_, err := NewProfileRepository(db).GetProfile(context.Background(), "test-profile")

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))

		_, err := NewProfileRepository(db).GetProfile(context.Background(), "test-profile")

		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── DeleteProfile ─────────────────────────────────────────────────────────────

func TestDeleteProfile_Unit(t *testing.T) {
	const profileID = "test-profile"

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// Existence check: exists=true, builtIn=false.
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).
				AddRow(true, false))
		// In-use check: profile is not referenced by any scan jobs.
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// Existence check: exists=false → CodeNotFound returned immediately.
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).
				AddRow(false, false))

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("db error"))

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.Error(t, err)
	})
}

// ── ListProfiles ──────────────────────────────────────────────────────────────

func TestListProfiles_Unit(t *testing.T) {
	t.Run("count error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewProfileRepository(db).ListProfiles(context.Background(), ProfileFilters{}, 0, 10)

		require.Error(t, err)
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT").
			WillReturnError(fmt.Errorf("list failed"))

		_, _, err := NewProfileRepository(db).ListProfiles(context.Background(), ProfileFilters{}, 0, 10)

		require.Error(t, err)
	})
}

// ── CreateProfile ─────────────────────────────────────────────────────────────

var profileColumns = []string{
	"id", "name", "description", "os_family", "ports", "scan_type",
	"timing", "scripts", "options", "priority", "built_in",
	"created_at", "updated_at",
}

func profileRow(id, name string, now time.Time) []driver.Value {
	return []driver.Value{
		id, name,
		nil, nil, // description, os_family (NULL)
		"22,80", "connect",
		nil, nil, // timing, scripts (NULL)
		[]byte("{}"), // options
		0, false,     // priority, built_in
		now, now,
	}
}

func TestCreateProfile_Unit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		// CreateProfile uses ExecContext (no RETURNING); the profile struct is
		// built from the input, not from a DB row.
		mock.ExpectExec(`INSERT INTO scan_profiles`).
			WillReturnResult(sqlmock.NewResult(1, 1))

		profile, err := NewProfileRepository(db).CreateProfile(context.Background(),
			CreateProfileInput{Name: "Fast Connect", Ports: "22,80", ScanType: "connect"})
		require.NoError(t, err)
		require.NotNil(t, profile)
		assert.Equal(t, "Fast Connect", profile.Name)
		assert.Equal(t, "connect", profile.ScanType)
		assert.Equal(t, "fast-connect", profile.ID) // ID is derived from Name
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty name returns validation error without hitting DB", func(t *testing.T) {
		db, _ := newMockDB(t)
		_, err := NewProfileRepository(db).CreateProfile(context.Background(),
			CreateProfileInput{Ports: "80", ScanType: "connect"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "profile name is required")
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`INSERT INTO scan_profiles`).
			WillReturnError(fmt.Errorf("db error"))

		_, err := NewProfileRepository(db).CreateProfile(context.Background(),
			CreateProfileInput{Name: "x", Ports: "80", ScanType: "connect"})
		require.Error(t, err)
	})

	t.Run("duplicate name returns CodeConflict", func(t *testing.T) {
		db, mock := newMockDB(t)
		// pq.As checks for *pq.Error with the right code.
		mock.ExpectExec(`INSERT INTO scan_profiles`).
			WillReturnError(&pq.Error{Code: "23505"})

		_, err := NewProfileRepository(db).CreateProfile(context.Background(),
			CreateProfileInput{Name: "Dup", Ports: "22", ScanType: "connect"})
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeConflict))
	})
}

// ── GetProfile (success) ──────────────────────────────────────────────────────

func TestGetProfile_Unit_Success(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success returns profile with all fields", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(profileColumns).
				AddRow(profileRow("linux-server", "Linux Server", now)...))

		profile, err := NewProfileRepository(db).GetProfile(context.Background(), "linux-server")
		require.NoError(t, err)
		require.NotNil(t, profile)
		assert.Equal(t, "linux-server", profile.ID)
		assert.Equal(t, "Linux Server", profile.Name)
		assert.Equal(t, "22,80", profile.Ports)
		assert.Equal(t, "connect", profile.ScanType)
		assert.False(t, profile.BuiltIn)
	})
}

// ── UpdateProfile ─────────────────────────────────────────────────────────────

func TestUpdateProfile_Unit(t *testing.T) {
	const profileID = "test-profile"

	t.Run("begin tx error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin().WillReturnError(fmt.Errorf("tx error"))

		name := "new"
		_, err := NewProfileRepository(db).UpdateProfile(context.Background(), profileID,
			UpdateProfileInput{Name: &name})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(false, false))
		mock.ExpectRollback()

		name := "x"
		_, err := NewProfileRepository(db).UpdateProfile(context.Background(), profileID,
			UpdateProfileInput{Name: &name})
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("built-in profile returns CodeForbidden", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(true, true))
		mock.ExpectRollback()

		name := "x"
		_, err := NewProfileRepository(db).UpdateProfile(context.Background(), profileID,
			UpdateProfileInput{Name: &name})
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeForbidden))
	})

	t.Run("success — name updated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(true, false))
		// UpdateProfile uses ExecContext for the UPDATE (no RETURNING), then
		// calls GetProfile which issues a SELECT.
		mock.ExpectExec(`UPDATE scan_profiles`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows(profileColumns).
				AddRow(profileRow(profileID, "Renamed", time.Now().UTC())...))

		name := "Renamed"
		profile, err := NewProfileRepository(db).UpdateProfile(context.Background(), profileID,
			UpdateProfileInput{Name: &name})
		require.NoError(t, err)
		assert.Equal(t, "Renamed", profile.Name)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update exec error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(true, false))
		mock.ExpectExec(`UPDATE scan_profiles`).
			WillReturnError(fmt.Errorf("constraint"))
		mock.ExpectRollback()

		name := "x"
		_, err := NewProfileRepository(db).UpdateProfile(context.Background(), profileID,
			UpdateProfileInput{Name: &name})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update profile")
	})
}

// ── DeleteProfile (additional edge cases) ─────────────────────────────────────

func TestDeleteProfile_Unit_EdgeCases(t *testing.T) {
	const profileID = "test-profile"

	t.Run("built-in profile returns CodeForbidden", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(true, true))
		mock.ExpectRollback()

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeForbidden))
	})

	t.Run("in-use profile returns plain error", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).AddRow(true, false))
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)
		require.Error(t, err)
		// DeleteProfile returns a plain fmt.Errorf for in-use profiles (not a coded error).
		assert.Contains(t, err.Error(), "in use by scan jobs")
	})
}
