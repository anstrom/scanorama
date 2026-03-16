// Package profiles provides unit tests using sqlmock for the Manager methods
// that interact with the database. These tests run without a real PostgreSQL
// instance and complement the integration tests in profiles_test.go.
package profiles

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// profileColumns lists the SELECT column order used in all ScanProfile queries.
var profileColumns = []string{
	"id", "name", "description", "os_family", "os_pattern",
	"ports", "scan_type", "timing", "scripts", "options",
	"priority", "built_in", "created_at", "updated_at",
}

// newMockManager creates a Manager backed by a sqlmock database.
// It returns the manager, the mock controller, and a cleanup function.
func newMockManager(t *testing.T) (*Manager, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	mgr := NewManager(wrappedDB)
	cleanup := func() { _ = sqlDB.Close() }

	return mgr, mock, cleanup
}

// sampleProfile returns a fully populated ScanProfile for use in tests.
func sampleProfile(id, name string) *db.ScanProfile {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	return &db.ScanProfile{
		ID:          id,
		Name:        name,
		Description: "Test profile",
		OSFamily:    pq.StringArray{"linux"},
		OSPattern:   pq.StringArray{".*Ubuntu.*"},
		Ports:       "22,80,443",
		ScanType:    db.ScanTypeConnect,
		Timing:      db.ScanTimingNormal,
		Scripts:     pq.StringArray{"banner"},
		Options:     db.JSONB(`{"retries":2}`),
		Priority:    50,
		BuiltIn:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// addProfileRow appends a ScanProfile as a sqlmock row.
func addProfileRow(rows *sqlmock.Rows, p *db.ScanProfile) {
	osFamilyVal, _ := p.OSFamily.Value()
	osPatternVal, _ := p.OSPattern.Value()
	scriptsVal, _ := p.Scripts.Value()
	optionsVal, _ := p.Options.Value()

	rows.AddRow(
		p.ID, p.Name, p.Description,
		osFamilyVal, osPatternVal,
		p.Ports, p.ScanType, p.Timing,
		scriptsVal, optionsVal,
		p.Priority, p.BuiltIn,
		p.CreatedAt, p.UpdatedAt,
	)
}

// ---------------------------------------------------------------------------
// GetAll
// ---------------------------------------------------------------------------

func TestGetAll_Mock_ReturnsProfiles(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p1 := sampleProfile("id-1", "Profile One")
	p2 := sampleProfile("id-2", "Profile Two")
	p2.OSFamily = pq.StringArray{"windows"}

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, p1)
	addProfileRow(rows, p2)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetAll(ctx)

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, p1.ID, got[0].ID)
	assert.Equal(t, p1.Name, got[0].Name)
	assert.Equal(t, p1.OSFamily, got[0].OSFamily)
	assert.Equal(t, p2.ID, got[1].ID)
	assert.Equal(t, p2.OSFamily, got[1].OSFamily)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAll_Mock_EmptyResult(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	rows := sqlmock.NewRows(profileColumns)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetAll(ctx)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAll_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	dbErr := errors.New("connection lost")
	mock.ExpectQuery("SELECT").WillReturnError(dbErr)

	ctx := context.Background()
	got, err := mgr.GetAll(ctx)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to query profiles")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAll_Mock_ScanError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// Return a row with too few columns to force a scan error.
	badRows := sqlmock.NewRows([]string{"id"}).AddRow("only-one-col")
	mock.ExpectQuery("SELECT").WillReturnRows(badRows)

	ctx := context.Background()
	got, err := mgr.GetAll(ctx)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetByID
// ---------------------------------------------------------------------------

func TestGetByID_Mock_ReturnsProfile(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("abc-123", "My Profile")
	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, p)

	mock.ExpectQuery("SELECT").
		WithArgs("abc-123").
		WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetByID(ctx, "abc-123")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.Ports, got.Ports)
	assert.Equal(t, p.OSFamily, got.OSFamily)
	assert.Equal(t, p.Priority, got.Priority)
	assert.Equal(t, p.BuiltIn, got.BuiltIn)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByID_Mock_NotFound(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// Empty rows set causes QueryRowContext.Scan to return sql.ErrNoRows.
	rows := sqlmock.NewRows(profileColumns)
	mock.ExpectQuery("SELECT").
		WithArgs("missing-id").
		WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetByID(ctx, "missing-id")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "missing-id")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByID_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WithArgs("bad-id").
		WillReturnError(errors.New("query error"))

	ctx := context.Background()
	got, err := mgr.GetByID(ctx, "bad-id")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to get profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByID_Mock_WrapsErrNoRows(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WithArgs("no-rows-id").
		WillReturnError(sql.ErrNoRows)

	ctx := context.Background()
	got, err := mgr.GetByID(ctx, "no-rows-id")

	require.Error(t, err)
	assert.Nil(t, got)
	// The underlying cause must be sql.ErrNoRows.
	assert.True(t, errors.Is(err, sql.ErrNoRows))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetByOSFamily
// ---------------------------------------------------------------------------

func TestGetByOSFamily_Mock_ReturnsMatchingProfiles(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("linux-profile", "Linux Profile")
	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, p)

	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetByOSFamily(ctx, "linux")

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, p.ID, got[0].ID)
	assert.Equal(t, pq.StringArray{"linux"}, got[0].OSFamily)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByOSFamily_Mock_EmptyResult(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	rows := sqlmock.NewRows(profileColumns)
	mock.ExpectQuery("SELECT").
		WithArgs("amiga").
		WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetByOSFamily(ctx, "amiga")

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByOSFamily_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnError(errors.New("timeout"))

	ctx := context.Background()
	got, err := mgr.GetByOSFamily(ctx, "linux")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to query profiles by OS family")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByOSFamily_Mock_ScanError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// Too few columns forces a scan error on the first row.
	badRows := sqlmock.NewRows([]string{"id"}).AddRow("short")
	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnRows(badRows)

	ctx := context.Background()
	got, err := mgr.GetByOSFamily(ctx, "linux")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreate_Mock_Success(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("new-profile", "New Profile")
	mock.ExpectExec("INSERT").
		WithArgs(
			p.ID, p.Name, p.Description,
			sqlmock.AnyArg(), // OSFamily (pq array serialized to string)
			sqlmock.AnyArg(), // OSPattern (pq array serialized to string)
			p.Ports, p.ScanType, p.Timing,
			sqlmock.AnyArg(), // Scripts (pq array serialized to string)
			sqlmock.AnyArg(), // Options (JSONB []byte)
			p.Priority, p.BuiltIn,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.Create(ctx, p)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreate_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("dup-profile", "Duplicate")
	mock.ExpectExec("INSERT").
		WillReturnError(errors.New("unique constraint violation"))

	ctx := context.Background()
	err := mgr.Create(ctx, p)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCreate_Mock_NilOptions(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("nil-opts", "No Options Profile")
	p.Options = nil

	mock.ExpectExec("INSERT").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.Create(ctx, p)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestUpdate_Mock_Success(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("edit-id", "Updated Name")
	mock.ExpectExec("UPDATE").
		WithArgs(
			p.ID, p.Name, p.Description,
			sqlmock.AnyArg(), // OSFamily
			sqlmock.AnyArg(), // OSPattern
			p.Ports, p.ScanType, p.Timing,
			sqlmock.AnyArg(), // Scripts
			sqlmock.AnyArg(), // Options
			p.Priority,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.Update(ctx, p)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdate_Mock_NotFoundOrBuiltIn(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("builtin-id", "Built-in Profile")
	p.BuiltIn = true
	// Zero rows affected simulates the WHERE built_in = false predicate
	// blocking the update.
	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ctx := context.Background()
	err := mgr.Update(ctx, p)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found or is built-in")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdate_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("err-id", "Error Profile")
	mock.ExpectExec("UPDATE").
		WillReturnError(errors.New("db gone away"))

	ctx := context.Background()
	err := mgr.Update(ctx, p)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdate_Mock_RowsAffectedError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("rows-err-id", "Rows Err")
	// NewErrorResult makes RowsAffected() itself return an error.
	mock.ExpectExec("UPDATE").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

	ctx := context.Background()
	err := mgr.Update(ctx, p)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get affected rows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete_Mock_Success(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectExec("DELETE").
		WithArgs("del-id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.Delete(ctx, "del-id")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDelete_Mock_NotFoundOrBuiltIn(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectExec("DELETE").
		WithArgs("missing-id").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ctx := context.Background()
	err := mgr.Delete(ctx, "missing-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found or is built-in")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDelete_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectExec("DELETE").
		WithArgs("bad-id").
		WillReturnError(errors.New("lock timeout"))

	ctx := context.Background()
	err := mgr.Delete(ctx, "bad-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDelete_Mock_RowsAffectedError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// NewErrorResult causes RowsAffected() to return an error.
	mock.ExpectExec("DELETE").
		WithArgs("err-rows-id").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

	ctx := context.Background()
	err := mgr.Delete(ctx, "err-rows-id")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get affected rows")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetProfileStats
// ---------------------------------------------------------------------------

func TestGetProfileStats_Mock_ReturnsStats(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"profile_id", "usage_count"}).
		AddRow("profile-a", 42).
		AddRow("profile-b", 7).
		AddRow("none", 3)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := context.Background()
	stats, err := mgr.GetProfileStats(ctx)

	require.NoError(t, err)
	require.Len(t, stats, 3)
	assert.Equal(t, 42, stats["profile-a"])
	assert.Equal(t, 7, stats["profile-b"])
	assert.Equal(t, 3, stats["none"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetProfileStats_Mock_Empty(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"profile_id", "usage_count"})
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := context.Background()
	stats, err := mgr.GetProfileStats(ctx)

	require.NoError(t, err)
	assert.Empty(t, stats)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetProfileStats_Mock_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").WillReturnError(errors.New("connection refused"))

	ctx := context.Background()
	stats, err := mgr.GetProfileStats(ctx)

	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "failed to get profile stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetProfileStats_Mock_ScanError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// One column instead of two forces a scan error on the first row.
	badRows := sqlmock.NewRows([]string{"only_col"}).AddRow("oops")
	mock.ExpectQuery("SELECT").WillReturnRows(badRows)

	ctx := context.Background()
	stats, err := mgr.GetProfileStats(ctx)

	require.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "failed to scan stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// CloneProfile
// ---------------------------------------------------------------------------

func TestCloneProfile_Mock_Success(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	source := sampleProfile("source-id", "Source Profile")
	source.BuiltIn = true // source is built-in; clone must not be

	// First: GetByID issues a SELECT.
	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, source)
	mock.ExpectQuery("SELECT").
		WithArgs("source-id").
		WillReturnRows(rows)

	// Second: Create issues an INSERT.
	mock.ExpectExec("INSERT").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.CloneProfile(ctx, "source-id", "clone-id", "Cloned Profile")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCloneProfile_Mock_SourceNotFound(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// GetByID returns no rows → sql.ErrNoRows.
	rows := sqlmock.NewRows(profileColumns)
	mock.ExpectQuery("SELECT").
		WithArgs("ghost-id").
		WillReturnRows(rows)

	ctx := context.Background()
	err := mgr.CloneProfile(ctx, "ghost-id", "new-id", "New Name")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get source profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCloneProfile_Mock_CreateFails(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	source := sampleProfile("src-2", "Source 2")

	// GetByID succeeds.
	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, source)
	mock.ExpectQuery("SELECT").
		WithArgs("src-2").
		WillReturnRows(rows)

	// Create fails.
	mock.ExpectExec("INSERT").
		WillReturnError(errors.New("unique violation"))

	ctx := context.Background()
	err := mgr.CloneProfile(ctx, "src-2", "clone-2", "Clone 2")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create profile")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCloneProfile_Mock_CloneIsNeverBuiltIn(t *testing.T) {
	// Verify that even when cloning a built-in profile, the clone has BuiltIn=false.
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	source := sampleProfile("builtin-src", "Built-In Profile")
	source.BuiltIn = true

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, source)
	mock.ExpectQuery("SELECT").
		WithArgs("builtin-src").
		WillReturnRows(rows)

	// The 12th positional arg in the INSERT is BuiltIn — must be false.
	mock.ExpectExec("INSERT").
		WithArgs(
			"clone-builtin", "Clone Name", source.Description,
			sqlmock.AnyArg(), sqlmock.AnyArg(), // OSFamily, OSPattern
			source.Ports, source.ScanType, source.Timing,
			sqlmock.AnyArg(), sqlmock.AnyArg(), // Scripts, Options
			source.Priority,
			false, // BuiltIn must be false for the clone
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	err := mgr.CloneProfile(ctx, "builtin-src", "clone-builtin", "Clone Name")

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// SelectBestProfile
// ---------------------------------------------------------------------------

func TestSelectBestProfile_Mock_WithOSInfo_PicksBestScore(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// Two Linux profiles with different priorities and patterns.
	linuxBest := sampleProfile("linux-best", "Linux Best")
	linuxBest.OSFamily = pq.StringArray{"linux"}
	linuxBest.OSPattern = pq.StringArray{".*Ubuntu.*"}
	linuxBest.Priority = 80

	linuxGeneric := sampleProfile("linux-generic", "Linux Generic")
	linuxGeneric.OSFamily = pq.StringArray{"linux"}
	linuxGeneric.OSPattern = pq.StringArray{}
	linuxGeneric.Priority = 40

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, linuxBest)
	addProfileRow(rows, linuxGeneric)

	// SelectBestProfile calls GetByOSFamily because the host has an OS family.
	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnRows(rows)

	linuxFamily := "linux"
	linuxName := "Ubuntu 22.04"
	linuxConfidence := 90
	host := &db.Host{
		OSFamily:     &linuxFamily,
		OSName:       &linuxName,
		OSConfidence: &linuxConfidence,
	}

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "linux-best", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectBestProfile_Mock_NoOSInfo_UsesGetAll(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// A profile with an empty OSFamily scores 10 when there is no OS info.
	generic := sampleProfile("generic-profile", "Generic")
	generic.OSFamily = pq.StringArray{}
	generic.Priority = 10

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, generic)

	// No OS info on the host → SelectBestProfile calls GetAll.
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	host := &db.Host{} // zero value — no OS info

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "generic-profile", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectBestProfile_Mock_NoProfiles_ReturnsError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	// GetByOSFamily returns empty — no profiles for this family.
	// SelectBestProfile sees len(profiles)==0 and returns an error immediately,
	// without attempting a GetByID("generic-default") fallback.
	emptyRows := sqlmock.NewRows(profileColumns)
	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnRows(emptyRows)

	linuxFamily := "linux"
	host := &db.Host{OSFamily: &linuxFamily}

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "no profiles available")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectBestProfile_Mock_GetByOSFamily_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").
		WithArgs("linux").
		WillReturnError(errors.New("db error"))

	linuxFamily := "linux"
	host := &db.Host{OSFamily: &linuxFamily}

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectBestProfile_Mock_GetAll_DBError(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	mock.ExpectQuery("SELECT").WillReturnError(errors.New("connection error"))

	host := &db.Host{} // no OS family → GetAll

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectBestProfile_Mock_AllScoreZero_FallsBackToGenericDefault(t *testing.T) {
	// When every profile scores 0 (bestProfile stays nil), SelectBestProfile
	// falls back to GetByID("generic-default").
	// With no OS info: profiles that have a non-empty OSFamily score 0;
	// only profiles with an empty OSFamily score 10.
	// So we return a single profile whose OSFamily is non-empty → score 0 → fallback.
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p1 := sampleProfile("os-profile-1", "OS Profile 1")
	p1.OSFamily = pq.StringArray{"freebsd"}
	p1.Priority = 0

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, p1)

	// GetAll triggered (no host OS info).
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	// Fallback: GetByID("generic-default") — succeeds.
	defaultProfile := sampleProfile("generic-default", "Generic Default")
	defaultProfile.OSFamily = pq.StringArray{}
	defaultRows := sqlmock.NewRows(profileColumns)
	addProfileRow(defaultRows, defaultProfile)
	mock.ExpectQuery("SELECT").
		WithArgs("generic-default").
		WillReturnRows(defaultRows)

	host := &db.Host{} // no OS info → GetAll

	ctx := context.Background()
	got, err := mgr.SelectBestProfile(ctx, host)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "generic-default", got.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// Composite: Create then GetByID round-trip
// ---------------------------------------------------------------------------

func TestCreate_Then_GetByID_Mock(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	p := sampleProfile("round-trip", "Round Trip")

	mock.ExpectExec("INSERT").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx := context.Background()
	require.NoError(t, mgr.Create(ctx, p))

	rows := sqlmock.NewRows(profileColumns)
	addProfileRow(rows, p)
	mock.ExpectQuery("SELECT").
		WithArgs("round-trip").
		WillReturnRows(rows)

	got, err := mgr.GetByID(ctx, "round-trip")
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// GetAll — NULL array columns
// ---------------------------------------------------------------------------

// TestGetAll_Mock_NullArrayColumns verifies that profiles whose os_family,
// os_pattern, and scripts columns are NULL in the database are scanned
// without error (pq.StringArray.Scan(nil) yields an empty/nil slice).
func TestGetAll_Mock_NullArrayColumns(t *testing.T) {
	mgr, mock, cleanup := newMockManager(t)
	defer cleanup()

	now := time.Now()
	rows := sqlmock.NewRows(profileColumns).
		AddRow(
			"null-arrays", "Null Array Profile", "desc",
			nil, nil,
			"22", db.ScanTypeConnect, db.ScanTimingNormal,
			nil,
			nil,
			10, false, now, now,
		)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx := context.Background()
	got, err := mgr.GetAll(ctx)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "null-arrays", got[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}
