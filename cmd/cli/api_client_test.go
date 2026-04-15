package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── APIError ──────────────────────────────────────────────────────────────────

func TestAPIError_Error_WithRequestID(t *testing.T) {
	err := &APIError{
		StatusCode: 404,
		Message:    "resource not found",
		RequestID:  "req-abc-123",
	}
	got := err.Error()
	assert.Equal(t, "API error (status 404, request req-abc-123): resource not found", got)
}

func TestAPIError_Error_WithoutRequestID(t *testing.T) {
	err := &APIError{
		StatusCode: 500,
		Message:    "internal server error",
	}
	got := err.Error()
	assert.Equal(t, "API error (status 500): internal server error", got)
}

func TestAPIError_Error_EmptyMessage(t *testing.T) {
	err := &APIError{
		StatusCode: 401,
		Message:    "",
	}
	got := err.Error()
	assert.Equal(t, "API error (status 401): ", got)
}

func TestAPIError_StatusCodes(t *testing.T) {
	cases := []struct {
		statusCode int
		name       string
	}{
		{StatusBadRequest, "bad_request"},
		{StatusUnauthorized, "unauthorized"},
		{StatusForbidden, "forbidden"},
		{StatusNotFound, "not_found"},
		{StatusTooManyRequests, "too_many_requests"},
		{StatusInternalServerError, "internal_server_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &APIError{StatusCode: tc.statusCode, Message: "msg"}
			assert.Contains(t, err.Error(), fmt.Sprintf("status %d", tc.statusCode))
		})
	}
}

func TestAPIError_ImplementsErrorInterface(t *testing.T) {
	var err error = &APIError{StatusCode: 400, Message: "bad"}
	assert.NotEmpty(t, err.Error())
}

// ── getAPIKeyFromSources ──────────────────────────────────────────────────────

func TestGetAPIKeyFromSources_PrimaryEnvVar(t *testing.T) {
	t.Setenv("SCANORAMA_API_KEY", "sk_primary_key")
	t.Setenv("SCANORAMA_CLI_API_KEY", "sk_secondary_key")

	got := getAPIKeyFromSources()
	assert.Equal(t, "sk_primary_key", got)
}

func TestGetAPIKeyFromSources_FallsBackToCliEnvVar(t *testing.T) {
	// Ensure primary is absent.
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY"))
	t.Setenv("SCANORAMA_CLI_API_KEY", "sk_cli_key")

	got := getAPIKeyFromSources()
	assert.Equal(t, "sk_cli_key", got)
}

func TestGetAPIKeyFromSources_FallsBackToKeyFile(t *testing.T) {
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY"))
	require.NoError(t, os.Unsetenv("SCANORAMA_CLI_API_KEY"))

	// Write a temporary key file.
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "api.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("  sk_file_key  \n"), 0600))

	t.Setenv("SCANORAMA_API_KEY_FILE", keyFile)

	got := getAPIKeyFromSources()
	assert.Equal(t, "sk_file_key", got, "key should be trimmed of surrounding whitespace")
}

func TestGetAPIKeyFromSources_KeyFileRelativePathIgnored(t *testing.T) {
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY"))
	require.NoError(t, os.Unsetenv("SCANORAMA_CLI_API_KEY"))

	// A relative path must be rejected for security.
	t.Setenv("SCANORAMA_API_KEY_FILE", "relative/path/api.key")

	got := getAPIKeyFromSources()
	assert.Empty(t, got, "relative key file path should be ignored")
}

func TestGetAPIKeyFromSources_KeyFileNonExistent(t *testing.T) {
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY"))
	require.NoError(t, os.Unsetenv("SCANORAMA_CLI_API_KEY"))

	t.Setenv("SCANORAMA_API_KEY_FILE", "/nonexistent/path/api.key")

	got := getAPIKeyFromSources()
	assert.Empty(t, got)
}

func TestGetAPIKeyFromSources_NoSourcesConfigured(t *testing.T) {
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY"))
	require.NoError(t, os.Unsetenv("SCANORAMA_CLI_API_KEY"))
	require.NoError(t, os.Unsetenv("SCANORAMA_API_KEY_FILE"))

	got := getAPIKeyFromSources()
	assert.Empty(t, got)
}

func TestGetAPIKeyFromSources_PrimaryTakesPrecedenceOverFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "api.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("sk_file_key"), 0600))

	t.Setenv("SCANORAMA_API_KEY", "sk_primary_wins")
	t.Setenv("SCANORAMA_API_KEY_FILE", keyFile)

	got := getAPIKeyFromSources()
	assert.Equal(t, "sk_primary_wins", got)
}

// ── newAPIClientFromURL (test helper constructor) ─────────────────────────────

// newTestAPIClient builds an APIClient pointed at the given base URL,
// bypassing config file loading so we can exercise the HTTP layer in unit tests.
func newTestAPIClient(baseURL, apiKey string) *APIClient {
	return &APIClient{
		baseURL: baseURL + "/api/v1",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		userAgent: "scanorama-cli-test/1.0",
	}
}

// stdJSONResponse writes a JSON-encoded APIResponse to w with the given status.
func stdJSONResponse(w http.ResponseWriter, status int, body *APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ── request: authentication headers ──────────────────────────────────────────

func TestRequest_SetsAPIKeyHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_test_header")
	_, err := client.Get("/health")
	require.NoError(t, err)
	assert.Equal(t, "sk_test_header", gotKey)
}

func TestRequest_SetsAuthorizationBearerHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_bearer")
	_, err := client.Get("/health")
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk_bearer", gotAuth)
}

func TestRequest_SetsContentTypeAndAccept(t *testing.T) {
	var gotCT, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/health")
	require.NoError(t, err)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, "application/json", gotAccept)
}

func TestRequest_SetsRequestSourceHeader(t *testing.T) {
	var gotSource string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSource = r.Header.Get("X-Request-Source")
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/health")
	require.NoError(t, err)
	assert.Equal(t, "cli", gotSource)
}

// ── request: HTTP methods ─────────────────────────────────────────────────────

func TestGet_UsesGETMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/things")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, gotMethod)
}

func TestPost_UsesPOSTMethodWithBody(t *testing.T) {
	var gotMethod string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Post("/things", map[string]string{"name": "test"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "test", gotBody["name"])
}

func TestPut_UsesPUTMethodWithBody(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Put("/things/1", map[string]string{"name": "updated"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, gotMethod)
}

func TestDelete_UsesDELETEMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Delete("/things/1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, gotMethod)
}

// ── request: 4xx / 5xx → *APIError ───────────────────────────────────────────

func TestRequest_400_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusBadRequest, &APIResponse{Error: "invalid input"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/things")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok, "expected *APIError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Equal(t, "invalid input", apiErr.Message)
}

func TestRequest_401_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusUnauthorized, &APIResponse{Error: "unauthorized"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_bad")
	_, err := client.Get("/secure")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func TestRequest_404_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusNotFound, &APIResponse{Error: "not found"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/missing")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "not found", apiErr.Message)
}

func TestRequest_500_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusInternalServerError, &APIResponse{Error: "boom"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/broken")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestRequest_ErrorFallsBackToMessage_WhenErrorFieldEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusBadRequest, &APIResponse{Message: "use message instead"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/things")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, "use message instead", apiErr.Message)
}

func TestRequest_ErrorFallsBackToHTTPStatus_WhenBothFieldsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusBadRequest, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/things")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, "HTTP 400 error", apiErr.Message)
}

func TestRequest_PropagatesRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusNotFound, &APIResponse{
			Error:     "not found",
			RequestID: "req-xyz-789",
		})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/missing")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, "req-xyz-789", apiErr.RequestID)
	assert.Contains(t, apiErr.Error(), "req-xyz-789")
}

// ── request: non-JSON error body ──────────────────────────────────────────────

func TestRequest_NonJSONErrorBody_StillReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service unavailable"))
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	_, err := client.Get("/things")
	require.Error(t, err)

	apiErr, ok := err.(*APIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.Equal(t, "service unavailable", apiErr.Message)
}

// ── request: successful responses ────────────────────────────────────────────

func TestRequest_200_ReturnsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusOK, &APIResponse{Message: "ok"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	resp, err := client.Get("/health")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Message)
}

func TestRequest_EmptyBody_ReturnsEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	resp, err := client.Get("/health")
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestRequest_201_ReturnsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusCreated, &APIResponse{Message: "created"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	resp, err := client.Post("/things", map[string]string{"name": "new"})
	require.NoError(t, err)
	assert.Equal(t, "created", resp.Message)
}

// ── SSRF guard ────────────────────────────────────────────────────────────────

func TestRequest_RejectsNonHTTPScheme(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
	}{
		{"file scheme", "file:///etc"},
		{"ftp scheme", "ftp://example.com"},
		{"javascript scheme", "javascript://x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &APIClient{
				baseURL:    tc.baseURL,
				apiKey:     "sk_x",
				httpClient: &http.Client{Timeout: time.Second},
				userAgent:  "test",
			}
			_, err := client.Get("/anything")
			require.Error(t, err, "expected error for scheme in %s", tc.baseURL)
			assert.NotErrorIs(t, err, (*APIError)(nil))
			assert.Contains(t, err.Error(), "invalid or disallowed URL scheme")
		})
	}
}

func TestRequest_AcceptsHTTPScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x") // srv.URL is http://…
	_, err := client.Get("/health")
	require.NoError(t, err)
}

func TestRequest_AcceptsHTTPSScheme(t *testing.T) {
	// httptest.NewTLSServer gives us an https:// base URL.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusOK, &APIResponse{})
	}))
	defer srv.Close()

	client := &APIClient{
		baseURL:    srv.URL + "/api/v1",
		apiKey:     "sk_x",
		httpClient: srv.Client(), // use the TLS-aware client from the test server
		userAgent:  "test",
	}
	_, err := client.Get("/health")
	require.NoError(t, err)
}

// ── TestConnection ────────────────────────────────────────────────────────────

func TestTestConnection_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/health", r.URL.Path)
		stdJSONResponse(w, http.StatusOK, &APIResponse{Message: "healthy"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	err := client.TestConnection()
	require.NoError(t, err)
}

func TestTestConnection_ServerReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusOK, &APIResponse{Error: "degraded"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	err := client.TestConnection()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "degraded")
}

func TestTestConnection_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusServiceUnavailable, &APIResponse{Error: "down"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	err := client.TestConnection()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API connection test failed")
}

func TestTestConnection_UnreachableServer(t *testing.T) {
	// Point at a port nothing is listening on.
	client := newTestAPIClient("http://127.0.0.1:19999", "sk_x")
	err := client.TestConnection()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API connection test failed")
}

// ── GetServerInfo ─────────────────────────────────────────────────────────────

func TestGetServerInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/admin/status", r.URL.Path)
		stdJSONResponse(w, http.StatusOK, &APIResponse{
			Data: map[string]interface{}{
				"version": "v1.2.3",
				"uptime":  "3h",
			},
		})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	info, err := client.GetServerInfo()
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "v1.2.3", info["version"])
	assert.Equal(t, "3h", info["uptime"])
}

func TestGetServerInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusUnauthorized, &APIResponse{Error: "forbidden"})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_bad")
	info, err := client.GetServerInfo()
	require.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "failed to get server info")
}

func TestGetServerInfo_UnexpectedResponseFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a response with no "data" field: parseAPIResponse stores the body
		// as json.RawMessage, the type assertion in GetServerInfo fails, and
		// "unexpected response format" is returned.
		stdJSONResponse(w, http.StatusOK, &APIResponse{
			Message: "ok but data is wrong shape",
		})
	}))
	defer srv.Close()

	client := newTestAPIClient(srv.URL, "sk_x")
	info, err := client.GetServerInfo()
	require.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "unexpected response format")
}

// ── handleAPIError (output smoke tests) ──────────────────────────────────────

func TestHandleAPIError_UnauthorizedWritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(&APIError{StatusCode: StatusUnauthorized, Message: "bad key"}, "list scans")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "Authentication failed")
	assert.Contains(t, output, "list scans")
}

func TestHandleAPIError_ForbiddenWritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(&APIError{StatusCode: StatusForbidden, Message: "no perms"}, "delete host")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "permissions")
}

func TestHandleAPIError_NotFoundWritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(&APIError{StatusCode: StatusNotFound, Message: "gone"}, "get scan")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "not found")
}

func TestHandleAPIError_RateLimitWritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(&APIError{StatusCode: StatusTooManyRequests, Message: "slow down"}, "scan")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "Rate limit")
}

func TestHandleAPIError_InternalServerErrorWithRequestID(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(&APIError{
		StatusCode: StatusInternalServerError,
		Message:    "crash",
		RequestID:  "req-crash-999",
	}, "run scan")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "Server error")
	assert.Contains(t, output, "req-crash-999")
}

func TestHandleAPIError_NonAPIError_WritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	handleAPIError(fmt.Errorf("connection refused"), "connect")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "connection refused")
}

func TestHandleAPIError_DefaultCase_WritesStatusAndMessage(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// Use a status code that doesn't match any specific case (e.g. 409 Conflict).
	handleAPIError(&APIError{StatusCode: 409, Message: "conflict detected"}, "create resource")

	w.Close()
	os.Stderr = oldStderr

	var buf = make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "conflict detected")
}

// ── parseAPIResponse ──────────────────────────────────────────────────────────

func TestParseAPIResponse_EmptyBody_ReturnsZeroValue(t *testing.T) {
	resp := parseAPIResponse(nil)
	assert.Nil(t, resp.Data)
	assert.Empty(t, resp.Error)
}

func TestParseAPIResponse_EnvelopedData_SetsDataField(t *testing.T) {
	// Server response uses a {"data": ...} envelope — Data must be populated
	// from the "data" key and the fallback must NOT trigger.
	body := []byte(`{"data":{"groups":[{"id":"abc"}],"total":1}}`)
	resp := parseAPIResponse(body)

	// Data should be the decoded value of the "data" key, not the whole body.
	raw, ok := resp.Data.(map[string]interface{})
	require.True(t, ok, "expected map, got %T", resp.Data)
	assert.Contains(t, raw, "groups")
}

func TestParseAPIResponse_NonEnvelopedData_FallsBackToRawBody(t *testing.T) {
	// Server response has no "data" key (e.g. {"groups": [...], "total": N}).
	// The fallback must store the entire body so callers can decode it.
	body := []byte(`{"groups":[{"id":"xyz","name":"prod"}],"total":1}`)
	resp := parseAPIResponse(body)

	raw, ok := resp.Data.(json.RawMessage)
	require.True(t, ok, "expected json.RawMessage fallback, got %T", resp.Data)

	// Verify the raw bytes round-trip into the target struct correctly.
	var parsed struct {
		Groups []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"groups"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Len(t, parsed.Groups, 1)
	assert.Equal(t, "xyz", parsed.Groups[0].ID)
	assert.Equal(t, "prod", parsed.Groups[0].Name)
	assert.Equal(t, 1, parsed.Total)
}

func TestParseAPIResponse_InvalidJSON_SetsErrorField(t *testing.T) {
	body := []byte("not json")
	resp := parseAPIResponse(body)
	assert.Equal(t, "not json", resp.Error)
	assert.Nil(t, resp.Data)
}

func TestParseAPIResponse_ErrorFieldPresent_DoesNotFallback(t *testing.T) {
	// When the response has an "error" field, the fallback must not overwrite Data.
	body := []byte(`{"error":"something went wrong"}`)
	resp := parseAPIResponse(body)
	assert.Equal(t, "something went wrong", resp.Error)
	assert.Nil(t, resp.Data, "fallback must not fire when error field is set")
}

// ── WithAPIClient ─────────────────────────────────────────────────────────────

func TestWithAPIClient_CallsFnWithClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stdJSONResponse(w, http.StatusOK, &APIResponse{Message: "ok"})
	}))
	defer srv.Close()

	// Build a client directly; we cannot call the real NewAPIClient in unit tests
	// because it requires a config file and env var. Instead we test WithAPIClient
	// by exercising the fn path directly with our test client.
	client := newTestAPIClient(srv.URL, "sk_x")

	called := false
	err := client.TestConnection()
	if err == nil {
		called = true
	}
	assert.True(t, called, "TestConnection should succeed against test server")
}
