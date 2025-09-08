package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricType(t *testing.T) {
	tests := []struct {
		name       string
		metricType MetricType
		expected   string
	}{
		{"counter type", TypeCounter, "counter"},
		{"gauge type", TypeGauge, "gauge"},
		{"histogram type", TypeHistogram, "histogram"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.metricType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.metricType))
			}
		})
	}
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()

	if registry == nil {
		t.Fatal("Registry should not be nil")
	}
	if !registry.IsEnabled() {
		t.Error("Registry should be enabled by default")
	}
	if registry.metrics == nil {
		t.Error("Metrics map should be initialized")
	}
}

func TestRegistryEnableDisable(t *testing.T) {
	registry := NewRegistry()

	t.Run("default enabled", func(t *testing.T) {
		if !registry.IsEnabled() {
			t.Error("Registry should be enabled by default")
		}
	})

	t.Run("disable", func(t *testing.T) {
		registry.SetEnabled(false)
		if registry.IsEnabled() {
			t.Error("Registry should be disabled")
		}
	})

	t.Run("enable", func(t *testing.T) {
		registry.SetEnabled(true)
		if !registry.IsEnabled() {
			t.Error("Registry should be enabled")
		}
	})
}

func TestCounter(t *testing.T) {
	registry := NewRegistry()

	t.Run("increment counter", func(t *testing.T) {
		labels := Labels{"component": "scanner"}
		registry.Counter("test_counter", labels)

		metrics := registry.GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}

		for _, metric := range metrics {
			if metric.Name != "test_counter" {
				t.Errorf("Expected name 'test_counter', got '%s'", metric.Name)
			}
			if metric.Type != TypeCounter {
				t.Errorf("Expected type %s, got %s", TypeCounter, metric.Type)
			}
			if metric.Value != 1 {
				t.Errorf("Expected value 1, got %f", metric.Value)
			}
		}
	})

	t.Run("multiple increments", func(t *testing.T) {
		registry.Reset()
		labels := Labels{"component": "scanner"}

		registry.Counter("test_counter", labels)
		registry.Counter("test_counter", labels)
		registry.Counter("test_counter", labels)

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			if metric.Value != 3 {
				t.Errorf("Expected value 3, got %f", metric.Value)
			}
		}
	})

	t.Run("different labels create different metrics", func(t *testing.T) {
		registry.Reset()

		registry.Counter("test_counter", Labels{"component": "scanner"})
		registry.Counter("test_counter", Labels{"component": "database"})

		metrics := registry.GetMetrics()
		if len(metrics) != 2 {
			t.Errorf("Expected 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("disabled registry", func(t *testing.T) {
		registry.Reset()
		registry.SetEnabled(false)

		registry.Counter("test_counter", nil)

		metrics := registry.GetMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics when disabled, got %d", len(metrics))
		}
	})
}

func TestGauge(t *testing.T) {
	registry := NewRegistry()

	t.Run("set gauge value", func(t *testing.T) {
		labels := Labels{"host": "localhost"}
		registry.Gauge("test_gauge", 42.5, labels)

		metrics := registry.GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}

		for _, metric := range metrics {
			if metric.Name != "test_gauge" {
				t.Errorf("Expected name 'test_gauge', got '%s'", metric.Name)
			}
			if metric.Type != TypeGauge {
				t.Errorf("Expected type %s, got %s", TypeGauge, metric.Type)
			}
			if metric.Value != 42.5 {
				t.Errorf("Expected value 42.5, got %f", metric.Value)
			}
		}
	})

	t.Run("overwrite gauge value", func(t *testing.T) {
		registry.Reset()
		labels := Labels{"host": "localhost"}

		registry.Gauge("test_gauge", 10.0, labels)
		registry.Gauge("test_gauge", 20.0, labels)

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			if metric.Value != 20.0 {
				t.Errorf("Expected value 20.0, got %f", metric.Value)
			}
		}
	})

	t.Run("disabled registry", func(t *testing.T) {
		registry.Reset()
		registry.SetEnabled(false)

		registry.Gauge("test_gauge", 100.0, nil)

		metrics := registry.GetMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics when disabled, got %d", len(metrics))
		}
	})
}

func TestHistogram(t *testing.T) {
	registry := NewRegistry()

	t.Run("record histogram value", func(t *testing.T) {
		labels := Labels{"operation": "scan"}
		registry.Histogram("test_histogram", 1.5, labels)

		metrics := registry.GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}

		for _, metric := range metrics {
			if metric.Name != "test_histogram" {
				t.Errorf("Expected name 'test_histogram', got '%s'", metric.Name)
			}
			if metric.Type != TypeHistogram {
				t.Errorf("Expected type %s, got %s", TypeHistogram, metric.Type)
			}
			if metric.Value != 1.5 {
				t.Errorf("Expected value 1.5, got %f", metric.Value)
			}
		}
	})

	t.Run("multiple histogram values", func(t *testing.T) {
		registry.Reset()
		labels := Labels{"operation": "scan"}

		registry.Histogram("test_histogram", 1.0, labels)
		registry.Histogram("test_histogram", 2.0, labels)

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			// Current implementation just keeps the last value
			if metric.Value != 2.0 {
				t.Errorf("Expected value 2.0, got %f", metric.Value)
			}
		}
	})

	t.Run("disabled registry", func(t *testing.T) {
		registry.Reset()
		registry.SetEnabled(false)

		registry.Histogram("test_histogram", 5.0, nil)

		metrics := registry.GetMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics when disabled, got %d", len(metrics))
		}
	})
}

func TestGetMetrics(t *testing.T) {
	registry := NewRegistry()

	t.Run("empty registry", func(t *testing.T) {
		metrics := registry.GetMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics, got %d", len(metrics))
		}
	})

	t.Run("multiple metrics", func(t *testing.T) {
		registry.Counter("counter1", Labels{"type": "test"})
		registry.Gauge("gauge1", 10.0, Labels{"type": "test"})
		registry.Histogram("histogram1", 2.5, Labels{"type": "test"})

		metrics := registry.GetMetrics()
		if len(metrics) != 3 {
			t.Errorf("Expected 3 metrics, got %d", len(metrics))
		}

		// Verify each metric type exists
		types := make(map[MetricType]bool)
		for _, metric := range metrics {
			types[metric.Type] = true
		}

		if !types[TypeCounter] {
			t.Error("Should have counter metric")
		}
		if !types[TypeGauge] {
			t.Error("Should have gauge metric")
		}
		if !types[TypeHistogram] {
			t.Error("Should have histogram metric")
		}
	})

	t.Run("metrics are copied", func(t *testing.T) {
		registry.Reset()
		registry.Counter("test", nil)

		metrics1 := registry.GetMetrics()
		metrics2 := registry.GetMetrics()

		// Modify one copy
		for key, metric := range metrics1 {
			metric.Value = 999
			metrics1[key] = metric
		}

		// Other copy should be unchanged
		for _, metric := range metrics2 {
			if metric.Value != 1 {
				t.Errorf("Expected original value 1, got %f", metric.Value)
			}
		}
	})
}

func TestReset(t *testing.T) {
	registry := NewRegistry()

	registry.Counter("counter1", nil)
	registry.Gauge("gauge1", 10.0, nil)
	registry.Histogram("histogram1", 2.5, nil)

	metrics := registry.GetMetrics()
	if len(metrics) != 3 {
		t.Errorf("Expected 3 metrics before reset, got %d", len(metrics))
	}

	registry.Reset()

	metrics = registry.GetMetrics()
	if len(metrics) != 0 {
		t.Errorf("Expected 0 metrics after reset, got %d", len(metrics))
	}
}

func TestConcurrentAccess(t *testing.T) {
	registry := NewRegistry()

	t.Run("concurrent counters", func(t *testing.T) {
		registry.Reset()

		var wg sync.WaitGroup
		numGoroutines := 10
		incrementsPerGoroutine := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < incrementsPerGoroutine; j++ {
					registry.Counter("concurrent_counter", nil)
				}
			}()
		}

		wg.Wait()

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			expected := float64(numGoroutines * incrementsPerGoroutine)
			if metric.Value != expected {
				t.Errorf("Expected value %f, got %f", expected, metric.Value)
			}
		}
	})

	t.Run("concurrent gauges", func(t *testing.T) {
		registry.Reset()

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(value float64) {
				defer wg.Done()
				registry.Gauge("concurrent_gauge", value, nil)
			}(float64(i))
		}

		wg.Wait()

		metrics := registry.GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}
		// The final value should be one of the goroutine values (0-9)
		for _, metric := range metrics {
			if metric.Value < 0 || metric.Value >= 10 {
				t.Errorf("Expected value between 0-9, got %f", metric.Value)
			}
		}
	})

	t.Run("concurrent reads and writes", func(t *testing.T) {
		registry.Reset()

		var wg sync.WaitGroup

		// Writers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					registry.Counter("rw_counter", nil)
				}
			}()
		}

		// Readers
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					_ = registry.GetMetrics()
				}
			}()
		}

		wg.Wait()

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			if metric.Value != 250 {
				t.Errorf("Expected value 250, got %f", metric.Value)
			}
		}
	})
}

func TestLabels(t *testing.T) {
	registry := NewRegistry()

	t.Run("nil labels", func(t *testing.T) {
		registry.Counter("test", nil)
		metrics := registry.GetMetrics()

		for _, metric := range metrics {
			if metric.Labels != nil {
				t.Error("Labels should be nil when passed as nil")
			}
		}
	})

	t.Run("empty labels", func(t *testing.T) {
		registry.Reset()
		registry.Counter("test", Labels{})
		metrics := registry.GetMetrics()

		for _, metric := range metrics {
			if len(metric.Labels) != 0 {
				t.Errorf("Expected empty labels, got %d labels", len(metric.Labels))
			}
		}
	})

	t.Run("multiple labels", func(t *testing.T) {
		registry.Reset()
		labels := Labels{
			"component": "scanner",
			"host":      "localhost",
			"port":      "80",
		}
		registry.Counter("test", labels)

		metrics := registry.GetMetrics()
		for _, metric := range metrics {
			if len(metric.Labels) != 3 {
				t.Errorf("Expected 3 labels, got %d", len(metric.Labels))
			}
			if metric.Labels["component"] != "scanner" {
				t.Errorf("Expected component 'scanner', got '%s'", metric.Labels["component"])
			}
		}
	})
}

func TestMakeKey(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name       string
		metricName string
		labels     Labels
		expected   string
	}{
		{
			name:       "no labels",
			metricName: "test_metric",
			labels:     nil,
			expected:   "test_metric",
		},
		{
			name:       "empty labels",
			metricName: "test_metric",
			labels:     Labels{},
			expected:   "test_metric",
		},
		{
			name:       "single label",
			metricName: "test_metric",
			labels:     Labels{"key": "value"},
			expected:   "test_metric:key=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := registry.makeKey(tt.metricName, tt.labels)
			// For multiple labels, we can't predict the exact order,
			// so we'll just check that it contains the expected parts
			if tt.name == "single label" || tt.name == "no labels" || tt.name == "empty labels" {
				if key != tt.expected {
					t.Errorf("Expected key '%s', got '%s'", tt.expected, key)
				}
			}
		})
	}
}

func TestCopyLabels(t *testing.T) {
	t.Run("nil labels", func(t *testing.T) {
		copied := copyLabels(nil)
		if copied != nil {
			t.Error("Copied nil labels should be nil")
		}
	})

	t.Run("empty labels", func(t *testing.T) {
		original := Labels{}
		copied := copyLabels(original)
		if len(copied) != 0 {
			t.Errorf("Expected empty copy, got %d labels", len(copied))
		}
	})

	t.Run("labels with data", func(t *testing.T) {
		original := Labels{"key1": "value1", "key2": "value2"}
		copied := copyLabels(original)

		if len(copied) != 2 {
			t.Errorf("Expected 2 labels in copy, got %d", len(copied))
		}

		// Modify original
		original["key3"] = "value3"

		// Copy should be unchanged
		if len(copied) != 2 {
			t.Errorf("Copy should be unaffected by original changes, got %d labels", len(copied))
		}
	})
}

func TestTimer(t *testing.T) {
	t.Run("timer creation", func(t *testing.T) {
		labels := Labels{"operation": "test"}
		timer := NewTimer("test_timer", labels)

		if timer == nil {
			t.Fatal("Timer should not be nil")
		}
		if timer.name != "test_timer" {
			t.Errorf("Expected name 'test_timer', got '%s'", timer.name)
		}
		if timer.labels["operation"] != "test" {
			t.Errorf("Expected operation 'test', got '%s'", timer.labels["operation"])
		}
		if timer.start.IsZero() {
			t.Error("Timer start time should be set")
		}
	})

	t.Run("timer measures duration", func(t *testing.T) {
		Reset() // Use global registry since Timer.Stop() calls global Histogram()

		timer := NewTimer("duration_test", nil)
		time.Sleep(10 * time.Millisecond) // Small sleep
		timer.Stop()

		metrics := GetMetrics() // Use global GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric after timer stop, got %d", len(metrics))
		}

		for _, metric := range metrics {
			if metric.Type != TypeHistogram {
				t.Errorf("Timer should create histogram, got %s", metric.Type)
			}
			if metric.Value <= 0 {
				t.Errorf("Timer should record positive duration, got %f", metric.Value)
			}
			// Should be at least 10ms (0.01 seconds)
			if metric.Value < 0.01 {
				t.Errorf("Timer should record at least 10ms, got %f seconds", metric.Value)
			}
		}
	})
}

func TestGlobalRegistry(t *testing.T) {
	// Save original registry
	originalRegistry := Default()
	defer SetDefault(originalRegistry)

	// Create test registry
	testRegistry := NewRegistry()
	SetDefault(testRegistry)

	t.Run("global functions use default registry", func(t *testing.T) {
		Reset()

		Counter("global_counter", Labels{"test": "true"})
		Gauge("global_gauge", 50.0, Labels{"test": "true"})
		Histogram("global_histogram", 3.14, Labels{"test": "true"})

		metrics := GetMetrics()
		if len(metrics) != 3 {
			t.Errorf("Expected 3 metrics, got %d", len(metrics))
		}

		types := make(map[MetricType]bool)
		for _, metric := range metrics {
			types[metric.Type] = true
		}

		if !types[TypeCounter] || !types[TypeGauge] || !types[TypeHistogram] {
			t.Error("Should have all three metric types")
		}
	})

	t.Run("global enable/disable", func(t *testing.T) {
		Reset()
		SetEnabled(false)

		Counter("disabled_counter", nil)

		metrics := GetMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics when globally disabled, got %d", len(metrics))
		}

		SetEnabled(true)
		Counter("enabled_counter", nil)

		metrics = GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric when re-enabled, got %d", len(metrics))
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	// Save original registry
	originalRegistry := Default()
	defer SetDefault(originalRegistry)

	// Create test registry
	testRegistry := NewRegistry()
	SetDefault(testRegistry)

	t.Run("RecordScanDuration", func(t *testing.T) {
		Reset()
		duration := 2500 * time.Millisecond
		RecordScanDuration("tcp", "192.168.1.1", duration)

		metrics := GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}

		for _, metric := range metrics {
			if metric.Name != MetricScanDuration {
				t.Errorf("Expected name '%s', got '%s'", MetricScanDuration, metric.Name)
			}
			if metric.Value != 2.5 {
				t.Errorf("Expected value 2.5, got %f", metric.Value)
			}
			if metric.Labels[LabelScanType] != "tcp" {
				t.Errorf("Expected scan type 'tcp', got '%s'", metric.Labels[LabelScanType])
			}
			if metric.Labels[LabelTarget] != "192.168.1.1" {
				t.Errorf("Expected target '192.168.1.1', got '%s'", metric.Labels[LabelTarget])
			}
		}
	})

	t.Run("IncrementScanTotal", func(t *testing.T) {
		Reset()
		IncrementScanTotal("tcp", "success")
		IncrementScanTotal("tcp", "success")

		metrics := GetMetrics()
		if len(metrics) == 0 {
			t.Fatal("Expected at least 1 metric, got 0")
		}

		found := false
		for _, metric := range metrics {
			if metric.Name == MetricScanTotal {
				found = true
				if metric.Value != 2 {
					t.Errorf("Expected value 2, got %f", metric.Value)
				}
				// Check labels are correct
				if metric.Labels[LabelScanType] != "tcp" {
					t.Errorf("Expected scan_type 'tcp', got '%s'", metric.Labels[LabelScanType])
				}
				if metric.Labels[LabelStatus] != "success" {
					t.Errorf("Expected status 'success', got '%s'", metric.Labels[LabelStatus])
				}
			}
		}
		if !found {
			t.Errorf("Expected to find metric with name '%s'", MetricScanTotal)
		}
	})

	t.Run("IncrementScanErrors", func(t *testing.T) {
		Reset()
		IncrementScanErrors("tcp", "192.168.1.1", "timeout")

		metrics := GetMetrics()
		for _, metric := range metrics {
			if metric.Name != MetricScanErrors {
				t.Errorf("Expected name '%s', got '%s'", MetricScanErrors, metric.Name)
			}
			if metric.Labels[LabelError] != "timeout" {
				t.Errorf("Expected error 'timeout', got '%s'", metric.Labels[LabelError])
			}
		}
	})

	t.Run("RecordDiscoveryDuration", func(t *testing.T) {
		Reset()
		duration := 1500 * time.Millisecond
		RecordDiscoveryDuration("192.168.1.0/24", "ping", duration)

		metrics := GetMetrics()
		for _, metric := range metrics {
			if metric.Name != MetricDiscoveryDuration {
				t.Errorf("Expected name '%s', got '%s'", MetricDiscoveryDuration, metric.Name)
			}
			if metric.Value != 1.5 {
				t.Errorf("Expected value 1.5, got %f", metric.Value)
			}
		}
	})

	t.Run("IncrementHostsDiscovered", func(t *testing.T) {
		Reset()
		IncrementHostsDiscovered("192.168.1.0/24", "ping", 3)

		metrics := GetMetrics()
		for _, metric := range metrics {
			if metric.Name != MetricHostsDiscovered {
				t.Errorf("Expected name '%s', got '%s'", MetricHostsDiscovered, metric.Name)
			}
			if metric.Value != 3 {
				t.Errorf("Expected value 3, got %f", metric.Value)
			}
		}
	})

	t.Run("RecordDatabaseQuery", func(t *testing.T) {
		Reset()
		duration := 250 * time.Millisecond

		// Success case
		RecordDatabaseQuery("SELECT", duration, true)

		// Error case
		RecordDatabaseQuery("INSERT", duration, false)

		metrics := GetMetrics()
		if len(metrics) != 4 { // 2 counters + 2 histograms
			t.Errorf("Expected 4 metrics, got %d", len(metrics))
		}

		// Check that we have both success and error counters
		var successCounter, errorCounter, selectHist, insertHist bool
		for _, metric := range metrics {
			if metric.Name == MetricDatabaseQueries {
				if metric.Labels[LabelStatus] == "success" {
					successCounter = true
				}
				if metric.Labels[LabelStatus] == "error" {
					errorCounter = true
				}
			}
			if metric.Name == MetricDatabaseDuration {
				if metric.Labels[LabelOperation] == "SELECT" {
					selectHist = true
				}
				if metric.Labels[LabelOperation] == "INSERT" {
					insertHist = true
				}
			}
		}

		if !successCounter {
			t.Error("Should have success counter")
		}
		if !errorCounter {
			t.Error("Should have error counter")
		}
		if !selectHist {
			t.Error("Should have SELECT histogram")
		}
		if !insertHist {
			t.Error("Should have INSERT histogram")
		}
	})

	t.Run("SetActiveConnections", func(t *testing.T) {
		Reset()
		SetActiveConnections(5)

		metrics := GetMetrics()
		for _, metric := range metrics {
			if metric.Name != MetricDatabaseConnections {
				t.Errorf("Expected name '%s', got '%s'", MetricDatabaseConnections, metric.Name)
			}
			if metric.Type != TypeGauge {
				t.Errorf("Expected gauge type, got %s", metric.Type)
			}
			if metric.Value != 5 {
				t.Errorf("Expected value 5, got %f", metric.Value)
			}
		}
	})
}

func TestMetricConstants(t *testing.T) {
	metricNames := []string{
		MetricScanDuration,
		MetricScanTotal,
		MetricScanErrors,
		MetricPortsScanned,
		MetricHostsScanned,
		MetricDiscoveryDuration,
		MetricDiscoveryTotal,
		MetricDiscoveryErrors,
		MetricHostsDiscovered,
		MetricDatabaseQueries,
		MetricDatabaseErrors,
		MetricDatabaseDuration,
		MetricDatabaseConnections,
		MetricMemoryUsage,
		MetricGoroutines,
		MetricUptime,
	}

	for _, name := range metricNames {
		if name == "" {
			t.Errorf("Metric name should not be empty")
		}
		if !strings.Contains(name, "_") {
			t.Errorf("Metric name '%s' should follow snake_case convention", name)
		}
	}

	labelKeys := []string{
		LabelScanType,
		LabelTarget,
		LabelNetwork,
		LabelMethod,
		LabelStatus,
		LabelOperation,
		LabelError,
		LabelComponent,
	}

	for _, key := range labelKeys {
		if key == "" {
			t.Errorf("Label key should not be empty")
		}
	}
}

func TestTimestamp(t *testing.T) {
	registry := NewRegistry()

	before := time.Now()
	registry.Counter("timestamp_test", nil)
	after := time.Now()

	metrics := registry.GetMetrics()
	for _, metric := range metrics {
		if metric.Timestamp.Before(before) || metric.Timestamp.After(after) {
			t.Error("Metric timestamp should be set to current time")
		}
	}
}

func TestMetricUpdate(t *testing.T) {
	registry := NewRegistry()

	t.Run("counter updates timestamp", func(t *testing.T) {
		registry.Counter("test", nil)

		metrics1 := registry.GetMetrics()
		time.Sleep(1 * time.Millisecond)

		registry.Counter("test", nil)
		metrics2 := registry.GetMetrics()

		var timestamp1, timestamp2 time.Time
		for _, metric := range metrics1 {
			timestamp1 = metric.Timestamp
		}
		for _, metric := range metrics2 {
			timestamp2 = metric.Timestamp
		}

		if !timestamp2.After(timestamp1) {
			t.Error("Second counter increment should have later timestamp")
		}
	})

	t.Run("gauge updates timestamp", func(t *testing.T) {
		registry.Reset()

		registry.Gauge("test", 1.0, nil)
		metrics1 := registry.GetMetrics()

		time.Sleep(1 * time.Millisecond)
		registry.Gauge("test", 2.0, nil)
		metrics2 := registry.GetMetrics()

		var timestamp1, timestamp2 time.Time
		for _, metric := range metrics1 {
			timestamp1 = metric.Timestamp
		}
		for _, metric := range metrics2 {
			timestamp2 = metric.Timestamp
		}

		if !timestamp2.After(timestamp1) {
			t.Error("Gauge update should have later timestamp")
		}
	})
}

func TestEdgeCases(t *testing.T) {
	registry := NewRegistry()

	t.Run("empty metric name", func(t *testing.T) {
		registry.Reset()
		registry.Counter("", nil)

		metrics := registry.GetMetrics()
		if len(metrics) != 1 {
			t.Errorf("Expected 1 metric, got %d", len(metrics))
		}
	})
}
