// Package cli provides command-line interface commands for the Scanorama network scanner.
package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetNextRunTime verifies that getNextRunTime correctly parses cron
// expressions and returns a future time relative to now.
func TestGetNextRunTime(t *testing.T) {
	t.Run("every minute returns time within next 60 seconds", func(t *testing.T) {
		before := time.Now()
		result := getNextRunTime("* * * * *")
		after := time.Now()

		require.False(t, result.IsZero(), "should not return zero time for valid expression")
		assert.True(t, result.After(before), "next run should be after now")
		assert.True(t, result.Before(after.Add(61*time.Second)),
			"next run for '* * * * *' should be within the next 61 seconds")
	})

	t.Run("hourly expression returns time within next hour", func(t *testing.T) {
		before := time.Now()
		result := getNextRunTime("0 * * * *")

		require.False(t, result.IsZero())
		assert.True(t, result.After(before))
		assert.True(t, result.Before(before.Add(61*time.Minute)),
			"next run for '0 * * * *' should be within the next 61 minutes")
	})

	t.Run("daily midnight expression returns time within next 24 hours", func(t *testing.T) {
		before := time.Now()
		result := getNextRunTime("0 0 * * *")

		require.False(t, result.IsZero())
		assert.True(t, result.After(before))
		assert.True(t, result.Before(before.Add(25*time.Hour)),
			"next run for '0 0 * * *' should be within the next 25 hours")
	})

	t.Run("weekly expression returns time within next 7 days", func(t *testing.T) {
		before := time.Now()
		result := getNextRunTime("0 0 * * 0")

		require.False(t, result.IsZero())
		assert.True(t, result.After(before))
		assert.True(t, result.Before(before.Add(8*24*time.Hour)),
			"next run for '0 0 * * 0' should be within the next 8 days")
	})

	t.Run("every 5 minutes returns time within next 5 minutes", func(t *testing.T) {
		before := time.Now()
		result := getNextRunTime("*/5 * * * *")

		require.False(t, result.IsZero())
		assert.True(t, result.After(before))
		assert.True(t, result.Before(before.Add(6*time.Minute)),
			"next run for '*/5 * * * *' should be within the next 6 minutes")
	})

	t.Run("invalid expression returns zero time", func(t *testing.T) {
		result := getNextRunTime("not a cron expression")
		assert.True(t, result.IsZero(), "invalid expression should return zero time")
	})

	t.Run("empty string returns zero time", func(t *testing.T) {
		result := getNextRunTime("")
		assert.True(t, result.IsZero(), "empty expression should return zero time")
	})

	t.Run("out of range field returns zero time", func(t *testing.T) {
		// Month field max is 12; 13 is invalid.
		result := getNextRunTime("0 0 1 13 *")
		assert.True(t, result.IsZero(), "out-of-range month should return zero time")
	})

	t.Run("too few fields returns zero time", func(t *testing.T) {
		result := getNextRunTime("* * *")
		assert.True(t, result.IsZero(), "expression with too few fields should return zero time")
	})

	t.Run("result is always strictly after now", func(t *testing.T) {
		expressions := []string{
			"* * * * *",
			"0 * * * *",
			"0 0 * * *",
			"*/15 * * * *",
			"30 6 * * 1-5",
		}
		for _, expr := range expressions {
			t.Run(expr, func(t *testing.T) {
				before := time.Now()
				result := getNextRunTime(expr)
				require.False(t, result.IsZero(), "valid expression should not return zero time")
				assert.True(t, result.After(before),
					"next run time must be strictly in the future")
			})
		}
	})
}

// TestGetNextRunTime_Idempotent verifies that calling getNextRunTime twice in
// quick succession with the same expression returns the same truncated minute,
// since cron granularity is per-minute.
func TestGetNextRunTime_Idempotent(t *testing.T) {
	expr := "*/5 * * * *"

	first := getNextRunTime(expr)
	second := getNextRunTime(expr)

	require.False(t, first.IsZero())
	require.False(t, second.IsZero())

	// Both calls happen within the same second, so both should land on the
	// same 5-minute boundary (or an adjacent one at most).
	diff := second.Sub(first)
	if diff < 0 {
		diff = -diff
	}
	assert.True(t, diff <= 5*time.Minute,
		"two consecutive calls should agree on the same or adjacent boundary")
}
