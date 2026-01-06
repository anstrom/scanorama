package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
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

func TestPrometheusMetrics_ScanMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	// Test IncrementScansTotal
	pm.IncrementScansTotal("nmap", "success")
	pm.IncrementScansTotal("nmap", "success")
	pm.IncrementScansTotal("masscan", "error")

	count := testutil.CollectAndCount(pm.scansTotal)
	if count != 2 {
		t.Errorf("expected 2 label combinations, got %d", count)
	}

	// Test RecordScanDuration
	pm.RecordScanDuration("nmap", 5*time.Second)
	pm.RecordScanDuration("nmap", 3*time.Second)
	pm.RecordScanDuration("masscan", 2*time.Second)

	count = testutil.CollectAndCount(pm.scanDuration)
	if count != 2 {
		t.Errorf("expected 2 scan types, got %d", count)
	}

	// Test IncrementScanErrors
	pm.IncrementScanErrors("nmap", "timeout")
	pm.IncrementScanErrors("nmap", "connection_refused")

	count = testutil.CollectAndCount(pm.scanErrors)
	if count != 2 {
		t.Errorf("expected 2 error types, got %d", count)
	}

	// Test IncrementPortsScanned
	pm.IncrementPortsScanned("nmap", "open", 10)
	pm.IncrementPortsScanned("nmap", "open", 5)
	pm.IncrementPortsScanned("nmap", "closed", 100)

	count = testutil.CollectAndCount(pm.portsScanned)
	if count != 2 {
		t.Errorf("expected 2 port status types, got %d", count)
	}

	// Test IncrementHostsScanned
	pm.IncrementHostsScanned("nmap", "success", 3)
	pm.IncrementHostsScanned("masscan", "success", 10)

	count = testutil.CollectAndCount(pm.hostsScanned)
	if count != 2 {
		t.Errorf("expected 2 scan type combinations, got %d", count)
	}

	// Test SetActiveScans
	pm.SetActiveScans(5)
	pm.SetActiveScans(3)

	count = testutil.CollectAndCount(pm.activeScans)
	if count != 1 {
		t.Errorf("expected 1 gauge metric, got %d", count)
	}
}

func TestPrometheusMetrics_DiscoveryMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	// Test IncrementDiscoveryTotal
	pm.IncrementDiscoveryTotal("ping", "success")
	pm.IncrementDiscoveryTotal("ping", "success")
	pm.IncrementDiscoveryTotal("arp", "error")

	count := testutil.CollectAndCount(pm.discoveryTotal)
	if count != 2 {
		t.Errorf("expected 2 label combinations, got %d", count)
	}

	// Test RecordDiscoveryDuration
	pm.RecordDiscoveryDuration("ping", 1*time.Second)
	pm.RecordDiscoveryDuration("arp", 500*time.Millisecond)

	count = testutil.CollectAndCount(pm.discoveryDuration)
	if count != 2 {
		t.Errorf("expected 2 discovery methods, got %d", count)
	}

	// Test IncrementDiscoveryErrors
	pm.IncrementDiscoveryErrors("ping", "timeout")
	pm.IncrementDiscoveryErrors("arp", "permission_denied")

	count = testutil.CollectAndCount(pm.discoveryErrors)
	if count != 2 {
		t.Errorf("expected 2 error types, got %d", count)
	}

	// Test IncrementHostsDiscovered
	pm.IncrementHostsDiscovered("ping", "192.168.1.0/24", 10)
	pm.IncrementHostsDiscovered("arp", "10.0.0.0/8", 5)

	count = testutil.CollectAndCount(pm.hostsDiscovered)
	if count != 2 {
		t.Errorf("expected 2 method/network combinations, got %d", count)
	}

	// Test SetActiveDiscovery
	pm.SetActiveDiscovery(2)
	pm.SetActiveDiscovery(0)

	count = testutil.CollectAndCount(pm.activeDiscovery)
	if count != 1 {
		t.Errorf("expected 1 gauge metric, got %d", count)
	}
}

func TestPrometheusMetrics_DatabaseMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	// Test IncrementDatabaseQueries
	pm.IncrementDatabaseQueries("select", "success")
	pm.IncrementDatabaseQueries("insert", "error")

	count := testutil.CollectAndCount(pm.dbQueries)
	if count != 2 {
		t.Errorf("expected 2 query types, got %d", count)
	}

	// Test RecordDatabaseQueryDuration
	pm.RecordDatabaseQueryDuration("select", 10*time.Millisecond)
	pm.RecordDatabaseQueryDuration("insert", 5*time.Millisecond)

	count = testutil.CollectAndCount(pm.dbQueryDuration)
	if count != 2 {
		t.Errorf("expected 2 operation types, got %d", count)
	}

	// Test SetActiveConnections
	pm.SetActiveConnections(10)
	pm.SetActiveConnections(8)

	count = testutil.CollectAndCount(pm.dbConnections)
	if count != 1 {
		t.Errorf("expected 1 gauge metric, got %d", count)
	}

	// Test IncrementDatabaseErrors
	pm.IncrementDatabaseErrors("select", "timeout")
	pm.IncrementDatabaseErrors("insert", "constraint_violation")

	count = testutil.CollectAndCount(pm.dbErrors)
	if count != 2 {
		t.Errorf("expected 2 error types, got %d", count)
	}
}

func TestPrometheusMetrics_APIMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	// Test IncrementHTTPRequests
	pm.IncrementHTTPRequests("GET", "/api/scans", "200")
	pm.IncrementHTTPRequests("POST", "/api/scans", "201")
	pm.IncrementHTTPRequests("GET", "/api/scans", "200")

	count := testutil.CollectAndCount(pm.httpRequests)
	if count != 2 {
		t.Errorf("expected 2 endpoint/status combinations, got %d", count)
	}

	// Test RecordHTTPDuration
	pm.RecordHTTPDuration("GET", "/api/scans", 100*time.Millisecond)
	pm.RecordHTTPDuration("POST", "/api/scans", 200*time.Millisecond)

	count = testutil.CollectAndCount(pm.httpDuration)
	if count != 2 {
		t.Errorf("expected 2 endpoint types, got %d", count)
	}

	// Test IncrementHTTPErrors
	pm.IncrementHTTPErrors("GET", "/api/scans", "timeout")
	pm.IncrementHTTPErrors("POST", "/api/scans", "validation_error")

	count = testutil.CollectAndCount(pm.httpErrors)
	if count != 2 {
		t.Errorf("expected 2 error types, got %d", count)
	}
}

func TestPrometheusMetrics_SystemMetrics(t *testing.T) {
	pm := NewPrometheusMetrics()

	// Test UpdateSystemMetrics
	pm.UpdateSystemMetrics()

	// Verify gauges are populated
	count := testutil.CollectAndCount(pm.memoryUsage)
	if count != 1 {
		t.Errorf("expected 1 memory metric, got %d", count)
	}

	count = testutil.CollectAndCount(pm.goroutines)
	if count != 1 {
		t.Errorf("expected 1 goroutines metric, got %d", count)
	}

	count = testutil.CollectAndCount(pm.uptime)
	if count != 1 {
		t.Errorf("expected 1 uptime metric, got %d", count)
	}

	// Test SetCPUUsage
	pm.SetCPUUsage(45.5)
	pm.SetCPUUsage(50.0)

	count = testutil.CollectAndCount(pm.cpuUsage)
	if count != 1 {
		t.Errorf("expected 1 CPU metric, got %d", count)
	}

	// Test GetLastUpdate
	before := pm.GetLastUpdate()
	time.Sleep(10 * time.Millisecond)
	pm.UpdateSystemMetrics()
	after := pm.GetLastUpdate()

	if !after.After(before) {
		t.Errorf("expected last update to change after UpdateSystemMetrics")
	}
}

func TestPrometheusMetrics_StartPeriodicUpdates(t *testing.T) {
	pm := NewPrometheusMetrics()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		pm.StartPeriodicUpdates(ctx, 20*time.Millisecond)
		close(done)
	}()

	// Wait for context to expire
	<-ctx.Done()
	<-done

	// Verify metrics were updated at least once
	count := testutil.CollectAndCount(pm.uptime)
	if count != 1 {
		t.Errorf("expected metrics to be updated, got %d uptime metrics", count)
	}
}

func TestPrometheusMetrics_GlobalInstance(t *testing.T) {
	// Test GetGlobalMetrics
	gm1 := GetGlobalMetrics()
	if gm1 == nil {
		t.Fatal("GetGlobalMetrics returned nil")
	}

	// Should return same instance
	gm2 := GetGlobalMetrics()
	if gm1 != gm2 {
		t.Error("GetGlobalMetrics should return same instance")
	}
}

func TestPrometheusMetrics_GlobalConvenienceFunctions(t *testing.T) {
	gm := GetGlobalMetrics()

	// Test RecordScanDurationPrometheus
	RecordScanDurationPrometheus("nmap", 5*time.Second)
	count := testutil.CollectAndCount(gm.scanDuration)
	if count == 0 {
		t.Error("RecordScanDurationPrometheus did not record metric")
	}

	// Test IncrementScanTotalPrometheus
	IncrementScanTotalPrometheus("nmap", "success")
	count = testutil.CollectAndCount(gm.scansTotal)
	if count == 0 {
		t.Error("IncrementScanTotalPrometheus did not record metric")
	}

	// Test IncrementScanErrorsPrometheus
	IncrementScanErrorsPrometheus("nmap", "timeout")
	count = testutil.CollectAndCount(gm.scanErrors)
	if count == 0 {
		t.Error("IncrementScanErrorsPrometheus did not record metric")
	}

	// Test RecordDiscoveryDurationPrometheus
	RecordDiscoveryDurationPrometheus("ping", 1*time.Second)
	count = testutil.CollectAndCount(gm.discoveryDuration)
	if count == 0 {
		t.Error("RecordDiscoveryDurationPrometheus did not record metric")
	}

	// Test IncrementHostsDiscoveredPrometheus
	IncrementHostsDiscoveredPrometheus("ping", "192.168.1.0/24", 5)
	count = testutil.CollectAndCount(gm.hostsDiscovered)
	if count == 0 {
		t.Error("IncrementHostsDiscoveredPrometheus did not record metric")
	}

	// Test RecordDatabaseQueryPrometheus with success
	RecordDatabaseQueryPrometheus("select", 10*time.Millisecond, true)
	count = testutil.CollectAndCount(gm.dbQueries)
	if count == 0 {
		t.Error("RecordDatabaseQueryPrometheus (success) did not record metric")
	}

	// Test RecordDatabaseQueryPrometheus with error
	RecordDatabaseQueryPrometheus("insert", 5*time.Millisecond, false)
	count = testutil.CollectAndCount(gm.dbQueryDuration)
	if count == 0 {
		t.Error("RecordDatabaseQueryPrometheus (error) did not record metric")
	}

	// Test SetActiveConnectionsPrometheus
	SetActiveConnectionsPrometheus(10)
	count = testutil.CollectAndCount(gm.dbConnections)
	if count == 0 {
		t.Error("SetActiveConnectionsPrometheus did not record metric")
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
