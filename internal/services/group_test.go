// Package services contains unit tests for GroupService.
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
)

// ---------------------------------------------------------------------------
// Hand-rolled mock for groupRepository
// ---------------------------------------------------------------------------

type mockGroupRepo struct {
	createGroupFn          func(ctx context.Context, input db.CreateGroupInput) (*db.HostGroup, error)
	getGroupFn             func(ctx context.Context, id uuid.UUID) (*db.HostGroup, error)
	listGroupsFn           func(ctx context.Context) ([]*db.HostGroup, error)
	updateGroupFn          func(ctx context.Context, id uuid.UUID, input db.UpdateGroupInput) (*db.HostGroup, error)
	deleteGroupFn          func(ctx context.Context, id uuid.UUID) error
	addHostsToGroupFn      func(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	removeHostsFromGroupFn func(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	getGroupMembersFn      func(ctx context.Context, groupID uuid.UUID, offset, limit int) ([]*db.Host, int64, error)
}

func (m *mockGroupRepo) CreateGroup(ctx context.Context, input db.CreateGroupInput) (*db.HostGroup, error) {
	if m.createGroupFn != nil {
		return m.createGroupFn(ctx, input)
	}
	panic("mockGroupRepo: CreateGroup called unexpectedly")
}

func (m *mockGroupRepo) GetGroup(ctx context.Context, id uuid.UUID) (*db.HostGroup, error) {
	if m.getGroupFn != nil {
		return m.getGroupFn(ctx, id)
	}
	panic("mockGroupRepo: GetGroup called unexpectedly")
}

func (m *mockGroupRepo) ListGroups(ctx context.Context) ([]*db.HostGroup, error) {
	if m.listGroupsFn != nil {
		return m.listGroupsFn(ctx)
	}
	panic("mockGroupRepo: ListGroups called unexpectedly")
}

func (m *mockGroupRepo) UpdateGroup(
	ctx context.Context, id uuid.UUID, input db.UpdateGroupInput,
) (*db.HostGroup, error) {
	if m.updateGroupFn != nil {
		return m.updateGroupFn(ctx, id, input)
	}
	panic("mockGroupRepo: UpdateGroup called unexpectedly")
}

func (m *mockGroupRepo) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	if m.deleteGroupFn != nil {
		return m.deleteGroupFn(ctx, id)
	}
	panic("mockGroupRepo: DeleteGroup called unexpectedly")
}

func (m *mockGroupRepo) AddHostsToGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	if m.addHostsToGroupFn != nil {
		return m.addHostsToGroupFn(ctx, groupID, hostIDs)
	}
	panic("mockGroupRepo: AddHostsToGroup called unexpectedly")
}

func (m *mockGroupRepo) RemoveHostsFromGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	if m.removeHostsFromGroupFn != nil {
		return m.removeHostsFromGroupFn(ctx, groupID, hostIDs)
	}
	panic("mockGroupRepo: RemoveHostsFromGroup called unexpectedly")
}

func (m *mockGroupRepo) GetGroupMembers(
	ctx context.Context, groupID uuid.UUID, offset, limit int,
) ([]*db.Host, int64, error) {
	if m.getGroupMembersFn != nil {
		return m.getGroupMembersFn(ctx, groupID, offset, limit)
	}
	panic("mockGroupRepo: GetGroupMembers called unexpectedly")
}

// ---------------------------------------------------------------------------
// NewGroupService
// ---------------------------------------------------------------------------

func TestNewGroupService(t *testing.T) {
	repo := &mockGroupRepo{}
	svc := NewGroupService(repo, slog.Default())
	require.NotNil(t, svc)
}

// ---------------------------------------------------------------------------
// CreateGroup
// ---------------------------------------------------------------------------

func TestCreateGroup_EmptyNameReturnsError(t *testing.T) {
	ctx := context.Background()

	// createGroupFn left nil — any accidental call panics.
	repo := &mockGroupRepo{}
	svc := NewGroupService(repo, slog.Default())

	got, err := svc.CreateGroup(ctx, db.CreateGroupInput{Name: ""})

	require.Error(t, err)
	assert.Nil(t, got)
}

func TestCreateGroup_ValidInputDelegates(t *testing.T) {
	ctx := context.Background()
	input := db.CreateGroupInput{Name: "infra", Description: "infrastructure team"}
	want := &db.HostGroup{ID: uuid.New(), Name: "infra", Description: "infrastructure team"}

	var gotInput db.CreateGroupInput
	repo := &mockGroupRepo{
		createGroupFn: func(ctx context.Context, in db.CreateGroupInput) (*db.HostGroup, error) {
			gotInput = in
			return want, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.CreateGroup(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, input, gotInput)
}

func TestCreateGroup_PropagatesRepoError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("db constraint violated")

	repo := &mockGroupRepo{
		createGroupFn: func(ctx context.Context, in db.CreateGroupInput) (*db.HostGroup, error) {
			return nil, wantErr
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.CreateGroup(ctx, db.CreateGroupInput{Name: "infra"})

	require.Error(t, err)
	assert.Equal(t, wantErr, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// GetGroup
// ---------------------------------------------------------------------------

func TestGetGroup_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	want := &db.HostGroup{ID: id, Name: "production"}

	var gotID uuid.UUID
	repo := &mockGroupRepo{
		getGroupFn: func(ctx context.Context, in uuid.UUID) (*db.HostGroup, error) {
			gotID = in
			return want, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.GetGroup(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
}

func TestGetGroup_PropagatesNotFoundError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("group not found")

	repo := &mockGroupRepo{
		getGroupFn: func(ctx context.Context, id uuid.UUID) (*db.HostGroup, error) {
			return nil, wantErr
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.GetGroup(ctx, uuid.New())

	require.Error(t, err)
	assert.Equal(t, wantErr, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// ListGroups
// ---------------------------------------------------------------------------

func TestListGroups_ReturnsAllGroups(t *testing.T) {
	ctx := context.Background()
	want := []*db.HostGroup{
		{ID: uuid.New(), Name: "alpha"},
		{ID: uuid.New(), Name: "beta"},
		{ID: uuid.New(), Name: "gamma"},
	}

	repo := &mockGroupRepo{
		listGroupsFn: func(ctx context.Context) ([]*db.HostGroup, error) {
			return want, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.ListGroups(ctx)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListGroups_EmptyList(t *testing.T) {
	ctx := context.Background()

	repo := &mockGroupRepo{
		listGroupsFn: func(ctx context.Context) ([]*db.HostGroup, error) {
			return []*db.HostGroup{}, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.ListGroups(ctx)

	require.NoError(t, err)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// DeleteGroup
// ---------------------------------------------------------------------------

func TestDeleteGroup_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	repo := &mockGroupRepo{
		deleteGroupFn: func(ctx context.Context, in uuid.UUID) error {
			called = true
			assert.Equal(t, id, in)
			return nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	err := svc.DeleteGroup(ctx, id)

	require.NoError(t, err)
	assert.True(t, called, "expected DeleteGroup to be called on the repository")
}

func TestDeleteGroup_PropagatesError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("delete failed")

	repo := &mockGroupRepo{
		deleteGroupFn: func(ctx context.Context, id uuid.UUID) error {
			return wantErr
		},
	}

	svc := NewGroupService(repo, slog.Default())
	err := svc.DeleteGroup(ctx, uuid.New())

	assert.Equal(t, wantErr, err)
}

// ---------------------------------------------------------------------------
// AddHostsToGroup
// ---------------------------------------------------------------------------

func TestAddHostsToGroup_Delegates(t *testing.T) {
	ctx := context.Background()
	groupID := uuid.New()
	hostIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	var gotGroupID uuid.UUID
	var gotHostIDs []uuid.UUID

	repo := &mockGroupRepo{
		addHostsToGroupFn: func(ctx context.Context, gID uuid.UUID, hIDs []uuid.UUID) error {
			gotGroupID = gID
			gotHostIDs = hIDs
			return nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	err := svc.AddHostsToGroup(ctx, groupID, hostIDs)

	require.NoError(t, err)
	assert.Equal(t, groupID, gotGroupID)
	assert.Equal(t, hostIDs, gotHostIDs)
}

// ---------------------------------------------------------------------------
// RemoveHostsFromGroup
// ---------------------------------------------------------------------------

func TestRemoveHostsFromGroup_Delegates(t *testing.T) {
	ctx := context.Background()
	groupID := uuid.New()
	hostIDs := []uuid.UUID{uuid.New(), uuid.New()}

	var gotGroupID uuid.UUID
	var gotHostIDs []uuid.UUID

	repo := &mockGroupRepo{
		removeHostsFromGroupFn: func(ctx context.Context, gID uuid.UUID, hIDs []uuid.UUID) error {
			gotGroupID = gID
			gotHostIDs = hIDs
			return nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	err := svc.RemoveHostsFromGroup(ctx, groupID, hostIDs)

	require.NoError(t, err)
	assert.Equal(t, groupID, gotGroupID)
	assert.Equal(t, hostIDs, gotHostIDs)
}

// ---------------------------------------------------------------------------
// GetGroupMembers
// ---------------------------------------------------------------------------

func TestGetGroupMembers_ReturnsPaginatedResults(t *testing.T) {
	ctx := context.Background()
	groupID := uuid.New()
	wantHosts := []*db.Host{{ID: uuid.New()}, {ID: uuid.New()}}
	wantTotal := int64(5)

	var gotGroupID uuid.UUID
	var gotOffset, gotLimit int

	repo := &mockGroupRepo{
		getGroupMembersFn: func(
			ctx context.Context, gID uuid.UUID, offset, limit int,
		) ([]*db.Host, int64, error) {
			gotGroupID = gID
			gotOffset = offset
			gotLimit = limit
			return wantHosts, wantTotal, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, total, err := svc.GetGroupMembers(ctx, groupID, 0, 2)

	require.NoError(t, err)
	assert.Equal(t, wantHosts, got)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, groupID, gotGroupID)
	assert.Equal(t, 0, gotOffset)
	assert.Equal(t, 2, gotLimit)
}

// ---------------------------------------------------------------------------
// UpdateGroup
// ---------------------------------------------------------------------------

func TestUpdateGroup_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	name := "renamed"
	input := db.UpdateGroupInput{Name: &name}
	want := &db.HostGroup{ID: id, Name: name}

	var gotID uuid.UUID
	var gotInput db.UpdateGroupInput
	repo := &mockGroupRepo{
		updateGroupFn: func(ctx context.Context, in uuid.UUID, inp db.UpdateGroupInput) (*db.HostGroup, error) {
			gotID = in
			gotInput = inp
			return want, nil
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.UpdateGroup(ctx, id, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
	assert.Equal(t, input, gotInput)
}

func TestUpdateGroup_PropagatesError(t *testing.T) {
	ctx := context.Background()
	wantErr := fmt.Errorf("group not found")
	name := "x"

	repo := &mockGroupRepo{
		updateGroupFn: func(ctx context.Context, id uuid.UUID, input db.UpdateGroupInput) (*db.HostGroup, error) {
			return nil, wantErr
		},
	}

	svc := NewGroupService(repo, slog.Default())
	got, err := svc.UpdateGroup(ctx, uuid.New(), db.UpdateGroupInput{Name: &name})

	require.Error(t, err)
	assert.Equal(t, wantErr, err)
	assert.Nil(t, got)
}
