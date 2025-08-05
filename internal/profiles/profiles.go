// Package profiles provides scanning profile management for scanorama.
// It handles predefined and custom scanning configurations, port lists,
// and scanning methodologies for different use cases.
package profiles

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Confidence scoring divisor for OS detection.
	confidenceScoringDivisor = 10
)

// Manager handles scan profile operations.
type Manager struct {
	db *db.DB
}

// NewManager creates a new profile manager.
func NewManager(database *db.DB) *Manager {
	return &Manager{
		db: database,
	}
}

// GetAll returns all scan profiles.
func (m *Manager) GetAll(ctx context.Context) ([]*db.ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, os_pattern, ports, scan_type,
		       timing, scripts, options, priority, built_in, created_at, updated_at
		FROM scan_profiles
		ORDER BY priority DESC, name ASC
	`

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query profiles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var profiles []*db.ScanProfile
	for rows.Next() {
		profile := &db.ScanProfile{}
		err := rows.Scan(
			&profile.ID, &profile.Name, &profile.Description,
			&profile.OSFamily, &profile.OSPattern, &profile.Ports,
			&profile.ScanType, &profile.Timing, &profile.Scripts,
			&profile.Options, &profile.Priority, &profile.BuiltIn,
			&profile.CreatedAt, &profile.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile: %w", err)
		}
		profiles = append(profiles, profile)
	}

	return profiles, rows.Err()
}

// GetByID returns a profile by ID.
func (m *Manager) GetByID(ctx context.Context, id string) (*db.ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, os_pattern, ports, scan_type,
		       timing, scripts, options, priority, built_in, created_at, updated_at
		FROM scan_profiles
		WHERE id = $1
	`

	profile := &db.ScanProfile{}
	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&profile.ID, &profile.Name, &profile.Description,
		&profile.OSFamily, &profile.OSPattern, &profile.Ports,
		&profile.ScanType, &profile.Timing, &profile.Scripts,
		&profile.Options, &profile.Priority, &profile.BuiltIn,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile %s: %w", id, err)
	}

	return profile, nil
}

// GetByOSFamily returns profiles matching the OS family.
func (m *Manager) GetByOSFamily(ctx context.Context, osFamily string) ([]*db.ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, os_pattern, ports, scan_type,
		       timing, scripts, options, priority, built_in, created_at, updated_at
		FROM scan_profiles
		WHERE $1 = ANY(os_family) OR array_length(os_family, 1) IS NULL
		ORDER BY priority DESC, name ASC
	`

	rows, err := m.db.QueryContext(ctx, query, osFamily)
	if err != nil {
		return nil, fmt.Errorf("failed to query profiles by OS family: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var profiles []*db.ScanProfile
	for rows.Next() {
		profile := &db.ScanProfile{}
		err := rows.Scan(
			&profile.ID, &profile.Name, &profile.Description,
			&profile.OSFamily, &profile.OSPattern, &profile.Ports,
			&profile.ScanType, &profile.Timing, &profile.Scripts,
			&profile.Options, &profile.Priority, &profile.BuiltIn,
			&profile.CreatedAt, &profile.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile: %w", err)
		}
		profiles = append(profiles, profile)
	}

	return profiles, rows.Err()
}

// SelectBestProfile selects the best profile for a host based on OS information.
func (m *Manager) SelectBestProfile(ctx context.Context, host *db.Host) (*db.ScanProfile, error) {
	// Get OS fingerprint
	osInfo := host.GetOSFingerprint()

	var profiles []*db.ScanProfile
	var err error

	if osInfo != nil && osInfo.Family != "" {
		// Get profiles for the specific OS family
		profiles, err = m.GetByOSFamily(ctx, osInfo.Family)
	} else {
		// Get all profiles if no OS info available
		profiles, err = m.GetAll(ctx)
	}

	if err != nil {
		return nil, err
	}

	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles available")
	}

	// Find the best matching profile
	var bestProfile *db.ScanProfile
	var bestScore int

	for _, profile := range profiles {
		score := m.calculateProfileScore(profile, osInfo)
		if score > bestScore {
			bestScore = score
			bestProfile = profile
		}
	}

	// If no specific match found, return generic default
	if bestProfile == nil {
		return m.GetByID(ctx, "generic-default")
	}

	return bestProfile, nil
}

// calculateProfileScore calculates how well a profile matches the host OS.
func (m *Manager) calculateProfileScore(profile *db.ScanProfile, osInfo *db.OSFingerprint) int {
	score := 0

	if osInfo == nil {
		// If no OS info, prefer generic profiles
		if len(profile.OSFamily) == 0 {
			score += 10
		}
		return score
	}

	// Check OS family match
	for _, family := range profile.OSFamily {
		if strings.EqualFold(family, osInfo.Family) {
			score += 50
			break
		}
	}

	// Check OS name pattern match
	if osInfo.Name != "" {
		for _, pattern := range profile.OSPattern {
			matched, err := regexp.MatchString(pattern, osInfo.Name)
			if err == nil && matched {
				score += 30
				break
			}
		}
	}

	// Add priority score
	score += profile.Priority

	// Bonus for higher OS detection confidence
	if osInfo.Confidence > 0 {
		score += osInfo.Confidence / confidenceScoringDivisor
	}

	return score
}

// Create creates a new custom scan profile.
func (m *Manager) Create(ctx context.Context, profile *db.ScanProfile) error {
	query := `
		INSERT INTO scan_profiles (id, name, description, os_family, os_pattern, ports,
								  scan_type, timing, scripts, options, priority, built_in,
								  created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
	`

	_, err := m.db.ExecContext(ctx, query,
		profile.ID, profile.Name, profile.Description,
		profile.OSFamily, profile.OSPattern, profile.Ports,
		profile.ScanType, profile.Timing, profile.Scripts,
		profile.Options, profile.Priority, profile.BuiltIn)
	if err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}

	return nil
}

// Update updates an existing scan profile.
func (m *Manager) Update(ctx context.Context, profile *db.ScanProfile) error {
	query := `
		UPDATE scan_profiles SET
			name = $2, description = $3, os_family = $4, os_pattern = $5,
			ports = $6, scan_type = $7, timing = $8, scripts = $9,
			options = $10, priority = $11, updated_at = NOW()
		WHERE id = $1 AND built_in = false
	`

	result, err := m.db.ExecContext(ctx, query,
		profile.ID, profile.Name, profile.Description,
		profile.OSFamily, profile.OSPattern, profile.Ports,
		profile.ScanType, profile.Timing, profile.Scripts,
		profile.Options, profile.Priority)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("profile not found or is built-in")
	}

	return nil
}

// Delete deletes a custom scan profile.
func (m *Manager) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM scan_profiles WHERE id = $1 AND built_in = false`

	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("profile not found or is built-in")
	}

	return nil
}

// ValidateProfile validates a scan profile configuration.
func (m *Manager) ValidateProfile(profile *db.ScanProfile) error {
	if profile.ID == "" {
		return fmt.Errorf("profile ID is required")
	}

	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}

	if profile.Ports == "" {
		return fmt.Errorf("ports specification is required")
	}

	// Validate scan type
	validScanTypes := map[string]bool{
		db.ScanTypeConnect: true,
		db.ScanTypeSYN:     true,
		db.ScanTypeVersion: true,
	}

	if !validScanTypes[profile.ScanType] {
		return fmt.Errorf("invalid scan type: %s", profile.ScanType)
	}

	// Validate timing
	validTimings := map[string]bool{
		db.ScanTimingParanoid:   true,
		db.ScanTimingPolite:     true,
		db.ScanTimingNormal:     true,
		db.ScanTimingAggressive: true,
		db.ScanTimingInsane:     true,
	}

	if profile.Timing != "" && !validTimings[profile.Timing] {
		return fmt.Errorf("invalid timing: %s", profile.Timing)
	}

	// Validate OS patterns (compile regex)
	for _, pattern := range profile.OSPattern {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid OS pattern regex '%s': %w", pattern, err)
		}
	}

	return nil
}

// GetProfileStats returns statistics about profile usage.
func (m *Manager) GetProfileStats(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT COALESCE(profile_id, 'none') as profile_id, COUNT(*) as usage_count
		FROM scan_jobs
		WHERE profile_id IS NOT NULL OR profile_id IS NULL
		GROUP BY profile_id
		ORDER BY usage_count DESC
	`

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile stats: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	stats := make(map[string]int)
	for rows.Next() {
		var profileID string
		var count int

		if err := rows.Scan(&profileID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}

		stats[profileID] = count
	}

	return stats, rows.Err()
}

// CloneProfile creates a copy of an existing profile with a new ID.
func (m *Manager) CloneProfile(ctx context.Context, sourceID, newID, newName string) error {
	// Get the source profile
	source, err := m.GetByID(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source profile: %w", err)
	}

	// Create the new profile
	newProfile := *source
	newProfile.ID = newID
	newProfile.Name = newName
	newProfile.BuiltIn = false // Custom profiles are never built-in

	return m.Create(ctx, &newProfile)
}
