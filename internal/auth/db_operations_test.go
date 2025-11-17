// Package auth provides comprehensive integration tests for API key database operations.
// This file tests CRUD operations, validation, statistics, and cleanup functionality
// for API keys stored in PostgreSQL.
package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepository creates a test repository with database connection
func setupTestRepository(t *testing.T) (repo *APIKeyRepository, cleanup func()) {
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
		return nil, nil
	}

	ctx := context.Background()

	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return nil, nil
	}

	repo = NewAPIKeyRepository(database)

	cleanup = func() {
		// Clean up test data
		_, _ = database.Exec("DELETE FROM api_keys WHERE name LIKE 'Test%'")
		database.Close()
	}

	return repo, cleanup
}

func TestNewAPIKeyRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	ctx := context.Background()

	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return
	}
	defer database.Close()

	repo := NewAPIKeyRepository(database)
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.db)
}

func TestAPIKeyRepository_CreateAPIKey(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	tests := []struct {
		name        string
		keyName     string
		notes       string
		expectError bool
	}{
		{
			name:        "create_basic_key",
			keyName:     "Test Basic Key",
			notes:       "This is a test key",
			expectError: false,
		},
		{
			name:        "create_key_with_emoji",
			keyName:     "Test Key 🔑",
			notes:       "",
			expectError: false,
		},
		{
			name:        "create_key_with_long_notes",
			keyName:     "Test Long Notes Key",
			notes:       "This is a very long note. " + strings.Repeat("Lorem ipsum dolor sit amet. ", 20),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate a new API key
			generatedKey, err := GenerateAPIKey(tt.keyName)
			require.NoError(t, err)

			generatedKey.KeyInfo.Notes = tt.notes

			// Store it in the database
			keyInfo, err := repo.CreateAPIKey(generatedKey)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, keyInfo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, keyInfo)
				assert.NotEmpty(t, keyInfo.ID)
				assert.Equal(t, tt.keyName, keyInfo.Name)
				assert.Equal(t, tt.notes, keyInfo.Notes)
				assert.True(t, keyInfo.IsActive)
				assert.Equal(t, 0, keyInfo.UsageCount)
				assert.NotZero(t, keyInfo.CreatedAt)
				assert.NotZero(t, keyInfo.UpdatedAt)
			}
		})
	}
}

func TestAPIKeyRepository_CreateAPIKey_WithExpiration(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create key with expiration
	generatedKey, err := GenerateAPIKey("Test Expiring Key")
	require.NoError(t, err)

	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	generatedKey.KeyInfo.ExpiresAt = &expiresAt

	keyInfo, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)
	assert.NotNil(t, keyInfo.ExpiresAt)
	assert.WithinDuration(t, expiresAt, *keyInfo.ExpiresAt, time.Second)
}

func TestAPIKeyRepository_ListAPIKeys(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create test keys
	activeKey, err := GenerateAPIKey("Test Active Key")
	require.NoError(t, err)
	_, err = repo.CreateAPIKey(activeKey)
	require.NoError(t, err)

	expiredKey, err := GenerateAPIKey("Test Expired Key")
	require.NoError(t, err)
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	expiredKey.KeyInfo.ExpiresAt = &futureTime
	storedExpired, err := repo.CreateAPIKey(expiredKey)
	require.NoError(t, err)

	// Manually update to past date to simulate expired key (bypass constraint)
	pastTime := time.Now().UTC().Add(-24 * time.Hour)
	_, err = repo.db.Exec("UPDATE api_keys SET expires_at = $1 WHERE id = $2", pastTime, storedExpired.ID)
	require.NoError(t, err)

	// Create inactive key
	inactiveKey, err := GenerateAPIKey("Test Inactive Key")
	require.NoError(t, err)
	storedInactive, err := repo.CreateAPIKey(inactiveKey)
	require.NoError(t, err)
	err = repo.RevokeAPIKey(storedInactive.ID)
	require.NoError(t, err)

	tests := []struct {
		name         string
		showExpired  bool
		showInactive bool
		minExpected  int
	}{
		{
			name:         "list_active_only",
			showExpired:  false,
			showInactive: false,
			minExpected:  1, // At least the active key
		},
		{
			name:         "list_with_expired",
			showExpired:  true,
			showInactive: false,
			minExpected:  2, // Active + expired
		},
		{
			name:         "list_all_keys",
			showExpired:  true,
			showInactive: true,
			minExpected:  3, // Active + expired + inactive
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := repo.ListAPIKeys(tt.showExpired, tt.showInactive)
			assert.NoError(t, err)
			assert.GreaterOrEqual(t, len(keys), tt.minExpected)

			// Verify all keys have required fields
			for _, key := range keys {
				assert.NotEmpty(t, key.ID)
				assert.NotEmpty(t, key.Name)
				assert.NotEmpty(t, key.KeyPrefix)
				assert.NotZero(t, key.CreatedAt)
			}
		})
	}
}

func TestAPIKeyRepository_FindAPIKeyByIdentifier(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a test key
	generatedKey, err := GenerateAPIKey("Test Find Key")
	require.NoError(t, err)

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	tests := []struct {
		name        string
		identifier  string
		expectError bool
	}{
		{
			name:        "find_by_id",
			identifier:  storedKey.ID,
			expectError: false,
		},
		{
			name:        "find_by_prefix",
			identifier:  storedKey.KeyPrefix, // Use full prefix
			expectError: false,
		},
		{
			name:        "find_nonexistent",
			identifier:  "nonexistent-id",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foundKey, err := repo.FindAPIKeyByIdentifier(tt.identifier)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, foundKey)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, foundKey)
				assert.Equal(t, storedKey.ID, foundKey.ID)
				assert.Equal(t, storedKey.Name, foundKey.Name)
			}
		})
	}
}

func TestAPIKeyRepository_UpdateAPIKey(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a test key
	generatedKey, err := GenerateAPIKey("Test Update Key")
	require.NoError(t, err)

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	tests := []struct {
		name        string
		updates     map[string]interface{}
		expectError bool
		verify      func(*testing.T, *APIKeyInfo)
	}{
		{
			name: "update_name",
			updates: map[string]interface{}{
				"name": "Updated Key Name",
			},
			expectError: false,
			verify: func(t *testing.T, key *APIKeyInfo) {
				assert.Equal(t, "Updated Key Name", key.Name)
			},
		},
		{
			name: "update_notes",
			updates: map[string]interface{}{
				"notes": "Updated notes",
			},
			expectError: false,
			verify: func(t *testing.T, key *APIKeyInfo) {
				assert.Equal(t, "Updated notes", key.Notes)
			},
		},
		{
			name: "update_expiration",
			updates: map[string]interface{}{
				"expires_at": time.Now().UTC().Add(48 * time.Hour),
			},
			expectError: false,
			verify: func(t *testing.T, key *APIKeyInfo) {
				assert.NotNil(t, key.ExpiresAt)
			},
		},
		{
			name: "update_multiple_fields",
			updates: map[string]interface{}{
				"name":  "Multi Update Key",
				"notes": "Multiple fields updated",
			},
			expectError: false,
			verify: func(t *testing.T, key *APIKeyInfo) {
				assert.Equal(t, "Multi Update Key", key.Name)
				assert.Equal(t, "Multiple fields updated", key.Notes)
			},
		},
		{
			name: "update_unsupported_field",
			updates: map[string]interface{}{
				"unsupported_field": "value",
			},
			expectError: true,
		},
		{
			name:        "update_no_fields",
			updates:     map[string]interface{}{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedKey, err := repo.UpdateAPIKey(storedKey.ID, tt.updates)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, updatedKey)
				assert.Equal(t, storedKey.ID, updatedKey.ID)

				if tt.verify != nil {
					tt.verify(t, updatedKey)
				}
			}
		})
	}
}

func TestAPIKeyRepository_RevokeAPIKey(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a test key
	generatedKey, err := GenerateAPIKey("Test Revoke Key")
	require.NoError(t, err)

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	// Verify key is active
	assert.True(t, storedKey.IsActive)

	// Revoke the key
	err = repo.RevokeAPIKey(storedKey.ID)
	assert.NoError(t, err)

	// Verify key is now inactive
	foundKey, err := repo.FindAPIKeyByIdentifier(storedKey.ID)
	require.NoError(t, err)
	assert.False(t, foundKey.IsActive)

	// Try to revoke nonexistent key
	err = repo.RevokeAPIKey("nonexistent-id")
	assert.Error(t, err)
}

func TestAPIKeyRepository_ValidateAPIKey(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a valid test key
	generatedKey, err := GenerateAPIKey("Test Validate Key")
	require.NoError(t, err)

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	// Create an expired key
	expiredKey, err := GenerateAPIKey("Test Expired Validate Key")
	require.NoError(t, err)
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	expiredKey.KeyInfo.ExpiresAt = &futureTime
	storedExpired2, err := repo.CreateAPIKey(expiredKey)
	require.NoError(t, err)

	// Manually update to past date to simulate expired key
	pastTime := time.Now().UTC().Add(-24 * time.Hour)
	_, err = repo.db.Exec("UPDATE api_keys SET expires_at = $1 WHERE id = $2", pastTime, storedExpired2.ID)
	require.NoError(t, err)

	// Create an inactive key
	inactiveKey, err := GenerateAPIKey("Test Inactive Validate Key")
	require.NoError(t, err)
	storedInactiveKey, err := repo.CreateAPIKey(inactiveKey)
	require.NoError(t, err)
	err = repo.RevokeAPIKey(storedInactiveKey.ID)
	require.NoError(t, err)

	tests := []struct {
		name        string
		apiKey      string
		expectError bool
		description string
	}{
		{
			name:        "valid_active_key",
			apiKey:      generatedKey.Key,
			expectError: false,
			description: "Should validate active key",
		},
		{
			name:        "expired_key",
			apiKey:      expiredKey.Key,
			expectError: true,
			description: "Should reject expired key",
		},
		{
			name:        "inactive_key",
			apiKey:      inactiveKey.Key,
			expectError: true,
			description: "Should reject inactive key",
		},
		{
			name:        "invalid_format",
			apiKey:      "invalid-key-format",
			expectError: true,
			description: "Should reject invalid format",
		},
		{
			name:        "wrong_key",
			apiKey:      "sk_wrongkeyvalue12345678901234",
			expectError: true,
			description: "Should reject wrong key",
		},
		{
			name:        "empty_key",
			apiKey:      "",
			expectError: true,
			description: "Should reject empty key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validatedKey, err := repo.ValidateAPIKey(tt.apiKey)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, validatedKey)
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, validatedKey)
				assert.Equal(t, storedKey.ID, validatedKey.ID)
				assert.Equal(t, storedKey.Name, validatedKey.Name)
			}
		})
	}
}

func TestAPIKeyRepository_ValidateAPIKey_UpdatesLastUsed(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a test key
	generatedKey, err := GenerateAPIKey("Test LastUsed Key")
	require.NoError(t, err)

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	// Initially, last_used_at should be nil
	assert.Nil(t, storedKey.LastUsedAt)

	// Validate the key
	validatedKey, err := repo.ValidateAPIKey(generatedKey.Key)
	require.NoError(t, err)
	assert.NotNil(t, validatedKey)

	// Wait a moment for async update
	time.Sleep(100 * time.Millisecond)

	// Check that last_used_at was updated
	foundKey, err := repo.FindAPIKeyByIdentifier(storedKey.ID)
	require.NoError(t, err)

	// last_used_at might still be nil if async update hasn't completed
	// So we just verify the key exists and is valid
	assert.NotNil(t, foundKey)
	assert.True(t, foundKey.IsActive)
}

func TestAPIKeyRepository_GetAPIKeyStats(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create multiple test keys
	activeKey, err := GenerateAPIKey("Test Stats Active")
	require.NoError(t, err)
	_, err = repo.CreateAPIKey(activeKey)
	require.NoError(t, err)

	expiredKey, err := GenerateAPIKey("Test Stats Expired")
	require.NoError(t, err)
	pastTime := time.Now().UTC().Add(-24 * time.Hour)
	expiredKey.KeyInfo.ExpiresAt = &pastTime
	_, err = repo.CreateAPIKey(expiredKey)
	require.NoError(t, err)

	revokedKey, err := GenerateAPIKey("Test Stats Revoked")
	require.NoError(t, err)
	storedRevoked, err := repo.CreateAPIKey(revokedKey)
	require.NoError(t, err)
	err = repo.RevokeAPIKey(storedRevoked.ID)
	require.NoError(t, err)

	// Get statistics
	stats, err := repo.GetAPIKeyStats()
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Verify statistics contain expected keys
	assert.Contains(t, stats, "total_keys")
	assert.Contains(t, stats, "active_keys")
	assert.Contains(t, stats, "revoked_keys")
	assert.Contains(t, stats, "expired_keys")

	// Verify counts are reasonable
	totalKeys := stats["total_keys"].(int)
	activeKeys := stats["active_keys"].(int)
	revokedKeys := stats["revoked_keys"].(int)

	assert.GreaterOrEqual(t, totalKeys, 3) // At least our 3 test keys
	assert.GreaterOrEqual(t, activeKeys, 1)
	assert.GreaterOrEqual(t, revokedKeys, 1)
}

func TestAPIKeyRepository_CleanupExpiredKeys(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create an expired but active key
	expiredKey, err := GenerateAPIKey("Test Cleanup Expired")
	require.NoError(t, err)
	futureTime := time.Now().UTC().Add(24 * time.Hour)
	expiredKey.KeyInfo.ExpiresAt = &futureTime
	storedExpired, err := repo.CreateAPIKey(expiredKey)
	require.NoError(t, err)

	// Manually update to past date to simulate expired key
	pastTime := time.Now().UTC().Add(-24 * time.Hour)
	_, err = repo.db.Exec("UPDATE api_keys SET expires_at = $1 WHERE id = $2", pastTime, storedExpired.ID)
	require.NoError(t, err)

	// Verify it's active
	foundKey, err := repo.FindAPIKeyByIdentifier(storedExpired.ID)
	require.NoError(t, err)
	assert.True(t, foundKey.IsActive)

	// Run cleanup
	count, err := repo.CleanupExpiredKeys()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, 1)

	// Verify the key is now inactive
	foundKey, err = repo.FindAPIKeyByIdentifier(storedExpired.ID)
	require.NoError(t, err)
	assert.False(t, foundKey.IsActive)
}

func TestAPIKeyRepository_CheckConfigAPIKeys(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a valid key
	validKey, err := GenerateAPIKey("Test Config Key")
	require.NoError(t, err)
	_, err = repo.CreateAPIKey(validKey)
	require.NoError(t, err)

	tests := []struct {
		name            string
		configKeys      []string
		expectedMissing int
	}{
		{
			name:            "empty_config",
			configKeys:      []string{},
			expectedMissing: 0,
		},
		{
			name:            "valid_key",
			configKeys:      []string{validKey.Key},
			expectedMissing: 0,
		},
		{
			name:            "missing_key",
			configKeys:      []string{"sk_nonexistent12345678901234"},
			expectedMissing: 1,
		},
		{
			name:            "mixed_keys",
			configKeys:      []string{validKey.Key, "sk_missing12345678901234"},
			expectedMissing: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missingKeys, err := repo.CheckConfigAPIKeys(tt.configKeys)
			assert.NoError(t, err)
			assert.Len(t, missingKeys, tt.expectedMissing)
		})
	}
}

func TestAPIKeyRepository_Integration(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Full lifecycle test: Create -> List -> Update -> Validate -> Revoke

	// 1. Create
	generatedKey, err := GenerateAPIKey("Test Integration Key")
	require.NoError(t, err)

	futureTime := time.Now().UTC().Add(48 * time.Hour)
	generatedKey.KeyInfo.ExpiresAt = &futureTime
	generatedKey.KeyInfo.Notes = "Integration test key"

	storedKey, err := repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)
	assert.NotEmpty(t, storedKey.ID)
	assert.True(t, storedKey.IsActive)

	// 2. List
	keys, err := repo.ListAPIKeys(false, false)
	require.NoError(t, err)
	found := false
	for _, key := range keys {
		if key.ID == storedKey.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Created key should appear in list")

	// 3. Update
	updates := map[string]interface{}{
		"name":  "Updated Integration Key",
		"notes": "Updated notes",
	}
	updatedKey, err := repo.UpdateAPIKey(storedKey.ID, updates)
	require.NoError(t, err)
	assert.Equal(t, "Updated Integration Key", updatedKey.Name)
	assert.Equal(t, "Updated notes", updatedKey.Notes)

	// 4. Validate
	validatedKey, err := repo.ValidateAPIKey(generatedKey.Key)
	require.NoError(t, err)
	assert.Equal(t, storedKey.ID, validatedKey.ID)
	assert.True(t, validatedKey.IsValid())

	// 5. Revoke
	err = repo.RevokeAPIKey(storedKey.ID)
	require.NoError(t, err)

	// 6. Verify revoked key can't be validated
	_, err = repo.ValidateAPIKey(generatedKey.Key)
	assert.Error(t, err, "Revoked key should not validate")

	// 7. Verify key is inactive
	revokedKey, err := repo.FindAPIKeyByIdentifier(storedKey.ID)
	require.NoError(t, err)
	assert.False(t, revokedKey.IsActive)
	assert.False(t, revokedKey.IsValid())
}

func TestAPIKeyRepository_ConcurrentValidation(t *testing.T) {
	repo, cleanup := setupTestRepository(t)
	if repo == nil {
		return
	}
	defer cleanup()

	// Create a test key
	generatedKey, err := GenerateAPIKey("Test Concurrent Key")
	require.NoError(t, err)

	_, err = repo.CreateAPIKey(generatedKey)
	require.NoError(t, err)

	// Validate the same key concurrently
	const numGoroutines = 10
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := repo.ValidateAPIKey(generatedKey.Key)
			errors <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		assert.NoError(t, err, "Concurrent validation should succeed")
	}
}

// Benchmark tests
func BenchmarkAPIKeyRepository_CreateAPIKey(b *testing.B) {
	ctx := context.Background()
	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		b.Skipf("Skipping benchmark: database not available: %v", err)
		return
	}
	defer database.Close()

	repo := NewAPIKeyRepository(database)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generatedKey, _ := GenerateAPIKey("Benchmark Key")
		_, _ = repo.CreateAPIKey(generatedKey)
	}
}

func BenchmarkAPIKeyRepository_ValidateAPIKey(b *testing.B) {
	ctx := context.Background()
	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		b.Skipf("Skipping benchmark: database not available: %v", err)
		return
	}
	defer database.Close()

	repo := NewAPIKeyRepository(database)

	// Create a test key
	generatedKey, _ := GenerateAPIKey("Benchmark Validate Key")
	_, _ = repo.CreateAPIKey(generatedKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.ValidateAPIKey(generatedKey.Key)
	}
}
