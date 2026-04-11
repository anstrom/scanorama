// Package services provides business logic for Scanorama operations.
// This file implements the pure knowledge-score calculation used to
// summarize how much is known about a discovered host.
package services

// ScoreInput holds the boolean inputs used in the pure score calculation.
// This type is exposed for testing without database access.
type ScoreInput struct {
	HasOSFamily   bool
	HasOpenPorts  bool
	HasServices   bool
	IsFresh       bool // last_seen within 7 days
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
