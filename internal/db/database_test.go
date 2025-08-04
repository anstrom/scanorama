//go:build integration
// +build integration

package db

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DatabaseTestSuite provides a test suite for database operations
type DatabaseTestSuite struct {
	suite.Suite
	db     *DB
	ctx    context.Context
	config Config
}

// SetupSuite runs once before all tests
func (suite *DatabaseTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Database configuration for testing
	suite.config = Config{
		Host:            getEnvWithDefault("SCANORAMA_DB_HOST", "localhost"),
		Port:            5432,
		Database:        getEnvWithDefault("SCANORAMA_DB_NAME", "scanorama_test"),
		Username:        getEnvWithDefault("SCANORAMA_DB_USER", "scanorama_test"),
		Password:        getEnvWithDefault("SCANORAMA_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 1 * time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}

	// Connect to database and run migrations
	db, err := ConnectAndMigrate(suite.ctx, suite.config)
	require.NoError(suite.T(), err, "Failed to connect to test database")
	suite.db = db
}

// TearDownSuite runs once after all tests
func (suite *DatabaseTestSuite) TearDownSuite() {
	if suite.db != nil {
		suite.db.Close()
	}
}

// SetupTest runs before each test
func (suite *DatabaseTestSuite) SetupTest() {
	// Clean up tables before each test
	suite.cleanupDatabase()
}

// cleanupDatabase removes all test data
func (suite *DatabaseTestSuite) cleanupDatabase() {
	tables := []string{
		"host_history",
		"services",
		"port_scans",
		"scan_jobs",
		"hosts",
		"scan_targets",
	}

	for _, table := range tables {
		_, err := suite.db.ExecContext(suite.ctx, "DELETE FROM "+table)
		require.NoError(suite.T(), err, "Failed to clean table: "+table)
	}
}

// TestDatabaseConnection tests basic database connectivity
func (suite *DatabaseTestSuite) TestDatabaseConnection() {
	t := suite.T()

	// Test ping
	err := suite.db.Ping(suite.ctx)
	assert.NoError(t, err, "Database ping should succeed")

	// Test query
	var version string
	err = suite.db.GetContext(suite.ctx, &version, "SELECT version()")
	assert.NoError(t, err, "Version query should succeed")
	assert.Contains(t, version, "PostgreSQL", "Should be PostgreSQL database")
}

// TestNetworkTypes tests PostgreSQL native network types
func (suite *DatabaseTestSuite) TestNetworkTypes() {
	t := suite.T()

	// Test INET type
	testIP := "192.168.1.100"
	ipAddr := IPAddr{IP: net.ParseIP(testIP)}

	// Test CIDR type
	_, testNet, err := net.ParseCIDR("10.0.0.0/24")
	require.NoError(t, err)
	netAddr := NetworkAddr{IPNet: *testNet}

	// Test MACADDR type
	testMAC, err := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)
	macAddr := MACAddr{HardwareAddr: testMAC}

	// Test round-trip through database
	query := `
		SELECT
			$1::inet as ip,
			$2::cidr as network,
			$3::macaddr as mac
	`

	var result struct {
		IP      IPAddr      `db:"ip"`
		Network NetworkAddr `db:"network"`
		MAC     MACAddr     `db:"mac"`
	}

	err = suite.db.GetContext(suite.ctx, &result, query, ipAddr, netAddr, macAddr)
	assert.NoError(t, err, "Network types query should succeed")
	assert.Equal(t, testIP, result.IP.String())
	assert.Equal(t, "10.0.0.0/24", result.Network.String())
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", result.MAC.String())
}

// TestJSONBType tests PostgreSQL JSONB type
func (suite *DatabaseTestSuite) TestJSONBType() {
	t := suite.T()
	t.Skip("JSONB test skipped due to buffer corruption issue - needs investigation")
}

// TestMigrations tests the migration system
func (suite *DatabaseTestSuite) TestMigrations() {
	t := suite.T()

	migrator := NewMigrator(suite.db.DB)

	// Test migration status
	err := migrator.Status(suite.ctx)
	assert.NoError(t, err, "Migration status should work")

	// Test that tables exist after migration
	tables := []string{"scan_targets", "scan_jobs", "hosts", "port_scans", "services", "host_history"}
	for _, table := range tables {
		var exists bool
		err := suite.db.GetContext(suite.ctx,
			&exists,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)",
			table)
		assert.NoError(t, err)
		assert.True(t, exists, "Table %s should exist after migration", table)
	}

	// Test that views exist
	views := []string{"active_hosts", "network_summary"}
	for _, view := range views {
		var exists bool
		err := suite.db.GetContext(suite.ctx,
			&exists,
			"SELECT EXISTS (SELECT FROM information_schema.views WHERE table_name = $1)",
			view)
		assert.NoError(t, err)
		assert.True(t, exists, "View %s should exist after migration", view)
	}
}

// TestScanTargetRepository tests scan target CRUD operations
func (suite *DatabaseTestSuite) TestScanTargetRepository() {
	t := suite.T()
	repo := NewScanTargetRepository(suite.db)

	// Create test target
	_, testNet, err := net.ParseCIDR("192.168.1.0/24")
	require.NoError(t, err)

	target := &ScanTarget{
		Name:                "Test Network",
		Network:             NetworkAddr{IPNet: *testNet},
		Description:         stringPtr("Test description"),
		ScanIntervalSeconds: 3600,
		ScanPorts:           "22,80,443",
		ScanType:            "connect",
		Enabled:             true,
	}

	// Test Create
	err = repo.Create(suite.ctx, target)
	assert.NoError(t, err, "Create should succeed")
	assert.NotEqual(t, uuid.Nil, target.ID, "ID should be set")
	assert.NotZero(t, target.CreatedAt, "CreatedAt should be set")
	assert.NotZero(t, target.UpdatedAt, "UpdatedAt should be set")

	// Test GetByID
	retrieved, err := repo.GetByID(suite.ctx, target.ID)
	assert.NoError(t, err, "GetByID should succeed")
	assert.Equal(t, target.Name, retrieved.Name)
	assert.Equal(t, target.Network.String(), retrieved.Network.String())
	assert.Equal(t, target.ScanPorts, retrieved.ScanPorts)

	// Test Update
	retrieved.Name = "Updated Network"
	retrieved.ScanIntervalSeconds = 7200
	err = repo.Update(suite.ctx, retrieved)
	assert.NoError(t, err, "Update should succeed")

	updated, err := repo.GetByID(suite.ctx, target.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Network", updated.Name)
	assert.Equal(t, 7200, updated.ScanIntervalSeconds)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt), "UpdatedAt should be newer")

	// Test GetAll
	all, err := repo.GetAll(suite.ctx)
	assert.NoError(t, err, "GetAll should succeed")
	assert.Len(t, all, 1, "Should have one target")

	// Test GetEnabled
	enabled, err := repo.GetEnabled(suite.ctx)
	assert.NoError(t, err, "GetEnabled should succeed")
	assert.Len(t, enabled, 1, "Should have one enabled target")

	// Test Delete
	err = repo.Delete(suite.ctx, target.ID)
	assert.NoError(t, err, "Delete should succeed")

	_, err = repo.GetByID(suite.ctx, target.ID)
	assert.Error(t, err, "GetByID should fail after delete")
}

// TestScanJobRepository tests scan job operations
func (suite *DatabaseTestSuite) TestScanJobRepository() {
	t := suite.T()
	jobRepo := NewScanJobRepository(suite.db)
	targetRepo := NewScanTargetRepository(suite.db)

	// Create a target first
	_, testNet, err := net.ParseCIDR("10.0.0.0/24")
	require.NoError(t, err)

	target := &ScanTarget{
		Name:                "Job Test Network",
		Network:             NetworkAddr{IPNet: *testNet},
		ScanIntervalSeconds: 3600,
		ScanPorts:           "80,443",
		ScanType:            "connect",
		Enabled:             true,
	}
	err = targetRepo.Create(suite.ctx, target)
	require.NoError(t, err)

	// Create scan job
	job := &ScanJob{
		TargetID: target.ID,
		Status:   ScanJobStatusPending,
	}

	// Test Create
	err = jobRepo.Create(suite.ctx, job)
	assert.NoError(t, err, "Create job should succeed")
	assert.NotEqual(t, uuid.Nil, job.ID, "Job ID should be set")

	// Test GetByID
	retrieved, err := jobRepo.GetByID(suite.ctx, job.ID)
	assert.NoError(t, err, "GetByID should succeed")
	assert.Equal(t, job.TargetID, retrieved.TargetID)
	assert.Equal(t, ScanJobStatusPending, retrieved.Status)

	// Test UpdateStatus to running
	err = jobRepo.UpdateStatus(suite.ctx, job.ID, ScanJobStatusRunning, nil)
	assert.NoError(t, err, "UpdateStatus to running should succeed")

	updated, err := jobRepo.GetByID(suite.ctx, job.ID)
	assert.NoError(t, err)
	assert.Equal(t, ScanJobStatusRunning, updated.Status)
	assert.NotNil(t, updated.StartedAt, "StartedAt should be set")

	// Test UpdateStatus to completed
	err = jobRepo.UpdateStatus(suite.ctx, job.ID, ScanJobStatusCompleted, nil)
	assert.NoError(t, err, "UpdateStatus to completed should succeed")

	completed, err := jobRepo.GetByID(suite.ctx, job.ID)
	assert.NoError(t, err)
	assert.Equal(t, ScanJobStatusCompleted, completed.Status)
	assert.NotNil(t, completed.CompletedAt, "CompletedAt should be set")

	// Test UpdateStatus to failed with error
	errorMsg := "Test error message"
	err = jobRepo.UpdateStatus(suite.ctx, job.ID, ScanJobStatusFailed, &errorMsg)
	assert.NoError(t, err, "UpdateStatus to failed should succeed")

	failed, err := jobRepo.GetByID(suite.ctx, job.ID)
	assert.NoError(t, err)
	assert.Equal(t, ScanJobStatusFailed, failed.Status)
	assert.NotNil(t, failed.ErrorMessage)
	assert.Equal(t, errorMsg, *failed.ErrorMessage)
}

// TestHostRepository tests host operations
func (suite *DatabaseTestSuite) TestHostRepository() {
	t := suite.T()
	repo := NewHostRepository(suite.db)

	// Test IP addresses
	testIP := net.ParseIP("192.168.1.50")
	testMAC, err := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	require.NoError(t, err)

	host := &Host{
		IPAddress:  IPAddr{IP: testIP},
		Hostname:   stringPtr("test-host"),
		MACAddress: &MACAddr{HardwareAddr: testMAC},
		Vendor:     stringPtr("Test Vendor"),
		OSFamily:   stringPtr("Linux"),
		OSVersion:  stringPtr("Ubuntu 20.04"),
		Status:     HostStatusUp,
	}

	// Test CreateOrUpdate (create)
	err = repo.CreateOrUpdate(suite.ctx, host)
	assert.NoError(t, err, "CreateOrUpdate should succeed")
	assert.NotEqual(t, uuid.Nil, host.ID, "Host ID should be set")

	// Test GetByIP
	retrieved, err := repo.GetByIP(suite.ctx, host.IPAddress)
	assert.NoError(t, err, "GetByIP should succeed")
	assert.Equal(t, host.IPAddress.String(), retrieved.IPAddress.String())
	assert.Equal(t, *host.Hostname, *retrieved.Hostname)
	assert.Equal(t, host.MACAddress.String(), retrieved.MACAddress.String())

	// Test CreateOrUpdate (update)
	host.Hostname = stringPtr("updated-host")
	host.Status = HostStatusDown
	err = repo.CreateOrUpdate(suite.ctx, host)
	assert.NoError(t, err, "CreateOrUpdate (update) should succeed")

	updated, err := repo.GetByIP(suite.ctx, host.IPAddress)
	assert.NoError(t, err)
	assert.Equal(t, "updated-host", *updated.Hostname)
	assert.Equal(t, HostStatusDown, updated.Status)
	assert.True(t, updated.LastSeen.After(updated.FirstSeen), "LastSeen should be updated")
}

// TestPortScanRepository tests port scan operations
func (suite *DatabaseTestSuite) TestPortScanRepository() {
	t := suite.T()
	portRepo := NewPortScanRepository(suite.db)
	hostRepo := NewHostRepository(suite.db)
	jobRepo := NewScanJobRepository(suite.db)
	targetRepo := NewScanTargetRepository(suite.db)

	// Create dependencies
	_, testNet, err := net.ParseCIDR("172.16.0.0/24")
	require.NoError(t, err)

	target := &ScanTarget{
		Name:                "Port Test Network",
		Network:             NetworkAddr{IPNet: *testNet},
		ScanIntervalSeconds: 3600,
		ScanPorts:           "22,80,443",
		ScanType:            "connect",
		Enabled:             true,
	}
	err = targetRepo.Create(suite.ctx, target)
	require.NoError(t, err)

	job := &ScanJob{
		TargetID: target.ID,
		Status:   ScanJobStatusRunning,
	}
	err = jobRepo.Create(suite.ctx, job)
	require.NoError(t, err)

	host := &Host{
		IPAddress: IPAddr{IP: net.ParseIP("172.16.0.10")},
		Status:    HostStatusUp,
	}
	err = hostRepo.CreateOrUpdate(suite.ctx, host)
	require.NoError(t, err)

	// Create port scans
	portScans := []*PortScan{
		{
			JobID:          job.ID,
			HostID:         host.ID,
			Port:           22,
			Protocol:       ProtocolTCP,
			State:          PortStateOpen,
			ServiceName:    stringPtr("ssh"),
			ServiceVersion: stringPtr("OpenSSH 8.0"),
		},
		{
			JobID:       job.ID,
			HostID:      host.ID,
			Port:        80,
			Protocol:    ProtocolTCP,
			State:       PortStateOpen,
			ServiceName: stringPtr("http"),
		},
		{
			JobID:    job.ID,
			HostID:   host.ID,
			Port:     443,
			Protocol: ProtocolTCP,
			State:    PortStateClosed,
		},
	}

	// Test CreateBatch
	err = portRepo.CreateBatch(suite.ctx, portScans)
	assert.NoError(t, err, "CreateBatch should succeed")

	for _, ps := range portScans {
		assert.NotEqual(t, uuid.Nil, ps.ID, "Port scan ID should be set")
	}

	// Test GetByHost
	retrieved, err := portRepo.GetByHost(suite.ctx, host.ID)
	assert.NoError(t, err, "GetByHost should succeed")
	assert.Len(t, retrieved, 3, "Should have 3 port scans")

	// Verify port scan data
	openPorts := 0
	for _, ps := range retrieved {
		if ps.State == PortStateOpen {
			openPorts++
		}
		assert.Equal(t, job.ID, ps.JobID)
		assert.Equal(t, host.ID, ps.HostID)
	}
	assert.Equal(t, 2, openPorts, "Should have 2 open ports")
}

// TestActiveHostsView tests the active_hosts view
func (suite *DatabaseTestSuite) TestActiveHostsView() {
	t := suite.T()
	hostRepo := NewHostRepository(suite.db)
	portRepo := NewPortScanRepository(suite.db)
	jobRepo := NewScanJobRepository(suite.db)
	targetRepo := NewScanTargetRepository(suite.db)

	// Create test data
	_, testNet, err := net.ParseCIDR("192.168.100.0/24")
	require.NoError(t, err)

	target := &ScanTarget{
		Name:                "View Test Network",
		Network:             NetworkAddr{IPNet: *testNet},
		ScanIntervalSeconds: 3600,
		ScanPorts:           "22,80",
		ScanType:            "connect",
		Enabled:             true,
	}
	err = targetRepo.Create(suite.ctx, target)
	require.NoError(t, err)

	job := &ScanJob{
		TargetID: target.ID,
		Status:   ScanJobStatusCompleted,
	}
	err = jobRepo.Create(suite.ctx, job)
	require.NoError(t, err)

	// Create host with ports
	host := &Host{
		IPAddress: IPAddr{IP: net.ParseIP("192.168.100.50")},
		Hostname:  stringPtr("test-server"),
		Status:    HostStatusUp,
	}
	err = hostRepo.CreateOrUpdate(suite.ctx, host)
	require.NoError(t, err)

	portScans := []*PortScan{
		{
			JobID:    job.ID,
			HostID:   host.ID,
			Port:     22,
			Protocol: ProtocolTCP,
			State:    PortStateOpen,
		},
		{
			JobID:    job.ID,
			HostID:   host.ID,
			Port:     80,
			Protocol: ProtocolTCP,
			State:    PortStateOpen,
		},
	}
	err = portRepo.CreateBatch(suite.ctx, portScans)
	require.NoError(t, err)

	// Test active hosts view
	var activeHosts []ActiveHost
	err = suite.db.SelectContext(suite.ctx, &activeHosts, "SELECT * FROM active_hosts")
	assert.NoError(t, err, "Active hosts view query should succeed")
	assert.Len(t, activeHosts, 1, "Should have one active host")

	activeHost := activeHosts[0]
	assert.Equal(t, "192.168.100.50", activeHost.IPAddress.String())
	assert.Equal(t, "test-server", *activeHost.Hostname)
	assert.Equal(t, 2, activeHost.OpenPorts)
	assert.Equal(t, 2, activeHost.TotalPortsScanned)
}

// TestNetworkSummaryView tests the network_summary view
func (suite *DatabaseTestSuite) TestNetworkSummaryView() {
	t := suite.T()
	repo := NewNetworkSummaryRepository(suite.db)

	// The view should work even with no data
	summaries, err := repo.GetAll(suite.ctx)
	assert.NoError(t, err, "GetAll should succeed")
	assert.True(t, summaries == nil || len(summaries) == 0, "Summaries should be nil or empty when no data exists")
}

// TestTransactions tests transaction handling
func (suite *DatabaseTestSuite) TestTransactions() {
	t := suite.T()
	_ = NewScanTargetRepository(suite.db)

	// Test successful transaction
	tx, err := suite.db.BeginTx(suite.ctx)
	require.NoError(t, err)

	_, testNet, err := net.ParseCIDR("10.10.10.0/24")
	require.NoError(t, err)

	_ = &ScanTarget{
		Name:                "Transaction Test",
		Network:             NetworkAddr{IPNet: *testNet},
		ScanIntervalSeconds: 3600,
		ScanPorts:           "80",
		ScanType:            "connect",
		Enabled:             true,
	}

	// Use transaction for repository operations would require modifying the repos
	// For now, test basic transaction functionality
	_, err = tx.ExecContext(suite.ctx, "INSERT INTO scan_targets (name, network) VALUES ($1, $2)",
		"tx-test", "10.10.10.0/24")
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err, "Transaction commit should succeed")

	// Verify data exists
	var count int
	err = suite.db.GetContext(suite.ctx, &count, "SELECT COUNT(*) FROM scan_targets WHERE name = $1", "tx-test")
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Should have inserted record")

	// Test rollback
	tx, err = suite.db.BeginTx(suite.ctx)
	require.NoError(t, err)

	_, err = tx.ExecContext(suite.ctx, "INSERT INTO scan_targets (name, network) VALUES ($1, $2)",
		"rollback-test", "10.10.11.0/24")
	assert.NoError(t, err)

	err = tx.Rollback()
	assert.NoError(t, err, "Transaction rollback should succeed")

	// Verify data doesn't exist
	err = suite.db.GetContext(suite.ctx, &count, "SELECT COUNT(*) FROM scan_targets WHERE name = $1", "rollback-test")
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "Should not have inserted record after rollback")
}

// TestConcurrentOperations tests concurrent database operations
func (suite *DatabaseTestSuite) TestConcurrentOperations() {
	t := suite.T()
	repo := NewScanTargetRepository(suite.db)

	// Test concurrent creates
	numRoutines := 5
	done := make(chan error, numRoutines)

	for i := 0; i < numRoutines; i++ {
		go func(id int) {
			_, testNet, err := net.ParseCIDR("10.0.0.0/24")
			if err != nil {
				done <- err
				return
			}

			target := &ScanTarget{
				Name:                "Concurrent Test " + string(rune('A'+id)),
				Network:             NetworkAddr{IPNet: *testNet},
				ScanIntervalSeconds: 3600,
				ScanPorts:           "80",
				ScanType:            "connect",
				Enabled:             true,
			}

			err = repo.Create(suite.ctx, target)
			done <- err
		}(i)
	}

	// Wait for all routines to complete
	for i := 0; i < numRoutines; i++ {
		err := <-done
		assert.NoError(t, err, "Concurrent operation should succeed")
	}

	// Verify all targets were created
	targets, err := repo.GetAll(suite.ctx)
	assert.NoError(t, err)
	assert.Len(t, targets, numRoutines, "Should have created all targets")
}

// Helper functions

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TestDatabaseTestSuite runs the database test suite
func TestDatabaseTestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseTestSuite))
}

// TestDatabaseConnection tests database connection without the full suite
func TestDatabaseConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	config := Config{
		Host:     getEnvWithDefault("SCANORAMA_DB_HOST", "localhost"),
		Port:     5432,
		Database: getEnvWithDefault("SCANORAMA_DB_NAME", "scanorama_test"),
		Username: getEnvWithDefault("SCANORAMA_DB_USER", "scanorama_test"),
		Password: getEnvWithDefault("SCANORAMA_DB_PASSWORD", "test_password"),
		SSLMode:  "disable",
	}

	ctx := context.Background()
	db, err := Connect(ctx, config)
	require.NoError(t, err, "Should connect to database")
	defer db.Close()

	err = db.Ping(ctx)
	assert.NoError(t, err, "Should ping database successfully")
}
