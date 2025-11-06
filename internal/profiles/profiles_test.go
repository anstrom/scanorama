package profiles

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewManager tests the creation of a new Manager.
func TestNewManager(t *testing.T) {
	mockDB := &db.DB{}
	manager := NewManager(mockDB)

	assert.NotNil(t, manager)
	assert.Equal(t, mockDB, manager.db)
}

// TestCalculateProfileScore tests profile scoring logic.
func TestCalculateProfileScore(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name     string
		profile  *db.ScanProfile
		osInfo   *db.OSFingerprint
		expected int
	}{
		{
			name: "exact OS family match with high confidence",
			profile: &db.ScanProfile{
				OSFamily: []string{"linux"},
				Priority: 10,
			},
			osInfo: &db.OSFingerprint{
				Family:     "Linux",
				Name:       "Ubuntu 22.04",
				Confidence: 95,
			},
			expected: 50 + 10 + (95 / confidenceScoringDivisor), // 69
		},
		{
			name: "OS family and pattern match",
			profile: &db.ScanProfile{
				OSFamily:  []string{"linux"},
				OSPattern: []string{"Ubuntu.*"},
				Priority:  5,
			},
			osInfo: &db.OSFingerprint{
				Family:     "Linux",
				Name:       "Ubuntu 20.04 LTS",
				Confidence: 90,
			},
			expected: 50 + 30 + 5 + (90 / confidenceScoringDivisor), // 94
		},
		{
			name: "no OS info - prefer generic",
			profile: &db.ScanProfile{
				OSFamily: []string{},
				Priority: 0,
			},
			osInfo:   nil,
			expected: 10,
		},
		{
			name: "no OS info - specific profile",
			profile: &db.ScanProfile{
				OSFamily: []string{"windows"},
				Priority: 5,
			},
			osInfo:   nil,
			expected: 0,
		},
		{
			name: "pattern mismatch",
			profile: &db.ScanProfile{
				OSFamily:  []string{"windows"},
				OSPattern: []string{"Windows Server.*"},
				Priority:  5,
			},
			osInfo: &db.OSFingerprint{
				Family:     "Linux",
				Name:       "Ubuntu",
				Confidence: 80,
			},
			expected: 5 + (80 / confidenceScoringDivisor), // 13
		},
		{
			name: "invalid regex pattern",
			profile: &db.ScanProfile{
				OSFamily:  []string{"linux"},
				OSPattern: []string{"[invalid(regex"},
				Priority:  10,
			},
			osInfo: &db.OSFingerprint{
				Family:     "Linux",
				Name:       "Ubuntu",
				Confidence: 50,
			},
			expected: 50 + 10 + (50 / confidenceScoringDivisor), // 65 (pattern fails silently)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := manager.calculateProfileScore(tt.profile, tt.osInfo)
			assert.Equal(t, tt.expected, score)
		})
	}
}

// TestValidateProfile tests profile validation.
func TestValidateProfile(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name        string
		profile     *db.ScanProfile
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid profile",
			profile: &db.ScanProfile{
				ID:       "test-profile",
				Name:     "Test Profile",
				Ports:    "1-1024",
				ScanType: db.ScanTypeSYN,
				Timing:   db.ScanTimingNormal,
			},
			expectError: false,
		},
		{
			name: "missing ID",
			profile: &db.ScanProfile{
				Name:     "Test",
				Ports:    "80,443",
				ScanType: db.ScanTypeConnect,
			},
			expectError: true,
			errorMsg:    "profile ID is required",
		},
		{
			name: "missing name",
			profile: &db.ScanProfile{
				ID:       "test",
				Ports:    "80,443",
				ScanType: db.ScanTypeConnect,
			},
			expectError: true,
			errorMsg:    "profile name is required",
		},
		{
			name: "missing ports",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				ScanType: db.ScanTypeConnect,
			},
			expectError: true,
			errorMsg:    "ports specification is required",
		},
		{
			name: "invalid scan type",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				Ports:    "80",
				ScanType: "invalid-scan",
			},
			expectError: true,
			errorMsg:    "invalid scan type",
		},
		{
			name: "invalid timing",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				Ports:    "80",
				ScanType: db.ScanTypeConnect,
				Timing:   "ultra-fast",
			},
			expectError: true,
			errorMsg:    "invalid timing",
		},
		{
			name: "invalid OS pattern regex",
			profile: &db.ScanProfile{
				ID:        "test",
				Name:      "Test",
				Ports:     "80",
				ScanType:  db.ScanTypeConnect,
				OSPattern: []string{"[invalid(regex"},
			},
			expectError: true,
			errorMsg:    "invalid OS pattern regex",
		},
		{
			name: "all valid scan types",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				Ports:    "80",
				ScanType: db.ScanTypeVersion,
			},
			expectError: false,
		},
		{
			name: "all valid timings",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				Ports:    "80",
				ScanType: db.ScanTypeConnect,
				Timing:   db.ScanTimingAggressive,
			},
			expectError: false,
		},
		{
			name: "empty timing is valid",
			profile: &db.ScanProfile{
				ID:       "test",
				Name:     "Test",
				Ports:    "80",
				ScanType: db.ScanTypeConnect,
				Timing:   "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateProfile(tt.profile)
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

// Integration tests require a real database connection.
// These tests should be run with the -short flag disabled.

func setupTestDB(t *testing.T) (database *db.DB, cleanup func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get database configuration
	dbConfig := &db.Config{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     getEnvIntOrDefault("TEST_DB_PORT", 5432),
		Database: getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username: getEnvOrDefault("TEST_DB_USER", "test_user"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:  "disable",
	}

	ctx := context.Background()
	var err error
	database, err = db.Connect(ctx, dbConfig)
	if err != nil {
		t.Skipf("Failed to connect to test database: %v", err)
	}

	cleanup = func() {
		database.Close()
	}

	return database, cleanup
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := context.Background().Value(key); val != nil {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func TestManager_GetAll_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	profiles, err := manager.GetAll(ctx)
	require.NoError(t, err)
	assert.NotNil(t, profiles)
	// Should have at least some built-in profiles
	assert.GreaterOrEqual(t, len(profiles), 1)
}

func TestManager_CreateGetUpdateDelete_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create a test profile
	profileID := uuid.New().String()
	optionsJSON, _ := json.Marshal([]string{"-v"})
	profile := &db.ScanProfile{
		ID:          profileID,
		Name:        "Test Profile",
		Description: "A test profile",
		OSFamily:    pq.StringArray{"linux"},
		OSPattern:   pq.StringArray{"Ubuntu.*"},
		Ports:       "1-1024",
		ScanType:    db.ScanTypeSYN,
		Timing:      db.ScanTimingNormal,
		Scripts:     pq.StringArray{"default"},
		Options:     db.JSONB(optionsJSON),
		Priority:    5,
		BuiltIn:     false,
	}

	// Test Create
	err := manager.Create(ctx, profile)
	require.NoError(t, err)

	// Test GetByID
	retrieved, err := manager.GetByID(ctx, profileID)
	require.NoError(t, err)
	assert.Equal(t, profile.ID, retrieved.ID)
	assert.Equal(t, profile.Name, retrieved.Name)
	assert.Equal(t, profile.Description, retrieved.Description)
	assert.Equal(t, profile.ScanType, retrieved.ScanType)
	assert.Equal(t, profile.Timing, retrieved.Timing)
	assert.Equal(t, profile.Priority, retrieved.Priority)
	assert.False(t, retrieved.BuiltIn)

	// Test Update
	retrieved.Name = "Updated Profile"
	retrieved.Description = "Updated description"
	retrieved.Priority = 10
	err = manager.Update(ctx, retrieved)
	require.NoError(t, err)

	// Verify update
	updated, err := manager.GetByID(ctx, profileID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Profile", updated.Name)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, 10, updated.Priority)

	// Test Delete
	err = manager.Delete(ctx, profileID)
	require.NoError(t, err)

	// Verify deletion
	_, err = manager.GetByID(ctx, profileID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get profile")
}

func TestManager_GetByOSFamily_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create test profiles with different OS families
	linuxProfile := &db.ScanProfile{
		ID:       uuid.New().String(),
		Name:     "Linux Profile",
		OSFamily: pq.StringArray{"linux"},
		Ports:    "1-1024",
		ScanType: db.ScanTypeConnect,
		Priority: 5,
		BuiltIn:  false,
	}
	err := manager.Create(ctx, linuxProfile)
	require.NoError(t, err)
	defer manager.Delete(ctx, linuxProfile.ID)

	// Get profiles by OS family
	profiles, err := manager.GetByOSFamily(ctx, "linux")
	require.NoError(t, err)
	assert.NotEmpty(t, profiles)

	// Verify at least our created profile is in the results
	found := false
	for _, p := range profiles {
		if p.ID == linuxProfile.ID {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestManager_SelectBestProfile_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create a specific profile for Linux
	linuxProfile := &db.ScanProfile{
		ID:          uuid.New().String(),
		Name:        "Linux Test Profile",
		Description: "For Linux systems",
		OSFamily:    pq.StringArray{"linux"},
		OSPattern:   pq.StringArray{"Ubuntu.*", "Debian.*"},
		Ports:       "1-1024",
		ScanType:    db.ScanTypeSYN,
		Timing:      db.ScanTimingNormal,
		Priority:    100, // High priority to ensure selection
		BuiltIn:     false,
	}
	err := manager.Create(ctx, linuxProfile)
	require.NoError(t, err)
	defer manager.Delete(ctx, linuxProfile.ID)

	tests := []struct {
		name            string
		host            *db.Host
		expectProfileID string
	}{
		{
			name: "host with Linux OS",
			host: &db.Host{
				ID:           uuid.New(),
				IPAddress:    db.IPAddr{IP: net.ParseIP("192.168.1.100")},
				OSName:       stringPtr("Ubuntu 22.04 LTS"),
				OSFamily:     stringPtr("linux"),
				OSConfidence: intPtr(95),
			},
			expectProfileID: linuxProfile.ID,
		},
		{
			name: "host with no OS info",
			host: &db.Host{
				ID:        uuid.New(),
				IPAddress: db.IPAddr{IP: net.ParseIP("192.168.1.200")},
			},
			expectProfileID: "", // Will get generic-default or any profile
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := manager.SelectBestProfile(ctx, tt.host)
			require.NoError(t, err)
			assert.NotNil(t, profile)

			if tt.expectProfileID != "" {
				assert.Equal(t, tt.expectProfileID, profile.ID)
			} else {
				// Should return some profile
				assert.NotEmpty(t, profile.ID)
			}
		})
	}
}

func TestManager_CloneProfile_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create source profile
	sourceID := uuid.New().String()
	source := &db.ScanProfile{
		ID:          sourceID,
		Name:        "Source Profile",
		Description: "Original profile",
		OSFamily:    pq.StringArray{"windows"},
		Ports:       "80,443",
		ScanType:    db.ScanTypeConnect,
		Timing:      db.ScanTimingNormal,
		Priority:    5,
		BuiltIn:     false,
	}
	err := manager.Create(ctx, source)
	require.NoError(t, err)
	defer manager.Delete(ctx, sourceID)

	// Clone the profile
	cloneID := uuid.New().String()
	cloneName := "Cloned Profile"
	err = manager.CloneProfile(ctx, sourceID, cloneID, cloneName)
	require.NoError(t, err)
	defer manager.Delete(ctx, cloneID)

	// Verify the clone
	cloned, err := manager.GetByID(ctx, cloneID)
	require.NoError(t, err)
	assert.Equal(t, cloneID, cloned.ID)
	assert.Equal(t, cloneName, cloned.Name)
	assert.Equal(t, source.Description, cloned.Description)
	assert.Equal(t, source.Ports, cloned.Ports)
	assert.Equal(t, source.ScanType, cloned.ScanType)
	assert.Equal(t, source.Timing, cloned.Timing)
	assert.Equal(t, source.Priority, cloned.Priority)
	assert.False(t, cloned.BuiltIn) // Clones are never built-in
}

func TestManager_CloneProfile_NonExistent_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Try to clone a non-existent profile
	err := manager.CloneProfile(ctx, "non-existent", uuid.New().String(), "Clone")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get source profile")
}

func TestManager_Update_BuiltIn_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create a built-in profile
	builtInID := uuid.New().String()
	builtIn := &db.ScanProfile{
		ID:       builtInID,
		Name:     "Built-in Profile",
		Ports:    "1-1024",
		ScanType: db.ScanTypeConnect,
		Priority: 10,
		BuiltIn:  true,
	}
	err := manager.Create(ctx, builtIn)
	require.NoError(t, err)
	defer manager.Delete(ctx, builtInID)

	// Try to update the built-in profile
	builtIn.Name = "Modified Name"
	err = manager.Update(ctx, builtIn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found or is built-in")
}

func TestManager_Delete_BuiltIn_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// Create a built-in profile
	builtInID := uuid.New().String()
	builtIn := &db.ScanProfile{
		ID:       builtInID,
		Name:     "Built-in Profile",
		Ports:    "1-1024",
		ScanType: db.ScanTypeConnect,
		Priority: 10,
		BuiltIn:  true,
	}
	err := manager.Create(ctx, builtIn)
	require.NoError(t, err)

	// Try to delete the built-in profile
	err = manager.Delete(ctx, builtInID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found or is built-in")

	// Clean up manually
	database.ExecContext(ctx, "DELETE FROM scan_profiles WHERE id = $1", builtInID)
}

func TestManager_GetProfileStats_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	stats, err := manager.GetProfileStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	// Stats might be empty if no scan jobs exist
}

func TestManager_GetByID_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	_, err := manager.GetByID(ctx, "non-existent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get profile")
}

func TestManager_Delete_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	err := manager.Delete(ctx, "non-existent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "profile not found or is built-in")
}

func TestManager_SelectBestProfile_NoProfiles_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	manager := NewManager(database)
	ctx := context.Background()

	// This test assumes the database might have profiles
	// In a real scenario with no profiles, it should return an error
	host := &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: net.ParseIP("192.168.1.1")},
	}

	profile, err := manager.SelectBestProfile(ctx, host)
	// Either we get a profile (if any exist) or an error
	if err != nil {
		assert.Contains(t, err.Error(), "no profiles available")
	} else {
		assert.NotNil(t, profile)
	}
}

// Test edge cases for calculateProfileScore
func TestCalculateProfileScore_EdgeCases(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name    string
		profile *db.ScanProfile
		osInfo  *db.OSFingerprint
	}{
		{
			name: "nil OSInfo",
			profile: &db.ScanProfile{
				OSFamily: []string{"linux"},
				Priority: 10,
			},
			osInfo: nil,
		},
		{
			name: "empty OSInfo fields",
			profile: &db.ScanProfile{
				OSFamily: []string{"linux"},
				Priority: 10,
			},
			osInfo: &db.OSFingerprint{
				Family:     "",
				Name:       "",
				Confidence: 0,
			},
		},
		{
			name: "multiple OS families",
			profile: &db.ScanProfile{
				OSFamily: []string{"linux", "unix", "freebsd"},
				Priority: 10,
			},
			osInfo: &db.OSFingerprint{
				Family:     "FreeBSD",
				Confidence: 80,
			},
		},
		{
			name: "multiple OS patterns",
			profile: &db.ScanProfile{
				OSFamily:  []string{"linux"},
				OSPattern: []string{"Ubuntu.*", "Debian.*", "CentOS.*"},
				Priority:  10,
			},
			osInfo: &db.OSFingerprint{
				Family:     "Linux",
				Name:       "CentOS 8",
				Confidence: 90,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := manager.calculateProfileScore(tt.profile, tt.osInfo)
			// Just verify it doesn't panic
			assert.GreaterOrEqual(t, score, 0)
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
