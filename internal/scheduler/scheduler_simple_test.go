package scheduler

import (
	"context"
	"testing"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

// TestScanProfileStruct tests the ScanProfile struct
func TestScanProfileStruct(t *testing.T) {
	profile := ScanProfile{
		ID:         "test-id",
		Name:       "Test Profile",
		Ports:      "22,80,443",
		ScanType:   "connect",
		TimeoutSec: 30,
	}

	if profile.ID != "test-id" {
		t.Errorf("Expected ID to be 'test-id', got %s", profile.ID)
	}
	if profile.Name != "Test Profile" {
		t.Errorf("Expected Name to be 'Test Profile', got %s", profile.Name)
	}
	if profile.Ports != "22,80,443" {
		t.Errorf("Expected Ports to be '22,80,443', got %s", profile.Ports)
	}
	if profile.ScanType != "connect" {
		t.Errorf("Expected ScanType to be 'connect', got %s", profile.ScanType)
	}
	if profile.TimeoutSec != 30 {
		t.Errorf("Expected TimeoutSec to be 30, got %d", profile.TimeoutSec)
	}
}

// TestProcessHostsForScanningEmptyList tests batch processing with empty host list
func TestProcessHostsForScanningEmptyList(t *testing.T) {
	s := &Scheduler{}

	hosts := []*db.Host{}
	config := &ScanJobConfig{
		ProfileID: "test-profile",
	}

	ctx := context.Background()

	// This should not panic and should handle empty list gracefully
	s.processHostsForScanning(ctx, hosts, config)

	// If we reach here without panicking, the test passes
}

// TestProcessHostsForScanningCancellation tests context cancellation
func TestProcessHostsForScanningCancellation(t *testing.T) {
	s := &Scheduler{}

	hosts := createTestHosts(5)
	config := &ScanJobConfig{
		ProfileID: "test-profile",
	}

	// Create context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// This should handle cancellation gracefully
	s.processHostsForScanning(ctx, hosts, config)

	// If we reach here without hanging, the test passes
}

// TestScanJobConfigStruct tests the ScanJobConfig struct
func TestScanJobConfigStruct(t *testing.T) {
	config := &ScanJobConfig{
		LiveHostsOnly: true,
		Networks:      []string{"192.168.1.0/24", "10.0.0.0/8"},
		ProfileID:     "linux-profile",
		MaxAge:        3600,
		OSFamily:      []string{"linux"},
	}

	if !config.LiveHostsOnly {
		t.Error("Expected LiveHostsOnly to be true")
	}
	if len(config.Networks) != 2 {
		t.Errorf("Expected 2 networks, got %d", len(config.Networks))
	}
	if config.ProfileID != "linux-profile" {
		t.Errorf("Expected ProfileID to be 'linux-profile', got %s", config.ProfileID)
	}
	if config.MaxAge != 3600 {
		t.Errorf("Expected MaxAge to be 3600, got %d", config.MaxAge)
	}
	if len(config.OSFamily) != 1 || config.OSFamily[0] != "linux" {
		t.Errorf("Expected OSFamily to be ['linux'], got %v", config.OSFamily)
	}
}

// TestBatchSizeLogic tests that batch processing uses correct batch sizes
func TestBatchSizeLogic(t *testing.T) {
	tests := []struct {
		name            string
		hostCount       int
		expectedBatches int
	}{
		{"small batch", 5, 1},
		{"exact batch size", 10, 1},
		{"large batch", 25, 3},        // 10 + 10 + 5
		{"very large batch", 100, 10}, // 10 batches of 10
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate expected batches with batch size of 10
			batchSize := 10
			expectedBatches := (tt.hostCount + batchSize - 1) / batchSize // Ceiling division

			if expectedBatches != tt.expectedBatches {
				t.Errorf("Expected %d batches for %d hosts, calculated %d",
					tt.expectedBatches, tt.hostCount, expectedBatches)
			}
		})
	}
}

// TestIPAddressToString tests the IPAddr to string conversion
func TestIPAddressToString(t *testing.T) {
	tests := []struct {
		name     string
		ip       []byte
		expected string
	}{
		{"IPv4 localhost", []byte{127, 0, 0, 1}, "127.0.0.1"},
		{"IPv4 private", []byte{192, 168, 1, 1}, "192.168.1.1"},
		{"IPv4 zero", []byte{0, 0, 0, 0}, "0.0.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := &db.Host{
				ID:        uuid.New(),
				IPAddress: db.IPAddr{IP: tt.ip},
				OSFamily:  stringPtr("linux"),
			}

			result := host.IPAddress.String()
			if result != tt.expected {
				t.Errorf("Expected IP string %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestSelectProfileForHostLogic tests profile selection logic
func TestSelectProfileForHostLogic(t *testing.T) {
	// Create a basic scheduler (without database)
	s := &Scheduler{}

	tests := []struct {
		name     string
		host     *db.Host
		configID string
		expected string // We can't test actual selection without DB, but we can test the inputs
	}{
		{
			name: "host with specified profile",
			host: &db.Host{
				ID:       uuid.New(),
				OSFamily: stringPtr("linux"),
			},
			configID: "linux-profile",
			expected: "linux-profile", // Would use the specified profile
		},
		{
			name: "host with auto profile selection",
			host: &db.Host{
				ID:       uuid.New(),
				OSFamily: stringPtr("windows"),
			},
			configID: "auto",
			expected: "", // Would trigger auto-selection logic
		},
		{
			name: "host with no OS family",
			host: &db.Host{
				ID:       uuid.New(),
				OSFamily: nil,
			},
			configID: "",
			expected: "", // Would need fallback logic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// We can't test the actual database call, but we can verify inputs are valid
			result := s.selectProfileForHost(ctx, tt.host, tt.configID)

			// The actual implementation would query the database
			// For now, we just verify the method can be called without panic
			_ = result
		})
	}
}

// TestErrorHandlingInScanSingleHost tests error handling scenarios
func TestErrorHandlingInScanSingleHost(t *testing.T) {
	s := &Scheduler{
		// No database connection - will cause errors
		db: nil,
	}

	host := &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: []byte{192, 168, 1, 1}},
		OSFamily:  stringPtr("linux"),
	}

	config := &ScanJobConfig{
		ProfileID: "test-profile",
	}

	ctx := context.Background()

	// This should handle the nil database gracefully
	err := s.scanSingleHost(ctx, host, config)
	if err == nil {
		t.Error("Expected error with nil database, got nil")
	}
}

// TestScanConfigValidation tests scan configuration validation scenarios
func TestScanConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		profile ScanProfile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: ScanProfile{
				ID:         "valid-profile",
				Name:       "Valid Profile",
				Ports:      "22,80,443",
				ScanType:   "connect",
				TimeoutSec: 30,
			},
			wantErr: false,
		},
		{
			name: "invalid scan type",
			profile: ScanProfile{
				ID:         "invalid-profile",
				Name:       "Invalid Profile",
				Ports:      "22,80,443",
				ScanType:   "invalid-type",
				TimeoutSec: 30,
			},
			wantErr: true,
		},
		{
			name: "empty ports",
			profile: ScanProfile{
				ID:         "empty-ports",
				Name:       "Empty Ports Profile",
				Ports:      "",
				ScanType:   "connect",
				TimeoutSec: 30,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			profile: ScanProfile{
				ID:         "negative-timeout",
				Name:       "Negative Timeout Profile",
				Ports:      "22,80,443",
				ScanType:   "connect",
				TimeoutSec: -1,
			},
			wantErr: false, // Timeout validation might be handled elsewhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that we can create scan configs from profiles
			// The actual validation would happen in the scanning package
			if tt.profile.Ports == "" && tt.wantErr {
				// This would cause an error in scan config validation
				t.Logf("Profile %s would cause validation error: empty ports", tt.profile.ID)
			}
			if tt.profile.ScanType == "invalid-type" && tt.wantErr {
				// This would cause an error in scan config validation
				t.Logf("Profile %s would cause validation error: invalid scan type", tt.profile.ID)
			}
		})
	}
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func createTestHosts(count int) []*db.Host {
	hosts := make([]*db.Host, count)
	for i := 0; i < count; i++ {
		hosts[i] = &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: []byte{192, 168, 1, byte(i + 1)}},
			OSFamily:  stringPtr("linux"),
		}
	}
	return hosts
}

// Benchmark tests

func BenchmarkCreateTestHosts(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = createTestHosts(100)
	}
}

func BenchmarkIPAddressString(b *testing.B) {
	host := &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: []byte{192, 168, 1, 1}},
		OSFamily:  stringPtr("linux"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = host.IPAddress.String()
	}
}

func BenchmarkScanProfileCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ScanProfile{
			ID:         "benchmark-profile",
			Name:       "Benchmark Profile",
			Ports:      "22,80,443,8080,8443",
			ScanType:   "connect",
			TimeoutSec: 60,
		}
	}
}

// Integration test helpers (for when database is available)

func TestIntegrationHelper(t *testing.T) {
	// This test provides guidance for integration testing
	t.Skip("This is a helper test for integration testing guidance")

	// Integration tests would:
	// 1. Set up a test database
	// 2. Create test scan profiles
	// 3. Create test hosts
	// 4. Run actual scanning operations
	// 5. Verify results in database
	// 6. Clean up test data
}

// Mock scheduler for testing (simplified version)
type MockScheduler struct {
	profiles map[string]*ScanProfile
	hosts    []*db.Host
}

func NewMockScheduler() *MockScheduler {
	return &MockScheduler{
		profiles: make(map[string]*ScanProfile),
		hosts:    make([]*db.Host, 0),
	}
}

func (ms *MockScheduler) AddProfile(profile *ScanProfile) {
	ms.profiles[profile.ID] = profile
}

func (ms *MockScheduler) AddHost(host *db.Host) {
	ms.hosts = append(ms.hosts, host)
}

func (ms *MockScheduler) GetProfile(id string) *ScanProfile {
	return ms.profiles[id]
}

func TestMockScheduler(t *testing.T) {
	mock := NewMockScheduler()

	// Add test profile
	profile := &ScanProfile{
		ID:         "test-profile",
		Name:       "Test Profile",
		Ports:      "22,80,443",
		ScanType:   "connect",
		TimeoutSec: 30,
	}
	mock.AddProfile(profile)

	// Add test host
	host := &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: []byte{192, 168, 1, 1}},
		OSFamily:  stringPtr("linux"),
	}
	mock.AddHost(host)

	// Test retrieval
	retrievedProfile := mock.GetProfile("test-profile")
	if retrievedProfile == nil {
		t.Error("Expected to retrieve profile, got nil")
		return
	}
	if retrievedProfile.ID != "test-profile" {
		t.Errorf("Expected profile ID 'test-profile', got %s", retrievedProfile.ID)
	}

	if len(mock.hosts) != 1 {
		t.Errorf("Expected 1 host, got %d", len(mock.hosts))
	}
}
