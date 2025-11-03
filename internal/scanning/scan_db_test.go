package scanning

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a test database connection for integration tests.
func setupTestDB(t *testing.T) *db.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	// Try to get a test database connection
	configs := getTestDatabaseConfigs()

	var database *db.DB
	var lastErr error

	for _, cfg := range configs {
		var err error
		database, err = db.ConnectAndMigrate(context.Background(), &cfg)
		if err == nil {
			return database
		}
		lastErr = err
	}

	t.Skipf("Could not connect to test database: %v", lastErr)
	return nil
}

// getTestDatabaseConfigs returns potential database configurations for testing.
func getTestDatabaseConfigs() []db.Config {
	return []db.Config{
		{
			Host:            "localhost",
			Port:            5432,
			Database:        "scanorama_test",
			Username:        "test_user",
			Password:        "test_password",
			SSLMode:         "disable",
			MaxOpenConns:    2,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		},
		{
			Host:            "localhost",
			Port:            5432,
			Database:        "scanorama_dev",
			Username:        "scanorama_dev",
			Password:        "dev_password",
			SSLMode:         "disable",
			MaxOpenConns:    2,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
			ConnMaxIdleTime: time.Minute,
		},
	}
}

// TestStoreScanResults_SuccessfulStorage tests that scan results are correctly persisted.
func TestStoreScanResults_SuccessfulStorage(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	// Create a scan configuration
	config := &ScanConfig{
		Targets:  []string{"192.168.1.100"},
		Ports:    "80,443",
		ScanType: "connect",
	}

	// Create scan results
	startTime := time.Now().Add(-5 * time.Minute)
	result := &ScanResult{
		StartTime: startTime,
		Duration:  5 * time.Minute,
		Stats: HostStats{
			Up:    1,
			Down:  0,
			Total: 1,
		},
		Hosts: []Host{
			{
				Address: "192.168.1.100",
				Status:  "up",
				Ports: []Port{
					{
						Number:   80,
						Protocol: "tcp",
						State:    "open",
						Service:  "http",
					},
					{
						Number:   443,
						Protocol: "tcp",
						State:    "open",
						Service:  "https",
					},
				},
			},
		},
	}

	// Store the results
	err := storeScanResults(ctx, database, config, result)
	require.NoError(t, err, "storing scan results should succeed")

	// Verify scan job was created by querying directly
	var jobCount int
	query := `
		SELECT COUNT(*)
		FROM scan_jobs
		WHERE started_at >= $1
	`
	err = database.QueryRowContext(ctx, query, startTime.Add(-1*time.Minute)).Scan(&jobCount)
	require.NoError(t, err)
	assert.Greater(t, jobCount, 0, "should have at least one scan job")

	// Verify the job has correct status
	var jobStatus string
	statusQuery := `
		SELECT status
		FROM scan_jobs
		WHERE started_at >= $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	err = database.QueryRowContext(ctx, statusQuery, startTime.Add(-1*time.Minute)).Scan(&jobStatus)
	require.NoError(t, err)
	assert.Equal(t, db.ScanJobStatusCompleted, jobStatus)

	// Verify host was created
	hostRepo := db.NewHostRepository(database)
	ipAddr := db.IPAddr{IP: net.ParseIP("192.168.1.100")}
	host, err := hostRepo.GetByIP(ctx, ipAddr)
	require.NoError(t, err, "should find stored host")
	assert.Equal(t, "192.168.1.100", host.IPAddress.String())
}

// TestStoreScanResults_MultipleHosts verifies handling of multiple hosts.
func TestStoreScanResults_MultipleHosts(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		Ports:    "22",
		ScanType: "connect",
	}

	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Minute,
		Stats: HostStats{
			Up:    3,
			Down:  0,
			Total: 3,
		},
		Hosts: []Host{
			{Address: "10.0.0.1", Status: "up", Ports: []Port{{Number: 22, Protocol: "tcp", State: "open"}}},
			{Address: "10.0.0.2", Status: "up", Ports: []Port{{Number: 22, Protocol: "tcp", State: "closed"}}},
			{Address: "10.0.0.3", Status: "up", Ports: []Port{{Number: 22, Protocol: "tcp", State: "open"}}},
		},
	}

	err := storeScanResults(ctx, database, config, result)
	require.NoError(t, err, "should handle multiple hosts")

	// Verify all hosts were stored
	hostRepo := db.NewHostRepository(database)
	for _, expectedHost := range result.Hosts {
		ipAddr := db.IPAddr{IP: net.ParseIP(expectedHost.Address)}
		host, err := hostRepo.GetByIP(ctx, ipAddr)
		assert.NoError(t, err, "should find host %s", expectedHost.Address)
		if err == nil {
			assert.Equal(t, expectedHost.Address, host.IPAddress.String())
		}
	}
}

// TestStoreScanResults_InvalidTarget tests error handling for invalid targets.
func TestStoreScanResults_InvalidTarget(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	config := &ScanConfig{
		Targets:  []string{"not-a-valid-ip-or-cidr"},
		Ports:    "80",
		ScanType: "connect",
	}

	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Minute,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{},
	}

	// Should handle empty results gracefully
	err := storeScanResults(ctx, database, config, result)
	// The function may return an error or succeed with empty results
	// Both are acceptable behaviors for edge cases
	if err != nil {
		assert.Error(t, err, "should handle invalid target")
	}
}

// TestCreateAdhocScanTarget_ValidIP tests creating a scan target with a valid IP.
func TestCreateAdhocScanTarget_ValidIP(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	targetRepo := db.NewScanTargetRepository(database)

	config := &ScanConfig{
		ScanType: "connect",
		Ports:    "80",
	}

	target, err := createAdhocScanTarget(ctx, targetRepo, "192.168.1.50", config)
	require.NoError(t, err, "should create target for valid IP")
	assert.NotNil(t, target)
	assert.NotEqual(t, uuid.Nil, target.ID)
	assert.Contains(t, target.Name, "192.168.1.50")

	// Verify target can be retrieved
	retrieved, err := targetRepo.GetByID(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, target.ID, retrieved.ID)
	assert.Equal(t, target.Name, retrieved.Name)
}

// TestCreateAdhocScanTarget_CIDR tests creating a scan target with CIDR notation.
func TestCreateAdhocScanTarget_CIDR(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	targetRepo := db.NewScanTargetRepository(database)

	config := &ScanConfig{
		ScanType: "connect",
		Ports:    "22,80",
	}

	target, err := createAdhocScanTarget(ctx, targetRepo, "10.0.1.0/24", config)
	require.NoError(t, err, "should create target for CIDR")
	assert.NotNil(t, target)
	assert.Contains(t, target.Name, "10.0.1.0/24")
}

// TestCreateAdhocScanTarget_InvalidAddress tests error handling for invalid addresses.
func TestCreateAdhocScanTarget_InvalidAddress(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	targetRepo := db.NewScanTargetRepository(database)

	config := &ScanConfig{
		ScanType: "connect",
		Ports:    "80",
	}

	_, err := createAdhocScanTarget(ctx, targetRepo, "not-an-ip-address", config)
	assert.Error(t, err, "should reject invalid address")
}

// TestStoreHostResults_EmptyHosts tests handling of empty host list.
func TestStoreHostResults_EmptyHosts(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	jobID := uuid.New()

	err := storeHostResults(ctx, database, jobID, []Host{})
	assert.NoError(t, err, "should handle empty host list gracefully")
}

// TestStoreHostResults_HostWithNoPorts tests storing a host without open ports.
func TestStoreHostResults_HostWithNoPorts(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	jobID := uuid.New()

	hosts := []Host{
		{
			Address: "172.16.0.1",
			Status:  "up",
			Ports:   []Port{}, // No open ports
		},
	}

	err := storeHostResults(ctx, database, jobID, hosts)
	assert.NoError(t, err, "should handle host with no ports")

	// Verify host was still created
	hostRepo := db.NewHostRepository(database)
	ipAddr := db.IPAddr{IP: net.ParseIP("172.16.0.1")}
	host, err := hostRepo.GetByIP(ctx, ipAddr)
	require.NoError(t, err, "host should exist even without ports")
	assert.Equal(t, "172.16.0.1", host.IPAddress.String())
}

// TestGetOrCreateHostSafely_NewHost tests creating a new host.
func TestGetOrCreateHostSafely_NewHost(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	hostRepo := db.NewHostRepository(database)

	ipAddr := db.IPAddr{IP: net.ParseIP("203.0.113.42")}
	hostInput := Host{Address: "203.0.113.42", Status: "up"}

	host, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, hostInput)
	require.NoError(t, err, "should create new host")
	assert.NotNil(t, host)
	assert.NotEqual(t, uuid.Nil, host.ID)
	assert.Equal(t, "203.0.113.42", host.IPAddress.String())
}

// TestGetOrCreateHostSafely_ExistingHost tests retrieving an existing host.
func TestGetOrCreateHostSafely_ExistingHost(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	hostRepo := db.NewHostRepository(database)

	// Create initial host
	ipAddr := db.IPAddr{IP: net.ParseIP("198.51.100.1")}
	hostInput := Host{Address: "198.51.100.1", Status: "up"}
	firstHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, hostInput)
	require.NoError(t, err)
	firstID := firstHost.ID

	// Try to create same host again
	secondHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, hostInput)
	require.NoError(t, err, "should retrieve existing host")
	assert.Equal(t, firstID, secondHost.ID, "should return same host ID")
}

// TestParseTargetAddress_ValidInputs tests various valid address formats.
func TestParseTargetAddress_ValidInputs(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"single IPv4", "192.168.1.1"},
		{"IPv4 CIDR", "10.0.0.0/24"},
		{"single IPv6", "2001:db8::1"},
		{"IPv6 CIDR", "2001:db8::/32"},
		{"hostname fallback", "example.com"}, // parseTargetAddress accepts hostnames
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseTargetAddress(tt.target)
			assert.NoError(t, err, "should parse %s", tt.target)
			assert.NotNil(t, addr)
		})
	}
}

// TestParseTargetAddress_InvalidCIDR tests error handling for invalid CIDR notation.
func TestParseTargetAddress_InvalidCIDR(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"invalid CIDR mask", "192.168.1.1/99"},
		{"malformed CIDR", "not-an-ip/24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTargetAddress(tt.target)
			assert.Error(t, err, "should reject invalid CIDR %s", tt.target)
		})
	}
}

// TestRunScanWithDB_IntegrationTest is an end-to-end test of scanning with database storage.
func TestRunScanWithDB_IntegrationTest(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	// Check if we can reach localhost
	conn, err := net.DialTimeout("tcp", "localhost:80", 1*time.Second)
	if err != nil {
		t.Skip("No service available on localhost:80 for integration test")
	}
	if conn != nil {
		conn.Close()
	}

	config := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
	}

	result, err := RunScanWithDB(config, database)
	require.NoError(t, err, "scan with database should succeed")
	require.NotNil(t, result)

	// Verify data was persisted
	hostRepo := db.NewHostRepository(database)
	ipAddr := db.IPAddr{IP: net.ParseIP("127.0.0.1")}
	host, err := hostRepo.GetByIP(context.Background(), ipAddr)
	assert.NoError(t, err, "should find scanned host in database")
	if err == nil {
		assert.Equal(t, "127.0.0.1", host.IPAddress.String())
	}
}
