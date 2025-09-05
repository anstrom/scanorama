package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestPrometheusMetrics_InitializationAndUpdate(t *testing.T) {
	pm := NewPrometheusMetrics()
	if pm == nil {
		t.Fatalf("NewPrometheusMetrics returned nil")
	}

	reg := pm.GetRegistry()
	if reg == nil {
		t.Fatalf("GetRegistry returned nil")
	}

	// Should be able to update system metrics without panic
	pm.UpdateSystemMetrics()
	// Uptime should be increasing
	before := pm.GetUptime()
	time.Sleep(10 * time.Millisecond)
	after := pm.GetUptime()
	if before >= after {
		t.Fatalf("expected uptime to increase, before=%v after=%v", before, after)
	}
}

func TestPrometheusMetrics_HTTPHandlerServes(t *testing.T) {
	pm := NewPrometheusMetrics()
	// Update once to populate gauges
	pm.UpdateSystemMetrics()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	handler := promhttp.HandlerFor(pm.GetRegistry(), promhttp.HandlerOpts{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if body == "" {
		t.Fatalf("expected non-empty metrics body")
	}
	// Expect a known metric name prefix (namespace + subsystem + name)
	if !contains(body, "scanorama_system_uptime_seconds") {
		end := minInt(200, len(body))
		t.Fatalf("expected uptime metric in output, got: %s", body[:end])
	}
}

// contains is a tiny helper to avoid importing strings just for tests
func contains(s, substr string) bool {
	return substr == "" || (len(s) >= len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	// naive search sufficient for test
	n := len(s)
	m := len(substr)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == substr {
			return i
		}
	}
	return -1
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
