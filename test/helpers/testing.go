// Package helpers provides common testing utilities for the Scanorama project.
// It includes helpers for database setup, mock servers, network testing, and test data generation.
package helpers

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	pollInterval       = 100 * time.Millisecond
	defaultTestTimeout = 30 * time.Second
	dnsPort            = 53
)

// TestDatabase provides utilities for database testing
type TestDatabase struct {
	DB       *sqlx.DB
	URL      string
	Host     string
	Port     string
	Name     string
	User     string
	Password string
}

// NewTestDatabase creates a new test database connection
// It checks for environment variables or uses defaults suitable for testing
func NewTestDatabase(t *testing.T) *TestDatabase {
	t.Helper()

	host := getEnvOrDefault("POSTGRES_HOST", "localhost")
	port := getEnvOrDefault("POSTGRES_PORT", "5432")
	dbname := getEnvOrDefault("POSTGRES_DB", "scanorama_test")
	user := getEnvOrDefault("POSTGRES_USER", "scanorama")
	password := getEnvOrDefault("POSTGRES_PASSWORD", "scanorama123")
	sslmode := getEnvOrDefault("POSTGRES_SSLMODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		host, port, dbname, user, password, sslmode)

	sqlDB, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Skipf("Skipping test requiring database: %v", err)
		return nil
	}

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		t.Skipf("Skipping test - database not responding: %v", err)
		return nil
	}

	return &TestDatabase{
		DB:       sqlDB,
		URL:      dsn,
		Host:     host,
		Port:     port,
		Name:     dbname,
		User:     user,
		Password: password,
	}
}

// Close closes the database connection
func (td *TestDatabase) Close() {
	if td.DB != nil {
		_ = td.DB.Close()
	}
}

// Cleanup removes test data from the database
func (td *TestDatabase) Cleanup(t *testing.T) {
	t.Helper()
	if td.DB == nil {
		return
	}

	// Clean up test tables in reverse dependency order
	tables := []string{
		"scans",
		"hosts",
		"profiles",
		"networks",
	}

	for _, table := range tables {
		_, err := td.DB.Exec(fmt.Sprintf("DELETE FROM %s WHERE created_at > NOW() - INTERVAL '1 hour'", table))
		if err != nil {
			t.Logf("Warning: failed to cleanup table %s: %v", table, err)
		}
	}
}

// MockHTTPServer provides utilities for HTTP testing
type MockHTTPServer struct {
	Server *httptest.Server
	URL    string
	Port   int
}

// NewMockHTTPServer creates a new mock HTTP server for testing
func NewMockHTTPServer(handler http.Handler) *MockHTTPServer {
	server := httptest.NewServer(handler)

	// Extract port from server URL
	_, portStr, _ := net.SplitHostPort(server.URL[7:]) // Remove "http://" prefix
	port, _ := strconv.Atoi(portStr)

	return &MockHTTPServer{
		Server: server,
		URL:    server.URL,
		Port:   port,
	}
}

// Close closes the mock server
func (ms *MockHTTPServer) Close() {
	if ms.Server != nil {
		ms.Server.Close()
	}
}

// NetworkTestHelper provides utilities for network testing
type NetworkTestHelper struct {
	OpenPorts   []int
	ClosedPorts []int
	MockServers []*MockHTTPServer
}

// NewNetworkTestHelper creates a new network test helper
func NewNetworkTestHelper() *NetworkTestHelper {
	return &NetworkTestHelper{
		OpenPorts:   []int{},
		ClosedPorts: []int{},
		MockServers: []*MockHTTPServer{},
	}
}

// SetupTestDB sets up a test database connection for integration tests
func SetupTestDB(t testing.TB) *db.DB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	database, _, err := ConnectToTestDatabase(ctx)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}

	// Ensure schema is ready
	if err := EnsureTestSchema(ctx, database); err != nil {
		t.Fatalf("Failed to ensure test schema: %v", err)
	}

	return database
}

// CleanupTestDB cleans up test database resources
func CleanupTestDB(t testing.TB, database *db.DB) {
	t.Helper()

	if database != nil {
		_ = database.Close()
	}
}

// StartMockService starts a mock service on a random available port
func (nth *NetworkTestHelper) StartMockService(handler http.Handler) *MockHTTPServer {
	server := NewMockHTTPServer(handler)
	nth.MockServers = append(nth.MockServers, server)
	nth.OpenPorts = append(nth.OpenPorts, server.Port)
	return server
}

// Cleanup closes all mock servers
func (nth *NetworkTestHelper) Cleanup() {
	for _, server := range nth.MockServers {
		server.Close()
	}
	nth.MockServers = nil
	nth.OpenPorts = nil
	nth.ClosedPorts = nil
}

// GetAvailablePort returns an available port for testing
func GetAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()

	return l.Addr().(*net.TCPAddr).Port, nil
}

// WaitForPort waits for a port to become available or timeout
func WaitForPort(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("port %s:%d not available after %v", host, port, timeout)
}

// IsPortOpen checks if a port is open
func IsPortOpen(host string, port int) bool {
	timeout := time.Second
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// TestContext provides a context with reasonable timeout for tests
func TestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		timeout = defaultTestTimeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// SkipIfShort skips a test if running with -short flag
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping test in short mode: %s", reason)
	}
}

// RequireNetwork skips test if network is not available
func RequireNetwork(t *testing.T) {
	t.Helper()
	if !IsPortOpen("8.8.8.8", dnsPort) {
		t.Skip("Skipping test: network not available")
	}
}

// RequireDocker skips test if Docker is not available
func RequireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("SKIP_DOCKER_TESTS") == "true" {
		t.Skip("Skipping Docker test: SKIP_DOCKER_TESTS=true")
	}
}

// LogTestName logs the test name for debugging
func LogTestName(t *testing.T) {
	t.Helper()
	if os.Getenv("DEBUG_TESTS") == "true" {
		log.Printf("=== Running test: %s ===", t.Name())
	}
}

// SimpleHTTPHandler returns a simple HTTP handler for testing
func SimpleHTTPHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}
}

// JSONHandler returns an HTTP handler that serves JSON
func JSONHandler(status int, jsonBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if jsonBody != "" {
			_, _ = w.Write([]byte(jsonBody))
		}
	}
}

// TestCleanup provides a standard cleanup function for tests
type TestCleanup struct {
	funcs []func()
}

// NewTestCleanup creates a new test cleanup helper
func NewTestCleanup() *TestCleanup {
	return &TestCleanup{}
}

// Add adds a cleanup function
func (tc *TestCleanup) Add(f func()) {
	tc.funcs = append(tc.funcs, f)
}

// Execute runs all cleanup functions in reverse order
func (tc *TestCleanup) Execute() {
	for i := len(tc.funcs) - 1; i >= 0; i-- {
		tc.funcs[i]()
	}
}

// SetupTestDatabase is a convenience function for common database test setup
func SetupTestDatabase(t *testing.T) (*TestDatabase, *TestCleanup) {
	t.Helper()

	cleanup := NewTestCleanup()
	testDB := NewTestDatabase(t)
	if testDB == nil {
		return nil, cleanup
	}

	cleanup.Add(func() {
		testDB.Close()
	})

	return testDB, cleanup
}

// SetupNetworkTest is a convenience function for network test setup
func SetupNetworkTest(t *testing.T) (*NetworkTestHelper, *TestCleanup) {
	t.Helper()
	RequireNetwork(t)

	cleanup := NewTestCleanup()
	helper := NewNetworkTestHelper()

	cleanup.Add(func() {
		helper.Cleanup()
	})

	return helper, cleanup
}
