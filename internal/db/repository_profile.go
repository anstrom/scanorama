// Package db provides typed repository for scan profile database operations.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"

	"github.com/anstrom/scanorama/internal/errors"
)

// ProfileRepository implements ProfileStore against a *DB connection.
type ProfileRepository struct {
	db *DB
}

// NewProfileRepository creates a new ProfileRepository.
func NewProfileRepository(db *DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

// ListProfiles retrieves profiles with filtering and pagination.
func (r *ProfileRepository) ListProfiles(ctx context.Context, filters ProfileFilters,
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
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, sanitizeDBError("get profile count", err)
	}

	// Get paginated results.
	listQuery := fmt.Sprintf(
		"%s %s ORDER BY priority DESC, name ASC LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, argIndex+1, argIndex+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, sanitizeDBError("list profiles", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
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
func (r *ProfileRepository) CreateProfile(ctx context.Context, input CreateProfileInput) (*ScanProfile, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("profile name is required")
	}
	if input.ScanType == "" {
		return nil, fmt.Errorf("scan_type is required")
	}
	if input.Ports == "" {
		return nil, fmt.Errorf("ports is required")
	}

	// Marshal options to JSON.
	var optionsJSON []byte
	if len(input.Options) > 0 {
		var err error
		if optionsJSON, err = json.Marshal(input.Options); err != nil {
			return nil, fmt.Errorf("failed to marshal options: %w", err)
		}
	}
	if optionsJSON == nil {
		optionsJSON = []byte("{}")
	}

	// Generate profile ID from name (sanitized).
	profileID := strings.ToLower(strings.ReplaceAll(input.Name, " ", "-"))
	now := time.Now().UTC()

	query := `
		INSERT INTO scan_profiles
		       (id, name, description, ports, scan_type, options, timing, priority, built_in, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, false, $8, $9)`

	_, err := r.db.ExecContext(ctx, query, profileID, input.Name, input.Description,
		input.Ports, input.ScanType, optionsJSON, input.Timing, now, now)
	if err != nil {
		if pq.As(err, pqerror.UniqueViolation) != nil {
			return nil, errors.ErrConflictWithReason("profile",
				fmt.Sprintf("a profile named %q already exists", input.Name))
		}
		return nil, sanitizeDBError("create profile", err)
	}

	profile := &ScanProfile{
		ID:          profileID,
		Name:        input.Name,
		Description: input.Description,
		Ports:       input.Ports,
		ScanType:    input.ScanType,
		Options:     JSONB(optionsJSON),
		Timing:      input.Timing,
		Priority:    0,
		BuiltIn:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return profile, nil
}

// GetProfile retrieves a profile by ID.
func (r *ProfileRepository) GetProfile(ctx context.Context, id string) (*ScanProfile, error) {
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

	err := r.db.QueryRowContext(ctx, query, id).Scan(
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
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("profile", id)
		}
		return nil, sanitizeDBError("get profile", err)
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
func (r *ProfileRepository) UpdateProfile(
	ctx context.Context, id string, input UpdateProfileInput,
) (*ScanProfile, error) {
	// Start a transaction.
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
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
		return nil, sanitizeDBError("check profile existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("profile", id)
	}
	if builtIn {
		return nil, errors.ErrForbidden("cannot update built-in profile")
	}

	setParts, args, err := buildProfileUpdateSetParts(input)
	if err != nil {
		return nil, err
	}
	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Append the WHERE clause argument and build the query.
	whereArgNum := len(args) + 1
	args = append(args, id)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scan_profiles SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(whereArgNum))

	_, err = tx.ExecContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return nil, sanitizeDBError("update profile", err)
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve the updated profile.
	return r.GetProfile(ctx, id)
}

// buildProfileUpdateSetParts constructs the SET clause parts and argument slice
// for an UPDATE scan_profiles statement from the non-nil fields in input.
func buildProfileUpdateSetParts(input UpdateProfileInput) (setParts []string, args []interface{}, err error) {
	argCount := 1

	addStr := func(col string, val *string) {
		if val != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argCount))
			args = append(args, *val)
			argCount++
		}
	}

	addStr("name", input.Name)
	addStr("description", input.Description)
	addStr("scan_type", input.ScanType)
	addStr("ports", input.Ports)
	addStr("timing", input.Timing)

	if input.Scripts != nil {
		setParts = append(setParts, fmt.Sprintf("scripts = $%d", argCount))
		args = append(args, pq.Array(input.Scripts))
		argCount++
	}

	if input.Options != nil {
		var optionsJSON []byte
		if optionsJSON, err = json.Marshal(input.Options); err != nil {
			return nil, nil, fmt.Errorf("failed to marshal options: %w", err)
		}
		setParts = append(setParts, fmt.Sprintf("options = $%d", argCount))
		args = append(args, optionsJSON)
		argCount++
	}

	if input.Priority != nil {
		setParts = append(setParts, fmt.Sprintf("priority = $%d", argCount))
		args = append(args, *input.Priority)
		argCount++
	}

	// Always update the updated_at timestamp.
	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now().UTC())

	return setParts, args, nil
}

// DeleteProfile deletes a profile by ID.
func (r *ProfileRepository) DeleteProfile(ctx context.Context, id string) error {
	// Start a transaction.
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
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
		return sanitizeDBError("check profile existence", err)
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
		return sanitizeDBError("check profile usage", err)
	}
	if inUse {
		return fmt.Errorf("cannot delete profile that is in use by scan jobs")
	}

	// Delete the profile.
	result, err := tx.ExecContext(ctx, "DELETE FROM scan_profiles WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete profile", err)
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
