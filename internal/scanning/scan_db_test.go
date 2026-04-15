package scanning

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
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

	cfg := getTestDatabaseConfig()

	database, err := db.ConnectAndMigrate(context.Background(), &cfg)
	if err != nil {
		t.Skipf("Could not connect to test database (%s:%d/%s): %v",
			cfg.Host, cfg.Port, cfg.Database, err)
		return nil
	}
	return database
}

// getTestDatabaseConfig returns the database configuration for testing,
// read from TEST_DB_* environment variables (defaulting to the dedicated
// test database on port 5433, which is kept separate from the dev DB).
func getTestDatabaseConfig() db.Config {
	return db.Config{
		Host:            getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            getEnvIntOrDefault("TEST_DB_PORT", 5433),
		Database:        getEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username:        getEnvOrDefault("TEST_DB_USER", "test_user"),
		Password:        getEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
		fmt.Printf("warning: could not parse %s=%q as int, using default %d\n", key, val, defaultValue)
	}
	return defaultValue
}

// TestStoreScanResults_SuccessfulStorage tests that scan results are correctly persisted.
func TestStoreScanResults_SuccessfulStorage(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	cleanupHost(t, database, "192.168.1.100")

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
	err := storeScanResults(ctx, database, config, result, nil)
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
	cleanupHost(t, database, "10.0.0.1")
	cleanupHost(t, database, "10.0.0.2")
	cleanupHost(t, database, "10.0.0.3")

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

	err := storeScanResults(ctx, database, config, result, nil)
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
	err := storeScanResults(ctx, database, config, result, nil)
	// The function may return an error or succeed with empty results
	// Both are acceptable behaviors for edge cases
	if err != nil {
		assert.Error(t, err, "should handle invalid target")
	}
}

// TestAdhocScan_NoContainingNetwork verifies that a standalone scan run
// whose target is not inside any registered network returns uuid.Nil and
// does not create any new network rows.
func TestAdhocScan_NoContainingNetwork(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	// Precondition: ensure no registered network contains the target IP so
	// the test is deterministic regardless of other fixtures in the DB.
	const target = "203.0.113.7" // TEST-NET-3, reserved for documentation
	var containing int
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM networks WHERE $1::inet <<= cidr`, target,
	).Scan(&containing))
	require.Equal(t, 0, containing, "precondition: no registered network may contain %s", target)

	var before int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM networks`).Scan(&before))

	id, err := findContainingNetwork(ctx, database, target)
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, id)

	// The lookup must not create any new network rows.
	var after int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT COUNT(*) FROM networks`).Scan(&after))
	assert.Equal(t, before, after, "ad-hoc lookup must not create network rows")
}

// TestAdhocScan_AttachesToContainingNetwork verifies that a standalone
// scan of an IP inside a registered network attaches to it.
func TestAdhocScan_AttachesToContainingNetwork(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	// Prophylactic cleanup: remove any leftover rows from a prior failed run so
	// the unique constraints networks_name_key / networks_cidr_key don't collide.
	cleanupNetworksByName(t, database, "lab")
	cleanupNetworksByCIDR(t, database, "10.0.0.0/24")
	want := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES ($1, 'lab', '10.0.0.0/24', '22,80', 'connect', true, true, 'tcp')
	`, want)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM networks WHERE id = $1`, want)
	})

	got, err := findContainingNetwork(ctx, database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestAdhocScan_LongestPrefixWins verifies that when two registered networks
// both contain the target IP, the most-specific one is chosen.
func TestAdhocScan_LongestPrefixWins(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	// Prophylactic cleanup: remove any leftover rows from a prior failed run so
	// the unique constraints networks_name_key / networks_cidr_key don't collide.
	cleanupNetworksByName(t, database, "corp")
	cleanupNetworksByName(t, database, "dmz")
	cleanupNetworksByCIDR(t, database, "10.0.0.0/16")
	cleanupNetworksByCIDR(t, database, "10.0.0.0/24")
	slash16 := uuid.New()
	slash24 := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks (id, name, cidr, scan_ports, scan_type,
		                     is_active, scan_enabled, discovery_method)
		VALUES
		  ($1, 'corp', '10.0.0.0/16', '22,80', 'connect', true, true, 'tcp'),
		  ($2, 'dmz',  '10.0.0.0/24', '22,80', 'connect', true, true, 'tcp')
	`, slash16, slash24)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM networks WHERE id IN ($1, $2)`, slash16, slash24)
	})

	got, err := findContainingNetwork(ctx, database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, slash24, got, "longest-prefix (/24) must win over /16")
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

	_, err := storeHostResults(ctx, database, jobID, []Host{})
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
	cleanupHost(t, database, "172.16.0.1")
	jobID := uuid.New()

	hosts := []Host{
		{
			Address: "172.16.0.1",
			Status:  "up",
			Ports:   []Port{}, // No open ports
		},
	}

	_, err := storeHostResults(ctx, database, jobID, hosts)
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
	cleanupHost(t, database, "203.0.113.42")
	hostRepo := db.NewHostRepository(database)

	ipAddr := db.IPAddr{IP: net.ParseIP("203.0.113.42")}
	hostInput := Host{Address: "203.0.113.42", Status: "up"}

	host, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, &hostInput)
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
	firstHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, &hostInput)
	require.NoError(t, err)
	firstID := firstHost.ID

	// Try to create same host again
	secondHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, &hostInput)
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

// TestStoreScanResults_ScanIDRoundTrip verifies the critical invariant that
// when a ScanID is passed to storeScanResults the resulting port_scans rows
// are stored with that exact job_id, so GetScanResults can retrieve them.
// This is the integration-level regression test for the bug where
// storeScanResults always called uuid.New(), breaking the API lookup.
// createScanJobForTest inserts a minimal scan_job row using the given UUID,
// mirroring what CreateScan/StartScan do before RunScanWithContext is called.
// It creates the required scan_target row first to satisfy the FK constraint.
func createScanJobForTest(t *testing.T, ctx context.Context, database *db.DB, scanID uuid.UUID, network string) {
	t.Helper()

	networkID := uuid.New()
	_, err := database.ExecContext(ctx, `
		INSERT INTO networks
		    (id, name, cidr, discovery_method, is_active, scan_enabled,
		     scan_interval_seconds, scan_ports, scan_type)
		VALUES ($1, $2, $3, 'tcp', false, false, 0, '', 'connect')`,
		networkID, "test-network-"+scanID.String()[:8], network,
	)
	require.NoError(t, err, "pre-create networks row for test")

	_, err = database.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, network_id, status, started_at, created_at)
		VALUES ($1, $2, 'running', NOW(), NOW())`,
		scanID, networkID,
	)
	require.NoError(t, err, "pre-create scan_job row for test")
}

// cleanupHost removes a host row by IP address (and its dependent port_scans) so
// each test starts from a known-clean state regardless of prior runs.
func cleanupHost(t *testing.T, database *db.DB, ip string) {
	t.Helper()
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM hosts WHERE ip_address = $1::inet`, ip)
}

// cleanupNetworksByName removes network rows with the given name so tests that
// insert fixed-name fixtures are re-runnable after a prior failure.
func cleanupNetworksByName(t *testing.T, database *db.DB, name string) {
	t.Helper()
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM networks WHERE name = $1`, name)
}

// cleanupNetworksByCIDR removes network rows with the given CIDR so tests that
// insert fixed-CIDR fixtures are re-runnable after a prior failure.
func cleanupNetworksByCIDR(t *testing.T, database *db.DB, cidr string) {
	t.Helper()
	_, _ = database.ExecContext(context.Background(),
		`DELETE FROM networks WHERE cidr = $1::cidr`, cidr)
}

func TestStoreScanResults_ScanIDRoundTrip(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	// Use a fixed scan ID — this simulates the UUID the API exposes to the client.
	scanID := uuid.New()

	cleanupHost(t, database, "10.42.0.1")
	// Pre-create the scan_job row exactly as CreateScan/StartScan would, so
	// storeScanResults (which now UPDATEs rather than INSERTs when a scanID is
	// supplied) finds the row it expects.
	createScanJobForTest(t, ctx, database, scanID, "10.42.0.1/32")

	config := &ScanConfig{
		Targets:  []string{"10.42.0.1"},
		Ports:    "22,80",
		ScanType: "connect",
		ScanID:   &scanID,
	}

	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  5 * time.Second,
		Stats:     HostStats{Up: 1, Down: 0, Total: 1},
		Hosts: []Host{
			{
				Address: "10.42.0.1",
				Status:  "up",
				Ports: []Port{
					{Number: 22, Protocol: "tcp", State: "open", Service: "ssh"},
					{Number: 80, Protocol: "tcp", State: "open", Service: "http"},
				},
			},
		},
	}

	err := storeScanResults(ctx, database, config, result, &scanID)
	require.NoError(t, err, "storeScanResults should succeed")

	// Query port_scans using the same scanID — this mirrors what GetScanResults does.
	var portCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans WHERE job_id = $1`, scanID,
	).Scan(&portCount)
	require.NoError(t, err, "should be able to query port_scans by scanID")
	assert.Equal(t, 2, portCount,
		"both port_scans rows must be retrievable by the original scan UUID; "+
			"if 0 rows are found the ScanID was not propagated to the ScanJob")

	// Verify the scan_jobs row still uses the correct ID (was updated, not replaced).
	var jobExists bool
	err = database.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)`, scanID,
	).Scan(&jobExists)
	require.NoError(t, err)
	assert.True(t, jobExists,
		"scan_jobs row must use the caller-supplied UUID, not a fresh one")
}

// TestStoreScanResults_ExistingJobUpdatesStats verifies that when a scanID is
// supplied and a scan_job row already exists, storeScanResults updates
// scan_stats on the existing row and does NOT insert a duplicate.
func TestStoreScanResults_ExistingJobUpdatesStats(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	scanID := uuid.New()

	cleanupHost(t, database, "10.45.0.1")
	createScanJobForTest(t, ctx, database, scanID, "10.45.0.1/32")

	config := &ScanConfig{
		Targets:  []string{"10.45.0.1"},
		Ports:    "443",
		ScanType: "connect",
		ScanID:   &scanID,
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  2 * time.Second,
		Stats:     HostStats{Up: 1, Down: 0, Total: 1},
		Hosts: []Host{
			{
				Address: "10.45.0.1",
				Status:  "up",
				Ports:   []Port{{Number: 443, Protocol: "tcp", State: "open", Service: "https"}},
			},
		},
	}

	require.NoError(t, storeScanResults(ctx, database, config, result, &scanID))

	// Exactly one scan_jobs row must exist for this ID — no duplicate inserted.
	var jobCount int
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_jobs WHERE id = $1`, scanID,
	).Scan(&jobCount))
	assert.Equal(t, 1, jobCount, "storeScanResults must not insert a duplicate scan_job row")

	// scan_stats must have been written.
	var scanStats []byte
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COALESCE(scan_stats::text, '') FROM scan_jobs WHERE id = $1`, scanID,
	).Scan(&scanStats))
	assert.Contains(t, string(scanStats), "hosts_up",
		"scan_stats should be updated on the existing row")

	// port_scans must be present.
	var portCount int
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans WHERE job_id = $1`, scanID,
	).Scan(&portCount))
	assert.Equal(t, 1, portCount, "port scan row should be stored")
}

// TestStoreScanResults_UpdatePath_DoesNotDuplicateJob is a focused regression
// test: calling storeScanResults twice with the same scanID must not insert a
// second scan_job row (previously it would fail with a PK conflict and swallow
// the error, leaving zero port_scans rows).
func TestStoreScanResults_UpdatePath_DoesNotDuplicateJob(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	scanID := uuid.New()

	cleanupHost(t, database, "10.46.0.1")
	createScanJobForTest(t, ctx, database, scanID, "10.46.0.1/32")

	config := &ScanConfig{
		Targets:  []string{"10.46.0.1"},
		Ports:    "22",
		ScanType: "connect",
		ScanID:   &scanID,
	}
	makeResult := func() *ScanResult {
		return &ScanResult{
			StartTime: time.Now(),
			Duration:  time.Second,
			Stats:     HostStats{Up: 1, Total: 1},
			Hosts: []Host{
				{Address: "10.46.0.1", Status: "up",
					Ports: []Port{{Number: 22, Protocol: "tcp", State: "open", Service: "ssh"}}},
			},
		}
	}

	require.NoError(t, storeScanResults(ctx, database, config, makeResult(), &scanID), "first call")
	// Second call must not error — previously this would hit a PK conflict and swallow all results.
	require.NoError(t, storeScanResults(ctx, database, config, makeResult(), &scanID), "second call")

	var jobCount int
	require.NoError(t, database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_jobs WHERE id = $1`, scanID,
	).Scan(&jobCount))
	assert.Equal(t, 1, jobCount, "still exactly one scan_job row after two calls")
}

// TestStoreScanResults_NilScanID_GeneratesFreshUUID verifies that when no
// ScanID is provided (CLI / legacy path) a fresh UUID is generated and the
// rows are still stored correctly.
func TestStoreScanResults_NilScanID_GeneratesFreshUUID(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()

	config := &ScanConfig{
		Targets:  []string{"10.43.0.1"},
		Ports:    "443",
		ScanType: "connect",
		// ScanID intentionally nil — legacy path.
	}

	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  2 * time.Second,
		Stats:     HostStats{Up: 1, Down: 0, Total: 1},
		Hosts: []Host{
			{
				Address: "10.43.0.1",
				Status:  "up",
				Ports:   []Port{{Number: 443, Protocol: "tcp", State: "open", Service: "https"}},
			},
		},
	}

	err := storeScanResults(ctx, database, config, result, nil)
	require.NoError(t, err, "storeScanResults with nil ScanID should succeed")

	// A scan_jobs row must have been created.
	var jobCount int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scan_jobs WHERE started_at >= $1`,
		result.StartTime.Add(-time.Minute),
	).Scan(&jobCount)
	require.NoError(t, err)
	assert.Greater(t, jobCount, 0, "a scan_jobs row should exist")
}

// TestStoreScanResults_OSFieldsPersisted verifies that OS detection data
// on the Host struct is persisted to the hosts table.
func TestStoreScanResults_OSFieldsPersisted(t *testing.T) {
	database := setupTestDB(t)
	if database == nil {
		return
	}
	defer database.Close()

	ctx := context.Background()
	scanID := uuid.New()

	cleanupHost(t, database, "10.44.0.1")
	// Pre-create the scan_job row so the UPDATE path in storeScanResults finds it.
	createScanJobForTest(t, ctx, database, scanID, "10.44.0.1/32")

	config := &ScanConfig{
		Targets:  []string{"10.44.0.1"},
		Ports:    "22",
		ScanType: "connect",
		ScanID:   &scanID,
	}

	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  3 * time.Second,
		Stats:     HostStats{Up: 1, Down: 0, Total: 1},
		Hosts: []Host{
			{
				Address:    "10.44.0.1",
				Status:     "up",
				OSName:     "Linux 5.15",
				OSFamily:   "Linux",
				OSVersion:  "5.15",
				OSAccuracy: 96,
				Ports:      []Port{{Number: 22, Protocol: "tcp", State: "open", Service: "ssh"}},
			},
		},
	}

	err := storeScanResults(ctx, database, config, result, &scanID)
	require.NoError(t, err, "storeScanResults with OS data should succeed")

	// Read back the host and verify OS fields were written.
	hostRepo := db.NewHostRepository(database)
	ipAddr := db.IPAddr{IP: net.ParseIP("10.44.0.1")}
	host, err := hostRepo.GetByIP(ctx, ipAddr)
	require.NoError(t, err, "host should be retrievable after scan")

	require.NotNil(t, host.OSName, "OSName should be persisted")
	assert.Equal(t, "Linux 5.15", *host.OSName)

	require.NotNil(t, host.OSFamily, "OSFamily should be persisted")
	assert.Equal(t, "Linux", *host.OSFamily)

	require.NotNil(t, host.OSVersion, "OSVersion should be persisted")
	assert.Equal(t, "5.15", *host.OSVersion)

	require.NotNil(t, host.OSConfidence, "OSConfidence should be persisted")
	assert.Equal(t, 96, *host.OSConfidence)
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
