// Package db — integration tests for SettingsRepository.
// These tests require a running Postgres instance and will be skipped
// automatically in short mode or when no database is reachable.
package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── ListSettings ───────────────────────────────────────────────────────────────

func TestSettings_ListSettings_ReturnsSeedRows(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewSettingsRepository(db)
	settings, err := repo.ListSettings(context.Background())

	require.NoError(t, err)
	// Migration 009 seeds at least these keys.
	keys := make(map[string]string, len(settings))
	for _, s := range settings {
		keys[s.Key] = s.Value
	}
	assert.Contains(t, keys, "scan.default_timing")
	assert.Contains(t, keys, "scan.max_concurrent")
	assert.Contains(t, keys, "notifications.scan_complete")
}

func TestSettings_ListSettings_OrderedByKey(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewSettingsRepository(db)
	settings, err := repo.ListSettings(context.Background())

	require.NoError(t, err)
	for i := 1; i < len(settings); i++ {
		assert.LessOrEqual(t, settings[i-1].Key, settings[i].Key,
			"expected settings ordered by key, got %q before %q",
			settings[i-1].Key, settings[i].Key)
	}
}

// ── GetSetting ─────────────────────────────────────────────────────────────────

func TestSettings_GetSetting_Exists(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewSettingsRepository(db)
	s, err := repo.GetSetting(context.Background(), "scan.default_timing")

	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "scan.default_timing", s.Key)
	assert.Equal(t, "int", s.Type)
	assert.NotEmpty(t, s.Description)
}

func TestSettings_GetSetting_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewSettingsRepository(db)
	_, err := repo.GetSetting(context.Background(), "this.key.does.not.exist")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected ErrNotFound, got: %v", err)
}

// ── SetSetting (write → read-back) ────────────────────────────────────────────

func TestSettings_SetSetting_WriteThenReadBack(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewSettingsRepository(db)

	// Restore original value at end of test.
	orig, err := repo.GetSetting(ctx, "scan.max_concurrent")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.SetSetting(ctx, "scan.max_concurrent", orig.Value)
	})

	require.NoError(t, repo.SetSetting(ctx, "scan.max_concurrent", "99"))

	got, err := repo.GetSetting(ctx, "scan.max_concurrent")
	require.NoError(t, err)
	assert.Equal(t, "99", got.Value)
}

func TestSettings_SetSetting_BoolRoundTrip(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewSettingsRepository(db)

	orig, err := repo.GetSetting(ctx, "notifications.scan_complete")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.SetSetting(ctx, "notifications.scan_complete", orig.Value)
	})

	// Toggle the bool value and read back.
	newVal := "false"
	if orig.Value == "false" {
		newVal = "true"
	}
	require.NoError(t, repo.SetSetting(ctx, "notifications.scan_complete", newVal))

	got, err := repo.GetSetting(ctx, "notifications.scan_complete")
	require.NoError(t, err)
	assert.Equal(t, newVal, got.Value)
}

func TestSettings_SetSetting_UpdatedAtAdvances(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewSettingsRepository(db)

	before, err := repo.GetSetting(ctx, "scan.default_timing")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.SetSetting(ctx, "scan.default_timing", before.Value)
	})

	require.NoError(t, repo.SetSetting(ctx, "scan.default_timing", "2"))

	after, err := repo.GetSetting(ctx, "scan.default_timing")
	require.NoError(t, err)
	assert.True(t, !after.UpdatedAt.Before(before.UpdatedAt),
		"updated_at should advance; before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
}

func TestSettings_SetSetting_UnknownKeyReturnsNotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewSettingsRepository(db)
	err := repo.SetSetting(context.Background(), "not.a.real.key", "42")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected ErrNotFound, got: %v", err)
}

func TestSettings_SetSetting_AllSeedKeysAreWritable(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewSettingsRepository(db)
	settings, err := repo.ListSettings(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, settings)

	for _, s := range settings {
		// Write back the same value — should always succeed.
		err := repo.SetSetting(ctx, s.Key, s.Value)
		assert.NoError(t, err, "SetSetting(%q) failed", s.Key)
	}
}
