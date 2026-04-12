// Package services contains same-package unit tests for ProfileService.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ---------------------------------------------------------------------------
// Hand-rolled mock for profileRepository
// ---------------------------------------------------------------------------

type mockProfileRepo struct {
	listProfilesFn func(
		ctx context.Context, filters db.ProfileFilters, offset, limit int,
	) ([]*db.ScanProfile, int64, error)
	createProfileFn   func(ctx context.Context, input db.CreateProfileInput) (*db.ScanProfile, error)
	getProfileFn      func(ctx context.Context, id string) (*db.ScanProfile, error)
	updateProfileFn   func(ctx context.Context, id string, input db.UpdateProfileInput) (*db.ScanProfile, error)
	deleteProfileFn   func(ctx context.Context, id string) error
	getProfileStatsFn func(ctx context.Context, id string) (*db.ProfileStats, error)
}

func (m *mockProfileRepo) ListProfiles(
	ctx context.Context, filters db.ProfileFilters, offset, limit int,
) ([]*db.ScanProfile, int64, error) {
	return m.listProfilesFn(ctx, filters, offset, limit)
}

func (m *mockProfileRepo) CreateProfile(ctx context.Context, input db.CreateProfileInput) (*db.ScanProfile, error) {
	return m.createProfileFn(ctx, input)
}

func (m *mockProfileRepo) GetProfile(ctx context.Context, id string) (*db.ScanProfile, error) {
	return m.getProfileFn(ctx, id)
}

func (m *mockProfileRepo) UpdateProfile(
	ctx context.Context, id string, input db.UpdateProfileInput,
) (*db.ScanProfile, error) {
	return m.updateProfileFn(ctx, id, input)
}

func (m *mockProfileRepo) DeleteProfile(ctx context.Context, id string) error {
	return m.deleteProfileFn(ctx, id)
}

func (m *mockProfileRepo) GetProfileStats(ctx context.Context, id string) (*db.ProfileStats, error) {
	if m.getProfileStatsFn != nil {
		return m.getProfileStatsFn(ctx, id)
	}
	panic("mockProfileRepo: GetProfileStats called unexpectedly")
}

// ---------------------------------------------------------------------------
// NewProfileService
// ---------------------------------------------------------------------------

func TestNewProfileService(t *testing.T) {
	repo := &mockProfileRepo{}
	svc := NewProfileService(repo, slog.Default())
	require.NotNil(t, svc)
}

// ---------------------------------------------------------------------------
// ListProfiles
// ---------------------------------------------------------------------------

func TestListProfiles_Delegates(t *testing.T) {
	ctx := context.Background()
	wantProfiles := []*db.ScanProfile{
		{ID: "profile-1", Name: "Quick Scan"},
		{ID: "profile-2", Name: "Full Scan"},
	}
	wantTotal := int64(2)
	filters := db.ProfileFilters{ScanType: db.ScanTypeConnect}

	var gotFilters db.ProfileFilters
	var gotOffset, gotLimit int

	repo := &mockProfileRepo{
		listProfilesFn: func(
			ctx context.Context, f db.ProfileFilters, offset, limit int,
		) ([]*db.ScanProfile, int64, error) {
			gotFilters = f
			gotOffset = offset
			gotLimit = limit
			return wantProfiles, wantTotal, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, total, err := svc.ListProfiles(ctx, filters, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, wantProfiles, got)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, filters, gotFilters)
	assert.Equal(t, 0, gotOffset)
	assert.Equal(t, 20, gotLimit)
}

// ---------------------------------------------------------------------------
// CreateProfile
// ---------------------------------------------------------------------------

func TestCreateProfile_Delegates(t *testing.T) {
	ctx := context.Background()
	input := db.CreateProfileInput{
		Name:     "Custom Scan",
		ScanType: db.ScanTypeSYN,
		Ports:    "22,80,443",
		Timing:   db.ScanTimingNormal,
	}
	want := &db.ScanProfile{ID: "new-profile-id", Name: "Custom Scan"}

	var gotInput db.CreateProfileInput
	repo := &mockProfileRepo{
		createProfileFn: func(ctx context.Context, in db.CreateProfileInput) (*db.ScanProfile, error) {
			gotInput = in
			return want, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.CreateProfile(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// GetProfile
// ---------------------------------------------------------------------------

func TestGetProfile_Delegates(t *testing.T) {
	ctx := context.Background()
	want := &db.ScanProfile{ID: "profile-abc", Name: "My Profile"}

	var gotID string
	repo := &mockProfileRepo{
		getProfileFn: func(ctx context.Context, id string) (*db.ScanProfile, error) {
			gotID = id
			return want, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.GetProfile(ctx, "profile-abc")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "profile-abc", gotID)
}

// ---------------------------------------------------------------------------
// UpdateProfile
// ---------------------------------------------------------------------------

func TestUpdateProfile_Delegates(t *testing.T) {
	ctx := context.Background()
	newName := "Renamed Profile"
	input := db.UpdateProfileInput{Name: &newName}
	want := &db.ScanProfile{ID: "profile-xyz", Name: newName}

	var gotID string
	var gotInput db.UpdateProfileInput
	repo := &mockProfileRepo{
		updateProfileFn: func(ctx context.Context, id string, in db.UpdateProfileInput) (*db.ScanProfile, error) {
			gotID = id
			gotInput = in
			return want, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.UpdateProfile(ctx, "profile-xyz", input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "profile-xyz", gotID)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// DeleteProfile
// ---------------------------------------------------------------------------

func TestDeleteProfile_Delegates(t *testing.T) {
	ctx := context.Background()
	called := false

	repo := &mockProfileRepo{
		deleteProfileFn: func(ctx context.Context, id string) error {
			called = true
			assert.Equal(t, "profile-del", id)
			return nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	err := svc.DeleteProfile(ctx, "profile-del")

	require.NoError(t, err)
	assert.True(t, called, "expected DeleteProfile to be called on the repository")
}

// ---------------------------------------------------------------------------
// CloneProfile
// ---------------------------------------------------------------------------

func TestCloneProfile_SourceNotFound_Error(t *testing.T) {
	ctx := context.Background()
	repoErr := fmt.Errorf("profile not found")

	repo := &mockProfileRepo{
		// createProfileFn intentionally left nil — must not be called.
		getProfileFn: func(ctx context.Context, id string) (*db.ScanProfile, error) {
			return nil, repoErr
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.CloneProfile(ctx, "missing-id", "clone-name")

	require.Error(t, err)
	assert.Nil(t, got)
	// The service wraps the error with context about which profile was cloned.
	assert.ErrorIs(t, err, repoErr)
	assert.Contains(t, err.Error(), "missing-id")
}

func TestCloneProfile_SourceFoundWithOptions_Success(t *testing.T) {
	ctx := context.Background()

	source := &db.ScanProfile{
		ID:       "source-id",
		Name:     "fast-scan",
		ScanType: db.ScanTypeConnect,
		Ports:    "80,443,8080",
		Timing:   db.ScanTimingAggressive,
		Options:  db.JSONB(`{"retries":2,"timeout":5}`),
	}

	wantClone := &db.ScanProfile{ID: "clone-id", Name: "fast-scan-copy"}

	var gotInput db.CreateProfileInput
	repo := &mockProfileRepo{
		getProfileFn: func(ctx context.Context, id string) (*db.ScanProfile, error) {
			return source, nil
		},
		createProfileFn: func(ctx context.Context, in db.CreateProfileInput) (*db.ScanProfile, error) {
			gotInput = in
			return wantClone, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.CloneProfile(ctx, "source-id", "fast-scan-copy")

	require.NoError(t, err)
	assert.Equal(t, wantClone, got)

	// Verify every field the service is responsible for setting.
	assert.Equal(t, "fast-scan-copy", gotInput.Name, "Name should be the requested clone name")
	assert.Equal(t, "Clone of fast-scan", gotInput.Description, "Description should reference source name")
	assert.Equal(t, db.ScanTypeConnect, gotInput.ScanType, "ScanType should be copied from source")
	assert.Equal(t, "80,443,8080", gotInput.Ports, "Ports should be copied from source")
	assert.Equal(t, db.ScanTimingAggressive, gotInput.Timing, "Timing should be copied from source")
	require.NotNil(t, gotInput.Options, "Options should be populated from source JSON")
	assert.Equal(t, float64(2), gotInput.Options["retries"])
	assert.Equal(t, float64(5), gotInput.Options["timeout"])
}

func TestCloneProfile_SourceFoundWithEmptyOptions_Success(t *testing.T) {
	ctx := context.Background()

	source := &db.ScanProfile{
		ID:       "source-id",
		Name:     "minimal-scan",
		ScanType: db.ScanTypeConnect,
		Ports:    "1-1024",
		Timing:   db.ScanTimingPolite,
		Options:  nil, // empty — no options to copy
	}

	wantClone := &db.ScanProfile{ID: "clone-id", Name: "minimal-copy"}

	var gotInput db.CreateProfileInput
	repo := &mockProfileRepo{
		getProfileFn: func(ctx context.Context, id string) (*db.ScanProfile, error) {
			return source, nil
		},
		createProfileFn: func(ctx context.Context, in db.CreateProfileInput) (*db.ScanProfile, error) {
			gotInput = in
			return wantClone, nil
		},
	}

	svc := NewProfileService(repo, slog.Default())
	got, err := svc.CloneProfile(ctx, "source-id", "minimal-copy")

	require.NoError(t, err)
	assert.Equal(t, wantClone, got)

	assert.Equal(t, "minimal-copy", gotInput.Name)
	assert.Equal(t, "Clone of minimal-scan", gotInput.Description)
	assert.Equal(t, db.ScanTypeConnect, gotInput.ScanType)
	assert.Equal(t, "1-1024", gotInput.Ports)
	assert.Equal(t, db.ScanTimingPolite, gotInput.Timing)
	assert.Nil(t, gotInput.Options, "Options should be nil when source has no options")
}

// ---------------------------------------------------------------------------
// GetProfileStats
// ---------------------------------------------------------------------------

func TestGetProfileStats_Delegates(t *testing.T) {
	ctx := context.Background()
	want := &db.ProfileStats{
		ProfileID:  "quick-scan",
		TotalScans: 5,
	}
	repo := &mockProfileRepo{
		getProfileStatsFn: func(_ context.Context, id string) (*db.ProfileStats, error) {
			return want, nil
		},
	}
	svc := NewProfileService(repo, slog.Default())
	got, err := svc.GetProfileStats(ctx, "quick-scan")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetProfileStats_PropagatesError(t *testing.T) {
	ctx := context.Background()
	repo := &mockProfileRepo{
		getProfileStatsFn: func(_ context.Context, _ string) (*db.ProfileStats, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	svc := NewProfileService(repo, slog.Default())
	_, err := svc.GetProfileStats(ctx, "quick-scan")
	require.Error(t, err)
}
