// Package services contains unit tests for the scan service business logic.
// These tests use a hand-rolled mock so no database or external dependencies
// are required.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// ─────────────────────────────────────────────────────────────────────────────
// Hand-rolled mock repository
// ─────────────────────────────────────────────────────────────────────────────

// mockScanRepo is a test double for scanRepository.  Each method is backed by
// an optional function field; when the field is nil the method returns sensible
// zero values so callers that don't care about a particular method need not
// configure it.
type mockScanRepo struct {
	listScans      func(context.Context, db.ScanFilters, int, int) ([]*db.Scan, int64, error)
	createScan     func(context.Context, db.CreateScanInput) (*db.Scan, error)
	getScan        func(context.Context, uuid.UUID) (*db.Scan, error)
	updateScan     func(context.Context, uuid.UUID, db.UpdateScanInput) (*db.Scan, error)
	deleteScan     func(context.Context, uuid.UUID) error
	startScan      func(context.Context, uuid.UUID) error
	stopScan       func(context.Context, uuid.UUID, ...string) error
	completeScan   func(context.Context, uuid.UUID) error
	getScanResults func(context.Context, uuid.UUID, int, int) ([]*db.ScanResult, int64, error)
	getScanSummary func(context.Context, uuid.UUID) (*db.ScanSummary, error)
	getProfile     func(context.Context, string) (*db.ScanProfile, error)
}

func (m *mockScanRepo) ListScans(
	ctx context.Context, filters db.ScanFilters, offset, limit int,
) ([]*db.Scan, int64, error) {
	if m.listScans != nil {
		return m.listScans(ctx, filters, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockScanRepo) CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error) {
	if m.createScan != nil {
		return m.createScan(ctx, input)
	}
	return nil, nil
}

func (m *mockScanRepo) GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error) {
	if m.getScan != nil {
		return m.getScan(ctx, id)
	}
	return nil, nil
}

func (m *mockScanRepo) UpdateScan(ctx context.Context, id uuid.UUID, input db.UpdateScanInput) (*db.Scan, error) {
	if m.updateScan != nil {
		return m.updateScan(ctx, id, input)
	}
	return nil, nil
}

func (m *mockScanRepo) DeleteScan(ctx context.Context, id uuid.UUID) error {
	if m.deleteScan != nil {
		return m.deleteScan(ctx, id)
	}
	return nil
}

func (m *mockScanRepo) StartScan(ctx context.Context, id uuid.UUID) error {
	if m.startScan != nil {
		return m.startScan(ctx, id)
	}
	return nil
}

func (m *mockScanRepo) StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error {
	if m.stopScan != nil {
		return m.stopScan(ctx, id, errMsg...)
	}
	return nil
}

func (m *mockScanRepo) CompleteScan(ctx context.Context, id uuid.UUID) error {
	if m.completeScan != nil {
		return m.completeScan(ctx, id)
	}
	return nil
}

func (m *mockScanRepo) GetScanResults(
	ctx context.Context, scanID uuid.UUID, offset, limit int,
) ([]*db.ScanResult, int64, error) {
	if m.getScanResults != nil {
		return m.getScanResults(ctx, scanID, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockScanRepo) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error) {
	if m.getScanSummary != nil {
		return m.getScanSummary(ctx, scanID)
	}
	return nil, nil
}

func (m *mockScanRepo) GetProfile(ctx context.Context, id string) (*db.ScanProfile, error) {
	if m.getProfile != nil {
		return m.getProfile(ctx, id)
	}
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// newTestScanService returns a ScanService wired to the supplied mock.
func newTestScanService(mock *mockScanRepo) *ScanService {
	return NewScanService(mock, slog.Default())
}

// validCreateInput returns a minimal, fully-valid CreateScanInput that passes
// all validation rules.
func validCreateInput() db.CreateScanInput {
	return db.CreateScanInput{
		Name:     "Test Scan",
		Targets:  []string{"192.168.1.0/24"},
		ScanType: "connect",
		Ports:    "80,443",
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NewScanService
// ─────────────────────────────────────────────────────────────────────────────

func TestNewScanService(t *testing.T) {
	mock := &mockScanRepo{}
	logger := slog.Default()

	svc := NewScanService(mock, logger)

	require.NotNil(t, svc)
	assert.Equal(t, mock, svc.repo)
	assert.Equal(t, logger, svc.logger)
}

// ─────────────────────────────────────────────────────────────────────────────
// DB
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_DB_ReturnsNilForMockRepo(t *testing.T) {
	svc := newTestScanService(&mockScanRepo{})
	assert.Nil(t, svc.DB(), "DB() must return nil when the repo is not *db.ScanRepository")
}

// ─────────────────────────────────────────────────────────────────────────────
// ListScans
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_ListScans_Success(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()
	expected := []*db.Scan{{ID: scanID}}

	mock := &mockScanRepo{
		listScans: func(_ context.Context, f db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error) {
			assert.Equal(t, "running", f.Status)
			assert.Equal(t, 10, offset)
			assert.Equal(t, 5, limit)
			return expected, 1, nil
		},
	}

	got, total, err := newTestScanService(mock).ListScans(ctx, db.ScanFilters{Status: "running"}, 10, 5)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.Equal(t, int64(1), total)
}

func TestScanService_ListScans_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("db failure")

	mock := &mockScanRepo{
		listScans: func(_ context.Context, _ db.ScanFilters, _, _ int) ([]*db.Scan, int64, error) {
			return nil, 0, wantErr
		},
	}

	_, _, err := newTestScanService(mock).ListScans(ctx, db.ScanFilters{}, 0, 10)
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// CreateScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_CreateScan_Valid_NoProfile(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()
	expected := &db.Scan{ID: scanID}

	mock := &mockScanRepo{
		createScan: func(_ context.Context, input db.CreateScanInput) (*db.Scan, error) {
			assert.Equal(t, "Test Scan", input.Name)
			return expected, nil
		},
	}

	got, err := newTestScanService(mock).CreateScan(ctx, validCreateInput())

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestScanService_CreateScan_Valid_WithProfile(t *testing.T) {
	ctx := context.Background()
	profileID := "profile-1"
	scanID := uuid.New()
	expected := &db.Scan{ID: scanID}

	mock := &mockScanRepo{
		getProfile: func(_ context.Context, id string) (*db.ScanProfile, error) {
			assert.Equal(t, profileID, id)
			return &db.ScanProfile{ID: profileID}, nil
		},
		createScan: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			return expected, nil
		},
	}

	input := validCreateInput()
	input.ProfileID = &profileID

	got, err := newTestScanService(mock).CreateScan(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestScanService_CreateScan_EmptyProfileID_SkipsProfileCheck(t *testing.T) {
	// A non-nil ProfileID that points to an empty string must skip the
	// GetProfile call entirely (the condition is: != nil && != "").
	ctx := context.Background()
	emptyProfile := ""
	scanID := uuid.New()
	expected := &db.Scan{ID: scanID}

	mock := &mockScanRepo{
		// getProfile intentionally omitted; if it were called the nil function
		// field would return (nil, nil) which would NOT cause a panic, but the
		// test would still verify the correct code path by asserting createScan
		// is reached.
		createScan: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			return expected, nil
		},
	}

	input := validCreateInput()
	input.ProfileID = &emptyProfile

	got, err := newTestScanService(mock).CreateScan(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestScanService_CreateScan_ValidationError(t *testing.T) {
	ctx := context.Background()
	input := validCreateInput()
	input.Name = "" // triggers validation failure before any repo call

	_, err := newTestScanService(&mockScanRepo{}).CreateScan(ctx, input)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation))
}

func TestScanService_CreateScan_ProfileNotFound(t *testing.T) {
	ctx := context.Background()
	profileID := "missing-profile"

	mock := &mockScanRepo{
		getProfile: func(_ context.Context, _ string) (*db.ScanProfile, error) {
			return nil, errors.ErrNotFound("profile")
		},
	}

	input := validCreateInput()
	input.ProfileID = &profileID

	_, err := newTestScanService(mock).CreateScan(ctx, input)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation),
		"a not-found profile should surface as a validation error")
	assert.Contains(t, err.Error(), "not found")
}

func TestScanService_CreateScan_ProfileCheckError(t *testing.T) {
	ctx := context.Background()
	profileID := "some-profile"
	dbErr := fmt.Errorf("database connection lost")

	mock := &mockScanRepo{
		getProfile: func(_ context.Context, _ string) (*db.ScanProfile, error) {
			return nil, dbErr
		},
	}

	input := validCreateInput()
	input.ProfileID = &profileID

	_, err := newTestScanService(mock).CreateScan(ctx, input)

	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr)
	assert.Contains(t, err.Error(), "failed to verify profile")
}

func TestScanService_CreateScan_RepoError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("insert failed")

	mock := &mockScanRepo{
		createScan: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			return nil, wantErr
		},
	}

	_, err := newTestScanService(mock).CreateScan(ctx, validCreateInput())

	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_GetScan_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	expected := &db.Scan{ID: id}

	mock := &mockScanRepo{
		getScan: func(_ context.Context, got uuid.UUID) (*db.Scan, error) {
			assert.Equal(t, id, got)
			return expected, nil
		},
	}

	result, err := newTestScanService(mock).GetScan(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestScanService_GetScan_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.ErrNotFound("scan")

	mock := &mockScanRepo{
		getScan: func(_ context.Context, _ uuid.UUID) (*db.Scan, error) {
			return nil, wantErr
		},
	}

	_, err := newTestScanService(mock).GetScan(ctx, uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

// ─────────────────────────────────────────────────────────────────────────────
// UpdateScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_UpdateScan_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	newName := "Updated Name"
	expected := &db.Scan{ID: id, Name: newName}

	mock := &mockScanRepo{
		updateScan: func(_ context.Context, gotID uuid.UUID, input db.UpdateScanInput) (*db.Scan, error) {
			assert.Equal(t, id, gotID)
			require.NotNil(t, input.Name)
			assert.Equal(t, newName, *input.Name)
			return expected, nil
		},
	}

	result, err := newTestScanService(mock).UpdateScan(ctx, id, db.UpdateScanInput{Name: &newName})

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestScanService_UpdateScan_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("update failed")

	mock := &mockScanRepo{
		updateScan: func(_ context.Context, _ uuid.UUID, _ db.UpdateScanInput) (*db.Scan, error) {
			return nil, wantErr
		},
	}

	_, err := newTestScanService(mock).UpdateScan(ctx, uuid.New(), db.UpdateScanInput{})
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// DeleteScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_DeleteScan_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	mock := &mockScanRepo{
		deleteScan: func(_ context.Context, gotID uuid.UUID) error {
			called = true
			assert.Equal(t, id, gotID)
			return nil
		},
	}

	err := newTestScanService(mock).DeleteScan(ctx, id)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestScanService_DeleteScan_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("delete failed")

	mock := &mockScanRepo{
		deleteScan: func(_ context.Context, _ uuid.UUID) error { return wantErr },
	}

	err := newTestScanService(mock).DeleteScan(ctx, uuid.New())
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// StartScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_StartScan_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	running := &db.Scan{ID: id, Status: db.ScanJobStatusRunning}

	mock := &mockScanRepo{
		startScan: func(_ context.Context, gotID uuid.UUID) error {
			assert.Equal(t, id, gotID)
			return nil
		},
		getScan: func(_ context.Context, _ uuid.UUID) (*db.Scan, error) {
			return running, nil
		},
	}

	result, err := newTestScanService(mock).StartScan(ctx, id)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, db.ScanJobStatusRunning, result.Status)
}

func TestScanService_StartScan_AlreadyRunning(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	mock := &mockScanRepo{
		startScan: func(_ context.Context, _ uuid.UUID) error {
			return errors.ErrConflictWithReason("scan", `scan is in state "running", expected "pending"`)
		},
	}
	_, err := newTestScanService(mock).StartScan(ctx, id)
	require.Error(t, err)
	assert.True(t, errors.IsConflict(err))
}

func TestScanService_StartScan_AlreadyCompleted(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	mock := &mockScanRepo{
		startScan: func(_ context.Context, _ uuid.UUID) error {
			return errors.ErrConflictWithReason("scan", `scan is in state "completed", expected "pending"`)
		},
	}
	_, err := newTestScanService(mock).StartScan(ctx, id)
	require.Error(t, err)
	assert.True(t, errors.IsConflict(err))
}

func TestScanService_StartScan_NotFound(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.ErrNotFound("scan")
	mock := &mockScanRepo{
		startScan: func(_ context.Context, _ uuid.UUID) error { return wantErr },
	}
	_, err := newTestScanService(mock).StartScan(ctx, uuid.New())
	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestScanService_StartScan_StartScanError(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	wantErr := fmt.Errorf("start transition failed")

	mock := &mockScanRepo{
		startScan: func(_ context.Context, _ uuid.UUID) error {
			return wantErr
		},
	}

	_, err := newTestScanService(mock).StartScan(ctx, id)

	assert.ErrorIs(t, err, wantErr)
}

func TestScanService_StartScan_FinalGetScanError(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	wantErr := fmt.Errorf("refresh query failed")
	mock := &mockScanRepo{
		startScan: func(_ context.Context, _ uuid.UUID) error { return nil },
		getScan:   func(_ context.Context, _ uuid.UUID) (*db.Scan, error) { return nil, wantErr },
	}
	_, err := newTestScanService(mock).StartScan(ctx, id)
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// StopScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_StopScan_NoMessage(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	mock := &mockScanRepo{
		stopScan: func(_ context.Context, gotID uuid.UUID, msg ...string) error {
			called = true
			assert.Equal(t, id, gotID)
			assert.Empty(t, msg)
			return nil
		},
	}

	err := newTestScanService(mock).StopScan(ctx, id)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestScanService_StopScan_WithMessage(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	mock := &mockScanRepo{
		stopScan: func(_ context.Context, _ uuid.UUID, msg ...string) error {
			require.Len(t, msg, 1)
			assert.Equal(t, "something went wrong", msg[0])
			return nil
		},
	}

	err := newTestScanService(mock).StopScan(ctx, id, "something went wrong")
	require.NoError(t, err)
}

func TestScanService_StopScan_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("stop failed")

	mock := &mockScanRepo{
		stopScan: func(_ context.Context, _ uuid.UUID, _ ...string) error { return wantErr },
	}

	err := newTestScanService(mock).StopScan(ctx, uuid.New())
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// CompleteScan
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_CompleteScan_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	mock := &mockScanRepo{
		completeScan: func(_ context.Context, gotID uuid.UUID) error {
			called = true
			assert.Equal(t, id, gotID)
			return nil
		},
	}

	err := newTestScanService(mock).CompleteScan(ctx, id)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestScanService_CompleteScan_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("complete failed")

	mock := &mockScanRepo{
		completeScan: func(_ context.Context, _ uuid.UUID) error { return wantErr },
	}

	err := newTestScanService(mock).CompleteScan(ctx, uuid.New())
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetScanResults
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_GetScanResults_Success(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()
	expected := []*db.ScanResult{{ScanID: scanID, Port: 80}}

	mock := &mockScanRepo{
		getScanResults: func(_ context.Context, gotID uuid.UUID, offset, limit int) ([]*db.ScanResult, int64, error) {
			assert.Equal(t, scanID, gotID)
			assert.Equal(t, 0, offset)
			assert.Equal(t, 10, limit)
			return expected, 1, nil
		},
	}

	results, total, err := newTestScanService(mock).GetScanResults(ctx, scanID, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, expected, results)
	assert.Equal(t, int64(1), total)
}

func TestScanService_GetScanResults_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("results query failed")

	mock := &mockScanRepo{
		getScanResults: func(_ context.Context, _ uuid.UUID, _, _ int) ([]*db.ScanResult, int64, error) {
			return nil, 0, wantErr
		},
	}

	_, _, err := newTestScanService(mock).GetScanResults(ctx, uuid.New(), 0, 10)
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetScanSummary
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_GetScanSummary_Success(t *testing.T) {
	ctx := context.Background()
	scanID := uuid.New()
	expected := &db.ScanSummary{ScanID: scanID, TotalHosts: 5, OpenPorts: 12}

	mock := &mockScanRepo{
		getScanSummary: func(_ context.Context, gotID uuid.UUID) (*db.ScanSummary, error) {
			assert.Equal(t, scanID, gotID)
			return expected, nil
		},
	}

	summary, err := newTestScanService(mock).GetScanSummary(ctx, scanID)

	require.NoError(t, err)
	assert.Equal(t, expected, summary)
}

func TestScanService_GetScanSummary_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("summary query failed")

	mock := &mockScanRepo{
		getScanSummary: func(_ context.Context, _ uuid.UUID) (*db.ScanSummary, error) {
			return nil, wantErr
		},
	}

	_, err := newTestScanService(mock).GetScanSummary(ctx, uuid.New())
	assert.ErrorIs(t, err, wantErr)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetProfile
// ─────────────────────────────────────────────────────────────────────────────

func TestScanService_GetProfile_Success(t *testing.T) {
	ctx := context.Background()
	expected := &db.ScanProfile{ID: "tcp-common"}

	mock := &mockScanRepo{
		getProfile: func(_ context.Context, id string) (*db.ScanProfile, error) {
			assert.Equal(t, "tcp-common", id)
			return expected, nil
		},
	}

	result, err := newTestScanService(mock).GetProfile(ctx, "tcp-common")

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestScanService_GetProfile_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.ErrNotFound("profile")

	mock := &mockScanRepo{
		getProfile: func(_ context.Context, _ string) (*db.ScanProfile, error) {
			return nil, wantErr
		},
	}

	_, err := newTestScanService(mock).GetProfile(ctx, "unknown")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}

// ─────────────────────────────────────────────────────────────────────────────
// validateScanInput — every branch
// ─────────────────────────────────────────────────────────────────────────────

func TestValidateScanInput(t *testing.T) {
	t.Run("valid input passes", func(t *testing.T) {
		assert.NoError(t, validateScanInput(validCreateInput()))
	})

	// ── Name ─────────────────────────────────────────────────────────────────

	t.Run("empty name", func(t *testing.T) {
		in := validCreateInput()
		in.Name = ""
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("name exactly at MaxScanNameLength is valid", func(t *testing.T) {
		in := validCreateInput()
		in.Name = strings.Repeat("a", MaxScanNameLength)
		assert.NoError(t, validateScanInput(in))
	})

	t.Run("name one character over MaxScanNameLength is rejected", func(t *testing.T) {
		in := validCreateInput()
		in.Name = strings.Repeat("a", MaxScanNameLength+1)
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "too long")
	})

	// ── Targets ───────────────────────────────────────────────────────────────

	t.Run("empty targets slice", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "at least one target")
	})

	t.Run("nil targets slice", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = nil
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "at least one target")
	})

	t.Run("too many targets", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = make([]string, MaxTargetCount+1)
		for i := range in.Targets {
			// Produce valid IPv4 addresses spread across /24 blocks so each is
			// distinct and passes the per-target IP check (the count check
			// fires first, but valid IPs keep the intent clear).
			in.Targets[i] = fmt.Sprintf("10.%d.%d.1", i/256, i%256)
		}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "too many targets")
	})

	t.Run("exactly MaxTargetCount targets is valid", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = make([]string, MaxTargetCount)
		for i := range in.Targets {
			in.Targets[i] = fmt.Sprintf("10.%d.%d.1", i/256, i%256)
		}
		assert.NoError(t, validateScanInput(in))
	})

	t.Run("empty target string inside slice", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"192.168.1.1", ""}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("target string too long", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{strings.Repeat("x", MaxTargetLength+1)}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "too long")
	})

	t.Run("target at MaxTargetLength but invalid IP is rejected for format", func(t *testing.T) {
		// Exactly MaxTargetLength chars: passes the length gate, fails IP check.
		in := validCreateInput()
		in.Targets = []string{strings.Repeat("a", MaxTargetLength)}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "not a valid IP")
	})

	t.Run("invalid target (hostname, not an IP or CIDR)", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"not-an-ip"}
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "not a valid IP")
	})

	t.Run("valid IPv4 address", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"10.0.0.1"}
		assert.NoError(t, validateScanInput(in))
	})

	t.Run("valid IPv6 address", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"::1"}
		assert.NoError(t, validateScanInput(in))
	})

	t.Run("valid IPv4 CIDR", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"10.0.0.0/8"}
		assert.NoError(t, validateScanInput(in))
	})

	t.Run("valid IPv6 CIDR", func(t *testing.T) {
		in := validCreateInput()
		in.Targets = []string{"2001:db8::/32"}
		assert.NoError(t, validateScanInput(in))
	})

	// ── ScanType ──────────────────────────────────────────────────────────────

	t.Run("invalid scan type is rejected", func(t *testing.T) {
		in := validCreateInput()
		in.ScanType = "stealth"
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "invalid scan type")
	})

	t.Run("empty scan type is rejected", func(t *testing.T) {
		in := validCreateInput()
		in.ScanType = ""
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "invalid scan type")
	})

	t.Run("all valid scan types are accepted", func(t *testing.T) {
		for _, st := range []string{"connect", "syn", "ack", "udp", "aggressive", "comprehensive"} {
			t.Run(st, func(t *testing.T) {
				in := validCreateInput()
				in.ScanType = st
				assert.NoError(t, validateScanInput(in))
			})
		}
	})

	// ── Ports ─────────────────────────────────────────────────────────────────

	t.Run("empty ports string", func(t *testing.T) {
		in := validCreateInput()
		in.Ports = ""
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
		assert.Contains(t, err.Error(), "ports is required")
	})

	t.Run("invalid port spec is wrapped as a validation error", func(t *testing.T) {
		in := validCreateInput()
		in.Ports = "abc"
		err := validateScanInput(in)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeValidation))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// ParsePortSpec — exported helper, exercised directly
// ─────────────────────────────────────────────────────────────────────────────

func TestParsePortSpec(t *testing.T) {
	// ── Valid cases ───────────────────────────────────────────────────────────

	t.Run("single port", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("80"))
	})

	t.Run("multiple comma-separated ports", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("80,443,8080"))
	})

	t.Run("hyphenated range", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("1024-9999"))
	})

	t.Run("T: prefix on single port", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("T:80"))
	})

	t.Run("U: prefix on single port", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("U:53"))
	})

	t.Run("T: prefix on range", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("T:80-443"))
	})

	t.Run("U: prefix on range", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("U:53-100"))
	})

	t.Run("mixed prefixes and bare ranges", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("T:80,U:53,1024-9999"))
	})

	t.Run("boundary port 1", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("1"))
	})

	t.Run("boundary port 65535", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("65535"))
	})

	t.Run("trailing comma produces empty token that is skipped", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("80,"))
	})

	t.Run("double comma produces empty token that is skipped", func(t *testing.T) {
		assert.NoError(t, ParsePortSpec("80,,443"))
	})

	// ── Invalid cases ─────────────────────────────────────────────────────────

	t.Run("port 0 is out of range", func(t *testing.T) {
		err := ParsePortSpec("0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("port 65536 is out of range", func(t *testing.T) {
		err := ParsePortSpec("65536")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("non-numeric port string", func(t *testing.T) {
		err := ParsePortSpec("http")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("range where start is greater than end", func(t *testing.T) {
		err := ParsePortSpec("9000-80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start must be <= end")
	})

	t.Run("range with non-numeric start", func(t *testing.T) {
		err := ParsePortSpec("abc-80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("range with non-numeric end", func(t *testing.T) {
		err := ParsePortSpec("80-xyz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("range with zero start", func(t *testing.T) {
		err := ParsePortSpec("0-80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("range with zero end", func(t *testing.T) {
		err := ParsePortSpec("1-0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("range with end above 65535", func(t *testing.T) {
		err := ParsePortSpec("1-65536")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("whitespace space inside token triggers rejection", func(t *testing.T) {
		// "80 - 443" is one comma token containing spaces.
		err := ParsePortSpec("80 - 443")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace not allowed")
	})

	t.Run("whitespace tab inside token triggers rejection", func(t *testing.T) {
		err := ParsePortSpec("80\t443")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace not allowed")
	})

	t.Run("error in second comma token surfaces correctly", func(t *testing.T) {
		err := ParsePortSpec("80,bad-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// parsePortToken (unexported — accessible because we are in package services)
// ─────────────────────────────────────────────────────────────────────────────

func TestParsePortToken(t *testing.T) {
	t.Run("valid single port", func(t *testing.T) {
		assert.NoError(t, parsePortToken("80"))
	})

	t.Run("valid port with T: prefix", func(t *testing.T) {
		assert.NoError(t, parsePortToken("T:443"))
	})

	t.Run("valid port with U: prefix", func(t *testing.T) {
		assert.NoError(t, parsePortToken("U:53"))
	})

	t.Run("valid ascending range", func(t *testing.T) {
		assert.NoError(t, parsePortToken("1000-2000"))
	})

	t.Run("valid range with T: prefix", func(t *testing.T) {
		assert.NoError(t, parsePortToken("T:80-443"))
	})

	t.Run("space triggers whitespace rejection", func(t *testing.T) {
		err := parsePortToken("80 443")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace not allowed")
	})

	t.Run("tab triggers whitespace rejection", func(t *testing.T) {
		err := parsePortToken("80\t443")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace not allowed")
	})

	t.Run("whitespace after prefix stripping is still caught", func(t *testing.T) {
		// After "T:" is stripped the remainder is "80 90" which has a space.
		err := parsePortToken("T:80 90")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace not allowed")
	})

	t.Run("non-numeric token returns number error", func(t *testing.T) {
		err := parsePortToken("xyz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("range with invalid start", func(t *testing.T) {
		err := parsePortToken("abc-100")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("range with invalid end", func(t *testing.T) {
		err := parsePortToken("80-xyz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// parsePortRange (unexported)
// ─────────────────────────────────────────────────────────────────────────────

func TestParsePortRange(t *testing.T) {
	t.Run("valid ascending range", func(t *testing.T) {
		assert.NoError(t, parsePortRange("100", "200"))
	})

	t.Run("equal start and end is valid", func(t *testing.T) {
		assert.NoError(t, parsePortRange("80", "80"))
	})

	t.Run("full boundary range 1-65535 is valid", func(t *testing.T) {
		assert.NoError(t, parsePortRange("1", "65535"))
	})

	t.Run("non-numeric start", func(t *testing.T) {
		err := parsePortRange("abc", "80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("non-numeric end", func(t *testing.T) {
		err := parsePortRange("80", "xyz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("start of zero is out of range", func(t *testing.T) {
		err := parsePortRange("0", "80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("start above 65535 is out of range", func(t *testing.T) {
		err := parsePortRange("65536", "65536")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("end of zero is out of range", func(t *testing.T) {
		err := parsePortRange("1", "0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("end above 65535 is out of range", func(t *testing.T) {
		err := parsePortRange("1", "65536")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("start greater than end", func(t *testing.T) {
		err := parsePortRange("9000", "80")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start must be <= end")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// validatePortNumber (unexported)
// ─────────────────────────────────────────────────────────────────────────────

func TestValidatePortNumber(t *testing.T) {
	t.Run("port 1 is valid", func(t *testing.T) {
		assert.NoError(t, validatePortNumber("1"))
	})

	t.Run("port 80 is valid", func(t *testing.T) {
		assert.NoError(t, validatePortNumber("80"))
	})

	t.Run("port 65535 is valid", func(t *testing.T) {
		assert.NoError(t, validatePortNumber("65535"))
	})

	t.Run("port 0 is invalid", func(t *testing.T) {
		err := validatePortNumber("0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("port 65536 is invalid", func(t *testing.T) {
		err := validatePortNumber("65536")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("negative port is invalid", func(t *testing.T) {
		err := validatePortNumber("-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be between")
	})

	t.Run("alphabetic string is not a number", func(t *testing.T) {
		err := validatePortNumber("https")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("empty string is not a number", func(t *testing.T) {
		err := validatePortNumber("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})

	t.Run("float string is not a valid integer port", func(t *testing.T) {
		err := validatePortNumber("80.5")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a number")
	})
}
