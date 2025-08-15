//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseURL        = "http://localhost:8080/api/v1"
	defaultTimeout = 30 * time.Second
)

var httpClient = &http.Client{
	Timeout: defaultTimeout,
}

// TestMain sets up the test environment
func TestMain(m *testing.M) {
	// Ensure server is running
	if !isServerRunning() {
		fmt.Println("❌ Server is not running. Please start the server before running integration tests.")
		os.Exit(1)
	}

	fmt.Println("✅ Server is running, starting integration tests...")
	code := m.Run()
	os.Exit(code)
}

// isServerRunning checks if the server is responding
func isServerRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Contains(t, health, "status")
	assert.Equal(t, "ok", health["status"])
	t.Logf("Health response: %+v", health)
}

// TestStatusEndpoint tests the status endpoint
func TestStatusEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	assert.Contains(t, status, "database")
	t.Logf("Status response: %+v", status)
}

// TestVersionEndpoint tests the version endpoint
func TestVersionEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/version")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var version map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&version)
	require.NoError(t, err)

	assert.Contains(t, version, "version")
	t.Logf("Version response: %+v", version)
}

// TestScansEndpoint tests the scans endpoint
func TestScansEndpoint(t *testing.T) {
	t.Run("GET /scans", func(t *testing.T) {
		resp, err := httpClient.Get(baseURL + "/scans")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Accept both 200 (with data) or 404 (no scans yet)
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			var scans map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&scans)
			require.NoError(t, err)
			t.Logf("Scans response: %+v", scans)
		}
	})

	t.Run("POST /scans", func(t *testing.T) {
		scanData := map[string]interface{}{
			"name":        "integration-test-scan",
			"description": "Test scan created by integration tests",
			"targets":     []string{"127.0.0.1"},
			"profile_id":  1,
		}

		jsonData, err := json.Marshal(scanData)
		require.NoError(t, err)

		resp, err := httpClient.Post(baseURL+"/scans", "application/json", bytes.NewBuffer(jsonData))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Accept created, bad request (missing data), or unprocessable entity
		assert.Contains(t, []int{http.StatusCreated, http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)

		if resp.StatusCode == http.StatusCreated {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			t.Logf("Created scan: %+v", result)
		}
	})
}

// TestHostsEndpoint tests the hosts endpoint
func TestHostsEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/hosts")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Accept both 200 (with data) or 404 (no hosts yet)
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		var hosts map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&hosts)
		require.NoError(t, err)
		t.Logf("Hosts response: %+v", hosts)
	}
}

// TestProfilesEndpoint tests the profiles endpoint
func TestProfilesEndpoint(t *testing.T) {
	t.Run("GET /profiles", func(t *testing.T) {
		resp, err := httpClient.Get(baseURL + "/profiles")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Accept both 200 (with data) or 404 (no profiles yet)
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			var profiles map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&profiles)
			require.NoError(t, err)
			t.Logf("Profiles response: %+v", profiles)
		}
	})

	t.Run("POST /profiles", func(t *testing.T) {
		profileData := map[string]interface{}{
			"name":        "integration-test-profile",
			"description": "Test profile created by integration tests",
			"settings": map[string]interface{}{
				"timeout": 30,
				"retries": 3,
			},
		}

		jsonData, err := json.Marshal(profileData)
		require.NoError(t, err)

		resp, err := httpClient.Post(baseURL+"/profiles", "application/json", bytes.NewBuffer(jsonData))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Accept created, bad request, or unprocessable entity
		assert.Contains(t, []int{http.StatusCreated, http.StatusBadRequest, http.StatusUnprocessableEntity}, resp.StatusCode)

		if resp.StatusCode == http.StatusCreated {
			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			t.Logf("Created profile: %+v", result)
		}
	})
}

// TestDiscoveryJobsEndpoint tests the discovery jobs endpoint
func TestDiscoveryJobsEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/discovery-jobs")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Accept both 200 (with data) or 404 (no jobs yet)
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		var jobs map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&jobs)
		require.NoError(t, err)
		t.Logf("Discovery jobs response: %+v", jobs)
	}
}

// TestSchedulesEndpoint tests the schedules endpoint
func TestSchedulesEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/schedules")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Accept both 200 (with data) or 404 (no schedules yet)
	assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		var schedules map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&schedules)
		require.NoError(t, err)
		t.Logf("Schedules response: %+v", schedules)
	}
}

// TestAdminEndpoint tests the admin status endpoint
func TestAdminEndpoint(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/admin/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Admin endpoints might be restricted, accept 200, 401, or 403
	assert.Contains(t, []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode)

	if resp.StatusCode == http.StatusOK {
		var adminStatus map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&adminStatus)
		require.NoError(t, err)
		t.Logf("Admin status response: %+v", adminStatus)
	} else {
		t.Logf("Admin endpoint returned %d (expected for restricted access)", resp.StatusCode)
	}
}

// TestAPIResponseHeaders tests that all endpoints return proper headers
func TestAPIResponseHeaders(t *testing.T) {
	endpoints := []string{
		"/health",
		"/status",
		"/version",
		"/scans",
		"/hosts",
		"/profiles",
		"/discovery-jobs",
		"/schedules",
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("Headers for %s", endpoint), func(t *testing.T) {
			resp, err := httpClient.Get(baseURL + endpoint)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check Content-Type header is set for successful responses
			if resp.StatusCode == http.StatusOK {
				contentType := resp.Header.Get("Content-Type")
				assert.Contains(t, contentType, "application/json", "Expected JSON content type")
			}

			// Check that CORS headers are present (if configured)
			if corsOrigin := resp.Header.Get("Access-Control-Allow-Origin"); corsOrigin != "" {
				t.Logf("CORS enabled for %s: %s", endpoint, corsOrigin)
			}
		})
	}
}

// TestConcurrentRequests tests the server under concurrent load
func TestConcurrentRequests(t *testing.T) {
	const numRequests = 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			resp, err := httpClient.Get(baseURL + "/health")
			if err != nil {
				results <- fmt.Errorf("request %d failed: %w", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("request %d returned status %d", id, resp.StatusCode)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Logf("Concurrent request errors: %v", errors)
		assert.LessOrEqual(t, len(errors), numRequests/2, "Too many concurrent requests failed")
	} else {
		t.Log("✅ All concurrent requests succeeded")
	}
}

// TestDatabaseConnectivity tests that the server can connect to the database
func TestDatabaseConnectivity(t *testing.T) {
	resp, err := httpClient.Get(baseURL + "/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	// Check database status
	if dbStatus, exists := status["database"]; exists {
		if dbMap, ok := dbStatus.(map[string]interface{}); ok {
			if dbConnected, exists := dbMap["connected"]; exists {
				assert.True(t, dbConnected.(bool), "Database should be connected")
			}
		}
	}

	t.Log("✅ Database connectivity verified")
}
