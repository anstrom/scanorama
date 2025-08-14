package integration

// Force CI rebuild and ensure clean compilation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/api"
	"github.com/anstrom/scanorama/internal/api/handlers"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// TestAPIIntegration runs comprehensive integration tests for the API with a real database
func TestAPIIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	ctx := context.Background()
	database, server := setupTestServer(t, ctx)
	defer database.Close()

	// Run the test suite
	t.Run("ProfileCRUD", func(t *testing.T) {
		testProfileCRUD(t, server)
	})

	t.Run("HostCRUD", func(t *testing.T) {
		testHostCRUD(t, server)
	})

	t.Run("ScanCRUD", func(t *testing.T) {
		testScanCRUD(t, server)
	})

	t.Run("HostScans", func(t *testing.T) {
		testHostScans(t, server, database)
	})

	t.Run("Pagination", func(t *testing.T) {
		testPagination(t, server)
	})
}

// setupTestServer creates a test server with database connection
func setupTestServer(t testing.TB, ctx context.Context) (*db.DB, *httptest.Server) {
	// Clean up any existing test data first
	cleanupTestDatabase(ctx)

	// Get database configuration from environment variables
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvIntOrDefault("TEST_DB_PORT", 5432)
	dbName := getEnvOrDefault("TEST_DB_NAME", "scanorama_test")
	username := getEnvOrDefault("TEST_DB_USER", "test_user")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "test_password")

	// Database configuration for testing
	dbConfig := &db.Config{
		Host:            host,
		Port:            port,
		Database:        dbName,
		Username:        username,
		Password:        password,
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	// Connect to database and run migrations
	dbConn, err := db.ConnectAndMigrate(ctx, dbConfig)
	require.NoError(t, err, "Failed to connect to test database and run migrations")

	// Create config
	cfg := &config.Config{
		Database: *dbConfig,
		API: config.APIConfig{
			Enabled:      true,
			Host:         "127.0.0.1",
			Port:         0,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
			EnableCORS:   true,
			CORSOrigins:  []string{"*"},
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
	}

	server, err := api.New(cfg, dbConn)
	require.NoError(t, err, "Failed to create API server")

	// Create test server
	testServer := httptest.NewServer(server.GetRouter())
	t.Cleanup(func() {
		testServer.Close()
		dbConn.Close()
	})

	return dbConn, testServer
}

// cleanupTestDatabase removes test data to avoid conflicts between test runs
func cleanupTestDatabase(ctx context.Context) {
	dbConfig := &db.Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "scanorama_test",
		Username:        "test_user",
		Password:        "test_password",
		SSLMode:         "disable",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	}

	database, err := db.Connect(ctx, dbConfig)
	if err != nil {
		// If we can't connect, just skip cleanup - tests will handle conflicts
		return
	}
	defer database.Close()

	// Dynamically drop all database objects to ensure clean state for migrations

	// Drop all materialized views
	var matViews []string
	database.Select(&matViews, `
		SELECT schemaname||'.'||matviewname
		FROM pg_matviews
		WHERE schemaname = 'public'`)
	for _, matView := range matViews {
		database.Exec("DROP MATERIALIZED VIEW IF EXISTS " + matView + " CASCADE")
	}

	// Drop all views
	var views []string
	database.Select(&views, `
		SELECT schemaname||'.'||viewname
		FROM pg_views
		WHERE schemaname = 'public'`)
	for _, view := range views {
		database.Exec("DROP VIEW IF EXISTS " + view + " CASCADE")
	}

	// Drop all functions
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

	// Drop all tables (including schema_migrations to ensure fresh migration state)
	var tables []string
	database.Select(&tables, `
		SELECT schemaname||'.'||tablename
		FROM pg_tables
		WHERE schemaname = 'public'`)
	for _, table := range tables {
		database.Exec("DROP TABLE IF EXISTS " + table + " CASCADE")
	}
}

// testProfileCRUD tests profile CRUD operations
func testProfileCRUD(t *testing.T, server *httptest.Server) {
	// Test data
	profileData := map[string]interface{}{
		"name":              "Test Profile",
		"description":       "Integration test profile",
		"scan_type":         "connect",
		"ports":             "22,80,443",
		"service_detection": true,
		"os_detection":      false,
		"script_scan":       false,
		"udp_scan":          false,
		"max_retries":       3,
		"host_timeout":      "30s",
		"max_rate_pps":      1000,
		"tags":              []string{"test", "integration"},
		"default":           false,
	}

	// Test Create Profile
	t.Run("CreateProfile", func(t *testing.T) {
		body, _ := json.Marshal(profileData)
		resp, err := http.Post(server.URL+"/api/v1/profiles", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result handlers.ProfileResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "Test Profile", result.Name)
		assert.Equal(t, "connect", result.ScanType)
		assert.True(t, result.ServiceDetection)
	})

	// Test List Profiles
	t.Run("ListProfiles", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/profiles")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should have data and pagination
		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})
}

// testHostCRUD tests host CRUD operations
func testHostCRUD(t *testing.T, server *httptest.Server) {
	// Test data with UUID-based unique IP to avoid conflicts
	testUUID := uuid.New()
	uniqueIP := fmt.Sprintf("192.168.1.%d", testUUID.ID()%200+10) // Range 10-209
	hostData := map[string]interface{}{
		"ip":          uniqueIP,
		"hostname":    "test-host",
		"description": "Integration test host",
		"os":          "Linux",
		"os_version":  "Ubuntu 22.04",
		"active":      true,
		"tags":        []string{"test", "integration"},
		"metadata": map[string]string{
			"test": "true",
			"env":  "integration",
		},
	}

	var createdHostID string

	// Test Create Host
	t.Run("CreateHost", func(t *testing.T) {
		body, _ := json.Marshal(hostData)
		resp, err := http.Post(server.URL+"/api/v1/hosts", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result handlers.HostResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, uniqueIP, result.IP)
		assert.Equal(t, "test-host", result.Hostname)
		assert.True(t, result.Active)

		createdHostID = result.ID
	})

	// Test Get Host
	t.Run("GetHost", func(t *testing.T) {
		if createdHostID == "" {
			t.Skip("No host created to test")
		}

		resp, err := http.Get(server.URL + "/api/v1/hosts/" + createdHostID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result handlers.HostResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, uniqueIP, result.IP)
		assert.Equal(t, "test-host", result.Hostname)
	})

	// Test List Hosts
	t.Run("ListHosts", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/hosts")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})
}

// testScanCRUD tests scan CRUD operations
func testScanCRUD(t *testing.T, server *httptest.Server) {
	// Test data
	scanData := map[string]interface{}{
		"name":        "Integration Test Scan",
		"description": "Test scan for integration testing",
		"targets":     []string{"192.168.1.0/24"},
		"scan_type":   "connect",
		"ports":       "22,80,443",
		"options": map[string]interface{}{
			"timing": "normal",
		},
		"tags": []string{"test", "integration"},
	}

	var createdScanID string

	// Test Create Scan
	t.Run("CreateScan", func(t *testing.T) {
		body, _ := json.Marshal(scanData)
		resp, err := http.Post(server.URL+"/api/v1/scans", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result handlers.ScanResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "Integration Test Scan", result.Name)
		assert.Equal(t, "connect", result.ScanType)
		assert.Equal(t, "pending", result.Status)

		createdScanID = result.ID.String()
	})

	// Test Get Scan
	t.Run("GetScan", func(t *testing.T) {
		if createdScanID == "" {
			t.Skip("No scan created to test")
		}

		resp, err := http.Get(server.URL + "/api/v1/scans/" + createdScanID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result handlers.ScanResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "Integration Test Scan", result.Name)
		assert.Equal(t, "connect", result.ScanType)
	})

	// Test List Scans
	t.Run("ListScans", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/scans")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "data")
		assert.Contains(t, result, "pagination")
	})

	// Test Get Scan Results
	t.Run("GetScanResults", func(t *testing.T) {
		if createdScanID == "" {
			t.Skip("No scan created to test")
		}

		resp, err := http.Get(server.URL + "/api/v1/scans/" + createdScanID + "/results")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result handlers.ScanResultsResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// New scan should have no results yet
		assert.Equal(t, 0, result.TotalHosts)
		assert.Equal(t, 0, result.TotalPorts)
	})
}

// testHostScans tests the host-scan relationship endpoints
func testHostScans(t *testing.T, server *httptest.Server, database *db.DB) {
	ctx := context.Background()

	// Create a test host directly in database for testing relationships with unique IP
	testUUID := uuid.New()
	uniqueIP := fmt.Sprintf("192.168.2.%d", testUUID.ID()%200+10) // Range 10-209
	hostData := map[string]interface{}{
		"ip_address": uniqueIP,
		"hostname":   "relationship-test-host",
		"status":     "up",
	}

	host, err := database.CreateHost(ctx, hostData)
	require.NoError(t, err)

	hostID := host.ID.String()

	// Test Get Host Scans (should be empty initially)
	t.Run("GetHostScans_Empty", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/hosts/" + hostID + "/scans")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "data")
		data := result["data"].([]interface{})
		assert.Equal(t, 0, len(data))
	})
}

// testPagination tests pagination functionality across endpoints
func testPagination(t *testing.T, server *httptest.Server) {
	// Test pagination parameters
	testCases := []struct {
		name     string
		endpoint string
		params   string
	}{
		{"Hosts_DefaultPagination", "/api/v1/hosts", ""},
		{"Hosts_CustomPagination", "/api/v1/hosts", "?page=1&page_size=10"},
		{"Scans_DefaultPagination", "/api/v1/scans", ""},
		{"Scans_CustomPagination", "/api/v1/scans", "?page=1&page_size=5"},
		{"Profiles_DefaultPagination", "/api/v1/profiles", ""},
		{"Profiles_CustomPagination", "/api/v1/profiles", "?page=1&page_size=20"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.endpoint + tc.params)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			// Validate pagination structure
			assert.Contains(t, result, "data")
			assert.Contains(t, result, "pagination")

			pagination := result["pagination"].(map[string]interface{})
			assert.Contains(t, pagination, "page")
			assert.Contains(t, pagination, "page_size")
			assert.Contains(t, pagination, "total_items")
			assert.Contains(t, pagination, "total_pages")
		})
	}
}

// TestHealthEndpoints tests the health and status endpoints
func TestHealthEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, server := setupTestServer(t, context.Background())

	endpoints := []struct {
		name string
		path string
	}{
		{"Health", "/api/v1/health"},
		{"Liveness", "/api/v1/liveness"},
		{"Status", "/api/v1/status"},
		{"Version", "/api/v1/version"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + endpoint.path)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
		})
	}
}

// TestErrorHandling tests API error responses
func TestErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, server := setupTestServer(t, context.Background())

	t.Run("NotFound", func(t *testing.T) {
		nonExistentID := uuid.New().String()
		resp, err := http.Get(server.URL + "/api/v1/hosts/" + nonExistentID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("BadRequest_InvalidJSON", func(t *testing.T) {
		invalidJSON := bytes.NewBufferString(`{"invalid": json}`)
		resp, err := http.Post(server.URL+"/api/v1/hosts", "application/json", invalidJSON)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("BadRequest_InvalidUUID", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/hosts/invalid-uuid")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("BadRequest_MissingFields", func(t *testing.T) {
		// Missing required IP field
		incompleteHost := map[string]interface{}{
			"hostname": "incomplete-host",
		}
		body, _ := json.Marshal(incompleteHost)
		resp, err := http.Post(server.URL+"/api/v1/hosts", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestFiltering tests filtering functionality
func TestFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, server := setupTestServer(t, context.Background())

	filterTests := []struct {
		name     string
		endpoint string
		filters  string
	}{
		{"HostsByOS", "/api/v1/hosts", "?os=linux"},
		{"HostsByStatus", "/api/v1/hosts", "?status=up"},
		{"ScansByStatus", "/api/v1/scans", "?status=pending"},
		{"ScansByType", "/api/v1/scans", "?scan_type=connect"},
		{"ProfilesByScanType", "/api/v1/profiles", "?scan_type=connect"},
	}

	for _, test := range filterTests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + test.endpoint + test.filters)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Contains(t, result, "data")
			assert.Contains(t, result, "pagination")
		})
	}
}

// TestConcurrentOperations tests concurrent API operations
func TestConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, server := setupTestServer(t, context.Background())

	// Test concurrent host creation
	t.Run("ConcurrentHostCreation", func(t *testing.T) {
		const numConcurrent = 5
		results := make(chan error, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			go func(id int) {
				hostData := map[string]interface{}{
					"ip":       fmt.Sprintf("192.168.2.%d", 100+id),
					"hostname": fmt.Sprintf("concurrent-host-%d", id),
					"active":   true,
				}

				body, _ := json.Marshal(hostData)
				resp, err := http.Post(server.URL+"/api/v1/hosts", "application/json", bytes.NewBuffer(body))
				if err != nil {
					results <- err
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusCreated {
					results <- fmt.Errorf("expected status 201, got %d", resp.StatusCode)
					return
				}

				results <- nil
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numConcurrent; i++ {
			err := <-results
			assert.NoError(t, err, "Concurrent operation %d failed", i)
		}
	})
}

// TestDatabaseTransactions tests that database operations are properly transactional
func TestDatabaseTransactions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	database, server := setupTestServer(t, context.Background())

	t.Run("HostCreationWithConflict", func(t *testing.T) {
		hostData := map[string]interface{}{
			"ip":       "192.168.3.100",
			"hostname": "conflict-test-host",
			"active":   true,
		}

		// Create first host
		body, _ := json.Marshal(hostData)
		resp1, err := http.Post(server.URL+"/api/v1/hosts", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		defer resp1.Body.Close()
		assert.Equal(t, http.StatusCreated, resp1.StatusCode)

		// Try to create duplicate host (should fail)
		body2, _ := json.Marshal(hostData)
		resp2, err := http.Post(server.URL+"/api/v1/hosts", "application/json", bytes.NewBuffer(body2))
		require.NoError(t, err)
		defer resp2.Body.Close()
		assert.Equal(t, http.StatusConflict, resp2.StatusCode)

		// Verify only one host exists in database
		ctx := context.Background()
		hosts, _, err := database.ListHosts(ctx, db.HostFilters{}, 0, 100)
		require.NoError(t, err)

		conflictCount := 0
		for _, host := range hosts {
			if host.IPAddress.String() == "192.168.3.100" {
				conflictCount++
			}
		}
		assert.Equal(t, 1, conflictCount, "Should have exactly one host with the IP")
	})
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntOrDefault returns environment variable as int or default if not set
func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// TestValidation tests input validation across endpoints
func TestValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_, server := setupTestServer(t, context.Background())

	validationTests := []struct {
		name         string
		method       string
		endpoint     string
		data         map[string]interface{}
		expectedCode int
	}{
		{
			name:     "Host_InvalidIP",
			method:   "POST",
			endpoint: "/api/v1/hosts",
			data: map[string]interface{}{
				"ip":     "invalid.ip.address",
				"active": true,
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "Host_MissingIP",
			method:   "POST",
			endpoint: "/api/v1/hosts",
			data: map[string]interface{}{
				"hostname": "test-host",
				"active":   true,
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "Scan_MissingName",
			method:   "POST",
			endpoint: "/api/v1/scans",
			data: map[string]interface{}{
				"targets":   []string{"192.168.1.0/24"},
				"scan_type": "connect",
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:     "Profile_InvalidScanType",
			method:   "POST",
			endpoint: "/api/v1/profiles",
			data: map[string]interface{}{
				"name":      "Invalid Profile",
				"scan_type": "invalid_type",
				"ports":     "80,443",
			},
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, test := range validationTests {
		t.Run(test.name, func(t *testing.T) {
			body, _ := json.Marshal(test.data)
			req, err := http.NewRequest(test.method, server.URL+test.endpoint, bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, test.expectedCode, resp.StatusCode)
		})
	}
}

// BenchmarkAPIPerformance benchmarks API endpoint performance
func BenchmarkAPIPerformance(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	_, server := setupTestServer(b, context.Background())

	b.Run("ListHosts", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := http.Get(server.URL + "/api/v1/hosts")
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})

	b.Run("ListScans", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := http.Get(server.URL + "/api/v1/scans")
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})

	b.Run("ListProfiles", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			resp, err := http.Get(server.URL + "/api/v1/profiles")
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})
}
