package test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

const (
	testLocalhostIP = "127.0.0.1"
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

	// Check if nmap is available for discovery tests
	if _, err := exec.LookPath("nmap"); err != nil {
		t.Skip("nmap not available - skipping discovery tests")
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

// cleanupTestData removes test data to ensure test isolation.
func (suite *TestSuite) cleanupTestData(t *testing.T) {
	// Clean up in reverse dependency order
	queries := []string{
		"DELETE FROM port_scans WHERE job_id IN " +
			"(SELECT id FROM scan_jobs WHERE created_at > NOW() - INTERVAL '1 hour')",
		"DELETE FROM scan_jobs WHERE created_at > NOW() - INTERVAL '1 hour'",
		"DELETE FROM discovery_jobs WHERE created_at > NOW() - INTERVAL '1 hour'",
		"DELETE FROM hosts WHERE ip_address = '" + testLocalhostIP + "' AND first_seen > NOW() - INTERVAL '1 hour'",
		"DELETE FROM scan_targets WHERE created_at > NOW() - INTERVAL '1 hour'",
	}

	for _, query := range queries {
		_, err := suite.database.ExecContext(suite.ctx, query)
		if err != nil {
			t.Logf("Warning: cleanup query failed (may be expected): %v", err)
		}
	}
}

// TestBasicScanFunctionality tests that scanning works and stores results correctly.
func TestBasicScanFunctionality(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Test scanning localhost with a common port
	scanConfig := &internal.ScanConfig{
		Targets:     []string{testLocalhostIP},
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
	query := `
		SELECT COUNT(*)
		FROM hosts
		WHERE ip_address = $2
		  AND last_seen >= $1`
	err = suite.database.QueryRowContext(suite.ctx, query, testStartTime, testLocalhostIP).Scan(&hostCount)
	require.NoError(t, err, "Should be able to query hosts table")
	assert.GreaterOrEqual(t, hostCount, 1, "Should have stored at least one host record")

	// Verify scan job was created
	var jobCount int
	jobQuery := `
		SELECT COUNT(*)
		FROM scan_jobs
		WHERE created_at >= $1
		  AND status = 'completed'`
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

// TestDiscoveryFunctionality tests the discovery system without relying on network conditions.
func TestDiscoveryFunctionality(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up any existing test data to ensure isolation
	suite.cleanupTestData(t)

	// Test discovery job creation and database operations
	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     testLocalhostIP + "/32", // Single host for reliability
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     15 * time.Second,
		Concurrency: 1,
	}

	// Execute discovery
	job, err := discoveryEngine.Discover(suite.ctx, &discoveryConfig)
	require.NoError(t, err, "Discovery should start successfully")
	require.NotEqual(t, uuid.Nil, job.ID, "Discovery job should have valid ID")

	// Wait for discovery to complete
	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 20*time.Second)
	require.NoError(t, err, "Discovery should complete within timeout")

	// Verify discovery job was stored and completed
	var jobStatus string
	var hostsDiscovered int
	jobQuery := `
		SELECT status, hosts_discovered
		FROM discovery_jobs
		WHERE id = $1`
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, job.ID).Scan(&jobStatus, &hostsDiscovered)
	require.NoError(t, err, "Should be able to query discovery job")
	assert.Equal(t, "completed", jobStatus, "Discovery job should be completed")

	t.Logf("Discovery completed: status=%s, hosts_discovered=%d", jobStatus, hostsDiscovered)

	// Don't assert on host discovery count as it's environment-dependent
	// The important thing is that the discovery system works and stores jobs correctly
}

// TestScanDiscoveredHost tests scanning functionality with a known host record.
func TestScanDiscoveredHost(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Clean up any existing test data to ensure isolation
	suite.cleanupTestData(t)

	// Use localhost since it's the only IP with CI services running
	testIP := testLocalhostIP

	// Create a known host record directly for reliable testing
	hostID := uuid.New()
	insertHostQuery := `
		INSERT INTO hosts (id, ip_address, status, discovery_method, last_seen)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := suite.database.ExecContext(suite.ctx, insertHostQuery,
		hostID, testIP, "up", "tcp", time.Now())
	require.NoError(t, err, "Should be able to create test host record")

	// Now scan the host
	scanConfig := &internal.ScanConfig{
		Targets:     []string{testIP},
		Ports:       "8080,8022,8379", // Use CI service ports
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err, "Scan should succeed")
	require.NotEmpty(t, result.Hosts, "Scan should return hosts")

	// Verify scan data was stored
	var scanCount int
	scanQuery := `
		SELECT COUNT(*) FROM port_scans ps
		JOIN hosts h ON ps.host_id = h.id
		WHERE h.ip_address = $2
		  AND ps.scanned_at >= $1`
	err = suite.database.QueryRowContext(suite.ctx, scanQuery, testStartTime, testIP).Scan(&scanCount)
	require.NoError(t, err, "Should query scan data")
	assert.GreaterOrEqual(t, scanCount, 1, "Should have stored scan results")

	t.Logf("Scan test completed: %d port scans stored", scanCount)
}

// TestDatabaseQueries tests that we can retrieve and query stored data correctly.
func TestDatabaseQueries(t *testing.T) {
	suite := setupTestSuite(t)
	testStartTime := time.Now()

	// Clean up any existing test data to ensure isolation
	suite.cleanupTestData(t)

	// First, ensure we have some data by running a scan with CI service ports
	testIP := testLocalhostIP // Use localhost since it has CI services running
	scanConfig := &internal.ScanConfig{
		Targets:     []string{testIP},
		Ports:       "8080,8022,8379", // Use CI service ports
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
		hostQuery := `
			SELECT id
			FROM hosts
			WHERE ip_address = $2
			  AND last_seen >= $1`
		err := suite.database.QueryRowContext(suite.ctx, hostQuery, testStartTime, testIP).Scan(&hostID)
		require.NoError(t, err, "Should find test host")

		// Query port scans for this host
		portRepo := db.NewPortScanRepository(suite.database)
		portScans, err := portRepo.GetByHost(suite.ctx, hostID)
		require.NoError(t, err, "Should be able to get port scans by host")

		// Verify we have scan data for CI service ports
		var foundCIPort bool
		ciPorts := []int{8080, 8022, 8379}
		for _, ps := range portScans {
			for _, port := range ciPorts {
				if ps.Port != port {
					continue
				}
				foundCIPort = true
				assert.Equal(t, "tcp", ps.Protocol, "Port should be TCP")
				assert.Equal(t, hostID, ps.HostID, "Port scan should belong to correct host")
				t.Logf("Found scan result for CI service port %d", port)
				break
			}
			if foundCIPort {
				break
			}
		}
		assert.True(t, foundCIPort, "Should find scan result for at least one CI service port (8080, 8022, 8379)")
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

	// Clean up any existing test data to ensure isolation
	suite.cleanupTestData(t)

	scanTypes := []string{"connect", "version"}

	for _, scanType := range scanTypes {
		t.Run(fmt.Sprintf("ScanType_%s", scanType), func(t *testing.T) {
			// Use localhost since it's the only IP with CI services running
			testIP := testLocalhostIP
			scanConfig := &internal.ScanConfig{
				Targets:     []string{testIP},
				Ports:       "8080,8022,8379", // Use CI service ports
				ScanType:    scanType,
				TimeoutSec:  15,
				Concurrency: 1,
			}

			result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
			require.NoError(t, err, "Scan with type %s should succeed", scanType)
			require.NotEmpty(t, result.Hosts, "Should return host results")

			// Verify this specific scan created port scan records for any of the CI service ports
			var portScanCount int
			query := `
				SELECT COUNT(*) FROM port_scans ps
				JOIN scan_jobs sj ON ps.job_id = sj.id
				JOIN hosts h ON ps.host_id = h.id
				WHERE h.ip_address = $2
				  AND ps.port IN (8080, 8022, 8379)
				  AND sj.created_at >= $1`
			err = suite.database.QueryRowContext(suite.ctx, query, testStartTime, testIP).Scan(&portScanCount)
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

// TestNmapAvailability tests that nmap is available and functional in the test environment.
func TestNmapAvailability(t *testing.T) {
	// Check if nmap binary exists
	nmapPath, err := exec.LookPath("nmap")
	if err != nil {
		t.Fatalf("nmap not found in PATH: %v", err)
	}
	t.Logf("Found nmap at: %s", nmapPath)

	// Test nmap version command
	cmd := exec.Command("nmap", "--version")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to run nmap --version: %v", err)
	}

	outputStr := string(output)
	t.Logf("nmap version output: %s", outputStr)

	// Verify it contains expected version info
	if !strings.Contains(outputStr, "Nmap") {
		t.Fatalf("nmap version output doesn't contain 'Nmap': %s", outputStr)
	}

	// Test basic nmap functionality with a simple host list scan
	cmd = exec.Command("nmap", "-sL", "127.0.0.1")
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("Failed to run nmap -sL: %v", err)
	}

	outputStr = string(output)
	t.Logf("nmap list scan output: %s", outputStr)

	// Verify localhost appears in the output
	if !strings.Contains(outputStr, "127.0.0.1") {
		t.Fatalf("nmap list scan output doesn't contain '127.0.0.1': %s", outputStr)
	}

	t.Log("nmap availability test passed")
}

// TestNetworkRangeDiscovery tests discovery job creation for network ranges.
func TestNetworkRangeDiscovery(t *testing.T) {
	suite := setupTestSuite(t)

	// Clean up any existing test data to ensure isolation
	suite.cleanupTestData(t)

	// Test discovery job creation for a small network range
	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.0/30", // Small range for testing
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     15 * time.Second,
		Concurrency: 2,
		MaxHosts:    10,
	}

	job, err := discoveryEngine.Discover(suite.ctx, &discoveryConfig)
	require.NoError(t, err, "Network range discovery should start successfully")
	require.NotEqual(t, uuid.Nil, job.ID, "Discovery job should have valid ID")

	// Wait for discovery to complete
	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 20*time.Second)
	require.NoError(t, err, "Network range discovery should complete within timeout")

	// Verify discovery job was stored and completed
	var jobStatus string
	var hostsDiscovered int
	jobQuery := `
		SELECT status, hosts_discovered
		FROM discovery_jobs
		WHERE id = $1`
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, job.ID).Scan(&jobStatus, &hostsDiscovered)
	require.NoError(t, err, "Should be able to query discovery job")
	assert.Equal(t, "completed", jobStatus, "Discovery job should be completed")

	t.Logf("Network range discovery completed: status=%s, hosts_discovered=%d", jobStatus, hostsDiscovered)
}

// TestDatabaseConnectionFailure tests that tests fail properly when database is unavailable.
func TestDatabaseConnectionFailure(t *testing.T) {
	// This test is mainly for documentation - the actual failure testing
	// is done by running tests with invalid database config
	t.Skip("This test documents that connection failures cause test failures, not skips")
}

// Helper functions
