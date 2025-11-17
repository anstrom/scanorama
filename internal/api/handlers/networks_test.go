package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateUniqueNetworkName creates a unique network name for testing
func generateUniqueNetworkName(base string) string {
	return fmt.Sprintf("%s %d", base, time.Now().UnixNano())
}

// setupNetworkHandlerTest sets up a network handler with database for testing
func setupNetworkHandlerTest(t *testing.T) (*NetworkHandler, *db.DB, func()) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return nil, nil, nil
	}

	logger := createTestLogger()
	metricsRegistry := metrics.NewRegistry()
	handler := NewNetworkHandler(database, logger, metricsRegistry)

	cleanup := func() {
		// Clean up test data
		_, _ = database.Exec(
			"DELETE FROM network_exclusions WHERE network_id IN " +
				"(SELECT id FROM networks WHERE name LIKE 'Test%')")
		_, _ = database.Exec("DELETE FROM networks WHERE name LIKE 'Test%'")
		database.Close()
	}

	return handler, database, cleanup
}

func TestNewNetworkHandler(t *testing.T) {
	logger := createTestLogger()
	metricsRegistry := metrics.NewRegistry()
	mockDB := &db.DB{}

	handler := NewNetworkHandler(mockDB, logger, metricsRegistry)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.service)
	assert.NotNil(t, handler.BaseHandler)
}

func TestNetworkHandler_ListNetworks(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create test networks with unique names
	testNetworks := []struct {
		name            string
		cidr            string
		discoveryMethod string
		isActive        bool
	}{
		{generateUniqueNetworkName("Test Active Network"), "10.0.1.0/24", "ping", true},
		{generateUniqueNetworkName("Test Inactive Network"), "10.0.2.0/24", "tcp", false},
		{generateUniqueNetworkName("Test Another Active"), "192.168.1.0/24", "arp", true},
	}

	for _, tn := range testNetworks {
		_, err := database.Exec(`
			INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
			VALUES ($1, $2, $3, $4, $5)
		`, tn.name, tn.cidr, tn.discoveryMethod, tn.isActive, true)
		require.NoError(t, err)
	}

	tests := []struct {
		name             string
		queryParams      map[string]string
		expectedCount    int
		expectedContains []string
	}{
		{
			name:             "list all active networks",
			queryParams:      map[string]string{},
			expectedCount:    2,
			expectedContains: []string{"Test Active Network", "Test Another Active"},
		},
		{
			name:             "list all networks including inactive",
			queryParams:      map[string]string{"show_inactive": "true"},
			expectedCount:    3,
			expectedContains: []string{"Test Active Network", "Test Inactive Network", "Test Another Active"},
		},
		{
			name:             "filter by name",
			queryParams:      map[string]string{"name": "Active"},
			expectedCount:    2,
			expectedContains: []string{"Test Active Network", "Test Another Active"},
		},
		{
			name:             "filter by name with inactive",
			queryParams:      map[string]string{"name": "Inactive", "show_inactive": "true"},
			expectedCount:    1,
			expectedContains: []string{"Test Inactive Network"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build URL with query parameters
			url := "/api/v1/networks"
			if len(tt.queryParams) > 0 {
				url += "?"
				first := true
				for k, v := range tt.queryParams {
					if !first {
						url += "&"
					}
					url += fmt.Sprintf("%s=%s", k, v)
					first = false
				}
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			handler.ListNetworks(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response struct {
				Data []map[string]interface{} `json:"data"`
			}
			err := json.NewDecoder(w.Body).Decode(&response)
			require.NoError(t, err)

			assert.Len(t, response.Data, tt.expectedCount)

			// Check that expected networks are in the response
			networkNames := make([]string, 0, len(response.Data))
			for _, net := range response.Data {
				if name, ok := net["name"].(string); ok {
					networkNames = append(networkNames, name)
				}
			}

			for _, expectedName := range tt.expectedContains {
				assert.Contains(t, networkNames, expectedName)
			}
		})
	}
}

func TestNetworkHandler_CreateNetwork(t *testing.T) {
	handler, _, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	tests := []struct {
		name           string
		request        CreateNetworkRequest
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "create valid network",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("Test Valid Network"),
				CIDR:            fmt.Sprintf("172.16.%d.0/24", time.Now().UnixNano()%250+1),
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Contains(t, network["name"], "Test Valid Network")
				assert.NotEmpty(t, network["cidr"])
				assert.NotEmpty(t, network["id"])
			},
		},
		{
			name: "create network with description",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("Test Network With Description"),
				CIDR:            fmt.Sprintf("172.17.%d.0/24", time.Now().UnixNano()%250+1),
				DiscoveryMethod: "tcp",
				Description:     stringPtr("Test description"),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Contains(t, network["name"], "Test Network With Description")
				assert.Equal(t, "Test description", network["description"])
			},
		},
		{
			name: "create network with inactive status",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("Test Inactive Network Create"),
				CIDR:            fmt.Sprintf("172.18.%d.0/24", time.Now().UnixNano()%250+1),
				DiscoveryMethod: "arp",
				IsActive:        boolPtr(false),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Equal(t, false, network["is_active"])
			},
		},
		{
			name: "missing required name",
			request: CreateNetworkRequest{
				CIDR:            fmt.Sprintf("172.19.%d.0/24", time.Now().UnixNano()%250+1),
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid CIDR",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("Test Invalid CIDR"),
				CIDR:            "not-a-cidr",
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid discovery method",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("Test Invalid Method"),
				CIDR:            fmt.Sprintf("172.20.%d.0/24", time.Now().UnixNano()%250+1),
				DiscoveryMethod: "invalid",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateNetwork(w, req)

			if w.Code != tt.expectedStatus {
				t.Logf("Expected status %d but got %d. Response: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil && w.Code == tt.expectedStatus {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

func TestNetworkHandler_GetNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network
	var networkID string
	networkName := generateUniqueNetworkName("Test Get Network")
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, networkName, "10.0.10.0/24", "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "get existing network",
			networkID:      networkID,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Contains(t, network["name"], "Test Get Network")
				assert.Equal(t, "10.0.10.0/24", network["cidr"])
			},
		},
		{
			name:           "get non-existent network",
			networkID:      "00000000-0000-0000-0000-000000000000",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid UUID",
			networkID:      "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/"+tt.networkID, nil)
			req.SetPathValue("id", tt.networkID)
			w := httptest.NewRecorder()

			handler.GetNetwork(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

func TestNetworkHandler_UpdateNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network
	var networkID string
	networkName := generateUniqueNetworkName("Test Update Network")
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, networkName, "10.0.20.0/24", "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		request        UpdateNetworkRequest
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:      "update network name",
			networkID: networkID,
			request: UpdateNetworkRequest{
				Name: stringPtr("Updated Network Name"),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Equal(t, "Updated Network Name", network["name"])
			},
		},
		{
			name:      "update discovery method",
			networkID: networkID,
			request: UpdateNetworkRequest{
				DiscoveryMethod: stringPtr("tcp"),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Equal(t, "tcp", network["discovery_method"])
			},
		},
		{
			name:      "update is_active status",
			networkID: networkID,
			request: UpdateNetworkRequest{
				IsActive: boolPtr(false),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)

				network := response["network"].(map[string]interface{})
				assert.Equal(t, false, network["is_active"])
			},
		},
		{
			name:      "update non-existent network",
			networkID: "00000000-0000-0000-0000-000000000000",
			request: UpdateNetworkRequest{
				Name: stringPtr("Does Not Exist"),
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/networks/"+tt.networkID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", tt.networkID)
			w := httptest.NewRecorder()

			handler.UpdateNetwork(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

func TestNetworkHandler_DeleteNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	tests := []struct {
		name           string
		setupNetwork   func() string
		expectedStatus int
	}{
		{
			name: "delete existing network",
			setupNetwork: func() string {
				var id string
				err := database.QueryRow(`
					INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
					VALUES ($1, $2, $3, $4, $5)
					RETURNING id
				`, generateUniqueNetworkName("Test Delete Network 1"), "10.0.30.0/24", "ping", true, true).Scan(&id)
				require.NoError(t, err)
				return id
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "delete non-existent network",
			setupNetwork: func() string {
				return "00000000-0000-0000-0000-000000000000"
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			networkID := tt.setupNetwork()

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/networks/"+networkID, nil)
			req.SetPathValue("id", networkID)
			w := httptest.NewRecorder()

			handler.DeleteNetwork(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify deletion if successful
			if tt.expectedStatus == http.StatusOK {
				var exists bool
				err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM networks WHERE id = $1)", networkID).Scan(&exists)
				require.NoError(t, err)
				assert.False(t, exists, "Network should be deleted")
			}
		})
	}
}

func TestNetworkHandler_GetNetworkStats(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create test networks with some data
	_, err := database.Exec(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES
			($1, '10.0.40.0/24', 'ping', true, true),
			($2, '10.0.41.0/24', 'tcp', true, false),
			($3, '10.0.42.0/24', 'arp', false, true)
	`, generateUniqueNetworkName("Test Stats Network 1"),
		generateUniqueNetworkName("Test Stats Network 2"),
		generateUniqueNetworkName("Test Stats Network 3"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/stats", nil)
	w := httptest.NewRecorder()

	handler.GetNetworkStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response NetworkStatsResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify stats structure
	assert.NotNil(t, response.Networks)
	assert.NotNil(t, response.Hosts)
	assert.NotNil(t, response.Exclusions)

	// Check that we have the expected keys
	assert.Contains(t, response.Networks, "total")
	assert.Contains(t, response.Networks, "active")
}
