// Package cli provides CLI authentication helpers for making API calls.
// This file implements HTTP client functionality with API key authentication
// for CLI commands that interact with the Scanorama API server.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/config"
)

// HTTP status code constants
const (
	StatusBadRequest          = 400
	StatusUnauthorized        = 401
	StatusForbidden           = 403
	StatusNotFound            = 404
	StatusTooManyRequests     = 429
	StatusInternalServerError = 500
)

// APIClient provides authenticated HTTP client functionality for CLI commands
type APIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

// APIResponse represents a standard API response structure
type APIResponse struct {
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Message   string      `json:"message,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp time.Time   `json:"timestamp,omitempty"`
}

// APIError represents an API error response
type APIError struct {
	StatusCode int
	Message    string
	RequestID  string
	Response   *APIResponse
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("API error (status %d, request %s): %s", e.StatusCode, e.RequestID, e.Message)
	}
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// NewAPIClient creates a new API client with authentication
func NewAPIClient() (*APIClient, error) {
	// Load configuration
	cfg, err := config.Load(getConfigFilePath())
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get API key from environment variables
	apiKey := getAPIKeyFromSources()
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured. Set SCANORAMA_API_KEY environment variable")
	}

	// Build base URL
	baseURL := fmt.Sprintf("http://%s/api/v1", cfg.GetAPIAddress())
	if cfg.API.TLS.Enabled {
		baseURL = fmt.Sprintf("https://%s/api/v1", cfg.GetAPIAddress())
	}

	// Create HTTP client with reasonable timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
			DisableKeepAlives:  false,
		},
	}

	return &APIClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
		userAgent:  "scanorama-cli/1.0",
	}, nil
}

// getAPIKeyFromSources retrieves API key from environment variables only
func getAPIKeyFromSources() string {
	// 1. Environment variable (highest priority)
	if key := os.Getenv("SCANORAMA_API_KEY"); key != "" {
		return key
	}

	// 2. CLI-specific environment variable
	if key := os.Getenv("SCANORAMA_CLI_API_KEY"); key != "" {
		return key
	}

	// 3. Check for API key file (for security)
	if keyFile := os.Getenv("SCANORAMA_API_KEY_FILE"); keyFile != "" {
		// #nosec G304 - Intentional file reading for API key configuration
		// Basic validation to prevent obvious path traversal
		if !strings.Contains(keyFile, "..") && keyFile != "" {
			if keyData, err := os.ReadFile(keyFile); err == nil {
				return strings.TrimSpace(string(keyData))
			}
		}
	}

	return ""
}

// Get performs a GET request to the specified endpoint
func (c *APIClient) Get(endpoint string) (*APIResponse, error) {
	return c.request("GET", endpoint, nil)
}

// Post performs a POST request with JSON payload
func (c *APIClient) Post(endpoint string, payload interface{}) (*APIResponse, error) {
	return c.request("POST", endpoint, payload)
}

// Put performs a PUT request with JSON payload
func (c *APIClient) Put(endpoint string, payload interface{}) (*APIResponse, error) {
	return c.request("PUT", endpoint, payload)
}

// Delete performs a DELETE request
func (c *APIClient) Delete(endpoint string) (*APIResponse, error) {
	return c.request("DELETE", endpoint, nil)
}

// request performs the actual HTTP request with authentication
func (c *APIClient) request(method, endpoint string, payload interface{}) (*APIResponse, error) {
	url := c.baseURL + endpoint

	// Prepare request body
	var requestBody io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	// Create HTTP request
	req, err := http.NewRequest(method, url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-API-Key", c.apiKey) // Primary auth method

	// Also set Authorization header as backup
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Add request ID for tracking
	req.Header.Set("X-Request-Source", "cli")

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response
	var apiResp APIResponse
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
			// If JSON parsing fails, treat as plain text error
			apiResp.Error = string(bodyBytes)
		}
	}

	// Handle HTTP error status codes
	if resp.StatusCode >= StatusBadRequest {
		errorMsg := apiResp.Error
		if errorMsg == "" {
			errorMsg = apiResp.Message
		}
		if errorMsg == "" {
			errorMsg = fmt.Sprintf("HTTP %d error", resp.StatusCode)
		}

		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    errorMsg,
			RequestID:  apiResp.RequestID,
			Response:   &apiResp,
		}
	}

	return &apiResp, nil
}

// TestConnection tests the API connection and authentication
func (c *APIClient) TestConnection() error {
	resp, err := c.Get("/health")
	if err != nil {
		return fmt.Errorf("API connection test failed: %w", err)
	}

	if resp.Error != "" {
		return fmt.Errorf("API health check failed: %s", resp.Error)
	}

	return nil
}

// GetServerInfo retrieves server information (requires authentication)
func (c *APIClient) GetServerInfo() (map[string]interface{}, error) {
	resp, err := c.Get("/admin/status")
	if err != nil {
		return nil, fmt.Errorf("failed to get server info: %w", err)
	}

	if data, ok := resp.Data.(map[string]interface{}); ok {
		return data, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// Helper functions for common CLI operations

// mustCreateAPIClient creates an API client or exits with error
func mustCreateAPIClient() *APIClient {
	client, err := NewAPIClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nTo configure API authentication:\n")
		fmt.Fprintf(os.Stderr, "  1. Create an API key: scanorama apikeys create --name \"CLI Access\"\n")
		fmt.Fprintf(os.Stderr, "  2. Set environment variable: export SCANORAMA_API_KEY=your_key_here\n")
		fmt.Fprintf(os.Stderr, "  3. Or use key file: export SCANORAMA_API_KEY_FILE=/path/to/keyfile\n")
		os.Exit(1)
	}
	return client
}

// handleAPIError provides user-friendly error handling for API errors
func handleAPIError(err error, operation string) {
	if apiErr, ok := err.(*APIError); ok {
		switch apiErr.StatusCode {
		case StatusUnauthorized:
			fmt.Fprintf(os.Stderr, "Error: Authentication failed for %s\n", operation)
			fmt.Fprintf(os.Stderr, "Please check your API key configuration.\n")
			if apiErr.RequestID != "" {
				fmt.Fprintf(os.Stderr, "Request ID: %s\n", apiErr.RequestID)
			}
		case StatusForbidden:
			fmt.Fprintf(os.Stderr, "Error: Insufficient permissions for %s\n", operation)
			fmt.Fprintf(os.Stderr, "Your API key does not have the required permissions.\n")
		case StatusNotFound:
			fmt.Fprintf(os.Stderr, "Error: Resource not found for %s\n", operation)
		case StatusTooManyRequests:
			fmt.Fprintf(os.Stderr, "Error: Rate limit exceeded for %s\n", operation)
			fmt.Fprintf(os.Stderr, "Please wait a moment and try again.\n")
		case StatusInternalServerError:
			fmt.Fprintf(os.Stderr, "Error: Server error during %s\n", operation)
			if apiErr.RequestID != "" {
				fmt.Fprintf(os.Stderr, "Please report this issue with request ID: %s\n", apiErr.RequestID)
			}
		default:
			fmt.Fprintf(os.Stderr, "Error: %s failed: %s\n", operation, apiErr.Message)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s failed: %v\n", operation, err)
	}
}

// WithAPIClient is a helper for commands that need API access
func WithAPIClient(operation string, fn func(*APIClient) error) error {
	client := mustCreateAPIClient()

	if err := fn(client); err != nil {
		handleAPIError(err, operation)
		return err
	}

	return nil
}

// printAPIKeyUsageHelp prints help text for API key configuration
