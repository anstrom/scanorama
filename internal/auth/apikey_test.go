// Package auth provides comprehensive unit tests for API key utilities.
// This file tests key generation, validation, hashing, and format checking
// with various edge cases and security scenarios.
package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		keyName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_name",
			keyName:     "Test API Key",
			expectError: false,
		},
		{
			name:        "single_character_name",
			keyName:     "A",
			expectError: false,
		},
		{
			name:        "long_valid_name",
			keyName:     strings.Repeat("A", 255),
			expectError: false,
		},
		{
			name:        "empty_name",
			keyName:     "",
			expectError: true,
			errorMsg:    "key name cannot be empty",
		},
		{
			name:        "too_long_name",
			keyName:     strings.Repeat("A", 256),
			expectError: true,
			errorMsg:    "key name must be at most 255 characters",
		},
		{
			name:        "name_with_control_chars",
			keyName:     "Test\x00Key",
			expectError: true,
			errorMsg:    "key name contains invalid characters",
		},
		{
			name:        "name_with_unicode",
			keyName:     "Test Key 🔑",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generatedKey, err := GenerateAPIKey(tt.keyName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, generatedKey)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, generatedKey)

				// Verify key structure
				assert.Equal(t, tt.keyName, generatedKey.KeyInfo.Name)
				assert.True(t, strings.HasPrefix(generatedKey.Key, "sk_"))
				assert.True(t, len(generatedKey.Key) >= 35) // sk_ + 32 chars minimum
				assert.True(t, len(generatedKey.Key) <= 45) // reasonable upper bound
				assert.True(t, strings.HasPrefix(generatedKey.KeyPrefix, "sk_"))
				assert.True(t, strings.HasSuffix(generatedKey.KeyPrefix, "..."))

				// Verify key info defaults
				assert.True(t, generatedKey.KeyInfo.IsActive)
				assert.Equal(t, 0, generatedKey.KeyInfo.UsageCount)
				assert.NotNil(t, generatedKey.KeyInfo.Permissions)
				assert.Empty(t, generatedKey.KeyInfo.Permissions)
			}
		})
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	const numKeys = 1000
	keys := make(map[string]bool)

	for i := 0; i < numKeys; i++ {
		generatedKey, err := GenerateAPIKey("Test Key")
		require.NoError(t, err)

		// Ensure no duplicates
		assert.False(t, keys[generatedKey.Key], "Generated duplicate key: %s", generatedKey.Key)
		keys[generatedKey.Key] = true

		// Ensure valid format
		assert.True(t, IsValidAPIKeyFormat(generatedKey.Key))
	}
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		apiKey      string
		expectError bool
	}{
		{
			name:        "valid_key",
			apiKey:      "sk_abc123def456ghi789",
			expectError: false,
		},
		{
			name:        "empty_key",
			apiKey:      "",
			expectError: true,
		},
		{
			name:        "long_key",
			apiKey:      strings.Repeat("a", 1000),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashAPIKey(tt.apiKey)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, hash)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)
				assert.True(t, strings.HasPrefix(hash, "$2a$12$"))

				// Verify hash works with ValidateAPIKey
				isValid := ValidateAPIKey(tt.apiKey, hash)
				assert.True(t, isValid)
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	validKey := "sk_test_key_123"
	validHash, err := HashAPIKey(validKey)
	require.NoError(t, err)

	tests := []struct {
		name     string
		apiKey   string
		hash     string
		expected bool
	}{
		{
			name:     "valid_key_and_hash",
			apiKey:   validKey,
			hash:     validHash,
			expected: true,
		},
		{
			name:     "invalid_key_valid_hash",
			apiKey:   "sk_wrong_key_123",
			hash:     validHash,
			expected: false,
		},
		{
			name:     "valid_key_invalid_hash",
			apiKey:   validKey,
			hash:     "invalid_hash",
			expected: false,
		},
		{
			name:     "empty_key",
			apiKey:   "",
			hash:     validHash,
			expected: false,
		},
		{
			name:     "empty_hash",
			apiKey:   validKey,
			hash:     "",
			expected: false,
		},
		{
			name:     "both_empty",
			apiKey:   "",
			hash:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAPIKey(tt.apiKey, tt.hash)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidAPIKeyFormat(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected bool
	}{
		{
			name:     "valid_format",
			apiKey:   "sk_abc123def456ghi789jkl012mno345",
			expected: true,
		},
		{
			name:     "valid_short_format",
			apiKey:   "sk_abc123def456",
			expected: true,
		},
		{
			name:     "empty_key",
			apiKey:   "",
			expected: false,
		},
		{
			name:     "missing_prefix",
			apiKey:   "abc123def456ghi789",
			expected: false,
		},
		{
			name:     "wrong_prefix",
			apiKey:   "pk_abc123def456ghi789",
			expected: false,
		},
		{
			name:     "missing_underscore",
			apiKey:   "skabc123def456ghi789",
			expected: false,
		},
		{
			name:     "too_short",
			apiKey:   "sk_abc",
			expected: false,
		},
		{
			name:     "too_long",
			apiKey:   "sk_" + strings.Repeat("a", 100),
			expected: false,
		},
		{
			name:     "invalid_characters",
			apiKey:   "sk_abc123@def456#ghi789",
			expected: false,
		},
		{
			name:     "spaces_in_key",
			apiKey:   "sk_abc123 def456 ghi789",
			expected: false,
		},
		{
			name:     "uppercase_letters",
			apiKey:   "sk_ABC123DEF456GHI789",
			expected: true,
		},
		{
			name:     "mixed_case",
			apiKey:   "sk_AbC123dEf456GhI789",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidAPIKeyFormat(tt.apiKey)
			assert.Equal(t, tt.expected, result, "Key: %s", tt.apiKey)
		})
	}
}

func TestCreateDisplayPrefix(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "valid_key",
			apiKey:   "sk_abcdefghijklmnopqrstuvwxyz123456",
			expected: "sk_abcdefgh...",
		},
		{
			name:     "short_key",
			apiKey:   "sk_abc123",
			expected: "invalid_key",
		},
		{
			name:     "invalid_format",
			apiKey:   "invalid_key_format",
			expected: "invalid_key",
		},
		{
			name:     "empty_key",
			apiKey:   "",
			expected: "invalid_key",
		},
		{
			name:     "missing_underscore",
			apiKey:   "skabcdefgh",
			expected: "invalid_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateDisplayPrefix(tt.apiKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIKeyInfo_IsExpired(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name      string
		expiresAt *time.Time
		expected  bool
	}{
		{
			name:      "not_expired",
			expiresAt: &[]time.Time{now.Add(time.Hour)}[0],
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: &[]time.Time{now.Add(-time.Hour)}[0],
			expected:  true,
		},
		{
			name:      "no_expiration",
			expiresAt: nil,
			expected:  false,
		},
		{
			name:      "expires_now",
			expiresAt: &now,
			expected:  true, // Expired if exactly now
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyInfo := &APIKeyInfo{
				ExpiresAt: tt.expiresAt,
			}

			result := keyInfo.IsExpired()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAPIKeyInfo_IsValid(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name      string
		isActive  bool
		expiresAt *time.Time
		expected  bool
	}{
		{
			name:      "active_not_expired",
			isActive:  true,
			expiresAt: &[]time.Time{now.Add(time.Hour)}[0],
			expected:  true,
		},
		{
			name:      "active_no_expiration",
			isActive:  true,
			expiresAt: nil,
			expected:  true,
		},
		{
			name:      "inactive_not_expired",
			isActive:  false,
			expiresAt: &[]time.Time{now.Add(time.Hour)}[0],
			expected:  false,
		},
		{
			name:      "active_expired",
			isActive:  true,
			expiresAt: &[]time.Time{now.Add(-time.Hour)}[0],
			expected:  false,
		},
		{
			name:      "inactive_expired",
			isActive:  false,
			expiresAt: &[]time.Time{now.Add(-time.Hour)}[0],
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyInfo := &APIKeyInfo{
				IsActive:  tt.isActive,
				ExpiresAt: tt.expiresAt,
			}

			result := keyInfo.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateKeyName(t *testing.T) {
	tests := []struct {
		name        string
		keyName     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_name",
			keyName:     "Test API Key",
			expectError: false,
		},
		{
			name:        "single_char",
			keyName:     "A",
			expectError: false,
		},
		{
			name:        "max_length",
			keyName:     strings.Repeat("A", 255),
			expectError: false,
		},
		{
			name:        "empty_name",
			keyName:     "",
			expectError: true,
			errorMsg:    "key name cannot be empty",
		},
		{
			name:        "too_long",
			keyName:     strings.Repeat("A", 256),
			expectError: true,
			errorMsg:    "key name must be at most 255 characters",
		},
		{
			name:        "control_character",
			keyName:     "Test\x01Key",
			expectError: true,
			errorMsg:    "key name contains invalid characters",
		},
		{
			name:        "null_byte",
			keyName:     "Test\x00Key",
			expectError: true,
			errorMsg:    "key name contains invalid characters",
		},
		{
			name:        "unicode_valid",
			keyName:     "Test Key 🔑",
			expectError: false,
		},
		{
			name:        "tabs_and_newlines",
			keyName:     "Test\tKey\n",
			expectError: true, // Tabs and newlines are control characters (ASCII 9, 10)
			errorMsg:    "key name contains invalid characters",
		},
		{
			name:        "unicode_chinese",
			keyName:     "测试密钥",
			expectError: false,
		},
		{
			name:        "unicode_arabic",
			keyName:     "مفتاح الاختبار",
			expectError: false,
		},
		{
			name:        "unicode_japanese",
			keyName:     "テストキー",
			expectError: false,
		},
		{
			name:        "unicode_russian",
			keyName:     "Тестовый ключ",
			expectError: false,
		},
		{
			name:        "unicode_mixed_scripts",
			keyName:     "Test مفتاح 테스트 🔑",
			expectError: false,
		},
		{
			name:        "unicode_mathematical_symbols",
			keyName:     "API Key ∑∆∇∞",
			expectError: false,
		},
		{
			name:        "unicode_accented_characters",
			keyName:     "Clé d'API café naïve",
			expectError: false,
		},
		{
			name:        "unicode_zalgo_text",
			keyName:     "T̸̰̈e̷̱̽s̶̰̈́t̵̰̾ ̷̱̈K̸̰̇e̷̱̽y̶̰̾",
			expectError: false,
		},
		{
			name:        "unicode_zero_width_chars",
			keyName:     "Test\u200bKey", // Contains zero-width space (U+200B)
			expectError: false,
		},
		{
			name:        "unicode_rtl_override",
			keyName:     "Test\u202EyeK", // Contains RTL override character
			expectError: true,            // RTL override is a control character
			errorMsg:    "key name contains invalid characters",
		},
		{
			name:        "unicode_line_separator",
			keyName:     "Test\u2028Key", // Line separator (U+2028)
			expectError: false,           // This is not an ASCII control character
		},
		{
			name:        "unicode_paragraph_separator",
			keyName:     "Test\u2029Key", // Paragraph separator (U+2029)
			expectError: false,           // This is not an ASCII control character
		},
		{
			name:        "ascii_delete_char",
			keyName:     "Test\x7fKey", // ASCII DEL character (127)
			expectError: true,
			errorMsg:    "key name contains invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKeyName(tt.keyName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAPIKeyValidator(t *testing.T) {
	t.Run("new_validator", func(t *testing.T) {
		validator := NewAPIKeyValidator()
		assert.NotNil(t, validator)
	})
}

func TestGeneratedAPIKey_Integration(t *testing.T) {
	// Test the full flow: generate -> hash -> validate
	keyName := "Integration Test Key"

	// Generate key
	generatedKey, err := GenerateAPIKey(keyName)
	require.NoError(t, err)
	require.NotNil(t, generatedKey)

	// Verify format
	assert.True(t, IsValidAPIKeyFormat(generatedKey.Key))

	// Hash the key
	hash, err := HashAPIKey(generatedKey.Key)
	require.NoError(t, err)

	// Validate the key against its hash
	assert.True(t, ValidateAPIKey(generatedKey.Key, hash))

	// Test with wrong key
	wrongKey := strings.Replace(generatedKey.Key, "a", "b", 1)
	if wrongKey != generatedKey.Key { // Make sure we actually changed something
		assert.False(t, ValidateAPIKey(wrongKey, hash))
	}

	// Test display prefix
	displayPrefix := CreateDisplayPrefix(generatedKey.Key)
	assert.Equal(t, generatedKey.KeyPrefix, displayPrefix)

	// Test key info validation
	keyInfo := &generatedKey.KeyInfo
	assert.True(t, keyInfo.IsValid())
	assert.False(t, keyInfo.IsExpired())
}

func TestConstants(t *testing.T) {
	// Test that constants have reasonable values
	assert.Equal(t, 32, APIKeyLength)
	assert.Equal(t, "sk", APIKeyPrefix)
	assert.Equal(t, 12, DisplayPrefixLength)
	assert.Equal(t, 12, BcryptCost)
	assert.Equal(t, 1, MinAPIKeyNameLength)
	assert.Equal(t, 255, MaxAPIKeyNameLength)
}

// Benchmark tests
func BenchmarkGenerateAPIKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateAPIKey("Benchmark Key")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHashAPIKey(b *testing.B) {
	key := "sk_benchmark_test_key_123456789"
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := HashAPIKey(key)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateAPIKey(b *testing.B) {
	key := "sk_benchmark_test_key_123456789"
	hash, err := HashAPIKey(key)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateAPIKey(key, hash)
	}
}

func BenchmarkIsValidAPIKeyFormat(b *testing.B) {
	key := "sk_benchmark_test_key_123456789"

	for i := 0; i < b.N; i++ {
		IsValidAPIKeyFormat(key)
	}
}
