package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
