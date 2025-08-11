// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements health check and system status endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/metrics"
)

// DatabasePinger defines the interface for database health checking.
type DatabasePinger interface {
	Ping(ctx context.Context) error
}

// Timeout constants.
const (
	healthCheckTimeout = 5 * time.Second
	readinessTimeout   = 10 * time.Second
	dependencyTimeout  = 3 * time.Second
)

// Status constants.
const (
	StatusHealthy       = "healthy"
	StatusUnhealthy     = "unhealthy"
	StatusNotConfigured = "not configured"
)

// HealthHandler handles health check and status endpoints.
type HealthHandler struct {
	database  DatabasePinger
	logger    *slog.Logger
	metrics   metrics.MetricsRegistry
	startTime time.Time
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(
	database DatabasePinger,
	logger *slog.Logger,
	metricsManager metrics.MetricsRegistry,
) *HealthHandler {
	return &HealthHandler{
		database:  database,
		logger:    logger.With("handler", "health"),
		metrics:   metricsManager,
		startTime: time.Now(),
	}
}

// HealthResponse represents a health check response.
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    string            `json:"uptime"`
	Checks    map[string]string `json:"checks"`
}

// LivenessResponse represents a simple liveness check response.
type LivenessResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

// StatusResponse represents a detailed status response.
type StatusResponse struct {
	Service   ServiceInfo    `json:"service"`
	System    SystemInfo     `json:"system"`
	Database  DatabaseInfo   `json:"database"`
	Metrics   MetricsInfo    `json:"metrics"`
	Health    HealthResponse `json:"health"`
	Timestamp time.Time      `json:"timestamp"`
}

// ServiceInfo contains service-related information.
type ServiceInfo struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	StartTime time.Time `json:"start_time"`
	Uptime    string    `json:"uptime"`
	PID       int       `json:"pid"`
}

// SystemInfo contains system-related information.
type SystemInfo struct {
	OS           string            `json:"os"`
	Architecture string            `json:"architecture"`
	CPUs         int               `json:"cpus"`
	GoVersion    string            `json:"go_version"`
	Memory       MemoryInfo        `json:"memory"`
	Goroutines   int               `json:"goroutines"`
	Environment  map[string]string `json:"environment,omitempty"`
}

// MemoryInfo contains memory usage information.
type MemoryInfo struct {
	Allocated   uint64 `json:"allocated_bytes"`
	TotalAlloc  uint64 `json:"total_alloc_bytes"`
	System      uint64 `json:"system_bytes"`
	GCCycles    uint32 `json:"gc_cycles"`
	LastGC      string `json:"last_gc"`
	HeapObjects uint64 `json:"heap_objects"`
}

// DatabaseInfo contains database connection information.
type DatabaseInfo struct {
	Connected    bool          `json:"connected"`
	Driver       string        `json:"driver"`
	Host         string        `json:"host"`
	Database     string        `json:"database"`
	LastPing     time.Time     `json:"last_ping"`
	ResponseTime time.Duration `json:"response_time_ms"`
	Error        string        `json:"error,omitempty"`
}

// MetricsInfo contains metrics system information.
type MetricsInfo struct {
	Enabled       bool                   `json:"enabled"`
	TotalCounters int                    `json:"total_counters"`
	TotalGauges   int                    `json:"total_gauges"`
	TotalHistos   int                    `json:"total_histograms"`
	LastUpdated   time.Time              `json:"last_updated"`
	Summary       map[string]interface{} `json:"summary,omitempty"`
}

// VersionResponse represents version information.
type VersionResponse struct {
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	Timestamp time.Time `json:"timestamp"`
}

// Health performs a basic health check.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	h.logger.Debug("Health check requested", "remote_addr", r.RemoteAddr)

	response := HealthResponse{
		Status:    StatusHealthy,
		Timestamp: time.Now().UTC(),
		Uptime:    time.Since(h.startTime).String(),
		Checks:    make(map[string]string),
	}

	// Check database connectivity
	if h.database != nil {
		if err := h.database.Ping(ctx); err != nil {
			response.Status = StatusUnhealthy
			response.Checks["database"] = "failed: " + err.Error()
			h.logger.Warn("Database health check failed", "error", err)
		} else {
			response.Checks["database"] = "ok"
		}
	} else {
		response.Checks["database"] = StatusNotConfigured
	}

	// Check metrics system
	if h.metrics != nil {
		response.Checks["metrics"] = "ok"
	} else {
		response.Checks["metrics"] = StatusNotConfigured
	}

	// Set HTTP status based on health
	statusCode := http.StatusOK
	if response.Status == StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode health response", "error", err)
		return
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_health_checks_total", metrics.Labels{
			"status": response.Status,
		})
	}
}

// Liveness performs a simple liveness check without dependencies.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("Liveness check requested", "remote_addr", r.RemoteAddr)

	response := LivenessResponse{
		Status:    "alive",
		Timestamp: time.Now().UTC(),
		Uptime:    time.Since(h.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode liveness response", "error", err)
		return
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_liveness_checks_total", nil)
	}
}

// Status provides detailed system status information.
func (h *HealthHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	h.logger.Debug("Status check requested", "remote_addr", r.RemoteAddr)

	response := StatusResponse{
		Timestamp: time.Now().UTC(),
	}

	// Service information
	response.Service = ServiceInfo{
		Name:      "scanorama",
		Version:   getVersion(),
		StartTime: h.startTime,
		Uptime:    time.Since(h.startTime).String(),
		PID:       getPID(),
	}

	// System information
	response.System = h.getSystemInfo()

	// Database information
	response.Database = h.getDatabaseInfo(ctx)

	// Metrics information
	response.Metrics = h.getMetricsInfo()

	// Health check
	dbCtx, dbCancel := context.WithTimeout(ctx, dependencyTimeout)
	defer dbCancel()
	response.Health = h.getHealthInfo(dbCtx)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode status response", "error", err)
		return
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_status_checks_total", metrics.Labels{
			"status": response.Health.Status,
		})
	}
}

// Version provides version information.
func (h *HealthHandler) Version(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("Version requested", "remote_addr", r.RemoteAddr)

	response := VersionResponse{
		Version:   getVersion(),
		Commit:    getCommit(),
		BuildTime: getBuildTime(),
		GoVersion: runtime.Version(),
		Timestamp: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode version response", "error", err)
		return
	}

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_version_requests_total", nil)
	}
}

// Metrics provides metrics endpoint (Prometheus format).
func (h *HealthHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("Metrics requested", "remote_addr", r.RemoteAddr)

	if h.metrics == nil {
		http.Error(w, "Metrics not available", http.StatusNotFound)
		return
	}

	// Export metrics in simple format (Prometheus format would need additional implementation)
	metricsData := h.metrics.GetMetrics()
	w.Header().Set("Content-Type", "text/plain")
	for _, metric := range metricsData {
		_, _ = fmt.Fprintf(w, "# TYPE %s %s\n", metric.Name, string(metric.Type))
		labelStr := ""
		if len(metric.Labels) > 0 {
			labelParts := make([]string, 0, len(metric.Labels))
			for k, v := range metric.Labels {
				labelParts = append(labelParts, fmt.Sprintf("%s=%q", k, v))
			}
			labelStr = "{" + strings.Join(labelParts, ",") + "}"
		}
		_, _ = fmt.Fprintf(w, "%s%s %g %d\n", metric.Name, labelStr, metric.Value, metric.Timestamp.Unix())
	}

	// Record metrics (don't count this in the metrics to avoid recursion)
}

// getSystemInfo gathers system information.
func (h *HealthHandler) getSystemInfo() SystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memInfo := MemoryInfo{
		Allocated:   memStats.Alloc,
		TotalAlloc:  memStats.TotalAlloc,
		System:      memStats.Sys,
		GCCycles:    memStats.NumGC,
		HeapObjects: memStats.HeapObjects,
	}

	if memStats.LastGC > 0 {
		// Safely convert uint64 to int64 for time.Unix
		// LastGC is in nanoseconds, so we need to handle potential overflow
		const maxInt64 = 9223372036854775807 // math.MaxInt64
		if memStats.LastGC <= maxInt64 {
			memInfo.LastGC = time.Unix(0, int64(memStats.LastGC)).Format(time.RFC3339)
		} else {
			// If overflow would occur, use current time as fallback
			memInfo.LastGC = time.Now().Format(time.RFC3339)
		}
	}

	return SystemInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPUs:         runtime.NumCPU(),
		GoVersion:    runtime.Version(),
		Memory:       memInfo,
		Goroutines:   runtime.NumGoroutine(),
	}
}

// getDatabaseInfo gathers database information.
func (h *HealthHandler) getDatabaseInfo(ctx context.Context) DatabaseInfo {
	info := DatabaseInfo{
		Connected: false,
		Driver:    "postgres",
	}

	if h.database == nil {
		info.Error = "database not configured"
		return info
	}

	// Get database configuration info if available
	// Note: We should not expose sensitive connection details
	info.Host = "configured"
	info.Database = "configured"

	// Test connection with timing
	start := time.Now()
	if err := h.database.Ping(ctx); err != nil {
		info.Error = err.Error()
		info.ResponseTime = time.Since(start)
		return info
	}

	info.Connected = true
	info.LastPing = time.Now().UTC()
	info.ResponseTime = time.Since(start)

	return info
}

// getMetricsInfo gathers metrics system information.
func (h *HealthHandler) getMetricsInfo() MetricsInfo {
	info := MetricsInfo{
		Enabled:     h.metrics != nil,
		LastUpdated: time.Now().UTC(),
	}

	if h.metrics != nil {
		// Get metrics summary
		allMetrics := h.metrics.GetMetrics()
		counterCount := 0
		gaugeCount := 0
		histogramCount := 0

		for _, metric := range allMetrics {
			switch metric.Type {
			case metrics.TypeCounter:
				counterCount++
			case metrics.TypeGauge:
				gaugeCount++
			case metrics.TypeHistogram:
				histogramCount++
			}
		}

		info.TotalCounters = counterCount
		info.TotalGauges = gaugeCount
		info.TotalHistos = histogramCount
		info.Summary = map[string]interface{}{
			"total_metrics": len(allMetrics),
			"last_updated":  time.Now().UTC(),
		}
	}

	return info
}

// getHealthInfo performs health checks and returns status.
func (h *HealthHandler) getHealthInfo(ctx context.Context) HealthResponse {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Uptime:    time.Since(h.startTime).String(),
		Checks:    make(map[string]string),
	}

	// Database health check
	if h.database != nil {
		if err := h.database.Ping(ctx); err != nil {
			response.Status = StatusUnhealthy
			response.Checks["database"] = "failed: " + err.Error()
		} else {
			response.Checks["database"] = "ok"
		}
	} else {
		response.Checks["database"] = StatusNotConfigured
	}

	// Memory health check
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Check if memory usage is excessive (over 1GB allocated)
	const maxMemory = 1 << 30 // 1GB
	if memStats.Alloc > maxMemory {
		response.Status = "degraded"
		response.Checks["memory"] = "high usage"
	} else {
		response.Checks["memory"] = "ok"
	}

	// Goroutine health check
	goroutines := runtime.NumGoroutine()
	const maxGoroutines = 1000
	if goroutines > maxGoroutines {
		if response.Status == StatusHealthy {
			response.Status = "degraded"
		}
		response.Checks["goroutines"] = "high count"
	} else {
		response.Checks["goroutines"] = "ok"
	}

	return response
}

// Helper functions for build information (these should be set via ldflags).
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func getVersion() string {
	return version
}

func getCommit() string {
	return commit
}

func getBuildTime() string {
	return buildTime
}

func getPID() int {
	return os.Getpid()
}

// SetBuildInfo sets build information (called by main package).
func SetBuildInfo(v, c, bt string) {
	version = v
	commit = c
	buildTime = bt
}
