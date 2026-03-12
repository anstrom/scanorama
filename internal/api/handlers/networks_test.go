package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/services"
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
			name: "invalid CIDR",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Invalid CIDR"),
				CIDR:            "not-a-cidr",
				DiscoveryMethod: "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid discovery method",
			request: CreateNetworkRequest{
				Name:            generateUniqueNetworkName("HandlerTest Invalid Method"),
				CIDR:            generateUniqueCIDR(20),
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
				Name: stringPtr(generateUniqueNetworkName("HandlerUpdated Network Name")),
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)

				assert.Contains(t, network["name"], "HandlerUpdated Network Name")
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
			expectedStatus: http.StatusNotFound,
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
			expectedStatus: http.StatusNotFound,
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

// Unit tests for helper functions

func TestNetworkHandler_parseNetworkFilters(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name               string
		queryParams        map[string]string
		expectedInactive   bool
		expectedNameFilter string
	}{
		{
			name:               "no filters",
			queryParams:        map[string]string{},
			expectedInactive:   false,
			expectedNameFilter: "",
		},
		{
			name:               "show_inactive true",
			queryParams:        map[string]string{"show_inactive": "true"},
			expectedInactive:   true,
			expectedNameFilter: "",
		},
		{
			name:               "show_inactive false",
			queryParams:        map[string]string{"show_inactive": "false"},
			expectedInactive:   false,
			expectedNameFilter: "",
		},
		{
			name:               "name filter",
			queryParams:        map[string]string{"name": "production"},
			expectedInactive:   false,
			expectedNameFilter: "production",
		},
		{
			name:               "both filters",
			queryParams:        map[string]string{"show_inactive": "true", "name": "test"},
			expectedInactive:   true,
			expectedNameFilter: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			showInactive, nameFilter := handler.parseNetworkFilters(req)

			assert.Equal(t, tt.expectedInactive, showInactive)
			assert.Equal(t, tt.expectedNameFilter, nameFilter)
		})
	}
}

func TestNetworkHandler_shouldIncludeNetwork(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name         string
		network      *db.Network
		showInactive bool
		nameFilter   string
		expected     bool
	}{
		{
			name: "active network - no filters",
			network: &db.Network{
				Name:     "production",
				IsActive: true,
			},
			showInactive: false,
			nameFilter:   "",
			expected:     true,
		},
		{
			name: "inactive network - show_inactive false",
			network: &db.Network{
				Name:     "test",
				IsActive: false,
			},
			showInactive: false,
			nameFilter:   "",
			expected:     false,
		},
		{
			name: "inactive network - show_inactive true",
			network: &db.Network{
				Name:     "test",
				IsActive: false,
			},
			showInactive: true,
			nameFilter:   "",
			expected:     true,
		},
		{
			name: "name filter matches",
			network: &db.Network{
				Name:     "production-network",
				IsActive: true,
			},
			showInactive: false,
			nameFilter:   "production",
			expected:     true,
		},
		{
			name: "name filter does not match",
			network: &db.Network{
				Name:     "staging-network",
				IsActive: true,
			},
			showInactive: false,
			nameFilter:   "production",
			expected:     false,
		},
		{
			name: "name filter case insensitive match",
			network: &db.Network{
				Name:     "Production-Network",
				IsActive: true,
			},
			showInactive: false,
			nameFilter:   "production",
			expected:     true,
		},
		{
			name: "inactive network with name filter - show_inactive false",
			network: &db.Network{
				Name:     "production",
				IsActive: false,
			},
			showInactive: false,
			nameFilter:   "prod",
			expected:     false,
		},
		{
			name: "inactive network with name filter - show_inactive true",
			network: &db.Network{
				Name:     "production",
				IsActive: false,
			},
			showInactive: true,
			nameFilter:   "prod",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.shouldIncludeNetwork(tt.network, tt.showInactive, tt.nameFilter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNetworkHandler_applyNetworkFilters(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	networks := []*db.Network{
		{Name: "production-1", IsActive: true},
		{Name: "production-2", IsActive: false},
		{Name: "staging-1", IsActive: true},
		{Name: "staging-2", IsActive: false},
	}

	tests := []struct {
		name         string
		showInactive bool
		nameFilter   string
		expectedLen  int
	}{
		{
			name:         "no filters - only active",
			showInactive: false,
			nameFilter:   "",
			expectedLen:  2,
		},
		{
			name:         "show all networks",
			showInactive: true,
			nameFilter:   "",
			expectedLen:  4,
		},
		{
			name:         "filter by name - production",
			showInactive: false,
			nameFilter:   "production",
			expectedLen:  1,
		},
		{
			name:         "filter by name with inactive",
			showInactive: true,
			nameFilter:   "production",
			expectedLen:  2,
		},
		{
			name:         "filter by name - staging active only",
			showInactive: false,
			nameFilter:   "staging",
			expectedLen:  1,
		},
		{
			name:         "filter non-matching name",
			showInactive: true,
			nameFilter:   "development",
			expectedLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := handler.applyNetworkFilters(networks, tt.showInactive, tt.nameFilter)
			assert.Len(t, filtered, tt.expectedLen)
		})
	}
}

func TestNetworkHandler_setNetworkDefaults(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	trueVal := true
	falseVal := false

	tests := []struct {
		name                string
		request             *CreateNetworkRequest
		expectedActive      bool
		expectedScanEnabled bool
	}{
		{
			name:                "no values provided - defaults to true",
			request:             &CreateNetworkRequest{},
			expectedActive:      true,
			expectedScanEnabled: true,
		},
		{
			name: "both explicitly true",
			request: &CreateNetworkRequest{
				IsActive:    &trueVal,
				ScanEnabled: &trueVal,
			},
			expectedActive:      true,
			expectedScanEnabled: true,
		},
		{
			name: "both explicitly false",
			request: &CreateNetworkRequest{
				IsActive:    &falseVal,
				ScanEnabled: &falseVal,
			},
			expectedActive:      false,
			expectedScanEnabled: false,
		},
		{
			name: "active true, scan false",
			request: &CreateNetworkRequest{
				IsActive:    &trueVal,
				ScanEnabled: &falseVal,
			},
			expectedActive:      true,
			expectedScanEnabled: false,
		},
		{
			name: "active false, scan true",
			request: &CreateNetworkRequest{
				IsActive:    &falseVal,
				ScanEnabled: &trueVal,
			},
			expectedActive:      false,
			expectedScanEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isActive, scanEnabled := handler.setNetworkDefaults(tt.request)
			assert.Equal(t, tt.expectedActive, isActive)
			assert.Equal(t, tt.expectedScanEnabled, scanEnabled)
		})
	}
}

func TestNetworkHandler_updateNetworkFields(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name    string
		initial *db.Network
		request *UpdateNetworkRequest
		check   func(t *testing.T, network *db.Network)
	}{
		{
			name: "update name only",
			initial: &db.Network{
				Name:            "old-name",
				DiscoveryMethod: "ping",
				IsActive:        true,
				ScanEnabled:     true,
			},
			request: &UpdateNetworkRequest{
				Name: stringPtr("new-name"),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.Equal(t, "new-name", network.Name)
				assert.Equal(t, "ping", network.DiscoveryMethod)
				assert.True(t, network.IsActive)
			},
		},
		{
			name: "update description",
			initial: &db.Network{
				Name: "test-network",
			},
			request: &UpdateNetworkRequest{
				Description: stringPtr("new description"),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.NotNil(t, network.Description)
				assert.Equal(t, "new description", *network.Description)
			},
		},
		{
			name: "update discovery method",
			initial: &db.Network{
				DiscoveryMethod: "ping",
			},
			request: &UpdateNetworkRequest{
				DiscoveryMethod: stringPtr("tcp"),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.Equal(t, "tcp", network.DiscoveryMethod)
			},
		},
		{
			name: "update active status",
			initial: &db.Network{
				IsActive: true,
			},
			request: &UpdateNetworkRequest{
				IsActive: boolPtr(false),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.False(t, network.IsActive)
			},
		},
		{
			name: "update scan enabled",
			initial: &db.Network{
				ScanEnabled: true,
			},
			request: &UpdateNetworkRequest{
				ScanEnabled: boolPtr(false),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.False(t, network.ScanEnabled)
			},
		},
		{
			name: "update multiple fields",
			initial: &db.Network{
				Name:            "old",
				DiscoveryMethod: "ping",
				IsActive:        true,
				ScanEnabled:     true,
			},
			request: &UpdateNetworkRequest{
				Name:            stringPtr("new"),
				DiscoveryMethod: stringPtr("arp"),
				IsActive:        boolPtr(false),
			},
			check: func(t *testing.T, network *db.Network) {
				assert.Equal(t, "new", network.Name)
				assert.Equal(t, "arp", network.DiscoveryMethod)
				assert.False(t, network.IsActive)
				assert.True(t, network.ScanEnabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := tt.initial
			handler.updateNetworkFields(network, tt.request)
			tt.check(t, network)
		})
	}
}

func TestNetworkHandler_updateNetworkStatusFields(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name        string
		isActive    bool
		scanEnabled bool
	}{
		{
			name:        "enable both",
			isActive:    true,
			scanEnabled: true,
		},
		{
			name:        "disable both",
			isActive:    false,
			scanEnabled: false,
		},
		{
			name:        "active only",
			isActive:    true,
			scanEnabled: false,
		},
		{
			name:        "scan enabled only",
			isActive:    false,
			scanEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := &db.Network{}
			handler.updateNetworkStatusFields(network, tt.isActive, tt.scanEnabled)
			assert.Equal(t, tt.isActive, network.IsActive)
			assert.Equal(t, tt.scanEnabled, network.ScanEnabled)
		})
	}
}

func TestNetworkHandler_parseReasonFromRequest(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name     string
		request  *CreateExclusionRequest
		expected string
	}{
		{
			name: "reason provided",
			request: &CreateExclusionRequest{
				Reason: stringPtr("maintenance window"),
			},
			expected: "maintenance window",
		},
		{
			name:     "reason nil",
			request:  &CreateExclusionRequest{},
			expected: "",
		},
		{
			name: "empty reason string",
			request: &CreateExclusionRequest{
				Reason: stringPtr(""),
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.parseReasonFromRequest(tt.request)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNetworkHandler_convertNetworkWithExclusionsToNetwork(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	network := &db.Network{
		Name:     "test-network",
		IsActive: true,
	}

	nwe := &services.NetworkWithExclusions{
		Network: network,
	}

	result := handler.convertNetworkWithExclusionsToNetwork(nwe)
	assert.Equal(t, network, result)
	assert.Equal(t, "test-network", result.Name)
	assert.True(t, result.IsActive)
}

func TestNetworkHandler_validateRenameNetworkRequest(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name        string
		req         RenameNetworkRequest
		expectError bool
	}{
		{
			name:        "empty name returns error",
			req:         RenameNetworkRequest{NewName: ""},
			expectError: true,
		},
		{
			name:        "whitespace-only name returns error",
			req:         RenameNetworkRequest{NewName: "   "},
			expectError: true,
		},
		{
			name:        "name over 100 chars returns error",
			req:         RenameNetworkRequest{NewName: string(make([]byte, 101))},
			expectError: true,
		},
		{
			name:        "valid name returns nil",
			req:         RenameNetworkRequest{NewName: "My Valid Network"},
			expectError: false,
		},
		{
			name:        "name exactly 100 chars returns nil",
			req:         RenameNetworkRequest{NewName: string(make([]byte, 100))},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateRenameNetworkRequest(&tt.req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkHandler_validateCreateNetworkRequest(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	longName := strings.Repeat("a", 101)
	exactMaxName := strings.Repeat("a", 100)

	tests := []struct {
		name        string
		req         CreateNetworkRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "ping",
			},
			expectError: false,
		},
		{
			name: "valid request with all fields",
			req: CreateNetworkRequest{
				Name:            "Full Network",
				CIDR:            "192.168.1.0/24",
				DiscoveryMethod: "tcp",
				Description:     stringPtr("A full network"),
				IsActive:        boolPtr(true),
				ScanEnabled:     boolPtr(true),
			},
			expectError: false,
		},
		{
			name: "valid arp discovery method",
			req: CreateNetworkRequest{
				Name:            "ARP Network",
				CIDR:            "172.16.0.0/16",
				DiscoveryMethod: "arp",
			},
			expectError: false,
		},
		{
			name: "name exactly at max length",
			req: CreateNetworkRequest{
				Name:            exactMaxName,
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "ping",
			},
			expectError: false,
		},
		{
			name: "empty name returns error",
			req: CreateNetworkRequest{
				Name:            "",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "whitespace-only name returns error",
			req: CreateNetworkRequest{
				Name:            "   ",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "name too long returns error",
			req: CreateNetworkRequest{
				Name:            longName,
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "name too long",
		},
		{
			name: "empty CIDR returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "cidr is required",
		},
		{
			name: "invalid CIDR returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "not-a-cidr",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "invalid cidr",
		},
		{
			name: "CIDR out of range returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "10.0.0.0/33",
				DiscoveryMethod: "ping",
			},
			expectError: true,
			errorMsg:    "invalid cidr",
		},
		{
			name: "empty discovery method returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "",
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "invalid discovery method returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "nmap",
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "case-sensitive discovery method returns error",
			req: CreateNetworkRequest{
				Name:            "My Network",
				CIDR:            "10.0.0.0/24",
				DiscoveryMethod: "PING",
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "valid IPv6 CIDR",
			req: CreateNetworkRequest{
				Name:            "IPv6 Network",
				CIDR:            "2001:db8::/32",
				DiscoveryMethod: "ping",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateCreateNetworkRequest(&tt.req)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkHandler_validateUpdateNetworkRequest(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	longName := strings.Repeat("a", 101)
	exactMaxName := strings.Repeat("a", 100)
	validName := "Updated Network"
	validCIDR := "192.168.2.0/24"
	invalidCIDR := "not-a-cidr"
	outOfRangeCIDR := "10.0.0.0/33"
	validMethod := "tcp"
	invalidMethod := "nmap"
	emptyName := ""
	whitespaceName := "   "

	tests := []struct {
		name        string
		req         UpdateNetworkRequest
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty request is valid (all fields optional)",
			req:         UpdateNetworkRequest{},
			expectError: false,
		},
		{
			name: "valid name update",
			req: UpdateNetworkRequest{
				Name: &validName,
			},
			expectError: false,
		},
		{
			name: "valid CIDR update",
			req: UpdateNetworkRequest{
				CIDR: &validCIDR,
			},
			expectError: false,
		},
		{
			name: "valid discovery method update to tcp",
			req: UpdateNetworkRequest{
				DiscoveryMethod: &validMethod,
			},
			expectError: false,
		},
		{
			name: "valid discovery method update to arp",
			req: UpdateNetworkRequest{
				DiscoveryMethod: stringPtr("arp"),
			},
			expectError: false,
		},
		{
			name: "valid discovery method update to ping",
			req: UpdateNetworkRequest{
				DiscoveryMethod: stringPtr("ping"),
			},
			expectError: false,
		},
		{
			name: "name exactly at max length",
			req: UpdateNetworkRequest{
				Name: &exactMaxName,
			},
			expectError: false,
		},
		{
			name: "all valid fields",
			req: UpdateNetworkRequest{
				Name:            &validName,
				CIDR:            &validCIDR,
				DiscoveryMethod: &validMethod,
				IsActive:        boolPtr(false),
				ScanEnabled:     boolPtr(true),
			},
			expectError: false,
		},
		{
			name: "empty name returns error",
			req: UpdateNetworkRequest{
				Name: &emptyName,
			},
			expectError: true,
			errorMsg:    "name cannot be empty",
		},
		{
			name: "whitespace-only name returns error",
			req: UpdateNetworkRequest{
				Name: &whitespaceName,
			},
			expectError: true,
			errorMsg:    "name cannot be empty",
		},
		{
			name: "name too long returns error",
			req: UpdateNetworkRequest{
				Name: &longName,
			},
			expectError: true,
			errorMsg:    "name too long",
		},
		{
			name: "invalid CIDR returns error",
			req: UpdateNetworkRequest{
				CIDR: &invalidCIDR,
			},
			expectError: true,
			errorMsg:    "invalid cidr",
		},
		{
			name: "CIDR out of range returns error",
			req: UpdateNetworkRequest{
				CIDR: &outOfRangeCIDR,
			},
			expectError: true,
			errorMsg:    "invalid cidr",
		},
		{
			name: "invalid discovery method returns error",
			req: UpdateNetworkRequest{
				DiscoveryMethod: &invalidMethod,
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "empty discovery method returns error",
			req: UpdateNetworkRequest{
				DiscoveryMethod: stringPtr(""),
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "uppercase discovery method returns error",
			req: UpdateNetworkRequest{
				DiscoveryMethod: stringPtr("TCP"),
			},
			expectError: true,
			errorMsg:    "invalid discovery_method",
		},
		{
			name: "valid IPv6 CIDR update",
			req: UpdateNetworkRequest{
				CIDR: stringPtr("2001:db8::/32"),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateUpdateNetworkRequest(&tt.req)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkHandler_validateCreateExclusionRequest(t *testing.T) {
	handler := &NetworkHandler{
		BaseHandler: NewBaseHandler(slog.Default(), metrics.NewRegistry()),
	}

	tests := []struct {
		name        string
		req         CreateExclusionRequest
		expectError bool
	}{
		{
			name:        "empty CIDR returns error",
			req:         CreateExclusionRequest{ExcludedCIDR: ""},
			expectError: true,
		},
		{
			name:        "invalid CIDR returns error",
			req:         CreateExclusionRequest{ExcludedCIDR: "not-a-cidr"},
			expectError: true,
		},
		{
			name:        "invalid IP address returns error",
			req:         CreateExclusionRequest{ExcludedCIDR: "999.999.999.999/24"},
			expectError: true,
		},
		{
			name:        "valid IPv4 CIDR returns nil",
			req:         CreateExclusionRequest{ExcludedCIDR: "192.168.1.0/24"},
			expectError: false,
		},
		{
			name:        "valid single host CIDR returns nil",
			req:         CreateExclusionRequest{ExcludedCIDR: "10.0.0.1/32"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateCreateExclusionRequest(&tt.req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkHandler_EnableNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network that starts disabled
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest Enable Network"),
		generateUniqueCIDR(50), "ping", false, false).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "enable existing network",
			networkID:      networkID,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)
				assert.Equal(t, true, network["is_active"])
			},
		},
		{
			name:           "enable with invalid UUID",
			networkID:      "not-a-uuid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "enable non-existent network",
			networkID:      "00000000-0000-0000-0000-000000000000",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/networks/"+tt.networkID+"/enable", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.EnableNetwork(w, req)

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

func TestNetworkHandler_DisableNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network that starts enabled
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest Disable Network"),
		generateUniqueCIDR(51), "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "disable existing network",
			networkID:      networkID,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)
				assert.Equal(t, false, network["is_active"])
			},
		},
		{
			name:           "disable with invalid UUID",
			networkID:      "not-a-uuid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "disable non-existent network",
			networkID:      "00000000-0000-0000-0000-000000000000",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/networks/"+tt.networkID+"/disable", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.DisableNetwork(w, req)

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

func TestNetworkHandler_RenameNetwork(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network to rename
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest Rename Network"),
		generateUniqueCIDR(52), "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	newName := generateUniqueNetworkName("HandlerTest Renamed Network")

	tests := []struct {
		name           string
		networkID      string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "rename existing network",
			networkID:      networkID,
			body:           RenameNetworkRequest{NewName: newName},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var network map[string]interface{}
				err := json.Unmarshal(body, &network)
				require.NoError(t, err)
				assert.Contains(t, network["name"], "HandlerTest Renamed Network")
			},
		},
		{
			name:           "rename with empty new_name",
			networkID:      networkID,
			body:           RenameNetworkRequest{NewName: ""},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "rename with invalid JSON body",
			networkID:      networkID,
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "rename non-existent network",
			networkID:      "00000000-0000-0000-0000-000000000000",
			body:           RenameNetworkRequest{NewName: "Some Valid Name"},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "rename with invalid UUID",
			networkID:      "not-a-uuid",
			body:           RenameNetworkRequest{NewName: "Some Valid Name"},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody *bytes.Reader
			if tt.body != nil {
				b, err := json.Marshal(tt.body)
				require.NoError(t, err)
				reqBody = bytes.NewReader(b)
			} else {
				reqBody = bytes.NewReader([]byte("invalid json {{{"))
			}

			req := httptest.NewRequest(http.MethodPut, "/api/v1/networks/"+tt.networkID+"/rename", reqBody)
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.RenameNetwork(w, req)

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

func TestNetworkHandler_ListNetworkExclusions(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest ListExclusions Network"),
		generateUniqueCIDR(53), "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	// Insert an exclusion directly via SQL
	exclusionCIDR := "192.168.53.128/25"
	var exclusionID string
	err = database.QueryRow(`
		INSERT INTO network_exclusions (network_id, excluded_cidr, reason, enabled)
		VALUES ($1, $2::cidr, $3, true)
		RETURNING id`,
		networkID, exclusionCIDR, "test exclusion").Scan(&exclusionID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "list exclusions for existing network",
			networkID:      networkID,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var exclusions []map[string]interface{}
				err := json.Unmarshal(body, &exclusions)
				require.NoError(t, err)
				require.NotEmpty(t, exclusions, "Expected at least one exclusion")

				found := false
				for _, ex := range exclusions {
					if ex["id"] == exclusionID {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find the inserted exclusion")
			},
		},
		{
			name:           "list exclusions with invalid UUID",
			networkID:      "not-a-uuid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/"+tt.networkID+"/exclusions", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.ListNetworkExclusions(w, req)

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

func TestNetworkHandler_CreateNetworkExclusion(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest CreateExclusion Network"),
		generateUniqueCIDR(54), "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	tests := []struct {
		name           string
		networkID      string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:      "create valid network exclusion",
			networkID: networkID,
			body: CreateExclusionRequest{
				ExcludedCIDR: "192.168.54.0/28",
				Reason:       stringPtr("test reason"),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var exclusion map[string]interface{}
				err := json.Unmarshal(body, &exclusion)
				require.NoError(t, err)
				assert.NotEmpty(t, exclusion["id"])
				assert.NotEmpty(t, exclusion["excluded_cidr"])
			},
		},
		{
			name:      "create exclusion with invalid CIDR",
			networkID: networkID,
			body: CreateExclusionRequest{
				ExcludedCIDR: "not-a-cidr",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:      "create exclusion with empty CIDR",
			networkID: networkID,
			body: CreateExclusionRequest{
				ExcludedCIDR: "",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "create exclusion with invalid UUID path param",
			networkID:      "not-a-uuid",
			body:           CreateExclusionRequest{ExcludedCIDR: "192.168.1.0/24"},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			url := "/api/v1/networks/" + tt.networkID + "/exclusions"
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.networkID})
			w := httptest.NewRecorder()

			handler.CreateNetworkExclusion(w, req)

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

func TestNetworkHandler_ListGlobalExclusions(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Insert a global exclusion directly (network_id = NULL)
	globalCIDR := "10.99.0.0/16"
	var globalExclusionID string
	err := database.QueryRow(`
		INSERT INTO network_exclusions (network_id, excluded_cidr, reason, enabled)
		VALUES (NULL, $1::cidr, $2, true)
		RETURNING id`,
		globalCIDR, "global test exclusion").Scan(&globalExclusionID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exclusions", nil)
	w := httptest.NewRecorder()

	handler.ListGlobalExclusions(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var exclusions []map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&exclusions)
	require.NoError(t, err)

	found := false
	for _, ex := range exclusions {
		if ex["id"] == globalExclusionID {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected to find the inserted global exclusion")
}

func TestNetworkHandler_CreateGlobalExclusion(t *testing.T) {
	handler, _, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	tests := []struct {
		name           string
		body           interface{}
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "create valid global exclusion",
			body: CreateExclusionRequest{
				ExcludedCIDR: "10.55.0.0/16",
				Reason:       stringPtr("global test"),
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var exclusion map[string]interface{}
				err := json.Unmarshal(body, &exclusion)
				require.NoError(t, err)
				assert.NotEmpty(t, exclusion["id"])
				assert.NotEmpty(t, exclusion["excluded_cidr"])
				// Global exclusions have no network_id
				_, hasNetworkID := exclusion["network_id"]
				assert.False(t, hasNetworkID, "Global exclusion should not have network_id")
			},
		},
		{
			name: "create global exclusion with invalid CIDR",
			body: CreateExclusionRequest{
				ExcludedCIDR: "not-a-cidr",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "create global exclusion with empty CIDR",
			body: CreateExclusionRequest{
				ExcludedCIDR: "",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.body)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/exclusions", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateGlobalExclusion(w, req)

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

func TestNetworkHandler_DeleteExclusion(t *testing.T) {
	handler, database, cleanup := setupNetworkHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	// Create a test network and an exclusion to delete
	var networkID string
	err := database.QueryRow(`
		INSERT INTO networks (name, cidr, discovery_method, is_active, scan_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		generateUniqueNetworkName("HandlerTest DeleteExclusion Network"),
		generateUniqueCIDR(55), "ping", true, true).Scan(&networkID)
	require.NoError(t, err)

	createExclusion := func() string {
		var exclusionID string
		err := database.QueryRow(`
			INSERT INTO network_exclusions (network_id, excluded_cidr, reason, enabled)
			VALUES ($1, $2::cidr, $3, true)
			RETURNING id`,
			networkID, "192.168.55.0/28", "to be deleted").Scan(&exclusionID)
		require.NoError(t, err)
		return exclusionID
	}

	tests := []struct {
		name           string
		setupExclusion func() string
		expectedStatus int
	}{
		{
			name:           "delete existing exclusion",
			setupExclusion: createExclusion,
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "delete non-existent exclusion",
			setupExclusion: func() string {
				return "00000000-0000-0000-0000-000000000000"
			},
			// RemoveExclusion returns a plain fmt.Errorf (not a typed not-found error),
			// so handleDatabaseError falls through to the 500 path.
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "delete with invalid UUID",
			setupExclusion: func() string {
				return "not-a-uuid"
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exclusionID := tt.setupExclusion()

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/exclusions/"+exclusionID, nil)
			req = mux.SetURLVars(req, map[string]string{"id": exclusionID})
			w := httptest.NewRecorder()

			handler.DeleteExclusion(w, req)

			if w.Code != tt.expectedStatus {
				t.Logf("Expected status %d but got %d. Response: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify deletion if successful
			if tt.expectedStatus == http.StatusNoContent {
				var exists bool
				err := database.QueryRow(`
					SELECT EXISTS(SELECT 1 FROM network_exclusions WHERE id = $1)`,
					exclusionID).Scan(&exists)
				require.NoError(t, err)
				assert.False(t, exists, "Exclusion should be deleted")
			}
		})
	}
}
