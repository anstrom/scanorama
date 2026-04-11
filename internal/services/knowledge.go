// Package services provides business logic for Scanorama operations.
// This file implements the knowledge score feature: a 0-100 integer that
// summarizes how much is known about a discovered host.
package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// knowledgeRepository is the DB interface needed by KnowledgeService.
type knowledgeRepository interface {
	RecalculateKnowledgeScore(ctx context.Context, hostID uuid.UUID) error
}

// KnowledgeService recalculates knowledge scores for hosts after enrichment.
type KnowledgeService struct {
	repo   knowledgeRepository
	logger *slog.Logger
}

// NewKnowledgeService creates a new KnowledgeService.
func NewKnowledgeService(repo knowledgeRepository, logger *slog.Logger) *KnowledgeService {
	return &KnowledgeService{repo: repo, logger: logger}
}

// RecalculateHostScore recomputes and persists the knowledge_score for a
// single host. It is safe to call concurrently for different host IDs.
// Errors are returned so callers can decide whether to log or ignore them.
func (s *KnowledgeService) RecalculateHostScore(ctx context.Context, hostID uuid.UUID) error {
	if err := s.repo.RecalculateKnowledgeScore(ctx, hostID); err != nil {
		s.logger.Warn("failed to recalculate knowledge score",
			"host_id", hostID, "error", err)
		return err
	}
	return nil
}

// RecalculateScores recomputes knowledge scores for a slice of host IDs.
// Individual failures are logged but do not abort the remaining updates.
func (s *KnowledgeService) RecalculateScores(ctx context.Context, hostIDs []uuid.UUID) {
	for _, id := range hostIDs {
		if err := s.repo.RecalculateKnowledgeScore(ctx, id); err != nil {
			s.logger.Warn("failed to recalculate knowledge score",
				"host_id", id, "error", err)
		}
	}
}

// ScoreInput holds the boolean inputs used in the pure score calculation.
// This type is exposed for testing without database access.
type ScoreInput struct {
	HasOSFamily  bool
	HasOpenPorts bool
	HasServices  bool
	IsFresh      bool // last_seen within 7 days
	HasEnrichment bool // banners or SNMP data present
}

const scorePointsPerFactor = 20

// CalculateScore returns a 0-100 score from the provided boolean inputs.
// Each of the five factors contributes 20 points.
func CalculateScore(in ScoreInput) int {
	score := 0
	if in.HasOSFamily {
		score += scorePointsPerFactor
	}
	if in.HasOpenPorts {
		score += scorePointsPerFactor
	}
	if in.HasServices {
		score += scorePointsPerFactor
	}
	if in.IsFresh {
		score += scorePointsPerFactor
	}
	if in.HasEnrichment {
		score += scorePointsPerFactor
	}
	return score
}
