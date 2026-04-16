// Package services provides business logic services for Scanorama.
// This file implements host management functionality including CRUD operations
// and host scan history retrieval.
package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// hostRepository defines the data-access operations required by HostService.
//
//nolint:dupl // interface mirrored by mockHostRepo in host_test.go; duplication is unavoidable
type hostRepository interface {
	ListHosts(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
	CreateHost(ctx context.Context, input db.CreateHostInput) (*db.Host, error)
	GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error)
	UpdateHost(ctx context.Context, id uuid.UUID, input db.UpdateHostInput) (*db.Host, error)
	UpdateCustomName(ctx context.Context, id uuid.UUID, name *string) (*db.Host, error)
	DeleteHost(ctx context.Context, id uuid.UUID) error
	BulkDeleteHosts(ctx context.Context, ids []uuid.UUID) (int64, error)
	GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error)
	GetAllTags(ctx context.Context) ([]string, error)
	UpdateHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	AddHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	RemoveHostTags(ctx context.Context, id uuid.UUID, tags []string) error
	BulkUpdateTags(ctx context.Context, ids []uuid.UUID, tags []string, action string) error
	GetHostGroups(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error)
	GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error)
}

// HostService handles business logic for host management.
type HostService struct {
	repo   hostRepository
	logger *slog.Logger
}

// NewHostService creates a new HostService with the provided repository and logger.
func NewHostService(repo hostRepository, logger *slog.Logger) *HostService {
	return &HostService{
		repo:   repo,
		logger: logger,
	}
}

// ListHosts returns a paginated list of hosts matching the given filters.
func (s *HostService) ListHosts(
	ctx context.Context, filters *db.HostFilters, offset, limit int,
) ([]*db.Host, int64, error) {
	return s.repo.ListHosts(ctx, filters, offset, limit)
}

// CreateHost validates the input and creates a new host record.
// It returns a validation error if the IP address is empty.
func (s *HostService) CreateHost(ctx context.Context, input db.CreateHostInput) (*db.Host, error) {
	if input.IPAddress == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "host IP is required")
	}

	return s.repo.CreateHost(ctx, input)
}

// GetHost retrieves a single host by its UUID.
func (s *HostService) GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error) {
	return s.repo.GetHost(ctx, id)
}

// UpdateHost applies the provided changes to an existing host record.
func (s *HostService) UpdateHost(ctx context.Context, id uuid.UUID, input db.UpdateHostInput) (*db.Host, error) {
	return s.repo.UpdateHost(ctx, id, input)
}

// UpdateCustomName sets or clears the user-defined display-name override for
// a host. Pass nil to clear.
func (s *HostService) UpdateCustomName(
	ctx context.Context, id uuid.UUID, name *string,
) (*db.Host, error) {
	return s.repo.UpdateCustomName(ctx, id, name)
}

// DeleteHost removes a host record by its UUID.
func (s *HostService) DeleteHost(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteHost(ctx, id)
}

// BulkDeleteHosts deletes multiple hosts and returns the count of deleted rows.
func (s *HostService) BulkDeleteHosts(ctx context.Context, ids []uuid.UUID) (int64, error) {
	return s.repo.BulkDeleteHosts(ctx, ids)
}

// GetHostScans returns a paginated list of scans associated with the given host.
func (s *HostService) GetHostScans(
	ctx context.Context, hostID uuid.UUID, offset, limit int,
) ([]*db.Scan, int64, error) {
	return s.repo.GetHostScans(ctx, hostID, offset, limit)
}

// ListTags returns a deduplicated, sorted list of all tags in use across all hosts.
func (s *HostService) ListTags(ctx context.Context) ([]string, error) {
	return s.repo.GetAllTags(ctx)
}

// UpdateHostTags replaces the entire tag list for the given host.
func (s *HostService) UpdateHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	return s.repo.UpdateHostTags(ctx, id, tags)
}

// AddHostTags appends tags to the host's tag list, deduplicating the result.
func (s *HostService) AddHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	return s.repo.AddHostTags(ctx, id, tags)
}

// RemoveHostTags removes the specified tags from the host's tag list.
func (s *HostService) RemoveHostTags(ctx context.Context, id uuid.UUID, tags []string) error {
	return s.repo.RemoveHostTags(ctx, id, tags)
}

// BulkUpdateTags applies an add/remove/set tag operation to multiple hosts at once.
func (s *HostService) BulkUpdateTags(ctx context.Context, ids []uuid.UUID, tags []string, action string) error {
	return s.repo.BulkUpdateTags(ctx, ids, tags, action)
}

// GetHostGroups returns the groups the given host belongs to.
func (s *HostService) GetHostGroups(ctx context.Context, hostID uuid.UUID) ([]db.HostGroupSummary, error) {
	return s.repo.GetHostGroups(ctx, hostID)
}

// GetHostNetworks returns the registered networks that contain the host's
// IP address, ordered by longest prefix first. Membership is derived from
// the host_network_memberships view; no row is stored. An empty slice
// (never nil) is returned when the host belongs to no registered network.
func (s *HostService) GetHostNetworks(ctx context.Context, hostID uuid.UUID) ([]*db.Network, error) {
	return s.repo.GetHostNetworks(ctx, hostID)
}
