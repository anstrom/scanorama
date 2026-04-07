// Package services contains same-package unit tests for HostService.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// ---------------------------------------------------------------------------
// Hand-rolled mock for hostRepository
// ---------------------------------------------------------------------------

type mockHostRepo struct {
	listHostsFn       func(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
	createHostFn      func(ctx context.Context, input db.CreateHostInput) (*db.Host, error)
	getHostFn         func(ctx context.Context, id uuid.UUID) (*db.Host, error)
	updateHostFn      func(ctx context.Context, id uuid.UUID, input db.UpdateHostInput) (*db.Host, error)
	deleteHostFn      func(ctx context.Context, id uuid.UUID) error
	bulkDeleteHostsFn func(ctx context.Context, ids []uuid.UUID) (int64, error)
	getHostScansFn    func(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error)
}

func (m *mockHostRepo) ListHosts(
	ctx context.Context, filters *db.HostFilters, offset, limit int,
) ([]*db.Host, int64, error) {
	return m.listHostsFn(ctx, filters, offset, limit)
}

func (m *mockHostRepo) CreateHost(ctx context.Context, input db.CreateHostInput) (*db.Host, error) {
	return m.createHostFn(ctx, input)
}

func (m *mockHostRepo) GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error) {
	return m.getHostFn(ctx, id)
}

func (m *mockHostRepo) UpdateHost(ctx context.Context, id uuid.UUID, input db.UpdateHostInput) (*db.Host, error) {
	return m.updateHostFn(ctx, id, input)
}

func (m *mockHostRepo) DeleteHost(ctx context.Context, id uuid.UUID) error {
	return m.deleteHostFn(ctx, id)
}

func (m *mockHostRepo) BulkDeleteHosts(ctx context.Context, ids []uuid.UUID) (int64, error) {
	if m.bulkDeleteHostsFn != nil {
		return m.bulkDeleteHostsFn(ctx, ids)
	}
	panic("mockHostRepo: BulkDeleteHosts called unexpectedly")
}

func (m *mockHostRepo) GetHostScans(
	ctx context.Context, hostID uuid.UUID, offset, limit int,
) ([]*db.Scan, int64, error) {
	return m.getHostScansFn(ctx, hostID, offset, limit)
}

// ---------------------------------------------------------------------------
// NewHostService
// ---------------------------------------------------------------------------

func TestNewHostService(t *testing.T) {
	repo := &mockHostRepo{}
	svc := NewHostService(repo, slog.Default())
	require.NotNil(t, svc)
}

// ---------------------------------------------------------------------------
// ListHosts
// ---------------------------------------------------------------------------

func TestListHosts_Delegates(t *testing.T) {
	ctx := context.Background()
	wantHosts := []*db.Host{{ID: uuid.New()}, {ID: uuid.New()}}
	wantTotal := int64(2)
	filters := &db.HostFilters{Status: db.HostStatusUp}

	var gotFilters *db.HostFilters
	var gotOffset, gotLimit int

	repo := &mockHostRepo{
		listHostsFn: func(ctx context.Context, f *db.HostFilters, offset, limit int) ([]*db.Host, int64, error) {
			gotFilters = f
			gotOffset = offset
			gotLimit = limit
			return wantHosts, wantTotal, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, total, err := svc.ListHosts(ctx, filters, 5, 25)

	require.NoError(t, err)
	assert.Equal(t, wantHosts, got)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, filters, gotFilters)
	assert.Equal(t, 5, gotOffset)
	assert.Equal(t, 25, gotLimit)
}

func TestListHosts_Error(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("db connection lost")

	repo := &mockHostRepo{
		listHostsFn: func(ctx context.Context, f *db.HostFilters, offset, limit int) ([]*db.Host, int64, error) {
			return nil, 0, wantErr
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, total, err := svc.ListHosts(ctx, nil, 0, 10)

	require.Error(t, err)
	assert.Equal(t, wantErr, err)
	assert.Nil(t, got)
	assert.Zero(t, total)
}

// ---------------------------------------------------------------------------
// CreateHost
// ---------------------------------------------------------------------------

func TestCreateHost_EmptyIP_ValidationError(t *testing.T) {
	ctx := context.Background()

	// Repo must not be called — no function fields set; any accidental call panics.
	repo := &mockHostRepo{}
	svc := NewHostService(repo, slog.Default())

	got, err := svc.CreateHost(ctx, db.CreateHostInput{IPAddress: ""})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
}

func TestCreateHost_Valid_Delegates(t *testing.T) {
	ctx := context.Background()
	input := db.CreateHostInput{
		IPAddress: "10.0.0.1",
		Hostname:  "web-01",
		Status:    db.HostStatusUp,
	}
	want := &db.Host{ID: uuid.New()}

	var gotInput db.CreateHostInput
	repo := &mockHostRepo{
		createHostFn: func(ctx context.Context, in db.CreateHostInput) (*db.Host, error) {
			gotInput = in
			return want, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.CreateHost(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// GetHost
// ---------------------------------------------------------------------------

func TestGetHost_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	want := &db.Host{ID: id}

	var gotID uuid.UUID
	repo := &mockHostRepo{
		getHostFn: func(ctx context.Context, in uuid.UUID) (*db.Host, error) {
			gotID = in
			return want, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHost(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
}

// ---------------------------------------------------------------------------
// UpdateHost
// ---------------------------------------------------------------------------

func TestUpdateHost_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	hostname := "updated-host"
	input := db.UpdateHostInput{Hostname: &hostname}
	want := &db.Host{ID: id}

	var gotID uuid.UUID
	var gotInput db.UpdateHostInput
	repo := &mockHostRepo{
		updateHostFn: func(ctx context.Context, in uuid.UUID, inp db.UpdateHostInput) (*db.Host, error) {
			gotID = in
			gotInput = inp
			return want, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.UpdateHost(ctx, id, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// DeleteHost
// ---------------------------------------------------------------------------

func TestDeleteHost_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	repo := &mockHostRepo{
		deleteHostFn: func(ctx context.Context, in uuid.UUID) error {
			called = true
			assert.Equal(t, id, in)
			return nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.DeleteHost(ctx, id)

	require.NoError(t, err)
	assert.True(t, called, "expected DeleteHost to be called on the repository")
}

// ---------------------------------------------------------------------------
// GetHostScans
// ---------------------------------------------------------------------------

func TestGetHostScans_Delegates(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	wantScans := []*db.Scan{{ID: uuid.New()}, {ID: uuid.New()}}
	wantTotal := int64(2)

	var gotHostID uuid.UUID
	var gotOffset, gotLimit int
	repo := &mockHostRepo{
		getHostScansFn: func(ctx context.Context, hID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error) {
			gotHostID = hID
			gotOffset = offset
			gotLimit = limit
			return wantScans, wantTotal, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, total, err := svc.GetHostScans(ctx, hostID, 10, 50)

	require.NoError(t, err)
	assert.Equal(t, wantScans, got)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, hostID, gotHostID)
	assert.Equal(t, 10, gotOffset)
	assert.Equal(t, 50, gotLimit)
}
