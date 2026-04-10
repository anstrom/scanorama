// Package db provides a typed repository for settings database operations.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/anstrom/scanorama/internal/errors"
)

// Setting represents a single application setting row.
type Setting struct {
	Key         string    `db:"key"         json:"key"`
	Value       string    `db:"value"       json:"value"` // raw JSON string
	Description string    `db:"description" json:"description"`
	Type        string    `db:"type"        json:"type"`
	UpdatedAt   time.Time `db:"updated_at"  json:"updated_at"`
}

// SettingsRepository handles settings database operations.
type SettingsRepository struct {
	db *DB
}

// NewSettingsRepository creates a new SettingsRepository.
func NewSettingsRepository(db *DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// ListSettings returns all settings ordered by key.
func (r *SettingsRepository) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, value::text, COALESCE(description, ''), type, updated_at
		FROM settings ORDER BY key`)
	if err != nil {
		return nil, sanitizeDBError("list settings", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = fmt.Errorf("list settings: close rows: %w", cerr)
		}
	}()

	var settings []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.Description, &s.Type, &s.UpdatedAt); err != nil {
			return nil, sanitizeDBError("scan setting row", err)
		}
		settings = append(settings, s)
	}
	if err := rows.Err(); err != nil {
		return nil, sanitizeDBError("iterate settings", err)
	}
	if settings == nil {
		settings = []Setting{}
	}
	return settings, nil
}

// GetSetting returns a single setting by key, or ErrNotFound if missing.
func (r *SettingsRepository) GetSetting(ctx context.Context, key string) (*Setting, error) {
	var s Setting
	row := r.db.QueryRowContext(ctx,
		`SELECT key, value::text, COALESCE(description, ''), type, updated_at
		FROM settings WHERE key = $1`, key)
	err := row.Scan(&s.Key, &s.Value, &s.Description, &s.Type, &s.UpdatedAt)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFound("setting")
		}
		return nil, sanitizeDBError("get setting", err)
	}
	return &s, nil
}

// SetSetting updates the value of an existing setting identified by key.
// Returns an error if the key does not exist (unknown setting).
func (r *SettingsRepository) SetSetting(ctx context.Context, key, valueJSON string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE settings SET value = $2::jsonb, updated_at = NOW() WHERE key = $1`,
		key, valueJSON)
	if err != nil {
		return sanitizeDBError("set setting", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return sanitizeDBError("set setting rows affected", err)
	}
	if rowsAffected == 0 {
		return errors.ErrNotFound("setting")
	}
	return nil
}
