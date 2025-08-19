// Package e2e provides comprehensive end-to-end tests for Scanorama.
// These tests verify complete workflows from API requests through
// the application stack to database persistence and back.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/api"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/test/helpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// E2ETestSuite provides a comprehensive end-to-end testing environment
// that includes a full application stack with database, API server, and services.
type E2ETestSuite struct {
	suite.Suite

	// Application components
	database *db.DB
	server   *httptest.Server
	config   *config.Config
	ctx      context.Context
	cancel   context.CancelFunc

	// Test state
	testStartTime time.Time
	apiKey        string
	baseURL       string
	client        *http.Client

	// Test data tracking
	createdScans    []uuid.UUID
	createdHosts    []uuid.UUID
	createdProfiles []uuid.UUID
	createdJobs     []uuid.UUID
}

// SetupSuite initializes the complete testing environment once for all tests.
func (suite *E2ETestSuite) SetupSuite() {
	suite.testStartTime = time.Now()

	// Skip if running in short mode
	if testing.Short() {
		suite.T().Skip("Skipping E2E tests in short mode")
	}

	// Initialize context
	suite.ctx, suite.cancel = context.WithTimeout(context.Background(), 5*time.Minute)

	// Setup database
	suite.setupDatabase()

	// Setup configuration
	suite.setupConfiguration()

	// Setup API server
	suite.setupAPIServer()

	// Setup HTTP client
	suite.setupHTTPClient()

	// Initialize test data tracking
	suite.createdScans = make([]uuid.UUID, 0)
	suite.createdHosts = make([]uuid.UUID, 0)
	suite.createdProfiles = make([]uuid.UUID, 0)
	suite.createdJobs = make([]uuid.UUID, 0)
}

// TearDownSuite cleans up the testing environment after all tests complete.
func (suite *E2ETestSuite) TearDownSuite() {
	if suite.cancel != nil {
		suite.cancel()
	}

	if suite.server != nil {
		suite.server.Close()
	}

	// Clean up test data
	suite.cleanupTestData()

	if suite.database != nil {
		suite.database.Close()
	}
}

// SetupTest prepares for each individual test.
func (suite *E2ETestSuite) SetupTest() {
	// Clean up any residual test data from previous tests
	suite.cleanupTestData()

	// Reset tracking slices
	suite.createdScans = suite.createdScans[:0]
	suite.createdHosts = suite.createdHosts[:0]
	suite.createdProfiles = suite.createdProfiles[:0]
	suite.createdJobs = suite.createdJobs[:0]
}

// setupDatabase initializes the test database connection and schema.
func (suite *E2ETestSuite) setupDatabase() {
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
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	database, err := db.ConnectAndMigrate(suite.ctx, dbConfig)
	require.NoError(suite.T(), err, "Failed to connect to test database and run migrations")

	suite.database = database
}

// setupConfiguration creates test configuration for the application.
func (suite *E2ETestSuite) setupConfiguration() {
	suite.config = &config.Config{
		API: config.APIConfig{
			Enabled:     true,
			Host:        "127.0.0.1",
			Port:        0, // Let the test server choose the port
			APIKeys:     []string{"test-api-key-e2e"},
			EnableCORS:  true,
			CORSOrigins: []string{"*"},
		},
		Scanning: config.ScanningConfig{
			WorkerPoolSize:         2,
			MaxScanTimeout:         time.Minute,
			DefaultPorts:           "22,80,443",
			DefaultScanType:        "connect",
			MaxConcurrentTargets:   5,
			EnableServiceDetection: false,
			EnableOSDetection:      false,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	}

	suite.apiKey = "test-api-key-e2e"
}

// setupAPIServer initializes the HTTP API server for testing.
func (suite *E2ETestSuite) setupAPIServer() {
	// Create API server with test configuration
	apiServer, err := api.New(suite.config, suite.database)
	require.NoError(suite.T(), err, "Failed to create API server")

	// Create test server
	suite.server = httptest.NewServer(apiServer.GetRouter())
	suite.baseURL = suite.server.URL + "/api/v1"
}

// setupHTTPClient configures the HTTP client for API requests.
func (suite *E2ETestSuite) setupHTTPClient() {
	suite.client = &http.Client{
		Timeout: 30 * time.Second,
	}
}

// cleanupTestData removes all test data created during tests.
func (suite *E2ETestSuite) cleanupTestData() {
	if suite.database == nil {
		return
	}

	// Clean up in reverse dependency order
	cleanupQueries := []string{
		"DELETE FROM port_scans WHERE host_id IN (SELECT id FROM hosts WHERE created_at >= $1)",
		"DELETE FROM scan_jobs WHERE created_at >= $1",
		"DELETE FROM hosts WHERE created_at >= $1",
		"DELETE FROM scan_targets WHERE created_at >= $1",
		"DELETE FROM discovery_jobs WHERE created_at >= $1",
		"DELETE FROM scan_profiles WHERE created_at >= $1",
	}

	for _, query := range cleanupQueries {
		if _, err := suite.database.ExecContext(suite.ctx, query, suite.testStartTime); err != nil {
			suite.T().Logf("Warning: cleanup query failed: %v", err)
		}
	}
}

// makeAPIRequest performs an authenticated API request.
func (suite *E2ETestSuite) makeAPIRequest(method, endpoint string, body interface{}) (*http.Response, error) {
	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(suite.ctx, method, suite.baseURL+endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("X-API-Key", suite.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return suite.client.Do(req)
}

// parseJSONResponse parses a JSON response into the provided structure.
func (suite *E2ETestSuite) parseJSONResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// TestCompleteScanning_E2E tests the complete scanning workflow from API to database.
func (suite *E2ETestSuite) TestCompleteScanning_E2E() {
	// 1. Create a scan profile via API
	profileReq := map[string]interface{}{
		"name":        "E2E Test Profile",
		"description": "Test profile for E2E testing",
		"scan_type":   "connect",
		"ports":       "22,80,443",
		"timeout":     60,
		"enabled":     true,
	}

	resp, err := suite.makeAPIRequest("POST", "/profiles", profileReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var profile map[string]interface{}
	err = suite.parseJSONResponse(resp, &profile)
	require.NoError(suite.T(), err)

	profileID := profile["id"].(string)
	parsedProfileID, err := uuid.Parse(profileID)
	require.NoError(suite.T(), err)
	suite.createdProfiles = append(suite.createdProfiles, parsedProfileID)

	// 2. Verify profile was stored in database
	var dbProfile db.ScanProfile
	query := "SELECT id, name, description, scan_type, ports FROM scan_profiles WHERE id = $1"
	err = suite.database.GetContext(suite.ctx, &dbProfile, query, parsedProfileID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "E2E Test Profile", dbProfile.Name)
	assert.Equal(suite.T(), "connect", dbProfile.ScanType)

	// 3. Create a scan using the profile
	scanReq := map[string]interface{}{
		"targets":    []string{"127.0.0.1"},
		"profile_id": profileID,
		"scan_type":  "connect",
		"ports":      "22",
		"timeout":    30,
	}

	resp, err = suite.makeAPIRequest("POST", "/scans", scanReq)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var scan map[string]interface{}
	err = suite.parseJSONResponse(resp, &scan)
	require.NoError(suite.T(), err)

	scanID := scan["id"].(string)
	parsedScanID, err := uuid.Parse(scanID)
	require.NoError(suite.T(), err)
	suite.createdJobs = append(suite.createdJobs, parsedScanID)

	// 4. Wait for scan completion (with timeout)
	maxWait := 2 * time.Minute
	checkInterval := 5 * time.Second
	deadline := time.Now().Add(maxWait)

	var scanCompleted bool
	for time.Now().Before(deadline) {
		resp, err := suite.makeAPIRequest("GET", "/scans/"+scanID, nil)
		require.NoError(suite.T(), err)

		var scanStatus map[string]interface{}
		err = suite.parseJSONResponse(resp, &scanStatus)
		require.NoError(suite.T(), err)

		status := scanStatus["status"].(string)
		if status == "completed" || status == "failed" {
			scanCompleted = true
			assert.Equal(suite.T(), "completed", status, "Scan should complete successfully")
			break
		}

		time.Sleep(checkInterval)
	}

	require.True(suite.T(), scanCompleted, "Scan should complete within timeout")

	// 5. Verify scan results in database
	var jobCount int
	jobQuery := "SELECT COUNT(*) FROM scan_jobs WHERE id = $1 AND status = 'completed'"
	err = suite.database.QueryRowContext(suite.ctx, jobQuery, parsedScanID).Scan(&jobCount)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, jobCount, "Should have one completed scan job")

	// 6. Verify hosts were discovered and stored
	var hostCount int
	hostQuery := "SELECT COUNT(*) FROM hosts WHERE ip_address = '127.0.0.1' AND last_seen >= $1"
	err = suite.database.QueryRowContext(suite.ctx, hostQuery, suite.testStartTime).Scan(&hostCount)
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), hostCount, 1, "Should have at least one host record")

	// 7. Retrieve scan results via API
	resp, err = suite.makeAPIRequest("GET", "/scans/"+scanID+"/results", nil)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var results map[string]interface{}
	err = suite.parseJSONResponse(resp, &results)
	require.NoError(suite.T(), err)

	hosts := results["hosts"].([]interface{})
	assert.GreaterOrEqual(suite.T(), len(hosts), 1, "Should return at least one host in results")
}

// TestAPIAuthentication_E2E tests authentication and authorization workflows.
func (suite *E2ETestSuite) TestAPIAuthentication_E2E() {
	// Test 1: Request without API key should fail
	req, err := http.NewRequestWithContext(suite.ctx, "GET", suite.baseURL+"/profiles", nil)
	require.NoError(suite.T(), err)

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Test 2: Request with invalid API key should fail
	req, err = http.NewRequestWithContext(suite.ctx, "GET", suite.baseURL+"/profiles", nil)
	require.NoError(suite.T(), err)
	req.Header.Set("X-API-Key", "invalid-key")

	resp, err = suite.client.Do(req)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Test 3: Request with valid API key should succeed
	resp, err = suite.makeAPIRequest("GET", "/profiles", nil)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// TestAPIErrorHandling_E2E tests error handling across the API stack.
func (suite *E2ETestSuite) TestAPIErrorHandling_E2E() {
	// Test 1: Invalid JSON body
	invalidJSON := bytes.NewBufferString("invalid json")
	req, err := http.NewRequestWithContext(suite.ctx, "POST", suite.baseURL+"/profiles", invalidJSON)
	require.NoError(suite.T(), err)
	req.Header.Set("X-API-Key", suite.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Test 2: Missing required fields
	invalidProfile := map[string]interface{}{
		"description": "Missing name field",
	}

	resp, err = suite.makeAPIRequest("POST", "/profiles", invalidProfile)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Test 3: Non-existent resource
	nonExistentID := uuid.New().String()
	resp, err = suite.makeAPIRequest("GET", "/scans/"+nonExistentID, nil)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestConcurrentOperations_E2E tests concurrent API operations and data consistency.
func (suite *E2ETestSuite) TestConcurrentOperations_E2E() {
	// Create multiple scan profiles concurrently
	const concurrentRequests = 5
	resultCh := make(chan error, concurrentRequests)

	for i := 0; i < concurrentRequests; i++ {
		go func(index int) {
			profileReq := map[string]interface{}{
				"name":        fmt.Sprintf("Concurrent Profile %d", index),
				"description": fmt.Sprintf("Test profile %d", index),
				"scan_type":   "connect",
				"ports":       "80,443",
				"timeout":     60,
				"enabled":     true,
			}

			resp, err := suite.makeAPIRequest("POST", "/profiles", profileReq)
			if err != nil {
				resultCh <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				resultCh <- fmt.Errorf("expected status 201, got %d", resp.StatusCode)
				return
			}

			resultCh <- nil
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < concurrentRequests; i++ {
		err := <-resultCh
		assert.NoError(suite.T(), err, "Concurrent profile creation should succeed")
	}

	// Verify all profiles were created
	var profileCount int
	query := "SELECT COUNT(*) FROM scan_profiles WHERE name LIKE 'Concurrent Profile %' AND created_at >= $1"
	err := suite.database.QueryRowContext(suite.ctx, query, suite.testStartTime).Scan(&profileCount)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), concurrentRequests, profileCount, "All concurrent profiles should be created")
}

// TestDataConsistency_E2E tests data consistency across operations.
func (suite *E2ETestSuite) TestDataConsistency_E2E() {
	// 1. Create a profile
	profileReq := map[string]interface{}{
		"name":        "Consistency Test Profile",
		"description": "Testing data consistency",
		"scan_type":   "connect",
		"ports":       "22,80",
		"timeout":     60,
		"enabled":     true,
	}

	resp, err := suite.makeAPIRequest("POST", "/profiles", profileReq)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), http.StatusCreated, resp.StatusCode)

	var profile map[string]interface{}
	err = suite.parseJSONResponse(resp, &profile)
	require.NoError(suite.T(), err)

	profileID := profile["id"].(string)

	// 2. Update the profile
	updateReq := map[string]interface{}{
		"name":        "Updated Consistency Test Profile",
		"description": "Updated description",
		"ports":       "22,80,443",
	}

	resp, err = suite.makeAPIRequest("PUT", "/profiles/"+profileID, updateReq)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	// 3. Verify update consistency via API
	resp, err = suite.makeAPIRequest("GET", "/profiles/"+profileID, nil)
	require.NoError(suite.T(), err)
	require.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var updatedProfile map[string]interface{}
	err = suite.parseJSONResponse(resp, &updatedProfile)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), "Updated Consistency Test Profile", updatedProfile["name"])
	assert.Equal(suite.T(), "22,80,443", updatedProfile["ports"])

	// 4. Verify update consistency in database
	var dbProfile db.ScanProfile
	parsedProfileID, err := uuid.Parse(profileID)
	require.NoError(suite.T(), err)

	query := "SELECT name, description, ports FROM scan_profiles WHERE id = $1"
	err = suite.database.GetContext(suite.ctx, &dbProfile, query, parsedProfileID)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), "Updated Consistency Test Profile", dbProfile.Name)
	assert.Equal(suite.T(), "22,80,443", dbProfile.Ports)
}

// TestHealthAndStatus_E2E tests health check and status endpoints.
func (suite *E2ETestSuite) TestHealthAndStatus_E2E() {
	// Health check should not require authentication
	req, err := http.NewRequestWithContext(suite.ctx, "GET", suite.baseURL+"/health", nil)
	require.NoError(suite.T(), err)

	resp, err := suite.client.Do(req)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = suite.parseJSONResponse(resp, &health)
	require.NoError(suite.T(), err)

	assert.Equal(suite.T(), "healthy", health["status"])
	assert.Contains(suite.T(), health, "database")
	assert.Contains(suite.T(), health, "timestamp")
}

// Run the E2E test suite
func TestE2EWorkflows(t *testing.T) {
	// Skip E2E tests if explicitly disabled
	if os.Getenv("SKIP_E2E_TESTS") == "true" {
		t.Skip("E2E tests disabled via SKIP_E2E_TESTS environment variable")
	}

	suite.Run(t, new(E2ETestSuite))
}
