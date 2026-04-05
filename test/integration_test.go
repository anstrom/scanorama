package test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/test/helpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLocalhostIP = "127.0.0.1"
)

// ── Package-level shared state ────────────────────────────────────────────────

// sharedDB is initialized once in TestMain and reused by every test.
var sharedDB *db.DB

// nmapAvailable is set in TestMain; tests that need nmap skip when false.
var nmapAvailable bool

// TestMain sets up the shared database connection and migrations once for the
// entire package, then runs all tests, then tears down.
func TestMain(m *testing.M) {
	flag.Parse()
	ctx := context.Background()

	// Determine nmap availability up front.
	if _, err := exec.LookPath("nmap"); err == nil {
		nmapAvailable = true
	}

	// Skip the DB setup entirely when running in short mode; individual tests
	// will call t.Skip() themselves via requireDB().
	if !testing.Short() {
		testConfig, err := helpers.GetAvailableDatabase()
		if err == nil {
			// Drop all tables / functions so migrations start clean.
			cleanupIntegrationDatabase(ctx, testConfig)

			dbConfig := &db.Config{
				Host:            testConfig.Host,
				Port:            testConfig.Port,
				Database:        testConfig.Database,
				Username:        testConfig.Username,
				Password:        testConfig.Password,
				SSLMode:         testConfig.SSLMode,
				MaxOpenConns:    10,
				MaxIdleConns:    5,
				ConnMaxLifetime: 5 * time.Minute,
				ConnMaxIdleTime: 2 * time.Minute,
			}

			sharedDB, err = db.ConnectAndMigrate(ctx, dbConfig)
			if err != nil {
				// Database unavailable — tests will skip individually.
				sharedDB = nil
			}
		}
	}

	code := m.Run()

	if sharedDB != nil {
		sharedDB.Close()
	}

	os.Exit(code)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// requireDB skips the test when the shared database is unavailable (e.g. no
// Postgres in the environment) and returns the shared connection.
func requireDB(t *testing.T) *db.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if sharedDB == nil {
		t.Skip("Skipping integration test: database not available")
	}
	return sharedDB
}

// requireNmap skips the test when nmap is not installed.
func requireNmap(t *testing.T) {
	t.Helper()
	if !nmapAvailable {
		t.Skip("Skipping test: nmap not available")
	}
}

// requireRoot skips the test when the process is not running as UID 0.
func requireRoot(t *testing.T) {
	t.Helper()
	out, err := exec.Command("id", "-u").Output()
	if err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Skip("Skipping test: requires root privileges (CAP_NET_RAW)")
	}
}

// cleanupRows removes rows written since startTime so parallel tests don't
// interfere with each other's assertions. Called via t.Cleanup.
//
// The 127.0.0.1 host row is intentionally NOT deleted: it is shared across
// all scan tests (CreateOrUpdate is idempotent on ip_address) and removing it
// would cascade-delete port_scans belonging to other parallel tests.
// Port scans are cleaned up directly by scanned_at timestamp instead.
func cleanupRows(t *testing.T, database *db.DB, startTime time.Time) {
	t.Helper()
	ctx := context.Background()
	queries := []string{
		// Remove port scans directly by time — avoids touching the shared host row.
		"DELETE FROM port_scans WHERE scanned_at >= $1",
		"DELETE FROM scan_jobs WHERE created_at >= $1",
		"DELETE FROM discovery_jobs WHERE created_at >= $1",
		// Only delete hosts that are NOT the shared localhost target.
		"DELETE FROM hosts WHERE first_seen >= $1 AND ip_address != '127.0.0.1'::inet",
		// Delete all networks created during this test. The is_active/scan_enabled
		// conditions were removed because tests like TestScanWithPreExistingScanJob
		// create networks with is_active=true, scan_enabled=false which the old
		// condition silently skipped, leaving orphan rows that broke subsequent
		// TestNetworkHandler_ListNetworks assertions.
		"DELETE FROM networks WHERE created_at >= $1",
	}
	for _, q := range queries {
		if _, err := database.ExecContext(ctx, q, startTime); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}
}

// cleanupIntegrationDatabase drops all user tables and functions so that
// ConnectAndMigrate always starts from a blank slate.
func cleanupIntegrationDatabase(ctx context.Context, testConfig *helpers.DatabaseConfig) {
	dbConfig := &db.Config{
		Host:            testConfig.Host,
		Port:            testConfig.Port,
		Database:        testConfig.Database,
		Username:        testConfig.Username,
		Password:        testConfig.Password,
		SSLMode:         testConfig.SSLMode,
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	database, err := db.Connect(ctx, dbConfig)
	if err != nil {
		return
	}
	defer database.Close()

	var tables []string
	database.Select(&tables, `
		SELECT schemaname||'.'||tablename
		FROM pg_tables
		WHERE schemaname = 'public'`)
	for _, table := range tables {
		database.Exec("DROP TABLE IF EXISTS " + table + " CASCADE")
	}

	var functions []string
	database.Select(&functions, `
		SELECT n.nspname||'.'||p.proname||'('||pg_get_function_identity_arguments(p.oid)||')'
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		LEFT JOIN pg_depend d ON d.objid = p.oid AND d.deptype = 'e'
		WHERE n.nspname = 'public' AND p.prokind = 'f' AND d.objid IS NULL`)
	for _, function := range functions {
		database.Exec("DROP FUNCTION IF EXISTS " + function + " CASCADE")
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestBasicScanFunctionality tests that scanning works and stores results correctly.
func TestBasicScanFunctionality(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "22",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	result, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "Scan execution should succeed")
	require.NotNil(t, result, "Scan result should not be nil")
	require.NotEmpty(t, result.Hosts, "Scan should return at least one host")

	host := result.Hosts[0]
	assert.Equal(t, "up", host.Status, "Host should be detected as up")
	assert.Equal(t, "127.0.0.1", host.Address, "Host IP should match target")

	var hostCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM hosts WHERE ip_address = $2 AND last_seen >= $1`,
		testStartTime, testLocalhostIP).Scan(&hostCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, hostCount, 1, "Should have stored at least one host record")

	var jobCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_jobs WHERE created_at >= $1 AND status = 'completed'`,
		testStartTime).Scan(&jobCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, jobCount, 1, "Should have created at least one scan job")

	var portScanCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans ps
		 JOIN hosts h ON ps.host_id = h.id
		 WHERE h.ip_address = '127.0.0.1' AND ps.port = 22 AND ps.scanned_at >= $1`,
		testStartTime).Scan(&portScanCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portScanCount, 1, "Should have stored port scan results")

	t.Logf("TestBasicScanFunctionality: %d hosts, %d jobs, %d port scans", hostCount, jobCount, portScanCount)
}

// TestDiscoveryFunctionality tests discovery job creation and database operations.
func TestDiscoveryFunctionality(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	discoveryEngine := discovery.NewEngine(database)
	discoveryConfig := discovery.Config{
		Network:     testLocalhostIP + "/32",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     15 * time.Second,
		Concurrency: 1,
	}

	job, err := discoveryEngine.Discover(ctx, &discoveryConfig)
	require.NoError(t, err, "Discovery should start successfully")
	require.NotEqual(t, uuid.Nil, job.ID, "Discovery job should have valid ID")

	err = discoveryEngine.WaitForCompletion(ctx, job.ID, 20*time.Second)
	require.NoError(t, err, "Discovery should complete within timeout")

	var jobStatus string
	var hostsDiscovered int
	err = database.QueryRowContext(ctx,
		`SELECT status, hosts_discovered FROM discovery_jobs WHERE id = $1`,
		job.ID).Scan(&jobStatus, &hostsDiscovered)
	require.NoError(t, err)
	assert.Equal(t, "completed", jobStatus)

	t.Logf("TestDiscoveryFunctionality: status=%s, hosts_discovered=%d", jobStatus, hostsDiscovered)
}

// TestScanDiscoveredHost tests scanning a pre-inserted host record.
func TestScanDiscoveredHost(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	// Use ON CONFLICT DO NOTHING: a prior test may have already created the
	// 127.0.0.1 row. We just need it to exist before the scan runs.
	_, err := database.ExecContext(ctx,
		`INSERT INTO hosts (id, ip_address, status, discovery_method, last_seen)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (ip_address) DO UPDATE SET last_seen = EXCLUDED.last_seen`,
		uuid.New(), testLocalhostIP, "up", "tcp", time.Now())
	require.NoError(t, err, "Should be able to upsert test host record")

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "8080,8022,8379",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}

	result, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "Scan should succeed")
	require.NotEmpty(t, result.Hosts, "Scan should return hosts")

	var scanCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans ps
		 JOIN hosts h ON ps.host_id = h.id
		 WHERE h.ip_address = $2 AND ps.scanned_at >= $1`,
		testStartTime, testLocalhostIP).Scan(&scanCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, scanCount, 1, "Should have stored scan results")

	t.Logf("TestScanDiscoveredHost: %d port scans stored", scanCount)
}

// TestDatabaseQueries tests that stored data can be retrieved and queried correctly.
func TestDatabaseQueries(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	// Seed data for this test.
	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "8080,8022,8379",
		ScanType:    "connect",
		TimeoutSec:  10,
		Concurrency: 1,
	}
	_, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "Setup scan should succeed")

	t.Run("QueryActiveHosts", func(t *testing.T) {
		hostRepo := db.NewHostRepository(database)
		hosts, err := hostRepo.GetActiveHosts(ctx)
		require.NoError(t, err)

		var found bool
		for _, host := range hosts {
			if host.IPAddress.String() == "127.0.0.1" {
				found = true
				assert.Equal(t, "up", host.Status)
				break
			}
		}
		assert.True(t, found, "Should find our test host in active hosts")
	})

	t.Run("QueryPortScansByHost", func(t *testing.T) {
		var hostID uuid.UUID
		err := database.QueryRowContext(ctx,
			`SELECT id FROM hosts WHERE ip_address = $2 AND last_seen >= $1`,
			testStartTime, testLocalhostIP).Scan(&hostID)
		require.NoError(t, err, "Should find test host")

		portRepo := db.NewPortScanRepository(database)
		portScans, err := portRepo.GetByHost(ctx, hostID)
		require.NoError(t, err)

		ciPorts := map[int]bool{8080: true, 8022: true, 8379: true}
		var foundCIPort bool
		for _, ps := range portScans {
			if !ciPorts[ps.Port] {
				continue
			}
			foundCIPort = true
			assert.Equal(t, "tcp", ps.Protocol)
			assert.Equal(t, hostID, ps.HostID)
			t.Logf("Found scan result for CI service port %d", ps.Port)
			break
		}
		assert.True(t, foundCIPort, "Should find scan result for at least one CI service port (8080, 8022, 8379)")
	})

	t.Run("QueryScanJobsWithResults", func(t *testing.T) {
		type jobRow struct {
			JobID        uuid.UUID  `db:"job_id"`
			TargetName   string     `db:"target_name"`
			Status       string     `db:"status"`
			HostsScanned int        `db:"hosts_scanned"`
			CompletedAt  *time.Time `db:"completed_at"`
		}
		var jobResults []jobRow

		err := database.SelectContext(ctx, &jobResults, `
			SELECT
				sj.id           AS job_id,
				n.name          AS target_name,
				sj.status,
				COUNT(DISTINCT ps.host_id) AS hosts_scanned,
				sj.completed_at
			FROM scan_jobs sj
			JOIN networks n ON sj.network_id = n.id
			LEFT JOIN port_scans ps ON sj.id = ps.job_id
			WHERE sj.created_at >= $1
			GROUP BY sj.id, n.name, sj.status, sj.completed_at
			ORDER BY sj.created_at DESC`, testStartTime)
		require.NoError(t, err)
		require.NotEmpty(t, jobResults, "Should find scan job results from our test")

		job := jobResults[0]
		assert.Equal(t, "completed", job.Status)
		assert.GreaterOrEqual(t, job.HostsScanned, 1)
		assert.NotNil(t, job.CompletedAt)
	})
}

// TestConnectScanType tests that the connect scan type works end-to-end.
func TestConnectScanType(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "8080,8022,8379",
		ScanType:    "connect",
		TimeoutSec:  15,
		Concurrency: 1,
	}

	result, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "connect scan should succeed")
	require.NotEmpty(t, result.Hosts, "Should return host results")

	var portScanCount int
	err = database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM port_scans ps
		JOIN scan_jobs sj ON ps.job_id = sj.id
		JOIN hosts h ON ps.host_id = h.id
		WHERE h.ip_address = $2
		  AND ps.port IN (8080, 8022, 8379)
		  AND sj.created_at >= $1`,
		testStartTime, testLocalhostIP).Scan(&portScanCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portScanCount, 1, "Should have port scan results for connect scan")
}

// TestSynScanType tests that SYN scans work when root privileges are available.
func TestSynScanType(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)
	requireRoot(t) // SYN scan requires CAP_NET_RAW

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "8080,8022,8379",
		ScanType:    "syn",
		TimeoutSec:  15,
		Concurrency: 1,
	}

	result, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "syn scan should succeed")
	require.NotEmpty(t, result.Hosts, "Should return host results")

	var portScanCount int
	err = database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM port_scans ps
		JOIN scan_jobs sj ON ps.job_id = sj.id
		JOIN hosts h ON ps.host_id = h.id
		WHERE h.ip_address = $2
		  AND ps.port IN (8080, 8022, 8379)
		  AND sj.created_at >= $1`,
		testStartTime, testLocalhostIP).Scan(&portScanCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portScanCount, 1, "Should have port scan results for syn scan")
}

// TestNmapAvailability tests that nmap is available and functional in the test environment.
// This test does not require a database and always runs first (no t.Parallel so it
// acts as an early gate for the rest of the suite).
func TestNmapAvailability(t *testing.T) {
	nmapPath, err := exec.LookPath("nmap")
	if err != nil {
		t.Fatalf("nmap not found in PATH: %v", err)
	}
	t.Logf("Found nmap at: %s", nmapPath)

	out, err := exec.Command("nmap", "--version").Output()
	if err != nil {
		t.Fatalf("Failed to run nmap --version: %v", err)
	}
	if !strings.Contains(string(out), "Nmap") {
		t.Fatalf("nmap version output doesn't contain 'Nmap': %s", out)
	}

	out, err = exec.Command("nmap", "-sL", "127.0.0.1").Output()
	if err != nil {
		t.Fatalf("Failed to run nmap -sL: %v", err)
	}
	if !strings.Contains(string(out), "127.0.0.1") {
		t.Fatalf("nmap list scan output doesn't contain '127.0.0.1': %s", out)
	}

	t.Log("nmap availability test passed")
}

// TestScanWithPreExistingScanJob verifies the API path for the scan-UUID reuse bugs:
//
//   - Bug A: storeScanResults must reuse the caller-supplied UUID (ScanConfig.ScanID)
//     rather than generating a fresh one, so port_scans rows end up linked to
//     the UUID returned to the API client.
//   - Bug B: when ScanID is supplied the existing scan_jobs row must be UPDATEd
//     (not INSERTed), so the PK constraint is not violated and port scan results
//     are actually written.
//
// TestScanWithPreExistingScanJob verifies the API path for the scan-UUID reuse bugs:
func TestScanWithPreExistingScanJob(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	knownUUID := uuid.New()

	networkID := uuid.New()
	_, err := database.ExecContext(ctx,
		`INSERT INTO networks
		    (id, name, cidr, discovery_method, is_active, scan_enabled,
		     scan_interval_seconds, scan_ports, scan_type)
		VALUES ($1,$2,$3,'tcp',true,false,0,'8080,8022','connect')`,
		networkID, "pre-existing-api-target-"+knownUUID.String()[:8], testLocalhostIP+"/32")
	require.NoError(t, err)

	jobRepo := db.NewScanJobRepository(database)
	preInsertedJob := &db.ScanJob{
		ID:        knownUUID,
		NetworkID: &networkID,
		Status:    db.ScanJobStatusPending,
	}
	require.NoError(t, jobRepo.Create(ctx, preInsertedJob),
		"Should pre-insert scan job (simulating API CreateScan)")

	scanConfig := &scanning.ScanConfig{
		Targets:     []string{testLocalhostIP},
		Ports:       "8080,8022",
		ScanType:    "connect",
		TimeoutSec:  15,
		Concurrency: 1,
		ScanID:      &knownUUID,
	}

	result, err := scanning.RunScanWithContext(ctx, scanConfig, database)
	require.NoError(t, err, "Scan with pre-existing scan job should succeed")
	require.NotEmpty(t, result.Hosts, "Scan should return at least one host")

	// Bug A: port_scans must be linked to the known UUID.
	var portScanCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans WHERE job_id = $1`, knownUUID).Scan(&portScanCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, portScanCount, 1,
		"port_scans must be stored under the pre-existing job UUID (Bug A)")

	// Bug B: the scan_jobs row must not have been duplicated.
	var jobRowCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_jobs WHERE id = $1`, knownUUID).Scan(&jobRowCount)
	require.NoError(t, err)
	assert.Equal(t, 1, jobRowCount,
		"Must be exactly one scan_jobs row for the known UUID (Bug B)")

	t.Logf("TestScanWithPreExistingScanJob: job_id=%s, port_scans=%d, scan_jobs rows=%d",
		knownUUID, portScanCount, jobRowCount)
}

// TestNetworkRangeDiscovery tests discovery for a small network range.
func TestNetworkRangeDiscovery(t *testing.T) {
	database := requireDB(t)
	requireNmap(t)

	testStartTime := time.Now()
	t.Cleanup(func() { cleanupRows(t, database, testStartTime) })
	ctx := context.Background()

	discoveryEngine := discovery.NewEngine(database)
	discoveryConfig := discovery.Config{
		Network:     "127.0.0.0/30",
		Method:      "tcp",
		DetectOS:    false,
		Timeout:     15 * time.Second,
		Concurrency: 2,
		MaxHosts:    10,
	}

	job, err := discoveryEngine.Discover(ctx, &discoveryConfig)
	require.NoError(t, err, "Network range discovery should start successfully")
	require.NotEqual(t, uuid.Nil, job.ID)

	err = discoveryEngine.WaitForCompletion(ctx, job.ID, 20*time.Second)
	require.NoError(t, err, "Network range discovery should complete within timeout")

	var jobStatus string
	var hostsDiscovered int
	err = database.QueryRowContext(ctx,
		`SELECT status, hosts_discovered FROM discovery_jobs WHERE id = $1`,
		job.ID).Scan(&jobStatus, &hostsDiscovered)
	require.NoError(t, err)
	assert.Equal(t, "completed", jobStatus)

	t.Logf("TestNetworkRangeDiscovery: status=%s, hosts_discovered=%d", jobStatus, hostsDiscovered)
}

// TestDatabaseQueriesOnly exercises DB-layer helpers that don't require nmap.
// Safe to run in parallel — no nmap, no shared 127.0.0.1 writes.
func TestDatabaseQueriesOnly(t *testing.T) {
	t.Parallel()
	database := requireDB(t)
	ctx := context.Background()

	// Verify core tables are present (schema sanity check).
	var count int
	err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('hosts', 'scan_jobs', 'port_scans')`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Core tables hosts, scan_jobs, port_scans must exist")

	// Verify host repository instantiates without error.
	hostRepo := db.NewHostRepository(database)
	require.NotNil(t, hostRepo)

	hosts, err := hostRepo.GetActiveHosts(ctx)
	require.NoError(t, err, "GetActiveHosts must not error on an empty or populated DB")
	t.Logf("TestDatabaseQueriesOnly: %d active hosts", len(hosts))

	// Verify networks table is queryable.
	var networkCount int
	err = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM networks").Scan(&networkCount)
	require.NoError(t, err, "SELECT COUNT(*) FROM networks must not error")
	t.Logf("TestDatabaseQueriesOnly: %d networks", networkCount)

	// Verify port-scan repository instantiates without error.
	portRepo := db.NewPortScanRepository(database)
	require.NotNil(t, portRepo)

	// Use a known-absent UUID so GetByHost returns empty without error.
	absentID := uuid.New()
	portScans, err := portRepo.GetByHost(ctx, absentID)
	require.NoError(t, err, "GetByHost with absent UUID must not error")
	assert.Empty(t, portScans, "No port scans expected for absent host")

	// Verify scan job repository instantiates without error.
	scanJobRepo := db.NewScanJobRepository(database)
	require.NotNil(t, scanJobRepo)
	_ = fmt.Sprintf("scan job repo: %T", scanJobRepo)
}

// TestRecoverStaleJobs verifies the end-to-end startup recovery path against a
// real PostgreSQL instance.  It inserts rows directly into scan_jobs and
// discovery_jobs with status='running', calls RecoverStaleJobs, and confirms
// every affected row is transitioned to 'failed' with the expected fields set.
func TestRecoverStaleJobs(t *testing.T) {
	t.Parallel()
	database := requireDB(t)
	ctx := context.Background()

	// ── seed stale scan_jobs ──────────────────────────────────────────────
	scanID1 := uuid.New()
	scanID2 := uuid.New()
	for _, id := range []uuid.UUID{scanID1, scanID2} {
		_, err := database.ExecContext(ctx, `
			INSERT INTO scan_jobs (id, status, created_at, started_at, execution_details)
			VALUES ($1, 'running', NOW(), NOW(), '{}')
		`, id)
		require.NoError(t, err, "seed stale scan_job")
	}

	// ── seed a stale discovery_job ────────────────────────────────────────
	discID := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO discovery_jobs (id, network, method, status, created_at, hosts_discovered, hosts_responsive)
		VALUES ($1, '10.99.0.0/16', 'tcp', 'running', NOW(), 0, 0)
	`, discID)
	require.NoError(t, err, "seed stale discovery_job")

	// ── seed a non-running job that must NOT be touched ───────────────────
	pendingID := uuid.New()
	_, err = database.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, status, created_at, execution_details)
		VALUES ($1, 'pending', NOW(), '{}')
	`, pendingID)
	require.NoError(t, err, "seed pending scan_job")

	// ── run recovery ─────────────────────────────────────────────────────
	result, err := db.RecoverStaleJobs(ctx, database)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, result.ScanJobsRecovered, 2,
		"at least the two seeded running scan_jobs must be recovered")
	assert.GreaterOrEqual(t, result.DiscoveryJobsRecovered, 1,
		"at least the seeded running discovery_job must be recovered")
	assert.GreaterOrEqual(t, result.Total(), 3)

	// ── verify scan_jobs rows ─────────────────────────────────────────────
	for _, id := range []uuid.UUID{scanID1, scanID2} {
		var status, errMsg string
		var completedAt *time.Time
		err := database.QueryRowContext(ctx,
			`SELECT status, error_message, completed_at FROM scan_jobs WHERE id = $1`, id,
		).Scan(&status, &errMsg, &completedAt)
		require.NoError(t, err)
		assert.Equal(t, "failed", status, "scan_job %s must be failed", id)
		assert.Equal(t, "interrupted by server restart", errMsg)
		assert.NotNil(t, completedAt, "completed_at must be set")
	}

	// ── verify discovery_job row ──────────────────────────────────────────
	var discStatus string
	var discCompletedAt *time.Time
	err = database.QueryRowContext(ctx,
		`SELECT status, completed_at FROM discovery_jobs WHERE id = $1`, discID,
	).Scan(&discStatus, &discCompletedAt)
	require.NoError(t, err)
	assert.Equal(t, "failed", discStatus, "discovery_job must be failed")
	assert.NotNil(t, discCompletedAt, "completed_at must be set")

	// ── verify pending job was not touched ────────────────────────────────
	var pendingStatus string
	err = database.QueryRowContext(ctx,
		`SELECT status FROM scan_jobs WHERE id = $1`, pendingID,
	).Scan(&pendingStatus)
	require.NoError(t, err)
	assert.Equal(t, "pending", pendingStatus, "pending job must remain untouched")

	// ── idempotency: a second call must recover zero rows ─────────────────
	result2, err := db.RecoverStaleJobs(ctx, database)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.ScanJobsRecovered,
		"second recovery call must find no stale scan_jobs")
	assert.Equal(t, 0, result2.DiscoveryJobsRecovered,
		"second recovery call must find no stale discovery_jobs")

	// ── cleanup ───────────────────────────────────────────────────────────
	for _, id := range []uuid.UUID{scanID1, scanID2, pendingID} {
		_, _ = database.ExecContext(ctx, `DELETE FROM scan_jobs WHERE id = $1`, id)
	}
	_, _ = database.ExecContext(ctx, `DELETE FROM discovery_jobs WHERE id = $1`, discID)
}
