// Package cli provides command-line interface commands for the Scanorama network scanner.
package cli

import (
	"encoding/json"
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

// TestDisplayScheduledJobsJSON verifies that displayScheduledJobsJSON emits
// valid JSON with the expected top-level "jobs" array and "count" field.
func TestDisplayScheduledJobsJSON(t *testing.T) {
	fixedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	nextRun := time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC)
	lastRun := time.Date(2024, 5, 31, 0, 0, 0, 0, time.UTC)

	t.Run("empty list produces valid JSON with count zero", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobsJSON([]ScheduledJob{})
		})

		require.NotEmpty(t, output)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result), "output must be valid JSON")

		assert.Contains(t, result, "jobs")
		assert.Contains(t, result, "count")
		assert.Equal(t, float64(0), result["count"])

		jobs, ok := result["jobs"].([]interface{})
		require.True(t, ok, "jobs must be an array")
		assert.Empty(t, jobs)
	})

	t.Run("single job produces correct JSON fields", func(t *testing.T) {
		jobs := []ScheduledJob{
			{
				ID:        "abc-123",
				Name:      "weekly-sweep",
				JobType:   "discovery",
				CronExpr:  "0 2 * * 0",
				IsActive:  true,
				CreatedAt: fixedTime,
				LastRun:   &lastRun,
				NextRun:   nextRun,
				RunCount:  1,
			},
		}

		output := captureStdout(t, func() {
			displayScheduledJobsJSON(jobs)
		})

		require.NotEmpty(t, output)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result), "output must be valid JSON")

		assert.Equal(t, float64(1), result["count"])

		jobsArr, ok := result["jobs"].([]interface{})
		require.True(t, ok)
		require.Len(t, jobsArr, 1)

		job := jobsArr[0].(map[string]interface{})
		assert.Equal(t, "abc-123", job["id"])
		assert.Equal(t, "weekly-sweep", job["name"])
		assert.Equal(t, "discovery", job["job_type"])
		assert.Equal(t, "0 2 * * 0", job["cron_expr"])
		assert.Equal(t, true, job["is_active"])
		assert.Contains(t, job, "created_at")
		assert.Contains(t, job, "last_run")
		assert.Contains(t, job, "next_run")
		assert.Equal(t, float64(1), job["run_count"])
	})

	t.Run("multiple jobs produce correct count", func(t *testing.T) {
		jobs := []ScheduledJob{
			{
				ID: "id-1", Name: "job-one", JobType: "discovery",
				CronExpr: "0 1 * * *", CreatedAt: fixedTime, NextRun: nextRun,
			},
			{
				ID: "id-2", Name: "job-two", JobType: "scan",
				CronExpr: "0 2 * * *", CreatedAt: fixedTime, NextRun: nextRun,
			},
		}

		output := captureStdout(t, func() {
			displayScheduledJobsJSON(jobs)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		assert.Equal(t, float64(2), result["count"])

		jobsArr, ok := result["jobs"].([]interface{})
		require.True(t, ok)
		assert.Len(t, jobsArr, 2)
	})

	t.Run("output contains indentation", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobsJSON([]ScheduledJob{})
		})
		assert.Contains(t, output, "  ", "JSON output should be indented")
	})

	t.Run("job with nil last_run serializes as null", func(t *testing.T) {
		jobs := []ScheduledJob{
			{
				ID: "no-run", Name: "never-run", JobType: "scan",
				CronExpr: "0 3 * * *", CreatedAt: fixedTime, NextRun: nextRun,
				LastRun: nil,
			},
		}

		output := captureStdout(t, func() {
			displayScheduledJobsJSON(jobs)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		jobsArr := result["jobs"].([]interface{})
		job := jobsArr[0].(map[string]interface{})
		assert.Nil(t, job["last_run"])
	})
}

// TestDisplayScheduledJobDetailsJSON verifies that displayScheduledJobDetailsJSON
// emits a single-job JSON object with all expected fields, including next_run.
func TestDisplayScheduledJobDetailsJSON(t *testing.T) {
	fixedTime := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	localNext := time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC)
	lastRun := time.Date(2024, 5, 31, 8, 0, 0, 0, time.UTC)

	baseJob := &ScheduledJob{
		ID:        "job-xyz",
		Name:      "daily-scan",
		JobType:   "scan",
		CronExpr:  "0 0 * * *",
		IsActive:  true,
		CreatedAt: fixedTime,
		LastRun:   &lastRun,
		NextRun:   localNext,
		RunCount:  5,
	}

	t.Run("all expected fields are present", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(baseJob, nil)
		})

		require.NotEmpty(t, output)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result), "output must be valid JSON")

		expectedFields := []string{
			"id", "name", "job_type", "cron_expr",
			"is_active", "created_at", "last_run", "next_run", "run_count",
		}
		for _, field := range expectedFields {
			assert.Contains(t, result, field, "JSON should contain field: %s", field)
		}
	})

	t.Run("field values match job struct", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(baseJob, nil)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		assert.Equal(t, "job-xyz", result["id"])
		assert.Equal(t, "daily-scan", result["name"])
		assert.Equal(t, "scan", result["job_type"])
		assert.Equal(t, "0 0 * * *", result["cron_expr"])
		assert.Equal(t, true, result["is_active"])
		assert.Equal(t, float64(5), result["run_count"])
	})

	t.Run("server next_run overrides locally computed value", func(t *testing.T) {
		serverTime := time.Date(2024, 6, 3, 6, 0, 0, 0, time.UTC)

		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(baseJob, &serverTime)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		nextRunStr, ok := result["next_run"].(string)
		require.True(t, ok, "next_run must be a string")

		parsed, err := time.Parse(time.RFC3339, nextRunStr)
		require.NoError(t, err)
		assert.True(t, serverTime.Equal(parsed),
			"next_run should reflect the server-provided time, got %s", nextRunStr)
	})

	t.Run("nil server next_run uses locally computed value", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(baseJob, nil)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		nextRunStr, ok := result["next_run"].(string)
		require.True(t, ok, "next_run must be a string")

		parsed, err := time.Parse(time.RFC3339, nextRunStr)
		require.NoError(t, err)
		assert.True(t, localNext.Equal(parsed),
			"next_run should reflect the local NextRun value when server is unavailable")
	})

	t.Run("job with nil last_run serializes last_run as null", func(t *testing.T) {
		jobNoLastRun := &ScheduledJob{
			ID:        "no-run",
			Name:      "fresh-job",
			JobType:   "discovery",
			CronExpr:  "0 1 * * *",
			IsActive:  false,
			CreatedAt: fixedTime,
			LastRun:   nil,
			NextRun:   localNext,
		}

		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(jobNoLastRun, nil)
		})

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &result))

		assert.Nil(t, result["last_run"])
	})

	t.Run("output contains indentation", func(t *testing.T) {
		output := captureStdout(t, func() {
			displayScheduledJobDetailsJSON(baseJob, nil)
		})
		assert.Contains(t, output, "  ", "JSON output should be indented")
	})
}
