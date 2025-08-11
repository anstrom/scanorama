// Package api provides HTTP REST API functionality for the Scanorama network scanner.
// It implements endpoints for scanning, discovery, host management, and system status.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "github.com/anstrom/scanorama/docs/swagger" // Import generated swagger docs
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Server timeout constants.
const (
	serverShutdownTimeout = 30 * time.Second
	healthCheckTimeout    = 5 * time.Second
)

// Server represents the API server.
type Server struct {
	httpServer *http.Server
	router     *mux.Router
	config     *config.Config
	database   *db.DB
	logger     *slog.Logger
	metrics    *metrics.Registry
	startTime  time.Time
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
func New(cfg *config.Config, database *db.DB) (*Server, error) {
	logger := logging.Default().With("component", "api")

	// Create metrics registry
	metricsManager := metrics.NewRegistry()

	// Create router
	router := mux.NewRouter()

	// Get API config from main config
	apiConfig := getAPIConfigFromConfig(cfg)

	server := &Server{
		router:    router,
		config:    cfg,
		database:  database,
		logger:    logger,
		metrics:   metricsManager,
		startTime: time.Now(),
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

	return server, nil
}

// Start starts the API server.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting API server",
		"address", s.httpServer.Addr,
		"read_timeout", s.httpServer.ReadTimeout,
		"write_timeout", s.httpServer.WriteTimeout)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("API server failed: %w", err)
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
	s.logger.Info("Stopping API server")

	ctx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("API server shutdown error", "error", err)
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	s.logger.Info("API server stopped successfully")
	return nil
}

// setupRoutes configures all API routes.
func (s *Server) setupRoutes() {
	// API version prefix
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Health and status endpoints
	api.HandleFunc("/liveness", s.livenessHandler).Methods("GET")
	api.HandleFunc("/health", s.healthHandler).Methods("GET")
	api.HandleFunc("/status", s.statusHandler).Methods("GET")
	api.HandleFunc("/version", s.versionHandler).Methods("GET")
	api.HandleFunc("/metrics", s.metricsHandler).Methods("GET")

	// Placeholder endpoints for future implementation
	// These will return "not implemented" responses for now
	api.HandleFunc("/scans", s.notImplementedHandler).Methods("GET", "POST")
	api.HandleFunc("/hosts", s.notImplementedHandler).Methods("GET", "POST")
	api.HandleFunc("/discovery", s.notImplementedHandler).Methods("GET", "POST")
	api.HandleFunc("/profiles", s.notImplementedHandler).Methods("GET", "POST")
	api.HandleFunc("/schedules", s.notImplementedHandler).Methods("GET", "POST")

	// Admin endpoints
	api.HandleFunc("/admin/status", s.adminStatusHandler).Methods("GET")

	// Swagger documentation endpoints
	s.router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
	))

	// Documentation aliases
	s.router.HandleFunc("/docs", s.redirectToSwagger).Methods("GET")
	s.router.HandleFunc("/docs/", s.redirectToSwagger).Methods("GET")
	s.router.HandleFunc("/api-docs", s.redirectToSwagger).Methods("GET")

	// Root redirect - send browsers to docs, API clients to health
	s.router.HandleFunc("/", s.redirectToAPI).Methods("GET")
}

// setupMiddleware configures middleware for the API server.
func (s *Server) setupMiddleware(apiConfig *Config) {
	// Basic recovery middleware to prevent panics
	s.router.Use(s.recoveryMiddleware)

	// Basic logging middleware
	s.router.Use(s.loggingMiddleware)

	// CORS middleware
	if apiConfig.EnableCORS {
		corsOptions := handlers.AllowedOrigins(apiConfig.CORSOrigins)
		corsHeaders := handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "X-API-Key"})
		corsMethods := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
		s.router.Use(handlers.CORS(corsOptions, corsHeaders, corsMethods))
	}

	// Content type validation
	s.router.Use(s.contentTypeMiddleware)
}

// redirectToAPI returns API information for root requests.
func (s *Server) redirectToAPI(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service": "Scanorama API",
		"version": "v1",
		"endpoints": map[string]string{
			"liveness": "/api/v1/liveness",
			"health":   "/api/v1/health",
			"status":   "/api/v1/status",
			"docs":     "/swagger/",
		},
		"timestamp": time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode API index response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// redirectToSwagger redirects to the Swagger UI.
func (s *Server) redirectToSwagger(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
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
	if s.httpServer == nil {
		return false
	}

	// Try to connect to the server
	conn, err := net.DialTimeout("tcp", s.httpServer.Addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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

// ErrorResponse represents a standard API error response.
type ErrorResponse struct {
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id,omitempty"`
}

// writeError writes a standardized error response.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, statusCode int, err error) {
	s.logger.Error("API error",
		"method", r.Method,
		"path", r.URL.Path,
		"status", statusCode,
		"error", err,
		"remote_addr", r.RemoteAddr)

	response := ErrorResponse{
		Error:     err.Error(),
		Timestamp: time.Now().UTC(),
		RequestID: getRequestID(r),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
		s.logger.Error("Failed to encode error response", "error", encodeErr)
	}
}

// Basic handler implementations.

// livenessHandler provides a simple liveness check endpoint.
// livenessHandler godoc
// @Summary Liveness check
// @Description Returns simple liveness status without dependency checks
// @Tags System
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /liveness [get]
func (s *Server) livenessHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(s.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// healthHandler provides basic health check endpoint.
// healthHandler godoc
// @Summary Health check
// @Description Returns service health status
// @Tags System
// @Produce json
// @Success 200 {object} handlers.HealthResponse
// @Success 503 {object} handlers.HealthResponse
// @Router /health [get]
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	status := "healthy"
	checks := make(map[string]string)

	// Check database
	if s.database != nil {
		if err := s.database.Ping(ctx); err != nil {
			status = "unhealthy"
			checks["database"] = "failed: " + err.Error()
		} else {
			checks["database"] = "ok"
		}
	} else {
		checks["database"] = "not configured"
	}

	response := map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().UTC(),
		"checks":    checks,
	}

	statusCode := http.StatusOK
	if status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// statusHandler provides detailed status information.
// statusHandler godoc
// @Summary System status
// @Description Returns detailed system status
// @Tags System
// @Produce json
// @Success 200 {object} handlers.StatusResponse
// @Router /status [get]
func (s *Server) statusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":   "scanorama-api",
		"timestamp": time.Now().UTC(),
		"uptime":    time.Since(time.Now()).String(), // Placeholder
		"version":   "0.2.0",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// versionHandler provides version information.
// versionHandler godoc
// @Summary Version information
// @Description Returns version and build info
// @Tags System
// @Produce json
// @Success 200 {object} handlers.VersionResponse
// @Router /version [get]
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"version":   "0.2.0",
		"timestamp": time.Now().UTC(),
		"service":   "scanorama",
	}

	s.WriteJSON(w, r, http.StatusOK, response)
}

// metricsHandler provides Prometheus-style metrics.
// metricsHandler godoc
// @Summary Application metrics
// @Description Returns Prometheus metrics
// @Tags System
// @Produce text/plain
// @Success 200 {string} string
// @Failure 404 {object} handlers.ErrorResponse
// @Router /metrics [get]
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	metricsData := s.metrics.GetMetrics()

	response := map[string]interface{}{
		"metrics":   metricsData,
		"timestamp": time.Now().UTC(),
	}

	s.WriteJSON(w, r, http.StatusOK, response)
}

// notImplementedHandler returns a not implemented response.
// notImplementedHandler godoc
// @Summary Not implemented
// @Description Endpoint not yet implemented
// @Tags System
// @Produce json
// @Success 501 {object} handlers.ErrorResponse
// @Router /scans [get]
// @Router /scans [post]
// @Router /hosts [get]
// @Router /discovery [get]
// @Router /discovery [post]
// @Router /profiles [get]
// @Router /profiles [post]
// @Router /schedules [get]
// @Router /schedules [post]
func (s *Server) notImplementedHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"error":     "endpoint not implemented",
		"timestamp": time.Now().UTC(),
		"endpoint":  r.URL.Path,
		"method":    r.Method,
	}

	s.WriteJSON(w, r, http.StatusNotImplemented, response)
}

// adminStatusHandler provides administrative status information.
// adminStatusHandler godoc
// @Summary Admin status
// @Description Returns admin status info
// @Tags Admin
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} handlers.AdminStatusResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 403 {object} handlers.ErrorResponse
// @Router /admin/status [get]
func (s *Server) adminStatusHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"admin_status": "active",
		"timestamp":    time.Now().UTC(),
		"server_info": map[string]interface{}{
			"address":       s.httpServer.Addr,
			"read_timeout":  s.httpServer.ReadTimeout,
			"write_timeout": s.httpServer.WriteTimeout,
		},
	}

	s.WriteJSON(w, r, http.StatusOK, response)
}

// WriteJSON writes a JSON response.
func (s *Server) WriteJSON(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response",
			"error", err,
			"path", r.URL.Path,
			"method", r.Method)
		// Don't try to write another error response here as headers are already sent
	}
}

// getRequestID extracts or generates a request ID.
func getRequestID(r *http.Request) string {
	// Try to get request ID from header first
	if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
		return reqID
	}

	// Generate a simple request ID
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Middleware functions.

// recoveryMiddleware recovers from panics and returns a 500 error.
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("Panic in API handler",
					"error", err,
					"path", r.URL.Path,
					"method", r.Method)

				if !headersSent(w) {
					s.writeError(w, r, http.StatusInternalServerError, fmt.Errorf("internal server error"))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent())

		// Record metrics
		s.metrics.Counter("http_requests_total", map[string]string{
			"method": r.Method,
			"status": fmt.Sprintf("%d", wrapped.statusCode),
		})

		s.metrics.Histogram("http_request_duration_seconds", duration.Seconds(), map[string]string{
			"method": r.Method,
		})
	})
}

// contentTypeMiddleware validates content type for POST/PUT requests.
func (s *Server) contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" {
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && contentType != "application/json" {
				s.writeError(w, r, http.StatusUnsupportedMediaType,
					fmt.Errorf("unsupported content type: %s", contentType))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// headersSent checks if headers have been sent.
func headersSent(w http.ResponseWriter) bool {
	// This is a simplified check - in real implementations you might use
	// reflection or other techniques to determine if headers were sent
	return false
}

// ParseJSON parses JSON request body into the provided struct.
func (s *Server) ParseJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Strict parsing

	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// GetQueryParam gets a query parameter with optional default value.
func (s *Server) GetQueryParam(r *http.Request, key, defaultValue string) string {
	if value := r.URL.Query().Get(key); value != "" {
		return value
	}
	return defaultValue
}

// GetQueryParamInt gets an integer query parameter with optional default value.
func (s *Server) GetQueryParamInt(r *http.Request, key string, defaultValue int) (int, error) {
	if value := r.URL.Query().Get(key); value != "" {
		return strconv.Atoi(value)
	}
	return defaultValue, nil
}

// GetQueryParamBool gets a boolean query parameter with optional default value.
func (s *Server) GetQueryParamBool(r *http.Request, key string, defaultValue bool) bool {
	if value := r.URL.Query().Get(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// ValidateRequest performs basic request validation.
func (s *Server) ValidateRequest(r *http.Request) error {
	// Check content type for POST/PUT requests
	if r.Method == "POST" || r.Method == "PUT" {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			return fmt.Errorf("content-type must be application/json, got: %s", contentType)
		}
	}

	return nil
}

// PaginationParams represents pagination parameters.
type PaginationParams struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Offset   int `json:"offset"`
}

// GetPaginationParams extracts pagination parameters from request.
func (s *Server) GetPaginationParams(r *http.Request) PaginationParams {
	const (
		defaultPage     = 1
		defaultPageSize = 20
		maxPageSize     = 100
	)

	page, err := s.GetQueryParamInt(r, "page", defaultPage)
	if err != nil || page < 1 {
		page = defaultPage
	}

	pageSize, err := s.GetQueryParamInt(r, "page_size", defaultPageSize)
	if err != nil || pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	offset := (page - 1) * pageSize

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
	}
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	Pagination struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"page_size"`
		TotalItems int64 `json:"total_items"`
		TotalPages int   `json:"total_pages"`
	} `json:"pagination"`
}

// WritePaginatedResponse writes a paginated response.
func (s *Server) WritePaginatedResponse(
	w http.ResponseWriter,
	r *http.Request,
	data interface{},
	params PaginationParams,
	totalItems int64,
) {
	totalPages := int((totalItems + int64(params.PageSize) - 1) / int64(params.PageSize))

	response := PaginatedResponse{
		Data: data,
	}
	response.Pagination.Page = params.Page
	response.Pagination.PageSize = params.PageSize
	response.Pagination.TotalItems = totalItems
	response.Pagination.TotalPages = totalPages

	s.WriteJSON(w, r, http.StatusOK, response)
}
