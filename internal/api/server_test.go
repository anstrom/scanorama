package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/config"
)

const (
	testPayloadSize1MB = 1024 * 1024 // 1MB test payload size
)

// MockDB provides a mock database for testing
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Test helper functions
func createTestConfig() *config.Config {
	return &config.Config{
		API: config.APIConfig{
			Host:              "localhost",
			Port:              8080,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			MaxHeaderBytes:    1048576,
			EnableCORS:        true,
			CORSOrigins:       []string{"*"},
			RateLimitEnabled:  true,
			RateLimitRequests: 100,
			RateLimitWindow:   time.Minute,
			AuthEnabled:       false,
			APIKeys:           []string{},
		},
	}
}

func createTestDatabase() *MockDB {
	return &MockDB{}
}

func TestDefaultConfig(t *testing.T) {
	t.Run("returns valid default configuration", func(t *testing.T) {
		cfg := DefaultConfig()

		assert.Equal(t, "127.0.0.1", cfg.Host)
		assert.Equal(t, 8080, cfg.Port)
		assert.Equal(t, 10*time.Second, cfg.ReadTimeout)
		assert.Equal(t, 10*time.Second, cfg.WriteTimeout)
		assert.Equal(t, 60*time.Second, cfg.IdleTimeout)
		assert.Equal(t, 1<<20, cfg.MaxHeaderBytes) // 1MB
		assert.True(t, cfg.EnableCORS)
		assert.Equal(t, []string{"*"}, cfg.CORSOrigins)
		assert.True(t, cfg.RateLimitEnabled)
		assert.Equal(t, 100, cfg.RateLimitRequests)
		assert.Equal(t, time.Minute, cfg.RateLimitWindow)
		assert.False(t, cfg.AuthEnabled)
		assert.Empty(t, cfg.APIKeys)
	})

	t.Run("configuration values are reasonable", func(t *testing.T) {
		cfg := DefaultConfig()

		// Validate timeout values are positive and reasonable
		assert.Positive(t, cfg.ReadTimeout)
		assert.Positive(t, cfg.WriteTimeout)
		assert.Positive(t, cfg.IdleTimeout)
		assert.LessOrEqual(t, cfg.ReadTimeout, 5*time.Minute)
		assert.LessOrEqual(t, cfg.WriteTimeout, 5*time.Minute)

		// Validate rate limiting is sensible
		assert.Positive(t, cfg.RateLimitRequests)
		assert.Positive(t, cfg.RateLimitWindow)
		assert.LessOrEqual(t, cfg.RateLimitRequests, 10000) // Not too permissive

		// Validate max header size is reasonable
		assert.Positive(t, cfg.MaxHeaderBytes)
		assert.LessOrEqual(t, cfg.MaxHeaderBytes, 10<<20) // Not more than 10MB
	})
}

func TestNewServer(t *testing.T) {
	t.Run("creates server with valid configuration", func(t *testing.T) {
		cfg := createTestConfig()

		server, err := New(cfg, nil)

		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.NotNil(t, server.router)
		assert.Equal(t, cfg, server.config)
		assert.Nil(t, server.database)
		assert.NotNil(t, server.logger)
		assert.NotNil(t, server.metrics)
		assert.NotNil(t, server.httpServer)
		assert.False(t, server.startTime.IsZero())
	})

	t.Run("configures HTTP server correctly", func(t *testing.T) {
		cfg := createTestConfig()

		server, err := New(cfg, nil)

		require.NoError(t, err)
		assert.Equal(t, "localhost:8080", server.httpServer.Addr)
		assert.Equal(t, cfg.API.ReadTimeout, server.httpServer.ReadTimeout)
		assert.Equal(t, cfg.API.WriteTimeout, server.httpServer.WriteTimeout)
		assert.Equal(t, cfg.API.IdleTimeout, server.httpServer.IdleTimeout)
		assert.Equal(t, cfg.API.MaxHeaderBytes, server.httpServer.MaxHeaderBytes)
		assert.Equal(t, server.router, server.httpServer.Handler)
	})

	t.Run("handles nil database gracefully", func(t *testing.T) {
		cfg := createTestConfig()

		server, err := New(cfg, nil)

		require.NoError(t, err)
		assert.NotNil(t, server)
		assert.Nil(t, server.database)
	})

	t.Run("handles different port configurations", func(t *testing.T) {
		testCases := []struct {
			name         string
			port         int
			host         string
			expectedAddr string
		}{
			{"default port", 8080, "localhost", "localhost:8080"},
			{"custom port", 3000, "localhost", "localhost:3000"},
			{"different host", 8080, "0.0.0.0", "0.0.0.0:8080"},
			{"high port", 65535, "127.0.0.1", "127.0.0.1:65535"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := createTestConfig()
				cfg.API.Host = tc.host
				cfg.API.Port = tc.port

				server, err := New(cfg, nil)

				require.NoError(t, err)
				assert.Equal(t, tc.expectedAddr, server.httpServer.Addr)
			})
		}
	})
}

func TestServerStartStop(t *testing.T) {
	t.Run("server can start and stop successfully", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.Port = 0 // Use random available port
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Start server in goroutine
		startErr := make(chan error, 1)
		go func() {
			startErr <- server.Start(context.Background())
		}()

		// Give server time to start
		time.Sleep(100 * time.Millisecond)

		// Verify server is running
		assert.True(t, server.IsRunning())

		// Stop server
		err = server.Stop()
		assert.NoError(t, err)

		// Verify server stopped
		assert.False(t, server.IsRunning())

		// Check if start completed (should return because of shutdown)
		select {
		case err := <-startErr:
			assert.Equal(t, http.ErrServerClosed, err)
		case <-time.After(time.Second):
			t.Fatal("Server start didn't complete after stop")
		}
	})

	t.Run("stop on non-running server is safe", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Stop server that was never started
		err = server.Stop()
		assert.NoError(t, err)
		assert.False(t, server.IsRunning())
	})

	t.Run("start on already running server is handled", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.Port = 0
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Start server
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go server.Start(ctx)
		time.Sleep(100 * time.Millisecond)
		assert.True(t, server.IsRunning())

		// Try to start again - should return error
		err = server.Start(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")

		// Cleanup
		server.Stop()
	})

	t.Run("multiple stop calls are safe", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.Port = 0
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Start server
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go server.Start(ctx)
		time.Sleep(100 * time.Millisecond)
		assert.True(t, server.IsRunning())

		// Stop server multiple times - should be safe
		err1 := server.Stop()
		assert.NoError(t, err1)
		assert.False(t, server.IsRunning())

		err2 := server.Stop()
		assert.NoError(t, err2)
		assert.False(t, server.IsRunning())

		err3 := server.Stop()
		assert.NoError(t, err3)
		assert.False(t, server.IsRunning())
	})
}

func TestServerMethods(t *testing.T) {
	t.Run("GetRouter returns correct router", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		router := server.GetRouter()
		assert.NotNil(t, router)
		assert.Equal(t, server.router, router)
	})

	t.Run("GetAddress returns correct address", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.Host = "test.example.com"
		cfg.API.Port = 9000
		server, err := New(cfg, nil)
		require.NoError(t, err)

		address := server.GetAddress()
		assert.Equal(t, "test.example.com:9000", address)
	})

	t.Run("IsRunning reflects server state", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.Port = 0
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Initially not running
		assert.False(t, server.IsRunning())

		// Start server
		go server.Start(context.Background())
		time.Sleep(100 * time.Millisecond)
		assert.True(t, server.IsRunning())

		// Stop server
		server.Stop()
		time.Sleep(100 * time.Millisecond)
		assert.False(t, server.IsRunning())
	})
}

func TestConcurrentAccess(t *testing.T) {
	cfg := createTestConfig()
	cfg.API.Port = 0 // Use random port
	server, err := New(cfg, nil)
	require.NoError(t, err)

	// Test router directly without starting HTTP server

	// Warmup request to ensure router is ready
	warmupReq := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
	warmupRec := httptest.NewRecorder()
	server.router.ServeHTTP(warmupRec, warmupReq)
	require.Equal(t, http.StatusOK, warmupRec.Code, "Warmup request should succeed")

	t.Run("handles concurrent requests", func(t *testing.T) {
		const numRequests = 50
		var wg sync.WaitGroup
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
				rec := httptest.NewRecorder()

				server.router.ServeHTTP(rec, req)
				results <- rec.Code
			}()
		}

		wg.Wait()
		close(results)

		// All requests should succeed
		for statusCode := range results {
			assert.Equal(t, http.StatusOK, statusCode)
		}
	})

	t.Run("server state is thread-safe", func(t *testing.T) {
		const numRoutines = 20
		var wg sync.WaitGroup

		for i := 0; i < numRoutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// These operations should be thread-safe
				assert.NotNil(t, server.GetRouter())
				assert.NotEmpty(t, server.GetAddress())
				assert.False(t, server.IsRunning()) // Server not started in this test
			}()
		}

		wg.Wait()
	})
}

func TestBusinessLogicInvariants(t *testing.T) {
	t.Run("server maintains consistent state", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Server should maintain consistent state throughout its lifecycle
		assert.Equal(t, cfg, server.config, "Config should remain unchanged")
		assert.NotNil(t, server.router, "Router should always be available")
		assert.NotNil(t, server.logger, "Logger should always be available")
		assert.NotNil(t, server.metrics, "Metrics should always be available")
		assert.False(t, server.startTime.IsZero(), "Start time should be set")

		// Start and stop server
		go server.Start(context.Background())
		time.Sleep(50 * time.Millisecond)
		server.Stop()

		// State should still be consistent
		assert.Equal(t, cfg, server.config)
		assert.NotNil(t, server.router)
		assert.False(t, server.startTime.IsZero())
	})

	t.Run("pagination calculations are mathematically correct", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		testCases := []struct {
			page     int
			pageSize int
			offset   int
		}{
			{1, 20, 0},
			{2, 20, 20},
			{5, 10, 40},
			{10, 25, 225},
		}

		for _, tc := range testCases {
			url := fmt.Sprintf("/test?page=%d&page_size=%d", tc.page, tc.pageSize)
			req := httptest.NewRequest("GET", url, http.NoBody)
			params := server.GetPaginationParams(req)

			assert.Equal(t, tc.offset, params.Offset)
		}
	})
}
