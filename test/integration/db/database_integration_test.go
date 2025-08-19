//go:build integration

package db_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/test/helpers"
)

// DatabaseIntegrationTestSuite tests database integration functionality.
type DatabaseIntegrationTestSuite struct {
	suite.Suite
	database      *db.DB
	testStartTime time.Time
	createdIDs    []uuid.UUID // Track created entities for cleanup
}

// SetupSuite initializes the test suite.
func (suite *DatabaseIntegrationTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping database integration tests in short mode")
	}

	suite.testStartTime = time.Now()
	suite.setupDatabase()
}

// TearDownSuite cleans up after all tests.
func (suite *DatabaseIntegrationTestSuite) TearDownSuite() {
	if suite.database != nil {
		suite.cleanupTestData()
		suite.database.Close()
	}
}

// SetupTest prepares for each individual test.
func (suite *DatabaseIntegrationTestSuite) SetupTest() {
	suite.createdIDs = suite.createdIDs[:0] // Reset slice
}

// TearDownTest cleans up after each test.
func (suite *DatabaseIntegrationTestSuite) TearDownTest() {
	suite.cleanupTestData()
}

// setupDatabase creates a test database connection.
func (suite *DatabaseIntegrationTestSuite) setupDatabase() {
	testConfig, err := helpers.GetAvailableDatabase()
	require.NoError(suite.T(), err, "Failed to get available test database")

	dbConfig := &db.Config{
		Host:            testConfig.Host,
		Port:            testConfig.Port,
		Database:        testConfig.Database,
		Username:        testConfig.Username,
		Password:        testConfig.Password,
		SSLMode:         testConfig.SSLMode,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suite.database, err = db.ConnectAndMigrate(ctx, dbConfig)
	require.NoError(suite.T(), err, "Failed to connect to test database")

	// Verify connection is working
	err = suite.database.PingContext(ctx)
	require.NoError(suite.T(), err, "Failed to ping test database")
}

// cleanupTestData removes all test data created during tests.
func (suite *DatabaseIntegrationTestSuite) cleanupTestData() {
	if suite.database == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Clean up in dependency order
	cleanupQueries := []string{
		"DELETE FROM port_scans WHERE scanned_at >= $1",
		"DELETE FROM scan_jobs WHERE created_at >= $1",
		"DELETE FROM hosts WHERE first_seen >= $1",
		"DELETE FROM scan_targets WHERE created_at >= $1",
		"DELETE FROM networks WHERE created_at >= $1",
	}

	for _, query := range cleanupQueries {
		_, err := suite.database.ExecContext(ctx, query, suite.testStartTime)
		if err != nil {
			suite.T().Logf("Warning: cleanup query failed: %s - %v", query, err)
		}
	}
}

// TestDatabaseConnection tests basic database connectivity and configuration.
func (suite *DatabaseIntegrationTestSuite) TestDatabaseConnection() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test ping
	err := suite.database.PingContext(ctx)
	assert.NoError(suite.T(), err, "Database ping should succeed")

	// Test query execution
	var result int
	err = suite.database.GetContext(ctx, &result, "SELECT 1")
	assert.NoError(suite.T(), err, "Simple query should succeed")
	assert.Equal(suite.T(), 1, result, "Query result should be 1")

	// Test connection stats
	stats := suite.database.Stats()
	assert.True(suite.T(), stats.MaxOpenConnections > 0, "Max open connections should be configured")
	assert.True(suite.T(), stats.OpenConnections >= 0, "Open connections should be non-negative")
}

// TestScanTargetRepository tests scan target CRUD operations.
func (suite *DatabaseIntegrationTestSuite) TestScanTargetRepository() {
	repo := db.NewScanTargetRepository(suite.database)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test Create
	network := db.NetworkAddr{}
	_, ipnet, err := net.ParseCIDR("192.168.1.0/24")
	require.NoError(suite.T(), err, "Parse CIDR should succeed")
	network.IPNet = *ipnet

	description := "Test network for integration testing"
	target := &db.ScanTarget{
		Name:                "Test Target",
		Network:             network,
		Description:         &description,
		ScanIntervalSeconds: 3600,
		ScanPorts:           "22,80,443",
		ScanType:            "connect",
		Enabled:             true,
	}

	err = repo.Create(ctx, target)
	require.NoError(suite.T(), err, "Create should succeed")
	require.NotEqual(suite.T(), uuid.Nil, target.ID, "ID should be generated")
	require.False(suite.T(), target.CreatedAt.IsZero(), "CreatedAt should be set")
	suite.createdIDs = append(suite.createdIDs, target.ID)

	// Test GetByID
	retrieved, err := repo.GetByID(ctx, target.ID)
	require.NoError(suite.T(), err, "GetByID should succeed")
	assert.Equal(suite.T(), target.Name, retrieved.Name)
	assert.Equal(suite.T(), target.Network, retrieved.Network)
	assert.Equal(suite.T(), target.Enabled, retrieved.Enabled)

	// Test GetByID with non-existent ID
	nonExistentID := uuid.New()
	_, err = repo.GetByID(ctx, nonExistentID)
	assert.Error(suite.T(), err, "GetByID with non-existent ID should fail")

	// Test Update
	updatedDesc := "Updated description"
	retrieved.Description = &updatedDesc
	retrieved.Enabled = false
	err = repo.Update(ctx, retrieved)
	require.NoError(suite.T(), err, "Update should succeed")

	// Verify update
	updated, err := repo.GetByID(ctx, target.ID)
	require.NoError(suite.T(), err, "GetByID after update should succeed")
	assert.Equal(suite.T(), "Updated description", *updated.Description)
	assert.False(suite.T(), updated.Enabled)
	assert.True(suite.T(), updated.UpdatedAt.After(updated.CreatedAt), "UpdatedAt should be after CreatedAt")

	// Test GetAll
	targets, err := repo.GetAll(ctx)
	require.NoError(suite.T(), err, "GetAll should succeed")
	assert.True(suite.T(), len(targets) >= 1, "Should have at least one target")

	// Test GetEnabled (should be empty since we disabled our target)
	enabledTargets, err := repo.GetEnabled(ctx)
	require.NoError(suite.T(), err, "GetEnabled should succeed")

	// Check if our disabled target is not in enabled targets
	found := false
	for _, t := range enabledTargets {
		if t.ID == target.ID {
			found = true
			break
		}
	}
	assert.False(suite.T(), found, "Disabled target should not be in enabled targets")

	// Test Delete
	err = repo.Delete(ctx, target.ID)
	require.NoError(suite.T(), err, "Delete should succeed")

	// Verify deletion
	_, err = repo.GetByID(ctx, target.ID)
	assert.Error(suite.T(), err, "GetByID after delete should fail")

	// Test Delete with non-existent ID
	err = repo.Delete(ctx, nonExistentID)
	assert.Error(suite.T(), err, "Delete with non-existent ID should fail")
}

// TestScanJobRepository tests scan job CRUD operations.
func (suite *DatabaseIntegrationTestSuite) TestScanJobRepository() {
	repo := db.NewScanJobRepository(suite.database)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First create a scan target for the job
	targetRepo := db.NewScanTargetRepository(suite.database)
	network2 := db.NetworkAddr{}
	_, ipnet2, err := net.ParseCIDR("192.168.2.0/24")
	require.NoError(suite.T(), err, "Parse CIDR should succeed")
	network2.IPNet = *ipnet2

	targetDesc := "Target for job testing"
	target := &db.ScanTarget{
		Name:                "Test Target for Job",
		Network:             network2,
		Description:         &targetDesc,
		ScanIntervalSeconds: 3600,
		ScanPorts:           "80,443",
		ScanType:            "connect",
		Enabled:             true,
	}
	err = targetRepo.Create(ctx, target)
	require.NoError(suite.T(), err, "Target creation should succeed")
	suite.createdIDs = append(suite.createdIDs, target.ID)

	// Test Create scan job
	job := &db.ScanJob{
		TargetID:  target.ID,
		Status:    "pending",
		ScanStats: db.JSONB(`{"ports": "80,443", "timeout": 30}`),
	}

	err = repo.Create(ctx, job)
	require.NoError(suite.T(), err, "Job creation should succeed")
	require.NotEqual(suite.T(), uuid.Nil, job.ID, "Job ID should be generated")
	suite.createdIDs = append(suite.createdIDs, job.ID)

	// Test GetByID
	retrieved, err := repo.GetByID(ctx, job.ID)
	require.NoError(suite.T(), err, "GetByID should succeed")
	assert.Equal(suite.T(), job.TargetID, retrieved.TargetID)
	assert.Equal(suite.T(), job.Status, retrieved.Status)

	// Test Update status to running
	err = repo.UpdateStatus(ctx, retrieved.ID, "running", nil)
	require.NoError(suite.T(), err, "UpdateStatus should succeed")

	// Verify status update
	updated, err := repo.GetByID(ctx, job.ID)
	require.NoError(suite.T(), err, "GetByID after status update should succeed")
	assert.Equal(suite.T(), "running", updated.Status)
	assert.NotNil(suite.T(), updated.StartedAt, "StartedAt should be set")

	// Test Update status to completed
	err = repo.UpdateStatus(ctx, job.ID, "completed", nil)
	require.NoError(suite.T(), err, "UpdateStatus to completed should succeed")

	// Verify completion
	completed, err := repo.GetByID(ctx, job.ID)
	require.NoError(suite.T(), err, "GetByID after completion should succeed")
	assert.Equal(suite.T(), "completed", completed.Status)
	assert.NotNil(suite.T(), completed.CompletedAt, "CompletedAt should be set")
}

// TestDatabaseHostOperations tests host-related database operations.
func (suite *DatabaseIntegrationTestSuite) TestDatabaseHostOperations() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test CreateHost
	hostData := map[string]interface{}{
		"ip_address": "192.168.1.100",
		"hostname":   "test.example.com",
		"status":     "up",
		"os_family":  "linux",
	}

	host, err := suite.database.CreateHost(ctx, hostData)
	require.NoError(suite.T(), err, "CreateHost should succeed")
	require.NotEqual(suite.T(), uuid.Nil, host.ID, "Host ID should be generated")
	suite.createdIDs = append(suite.createdIDs, host.ID)

	// Test GetHost
	retrieved, err := suite.database.GetHost(ctx, host.ID)
	require.NoError(suite.T(), err, "GetHost should succeed")
	assert.Equal(suite.T(), "192.168.1.100", retrieved.IPAddress.String())
	assert.Equal(suite.T(), "test.example.com", *retrieved.Hostname)

	// Test UpdateHost
	updateData := map[string]interface{}{
		"status": "down",
	}
	updated, err := suite.database.UpdateHost(ctx, host.ID, updateData)
	require.NoError(suite.T(), err, "UpdateHost should succeed")
	assert.Equal(suite.T(), "down", updated.Status)

	// Test ListHosts
	hosts, total, err := suite.database.ListHosts(ctx, db.HostFilters{}, 0, 100)
	require.NoError(suite.T(), err, "ListHosts should succeed")
	assert.True(suite.T(), len(hosts) >= 1, "Should have at least one host")
	assert.True(suite.T(), total >= 1, "Total count should be at least 1")
}

// TestDatabaseErrors tests error handling scenarios.
func (suite *DatabaseIntegrationTestSuite) TestDatabaseErrors() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test invalid SQL
	_, err := suite.database.ExecContext(ctx, "INVALID SQL STATEMENT")
	assert.Error(suite.T(), err, "Invalid SQL should return error")

	// Test transaction rollback
	tx, err := suite.database.BeginTxx(ctx, nil)
	require.NoError(suite.T(), err, "Begin transaction should succeed")

	_, insertErr := tx.ExecContext(ctx, "INSERT INTO hosts (id, ip_address) VALUES ($1, $2)", uuid.New(), "invalid-ip")
	// This might succeed in PostgreSQL depending on validation, so let's force a rollback
	_ = insertErr // We don't care about the insert result, just testing rollback
	err = tx.Rollback()
	assert.NoError(suite.T(), err, "Rollback should succeed")

	// Test context cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = suite.database.ExecContext(cancelCtx, "SELECT 1")
	assert.Error(suite.T(), err, "Canceled context should return error")
}

// TestDatabaseConnectionPool tests connection pool behavior.
func (suite *DatabaseIntegrationTestSuite) TestDatabaseConnectionPool() {
	// Test connection pool stats
	stats := suite.database.Stats()

	assert.True(suite.T(), stats.MaxOpenConnections > 0, "Should have max open connections configured")
	assert.True(suite.T(), stats.OpenConnections >= 0, "Open connections should be non-negative")
	assert.True(suite.T(), stats.InUse >= 0, "InUse connections should be non-negative")
	assert.True(suite.T(), stats.Idle >= 0, "Idle connections should be non-negative")

	// Test concurrent connections
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const numGoroutines = 5
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			var result int
			err := suite.database.GetContext(ctx, &result, "SELECT $1", id)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(suite.T(), err, "Concurrent query should succeed")
	}
}

// TestTransactionOperations tests database transaction handling.
func (suite *DatabaseIntegrationTestSuite) TestTransactionOperations() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test successful transaction
	tx, err := suite.database.BeginTxx(ctx, nil)
	require.NoError(suite.T(), err, "Begin transaction should succeed")

	network3 := db.NetworkAddr{}
	_, ipnet3, err := net.ParseCIDR("10.0.0.0/24")
	require.NoError(suite.T(), err, "Parse CIDR should succeed")
	network3.IPNet = *ipnet3

	txDesc := "Target for transaction testing"
	target := &db.ScanTarget{
		Name:                "Transaction Test Target",
		Network:             network3,
		Description:         &txDesc,
		ScanIntervalSeconds: 7200,
		ScanPorts:           "22,80",
		ScanType:            "connect",
		Enabled:             true,
	}

	// Insert within transaction
	query := `
		INSERT INTO scan_targets (id, name, network, description, scan_interval_seconds, scan_ports, scan_type, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	target.ID = uuid.New()
	err = tx.QueryRowContext(ctx, query,
		target.ID, target.Name, target.Network.String(), target.Description,
		target.ScanIntervalSeconds, target.ScanPorts, target.ScanType, target.Enabled,
	).Scan(&target.CreatedAt, &target.UpdatedAt)
	require.NoError(suite.T(), err, "Insert in transaction should succeed")

	// Commit transaction
	err = tx.Commit()
	require.NoError(suite.T(), err, "Commit should succeed")
	suite.createdIDs = append(suite.createdIDs, target.ID)

	// Verify data exists after commit
	var count int
	err = suite.database.GetContext(ctx, &count, "SELECT COUNT(*) FROM scan_targets WHERE id = $1", target.ID)
	require.NoError(suite.T(), err, "Count query should succeed")
	assert.Equal(suite.T(), 1, count, "Target should exist after commit")

	// Test transaction rollback
	tx2, err := suite.database.BeginTxx(ctx, nil)
	require.NoError(suite.T(), err, "Begin second transaction should succeed")

	target2ID := uuid.New()
	rollbackDesc := "Will be rolled back"
	_, err = tx2.ExecContext(ctx, query,
		target2ID, "Rollback Test", "10.1.0.0/24", &rollbackDesc,
		3600, "80", "connect", true,
	)
	require.NoError(suite.T(), err, "Insert in second transaction should succeed")

	// Rollback transaction
	err = tx2.Rollback()
	require.NoError(suite.T(), err, "Rollback should succeed")

	// Verify data doesn't exist after rollback
	err = suite.database.GetContext(ctx, &count, "SELECT COUNT(*) FROM scan_targets WHERE id = $1", target2ID)
	require.NoError(suite.T(), err, "Count query after rollback should succeed")
	assert.Equal(suite.T(), 0, count, "Target should not exist after rollback")
}

// TestConfigDefaults tests database configuration defaults and validation.
func (suite *DatabaseIntegrationTestSuite) TestConfigDefaults() {
	cfg := db.DefaultConfig()

	assert.Equal(suite.T(), "localhost", cfg.Host)
	assert.Equal(suite.T(), 5432, cfg.Port)
	assert.Equal(suite.T(), "disable", cfg.SSLMode)
	assert.Equal(suite.T(), 25, cfg.MaxOpenConns)
	assert.Equal(suite.T(), 5, cfg.MaxIdleConns)
	assert.True(suite.T(), cfg.ConnMaxLifetime > 0)
	assert.True(suite.T(), cfg.ConnMaxIdleTime > 0)
}

// TestDatabaseIntegration runs the database integration test suite.
func TestDatabaseIntegration(t *testing.T) {
	suite.Run(t, new(DatabaseIntegrationTestSuite))
}
