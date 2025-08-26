// Package auth provides authentication utilities for the Scanorama API server.
// This package implements API key generation, validation, and management functions
// with security best practices including secure random generation and bcrypt hashing.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// API key generation and validation constants
const (
	// APIKeyLength is the length of the random part of an API key
	APIKeyLength = 32
	// APIKeyPrefix is the standard prefix for all API keys
	APIKeyPrefix = "sk"
	// DisplayPrefixLength is the length of prefix shown in UI (e.g., "sk_abc...")
	DisplayPrefixLength = 12

	// BcryptCost is the bcrypt cost for hashing API keys (12 is a good balance of security and performance)
	BcryptCost = 12
	// BcryptMaxInputLength is the maximum input length for bcrypt (72 bytes)
	BcryptMaxInputLength = 72

	// MinAPIKeyNameLength is the minimum length for API key names
	MinAPIKeyNameLength = 1
	// MaxAPIKeyNameLength is the maximum length for API key names
	MaxAPIKeyNameLength = 255
)

// APIKeyInfo contains metadata about an API key
type APIKeyInfo struct {
	ID         string     `json:"id" db:"id"`
	Name       string     `json:"name" db:"name" validate:"required,min=1,max=255"`
	KeyPrefix  string     `json:"key_prefix" db:"key_prefix"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	IsActive   bool       `json:"is_active" db:"is_active"`
	UsageCount int        `json:"usage_count" db:"usage_count"`
	Notes      string     `json:"notes,omitempty" db:"notes"`

	// Phase 2 ready fields (RBAC support)
	// Note: Roles are now managed through api_key_roles junction table
	Permissions map[string]interface{} `json:"permissions,omitempty" db:"permissions"` // Deprecated: use roles instead
	CreatedBy   *string                `json:"created_by,omitempty" db:"created_by"`
}

// GeneratedAPIKey contains a newly generated API key and its metadata
type GeneratedAPIKey struct {
	Key       string     `json:"key"`        // The actual API key (only shown once)
	KeyInfo   APIKeyInfo `json:"key_info"`   // Metadata about the key
	KeyPrefix string     `json:"key_prefix"` // Display-safe prefix
}

// APIKeyValidator provides methods for validating API keys
type APIKeyValidator struct {
	// Future: could include database connection, cache, etc.
}

// NewAPIKeyValidator creates a new API key validator
func NewAPIKeyValidator() *APIKeyValidator {
	return &APIKeyValidator{}
}

// GenerateAPIKey creates a new API key with the specified name
func GenerateAPIKey(name string) (*GeneratedAPIKey, error) {
	if err := validateKeyName(name); err != nil {
		return nil, fmt.Errorf("invalid key name: %w", err)
	}

	// Generate the random part of the key
	randomBytes := make([]byte, APIKeyLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	// Use base32 encoding for better readability (no ambiguous characters)
	randomPart := strings.ToLower(base32.StdEncoding.EncodeToString(randomBytes))
	// Trim padding and take exactly APIKeyLength characters
	if len(randomPart) > APIKeyLength {
		randomPart = randomPart[:APIKeyLength]
	}

	// Construct the full API key
	fullKey := fmt.Sprintf("%s_%s", APIKeyPrefix, randomPart)

	// Create display prefix (first part + ellipsis)
	displayPrefix := CreateDisplayPrefix(fullKey)

	// Create the key info (ID will be set by database)
	keyInfo := APIKeyInfo{
		Name:        name,
		KeyPrefix:   displayPrefix,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		IsActive:    true,
		UsageCount:  0,
		Permissions: make(map[string]interface{}), // Deprecated: roles managed via junction table
	}

	return &GeneratedAPIKey{
		Key:       fullKey,
		KeyInfo:   keyInfo,
		KeyPrefix: displayPrefix,
	}, nil
}

// HashAPIKey creates a bcrypt hash of an API key for secure storage
func HashAPIKey(apiKey string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}

	// bcrypt has a 72-byte limit, so for longer keys we first hash with SHA-256
	keyBytes := []byte(apiKey)
	if len(keyBytes) > BcryptMaxInputLength {
		sha256Hash := sha256.Sum256(keyBytes)
		keyBytes = sha256Hash[:]
	}

	hash, err := bcrypt.GenerateFromPassword(keyBytes, BcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash API key: %w", err)
	}

	return string(hash), nil
}

// ValidateAPIKey checks if a provided API key matches the stored hash
func ValidateAPIKey(apiKey, storedHash string) bool {
	if apiKey == "" || storedHash == "" {
		return false
	}

	// Apply the same pre-processing as HashAPIKey for consistency
	keyBytes := []byte(apiKey)
	if len(keyBytes) > BcryptMaxInputLength {
		sha256Hash := sha256.Sum256(keyBytes)
		keyBytes = sha256Hash[:]
	}

	err := bcrypt.CompareHashAndPassword([]byte(storedHash), keyBytes)
	return err == nil
}

// IsValidAPIKeyFormat checks if an API key has the correct format
func IsValidAPIKeyFormat(apiKey string) bool {
	if apiKey == "" {
		return false
	}

	// Check for valid prefix
	if !strings.HasPrefix(apiKey, APIKeyPrefix+"_") {
		return false
	}

	// Check total length (prefix + underscore + random part)
	// Example: sk_abcd1234... should be around 35-40 characters
	if len(apiKey) < 15 || len(apiKey) > 50 {
		return false
	}

	// Check that it contains only valid characters (alphanumeric + underscores)
	for _, char := range apiKey {
		// Check if character is valid (alphanumeric or underscore)
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '_' {
			return false
		}
	}

	return true
}

// CreateDisplayPrefix creates a safe-to-display prefix from a full API key
func CreateDisplayPrefix(apiKey string) string {
	if !IsValidAPIKeyFormat(apiKey) {
		return "invalid_key"
	}

	// Find the underscore after the prefix
	parts := strings.Split(apiKey, "_")
	if len(parts) < 2 {
		return "invalid_key"
	}

	// Return prefix + first few characters of random part
	if len(parts[1]) >= 8 {
		return fmt.Sprintf("%s_%s...", parts[0], parts[1][:8])
	}

	return fmt.Sprintf("%s_%s...", parts[0], parts[1])
}

// IsExpired checks if an API key has expired
func (k *APIKeyInfo) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false // No expiration set
	}
	return k.ExpiresAt.Before(time.Now().UTC())
}

// IsValid checks if an API key is active and not expired
func (k *APIKeyInfo) IsValid() bool {
	return k.IsActive && !k.IsExpired()
}

// validateKeyName validates the API key name
func validateKeyName(name string) error {
	if name == "" {
		return fmt.Errorf("key name cannot be empty")
	}

	if len(name) < MinAPIKeyNameLength {
		return fmt.Errorf("key name must be at least %d characters", MinAPIKeyNameLength)
	}

	if len(name) > MaxAPIKeyNameLength {
		return fmt.Errorf("key name must be at most %d characters", MaxAPIKeyNameLength)
	}

	// Check for invalid characters (allow Unicode, but block control characters)
	for _, char := range name {
		// Block ASCII control characters (0-31 and 127)
		if char < 32 || char == 127 {
			return fmt.Errorf("key name contains invalid characters")
		}

		// Block Unicode control characters and formatting characters
		// U+0080-U+009F: C1 Controls
		// U+202A-U+202E: Bidirectional formatting (including RTL override)
		// U+2066-U+2069: Directional isolates
		if (char >= 0x0080 && char <= 0x009F) ||
			(char >= 0x202A && char <= 0x202E) ||
			(char >= 0x2066 && char <= 0x2069) {
			return fmt.Errorf("key name contains invalid characters")
		}
	}

	return nil
}
