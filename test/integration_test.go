package test

import (
	"context"
	"fmt"
	"os"
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
	statusCompleted         = "completed"
	nullValue               = "NULL"
	discoveryJobStatusQuery = "SELECT status, hosts_discovered FROM discovery_jobs WHERE id = $1"
	hostsWithDiscoveryQuery = "SELECT COUNT(*) FROM hosts WHERE discovery_method IS NOT NULL"
	hostsWithIPQuery        = "SELECT COUNT(*) FROM hosts WHERE ip_address = '127.0.0.1'"
)

// IntegrationTestSuite holds test environment and database connection.
// IntegrationTestSuite holds the test database and configuration.
type IntegrationTestSuite struct {
	database *db.DB
	ctx      context.Context
	cancel   context.CancelFunc
}

// setupIntegrationTestSuite sets up the test database and environment.
func setupIntegrationTestSuite(t *testing.T) *IntegrationTestSuite {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	// Set up test database
	database := setupTestDatabase(t)

	// Create cancellable context for proper cleanup
	ctx, cancel := context.WithCancel(context.Background())

	return &IntegrationTestSuite{
		database: database,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// setupTestDatabase creates a test database connection.
func setupTestDatabase(t *testing.T) *db.DB {
	ctx := context.Background()

	// Use the helper to connect to an available database
	database, config, err := helpers.ConnectToTestDatabase(ctx)
	require.NoError(t, err, "Failed to connect to any test database")

	t.Logf("Successfully connected to database: %s@%s:%d/%s",
		config.Username, config.Host, config.Port, config.Database)

	// Ensure schema is available
	err = helpers.EnsureTestSchema(ctx, database)
	require.NoError(t, err, "Database schema is not available")

	return database
}

// cleanupTestData removes any existing test data.
func cleanupTestData(t *testing.T, database *db.DB) {
	err := helpers.CleanupTestTables(context.Background(), database)
	if err != nil {
		t.Logf("Warning: Failed to cleanup test data: %v", err)
	}
}

// teardown cleans up test resources.
func (suite *IntegrationTestSuite) teardown(t *testing.T) {
	// Cancel context to stop background processes
	if suite.cancel != nil {
		suite.cancel()
	}

	// Give background processes time to finish
	time.Sleep(100 * time.Millisecond)

	// Clean up test data to prevent database pollution
	cleanupTestData(t, suite.database)

	if err := suite.database.Close(); err != nil {
		t.Logf("Warning: Failed to close database: %v", err)
	}
}

// TestScanWithDatabaseStorage tests scanning functionality with database storage.
func TestScanWithDatabaseStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	// Check initial database state
	var initialHosts, initialScanJobs, initialPortScans int
	err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&initialHosts)
	require.NoError(t, err)
	err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&initialScanJobs)
	require.NoError(t, err)
	err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM port_scans").Scan(&initialPortScans)
	require.NoError(t, err)

	t.Logf("Testing scan with port 22")
	// Use a real port that should be available for testing
	testPort := "22" // SSH port is commonly available
	t.Logf("Testing scan with port %s", testPort)

	// Create scan configuration with unique target
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	// Run scan with database storage
	result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Hosts)

	// Verify scan results
	host := result.Hosts[0]
	assert.Equal(t, "up", host.Status)
	assert.NotEmpty(t, host.Ports)

	// Verify results are stored in database
	t.Run("VerifyHostStored", func(t *testing.T) {
		// Query hosts from database
		var hosts []*db.Host
		query := `SELECT * FROM hosts WHERE status = 'up' ORDER BY last_seen DESC LIMIT 10`
		err := suite.database.SelectContext(suite.ctx, &hosts, query)
		require.NoError(t, err)
		require.NotEmpty(t, hosts, "No hosts found in database")

		// Verify host data
		dbHost := hosts[0]
		assert.Equal(t, "up", dbHost.Status)
		assert.NotNil(t, dbHost.IPAddress.IP)
	})

	t.Run("VerifyPortScansStored", func(t *testing.T) {
		// Query port scans from database
		var portScans []*db.PortScan
		query := `SELECT * FROM port_scans ORDER BY scanned_at DESC LIMIT 10`
		err := suite.database.SelectContext(suite.ctx, &portScans, query)
		require.NoError(t, err)
		require.NotEmpty(t, portScans, "No port scans found in database")

		// Verify port scan data
		portScan := portScans[0]
		assert.NotEqual(t, uuid.Nil, portScan.HostID)
		assert.NotEqual(t, uuid.Nil, portScan.JobID)
		assert.Greater(t, portScan.Port, 0)
		assert.Equal(t, "tcp", portScan.Protocol)
	})

	t.Run("VerifyScanJobStored", func(t *testing.T) {
		// Query scan jobs from database
		var scanJobs []*db.ScanJob
		query := `SELECT * FROM scan_jobs ORDER BY created_at DESC LIMIT 10`
		err := suite.database.SelectContext(suite.ctx, &scanJobs, query)
		require.NoError(t, err)

		if len(scanJobs) == 0 {
			t.Fatal("Expected at least one scan job in database")
		}

		require.NotEmpty(t, scanJobs, "No scan jobs found in database")

		// Verify scan job data
		scanJob := scanJobs[0]
		assert.Equal(t, "completed", scanJob.Status)
		assert.NotEqual(t, uuid.Nil, scanJob.TargetID)
	})
}

// TestDiscoveryWithDatabaseStorage tests discovery functionality with database storage.
func TestDiscoveryWithDatabaseStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	// Create discovery engine
	discoveryEngine := discovery.NewEngine(suite.database)

	// Run discovery on localhost for reliable testing
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     10 * time.Second,
		Concurrency: 1,
	}

	job, err := discoveryEngine.Discover(suite.ctx, discoveryConfig)
	require.NoError(t, err)

	// Wait for discovery to complete properly
	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 15*time.Second)
	require.NoError(t, err)
	maxWait := 30 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	for {
		if time.Since(startTime) > maxWait {
			// Get final status
			var finalStatus string
			var finalHosts int
			query := discoveryJobStatusQuery
			err := suite.database.QueryRowContext(suite.ctx, query, job.ID).Scan(&finalStatus, &finalHosts)
			if err == nil {
				t.Fatalf("Discovery timed out after %v - final status: %s, hosts: %d",
					maxWait, finalStatus, finalHosts)
			} else {
				t.Fatalf("Discovery timed out after %v - could not get final status: %v",
					maxWait, err)
			}
		}

		// Check job status
		var status string
		var hostsDiscovered int
		query := `SELECT status, hosts_discovered FROM discovery_jobs WHERE id = $1`
		err := suite.database.QueryRowContext(suite.ctx, query, job.ID).Scan(&status, &hostsDiscovered)
		require.NoError(t, err)

		if status == statusCompleted || status == "failed" {
			t.Logf("Discovery %s. Found %d hosts.", status, hostsDiscovered)
			assert.Equal(t, statusCompleted, status)

			// Additional verification that hosts were actually stored
			if status == statusCompleted && hostsDiscovered > 0 {
				var actualHostCount int
				hostQuery := `SELECT COUNT(*) FROM hosts WHERE discovery_method = 'tcp'`
				if err := suite.database.QueryRowContext(suite.ctx, hostQuery).Scan(&actualHostCount); err == nil {
					t.Logf("Verified %d hosts with discovery_method=tcp in database", actualHostCount)
				}
			}
			break
		}

		time.Sleep(checkInterval)
	}

	// Verify discovery results in database
	t.Run("VerifyDiscoveryJobStored", func(t *testing.T) {
		var discoveryJobs []*db.DiscoveryJob
		query := `
			SELECT * FROM discovery_jobs
			WHERE status = 'completed' AND network >>= '127.0.0.1/32'
			ORDER BY created_at DESC LIMIT 5`
		err := suite.database.SelectContext(suite.ctx, &discoveryJobs, query)
		require.NoError(t, err)
		require.NotEmpty(t, discoveryJobs, "No completed discovery jobs found")

		// Verify discovery job data
		discoveryJob := discoveryJobs[0]
		assert.Equal(t, "completed", discoveryJob.Status)
		assert.GreaterOrEqual(t, discoveryJob.HostsDiscovered, 1)
		assert.GreaterOrEqual(t, discoveryJob.HostsResponsive, 1)
	})

	t.Run("VerifyDiscoveredHostsStored", func(t *testing.T) {
		var hosts []*db.Host
		query := `
			SELECT * FROM hosts
			WHERE discovery_method = 'tcp' AND ip_address = '127.0.0.1'
			ORDER BY last_seen DESC LIMIT 5`
		err := suite.database.SelectContext(suite.ctx, &hosts, query)
		require.NoError(t, err)
		require.NotEmpty(t, hosts, "No discovered hosts found")

		// Verify host data
		host := hosts[0]
		assert.Equal(t, "up", host.Status)
		assert.NotNil(t, host.DiscoveryMethod)
		assert.Equal(t, "tcp", *host.DiscoveryMethod)
	})
}

// TestScanDiscoveredHosts tests scanning hosts that were previously discovered.
func TestScanDiscoveredHosts(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	// First, run discovery on localhost
	discoveryEngine := discovery.NewEngine(suite.database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     10 * time.Second,
		Concurrency: 1,
	}

	job, err := discoveryEngine.Discover(suite.ctx, discoveryConfig)
	require.NoError(t, err)

	// Wait for discovery to complete
	err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 30*time.Second)
	require.NoError(t, err)

	// Verify that discovery actually saved the host with correct discovery_method
	var discoveredHostCount int
	query := `SELECT COUNT(*) FROM hosts WHERE discovery_method = 'tcp' AND ip_address = '127.0.0.1'`
	err = suite.database.QueryRowContext(suite.ctx, query).Scan(&discoveredHostCount)
	require.NoError(t, err)

	require.Equal(t, 1, discoveredHostCount,
		"Discovery should have created exactly one host with discovery_method=tcp")
	t.Logf("Discovery verification successful: found %d host with discovery_method=tcp", discoveredHostCount)

	// Force transaction consistency
	for i := 0; i < 3; i++ {
		var commitCheck int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT 1").Scan(&commitCheck)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// 2. Verify discovery job completion and consistency
	var jobStatus string
	var jobHosts int
	jobQuery := discoveryJobStatusQuery
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, job.ID).Scan(&jobStatus, &jobHosts)
	require.NoError(t, err)
	require.Equal(t, "completed", jobStatus)
	require.Equal(t, 1, jobHosts)
	t.Logf("Discovery job verification: status=%s, hosts_discovered=%d", jobStatus, jobHosts)

	// Small delay to ensure transaction consistency
	time.Sleep(100 * time.Millisecond)

	// 4. Multi-attempt verification that the host persists with correct discovery_method
	const maxVerifyAttempts = 5
	var finalHostCount int
	for attempt := 1; attempt <= maxVerifyAttempts; attempt++ {
		err = suite.database.QueryRowContext(suite.ctx, query).Scan(&finalHostCount)
		require.NoError(t, err)

		if finalHostCount == 1 {
			t.Logf("Host verification successful on attempt %d: found %d host(s)", attempt, finalHostCount)
			break
		}

		if attempt < maxVerifyAttempts {
			t.Logf("Host verification attempt %d failed, retrying... (found %d hosts)", attempt, finalHostCount)
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 5. Final comprehensive database state verification
	var totalHosts, hostsWithDiscovery, hostsWithIP int
	err = suite.database.QueryRowContext(suite.ctx,
		"SELECT COUNT(*) FROM hosts").Scan(&totalHosts)
	require.NoError(t, err)
	err = suite.database.QueryRowContext(suite.ctx, hostsWithDiscoveryQuery).Scan(&hostsWithDiscovery)
	require.NoError(t, err)
	err = suite.database.QueryRowContext(suite.ctx, hostsWithIPQuery).Scan(&hostsWithIP)
	require.NoError(t, err)

	t.Logf("Final database state: total_hosts=%d, hosts_with_discovery=%d, hosts_with_ip_127.0.0.1=%d",
		totalHosts, hostsWithDiscovery, hostsWithIP)

	require.Equal(t, 1, hostsWithIP, "Should have exactly 1 host with IP 127.0.0.1")
	require.GreaterOrEqual(t, hostsWithDiscovery, 1,
		"Should have at least 1 host with discovery_method")

	// Now scan the discovered hosts
	testPort := "22" // SSH port for testing
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       testPort,
		ScanType:    "version",
		TimeoutSec:  15,
		Concurrency: 1,
	}

	// Add pre-scan database consistency check
	var preScanHostCount int
	preScanQuery := `SELECT COUNT(*) FROM hosts WHERE ip_address = '127.0.0.1' ` +
		`AND discovery_method = 'tcp'`
	err = suite.database.QueryRowContext(suite.ctx, preScanQuery).Scan(&preScanHostCount)
	require.NoError(t, err)
	require.Equal(t, 1, preScanHostCount,
		"Host must exist with discovery_method=tcp before scanning")
	t.Logf("Pre-scan verification: %d host(s) confirmed before scanning", preScanHostCount)

	result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Run("VerifyIntegratedData", func(t *testing.T) {
		// Check that we have hosts with both discovery and scan data
		var count int
		query := `
			SELECT COUNT(DISTINCT h.id)
			FROM hosts h
			INNER JOIN port_scans ps ON h.id = ps.host_id
			WHERE h.discovery_method IS NOT NULL
		`
		err := suite.database.QueryRowContext(suite.ctx, query).Scan(&count)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1, "Should have at least one host with both discovery and scan data")
	})
}

// TestQueryScanResults tests querying stored scan results.
func TestQueryScanResults(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	// Run a scan to populate database
	testPort := "22" // SSH port for testing
	scanConfig := &internal.ScanConfig{
		Targets:     []string{"localhost"},
		Ports:       testPort,
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	_, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
	require.NoError(t, err)

	// Test various database queries
	t.Run("QueryActiveHosts", func(t *testing.T) {
		hostRepo := db.NewHostRepository(suite.database)
		hosts, err := hostRepo.GetActiveHosts(suite.ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(hosts), 1, "Should have at least one active host")

		if len(hosts) > 0 {
			host := hosts[0]
			assert.Equal(t, "up", host.Status)
			assert.GreaterOrEqual(t, host.OpenPorts, 0)
		}
	})

	t.Run("QueryPortScansByHost", func(t *testing.T) {
		// Get a host ID first
		var hostID uuid.UUID
		query := `SELECT id FROM hosts WHERE status = 'up' LIMIT 1`
		err := suite.database.QueryRowContext(suite.ctx, query).Scan(&hostID)
		require.NoError(t, err)

		// Query port scans for this host
		portRepo := db.NewPortScanRepository(suite.database)
		portScans, err := portRepo.GetByHost(suite.ctx, hostID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(portScans), 1, "Should have at least one port scan")

		if len(portScans) > 0 {
			portScan := portScans[0]
			assert.Equal(t, hostID, portScan.HostID)
			assert.Greater(t, portScan.Port, 0)
		}
	})

	t.Run("QueryNetworkSummary", func(t *testing.T) {
		summaryRepo := db.NewNetworkSummaryRepository(suite.database)
		summaries, err := summaryRepo.GetAll(suite.ctx)
		require.NoError(t, err)

		// We might not have summaries if no scan targets exist, so just verify no error
		t.Logf("Found %d network summaries", len(summaries))
	})

	t.Run("QueryScanJobsWithResults", func(t *testing.T) {
		var jobResults []struct {
			JobID        uuid.UUID  `db:"job_id"`
			TargetName   string     `db:"target_name"`
			Status       string     `db:"status"`
			HostsScanned int        `db:"hosts_scanned"`
			OpenPorts    int        `db:"open_ports"`
			CompletedAt  *time.Time `db:"completed_at"`
		}

		query := `
			SELECT
				sj.id as job_id,
				st.name as target_name,
				sj.status,
				COUNT(DISTINCT ps.host_id) as hosts_scanned,
				COUNT(ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
				sj.completed_at
			FROM scan_jobs sj
			JOIN scan_targets st ON sj.target_id = st.id
			LEFT JOIN port_scans ps ON sj.id = ps.job_id
			WHERE sj.status = 'completed'
			GROUP BY sj.id, st.name, sj.status, sj.completed_at
			ORDER BY sj.created_at DESC
			LIMIT 5
		`

		err := suite.database.SelectContext(suite.ctx, &jobResults, query)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(jobResults), 1, "Should have at least one completed scan job")

		if len(jobResults) > 0 {
			result := jobResults[0]
			assert.Equal(t, "completed", result.Status)
			assert.GreaterOrEqual(t, result.HostsScanned, 1)
			t.Logf("Scan job %s scanned %d hosts with %d open ports",
				result.TargetName, result.HostsScanned, result.OpenPorts)
		}
	})
}

// TestMultipleScanTypes tests different scan types with database storage.
func TestMultipleScanTypes(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	scanTypes := []string{"connect", "version"}

	for _, scanType := range scanTypes {
		t.Run(fmt.Sprintf("ScanType_%s", scanType), func(t *testing.T) {
			scanConfig := &internal.ScanConfig{
				Targets:     []string{"127.0.0.1"},
				Ports:       "22", // Use single port for each scan type test
				ScanType:    scanType,
				TimeoutSec:  15,
				Concurrency: 1,
			}

			result, err := internal.RunScanWithContext(suite.ctx, scanConfig, suite.database)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Hosts)

			// Verify results in database by checking recent port scans
			var portScanCount int
			query := `
				SELECT COUNT(*) FROM port_scans ps
				JOIN scan_jobs sj ON ps.job_id = sj.id
				WHERE sj.created_at > NOW() - INTERVAL '1 minute'
			`
			err = suite.database.QueryRowContext(suite.ctx, query).Scan(&portScanCount)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, portScanCount, 1,
				"Should have port scan results for %s", scanType)
		})
	}
}

// Helper functions.

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if intVal, err := time.ParseDuration(val + "s"); err == nil {
			return int(intVal.Seconds())
		}
	}
	return defaultValue
}
