// Package auth provides database operations for API key management.
// This file implements CRUD operations for API keys stored in PostgreSQL,
// including creation, validation, updating, and revocation of keys.
package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

// APIKeyRepository provides database operations for API keys
type APIKeyRepository struct {
	db *db.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(database *db.DB) *APIKeyRepository {
	return &APIKeyRepository{
		db: database,
	}
}

// CreateAPIKey stores a new API key in the database
func (r *APIKeyRepository) CreateAPIKey(generatedKey *GeneratedAPIKey) (*APIKeyInfo, error) {
	// Hash the key for secure storage
	keyHash, err := HashAPIKey(generatedKey.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to hash API key: %w", err)
	}

	// Convert permissions to JSONB
	permissionsJSON, err := json.Marshal(generatedKey.KeyInfo.Permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	query := `
		INSERT INTO api_keys (
			name, key_hash, key_prefix, expires_at, is_active,
			usage_count, permissions, created_by, notes
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created_at, updated_at`

	var keyInfo APIKeyInfo
	err = r.db.QueryRow(query,
		generatedKey.KeyInfo.Name,
		keyHash,
		generatedKey.KeyPrefix,
		generatedKey.KeyInfo.ExpiresAt,
		true, // is_active
		0,    // usage_count
		permissionsJSON,
		generatedKey.KeyInfo.CreatedBy,
		generatedKey.KeyInfo.Notes,
	).Scan(&keyInfo.ID, &keyInfo.CreatedAt, &keyInfo.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to store API key: %w", err)
	}

	// Copy the rest of the fields
	keyInfo.Name = generatedKey.KeyInfo.Name
	keyInfo.KeyPrefix = generatedKey.KeyPrefix
	keyInfo.ExpiresAt = generatedKey.KeyInfo.ExpiresAt
	keyInfo.IsActive = true
	keyInfo.UsageCount = 0
	keyInfo.Permissions = generatedKey.KeyInfo.Permissions
	keyInfo.CreatedBy = generatedKey.KeyInfo.CreatedBy
	keyInfo.Notes = generatedKey.KeyInfo.Notes

	return &keyInfo, nil
}

// ListAPIKeys retrieves API keys with optional filters
func (r *APIKeyRepository) ListAPIKeys(showExpired, showInactive bool) ([]APIKeyInfo, error) {
	query := `
		SELECT id, name, key_prefix, created_at, updated_at, last_used_at,
		       expires_at, is_active, usage_count, permissions,
		       created_by, notes
		FROM api_keys
		WHERE 1=1`

	var args []interface{}
	argIndex := 1

	// Apply filters
	if !showInactive {
		query += fmt.Sprintf(" AND is_active = $%d", argIndex)
		args = append(args, true)
		argIndex++
	}

	if !showExpired {
		query += fmt.Sprintf(" AND (expires_at IS NULL OR expires_at > $%d)", argIndex)
		args = append(args, time.Now().UTC())
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []APIKeyInfo
	for rows.Next() {
		var key APIKeyInfo
		var permissionsJSON []byte

		err := rows.Scan(
			&key.ID, &key.Name, &key.KeyPrefix, &key.CreatedAt, &key.UpdatedAt,
			&key.LastUsedAt, &key.ExpiresAt, &key.IsActive, &key.UsageCount,
			&permissionsJSON, &key.CreatedBy, &key.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key row: %w", err)
		}

		// Unmarshal permissions
		if len(permissionsJSON) > 0 {
			if err := json.Unmarshal(permissionsJSON, &key.Permissions); err != nil {
				return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
			}
		} else {
			key.Permissions = make(map[string]interface{})
		}

		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API key rows: %w", err)
	}

	return keys, nil
}

// FindAPIKeyByIdentifier finds an API key by ID or prefix
func (r *APIKeyRepository) FindAPIKeyByIdentifier(identifier string) (*APIKeyInfo, error) {
	var query string
	var args []interface{}

	// Try to parse as UUID first, if it fails, search by prefix
	_, err := uuid.Parse(identifier)
	if err == nil {
		// Valid UUID - search by ID only
		query = `
			SELECT id, name, key_prefix, created_at, updated_at, last_used_at,
			       expires_at, is_active, usage_count, permissions,
			       created_by, notes
			FROM api_keys
			WHERE id = $1
			LIMIT 1`
		args = []interface{}{identifier}
	} else {
		// Not a valid UUID - search by prefix
		query = `
			SELECT id, name, key_prefix, created_at, updated_at, last_used_at,
			       expires_at, is_active, usage_count, permissions,
			       created_by, notes
			FROM api_keys
			WHERE key_prefix LIKE $1 || '%'
			LIMIT 1`
		args = []interface{}{identifier}
	}

	var key APIKeyInfo
	var permissionsJSON []byte

	err = r.db.QueryRow(query, args...).Scan(
		&key.ID, &key.Name, &key.KeyPrefix, &key.CreatedAt, &key.UpdatedAt,
		&key.LastUsedAt, &key.ExpiresAt, &key.IsActive, &key.UsageCount,
		&permissionsJSON, &key.CreatedBy, &key.Notes,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("API key not found: %s", identifier)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query API key: %w", err)
	}

	// Unmarshal permissions
	if len(permissionsJSON) > 0 {
		if err := json.Unmarshal(permissionsJSON, &key.Permissions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
		}
	} else {
		key.Permissions = make(map[string]interface{})
	}

	return &key, nil
}

// UpdateAPIKey updates an API key's metadata
func (r *APIKeyRepository) UpdateAPIKey(keyID string, updates map[string]interface{}) (*APIKeyInfo, error) {
	if len(updates) == 0 {
		return r.FindAPIKeyByIdentifier(keyID)
	}

	// Build dynamic update query
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	for field, value := range updates {
		switch field {
		case "name":
			setParts = append(setParts, fmt.Sprintf("name = $%d", argIndex))
			args = append(args, value)
			argIndex++
		case "notes":
			setParts = append(setParts, fmt.Sprintf("notes = $%d", argIndex))
			args = append(args, value)
			argIndex++
		case "expires_at":
			setParts = append(setParts, fmt.Sprintf("expires_at = $%d", argIndex))
			args = append(args, value)
			argIndex++
		default:
			return nil, fmt.Errorf("unsupported update field: %s", field)
		}
	}

	// Always update the updated_at timestamp
	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now().UTC())
	argIndex++

	// Add WHERE clause
	args = append(args, keyID)

	query := fmt.Sprintf(`
		UPDATE api_keys
		SET %s
		WHERE id = $%d OR key_prefix LIKE $%d || '%%'`,
		strings.Join(setParts, ", "), argIndex, argIndex)

	result, err := r.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get update result: %w", err)
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("API key not found: %s", keyID)
	}

	// Return the updated key
	return r.FindAPIKeyByIdentifier(keyID)
}

// RevokeAPIKey deactivates an API key
func (r *APIKeyRepository) RevokeAPIKey(keyID string) error {
	query := `
		UPDATE api_keys
		SET is_active = false, updated_at = $1
		WHERE id = $2 OR key_prefix LIKE $2 || '%'`

	result, err := r.db.Exec(query, time.Now().UTC(), keyID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get revoke result: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("API key not found: %s", keyID)
	}

	return nil
}

// ValidateAPIKey checks if an API key is valid for authentication
func (r *APIKeyRepository) ValidateAPIKey(apiKey string) (*APIKeyInfo, error) {
	if !IsValidAPIKeyFormat(apiKey) {
		return nil, fmt.Errorf("invalid API key format")
	}

	query := `
		SELECT id, name, key_prefix, key_hash, created_at, updated_at,
		       last_used_at, expires_at, is_active, usage_count,
		       permissions, created_by, notes
		FROM api_keys
		WHERE is_active = true
		AND (expires_at IS NULL OR expires_at > $1)`

	rows, err := r.db.Query(query, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys for validation: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key APIKeyInfo
		var keyHash string
		var permissionsJSON []byte

		err := rows.Scan(
			&key.ID, &key.Name, &key.KeyPrefix, &keyHash,
			&key.CreatedAt, &key.UpdatedAt, &key.LastUsedAt,
			&key.ExpiresAt, &key.IsActive, &key.UsageCount,
			&permissionsJSON, &key.CreatedBy, &key.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key for validation: %w", err)
		}

		// Check if this key matches the provided API key
		if ValidateAPIKey(apiKey, keyHash) {
			// Unmarshal permissions
			if len(permissionsJSON) > 0 {
				if err := json.Unmarshal(permissionsJSON, &key.Permissions); err != nil {
					return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
				}
			} else {
				key.Permissions = make(map[string]interface{})
			}

			// Update last used timestamp asynchronously to avoid slowing down auth
			go r.updateLastUsed(key.ID)

			return &key, nil
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during API key validation: %w", err)
	}

	return nil, fmt.Errorf("invalid API key")
}

// updateLastUsed updates the last_used_at timestamp and usage count
func (r *APIKeyRepository) updateLastUsed(keyID string) {
	query := `
		UPDATE api_keys
		SET last_used_at = $1, usage_count = usage_count + 1, updated_at = $1
		WHERE id = $2`

	_, err := r.db.Exec(query, time.Now().UTC(), keyID)
	if err != nil {
		// Log error but don't fail the authentication
		// In production, you might want to use a proper logger here
		fmt.Printf("Warning: Failed to update last_used_at for API key %s: %v\n", keyID, err)
	}
}

// GetAPIKeyStats returns statistics about API key usage
func (r *APIKeyRepository) GetAPIKeyStats() (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_keys,
			COUNT(*) FILTER (WHERE is_active = true) as active_keys,
			COUNT(*) FILTER (WHERE is_active = false) as revoked_keys,
			COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at < NOW()) as expired_keys,
			COUNT(*) FILTER (WHERE last_used_at IS NOT NULL) as used_keys,
			AVG(usage_count) as avg_usage_count,
			MAX(last_used_at) as most_recent_use
		FROM api_keys`

	var stats struct {
		TotalKeys     int             `json:"total_keys"`
		ActiveKeys    int             `json:"active_keys"`
		RevokedKeys   int             `json:"revoked_keys"`
		ExpiredKeys   int             `json:"expired_keys"`
		UsedKeys      int             `json:"used_keys"`
		AvgUsageCount sql.NullFloat64 `json:"avg_usage_count"`
		MostRecentUse sql.NullTime    `json:"most_recent_use"`
	}

	err := r.db.QueryRow(query).Scan(
		&stats.TotalKeys, &stats.ActiveKeys, &stats.RevokedKeys,
		&stats.ExpiredKeys, &stats.UsedKeys, &stats.AvgUsageCount,
		&stats.MostRecentUse,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key statistics: %w", err)
	}

	result := map[string]interface{}{
		"total_keys":   stats.TotalKeys,
		"active_keys":  stats.ActiveKeys,
		"revoked_keys": stats.RevokedKeys,
		"expired_keys": stats.ExpiredKeys,
		"used_keys":    stats.UsedKeys,
	}

	if stats.AvgUsageCount.Valid {
		result["avg_usage_count"] = stats.AvgUsageCount.Float64
	}

	if stats.MostRecentUse.Valid {
		result["most_recent_use"] = stats.MostRecentUse.Time
	}

	return result, nil
}

// CleanupExpiredKeys deactivates expired API keys
func (r *APIKeyRepository) CleanupExpiredKeys() (int, error) {
	query := `
		UPDATE api_keys
		SET is_active = false, updated_at = $1
		WHERE is_active = true
		AND expires_at IS NOT NULL
		AND expires_at < $1`

	result, err := r.db.Exec(query, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired API keys: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get cleanup result: %w", err)
	}

	return int(rowsAffected), nil
}

// CheckConfigAPIKeys validates that configured API keys exist in database
func (r *APIKeyRepository) CheckConfigAPIKeys(configKeys []string) ([]string, error) {
	if len(configKeys) == 0 {
		return []string{}, nil
	}

	var missingKeys []string
	for _, configKey := range configKeys {
		_, err := r.ValidateAPIKey(configKey)
		if err != nil {
			missingKeys = append(missingKeys, configKey)
		}
	}

	return missingKeys, nil
}
