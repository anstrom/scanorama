// Package metrics provides basic monitoring and metrics collection for scanorama.
// It supports counters, gauges, and histograms with label support for tracking
// application performance and operational metrics.
package metrics

import (
	"sync"
	"time"
)

// MetricType represents the type of metric.
type MetricType string

const (
	TypeCounter   MetricType = "counter"
	TypeGauge     MetricType = "gauge"
	TypeHistogram MetricType = "histogram"
)

// Labels represents key-value pairs for metric labels.
type Labels map[string]string

// Metric represents a single metric with its metadata.
type Metric struct {
	Name      string
	Type      MetricType
	Value     float64
	Labels    Labels
	Timestamp time.Time
}

// Registry holds all metrics and provides collection functionality.
type Registry struct {
	mu      sync.RWMutex
	metrics map[string]*Metric
	enabled bool
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		metrics: make(map[string]*Metric),
		enabled: true,
	}
}

// SetEnabled enables or disables metrics collection.
func (r *Registry) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
}

// IsEnabled returns whether metrics collection is enabled.
func (r *Registry) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

// Counter increments a counter metric.
func (r *Registry) Counter(name string, labels Labels) {
	if !r.IsEnabled() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(name, labels)
	if metric, exists := r.metrics[key]; exists {
		metric.Value++
		metric.Timestamp = time.Now()
	} else {
		r.metrics[key] = &Metric{
			Name:      name,
			Type:      TypeCounter,
			Value:     1,
			Labels:    labels,
			Timestamp: time.Now(),
		}
	}
}

// Gauge sets a gauge metric value.
func (r *Registry) Gauge(name string, value float64, labels Labels) {
	if !r.IsEnabled() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(name, labels)
	r.metrics[key] = &Metric{
		Name:      name,
		Type:      TypeGauge,
		Value:     value,
		Labels:    labels,
		Timestamp: time.Now(),
	}
}

// Histogram records a value in a histogram metric.
func (r *Registry) Histogram(name string, value float64, labels Labels) {
	if !r.IsEnabled() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(name, labels)
	if metric, exists := r.metrics[key]; exists {
		// Simple histogram implementation - just track last value
		// Can be extended to proper buckets later
		metric.Value = value
		metric.Timestamp = time.Now()
	} else {
		r.metrics[key] = &Metric{
			Name:      name,
			Type:      TypeHistogram,
			Value:     value,
			Labels:    labels,
			Timestamp: time.Now(),
		}
	}
}

// GetMetrics returns a snapshot of all current metrics.
func (r *Registry) GetMetrics() map[string]*Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*Metric)
	for key, metric := range r.metrics {
		// Create a copy to avoid race conditions
		result[key] = &Metric{
			Name:      metric.Name,
			Type:      metric.Type,
			Value:     metric.Value,
			Labels:    copyLabels(metric.Labels),
			Timestamp: metric.Timestamp,
		}
	}
	return result
}

// Reset clears all metrics.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = make(map[string]*Metric)
}

// makeKey creates a unique key for a metric based on name and labels.
func (r *Registry) makeKey(name string, labels Labels) string {
	if len(labels) == 0 {
		return name
	}

	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}

// copyLabels creates a copy of labels map.
func copyLabels(labels Labels) Labels {
	if labels == nil {
		return nil
	}
	result := make(Labels)
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// Global registry instance.
var defaultRegistry = NewRegistry()

// SetDefault sets the default metrics registry.
func SetDefault(registry *Registry) {
	defaultRegistry = registry
}

// Default returns the default metrics registry.
func Default() *Registry {
	return defaultRegistry
}

// SetEnabled enables or disables metrics collection on the default registry.
func SetEnabled(enabled bool) {
	defaultRegistry.SetEnabled(enabled)
}

// Counter increments a counter metric on the default registry.
func Counter(name string, labels Labels) {
	defaultRegistry.Counter(name, labels)
}

// Gauge sets a gauge metric on the default registry.
func Gauge(name string, value float64, labels Labels) {
	defaultRegistry.Gauge(name, value, labels)
}

// Histogram records a histogram value on the default registry.
func Histogram(name string, value float64, labels Labels) {
	defaultRegistry.Histogram(name, value, labels)
}

// GetMetrics returns all metrics from the default registry.
func GetMetrics() map[string]*Metric {
	return defaultRegistry.GetMetrics()
}

// Reset clears all metrics from the default registry.
func Reset() {
	defaultRegistry.Reset()
}

// Timer provides a simple way to measure execution time.
type Timer struct {
	start  time.Time
	name   string
	labels Labels
}

// NewTimer creates a new timer for measuring execution time.
func NewTimer(name string, labels Labels) *Timer {
	return &Timer{
		start:  time.Now(),
		name:   name,
		labels: labels,
	}
}

// Stop stops the timer and records the duration as a histogram.
func (t *Timer) Stop() {
	duration := time.Since(t.start)
	Histogram(t.name, duration.Seconds(), t.labels)
}

// Predefined metric names for common operations.
const (
	// Scan metrics.
	MetricScanDuration = "scan_duration_seconds"
	MetricScanTotal    = "scan_total"
	MetricScanErrors   = "scan_errors_total"
	MetricPortsScanned = "ports_scanned_total"
	MetricHostsScanned = "hosts_scanned_total"

	// Discovery metrics.
	MetricDiscoveryDuration = "discovery_duration_seconds"
	MetricDiscoveryTotal    = "discovery_total"
	MetricDiscoveryErrors   = "discovery_errors_total"
	MetricHostsDiscovered   = "hosts_discovered_total"

	// Database metrics.
	MetricDatabaseQueries     = "database_queries_total"
	MetricDatabaseErrors      = "database_errors_total"
	MetricDatabaseDuration    = "database_query_duration_seconds"
	MetricDatabaseConnections = "database_connections_active"

	// System metrics.
	MetricMemoryUsage = "memory_usage_bytes"
	MetricGoroutines  = "goroutines_active"
	MetricUptime      = "uptime_seconds"
)

// Common label keys.
const (
	LabelScanType  = "scan_type"
	LabelTarget    = "target"
	LabelNetwork   = "network"
	LabelMethod    = "method"
	LabelStatus    = "status"
	LabelOperation = "operation"
	LabelError     = "error"
	LabelComponent = "component"
)

// Helper functions for common metrics

// RecordScanDuration records the duration of a scan operation.
func RecordScanDuration(scanType, target string, duration time.Duration) {
	Histogram(MetricScanDuration, duration.Seconds(), Labels{
		LabelScanType: scanType,
		LabelTarget:   target,
	})
}

// IncrementScanTotal increments the total scan counter.
func IncrementScanTotal(scanType, status string) {
	Counter(MetricScanTotal, Labels{
		LabelScanType: scanType,
		LabelStatus:   status,
	})
}

// IncrementScanErrors increments the scan error counter.
func IncrementScanErrors(scanType, target, errorType string) {
	Counter(MetricScanErrors, Labels{
		LabelScanType: scanType,
		LabelTarget:   target,
		LabelError:    errorType,
	})
}

// RecordDiscoveryDuration records the duration of a discovery operation.
func RecordDiscoveryDuration(network, method string, duration time.Duration) {
	Histogram(MetricDiscoveryDuration, duration.Seconds(), Labels{
		LabelNetwork: network,
		LabelMethod:  method,
	})
}

// IncrementHostsDiscovered increments the hosts discovered counter.
func IncrementHostsDiscovered(network, method string, count int) {
	for i := 0; i < count; i++ {
		Counter(MetricHostsDiscovered, Labels{
			LabelNetwork: network,
			LabelMethod:  method,
		})
	}
}

// RecordDatabaseQuery records database query metrics.
func RecordDatabaseQuery(operation string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}

	Counter(MetricDatabaseQueries, Labels{
		LabelOperation: operation,
		LabelStatus:    status,
	})

	Histogram(MetricDatabaseDuration, duration.Seconds(), Labels{
		LabelOperation: operation,
	})
}

// SetActiveConnections sets the number of active database connections.
func SetActiveConnections(count int) {
	Gauge(MetricDatabaseConnections, float64(count), nil)
}
