// Package db provides network and discovery-related database operations for scanorama.
package db

import (
	"context"
)

// NetworkSummaryRepository handles network summary operations.
type NetworkSummaryRepository struct {
	db *DB
}

// NewNetworkSummaryRepository creates a new network summary repository.
func NewNetworkSummaryRepository(db *DB) *NetworkSummaryRepository {
	return &NetworkSummaryRepository{db: db}
}

// GetAll retrieves all network summaries.
func (r *NetworkSummaryRepository) GetAll(ctx context.Context) ([]*NetworkSummary, error) {
	var summaries []*NetworkSummary
	query := `
		SELECT *
		FROM network_summary
		ORDER BY target_name`

	if err := r.db.SelectContext(ctx, &summaries, query); err != nil {
		return nil, sanitizeDBError("get network summaries", err)
	}

	return summaries, nil
}

// DiscoveryFilters represents filters for listing discovery jobs.
type DiscoveryFilters struct {
	Status string
	Method string
}
