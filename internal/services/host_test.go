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
	getAllTagsFn      func(ctx context.Context) ([]string, error)
	updateHostTagsFn  func(ctx context.Context, id uuid.UUID, tags []string) error
	addHostTagsFn     func(ctx context.Context, id uuid.UUID, tags []string) error
	removeHostTagsFn  func(ctx context.Context, id uuid.UUID, tags []string) error
	bulkUpdateTagsFn  func(ctx context.Context, ids []uuid.UUID, tags []string, action string) error
	getHostGroupsFn   func(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error)
	getHostNetworksFn func(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error)
}

func (m *mockHostRepo) GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error) {
	if m.getHostNetworksFn == nil {
		return []*db.Network{}, nil
	}
	return m.getHostNetworksFn(ctx, hostID)
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

func (m *mockHostRepo) GetAllTags(ctx context.Context) ([]string, error) {
	if m.getAllTagsFn != nil {
		return m.getAllTagsFn(ctx)
	}
	return []string{}, nil
}

func (m *mockHostRepo) UpdateHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	if m.updateHostTagsFn != nil {
		return m.updateHostTagsFn(ctx, id, tags)
	}
	return nil
}

func (m *mockHostRepo) AddHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	if m.addHostTagsFn != nil {
		return m.addHostTagsFn(ctx, id, tags)
	}
	return nil
}

func (m *mockHostRepo) RemoveHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	if m.removeHostTagsFn != nil {
		return m.removeHostTagsFn(ctx, id, tags)
	}
	return nil
}

func (m *mockHostRepo) BulkUpdateTags(ctx context.Context, ids []uuid.UUID, tags []string, action string) error {
	if m.bulkUpdateTagsFn != nil {
		return m.bulkUpdateTagsFn(ctx, ids, tags, action)
	}
	return nil
}

func (m *mockHostRepo) GetHostGroups(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error) {
	if m.getHostGroupsFn != nil {
		return m.getHostGroupsFn(ctx, hostID)
	}
	return []db.HostGroupSummary{}, nil
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

// ---------------------------------------------------------------------------
// ListTags
// ---------------------------------------------------------------------------

func TestListTags_ReturnsSortedTags(t *testing.T) {
	ctx := context.Background()
	want := []string{"web", "prod", "db"}

	repo := &mockHostRepo{
		getAllTagsFn: func(ctx context.Context) ([]string, error) {
			return want, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.ListTags(ctx)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListTags_EmptyWhenNoTags(t *testing.T) {
	ctx := context.Background()

	repo := &mockHostRepo{
		getAllTagsFn: func(ctx context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.ListTags(ctx)

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestListTags_PropagatesError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("db error")

	repo := &mockHostRepo{
		getAllTagsFn: func(ctx context.Context) ([]string, error) {
			return nil, wantErr
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.ListTags(ctx)

	require.Error(t, err)
	assert.Equal(t, wantErr, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// UpdateHostTags
// ---------------------------------------------------------------------------

func TestUpdateHostTags_PassesTagsAndID(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	tags := []string{"prod", "web"}

	var gotID uuid.UUID
	var gotTags []string

	repo := &mockHostRepo{
		updateHostTagsFn: func(ctx context.Context, id uuid.UUID, ts []string) error {
			gotID = id
			gotTags = ts
			return nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.UpdateHostTags(ctx, hostID, tags)

	require.NoError(t, err)
	assert.Equal(t, hostID, gotID)
	assert.Equal(t, tags, gotTags)
}

func TestUpdateHostTags_PropagatesNotFoundError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("host not found")

	repo := &mockHostRepo{
		updateHostTagsFn: func(ctx context.Context, id uuid.UUID, tags []string) error {
			return wantErr
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.UpdateHostTags(ctx, uuid.New(), []string{"prod"})

	assert.Equal(t, wantErr, err)
}

// ---------------------------------------------------------------------------
// AddHostTags
// ---------------------------------------------------------------------------

func TestAddHostTags_PassesCorrectArgs(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	tags := []string{"staging", "api"}

	var gotID uuid.UUID
	var gotTags []string

	repo := &mockHostRepo{
		addHostTagsFn: func(ctx context.Context, id uuid.UUID, ts []string) error {
			gotID = id
			gotTags = ts
			return nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.AddHostTags(ctx, hostID, tags)

	require.NoError(t, err)
	assert.Equal(t, hostID, gotID)
	assert.Equal(t, tags, gotTags)
}

// ---------------------------------------------------------------------------
// RemoveHostTags
// ---------------------------------------------------------------------------

func TestRemoveHostTags_PassesCorrectArgs(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	tags := []string{"old-tag", "deprecated"}

	var gotID uuid.UUID
	var gotTags []string

	repo := &mockHostRepo{
		removeHostTagsFn: func(ctx context.Context, id uuid.UUID, ts []string) error {
			gotID = id
			gotTags = ts
			return nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.RemoveHostTags(ctx, hostID, tags)

	require.NoError(t, err)
	assert.Equal(t, hostID, gotID)
	assert.Equal(t, tags, gotTags)
}

// ---------------------------------------------------------------------------
// BulkUpdateTags
// ---------------------------------------------------------------------------

func TestBulkUpdateTags_PassesActionAndIDs(t *testing.T) {
	ctx := context.Background()
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	tags := []string{"prod"}
	action := "add"

	var gotIDs []uuid.UUID
	var gotTags []string
	var gotAction string

	repo := &mockHostRepo{
		bulkUpdateTagsFn: func(ctx context.Context, is []uuid.UUID, ts []string, a string) error {
			gotIDs = is
			gotTags = ts
			gotAction = a
			return nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.BulkUpdateTags(ctx, ids, tags, action)

	require.NoError(t, err)
	assert.Equal(t, ids, gotIDs)
	assert.Equal(t, tags, gotTags)
	assert.Equal(t, action, gotAction)
}

func TestBulkUpdateTags_PropagatesError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("bulk operation failed")

	repo := &mockHostRepo{
		bulkUpdateTagsFn: func(ctx context.Context, ids []uuid.UUID, tags []string, action string) error {
			return wantErr
		},
	}

	svc := NewHostService(repo, slog.Default())
	err := svc.BulkUpdateTags(ctx, []uuid.UUID{uuid.New()}, []string{"prod"}, "add")

	assert.Equal(t, wantErr, err)
}

// ---------------------------------------------------------------------------
// GetHostGroups
// ---------------------------------------------------------------------------

func TestGetHostGroups_ReturnsGroupSummaries(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	id1 := uuid.New()
	id2 := uuid.New()
	want := []db.HostGroupSummary{
		{ID: id1, Name: "production"},
		{ID: id2, Name: "web-tier"},
	}

	repo := &mockHostRepo{
		getHostGroupsFn: func(ctx context.Context, hID uuid.UUID) ([]db.HostGroupSummary, error) {
			return want, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHostGroups(ctx, hostID)

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, id1, got[0].ID)
	assert.Equal(t, "production", got[0].Name)
	assert.Equal(t, id2, got[1].ID)
	assert.Equal(t, "web-tier", got[1].Name)
}

func TestGetHostGroups_ReturnsEmptySliceForHostWithNoGroups(t *testing.T) {
	ctx := context.Background()

	repo := &mockHostRepo{
		getHostGroupsFn: func(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error) {
			return []db.HostGroupSummary{}, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHostGroups(ctx, uuid.New())

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// GetHostNetworks
// ---------------------------------------------------------------------------

func TestGetHostNetworks_DelegatesToRepository(t *testing.T) {
	ctx := context.Background()
	hostID := uuid.New()
	netA := &db.Network{ID: uuid.New(), Name: "dmz"}
	netB := &db.Network{ID: uuid.New(), Name: "corp"}

	var gotHostID uuid.UUID
	repo := &mockHostRepo{
		getHostNetworksFn: func(_ context.Context, h uuid.UUID) ([]*db.Network, error) {
			gotHostID = h
			return []*db.Network{netA, netB}, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHostNetworks(ctx, hostID)

	require.NoError(t, err)
	assert.Equal(t, hostID, gotHostID)
	require.Len(t, got, 2)
	assert.Equal(t, netA.ID, got[0].ID)
	assert.Equal(t, netB.ID, got[1].ID)
}

func TestGetHostNetworks_ReturnsEmptySliceForUnmatchedHost(t *testing.T) {
	ctx := context.Background()

	repo := &mockHostRepo{
		getHostNetworksFn: func(_ context.Context, _ uuid.UUID) ([]*db.Network, error) {
			return []*db.Network{}, nil
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHostNetworks(ctx, uuid.New())

	require.NoError(t, err)
	assert.NotNil(t, got, "empty result must be [] not nil for JSON encoding")
	assert.Empty(t, got)
}

func TestGetHostNetworks_PropagatesRepositoryError(t *testing.T) {
	ctx := context.Background()

	repo := &mockHostRepo{
		getHostNetworksFn: func(_ context.Context, _ uuid.UUID) ([]*db.Network, error) {
			return nil, fmt.Errorf("db down")
		},
	}

	svc := NewHostService(repo, slog.Default())
	got, err := svc.GetHostNetworks(ctx, uuid.New())

	require.Error(t, err)
	assert.Nil(t, got)
}
