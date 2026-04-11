package services

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("test error")

// ── CalculateScore ────────────────────────────────────────────────────────────

func TestCalculateScore_AllFactors(t *testing.T) {
	score := CalculateScore(ScoreInput{
		HasOSFamily:   true,
		HasOpenPorts:  true,
		HasServices:   true,
		IsFresh:       true,
		HasEnrichment: true,
	})
	assert.Equal(t, 100, score)
}

func TestCalculateScore_NoFactors(t *testing.T) {
	score := CalculateScore(ScoreInput{})
	assert.Equal(t, 0, score)
}

func TestCalculateScore_OSOnly(t *testing.T) {
	score := CalculateScore(ScoreInput{HasOSFamily: true})
	assert.Equal(t, 20, score)
}

func TestCalculateScore_PortsAndFreshness(t *testing.T) {
	score := CalculateScore(ScoreInput{HasOpenPorts: true, IsFresh: true})
	assert.Equal(t, 40, score)
}

func TestCalculateScore_EachFactorContributes20(t *testing.T) {
	cases := []struct {
		in    ScoreInput
		label string
	}{
		{ScoreInput{HasOSFamily: true}, "os_family"},
		{ScoreInput{HasOpenPorts: true}, "open_ports"},
		{ScoreInput{HasServices: true}, "services"},
		{ScoreInput{IsFresh: true}, "freshness"},
		{ScoreInput{HasEnrichment: true}, "enrichment"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			assert.Equal(t, 20, CalculateScore(tc.in))
		})
	}
}

// ── KnowledgeService ──────────────────────────────────────────────────────────

type mockKnowledgeRepo struct {
	calls []uuid.UUID
	err   error
}

func (m *mockKnowledgeRepo) RecalculateKnowledgeScore(_ context.Context, id uuid.UUID) error {
	m.calls = append(m.calls, id)
	return m.err
}

func TestKnowledgeService_RecalculateHostScore_OK(t *testing.T) {
	repo := &mockKnowledgeRepo{}
	svc := NewKnowledgeService(repo, slog.Default())
	id := uuid.New()

	err := svc.RecalculateHostScore(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, repo.calls, 1)
	assert.Equal(t, id, repo.calls[0])
}

func TestKnowledgeService_RecalculateHostScore_Error(t *testing.T) {
	repo := &mockKnowledgeRepo{err: errTest}
	svc := NewKnowledgeService(repo, slog.Default())

	err := svc.RecalculateHostScore(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestKnowledgeService_RecalculateScores_AllHostsUpdated(t *testing.T) {
	repo := &mockKnowledgeRepo{}
	svc := NewKnowledgeService(repo, slog.Default())
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	svc.RecalculateScores(context.Background(), ids)
	assert.Len(t, repo.calls, 3)
}

func TestKnowledgeService_RecalculateScores_ContinuesOnError(t *testing.T) {
	// Even when the repo returns errors, all IDs should be attempted.
	repo := &mockKnowledgeRepo{err: errTest}
	svc := NewKnowledgeService(repo, slog.Default())
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	svc.RecalculateScores(context.Background(), ids)
	assert.Len(t, repo.calls, 2, "all IDs should be attempted even if errors occur")
}
