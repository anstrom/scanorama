// Package metrics provides interfaces for metrics collection and monitoring.
package metrics

// MetricsRegistry defines the interface for metrics collection and management.
// This interface allows for easy mocking and testing of metrics functionality.
type MetricsRegistry interface {
	// SetEnabled enables or disables metrics collection.
	SetEnabled(enabled bool)

	// IsEnabled returns whether metrics collection is enabled.
	IsEnabled() bool

	// Counter increments a counter metric with the given name and labels.
	Counter(name string, labels Labels)

	// Gauge sets a gauge metric to the specified value with the given name and labels.
	Gauge(name string, value float64, labels Labels)

	// Histogram records a value in a histogram metric with the given name and labels.
	Histogram(name string, value float64, labels Labels)

	// GetMetrics returns a snapshot of all current metrics.
	GetMetrics() map[string]*Metric

	// Reset clears all metrics from the registry.
	Reset()
}

// Ensure that Registry implements MetricsRegistry interface.
var _ MetricsRegistry = (*Registry)(nil)
