// Package services provides business logic services for Scanorama.
// This file implements scan profile management functionality including
// CRUD operations and profile cloning.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anstrom/scanorama/internal/db"
)

// profileRepository defines the data-access operations required by ProfileService.
type profileRepository interface {
	ListProfiles(ctx context.Context, filters db.ProfileFilters, offset, limit int) ([]*db.ScanProfile, int64, error)
	CreateProfile(ctx context.Context, input db.CreateProfileInput) (*db.ScanProfile, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
	UpdateProfile(ctx context.Context, id string, input db.UpdateProfileInput) (*db.ScanProfile, error)
	DeleteProfile(ctx context.Context, id string) error
}

// ProfileService handles business logic for scan profile management.
type ProfileService struct {
	repo   profileRepository
	logger *slog.Logger
}

// NewProfileService creates a new ProfileService with the provided repository and logger.
func NewProfileService(repo profileRepository, logger *slog.Logger) *ProfileService {
	return &ProfileService{
		repo:   repo,
		logger: logger,
	}
}

// ListProfiles returns a paginated list of profiles matching the given filters.
func (s *ProfileService) ListProfiles(
	ctx context.Context, filters db.ProfileFilters, offset, limit int,
) ([]*db.ScanProfile, int64, error) {
	return s.repo.ListProfiles(ctx, filters, offset, limit)
}

// CreateProfile creates a new scan profile record.
func (s *ProfileService) CreateProfile(ctx context.Context, input db.CreateProfileInput) (*db.ScanProfile, error) {
	return s.repo.CreateProfile(ctx, input)
}

// GetProfile retrieves a single scan profile by its ID.
func (s *ProfileService) GetProfile(ctx context.Context, id string) (*db.ScanProfile, error) {
	return s.repo.GetProfile(ctx, id)
}

// UpdateProfile applies the provided changes to an existing scan profile.
func (s *ProfileService) UpdateProfile(
	ctx context.Context, id string, input db.UpdateProfileInput,
) (*db.ScanProfile, error) {
	return s.repo.UpdateProfile(ctx, id, input)
}

// DeleteProfile removes a scan profile by its ID.
func (s *ProfileService) DeleteProfile(ctx context.Context, id string) error {
	return s.repo.DeleteProfile(ctx, id)
}

// CloneProfile creates a new scan profile based on an existing one, using newName
// as the name of the clone. Scripts are not copied to the clone.
func (s *ProfileService) CloneProfile(ctx context.Context, fromID, newName string) (*db.ScanProfile, error) {
	source, err := s.repo.GetProfile(ctx, fromID)
	if err != nil {
		return nil, fmt.Errorf("cloning profile %s: %w", fromID, err)
	}

	input := db.CreateProfileInput{
		Name:        newName,
		Description: fmt.Sprintf("Clone of %s", source.Name),
		ScanType:    source.ScanType,
		Ports:       source.Ports,
		Timing:      source.Timing,
	}

	if len(source.Options) > 0 {
		var opts map[string]interface{}
		if err := json.Unmarshal([]byte(source.Options), &opts); err == nil {
			input.Options = opts
		}
	}

	return s.repo.CreateProfile(ctx, input)
}
