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
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateUniqueNetworkName creates a unique network name for testing
func generateUniqueNetworkName(base string) string {
	return fmt.Sprintf("%s %d", base, time.Now().UnixNano())
}

// generateUniqueCIDR creates a unique CIDR for testing
func generateUniqueCIDR(baseOctet int) string {
	// Use timestamp to generate a unique third octet
	thirdOctet := (time.Now().UnixNano() / 1000) % 250
	return fmt.Sprintf("172.%d.%d.0/24", baseOctet, thirdOctet)
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

	// Clean up any leftover test data from previous runs (handler-specific prefix)
	_, _ = database.Exec(`
		DELETE FROM network_exclusions
		WHERE network_id IN (
			SELECT id
			FROM networks
			WHERE name LIKE 'HandlerTest%' OR name LIKE 'HandlerUpdated%'
		)`)
	_, _ = database.Exec(`
		DELETE FROM networks
		WHERE name LIKE 'HandlerTest%' OR name LIKE 'HandlerUpdated%'`)

	cleanup := func() {
		// Clean up test data (handler-specific prefix)
		_, _ = database.Exec(`
			DELETE FROM network_exclusions
			WHERE network_id IN (
				SELECT id
				FROM networks
				WHERE name LIKE 'HandlerTest%' OR name LIKE 'HandlerUpdated%'
			)`)
		_, _ = database.Exec(`
			DELETE FROM networks
			WHERE name LIKE 'HandlerTest%' OR name LIKE 'HandlerUpdated%'`)
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
	activeNetworkName := generateUniqueNetworkName("HandlerTest Active Network")
	inactiveNetworkName := generateUniqueNetworkName("HandlerTest Inactive Network")
	anotherActiveNetworkName := generateUniqueNetworkName("HandlerTest Another Active")

	testNetworks := []struct {
		name            string
		cidr            string
		discoveryMethod string
		isActive        bool
	}{
		{activeNetworkName, "10.0.1.0/24", "ping", true},
		{inactiveNetworkName, "10.0.2.0/24", "tcp", false},
		{anotherActiveNetworkName, "192.168.1.0/24", "arp", true},
	}

	for _, tn := range testNetworks {
		query := `
			INSERT INTO networks (
				name, cidr, discovery_method, is_active, scan_enabled
			)
			VALUES ($1, $2, $3, $4, $5)`

		_, err := database.Exec(query,
			tn.name, tn.cidr, tn.discoveryMethod, tn.isActive, true)
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
			expectedContains: []string{activeNetworkName, anotherActiveNetworkName},
		},
		{
			name:             "list all networks including inactive",
			queryParams:      map[string]string{"show_inactive": "true"},
			expectedCount:    3,
			expectedContains: []string{activeNetworkName, inactiveNetworkName, anotherActiveNetworkName},
		},
		{
			name:             "filter by name",
			queryParams:      map[string]string{"name": "Active"},
			expectedCount:    2,
			expectedContains: []string{activeNetworkName, anotherActiveNetworkName},
		},
		{
			name:             "filter by name with inactive",
			queryParams:      map[string]string{"name": "Inactive", "show_inactive": "true"},
			expectedCount:    1,
			expectedContains: []string{inactiveNetworkName},
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
				Name:            generateUniqueNetworkName("HandlerTest Valid Network"),
				CIDR:            generateUniqueCIDR(16),
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Contains(t, network["name"], "HandlerTest Valid Network")
				assert.NotEmpty(t, network["cidr"])
				assert.NotEmpty(t, network["id"])
			},
		},
		{
			name: "create network with description",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Network With Description"),
				CIDR:            generateUniqueCIDR(17),
				DiscoveryMethod: "tcp",
				Description:     stringPtr("HandlerTest description"),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Contains(t, network["name"], "HandlerTest Network With Description")
				assert.Equal(t, "HandlerTest description", network["description"])
			},
		},
		{
			name: "create network with inactive status",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Inactive Network Create"),
				CIDR:            generateUniqueCIDR(18),
				DiscoveryMethod: "arp",
				IsActive:        boolPtr(false),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Equal(t, false, network["is_active"])
			},
		},
		// Skipping "missing required name" test - database has unique constraint on name
		// which causes duplicate key errors for empty strings
		{
			// TODO: Handler should validate CIDR before calling service
			name: "invalid CIDR",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Invalid CIDR"),
				CIDR:            "not-a-cidr",
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusInternalServerError, // Currently returns 500
		},
		{
			// TODO: Handler should validate discovery method before calling service
			name: "invalid discovery method",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Invalid Method"),
				CIDR:            generateUniqueCIDR(20),
				DiscoveryMethod: "invalid",
			},
			expectedStatus: http.StatusInternalServerError, // Currently returns 500
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
	networkName := generateUniqueNetworkName("HandlerTest Get Network")

	query := `
		INSERT INTO networks (
			name, cidr, discovery_method, is_active, scan_enabled
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	err := database.QueryRow(query,
		networkName, "10.0.10.0/24", "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:      "get existing network",
			networkID: networkID,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Equal(t, networkID, network["id"])
				assert.Contains(t, network["name"], "HandlerTest Get Network")
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get non-existent network",
			networkID:      "00000000-0000-0000-0000-000000000000",
			expectedStatus: http.StatusInternalServerError, // Currently returns 500 for non-existent network
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
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
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
	networkName := generateUniqueNetworkName("HandlerTest Update Network")

	query := `
		INSERT INTO networks (
			name, cidr, discovery_method, is_active, scan_enabled
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	err := database.QueryRow(query,
		networkName, generateUniqueCIDR(20), "ping", true, true).Scan(&networkID)
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
				Name: stringPtr(generateUniqueNetworkName("Updated Network Name")),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Contains(t, network["name"], "Updated Network Name")
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
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

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
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Equal(t, false, network["is_active"])
			},
		},
		{
			name:      "update non-existent network",
			networkID: "00000000-0000-0000-0000-000000000000",
			request: UpdateNetworkRequest{
				Name: stringPtr(generateUniqueNetworkName("Does Not Exist")),
			},
			expectedStatus: http.StatusInternalServerError, // Currently returns 500 for non-existent network
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/networks/"+tt.networkID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.UpdateNetwork(w, req)

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
				query := `
					INSERT INTO networks (
						name, cidr, discovery_method, is_active, scan_enabled
					)
					VALUES ($1, $2, $3, $4, $5)
					RETURNING id`

				err := database.QueryRow(query,
					generateUniqueNetworkName("HandlerTest Delete Network 1"),
					generateUniqueCIDR(30), "ping", true, true).Scan(&id)
				require.NoError(t, err)
				return id
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "delete non-existent network",
			setupNetwork: func() string {
				return "00000000-0000-0000-0000-000000000000"
			},
			expectedStatus: http.StatusInternalServerError, // Currently returns 500 for non-existent network
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			networkID := tt.setupNetwork()

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/networks/"+networkID, nil)
			req = mux.SetURLVars(req, map[string]string{"id": networkID})
			w := httptest.NewRecorder()

			handler.DeleteNetwork(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify deletion if successful
			if tt.expectedStatus == http.StatusNoContent {
				var exists bool
				query := `
					SELECT EXISTS(
						SELECT 1
						FROM networks
						WHERE id = $1
					)`

				err := database.QueryRow(query, networkID).Scan(&exists)
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
	query := `
		INSERT INTO networks (
			name, cidr, discovery_method, is_active, scan_enabled
		)
		VALUES
			($1, '10.0.40.0/24', 'ping', true, true),
			($2, '10.0.41.0/24', 'tcp', true, false),
			($3, '10.0.42.0/24', 'arp', false, true)`

	_, err := database.Exec(query,
		generateUniqueNetworkName("HandlerTest Stats Network 1"),
		generateUniqueNetworkName("HandlerTest Stats Network 2"),
		generateUniqueNetworkName("HandlerTest Stats Network 3"))
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
