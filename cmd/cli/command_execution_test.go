// Package cli provides white-box unit tests for the cli package.
package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// TestGetVersion
// ----------------------------------------------------------------------------

func TestGetVersion(t *testing.T) {
	t.Run("default build vars", func(t *testing.T) {
		// Save and restore build-info globals altered by other tests.
		origVersion, origCommit, origBuildTime := version, commit, buildTime
		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
			rootCmd.Version = getVersion()
		}()

		version = "dev"
		commit = "none"
		buildTime = "unknown"

		result := getVersion()

		assert.Contains(t, result, "dev", "default version should contain 'dev'")
		assert.Contains(t, result, "none", "default commit should contain 'none'")
		assert.Contains(t, result, "unknown", "default buildTime should contain 'unknown'")
	})

	t.Run("SetVersion propagates all three fields", func(t *testing.T) {
		origVersion, origCommit, origBuildTime := version, commit, buildTime
		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
			rootCmd.Version = getVersion()
		}()

		SetVersion("1.2.3", "abc123", "2024-01-01")

		result := getVersion()

		assert.Contains(t, result, "1.2.3", "version string should contain the supplied version")
		assert.Contains(t, result, "abc123", "version string should contain the supplied commit")
		assert.Contains(t, result, "2024-01-01", "version string should contain the supplied build time")
	})

	t.Run("SetVersion updates rootCmd.Version", func(t *testing.T) {
		origVersion, origCommit, origBuildTime := version, commit, buildTime
		defer func() {
			version = origVersion
			commit = origCommit
			buildTime = origBuildTime
			rootCmd.Version = getVersion()
		}()

		SetVersion("9.8.7", "deadbeef", "2099-12-31")

		assert.Equal(t, getVersion(), rootCmd.Version,
			"rootCmd.Version must mirror getVersion() after SetVersion call")
	})
}

// ----------------------------------------------------------------------------
// TestValidateCronExpression
// ----------------------------------------------------------------------------

func TestValidateCronExpression(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		// Valid 5-field expressions
		{name: "every minute", expr: "* * * * *", wantErr: false},
		{name: "weekly at 2am Sunday", expr: "0 2 * * 0", wantErr: false},
		{name: "every 5 minutes", expr: "*/5 * * * *", wantErr: false},
		{name: "weekday mornings", expr: "30 6 * * 1-5", wantErr: false},
		{name: "monthly", expr: "0 0 1 * *", wantErr: false},

		// Too few fields
		{name: "three fields", expr: "* * *", wantErr: true},
		{name: "four fields", expr: "0 2 * *", wantErr: true},
		{name: "one field", expr: "*", wantErr: true},

		// Too many fields
		{name: "six fields", expr: "0 0 0 * * *", wantErr: true},
		{name: "seven fields", expr: "0 0 * * * * *", wantErr: true},

		// Empty string
		{name: "empty string", expr: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCronExpression(tc.expr)
			if tc.wantErr {
				assert.Error(t, err, "expr %q: expected an error but got none", tc.expr)
			} else {
				assert.NoError(t, err, "expr %q: expected no error but got: %v", tc.expr, err)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestTruncateString
// ----------------------------------------------------------------------------

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "shorter than maxLen returned as-is",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exactly at maxLen returned as-is",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "longer than maxLen gets ellipsis",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "total length equals maxLen after truncation",
			input:  "abcdefghij",
			maxLen: 7,
			want:   "abcd...",
		},
		{
			name:   "edge: maxLen=3 with longer string returns only ellipsis",
			input:  "toolong",
			maxLen: 3,
			want:   "...",
		},
		{
			name:   "empty string returned as-is",
			input:  "",
			maxLen: 5,
			want:   "",
		},
		{
			name:   "exactly maxLen=3 string returned as-is",
			input:  "abc",
			maxLen: 3,
			want:   "abc",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateString(tc.input, tc.maxLen)
			assert.Equal(t, tc.want, got)

			// Verify the invariant: result is never longer than maxLen.
			assert.LessOrEqual(t, len(got), tc.maxLen,
				"truncateString must never return a string longer than maxLen")
		})
	}
}

// ----------------------------------------------------------------------------
// TestIsDaemonRunning
// ----------------------------------------------------------------------------

func TestIsDaemonRunning(t *testing.T) {
	origPidFile := daemonPidFile
	defer func() { daemonPidFile = origPidFile }()

	tempDir, err := os.MkdirTemp("", "scanorama-pid-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("no PID file present returns false", func(t *testing.T) {
		daemonPidFile = filepath.Join(tempDir, "nonexistent.pid")
		assert.False(t, isDaemonRunning())
	})

	t.Run("PID file with non-numeric content returns false", func(t *testing.T) {
		pidFile := filepath.Join(tempDir, "invalid.pid")
		require.NoError(t, os.WriteFile(pidFile, []byte("not-a-number"), 0o644))
		daemonPidFile = pidFile
		assert.False(t, isDaemonRunning())
	})

	t.Run("PID file with non-existent PID returns false", func(t *testing.T) {
		pidFile := filepath.Join(tempDir, "stale.pid")
		// PID 99999 is extremely unlikely to be a live process on any OS.
		require.NoError(t, os.WriteFile(pidFile, []byte("99999"), 0o644))
		daemonPidFile = pidFile
		assert.False(t, isDaemonRunning())
	})

	t.Run("PID file with current process PID returns true", func(t *testing.T) {
		pidFile := filepath.Join(tempDir, "self.pid")
		require.NoError(t, os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644))
		daemonPidFile = pidFile
		assert.True(t, isDaemonRunning())
	})
}

// ----------------------------------------------------------------------------
// TestReadPIDFile
// ----------------------------------------------------------------------------

func TestReadPIDFile(t *testing.T) {
	origPidFile := daemonPidFile
	defer func() { daemonPidFile = origPidFile }()

	tempDir, err := os.MkdirTemp("", "scanorama-readpid-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("valid PID in file is returned", func(t *testing.T) {
		f := filepath.Join(tempDir, "valid.pid")
		require.NoError(t, os.WriteFile(f, []byte("12345"), 0o644))
		daemonPidFile = f

		pid, err := readPIDFile()
		require.NoError(t, err)
		assert.Equal(t, 12345, pid)
	})

	t.Run("PID with surrounding whitespace and newline", func(t *testing.T) {
		f := filepath.Join(tempDir, "whitespace.pid")
		require.NoError(t, os.WriteFile(f, []byte("  42\n"), 0o644))
		daemonPidFile = f

		pid, err := readPIDFile()
		require.NoError(t, err)
		assert.Equal(t, 42, pid)
	})

	t.Run("non-numeric content returns error", func(t *testing.T) {
		f := filepath.Join(tempDir, "bad.pid")
		require.NoError(t, os.WriteFile(f, []byte("not-a-pid"), 0o644))
		daemonPidFile = f

		_, err := readPIDFile()
		assert.Error(t, err)
	})

	t.Run("file does not exist returns error", func(t *testing.T) {
		daemonPidFile = filepath.Join(tempDir, "missing.pid")

		_, err := readPIDFile()
		assert.Error(t, err)
	})
}

// ----------------------------------------------------------------------------
// TestGetLogFileDesc
// ----------------------------------------------------------------------------

func TestGetLogFileDesc(t *testing.T) {
	origLogFile := daemonLogFile
	defer func() { daemonLogFile = origLogFile }()

	t.Run("empty daemonLogFile returns 'stdout'", func(t *testing.T) {
		daemonLogFile = ""
		assert.Equal(t, "stdout", getLogFileDesc())
	})

	t.Run("set path is returned verbatim", func(t *testing.T) {
		daemonLogFile = "/var/log/scanorama.log"
		assert.Equal(t, "/var/log/scanorama.log", getLogFileDesc())
	})

	t.Run("relative path is returned verbatim", func(t *testing.T) {
		daemonLogFile = "logs/scanorama.log"
		assert.Equal(t, "logs/scanorama.log", getLogFileDesc())
	})
}

// ----------------------------------------------------------------------------
// Helpers for stdout capture
// ----------------------------------------------------------------------------

// captureStdout redirects os.Stdout to a pipe, calls fn, then returns the
// captured output as a string.  os.Stdout is restored before returning.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err, "os.Pipe() must not fail")

	os.Stdout = w

	// Ensure we always restore, even if fn panics.
	defer func() { os.Stdout = origStdout }()

	fn()

	// Close the write end so ReadAll sees EOF.
	require.NoError(t, w.Close())

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	return buf.String()
}

// ----------------------------------------------------------------------------
// TestDisplayScheduledJobs
// ----------------------------------------------------------------------------

func TestDisplayScheduledJobs(t *testing.T) {
	t.Run("nil slice prints no-jobs message", func(t *testing.T) {
		out := captureStdout(t, func() {
			displayScheduledJobs(nil)
		})
		assert.Contains(t, out, "No scheduled jobs found")
	})

	t.Run("empty slice prints no-jobs message", func(t *testing.T) {
		out := captureStdout(t, func() {
			displayScheduledJobs([]ScheduledJob{})
		})
		assert.Contains(t, out, "No scheduled jobs found")
	})

	t.Run("one active and one inactive job", func(t *testing.T) {
		now := time.Now()
		jobs := []ScheduledJob{
			{
				ID:        "id-active",
				Name:      "active-scan",
				JobType:   "scan",
				CronExpr:  "*/5 * * * *",
				IsActive:  true,
				CreatedAt: now,
				NextRun:   now.Add(5 * time.Minute),
				RunCount:  3,
			},
			{
				ID:        "id-inactive",
				Name:      "inactive-discovery",
				JobType:   "discovery",
				CronExpr:  "0 2 * * 0",
				IsActive:  false,
				CreatedAt: now,
				NextRun:   now.Add(24 * time.Hour),
				RunCount:  0,
			},
		}

		out := captureStdout(t, func() {
			displayScheduledJobs(jobs)
		})

		// Job names must appear.
		assert.Contains(t, out, "active-scan", "active job name should be in output")
		assert.Contains(t, out, "inactive-discovery", "inactive job name should be in output")

		// Cron expressions must appear.
		assert.Contains(t, out, "*/5 * * * *", "active job cron expr should be in output")
		assert.Contains(t, out, "0 2 * * 0", "inactive job cron expr should be in output")

		// Active column must distinguish Yes / No.
		assert.Contains(t, out, "Yes", "active job should show 'Yes'")
		assert.Contains(t, out, "No", "inactive job should show 'No'")

		// Job type strings must appear.
		assert.Contains(t, out, "scan", "job type 'scan' should appear")
		assert.Contains(t, out, "discovery", "job type 'discovery' should appear")
	})

	t.Run("long job name is truncated in list output", func(t *testing.T) {
		longName := strings.Repeat("x", maxJobNameLength+10)
		jobs := []ScheduledJob{
			{
				Name:     longName,
				JobType:  "scan",
				CronExpr: "* * * * *",
				IsActive: true,
				NextRun:  time.Now().Add(time.Minute),
			},
		}

		out := captureStdout(t, func() {
			displayScheduledJobs(jobs)
		})

		// The full name should not appear; the truncated version should.
		assert.NotContains(t, out, longName,
			"full long name must not appear — it should be truncated")
		assert.Contains(t, out, "...",
			"ellipsis must appear for a truncated name")
	})
}

// ----------------------------------------------------------------------------
// TestDisplayScheduledJobDetails
// ----------------------------------------------------------------------------

func TestDisplayScheduledJobDetails(t *testing.T) {
	t.Run("all fields printed, LastRun nil shows Never", func(t *testing.T) {
		createdAt := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
		job := &ScheduledJob{
			ID:        "job-uuid-001",
			Name:      "nightly-scan",
			JobType:   "scan",
			CronExpr:  "0 22 * * *",
			IsActive:  true,
			CreatedAt: createdAt,
			LastRun:   nil, // never run yet
			NextRun:   createdAt.Add(14 * time.Hour),
			RunCount:  0,
		}

		out := captureStdout(t, func() {
			displayScheduledJobDetails(job)
		})

		assert.Contains(t, out, "job-uuid-001", "output must contain the job ID")
		assert.Contains(t, out, "scan", "output must contain the JobType")
		assert.Contains(t, out, "0 22 * * *", "output must contain the cron expression")
		assert.Contains(t, out, "Never", "output must say 'Never' when LastRun is nil")
		assert.Contains(t, out, createdAt.Format("2006-01-02 15:04:05"),
			"output must contain the formatted CreatedAt timestamp")
	})

	t.Run("LastRun non-nil shows formatted timestamp", func(t *testing.T) {
		lastRun := time.Date(2024, 6, 1, 8, 0, 0, 0, time.UTC)
		createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		job := &ScheduledJob{
			ID:        "job-uuid-002",
			Name:      "hourly-discovery",
			JobType:   "discovery",
			CronExpr:  "0 * * * *",
			IsActive:  true,
			CreatedAt: createdAt,
			LastRun:   &lastRun,
			NextRun:   lastRun.Add(time.Hour),
			RunCount:  42,
		}

		out := captureStdout(t, func() {
			displayScheduledJobDetails(job)
		})

		assert.Contains(t, out, "job-uuid-002", "output must contain the job ID")
		assert.Contains(t, out, "discovery", "output must contain the JobType")
		assert.Contains(t, out, "0 * * * *", "output must contain the cron expression")
		assert.NotContains(t, out, "Never",
			"'Never' must not appear when LastRun is set")
		assert.Contains(t, out, lastRun.Format("2006-01-02 15:04:05"),
			"output must contain the formatted LastRun timestamp")
		assert.Contains(t, out, createdAt.Format("2006-01-02 15:04:05"),
			"output must contain the formatted CreatedAt timestamp")
	})

	t.Run("output contains job name in header", func(t *testing.T) {
		createdAt := time.Now()
		job := &ScheduledJob{
			ID:        "job-uuid-003",
			Name:      "my-special-job",
			JobType:   "scan",
			CronExpr:  "*/15 * * * *",
			IsActive:  false,
			CreatedAt: createdAt,
			LastRun:   nil,
			NextRun:   createdAt.Add(15 * time.Minute),
		}

		out := captureStdout(t, func() {
			displayScheduledJobDetails(job)
		})

		assert.Contains(t, out, "my-special-job",
			"the job name must appear somewhere in the detail output")
	})
}
