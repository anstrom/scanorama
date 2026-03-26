// Package api provides HTTP REST API functionality for the Scanorama network scanner.
// It implements endpoints for scanning, discovery, host management, and system status.
package api

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
)

// Server timeout constants.
const (
	serverShutdownTimeout    = 30 * time.Second
	prometheusUpdateInterval = 5 * time.Second
)

// Option is a functional option for configuring a Server.
type Option func(*Server)

// WithFrontend sets the fs.FS used to serve the embedded frontend SPA.
// Pass the result of frontend.FS() from the internal/frontend package.
// When not provided (or nil), the server falls back to the plain API index
// response on "/" and does not serve any frontend assets.
func WithFrontend(fsys fs.FS) Option {
	return func(s *Server) {
		s.frontendFS = fsys
	}
}

// Server represents the API server.
type Server struct {
	httpServer      *http.Server
	router          *mux.Router
	config          *config.Config
	database        *db.DB
	discoveryEngine *discovery.Engine
	logger          *slog.Logger
	metrics         *metrics.Registry
	prom            *metrics.PrometheusMetrics
	startTime       time.Time
	ringBuffer      *logging.RingBuffer

	// frontendFS, when non-nil, is used to serve the embedded frontend SPA.
	// It is set via WithFrontend and can be overridden at runtime by
	// config.API.FrontendDir (which causes resolveFrontendFS to prefer disk).
	frontendFS fs.FS

	// scanQueue manages the bounded worker pool for scan execution.
	scanQueue *scanning.ScanQueue

	// State management
	mu      sync.RWMutex
	running bool

	// background metrics updater
	metricsCancel context.CancelFunc
}

// Config holds API server configuration.
type Config struct {
	Host              string        `yaml:"host" json:"host"`
	Port              int           `yaml:"port" json:"port"`
	ReadTimeout       time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	EnableCORS        bool          `yaml:"enable_cors" json:"enable_cors"`
	CORSOrigins       []string      `yaml:"cors_origins" json:"cors_origins"`
	RateLimitEnabled  bool          `yaml:"rate_limit_enabled" json:"rate_limit_enabled"`
	RateLimitRequests int           `yaml:"rate_limit_requests" json:"rate_limit_requests"`
	RateLimitWindow   time.Duration `yaml:"rate_limit_window" json:"rate_limit_window"`
	AuthEnabled       bool          `yaml:"auth_enabled" json:"auth_enabled"`
	APIKeys           []string      `yaml:"api_keys" json:"api_keys"`
}

// DefaultConfig returns default API server configuration.
func DefaultConfig() Config {
	return Config{
		Host:              "127.0.0.1",
		Port:              8080,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
		EnableCORS:        true,
		CORSOrigins:       []string{"*"},
		RateLimitEnabled:  true,
		RateLimitRequests: 100,
		RateLimitWindow:   time.Minute,
		AuthEnabled:       false,
		APIKeys:           []string{},
	}
}

// New creates a new API server instance.
// Pass functional options (e.g. WithFrontend) to configure optional behavior.
func New(cfg *config.Config, database *db.DB, opts ...Option) (*Server, error) {
	rb := logging.NewRingBuffer(0)
	wrappedLogger := slog.New(logging.TeeHandler(logging.Default().Handler(), rb.Handler()))
	logger := wrappedLogger.With("component", "api")

	// Create metrics registry
	metricsManager := metrics.NewRegistry()

	// Create Prometheus metrics (global instance)
	promMetrics := metrics.GetGlobalMetrics()

	// Create router
	router := mux.NewRouter()

	// Get API config from main config
	apiConfig := getAPIConfigFromConfig(cfg)

	// Create the discovery engine so API-triggered jobs actually run nmap.
	discoveryEngine := discovery.NewEngine(database)

	// Create the scan queue using pool size and queue depth from config.
	scanQueue := scanning.NewScanQueue(
		cfg.Scanning.WorkerPoolSize,
		cfg.Scanning.MaxConcurrentTargets,
	)

	server := &Server{
		router:          router,
		config:          cfg,
		database:        database,
		discoveryEngine: discoveryEngine,
		logger:          logger,
		metrics:         metricsManager,
		prom:            promMetrics,
		startTime:       time.Now(),
		ringBuffer:      rb,
		scanQueue:       scanQueue,
	}

	// Apply functional options before setting up routes so that e.g.
	// WithFrontend is visible to setupRoutes.
	for _, opt := range opts {
		opt(server)
	}

	// Setup routes
	server.setupRoutes()

	// Setup middleware
	server.setupMiddleware(&apiConfig)

	// Create HTTP server
	server.httpServer = &http.Server{
		Addr:           net.JoinHostPort(apiConfig.Host, strconv.Itoa(apiConfig.Port)),
		Handler:        server.router,
		ReadTimeout:    apiConfig.ReadTimeout,
		WriteTimeout:   apiConfig.WriteTimeout,
		IdleTimeout:    apiConfig.IdleTimeout,
		MaxHeaderBytes: apiConfig.MaxHeaderBytes,
	}

	// Recover any jobs left in 'running' by a previous crash or restart.
	// This runs before the HTTP listener opens so no client ever sees a job
	// that is permanently 'running' with no live goroutine behind it.
	// Errors are non-fatal: a warn-log is emitted and startup continues.
	if result, err := db.RecoverStaleJobs(context.Background(), database); err != nil {
		logger.Warn("Failed to recover stale jobs on startup", "error", err)
	} else if result.Total() > 0 {
		logger.Info("Startup recovery complete",
			"scan_jobs_recovered", result.ScanJobsRecovered,
			"discovery_jobs_recovered", result.DiscoveryJobsRecovered,
		)
	}

	return server, nil
}

// Start starts the API server.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server is already running")
	}
	s.mu.Unlock()

	// Start the scan queue worker pool before marking the server as running.
	// This ensures all workers have called wg.Add(1) before any concurrent
	// Stop() call reaches wg.Wait(), avoiding a wg.Add/wg.Wait data race.
	if s.scanQueue != nil {
		s.scanQueue.Start(ctx)
		s.logger.Info("Scan queue started",
			"workers", s.config.Scanning.WorkerPoolSize,
			"queue_depth", s.config.Scanning.MaxConcurrentTargets)
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	s.logger.Info("Starting API server",
		"address", s.httpServer.Addr,
		"read_timeout", s.httpServer.ReadTimeout,
		"write_timeout", s.httpServer.WriteTimeout)

	// Start background Prometheus system metrics updates
	if s.prom != nil {
		mctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.metricsCancel = cancel
		s.mu.Unlock()
		go s.prom.StartPeriodicUpdates(mctx, prometheusUpdateInterval)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		err := s.httpServer.ListenAndServe()
		// Mark as not running when server stops for any reason
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()

		if err != nil {
			if err == http.ErrServerClosed {
				// Normal shutdown
				errChan <- err
			} else {
				// Actual error
				errChan <- fmt.Errorf("API server failed: %w", err)
			}
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errChan:
		return err
	}
}

// Stop gracefully stops the API server.
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil // Already stopped
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping API server")

	// Stop the scan queue first so in-flight scans can finish before the
	// HTTP server stops accepting new requests.
	if s.scanQueue != nil {
		s.scanQueue.Stop()
		s.logger.Info("Scan queue stopped")
	}

	// Stop background metrics updates if running (guarded by mutex)
	s.mu.Lock()
	cancel := s.metricsCancel
	s.metricsCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("API server shutdown error", "error", err)
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	s.logger.Info("API server stopped successfully")
	return nil
}

// GetRouter returns the configured router.
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// GetAddress returns the server address.
func (s *Server) GetAddress() string {
	return s.httpServer.Addr
}

// IsRunning checks if the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// getAPIConfigFromConfig extracts API configuration from main config.
func getAPIConfigFromConfig(cfg *config.Config) Config {
	apiConfig := DefaultConfig()

	// Override with values from main config
	apiConfig.Host = cfg.API.Host
	apiConfig.Port = cfg.API.Port
	apiConfig.ReadTimeout = cfg.API.ReadTimeout
	apiConfig.WriteTimeout = cfg.API.WriteTimeout
	apiConfig.IdleTimeout = cfg.API.IdleTimeout
	apiConfig.MaxHeaderBytes = cfg.API.MaxHeaderBytes

	// Security settings
	apiConfig.EnableCORS = cfg.API.EnableCORS
	apiConfig.CORSOrigins = cfg.API.CORSOrigins
	apiConfig.AuthEnabled = cfg.API.AuthEnabled
	apiConfig.APIKeys = cfg.API.APIKeys

	// Rate limiting
	apiConfig.RateLimitEnabled = cfg.API.RateLimitEnabled
	apiConfig.RateLimitRequests = cfg.API.RateLimitRequests
	apiConfig.RateLimitWindow = cfg.API.RateLimitWindow

	return apiConfig
}
