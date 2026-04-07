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
	DeleteHost(ctx context.Context, id uuid.UUID) error
	BulkDeleteHosts(ctx context.Context, ids []uuid.UUID) (int64, error)
	GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*db.Scan, int64, error)
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
