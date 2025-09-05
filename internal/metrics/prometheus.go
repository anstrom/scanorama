// Package metrics provides Prometheus-based metrics collection for scanorama.
// This replaces the custom metrics implementation with industry-standard
// Prometheus client library for proper observability and monitoring integration.
package metrics

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	// Namespace for all scanorama metrics
	namespace = "scanorama"

	// Subsystems
	subsystemScan      = "scan"
	subsystemDiscovery = "discovery"
	subsystemDatabase  = "database"
	subsystemSystem    = "system"
	subsystemAPI       = "api"
)

// PrometheusMetrics holds all Prometheus metric collectors
type PrometheusMetrics struct {
	// Scan metrics
	scansTotal   *prometheus.CounterVec
	scanDuration *prometheus.HistogramVec
	scanErrors   *prometheus.CounterVec
	portsScanned *prometheus.CounterVec
	hostsScanned *prometheus.CounterVec
	activeScans  prometheus.Gauge

	// Discovery metrics
	discoveryTotal    *prometheus.CounterVec
	discoveryDuration *prometheus.HistogramVec
	discoveryErrors   *prometheus.CounterVec
	hostsDiscovered   *prometheus.CounterVec
	activeDiscovery   prometheus.Gauge

	// Database metrics
	dbQueries       *prometheus.CounterVec
	dbQueryDuration *prometheus.HistogramVec
	dbConnections   prometheus.Gauge
	dbErrors        *prometheus.CounterVec

	// API metrics
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpErrors   *prometheus.CounterVec

	// System metrics
	memoryUsage prometheus.Gauge
	goroutines  prometheus.Gauge
	uptime      prometheus.Gauge
	cpuUsage    prometheus.Gauge

	// Performance tracking
	startTime  time.Time
	lastUpdate time.Time
	mu         sync.RWMutex
	registry   *prometheus.Registry
}

// NewPrometheusMetrics creates a new Prometheus metrics instance with all collectors
func NewPrometheusMetrics() *PrometheusMetrics {
	registry := prometheus.NewRegistry()

	pm := &PrometheusMetrics{
		startTime: time.Now(),
		registry:  registry,
	}

	// Initialize all metrics
	pm.initScanMetrics()
	pm.initDiscoveryMetrics()
	pm.initDatabaseMetrics()
	pm.initAPIMetrics()
	pm.initSystemMetrics()

	// Register all metrics with the registry
	pm.registerMetrics()

	// Register standard Go and process collectors for runtime visibility
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return pm
}

// initScanMetrics initializes scan-related metrics
func (pm *PrometheusMetrics) initScanMetrics() {
	pm.scansTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "total",
			Help:      "Total number of scans performed by type and status",
		},
		[]string{"scan_type", "status"},
	)

	pm.scanDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "duration_seconds",
			Help:      "Duration of scan operations in seconds",
			Buckets:   []float64{0.1, 0.5, 1.0, 5.0, 10.0, 30.0, 60.0, 300.0, 600.0},
		},
		[]string{"scan_type"},
	)

	pm.scanErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "errors_total",
			Help:      "Total number of scan errors by type and error",
		},
		[]string{"scan_type", "error_type"},
	)

	pm.portsScanned = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "ports_total",
			Help:      "Total number of ports scanned",
		},
		[]string{"scan_type", "port_status"},
	)

	pm.hostsScanned = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "hosts_total",
			Help:      "Total number of hosts scanned",
		},
		[]string{"scan_type", "host_status"},
	)

	pm.activeScans = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemScan,
			Name:      "active",
			Help:      "Number of currently active scans",
		},
	)
}

// initDiscoveryMetrics initializes discovery-related metrics
func (pm *PrometheusMetrics) initDiscoveryMetrics() {
	pm.discoveryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemDiscovery,
			Name:      "total",
			Help:      "Total number of discovery operations by method and status",
		},
		[]string{"method", "status"},
	)

	pm.discoveryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystemDiscovery,
			Name:      "duration_seconds",
			Help:      "Duration of discovery operations in seconds",
			Buckets:   []float64{1.0, 5.0, 10.0, 30.0, 60.0, 300.0, 600.0, 1800.0},
		},
		[]string{"method"},
	)

	pm.discoveryErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemDiscovery,
			Name:      "errors_total",
			Help:      "Total number of discovery errors by method and error type",
		},
		[]string{"method", "error_type"},
	)

	pm.hostsDiscovered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemDiscovery,
			Name:      "hosts_total",
			Help:      "Total number of hosts discovered",
		},
		[]string{"method", "network"},
	)

	pm.activeDiscovery = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemDiscovery,
			Name:      "active",
			Help:      "Number of currently active discovery operations",
		},
	)
}

// initDatabaseMetrics initializes database-related metrics
func (pm *PrometheusMetrics) initDatabaseMetrics() {
	pm.dbQueries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemDatabase,
			Name:      "queries_total",
			Help:      "Total number of database queries by operation and status",
		},
		[]string{"operation", "status"},
	)

	pm.dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystemDatabase,
			Name:      "query_duration_seconds",
			Help:      "Duration of database queries in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0},
		},
		[]string{"operation"},
	)

	pm.dbConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemDatabase,
			Name:      "connections_active",
			Help:      "Number of active database connections",
		},
	)

	pm.dbErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemDatabase,
			Name:      "errors_total",
			Help:      "Total number of database errors by operation and error type",
		},
		[]string{"operation", "error_type"},
	)
}

// initAPIMetrics initializes API-related metrics
func (pm *PrometheusMetrics) initAPIMetrics() {
	pm.httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemAPI,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests by method, path and status",
		},
		[]string{"method", "path", "status"},
	)

	pm.httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystemAPI,
			Name:      "request_duration_seconds",
			Help:      "Duration of HTTP requests in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0},
		},
		[]string{"method", "path"},
	)

	pm.httpErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystemAPI,
			Name:      "errors_total",
			Help:      "Total number of HTTP errors by method, path and error type",
		},
		[]string{"method", "path", "error_type"},
	)
}

// initSystemMetrics initializes system-related metrics
func (pm *PrometheusMetrics) initSystemMetrics() {
	pm.memoryUsage = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemSystem,
			Name:      "memory_bytes",
			Help:      "Current memory usage in bytes",
		},
	)

	pm.goroutines = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemSystem,
			Name:      "goroutines",
			Help:      "Current number of goroutines",
		},
	)

	pm.uptime = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemSystem,
			Name:      "uptime_seconds",
			Help:      "Application uptime in seconds",
		},
	)

	pm.cpuUsage = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystemSystem,
			Name:      "cpu_usage_percent",
			Help:      "Current CPU usage percentage",
		},
	)
}

// registerMetrics registers all metrics with the Prometheus registry
func (pm *PrometheusMetrics) registerMetrics() {
	// Scan metrics
	pm.registry.MustRegister(pm.scansTotal)
	pm.registry.MustRegister(pm.scanDuration)
	pm.registry.MustRegister(pm.scanErrors)
	pm.registry.MustRegister(pm.portsScanned)
	pm.registry.MustRegister(pm.hostsScanned)
	pm.registry.MustRegister(pm.activeScans)

	// Discovery metrics
	pm.registry.MustRegister(pm.discoveryTotal)
	pm.registry.MustRegister(pm.discoveryDuration)
	pm.registry.MustRegister(pm.discoveryErrors)
	pm.registry.MustRegister(pm.hostsDiscovered)
	pm.registry.MustRegister(pm.activeDiscovery)

	// Database metrics
	pm.registry.MustRegister(pm.dbQueries)
	pm.registry.MustRegister(pm.dbQueryDuration)
	pm.registry.MustRegister(pm.dbConnections)
	pm.registry.MustRegister(pm.dbErrors)

	// API metrics
	pm.registry.MustRegister(pm.httpRequests)
	pm.registry.MustRegister(pm.httpDuration)
	pm.registry.MustRegister(pm.httpErrors)

	// System metrics
	pm.registry.MustRegister(pm.memoryUsage)
	pm.registry.MustRegister(pm.goroutines)
	pm.registry.MustRegister(pm.uptime)
	pm.registry.MustRegister(pm.cpuUsage)
}

// GetRegistry returns the Prometheus registry for HTTP handler
func (pm *PrometheusMetrics) GetRegistry() *prometheus.Registry {
	return pm.registry
}

// Scan Metrics Methods

// IncrementScansTotal increments the total scan counter
func (pm *PrometheusMetrics) IncrementScansTotal(scanType, status string) {
	pm.scansTotal.WithLabelValues(scanType, status).Inc()
}

// RecordScanDuration records a scan duration
func (pm *PrometheusMetrics) RecordScanDuration(scanType string, duration time.Duration) {
	pm.scanDuration.WithLabelValues(scanType).Observe(duration.Seconds())
}

// IncrementScanErrors increments scan error counter
func (pm *PrometheusMetrics) IncrementScanErrors(scanType, errorType string) {
	pm.scanErrors.WithLabelValues(scanType, errorType).Inc()
}

// IncrementPortsScanned increments ports scanned counter
func (pm *PrometheusMetrics) IncrementPortsScanned(scanType, status string, count int) {
	pm.portsScanned.WithLabelValues(scanType, status).Add(float64(count))
}

// IncrementHostsScanned increments hosts scanned counter
func (pm *PrometheusMetrics) IncrementHostsScanned(scanType, status string, count int) {
	pm.hostsScanned.WithLabelValues(scanType, status).Add(float64(count))
}

// SetActiveScans sets the number of active scans
func (pm *PrometheusMetrics) SetActiveScans(count int) {
	pm.activeScans.Set(float64(count))
}

// Discovery Metrics Methods

// IncrementDiscoveryTotal increments discovery counter
func (pm *PrometheusMetrics) IncrementDiscoveryTotal(method, status string) {
	pm.discoveryTotal.WithLabelValues(method, status).Inc()
}

// RecordDiscoveryDuration records discovery duration
func (pm *PrometheusMetrics) RecordDiscoveryDuration(method string, duration time.Duration) {
	pm.discoveryDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// IncrementDiscoveryErrors increments discovery error counter
func (pm *PrometheusMetrics) IncrementDiscoveryErrors(method, errorType string) {
	pm.discoveryErrors.WithLabelValues(method, errorType).Inc()
}

// IncrementHostsDiscovered increments hosts discovered counter
func (pm *PrometheusMetrics) IncrementHostsDiscovered(method, network string, count int) {
	pm.hostsDiscovered.WithLabelValues(method, network).Add(float64(count))
}

// SetActiveDiscovery sets the number of active discovery operations
func (pm *PrometheusMetrics) SetActiveDiscovery(count int) {
	pm.activeDiscovery.Set(float64(count))
}

// Database Metrics Methods

// IncrementDatabaseQueries increments database query counter
func (pm *PrometheusMetrics) IncrementDatabaseQueries(operation, status string) {
	pm.dbQueries.WithLabelValues(operation, status).Inc()
}

// RecordDatabaseQueryDuration records database query duration
func (pm *PrometheusMetrics) RecordDatabaseQueryDuration(operation string, duration time.Duration) {
	pm.dbQueryDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// SetActiveConnections sets the number of active database connections
func (pm *PrometheusMetrics) SetActiveConnections(count int) {
	pm.dbConnections.Set(float64(count))
}

// IncrementDatabaseErrors increments database error counter
func (pm *PrometheusMetrics) IncrementDatabaseErrors(operation, errorType string) {
	pm.dbErrors.WithLabelValues(operation, errorType).Inc()
}

// API Metrics Methods

// IncrementHTTPRequests increments HTTP request counter
func (pm *PrometheusMetrics) IncrementHTTPRequests(method, path, status string) {
	pm.httpRequests.WithLabelValues(method, path, status).Inc()
}

// RecordHTTPDuration records HTTP request duration
func (pm *PrometheusMetrics) RecordHTTPDuration(method, path string, duration time.Duration) {
	pm.httpDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

// IncrementHTTPErrors increments HTTP error counter
func (pm *PrometheusMetrics) IncrementHTTPErrors(method, path, errorType string) {
	pm.httpErrors.WithLabelValues(method, path, errorType).Inc()
}

// System Metrics Methods

// UpdateSystemMetrics updates all system metrics with current values
func (pm *PrometheusMetrics) UpdateSystemMetrics() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Update memory usage
	pm.memoryUsage.Set(float64(memStats.Alloc))

	// Update goroutine count
	pm.goroutines.Set(float64(runtime.NumGoroutine()))

	// Update uptime
	uptime := time.Since(pm.startTime).Seconds()
	pm.uptime.Set(uptime)

	// Update last update time
	pm.lastUpdate = time.Now()
}

// SetCPUUsage sets the CPU usage percentage
func (pm *PrometheusMetrics) SetCPUUsage(percent float64) {
	pm.cpuUsage.Set(percent)
}

// Utility Methods

// GetUptime returns the application uptime
func (pm *PrometheusMetrics) GetUptime() time.Duration {
	return time.Since(pm.startTime)
}

// GetLastUpdate returns the last metrics update time
func (pm *PrometheusMetrics) GetLastUpdate() time.Time {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.lastUpdate
}

// StartPeriodicUpdates starts a goroutine that periodically updates system metrics
func (pm *PrometheusMetrics) StartPeriodicUpdates(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Update immediately
	pm.UpdateSystemMetrics()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.UpdateSystemMetrics()
		}
	}
}

// Global instance for easy access
var globalMetrics *PrometheusMetrics
var metricsOnce sync.Once

// GetGlobalMetrics returns the global Prometheus metrics instance
func GetGlobalMetrics() *PrometheusMetrics {
	metricsOnce.Do(func() {
		globalMetrics = NewPrometheusMetrics()
	})
	return globalMetrics
}

// Convenience functions using global instance

// RecordScanDurationPrometheus records a scan duration using global metrics
func RecordScanDurationPrometheus(scanType string, duration time.Duration) {
	GetGlobalMetrics().RecordScanDuration(scanType, duration)
}

// IncrementScanTotalPrometheus increments scan total using global metrics
func IncrementScanTotalPrometheus(scanType, status string) {
	GetGlobalMetrics().IncrementScansTotal(scanType, status)
}

// IncrementScanErrorsPrometheus increments scan errors using global metrics
func IncrementScanErrorsPrometheus(scanType, errorType string) {
	GetGlobalMetrics().IncrementScanErrors(scanType, errorType)
}

// RecordDiscoveryDurationPrometheus records discovery duration using global metrics
func RecordDiscoveryDurationPrometheus(method string, duration time.Duration) {
	GetGlobalMetrics().RecordDiscoveryDuration(method, duration)
}

// IncrementHostsDiscoveredPrometheus increments hosts discovered using global metrics
func IncrementHostsDiscoveredPrometheus(method, network string, count int) {
	GetGlobalMetrics().IncrementHostsDiscovered(method, network, count)
}

// RecordDatabaseQueryPrometheus records database query metrics using global metrics
func RecordDatabaseQueryPrometheus(operation string, duration time.Duration, success bool) {
	metrics := GetGlobalMetrics()
	status := "success"
	if !success {
		status = "error"
	}
	metrics.IncrementDatabaseQueries(operation, status)
	metrics.RecordDatabaseQueryDuration(operation, duration)
}

// SetActiveConnectionsPrometheus sets active database connections using global metrics
func SetActiveConnectionsPrometheus(count int) {
	GetGlobalMetrics().SetActiveConnections(count)
}
