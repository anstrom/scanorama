package test

import (
	"context"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCIMinimalScanStorage tests just the scanning storage functionality
// without any discovery dependencies to isolate CI issues.
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
		runMinimalScanTest(t, suite)
	})

	// Test 3: Check what's actually in the database
	t.Run("DatabaseStateDebug", func(t *testing.T) {
		debugDatabaseState(t, suite)
	})
}

// runMinimalScanTest executes a minimal scan test and checks database storage.
func runMinimalScanTest(t *testing.T, suite *IntegrationTestSuite) {
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
	before := getDatabaseCounts(t, suite)

	// Run the scan
	t.Log("CI DEBUG: Running scan...")
	result, err := internal.RunScanWithContext(suite.ctx, &scanConfig, suite.database)
	require.NoError(t, err)
	require.NotNil(t, result)
	t.Logf("CI DEBUG: Scan completed, found %d hosts", len(result.Hosts))

	// Check database state after scan
	time.Sleep(100 * time.Millisecond) // Give time for transactions to complete
	after := getDatabaseCounts(t, suite)

	// Verify scan jobs were created
	if after.scanJobs <= before.scanJobs {
		debugScanJobFailure(t, suite, before.scanJobs, after.scanJobs)
	} else {
		t.Logf("✅ Scan jobs created successfully: %d new jobs", after.scanJobs-before.scanJobs)
	}

	// Verify port scans were created
	if after.portScans <= before.portScans {
		debugPortScanFailure(t, suite, before.portScans, after.portScans)
	} else {
		t.Logf("✅ Port scans created successfully: %d new scans", after.portScans-before.portScans)
	}
}

// databaseCounts holds counts of various database tables.
type databaseCounts struct {
	hosts     int
	scanJobs  int
	portScans int
}

// getDatabaseCounts retrieves current counts from all relevant database tables.
func getDatabaseCounts(t *testing.T, suite *IntegrationTestSuite) databaseCounts {
	var counts databaseCounts

	err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM hosts").Scan(&counts.hosts)
	require.NoError(t, err)
	t.Logf("CI DEBUG: Hosts in database: %d", counts.hosts)

	err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&counts.scanJobs)
	require.NoError(t, err)
	t.Logf("CI DEBUG: Scan jobs in database: %d", counts.scanJobs)

	err = suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM port_scans").Scan(&counts.portScans)
	require.NoError(t, err)
	t.Logf("CI DEBUG: Port scans in database: %d", counts.portScans)

	return counts
}

// debugScanJobFailure handles debugging when scan job creation fails.
func debugScanJobFailure(t *testing.T, suite *IntegrationTestSuite, before, after int) {
	t.Errorf("❌ CI ISSUE: No scan jobs created - before: %d, after: %d", before, after)

	// Debug: Check if scan jobs exist with any status
	var allScanJobs int
	err := suite.database.QueryRowContext(suite.ctx, "SELECT COUNT(*) FROM scan_jobs").Scan(&allScanJobs)
	if err == nil {
		t.Logf("CI DEBUG: Total scan jobs in database: %d", allScanJobs)
	}

	// Debug: Check scan_jobs table structure and recent entries
	rows, err := suite.database.QueryContext(suite.ctx,
		"SELECT id, status, started_at, completed_at, error_message FROM scan_jobs ORDER BY created_at DESC LIMIT 3")
	if err == nil {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				t.Logf("CI DEBUG: Error closing rows: %v", closeErr)
			}
		}()
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
}

// debugPortScanFailure handles debugging when port scan creation fails.
func debugPortScanFailure(t *testing.T, suite *IntegrationTestSuite, before, after int) {
	t.Errorf("❌ CI ISSUE: No port scans created - before: %d, after: %d", before, after)

	// Debug: Check port_scans table structure and entries
	rows, err := suite.database.QueryContext(suite.ctx,
		"SELECT job_id, host_id, port, protocol, state, service_name FROM port_scans ORDER BY scanned_at DESC LIMIT 3")
	if err == nil {
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				t.Logf("CI DEBUG: Error closing rows: %v", closeErr)
			}
		}()
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
}

// debugDatabaseState analyzes the current database state for debugging.
func debugDatabaseState(t *testing.T, suite *IntegrationTestSuite) {
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
}

// TestCIIsolatedStorage tests database storage operations in complete isolation
// from any scanning or discovery logic to pinpoint CI-specific issues.
func TestCIIsolatedStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	t.Log("=== CI DEBUG: Testing isolated database storage operations ===")

	// Test 1: Direct scan target creation
	t.Run("DirectScanTargetCreation", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct scan target creation...")

		// Create scan target directly
		targetID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Test Target", "192.168.1.0/24")
		require.NoError(t, err)
		t.Logf("✅ Created scan target: %s", targetID)

		// Verify it exists
		var count int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Scan target should exist in database")
		t.Log("✅ Verified scan target exists")
	})

	// Test 2: Direct scan job creation
	t.Run("DirectScanJobCreation", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct scan job creation...")

		// First create a scan target
		targetID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Test Job Target", "10.0.0.0/24")
		require.NoError(t, err)

		// Create scan job directly
		jobID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_jobs (id, target_id, status, created_at) VALUES ($1, $2, $3, $4)",
			jobID, targetID, "pending", time.Now())
		require.NoError(t, err)
		t.Logf("✅ Created scan job: %s", jobID)

		// Verify it exists
		var jobCount int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_jobs WHERE id = $1", jobID).Scan(&jobCount)
		require.NoError(t, err)
		assert.Equal(t, 1, jobCount, "Scan job should exist in database")
		t.Log("✅ Verified scan job exists")

		// Update job status
		_, err = suite.database.ExecContext(suite.ctx,
			"UPDATE scan_jobs SET status = 'completed', completed_at = $1 WHERE id = $2",
			time.Now(), jobID)
		require.NoError(t, err)
		t.Log("✅ Updated scan job status to completed")

		// Verify status update
		var status string
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT status FROM scan_jobs WHERE id = $1", jobID).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "completed", status, "Scan job status should be updated")
		t.Log("✅ Verified scan job status update")
	})

	// Test 3: Direct host creation and port scan storage
	t.Run("DirectHostAndPortScanStorage", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct host and port scan storage...")

		// Create host directly
		hostID := uuid.New().String()
		testIP := "172.16.0.1"
		_, err := suite.database.ExecContext(suite.ctx,
			"INSERT INTO hosts (id, ip_address, status, discovery_method, first_seen, last_seen) VALUES ($1, $2, $3, $4, $5, $6)",
			hostID, testIP, "up", "tcp", time.Now(), time.Now())
		require.NoError(t, err)
		t.Logf("✅ Created host: %s (%s)", hostID, testIP)

		// Verify host exists
		var hostCount int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM hosts WHERE id = $1", hostID).Scan(&hostCount)
		require.NoError(t, err)
		assert.Equal(t, 1, hostCount, "Host should exist in database")
		t.Log("✅ Verified host exists")

		// Create scan target and job for port scans
		targetID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Test Port Target", "172.16.0.0/24")
		require.NoError(t, err)

		jobID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_jobs (id, target_id, status, created_at) VALUES ($1, $2, $3, $4)",
			jobID, targetID, "running", time.Now())
		require.NoError(t, err)

		// Create port scan directly
		portScanID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO port_scans (id, job_id, host_id, port, protocol, state, scanned_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			portScanID, jobID, hostID, 80, "tcp", "open", time.Now())
		require.NoError(t, err)
		t.Logf("✅ Created port scan: %s (port 80/tcp open)", portScanID)

		// Verify port scan exists
		var portScanCount int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM port_scans WHERE id = $1", portScanID).Scan(&portScanCount)
		require.NoError(t, err)
		assert.Equal(t, 1, portScanCount, "Port scan should exist in database")
		t.Log("✅ Verified port scan exists")

		// Test querying port scans by job
		var scansForJob int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM port_scans WHERE job_id = $1", jobID).Scan(&scansForJob)
		require.NoError(t, err)
		assert.Equal(t, 1, scansForJob, "Should find port scans for job")
		t.Log("✅ Verified port scan query by job ID")
	})

	// Test 4: Transaction rollback behavior
	t.Run("TransactionBehavior", func(t *testing.T) {
		t.Log("CI DEBUG: Testing transaction behavior...")

		// Start transaction
		tx, err := suite.database.BeginTx(suite.ctx)
		require.NoError(t, err)

		// Create scan target in transaction
		targetID := uuid.New().String()
		_, err = tx.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Test TX Target", "192.168.100.0/24")
		require.NoError(t, err)
		t.Log("✅ Created scan target in transaction")

		// Verify it exists within transaction
		var countInTx int
		err = tx.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID).Scan(&countInTx)
		require.NoError(t, err)
		assert.Equal(t, 1, countInTx, "Should see scan target within transaction")
		t.Log("✅ Verified scan target exists within transaction")

		// Rollback transaction
		err = tx.Rollback()
		require.NoError(t, err)
		t.Log("✅ Rolled back transaction")

		// Verify it doesn't exist after rollback
		var countAfterRollback int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID).Scan(&countAfterRollback)
		require.NoError(t, err)
		assert.Equal(t, 0, countAfterRollback, "Should not see scan target after rollback")
		t.Log("✅ Verified scan target was rolled back")

		// Test successful commit
		tx2, err := suite.database.BeginTx(suite.ctx)
		require.NoError(t, err)

		targetID2 := uuid.New().String()
		_, err = tx2.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID2, "Test Commit Target", "192.168.200.0/24")
		require.NoError(t, err)

		err = tx2.Commit()
		require.NoError(t, err)
		t.Log("✅ Committed transaction")

		// Verify it exists after commit
		var countAfterCommit int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID2).Scan(&countAfterCommit)
		require.NoError(t, err)
		assert.Equal(t, 1, countAfterCommit, "Should see scan target after commit")
		t.Log("✅ Verified scan target was committed")
	})

	// Test 5: Database constraints and error handling
	t.Run("DatabaseConstraintsAndErrors", func(t *testing.T) {
		t.Log("CI DEBUG: Testing database constraints and error handling...")

		// Test duplicate IP address constraint
		hostID1 := uuid.New().String()
		testIP := "10.10.10.10"
		_, err := suite.database.ExecContext(suite.ctx,
			"INSERT INTO hosts (id, ip_address, status) VALUES ($1, $2, $3)",
			hostID1, testIP, "up")
		require.NoError(t, err)
		t.Log("✅ Created first host with IP")

		// Try to create another host with same IP - should fail
		hostID2 := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO hosts (id, ip_address, status) VALUES ($1, $2, $3)",
			hostID2, testIP, "up")
		assert.Error(t, err, "Should get constraint violation for duplicate IP")
		t.Log("✅ Verified unique IP constraint works")

		// Test foreign key constraints
		fakeJobID := uuid.New().String()
		fakeHostID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO port_scans (job_id, host_id, port, protocol, state) VALUES ($1, $2, $3, $4, $5)",
			fakeJobID, fakeHostID, 443, "tcp", "open")
		assert.Error(t, err, "Should get foreign key constraint violation")
		t.Log("✅ Verified foreign key constraints work")
	})
}

// TestCITransactionDebugging tests the exact transaction pattern used by scanning
// to isolate the foreign key constraint violation issue seen in CI.
func TestCITransactionDebugging(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	t.Log("=== CI DEBUG: Testing transaction patterns that match scanning behavior ===")

	// Test 1: Reproduce the exact sequence from CI logs
	t.Run("ReproduceCIFailureSequence", func(t *testing.T) {
		t.Log("CI DEBUG: Reproducing the exact sequence that fails in CI...")

		// Step 1: Create scan target (this works in CI)
		targetID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Ad-hoc scan: 127.0.0.1/32", "127.0.0.1/32")
		require.NoError(t, err)
		t.Log("✅ Step 1: Created scan target")

		// Step 2: Create scan job (this works in CI according to logs)
		jobID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at, scan_stats) VALUES ($1, $2, $3, $4, $5, $6)",
			jobID, targetID, "completed", time.Now(), time.Now(), `{"hosts_up": 1, "hosts_down": 0}`)
		require.NoError(t, err)
		t.Logf("✅ Step 2: Created scan job: %s", jobID)

		// Verify scan job exists immediately after creation
		var jobCount int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_jobs WHERE id = $1", jobID).Scan(&jobCount)
		require.NoError(t, err)
		assert.Equal(t, 1, jobCount, "Scan job should exist immediately after creation")
		t.Log("✅ Step 2a: Verified scan job exists")

		// Step 3: Create host (this works in CI)
		hostID := uuid.New().String()
		testIP := "10.0.0.100" // Use unique IP to avoid constraint violation
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO hosts (id, ip_address, status) VALUES ($1, $2, $3)",
			hostID, testIP, "up")
		require.NoError(t, err)
		t.Log("✅ Step 3: Created host")

		// Step 4: Try to create port scan (this fails in CI)
		t.Log("CI DEBUG: About to attempt port scan creation - this is where CI fails...")
		portScanID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			"INSERT INTO port_scans (id, job_id, host_id, port, protocol, state, scanned_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			portScanID, jobID, hostID, 22, "tcp", "closed", time.Now())

		if err != nil {
			t.Logf("❌ CI ISSUE REPRODUCED: Port scan creation failed: %v", err)

			// Check if scan job still exists after port scan failure
			var jobCountAfterFailure int
			err2 := suite.database.QueryRowContext(suite.ctx,
				"SELECT COUNT(*) FROM scan_jobs WHERE id = $1", jobID).Scan(&jobCountAfterFailure)
			if err2 == nil {
				t.Logf("CI DEBUG: Scan job count after port scan failure: %d", jobCountAfterFailure)
				if jobCountAfterFailure == 0 {
					t.Log("❌ CI CRITICAL: Scan job was rolled back after port scan failure!")
				}
			}

			// This should fail the test to show the issue
			require.NoError(t, err)
		}
		t.Logf("✅ Step 4: Created port scan: %s", portScanID)

		// Final verification
		var finalJobCount, finalPortCount int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_jobs WHERE id = $1", jobID).Scan(&finalJobCount)
		require.NoError(t, err)
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM port_scans WHERE id = $1", portScanID).Scan(&finalPortCount)
		require.NoError(t, err)

		t.Logf("✅ Final verification: scan job count=%d, port scan count=%d", finalJobCount, finalPortCount)
	})
}

// TestCIIsolatedDatabaseStorage tests database storage operations in complete isolation
// This helps determine if the issue is with database operations or scanning integration.
func TestCIIsolatedDatabaseStorage(t *testing.T) {
	suite := setupIntegrationTestSuite(t)
	defer suite.teardown(t)

	t.Log("=== CI DEBUG: Starting isolated database storage test ===")

	// Test 1: Direct host insertion
	t.Run("DirectHostInsertion", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct host insertion...")

		// Insert a host directly
		hostID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			`INSERT INTO hosts (id, ip_address, status, discovery_method, first_seen, last_seen)
			 VALUES ($1, $2, $3, $4, NOW(), NOW())`,
			hostID, "192.168.1.100", "up", "tcp")
		require.NoError(t, err)
		t.Log("✅ Direct host insertion successful")

		// Verify host was inserted
		var count int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM hosts WHERE id = $1", hostID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		t.Logf("✅ Host verified in database: count = %d", count)
	})

	// Test 2: Direct scan target and job insertion
	t.Run("DirectScanJobInsertion", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct scan job insertion...")

		// Insert scan target first
		targetID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			`INSERT INTO scan_targets (id, name, network, description)
			 VALUES ($1, $2, $3, $4)`,
			targetID, "Test Target", "192.168.1.0/24", "Test target for CI debugging")
		require.NoError(t, err)
		t.Log("✅ Direct scan target insertion successful")

		// Insert scan job
		jobID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			`INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at)
			 VALUES ($1, $2, $3, NOW(), NOW())`,
			jobID, targetID, "completed")
		require.NoError(t, err)
		t.Log("✅ Direct scan job insertion successful")

		// Verify scan job was inserted
		var count int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_jobs WHERE id = $1", jobID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		t.Logf("✅ Scan job verified in database: count = %d", count)
	})

	// Test 3: Direct port scan insertion
	t.Run("DirectPortScanInsertion", func(t *testing.T) {
		t.Log("CI DEBUG: Testing direct port scan insertion...")

		// First ensure we have a host and job to reference
		hostID := uuid.New().String()
		_, err := suite.database.ExecContext(suite.ctx,
			`INSERT INTO hosts (id, ip_address, status, discovery_method)
			 VALUES ($1, $2, $3, $4) ON CONFLICT (ip_address) DO NOTHING`,
			hostID, "192.168.1.101", "up", "tcp")
		require.NoError(t, err)

		// Get the actual host ID (in case it already existed)
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT id FROM hosts WHERE ip_address = $1", "192.168.1.101").Scan(&hostID)
		require.NoError(t, err)

		targetID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			`INSERT INTO scan_targets (id, name, network)
			 VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
			targetID, "Test Target 2", "192.168.1.0/24")
		require.NoError(t, err)

		jobID := uuid.New().String()
		_, err = suite.database.ExecContext(suite.ctx,
			`INSERT INTO scan_jobs (id, target_id, status)
			 VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`,
			jobID, targetID, "completed")
		require.NoError(t, err)

		// Now insert port scan
		_, err = suite.database.ExecContext(suite.ctx,
			`INSERT INTO port_scans (job_id, host_id, port, protocol, state, scanned_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())`,
			jobID, hostID, 80, "tcp", "open")
		require.NoError(t, err)
		t.Log("✅ Direct port scan insertion successful")

		// Verify port scan was inserted
		var count int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM port_scans WHERE job_id = $1 AND host_id = $2 AND port = 80",
			jobID, hostID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		t.Logf("✅ Port scan verified in database: count = %d", count)
	})

	// Test 4: Transaction rollback behavior
	t.Run("TransactionBehavior", func(t *testing.T) {
		t.Log("CI DEBUG: Testing transaction behavior...")

		// Start a transaction
		tx, err := suite.database.BeginTx(suite.ctx)
		require.NoError(t, err)

		// Insert scan target in transaction
		targetID := uuid.New().String()
		_, err = tx.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID, "Test TX Target", "192.168.100.0/24")
		require.NoError(t, err)
		t.Log("✅ Created scan target in transaction")

		// Verify it exists within transaction
		var countInTx int
		err = tx.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID).Scan(&countInTx)
		require.NoError(t, err)
		assert.Equal(t, 1, countInTx, "Should see scan target within transaction")
		t.Log("✅ Verified scan target exists within transaction")

		// Rollback transaction
		err = tx.Rollback()
		require.NoError(t, err)
		t.Log("✅ Rolled back transaction")

		// Verify it doesn't exist after rollback
		var countAfterRollback int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID).Scan(&countAfterRollback)
		require.NoError(t, err)
		assert.Equal(t, 0, countAfterRollback, "Should not see scan target after rollback")
		t.Log("✅ Verified scan target was rolled back")

		// Test successful commit
		tx2, err := suite.database.BeginTx(suite.ctx)
		require.NoError(t, err)

		targetID2 := uuid.New().String()
		_, err = tx2.ExecContext(suite.ctx,
			"INSERT INTO scan_targets (id, name, network) VALUES ($1, $2, $3)",
			targetID2, "Test Commit Target", "192.168.200.0/24")
		require.NoError(t, err)

		err = tx2.Commit()
		require.NoError(t, err)
		t.Log("✅ Committed transaction")

		// Verify it exists after commit
		var countAfterCommit int
		err = suite.database.QueryRowContext(suite.ctx,
			"SELECT COUNT(*) FROM scan_targets WHERE id = $1", targetID2).Scan(&countAfterCommit)
		require.NoError(t, err)
		assert.Equal(t, 1, countAfterCommit, "Should see scan target after commit")
		t.Log("✅ Verified scan target was committed")
	})
}

// TestCIMinimalDiscoveryStorage tests just the discovery storage functionality.
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
