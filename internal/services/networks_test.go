package services

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewNetworkService tests the creation of a new NetworkService.
func TestNewNetworkService(t *testing.T) {
	mockDB := &db.DB{}
	service := NewNetworkService(mockDB)

	assert.NotNil(t, service)
	assert.Equal(t, mockDB, service.database)
}

// TestNormalizeCIDR tests CIDR normalization logic.
func TestNormalizeCIDR(t *testing.T) {
	service := &NetworkService{}

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "valid CIDR",
			input:       "192.168.1.0/24",
			expected:    "192.168.1.0/24",
			expectError: false,
		},
		{
			name:        "single IPv4 address",
			input:       "192.168.1.1",
			expected:    "192.168.1.1/32",
			expectError: false,
		},
		{
			name:        "single IPv6 address",
			input:       "2001:db8::1",
			expected:    "2001:db8::1/128",
			expectError: false,
		},
		{
			name:        "valid IPv6 CIDR",
			input:       "2001:db8::/32",
			expected:    "2001:db8::/32",
			expectError: false,
		},
		{
			name:        "invalid CIDR",
			input:       "192.168.1.0/33",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid IP",
			input:       "not-an-ip",
			expected:    "",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "invalid format",
			input:       "192.168.1",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.normalizeCIDR(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid CIDR or IP address")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestValidateNetworkConfig tests network config validation.
func TestValidateNetworkConfig(t *testing.T) {
	service := &NetworkService{}

	tests := []struct {
		name        string
		config      *config.NetworkConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: &config.NetworkConfig{
				Name: "Test Network",
				CIDR: "192.168.1.0/24",
			},
			expectError: false,
		},
		{
			name: "missing name",
			config: &config.NetworkConfig{
				CIDR: "192.168.1.0/24",
			},
			expectError: true,
			errorMsg:    "network name is required",
		},
		{
			name: "missing CIDR",
			config: &config.NetworkConfig{
				Name: "Test Network",
			},
			expectError: true,
			errorMsg:    "invalid CIDR",
		},
		{
			name: "invalid CIDR format",
			config: &config.NetworkConfig{
				Name: "Test Network",
				CIDR: "not-a-cidr",
			},
			expectError: true,
			errorMsg:    "invalid CIDR",
		},
		{
			name: "invalid CIDR range",
			config: &config.NetworkConfig{
				Name: "Test Network",
				CIDR: "192.168.1.0/33",
			},
			expectError: true,
			errorMsg:    "invalid CIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateNetworkConfig(tt.config)
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

func TestNetworkService_CreateGetUpdateDelete_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Test Create
	network, err := service.CreateNetwork(
		ctx,
		"Test Network",
		"10.0.0.0/24",
		"A test network",
		"ping",
		true,
	)
	require.NoError(t, err)
	assert.NotNil(t, network)
	assert.Equal(t, "Test Network", network.Name)
	assert.Equal(t, "10.0.0.0/24", network.CIDR)
	assert.Equal(t, "A test network", network.Description)
	assert.Equal(t, "ping", network.DiscoveryMethod)
	assert.True(t, network.IsActive)

	networkID := network.ID

	// Test GetNetworkByID
	retrieved, err := service.GetNetworkByID(ctx, networkID)
	require.NoError(t, err)
	assert.Equal(t, networkID, retrieved.Network.ID)
	assert.Equal(t, "Test Network", retrieved.Network.Name)

	// Test GetNetworkByName
	retrievedByName, err := service.GetNetworkByName(ctx, "Test Network")
	require.NoError(t, err)
	assert.Equal(t, networkID, retrievedByName.Network.ID)

	// Test Update
	updated, err := service.UpdateNetwork(
		ctx,
		networkID,
		"Updated Network",
		"10.0.1.0/24",
		"Updated description",
		"tcp",
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, "Updated Network", updated.Name)
	assert.Equal(t, "10.0.1.0/24", updated.CIDR)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, "tcp", updated.DiscoveryMethod)
	assert.False(t, updated.IsActive)

	// Test Delete
	err = service.DeleteNetwork(ctx, networkID)
	require.NoError(t, err)

	// Verify deletion
	_, err = service.GetNetworkByID(ctx, networkID)
	assert.Error(t, err)
}

func TestNetworkService_CreateNetwork_ValidationErrors_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	tests := []struct {
		name          string
		networkName   string
		cidr          string
		method        string
		errorContains string
	}{
		{
			name:          "invalid CIDR",
			networkName:   "Invalid CIDR",
			cidr:          "not-a-cidr",
			method:        "ping",
			errorContains: "invalid CIDR",
		},
		{
			name:          "invalid discovery method",
			networkName:   "Invalid Method",
			cidr:          "192.168.1.0/24",
			method:        "invalid-method",
			errorContains: "invalid discovery method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateNetwork(ctx, tt.networkName, tt.cidr, "Test", tt.method, true)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
		})
	}
}

func TestNetworkService_ListNetworks_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Create test networks
	activeNetwork, err := service.CreateNetwork(ctx, "Active Network", "10.1.0.0/24", "Test", "ping", true)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, activeNetwork.ID)

	inactiveNetwork, err := service.CreateNetwork(ctx, "Inactive Network", "10.2.0.0/24", "Test", "ping", false)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, inactiveNetwork.ID)

	// Test listing all networks
	allNetworks, err := service.ListNetworks(ctx, false)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allNetworks), 2)

	// Test listing active networks only
	activeNetworks, err := service.ListNetworks(ctx, true)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(activeNetworks), 1)

	// Verify active network is in the list
	foundActive := false
	for _, n := range activeNetworks {
		if n.ID == activeNetwork.ID {
			foundActive = true
			break
		}
	}
	assert.True(t, foundActive)
}

func TestNetworkService_GetActiveNetworks_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Create an active network
	activeNetwork, err := service.CreateNetwork(ctx, "Active Test", "10.3.0.0/24", "Test", "ping", true)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, activeNetwork.ID)

	// Get active networks
	networks, err := service.GetActiveNetworks(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(networks), 1)

	// Verify our network is in the list
	found := false
	for _, n := range networks {
		if n.Network.ID == activeNetwork.ID {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestNetworkService_SeedNetworksFromConfig_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	tests := []struct {
		name        string
		config      *config.Config
		expectError bool
	}{
		{
			name: "auto-seed disabled",
			config: &config.Config{
				Discovery: config.DiscoveryConfig{
					AutoSeed: false,
					Networks: []config.NetworkConfig{
						{
							Name: "Test Network",
							CIDR: "192.168.1.0/24",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid networks",
			config: &config.Config{
				Discovery: config.DiscoveryConfig{
					AutoSeed: true,
					Defaults: config.DiscoveryDefaults{
						Method: "ping",
					},
					Networks: []config.NetworkConfig{
						{
							Name:        "Seed Network 1",
							CIDR:        "172.16.0.0/24",
							Description: "Test network 1",
						},
						{
							Name:        "Seed Network 2",
							CIDR:        "172.16.1.0/24",
							Description: "Test network 2",
							Method:      "tcp",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid network config",
			config: &config.Config{
				Discovery: config.DiscoveryConfig{
					AutoSeed: true,
					Networks: []config.NetworkConfig{
						{
							Name: "", // Invalid: empty name
							CIDR: "192.168.1.0/24",
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SeedNetworksFromConfig(ctx, tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Cleanup created networks
			if !tt.expectError && tt.config.Discovery.AutoSeed {
				for _, netConfig := range tt.config.Discovery.Networks {
					if netConfig.Name != "" {
						if network, err := service.GetNetworkByName(ctx, netConfig.Name); err == nil {
							service.DeleteNetwork(ctx, network.Network.ID)
						}
					}
				}
			}
		})
	}
}

func TestNetworkService_AddRemoveExclusion_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Create a test network
	network, err := service.CreateNetwork(ctx, "Exclusion Test", "10.4.0.0/24", "Test", "ping", true)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, network.ID)

	// Test adding an exclusion
	exclusion, err := service.AddExclusion(ctx, &network.ID, "10.4.0.100", "Test exclusion")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, exclusion.ID)

	// Get exclusions to verify
	exclusions, err := service.getNetworkExclusions(ctx, &network.ID)
	require.NoError(t, err)
	assert.Len(t, exclusions, 1)
	assert.Equal(t, "10.4.0.100/32", exclusions[0].ExcludedCIDR)
	if exclusions[0].Reason != nil {
		assert.Equal(t, "Test exclusion", *exclusions[0].Reason)
	}

	// Test removing the exclusion
	err = service.RemoveExclusion(ctx, exclusion.ID)
	require.NoError(t, err)

	// Verify removal
	exclusions, err = service.getNetworkExclusions(ctx, &network.ID)
	require.NoError(t, err)
	assert.Len(t, exclusions, 0)
}

func TestNetworkService_GlobalExclusions_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Add a global exclusion
	exclusion, err := service.AddExclusion(ctx, nil, "192.168.99.99", "Global test exclusion")
	require.NoError(t, err)
	defer service.RemoveExclusion(ctx, exclusion.ID)

	// Get global exclusions
	exclusions, err := service.GetGlobalExclusions(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(exclusions), 1)

	// Verify our exclusion is in the list
	found := false
	for _, e := range exclusions {
		if e.ID == exclusion.ID {
			found = true
			assert.Equal(t, "192.168.99.99/32", e.ExcludedCIDR)
			if e.Reason != nil {
				assert.Equal(t, "Global test exclusion", *e.Reason)
			}
			break
		}
	}
	assert.True(t, found)
}

func TestNetworkService_UpdateNetworkDiscoveryTime_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Create a test network
	network, err := service.CreateNetwork(ctx, "Discovery Time Test", "10.5.0.0/24", "Test", "ping", true)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, network.ID)

	// Update discovery time
	err = service.UpdateNetworkDiscoveryTime(ctx, network.ID, 10, 8)
	require.NoError(t, err)

	// Verify the time was updated
	updated, err := service.GetNetworkByID(ctx, network.ID)
	require.NoError(t, err)
	assert.NotNil(t, updated.Network.LastDiscovery)
}

func TestNetworkService_GetNetworkStats_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Get network stats
	stats, err := service.GetNetworkStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	// Stats is a map, check keys exist
	assert.Contains(t, stats, "total_networks")
	assert.Contains(t, stats, "active_networks")
	assert.Contains(t, stats, "total_hosts")
}

func TestNetworkService_GenerateTargetsForNetwork_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	// Create a test network with a small CIDR
	network, err := service.CreateNetwork(ctx, "Target Gen Test", "10.6.0.0/30", "Test", "ping", true)
	require.NoError(t, err)
	defer service.DeleteNetwork(ctx, network.ID)

	// Generate targets (limit to 10)
	targets, err := service.GenerateTargetsForNetwork(ctx, network.ID, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, targets)
	// A /30 network has 4 IPs, but typically 2 are usable (excluding network and broadcast)
	assert.LessOrEqual(t, len(targets), 4)
}

func TestNetworkService_GetNetworkByName_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	_, err := service.GetNetworkByName(ctx, "non-existent-network")
	assert.Error(t, err)
}

func TestNetworkService_GetNetworkByID_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	_, err := service.GetNetworkByID(ctx, uuid.New())
	assert.Error(t, err)
}

func TestNetworkService_DeleteNetwork_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	err := service.DeleteNetwork(ctx, uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network not found")
}

func TestNetworkService_RemoveExclusion_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	err := service.RemoveExclusion(ctx, uuid.New())
	assert.Error(t, err)
}

func TestNetworkService_UpdateNetwork_NotFound_Integration(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	service := NewNetworkService(database)
	ctx := context.Background()

	_, err := service.UpdateNetwork(ctx, uuid.New(), "Test", "10.0.0.0/24", "Test", "ping", true)
	assert.Error(t, err)
}
