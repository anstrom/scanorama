// Package metrics now provides Prometheus-based metrics. The legacy in-memory
// registry is removed. This file retains minimal API types and no-op shims
// to keep existing call sites compiling while Prometheus is the source of truth.
package metrics

import "time"

// MetricType and Metric kept for compatibility with existing interfaces/tests.
type MetricType string

const (
    TypeCounter   MetricType = "counter"
    TypeGauge     MetricType = "gauge"
    TypeHistogram MetricType = "histogram"
)

type Labels map[string]string

type Metric struct {
    Name      string
    Type      MetricType
    Value     float64
    Labels    Labels
    Timestamp time.Time
}

// Registry is a no-op adapter kept for compatibility with existing call sites.
type Registry struct{}

func NewRegistry() *Registry                         { return &Registry{} }
func (r *Registry) SetEnabled(enabled bool)          {}
func (r *Registry) IsEnabled() bool                  { return true }
func (r *Registry) Counter(name string, labels Labels)                {}
func (r *Registry) Gauge(name string, value float64, labels Labels)   {}
func (r *Registry) Histogram(name string, value float64, labels Labels) {}
func (r *Registry) GetMetrics() map[string]*Metric   { return map[string]*Metric{} }
func (r *Registry) Reset()                           {}

// Default registry shims for package-level helpers.
var defaultRegistry = NewRegistry()

func SetDefault(registry *Registry)                 { defaultRegistry = registry }
func Default() *Registry                            { return defaultRegistry }
func SetEnabled(enabled bool)                       { defaultRegistry.SetEnabled(enabled) }
func Counter(name string, labels Labels)            { defaultRegistry.Counter(name, labels) }
func Gauge(name string, value float64, labels Labels) { defaultRegistry.Gauge(name, value, labels) }
func Histogram(name string, value float64, labels Labels) {
    defaultRegistry.Histogram(name, value, labels)
}
func GetMetrics() map[string]*Metric { return defaultRegistry.GetMetrics() }
func Reset()                         { defaultRegistry.Reset() }

// Timer provides a simple way to measure execution time (no-op record).
type Timer struct {
    start  time.Time
    name   string
    labels Labels
}

func NewTimer(name string, labels Labels) *Timer {
    return &Timer{start: time.Now(), name: name, labels: labels}
}

func (t *Timer) Stop() {
    _ = time.Since(t.start) // no-op record
}

// Legacy metric name constants kept for compatibility. Prefer structured
// Prometheus metrics in prometheus.go.
const (
    MetricScanDuration       = "scan_duration_seconds"
    MetricScanTotal          = "scan_total"
    MetricScanErrors         = "scan_errors_total"
    MetricPortsScanned       = "ports_scanned_total"
    MetricHostsScanned       = "hosts_scanned_total"
    MetricDiscoveryDuration  = "discovery_duration_seconds"
    MetricDiscoveryTotal     = "discovery_total"
    MetricDiscoveryErrors    = "discovery_errors_total"
    MetricHostsDiscovered    = "hosts_discovered_total"
    MetricDatabaseQueries    = "database_queries_total"
    MetricDatabaseErrors     = "database_errors_total"
    MetricDatabaseDuration   = "database_query_duration_seconds"
    MetricDatabaseConnections = "database_connections_active"

    // System metrics (legacy names)
    MetricMemoryUsage = "memory_usage_bytes"
    MetricGoroutines  = "goroutines_active"
    MetricUptime      = "uptime_seconds"
)

// Legacy label keys kept for compatibility.
const (
    LabelScanType  = "scan_type"
    LabelTarget    = "target"
    LabelNetwork   = "network"
    LabelMethod    = "method"
    LabelStatus    = "status"
    LabelOperation = "operation"
    LabelError     = "error"
    LabelComponent = "component"

    StatusSuccess = "success"
    StatusError   = "error"
)
