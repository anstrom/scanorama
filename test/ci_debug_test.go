package test

import (
	"context"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCIMinimalScanStorage tests just the scanning storage functionality
// without any discovery dependencies to isolate CI issues
func TestCIMinimalScanStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	t.Log("=== CI DEBUG: Starting minimal scan storage test ===")

	// Test 1: Verify database is working
	t.Run("VerifyDatabaseConnection", func(t *testing.T) {
		var result int
		err := suite.database.QueryRowContext(suite.ctx, "SELECT 1").Scan(&result)
		require.NoError(t, err)
		assert.Equal(t, 1, result)
		t.Log("✅ Database connection verified")
	})

	// Test 2: Minimal scan without discovery
	t.Run("MinimalScanOnly", func(t *testing.T) {
		t.Log("CI DEBUG: Starting minimal scan test...")

		// Simple scan configuration
		scanConfig := internal.ScanConfig{
			Targets:    []string{"127.0.0.1"},
			Ports:      "22",
			ScanType:   "connect",
			TimeoutSec: 5,
		}

		t.Logf("CI DEBUG: Scan config - targets: %v, ports: %s, type: %s",
			scanConfig.Targets, scanConfig.Ports, scanConfig.ScanType)

		// Check database state before scan
		var hostsBeforeScan int
		err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&hostsBeforeScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Hosts in database before scan: %d", hostsBeforeScan)

		var scanJobsBeforeScan int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&scanJobsBeforeScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Scan jobs in database before scan: %d", scanJobsBeforeScan)

		var portScansBeforeScan int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM port_scans").Scan(&portScansBeforeScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Port scans in database before scan: %d", portScansBeforeScan)

		// Run the scan
		t.Log("CI DEBUG: Running scan...")
		result, err := internal.RunScanWithContext(suite.ctx, &scanConfig, suite.database)
		require.NoError(t, err)
		require.NotNil(t, result)
		t.Logf("CI DEBUG: Scan completed, found %d hosts", len(result.Hosts))

		// Check database state after scan
		time.Sleep(100 * time.Millisecond) // Give time for transactions to complete

		var hostsAfterScan int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&hostsAfterScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Hosts in database after scan: %d (was %d)", hostsAfterScan, hostsBeforeScan)

		var scanJobsAfterScan int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&scanJobsAfterScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Scan jobs in database after scan: %d (was %d)", scanJobsAfterScan, scanJobsBeforeScan)

		var portScansAfterScan int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM port_scans").Scan(&portScansAfterScan)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Port scans in database after scan: %d (was %d)", portScansAfterScan, portScansBeforeScan)

		// Verify scan jobs were created
		if scanJobsAfterScan <= scanJobsBeforeScan {
			t.Errorf("❌ CI ISSUE: No scan jobs created - before: %d, after: %d",
				scanJobsBeforeScan, scanJobsAfterScan)

			// Debug: Check if scan jobs exist with any status
			var allScanJobs int
			err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&allScanJobs)
			if err == nil {
				t.Logf("CI DEBUG: Total scan jobs in database: %d", allScanJobs)
			}

			// Debug: Check scan_jobs table structure and recent entries
			rows, err := suite.database.QueryContext(suite.ctx,
				"SELECT id, status, started_at, completed_at, error_message FROM scan_jobs ORDER BY created_at DESC LIMIT 3")
			if err == nil {
				defer rows.Close()
				t.Log("CI DEBUG: Recent scan jobs:")
				for i := 0; rows.Next() && i < 3; i++ {
					var id, status string
					var startedAt, completedAt interface{}
					var errorMsg interface{}
					if err := rows.Scan(&id, &status, &startedAt, &completedAt, &errorMsg); err == nil {
						t.Logf("  Job %d: %s (status: %s, started: %v, completed: %v, error: %v)",
							i+1, id, status, startedAt, completedAt, errorMsg)
					}
				}
			}
		} else {
			t.Logf("✅ Scan jobs created successfully: %d new jobs", scanJobsAfterScan-scanJobsBeforeScan)
		}

		// Verify port scans were created
		if portScansAfterScan <= portScansBeforeScan {
			t.Errorf("❌ CI ISSUE: No port scans created - before: %d, after: %d",
				portScansBeforeScan, portScansAfterScan)

			// Debug: Check port_scans table structure and entries
			rows, err := suite.database.QueryContext(suite.ctx,
				"SELECT job_id, host_id, port, protocol, state, service_name FROM port_scans ORDER BY scanned_at DESC LIMIT 3")
			if err == nil {
				defer rows.Close()
				t.Log("CI DEBUG: Recent port scans:")
				for i := 0; rows.Next() && i < 3; i++ {
					var jobID, hostID string
					var port int
					var protocol, state string
					var serviceName interface{}
					if err := rows.Scan(&jobID, &hostID, &port, &protocol, &state, &serviceName); err == nil {
						t.Logf("  Scan %d: job=%s, host=%s, port=%d/%s, state=%s, service=%v",
							i+1, jobID, hostID, port, protocol, state, serviceName)
					}
				}
			}

			// Debug: Check if there are any scan jobs at all
			var totalScanJobs int
			err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&totalScanJobs)
			if err == nil {
				t.Logf("CI DEBUG: Total scan jobs in database: %d", totalScanJobs)
				if totalScanJobs == 0 {
					t.Log("❌ CI CRITICAL: No scan jobs found - scanning may not be working at all")
				}
			}
		} else {
			t.Logf("✅ Port scans created successfully: %d new scans", portScansAfterScan-portScansBeforeScan)
		}
	})

	// Test 3: Check what's actually in the database
	t.Run("DatabaseStateDebug", func(t *testing.T) {
		t.Log("CI DEBUG: Analyzing current database state...")

		// Check hosts table
		var hosts []struct {
			IPAddress       string  `db:"ip_address"`
			Status          string  `db:"status"`
			DiscoveryMethod *string `db:"discovery_method"`
		}
		err := suite.database.SelectContext(suite.ctx, &hosts,
			"SELECT ip_address, status, discovery_method FROM hosts ORDER BY last_seen DESC LIMIT 5")
		require.NoError(t, err)

		t.Logf("CI DEBUG: Found %d hosts:", len(hosts))
		for i, host := range hosts {
			method := "NULL"
			if host.DiscoveryMethod != nil {
				method = *host.DiscoveryMethod
			}
			t.Logf("  Host %d: %s (status: %s, discovery_method: %s)", i+1, host.IPAddress, host.Status, method)
		}

		// Check scan_jobs table
		var scanJobs []struct {
			ID     string `db:"id"`
			Status string `db:"status"`
		}
		err = suite.database.SelectContext(suite.ctx, &scanJobs,
			"SELECT id, status FROM scan_jobs ORDER BY created_at DESC LIMIT 5")
		require.NoError(t, err)

		t.Logf("CI DEBUG: Found %d scan jobs:", len(scanJobs))
		for i, job := range scanJobs {
			t.Logf("  Job %d: %s (status: %s)", i+1, job.ID, job.Status)
		}

		// Check port_scans table
		var portScans []struct {
			Port     int    `db:"port"`
			Protocol string `db:"protocol"`
			State    string `db:"state"`
		}
		err = suite.database.SelectContext(suite.ctx, &portScans,
			"SELECT port, protocol, state FROM port_scans ORDER BY scanned_at DESC LIMIT 5")
		require.NoError(t, err)

		t.Logf("CI DEBUG: Found %d port scans:", len(portScans))
		for i, scan := range portScans {
			t.Logf("  Scan %d: %d/%s (state: %s)", i+1, scan.Port, scan.Protocol, scan.State)
		}

		// Additional debugging: Check table row counts
		tables := []string{"hosts", "scan_jobs", "port_scans", "scan_targets", "discovery_jobs"}
		for _, table := range tables {
			var count int
			err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
			if err == nil {
				t.Logf("CI DEBUG: Table %s has %d rows", table, count)
			} else {
				t.Logf("CI DEBUG: Error counting %s table: %v", table, err)
			}
		}

		// Check if scan jobs have any associated port scans
		var jobsWithScans int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(DISTINCT job_id) FROM port_scans").Scan(&jobsWithScans)
		if err == nil {
			t.Logf("CI DEBUG: Scan jobs with port scans: %d", jobsWithScans)
		}

		// Check for failed scan jobs
		var failedJobs int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_jobs WHERE status = 'failed'").Scan(&failedJobs)
		if err == nil {
			t.Logf("CI DEBUG: Failed scan jobs: %d", failedJobs)
			if failedJobs > 0 {
				// Get details of failed jobs
				var errorMsg interface{}
				err = suite.database.QueryRowContext(suite.ctx,
					"SELECT error_message FROM scan_jobs WHERE status = 'failed' AND error_message IS NOT NULL LIMIT 1").Scan(&errorMsg)
				if err == nil && errorMsg != nil {
					t.Logf("CI DEBUG: Sample error from failed job: %v", errorMsg)
				}
			}
		}
	})
}

// TestCIMinimalDiscoveryStorage tests just the discovery storage functionality
func TestCIMinimalDiscoveryStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	t.Log("=== CI DEBUG: Starting minimal discovery storage test ===")

	// Create discovery engine
	discoveryEngine := discovery.NewEngine(suite.database)

	// Test discovery with explicit method tracking
	t.Run("MinimalDiscoveryOnly", func(t *testing.T) {
		t.Log("CI DEBUG: Starting minimal discovery test with method=tcp...")

		// Simple discovery configuration
		discoveryConfig := discovery.Config{
			Network:     "127.0.0.1/32",
			Method:      "tcp",
			DetectOS:    false,
			Timeout:     10 * time.Second,
			Concurrency: 1,
		}

		// Check database state before discovery
		var hostsBefore int
		err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&hostsBefore)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Hosts before discovery: %d", hostsBefore)

		var discoveryJobsBefore int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM discovery_jobs").Scan(&discoveryJobsBefore)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Discovery jobs before: %d", discoveryJobsBefore)

		// Run discovery
		job, err := discoveryEngine.Discover(suite.ctx, discoveryConfig)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Discovery job created: %s", job.ID)

		// Wait for completion with timeout
		err = discoveryEngine.WaitForCompletion(suite.ctx, job.ID, 15*time.Second)
		require.NoError(t, err)
		t.Log("CI DEBUG: Discovery completed successfully")

		// Check database state after discovery
		time.Sleep(200 * time.Millisecond) // Allow transactions to complete

		var hostsAfter int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&hostsAfter)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Hosts after discovery: %d (was %d)", hostsAfter, hostsBefore)

		var discoveryJobsAfter int
		err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM discovery_jobs").Scan(&discoveryJobsAfter)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Discovery jobs after: %d (was %d)", discoveryJobsAfter, discoveryJobsBefore)

		// Specifically check for hosts with method=tcp
		var hostsWithTcp int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM hosts WHERE discovery_method = 'tcp'").Scan(&hostsWithTcp)
		require.NoError(t, err)
		t.Logf("CI DEBUG: Hosts with discovery_method=tcp: %d", hostsWithTcp)

		// This should work since discovery is working in CI
		assert.GreaterOrEqual(t, hostsWithTcp, 1, "Should have at least 1 host with discovery_method=tcp")
	})
}

// TestCIServiceContainerConnectivity tests if the issue is with service container connectivity
func TestCIServiceContainerConnectivity(t *testing.T) {
	t.Log("=== CI DEBUG: Testing service container connectivity ===")

	// Test if we can reach the service containers from CI
	serviceTests := []struct {
		name string
		host string
		port string
	}{
		{"nginx", "127.0.0.1", "8080"},
		{"redis", "127.0.0.1", "8379"},
		{"ssh", "127.0.0.1", "8022"},
	}

	for _, test := range serviceTests {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("CI DEBUG: Testing connectivity to %s on %s:%s", test.name, test.host, test.port)

			// Use simple nmap connectivity test
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Try basic connectivity check first
			config := internal.ScanConfig{
				Targets:    []string{test.host},
				Ports:      test.port,
				ScanType:   "connect",
				TimeoutSec: 5,
			}

			result, err := internal.RunScanWithContext(ctx, &config, nil) // No database storage
			if err != nil {
				t.Logf("❌ CI ISSUE: Failed to scan %s: %v", test.name, err)
				return
			}

			if result == nil || len(result.Hosts) == 0 {
				t.Logf("❌ CI ISSUE: No hosts found for %s", test.name)
				return
			}

			host := result.Hosts[0]
			t.Logf("✅ %s connectivity: host status = %s", test.name, host.Status)

			for _, port := range host.Ports {
				t.Logf("  Port %d/%s: %s", port.Number, port.Protocol, port.State)
			}
		})
	}
}
