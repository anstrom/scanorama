// Package db provides scan profile database operations for scanorama.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// marshalSQLArg ensures that complex types (maps, slices) are JSON-marshaled
// to []byte before being passed as SQL arguments, which the driver cannot handle
// natively.
func marshalSQLArg(val interface{}) interface{} {
	switch val.(type) {
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(val)
		if err != nil {
			return val
		}
		return b
	default:
		return val
	}
}

// parsePostgreSQLArray converts a PostgreSQL array interface{} to []string.
func parsePostgreSQLArray(arrayInterface interface{}) []string {
	if arrayInterface == nil {
		return nil
	}

	arr, ok := arrayInterface.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, len(arr))
	for i, v := range arr {
		if s, ok := v.(string); ok {
			result[i] = s
		}
	}
	return result
}

// ProfileFilters represents filters for listing profiles.
type ProfileFilters struct {
	ScanType string
}

// ListProfiles retrieves profiles with filtering and pagination.
func (db *DB) ListProfiles(ctx context.Context, filters ProfileFilters,
	offset, limit int) ([]*ScanProfile, int64, error) {
	// Build the base query.
	baseQuery := `
		SELECT id, name, description, os_family, ports, scan_type,
		       timing, scripts, options, priority, built_in,
		       created_at, updated_at
		FROM scan_profiles`

	// Build WHERE clause based on filters.
	var whereClause string
	var args []interface{}
	argIndex := 0

	if filters.ScanType != "" {
		whereClause += fmt.Sprintf(" AND scan_type = $%d", argIndex+1)
		args = append(args, filters.ScanType)
		argIndex++
	}

	// Remove leading "AND" if we have where clauses.
	if whereClause != "" {
		whereClause = "WHERE" + whereClause[4:]
	}

	// Get total count.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM scan_profiles %s", whereClause)

	var total int64
	err := db.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get profile count: %w", err)
	}

	// Get paginated results.
	listQuery := fmt.Sprintf(
		"%s %s ORDER BY priority DESC, name ASC LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, argIndex+1, argIndex+2)
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query profiles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var profiles []*ScanProfile
	for rows.Next() {
		profile := &ScanProfile{}
		var description *string
		var timing *string
		var options []byte
		var osFamily, scripts interface{}

		err := rows.Scan(
			&profile.ID,
			&profile.Name,
			&description,
			&osFamily,
			&profile.Ports,
			&profile.ScanType,
			&timing,
			&scripts,
			&options,
			&profile.Priority,
			&profile.BuiltIn,
			&profile.CreatedAt,
			&profile.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Handle nullable fields.
		if description != nil {
			profile.Description = *description
		}
		if timing != nil {
			profile.Timing = *timing
		}

		// Handle PostgreSQL arrays.
		profile.OSFamily = parsePostgreSQLArray(osFamily)
		profile.Scripts = parsePostgreSQLArray(scripts)

		// Handle JSONB options.
		if len(options) > 0 {
			profile.Options = JSONB(string(options))
		}

		profiles = append(profiles, profile)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate profile rows: %w", err)
	}

	return profiles, total, nil
}

// CreateProfile creates a new profile.
func (db *DB) CreateProfile(ctx context.Context, profileData interface{}) (*ScanProfile, error) {
	data, ok := profileData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile data format")
	}

	// Extract data from request.
	name, ok := data["name"].(string)
	if !ok {
		return nil, fmt.Errorf("profile name is required")
	}

	description := ""
	if desc, ok := data["description"].(string); ok {
		description = desc
	}

	scanType, ok := data["scan_type"].(string)
	if !ok {
		return nil, fmt.Errorf("scan_type is required")
	}

	ports, ok := data["ports"].(string)
	if !ok {
		return nil, fmt.Errorf("ports is required")
	}

	// Extract options and timing.
	var optionsJSON []byte
	var timingStr string

	if options, ok := data["options"].(map[string]interface{}); ok {
		if optionsBytes, err := json.Marshal(options); err == nil {
			optionsJSON = optionsBytes
		}
	}
	if optionsJSON == nil {
		optionsJSON = []byte("{}")
	}

	if timing, ok := data["timing"].(string); ok {
		timingStr = timing
	}

	// Generate profile ID from name (sanitized).
	profileID := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	now := time.Now().UTC()

	query := `
		INSERT INTO scan_profiles
		       (id, name, description, ports, scan_type, options, timing, priority, built_in, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, false, $8, $9)`

	_, err := db.ExecContext(ctx, query, profileID, name, description,
		ports, scanType, optionsJSON, timingStr, now, now)
	if err != nil {
		var pqErr *pq.Error
		if stdErrors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, errors.ErrConflictWithReason("profile",
				fmt.Sprintf("a profile named %q already exists", name))
		}
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	profile := &ScanProfile{
		ID:          profileID,
		Name:        name,
		Description: description,
		Ports:       ports,
		ScanType:    scanType,
		Options:     JSONB(optionsJSON),
		Timing:      timingStr,
		Priority:    0,
		BuiltIn:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return profile, nil
}

// GetProfile retrieves a profile by ID.
func (db *DB) GetProfile(ctx context.Context, id string) (*ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, ports, scan_type,
		       timing, scripts, options, priority, built_in,
		       created_at, updated_at
		FROM scan_profiles
		WHERE id = $1`

	profile := &ScanProfile{}
	var description sql.NullString
	var timing sql.NullString
	var options []byte
	var osFamily, scripts interface{}

	err := db.DB.QueryRowContext(ctx, query, id).Scan(
		&profile.ID,
		&profile.Name,
		&description,
		&osFamily,
		&profile.Ports,
		&profile.ScanType,
		&timing,
		&scripts,
		&options,
		&profile.Priority,
		&profile.BuiltIn,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("profile", id)
		}
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Handle nullable fields.
	if description.Valid {
		profile.Description = description.String
	}
	if timing.Valid {
		profile.Timing = timing.String
	}

	// Handle PostgreSQL arrays.
	if osFamily != nil {
		if arr, ok := osFamily.(pq.StringArray); ok {
			profile.OSFamily = []string(arr)
		}
	}
	if scripts != nil {
		if arr, ok := scripts.(pq.StringArray); ok {
			profile.Scripts = []string(arr)
		}
	}

	// Handle JSONB options.
	if len(options) > 0 {
		profile.Options = JSONB(string(options))
	}

	return profile, nil
}

// UpdateProfile updates an existing profile.
func (db *DB) UpdateProfile(ctx context.Context, id string, profileData interface{}) (*ScanProfile, error) {
	// Convert the interface{} to profile data.
	data, ok := profileData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile data format")
	}

	// Start a transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the profile exists and is not built-in.
	// This query always returns exactly one row, avoiding sql.ErrNoRows when
	// the profile does not exist.
	var exists bool
	var builtIn bool
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) > 0,
		        COALESCE(MAX(CASE WHEN built_in THEN 1 ELSE 0 END), 0) = 1
		 FROM scan_profiles WHERE id = $1`,
		id).Scan(&exists, &builtIn)
	if err != nil {
		return nil, fmt.Errorf("failed to check profile existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("profile", id)
	}
	if builtIn {
		return nil, errors.ErrForbidden("cannot update built-in profile")
	}

	// Build dynamic update query.
	fieldMappings := map[string]string{
		"name":        "name",
		"description": "description",
		"scan_type":   "scan_type",
		"ports":       "ports",
		"timing":      "timing",
		"scripts":     "scripts",
		"options":     "options",
		"priority":    "priority",
	}

	setParts := []string{}
	args := []interface{}{}
	argCount := 1

	for dataField, dbField := range fieldMappings {
		if value, exists := data[dataField]; exists {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", dbField, argCount))
			args = append(args, marshalSQLArg(value))
			argCount++
		}
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Always update the updated_at timestamp.
	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now().UTC())
	argCount++

	// Add WHERE clause argument.
	args = append(args, id)

	setClause := strings.Join(setParts, ", ")
	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scan_profiles SET ")
	queryBuilder.WriteString(setClause)
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argCount))
	updateQuery := queryBuilder.String()

	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve the updated profile.
	return db.GetProfile(ctx, id)
}

// DeleteProfile deletes a profile by ID.
func (db *DB) DeleteProfile(ctx context.Context, id string) error {
	// Start a transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the profile exists and is not built-in.
	// This query always returns exactly one row, avoiding sql.ErrNoRows when
	// the profile does not exist.
	var exists bool
	var builtIn bool
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) > 0,
		        COALESCE(MAX(CASE WHEN built_in THEN 1 ELSE 0 END), 0) = 1
		 FROM scan_profiles WHERE id = $1`,
		id).Scan(&exists, &builtIn)
	if err != nil {
		return fmt.Errorf("failed to check profile existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("profile", id)
	}
	if builtIn {
		return errors.ErrForbidden("cannot delete built-in profile")
	}

	// Check if profile is in use by any scan jobs.
	var inUse bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE profile_id = $1)",
		id).Scan(&inUse)
	if err != nil {
		return fmt.Errorf("failed to check profile usage: %w", err)
	}
	if inUse {
		return fmt.Errorf("cannot delete profile that is in use by scan jobs")
	}

	// Delete the profile.
	result, err := tx.ExecContext(ctx, "DELETE FROM scan_profiles WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("profile", id)
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
