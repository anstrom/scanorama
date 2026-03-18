package metrics

import (
	"testing"
	"time"
)

// ── Registry construction ─────────────────────────────────────────────────────

func TestNewRegistry_ReturnsNonNil(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_IsEnabled_DefaultTrue(t *testing.T) {
	r := NewRegistry()
	if !r.IsEnabled() {
		t.Error("expected IsEnabled() to return true by default")
	}
}

func TestRegistry_SetEnabled_DoesNotPanic(t *testing.T) {
	// SetEnabled is an intentional no-op on the compatibility shim; verify it
	// accepts both values without panicking and that IsEnabled stays true.
	r := NewRegistry()
	r.SetEnabled(false)
	r.SetEnabled(true)
	if !r.IsEnabled() {
		t.Error("IsEnabled() should always return true on the no-op Registry")
	}
}

// ── No-op methods don't panic ─────────────────────────────────────────────────

func TestRegistry_Counter_DoesNotPanic(t *testing.T) {
	r := NewRegistry()
	r.Counter("test_counter", Labels{"env": "test"})
	r.Counter("test_counter", nil)
	r.Counter("", Labels{})
}

func TestRegistry_Gauge_DoesNotPanic(t *testing.T) {
	r := NewRegistry()
	r.Gauge("test_gauge", 42.0, Labels{"env": "test"})
	r.Gauge("test_gauge", 0, nil)
	r.Gauge("test_gauge", -1.5, Labels{})
}

func TestRegistry_Histogram_DoesNotPanic(t *testing.T) {
	r := NewRegistry()
	r.Histogram("test_histogram", 1.23, Labels{"env": "test"})
	r.Histogram("test_histogram", 0, nil)
	r.Histogram("", 99.9, Labels{})
}

func TestRegistry_Reset_DoesNotPanic(t *testing.T) {
	r := NewRegistry()
	r.Reset()
	r.Reset() // idempotent
}

// ── GetMetrics returns empty map, not nil ─────────────────────────────────────

func TestRegistry_GetMetrics_ReturnsEmptyMap(t *testing.T) {
	r := NewRegistry()
	m := r.GetMetrics()
	if m == nil {
		t.Fatal("GetMetrics() returned nil; expected empty map")
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestRegistry_GetMetrics_UnaffectedByOperations(t *testing.T) {
	r := NewRegistry()
	r.Counter("c", Labels{"k": "v"})
	r.Gauge("g", 1.0, nil)
	r.Histogram("h", 2.0, nil)

	m := r.GetMetrics()
	if len(m) != 0 {
		t.Errorf("no-op registry should always return empty map; got %d entries", len(m))
	}
}

// ── Default registry and package-level helpers ────────────────────────────────

func TestDefault_ReturnsNonNil(t *testing.T) {
	if Default() == nil {
		t.Fatal("Default() returned nil")
	}
}

func TestSetDefault_ReplacesSingleton(t *testing.T) {
	original := Default()
	defer SetDefault(original) // restore after test

	replacement := NewRegistry()
	SetDefault(replacement)

	if Default() != replacement {
		t.Error("SetDefault() did not replace the default registry")
	}
}

func TestSetEnabled_PackageLevel_DoesNotPanic(t *testing.T) {
	SetEnabled(false)
	SetEnabled(true)
}

func TestCounter_PackageLevel_DoesNotPanic(t *testing.T) {
	Counter("pkg_counter", Labels{"source": "test"})
	Counter("pkg_counter", nil)
}

func TestGauge_PackageLevel_DoesNotPanic(t *testing.T) {
	Gauge("pkg_gauge", 3.14, Labels{"source": "test"})
	Gauge("pkg_gauge", 0, nil)
}

func TestHistogram_PackageLevel_DoesNotPanic(t *testing.T) {
	Histogram("pkg_histogram", 1.0, Labels{"source": "test"})
	Histogram("pkg_histogram", 0, nil)
}

func TestGetMetrics_PackageLevel_ReturnsEmptyMap(t *testing.T) {
	m := GetMetrics()
	if m == nil {
		t.Fatal("package-level GetMetrics() returned nil")
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestReset_PackageLevel_DoesNotPanic(t *testing.T) {
	Reset()
	Reset()
}

// ── Timer ─────────────────────────────────────────────────────────────────────

func TestNewTimer_ReturnsNonNil(t *testing.T) {
	timer := NewTimer("test_op", Labels{"env": "test"})
	if timer == nil {
		t.Fatal("NewTimer() returned nil")
	}
}

func TestTimer_Stop_DoesNotPanic(t *testing.T) {
	timer := NewTimer("test_op", Labels{"env": "test"})
	timer.Stop()
}

func TestTimer_Stop_CanBeCalledMultipleTimes(t *testing.T) {
	timer := NewTimer("test_op", nil)
	timer.Stop()
	timer.Stop() // no-op, must not panic
}

func TestNewTimer_NilLabels_DoesNotPanic(t *testing.T) {
	timer := NewTimer("test_op", nil)
	if timer == nil {
		t.Fatal("NewTimer() with nil labels returned nil")
	}
	timer.Stop()
}

func TestNewTimer_EmptyName_DoesNotPanic(t *testing.T) {
	timer := NewTimer("", Labels{})
	if timer == nil {
		t.Fatal("NewTimer() with empty name returned nil")
	}
	timer.Stop()
}

func TestTimer_StartIsRecorded(t *testing.T) {
	before := time.Now()
	timer := NewTimer("test_op", nil)
	after := time.Now()

	if timer.start.Before(before) || timer.start.After(after) {
		t.Errorf("timer.start %v is outside expected range [%v, %v]", timer.start, before, after)
	}
}

func TestTimer_NameAndLabelsStored(t *testing.T) {
	labels := Labels{"component": "scanner", "env": "test"}
	timer := NewTimer("my_operation", labels)

	if timer.name != "my_operation" {
		t.Errorf("expected name %q, got %q", "my_operation", timer.name)
	}
	if timer.labels["component"] != "scanner" {
		t.Errorf("expected label component=scanner, got %q", timer.labels["component"])
	}
}

// ── Metric type constants ─────────────────────────────────────────────────────

func TestMetricTypeConstants(t *testing.T) {
	if TypeCounter != "counter" {
		t.Errorf("TypeCounter = %q, want %q", TypeCounter, "counter")
	}
	if TypeGauge != "gauge" {
		t.Errorf("TypeGauge = %q, want %q", TypeGauge, "gauge")
	}
	if TypeHistogram != "histogram" {
		t.Errorf("TypeHistogram = %q, want %q", TypeHistogram, "histogram")
	}
}

// ── Legacy metric name constants ──────────────────────────────────────────────

func TestLegacyMetricNameConstants(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"MetricScanDuration", MetricScanDuration},
		{"MetricScanTotal", MetricScanTotal},
		{"MetricScanErrors", MetricScanErrors},
		{"MetricPortsScanned", MetricPortsScanned},
		{"MetricHostsScanned", MetricHostsScanned},
		{"MetricDiscoveryDuration", MetricDiscoveryDuration},
		{"MetricDiscoveryTotal", MetricDiscoveryTotal},
		{"MetricDiscoveryErrors", MetricDiscoveryErrors},
		{"MetricHostsDiscovered", MetricHostsDiscovered},
		{"MetricDatabaseQueries", MetricDatabaseQueries},
		{"MetricDatabaseErrors", MetricDatabaseErrors},
		{"MetricDatabaseDuration", MetricDatabaseDuration},
		{"MetricDatabaseConnections", MetricDatabaseConnections},
		{"MetricMemoryUsage", MetricMemoryUsage},
		{"MetricGoroutines", MetricGoroutines},
		{"MetricUptime", MetricUptime},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "" {
				t.Errorf("constant %s is empty", tc.name)
			}
		})
	}
}

// ── Legacy label key constants ────────────────────────────────────────────────

func TestLegacyLabelKeyConstants(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"LabelScanType", LabelScanType},
		{"LabelTarget", LabelTarget},
		{"LabelNetwork", LabelNetwork},
		{"LabelMethod", LabelMethod},
		{"LabelStatus", LabelStatus},
		{"LabelOperation", LabelOperation},
		{"LabelError", LabelError},
		{"LabelComponent", LabelComponent},
		{"StatusSuccess", StatusSuccess},
		{"StatusError", StatusError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "" {
				t.Errorf("constant %s is empty", tc.name)
			}
		})
	}
}

// ── Labels type ───────────────────────────────────────────────────────────────

func TestLabels_IsMapStringString(t *testing.T) {
	l := Labels{"key1": "val1", "key2": "val2"}
	if l["key1"] != "val1" {
		t.Errorf("expected val1, got %q", l["key1"])
	}
	if l["key2"] != "val2" {
		t.Errorf("expected val2, got %q", l["key2"])
	}
}

// ── Metric struct ─────────────────────────────────────────────────────────────

func TestMetric_FieldsAssignable(t *testing.T) {
	now := time.Now()
	m := Metric{
		Name:      "test_metric",
		Type:      TypeCounter,
		Value:     1.0,
		Labels:    Labels{"env": "test"},
		Timestamp: now,
	}

	if m.Name != "test_metric" {
		t.Errorf("Name = %q, want %q", m.Name, "test_metric")
	}
	if m.Type != TypeCounter {
		t.Errorf("Type = %q, want %q", m.Type, TypeCounter)
	}
	if m.Value != 1.0 {
		t.Errorf("Value = %v, want 1.0", m.Value)
	}
	if m.Labels["env"] != "test" {
		t.Errorf("Labels[env] = %q, want %q", m.Labels["env"], "test")
	}
	if !m.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", m.Timestamp, now)
	}
}
