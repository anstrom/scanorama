// Package services provides business logic for host group management.
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

// groupRepository defines the data-access operations required by GroupService.
//
//nolint:dupl // mirrors the mockGroupRepo in group_test.go; duplication is unavoidable
type groupRepository interface {
	CreateGroup(ctx context.Context, input db.CreateGroupInput) (*db.HostGroup, error)
	GetGroup(ctx context.Context, id uuid.UUID) (*db.HostGroup, error)
	ListGroups(ctx context.Context) ([]*db.HostGroup, error)
	UpdateGroup(ctx context.Context, id uuid.UUID, input db.UpdateGroupInput) (*db.HostGroup, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	AddHostsToGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	RemoveHostsFromGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error
	GetGroupMembers(ctx context.Context, groupID uuid.UUID, offset, limit int) ([]*db.Host, int64, error)
}

// GroupService handles business logic for host group management.
type GroupService struct {
	repo   groupRepository
	logger *slog.Logger
}

// NewGroupService creates a new GroupService.
func NewGroupService(repo groupRepository, logger *slog.Logger) *GroupService {
	return &GroupService{
		repo:   repo,
		logger: logger.With("service", "group"),
	}
}

// CreateGroup validates the input and creates a new host group.
func (s *GroupService) CreateGroup(ctx context.Context, input db.CreateGroupInput) (*db.HostGroup, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("group name is required")
	}
	return s.repo.CreateGroup(ctx, input)
}

// GetGroup retrieves a single host group by its UUID.
func (s *GroupService) GetGroup(ctx context.Context, id uuid.UUID) (*db.HostGroup, error) {
	return s.repo.GetGroup(ctx, id)
}

// ListGroups returns all host groups.
func (s *GroupService) ListGroups(ctx context.Context) ([]*db.HostGroup, error) {
	return s.repo.ListGroups(ctx)
}

// UpdateGroup applies the provided changes to an existing host group.
func (s *GroupService) UpdateGroup(
	ctx context.Context, id uuid.UUID, input db.UpdateGroupInput,
) (*db.HostGroup, error) {
	return s.repo.UpdateGroup(ctx, id, input)
}

// DeleteGroup removes a host group by its UUID.
func (s *GroupService) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteGroup(ctx, id)
}

// AddHostsToGroup adds one or more hosts to the specified group.
func (s *GroupService) AddHostsToGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	return s.repo.AddHostsToGroup(ctx, groupID, hostIDs)
}

// RemoveHostsFromGroup removes one or more hosts from the specified group.
func (s *GroupService) RemoveHostsFromGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	return s.repo.RemoveHostsFromGroup(ctx, groupID, hostIDs)
}

// GetGroupMembers returns a paginated list of hosts belonging to the given group.
func (s *GroupService) GetGroupMembers(
	ctx context.Context, groupID uuid.UUID, offset, limit int,
) ([]*db.Host, int64, error) {
	return s.repo.GetGroupMembers(ctx, groupID, offset, limit)
}
