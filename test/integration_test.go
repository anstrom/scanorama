package test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/test/helpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuite holds common test infrastructure.
type TestSuite struct {
	database *db.DB
	ctx      context.Context
	cancel   context.CancelFunc
}

// setupTestSuite creates a new test suite with database connection.
func setupTestSuite(t *testing.T) *TestSuite {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Connect to test database - this will fail the test if unavailable
	database, config, err := helpers.ConnectToTestDatabase(ctx)
	require.NoError(t, err, "Failed to connect to test database")

	t.Logf("Connected to database: %s@%s:%d/%s", config.Username, config.Host, config.Port, config.Database)

	// Ensure schema is available
	err = helpers.EnsureTestSchema(ctx, database)
	require.NoError(t, err, "Database schema is not available")

	return &TestSuite{
		database: database,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// TestBasicScanFunctionality tests that scanning works and stores results correctly.
func TestBasicScanFunctionality(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Test scanning localhost with a common port
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "22",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	// Execute the scan
	result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err, "Scan execution should succeed")
	require.NotNil(t, result, "Scan result should not be nil")
	require.NotEmpty(t, result.Hosts, "Scan should return at least one host")

	// Verify the scan result structure
	host := result.Hosts[0]
	assert.Equal(t, "up", host.Status, "Host should be detected as up")
	assert.Equal(t, "127.0.0.1", host.Address, "Host IP should match target")

	// Verify data was stored in database
	var hostCount int
	query := `SELECT COUNT(*) FROM hosts WHERE ip_address = '127.0.0.1' AND last_seen >= $1`
	err = suite.database.QueryRowContext(suite.ctx, query, testStartTime).Scan(&hostCount)
	require.NoError(t, err, "Should be able to query hosts table")
	assert.GreaterOrEqual(t, hostCount, 1, "Should have stored at least one host record")

	// Verify scan job was created
	var jobCount int
	jobQuery := `SELECT COUNT(*) FROM scan_jobs WHERE created_at >= $1 AND status = 'completed'`
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, testStartTime).Scan(&jobCount)
	require.NoError(t, err, "Should be able to query scan_jobs table")
	assert.GreaterOrEqual(t, jobCount, 1, "Should have created at least one scan job")

	// Verify port scan data was stored
	var portScanCount int
	portQuery := `
		SELECT COUNT(*) FROM port_scans ps
		JOIN hosts h ON ps.host_id = h.id
		WHERE h.ip_address = '127.0.0.1' AND ps.port = 22 AND ps.scanned_at >= $1`
	err = suite.database.QueryRowContext(suite.ctx, portQuery, testStartTime).Scan(&portScanCount)
	require.NoError(t, err, "Should be able to query port_scans table")
	assert.GreaterOrEqual(t, portScanCount, 1, "Should have stored port scan results")

	t.Logf("Test completed successfully: found %d hosts, %d jobs, %d port scans",
		hostCount, jobCount, portScanCount)
}

// TestDiscoveryFunctionality tests that host discovery works and stores results.
func TestDiscoveryFunctionality(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Create discovery engine and configure for localhost
	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     10 * time.Second,
		Concurrency: 1,
	}

	// Execute discovery
	job, err := discoveryEngine.Discover(suite.ctx, discoveryConfig)
	require.NoError(t, err, "Discovery should start successfully")
	require.NotEqual(t, uuid.Nil, job.ID, "Discovery job should have valid ID")

	// Wait for discovery to complete
	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 15*time.Second)
	require.NoError(t, err, "Discovery should complete within timeout")

	// Verify discovery job was stored and completed
	var jobStatus string
	var hostsDiscovered int
	jobQuery := `SELECT status, hosts_discovered FROM discovery_jobs WHERE id = $1`
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, job.ID).Scan(&jobStatus, &hostsDiscovered)
	require.NoError(t, err, "Should be able to query discovery job")
	assert.Equal(t, "completed", jobStatus, "Discovery job should be completed")
	assert.Greater(t, hostsDiscovered, 0, "Should have discovered at least one host")

	// Verify discovered host was stored with correct method
	var discoveredHostCount int
	hostQuery := `
		SELECT COUNT(*) FROM hosts
		WHERE discovery_method = 'tcp' AND ip_address = '127.0.0.1' AND last_seen >= $1`
	err = suite.database.QueryRowContext(suite.ctx, hostQuery, testStartTime).Scan(&discoveredHostCount)
	require.NoError(t, err, "Should be able to query discovered hosts")
	assert.GreaterOrEqual(t, discoveredHostCount, 1, "Should have at least one discovered host")

	t.Logf("Discovery completed: job status=%s, hosts_discovered=%d, hosts_in_db=%d",
		jobStatus, hostsDiscovered, discoveredHostCount)
}

// TestScanDiscoveredHost tests scanning a host that was previously discovered.
func TestScanDiscoveredHost(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Step 1: First discover localhost
	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     10 * time.Second,
		Concurrency: 1,
	}

	job, err := discoveryEngine.Discover(suite.ctx, discoveryConfig)
	require.NoError(t, err, "Discovery should succeed")

	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 15*time.Second)
	require.NoError(t, err, "Discovery should complete")

	// Verify discovery worked
	var discoveredCount int
	discoveryQuery := `
		SELECT COUNT(*) FROM hosts
		WHERE discovery_method = 'tcp' AND ip_address = '127.0.0.1' AND last_seen >= $1`
	err = suite.database.QueryRowContext(suite.ctx, discoveryQuery, testStartTime).Scan(&discoveredCount)
	require.NoError(t, err, "Should query discovered hosts")
	require.GreaterOrEqual(t, discoveredCount, 1, "Should have discovered localhost")

	// Step 2: Now scan the discovered host
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "22",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err, "Scan should succeed")
	require.NotEmpty(t, result.Hosts, "Scan should return hosts")

	// Step 3: Verify the host record now has both discovery and scan data
	var hostWithBothCount int
	combinedQuery := `
		SELECT COUNT(DISTINCT h.id) FROM hosts h
		JOIN port_scans ps ON h.id = ps.host_id
		WHERE h.ip_address = '127.0.0.1'
		  AND h.discovery_method = 'tcp'
		  AND h.last_seen >= $1
		  AND ps.scanned_at >= $1`
	err = suite.database.QueryRowContext(suite.ctx, combinedQuery, testStartTime).Scan(&hostWithBothCount)
	require.NoError(t, err, "Should query combined data")
	assert.GreaterOrEqual(t, hostWithBothCount, 1, "Should have host with both discovery and scan data")

	t.Logf("Integration test completed: discovered=%d hosts, combined_data=%d hosts",
		discoveredCount, hostWithBothCount)
}

// TestDatabaseQueries tests that we can retrieve and query stored data correctly.
func TestDatabaseQueries(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// First, ensure we have some data by running a scan
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "22",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	_, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err, "Setup scan should succeed")

	// Test querying active hosts
	t.Run("QueryActiveHosts", func(t *testing.T) {
		hostRepo := db.NewHostRepository(suite.database)
		hosts, err := hostRepo.GetActiveHosts(suite.ctx)
		require.NoError(t, err, "Should be able to get active hosts")

		// Find our test host
		var foundTestHost bool
		for _, host := range hosts {
			if host.IPAddress.String() == "127.0.0.1" {
				foundTestHost = true
				assert.Equal(t, "up", host.Status, "Test host should be up")
				break
			}
		}
		assert.True(t, foundTestHost, "Should find our test host in active hosts")
	})

	// Test querying port scans by host
	t.Run("QueryPortScansByHost", func(t *testing.T) {
		// Get the host ID for localhost
		var hostID uuid.UUID
		hostQuery := `SELECT id FROM hosts WHERE ip_address = '127.0.0.1' AND last_seen >= $1`
		err := suite.database.QueryRowContext(suite.ctx, hostQuery, testStartTime).Scan(&hostID)
		require.NoError(t, err, "Should find localhost host")

		// Query port scans for this host
		portRepo := db.NewPortScanRepository(suite.database)
		portScans, err := portRepo.GetByHost(suite.ctx, hostID)
		require.NoError(t, err, "Should be able to get port scans by host")

		// Verify we have scan data
		var foundPort22 bool
		for _, ps := range portScans {
			if ps.Port == 22 {
				foundPort22 = true
				assert.Equal(t, "tcp", ps.Protocol, "Port 22 should be TCP")
				assert.Equal(t, hostID, ps.HostID, "Port scan should belong to correct host")
				break
			}
		}
		assert.True(t, foundPort22, "Should find port 22 scan result")
	})

	// Test querying scan jobs with results
	t.Run("QueryScanJobsWithResults", func(t *testing.T) {
		var jobResults []struct {
			JobID        uuid.UUID  `db:"job_id"`
			TargetName   string     `db:"target_name"`
			Status       string     `db:"status"`
			HostsScanned int        `db:"hosts_scanned"`
			CompletedAt  *time.Time `db:"completed_at"`
		}

		query := `
			SELECT
				sj.id as job_id,
				st.name as target_name,
				sj.status,
				COUNT(DISTINCT ps.host_id) as hosts_scanned,
				sj.completed_at
			FROM scan_jobs sj
			JOIN scan_targets st ON sj.target_id = st.id
			LEFT JOIN port_scans ps ON sj.id = ps.job_id
			WHERE sj.created_at >= $1
			GROUP BY sj.id, st.name, sj.status, sj.completed_at
			ORDER BY sj.created_at DESC`

		err := suite.database.SelectContext(suite.ctx, &jobResults, query, testStartTime)
		require.NoError(t, err, "Should be able to query scan jobs")
		assert.NotEmpty(t, jobResults, "Should find scan job results from our test")

		// Verify job data
		job := jobResults[0]
		assert.Equal(t, "completed", job.Status, "Scan job should be completed")
		assert.GreaterOrEqual(t, job.HostsScanned, 1, "Should have scanned at least one host")
		assert.NotNil(t, job.CompletedAt, "Completed job should have completion time")
	})
}

// TestMultipleScanTypes tests different scan types work correctly.
func TestMultipleScanTypes(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	scanTypes := []string{"connect", "version"}

	for _, scanType := range scanTypes {
		t.Run(fmt.Sprintf("ScanType_%s", scanType), func(t *testing.T) {
			scanConfig := &internal.ScanConfig{
				Targets:     []string{"127.0.0.1"},
				Ports:       "22",
				ScanType:    scanType,
				TimeoutSec:  15,
				Concurrency: 1,
			}

			result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
			require.NoError(t, err, "Scan with type %s should succeed", scanType)
			require.NotEmpty(t, result.Hosts, "Should return host results")

			// Verify this specific scan created port scan records
			var portScanCount int
			query := `
				SELECT COUNT(*) FROM port_scans ps
				JOIN scan_jobs sj ON ps.job_id = sj.id
				JOIN hosts h ON ps.host_id = h.id
				WHERE h.ip_address = '127.0.0.1'
				  AND ps.port = 22
				  AND sj.created_at >= $1`
			err = suite.database.QueryRowContext(suite.ctx, query, testStartTime).Scan(&portScanCount)
			require.NoError(t, err, "Should query port scans for scan type %s", scanType)
			assert.GreaterOrEqual(t, portScanCount, 1, "Should have port scan results for %s", scanType)
		})
	}
}

// Helper functions for environment variables.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// TestDatabaseConnectionFailure tests that tests fail properly when database is unavailable.
func TestDatabaseConnectionFailure(t *testing.T) {
	// This test is mainly for documentation - the actual failure testing
	// is done by running tests with invalid database config
	t.Skip("This test documents that connection failures cause test failures, not skips")
}

// Helper functions
