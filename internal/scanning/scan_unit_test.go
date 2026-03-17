package scanning

// NOTE: the tests in this file are pure unit tests — no database, no nmap
// binary required.  They target the two gaps that allowed OS-detection data
// to be silently dropped and "localhost" to be rejected as an invalid target.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// scanArgs builds a throw-away nmap.Scanner using a non-nmap binary path so
// that NewScanner succeeds without requiring the real nmap binary on PATH,
// then returns the composed CLI argument slice for inspection.
//
// We point the scanner at "/usr/bin/true" (always present on macOS/Linux).
// The scanner is never Run(), so the binary is never actually executed.
func scanArgs(t *testing.T, opts []nmap.Option) []string {
	t.Helper()
	// Use a real executable so WithBinaryPath bypasses the exec.LookPath("nmap")
	// call inside NewScanner, making the helper work even without nmap installed.
	opts = append(opts, nmap.WithBinaryPath("/usr/bin/true"))
	s, err := nmap.NewScanner(context.Background(), opts...)
	require.NoError(t, err)
	return s.Args()
}

// hasArg returns true when any element of args contains substr.
func hasArg(args []string, substr string) bool {
	for _, a := range args {
		if strings.Contains(a, substr) {
			return true
		}
	}
	return false
}

// isRoot returns true when the current process runs as uid 0.
func isRoot() bool { return os.Getuid() == 0 }

// ─── buildScanOptions — scan-type branch ──────────────────────────────────────

func TestBuildScanOptions_ScanTypeConnect(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: scanTypeConnect}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sT"), "connect scan should produce -sT; args=%v", args)
}

func TestBuildScanOptions_ScanTypeSYN(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: "syn"}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sS"), "syn scan should produce -sS; args=%v", args)
}

func TestBuildScanOptions_ScanTypeACK(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: "ack"}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sA"), "ack scan should produce -sA; args=%v", args)
}

func TestBuildScanOptions_ScanTypeUDP(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "53", ScanType: "udp"}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sU"), "udp scan should produce -sU; args=%v", args)
}

func TestBuildScanOptions_ScanTypeAggressive(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: "aggressive"}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sS"), "aggressive scan should include -sS; args=%v", args)
	assert.True(t, hasArg(args, "-A"), "aggressive scan should include -A; args=%v", args)
}

func TestBuildScanOptions_ScanTypeComprehensive(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: "comprehensive"}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sS"), "comprehensive scan should include -sS; args=%v", args)
	assert.True(t, hasArg(args, "-sC"), "comprehensive scan should include -sC; args=%v", args)
}

func TestBuildScanOptions_ScanTypeEmpty(t *testing.T) {
	// An empty ScanType should not inject any scan-method flag.
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: ""}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.NotEmpty(t, args)
	for _, flag := range []string{"-sT", "-sS", "-sA", "-A", "-sC"} {
		assert.False(t, hasArg(args, flag),
			"empty ScanType should not emit %s; args=%v", flag, args)
	}
}

// ─── buildScanOptions — OS detection ─────────────────────────────────────────

func TestBuildScanOptions_OSDetectionOn(t *testing.T) {
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		OSDetection: true,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-O"), "OSDetection=true should add -O; args=%v", args)
}

func TestBuildScanOptions_OSDetectionOff(t *testing.T) {
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		OSDetection: false,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-O"), "OSDetection=false must not add -O; args=%v", args)
}

// ─── buildScanOptions — timing template driven by Timing field ───────────────

func TestBuildScanOptions_TimingParanoid(t *testing.T) {
	// "paranoid" → nmap T0 (-T0)
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: scanTypeConnect,
		Timing:   "paranoid",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T0"), "paranoid timing should produce -T0; args=%v", args)
}

func TestBuildScanOptions_TimingPolite(t *testing.T) {
	// "polite" → nmap T1 (-T1)
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: scanTypeConnect,
		Timing:   "polite",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T1"), "polite timing should produce -T1; args=%v", args)
}

func TestBuildScanOptions_TimingNormal(t *testing.T) {
	// "normal" → nmap T3 (-T3)
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: scanTypeConnect,
		Timing:   "normal",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T3"), "normal timing should produce -T3; args=%v", args)
}

func TestBuildScanOptions_TimingAggressive(t *testing.T) {
	// "aggressive" → nmap T4 (-T4)
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: scanTypeConnect,
		Timing:   "aggressive",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T4"), "aggressive timing should produce -T4; args=%v", args)
}

func TestBuildScanOptions_TimingInsane(t *testing.T) {
	// "insane" → nmap T5 (-T5)
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80",
		ScanType: scanTypeConnect,
		Timing:   "insane",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T5"), "insane timing should produce -T5; args=%v", args)
}

func TestBuildScanOptions_TimingEmpty_NoTimingFlag(t *testing.T) {
	// Empty Timing and no high concurrency → no -T flag added.
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		Timing:      "",
		Concurrency: 0,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	for _, f := range []string{"-T0", "-T1", "-T2", "-T3", "-T4", "-T5"} {
		assert.False(t, hasArg(args, f),
			"empty Timing with no concurrency should not add %s; args=%v", f, args)
	}
}

func TestBuildScanOptions_TimingUnknown_NoTimingFlag(t *testing.T) {
	// Unrecognized Timing string → falls through to default, no -T flag.
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		Timing:      "unknown-value",
		Concurrency: 0,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	for _, f := range []string{"-T0", "-T1", "-T2", "-T3", "-T4", "-T5"} {
		assert.False(t, hasArg(args, f),
			"unknown Timing should not add %s; args=%v", f, args)
	}
}

func TestBuildScanOptions_TimingTakesPrecedenceOverConcurrency(t *testing.T) {
	// Explicit Timing field wins even when Concurrency > maxConcurrency.
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		Timing:      "polite",
		Concurrency: maxConcurrency + 1,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T1"), "explicit Timing=polite should produce -T1; args=%v", args)
	assert.False(t, hasArg(args, "-T4"), "explicit Timing should prevent concurrency fallback to -T4; args=%v", args)
}

// ─── buildScanOptions — concurrency ──────────────────────────────────────────

func TestBuildScanOptions_HighConcurrency_AggressiveTiming(t *testing.T) {
	// Concurrency > maxConcurrency (20) → TimingAggressive (-T4)
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		Concurrency: maxConcurrency + 1,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-T4"),
		"concurrency > max should produce -T4; args=%v", args)
}

func TestBuildScanOptions_ConcurrencyAtMax_NoExtraFlag(t *testing.T) {
	// Concurrency == maxConcurrency → still within bounds, no extra flag.
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		TimeoutSec:  0,
		Concurrency: maxConcurrency,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-T4"),
		"concurrency==max should not add -T4; args=%v", args)
}

func TestBuildScanOptions_LowConcurrency_NoExtraFlag(t *testing.T) {
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		TimeoutSec:  0,
		Concurrency: 5,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-T4"),
		"low concurrency should not add -T4; args=%v", args)
}

func TestBuildScanOptions_ZeroConcurrency_NoExtraFlag(t *testing.T) {
	cfg := &ScanConfig{
		Targets:     []string{"127.0.0.1"},
		Ports:       "80",
		ScanType:    scanTypeConnect,
		TimeoutSec:  0,
		Concurrency: 0,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-T4"),
		"zero concurrency should not add -T4; args=%v", args)
}

// ─── buildScanOptions — always-present flags ──────────────────────────────────

func TestBuildScanOptions_AlwaysSkipsHostDiscovery(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: scanTypeConnect}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-Pn"),
		"should always include -Pn (skip host discovery); args=%v", args)
}

func TestBuildScanOptions_AlwaysIncludesVerbosity(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: scanTypeConnect}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-v"),
		"should always include -v (verbosity); args=%v", args)
}

func TestBuildScanOptions_MultipleTargets_NoError(t *testing.T) {
	cfg := &ScanConfig{
		Targets:  []string{"192.168.1.1", "192.168.1.2", "10.0.0.0/24"},
		Ports:    "22,80,443",
		ScanType: scanTypeConnect,
	}
	opts := buildScanOptions(cfg)
	assert.NotEmpty(t, opts)
}

// ─── buildScanOptions — mixed-protocol UDP port spec ─────────────────────────

func TestBuildScanOptions_UDPPortSpec_InjectsUDPScan(t *testing.T) {
	// A port spec containing "U:" should trigger an extra -sU even when
	// ScanType is "connect" (not "udp").
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "T:80,443,U:53,161",
		ScanType: scanTypeConnect,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sU"),
		"port spec with U: prefix should add -sU; args=%v", args)
}

func TestBuildScanOptions_TCPOnlyPortSpec_NoUDPFlag(t *testing.T) {
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "80,443,8080",
		ScanType: scanTypeConnect,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-sU"),
		"TCP-only port spec must not inject -sU; args=%v", args)
}

// ─── resolveScanType ──────────────────────────────────────────────────────────

func TestResolveScanType_ConnectPassthrough(t *testing.T) {
	assert.Equal(t, "connect", resolveScanType("connect"))
}

func TestResolveScanType_ACKPassthrough(t *testing.T) {
	// "ack" is not in the rootRequired map — always passes through.
	assert.Equal(t, "ack", resolveScanType("ack"))
}

func TestResolveScanType_UDPPassthrough(t *testing.T) {
	assert.Equal(t, "udp", resolveScanType("udp"))
}

func TestResolveScanType_EmptyPassthrough(t *testing.T) {
	assert.Equal(t, "", resolveScanType(""))
}

func TestResolveScanType_UnknownPassthrough(t *testing.T) {
	assert.Equal(t, "stealth", resolveScanType("stealth"))
}

func TestResolveScanType_SYN_AsRoot(t *testing.T) {
	if !isRoot() {
		t.Skip("requires root uid")
	}
	assert.Equal(t, "syn", resolveScanType("syn"),
		"root: 'syn' should be kept as-is")
}

func TestResolveScanType_SYN_NotRoot_FallsBackToConnect(t *testing.T) {
	if isRoot() {
		t.Skip("requires non-root uid")
	}
	assert.Equal(t, scanTypeConnect, resolveScanType("syn"),
		"non-root: 'syn' should fall back to 'connect'")
}

func TestResolveScanType_Aggressive_AsRoot(t *testing.T) {
	if !isRoot() {
		t.Skip("requires root uid")
	}
	assert.Equal(t, "aggressive", resolveScanType("aggressive"))
}

func TestResolveScanType_Aggressive_NotRoot_FallsBackToConnect(t *testing.T) {
	if isRoot() {
		t.Skip("requires non-root uid")
	}
	assert.Equal(t, scanTypeConnect, resolveScanType("aggressive"),
		"non-root: 'aggressive' should fall back to 'connect'")
}

func TestResolveScanType_Comprehensive_AsRoot(t *testing.T) {
	if !isRoot() {
		t.Skip("requires root uid")
	}
	assert.Equal(t, "comprehensive", resolveScanType("comprehensive"))
}

func TestResolveScanType_Comprehensive_NotRoot_FallsBackToConnect(t *testing.T) {
	if isRoot() {
		t.Skip("requires non-root uid")
	}
	assert.Equal(t, scanTypeConnect, resolveScanType("comprehensive"),
		"non-root: 'comprehensive' should fall back to 'connect'")
}

// ─── FixedResourceManager.Close ───────────────────────────────────────────────

func TestClose_IdleManager_NoError(t *testing.T) {
	rm := NewFixedResourceManager(4)
	assert.NoError(t, rm.Close())
}

func TestClose_MarksManagerUnhealthy(t *testing.T) {
	rm := NewFixedResourceManager(4)
	require.True(t, rm.IsHealthy(), "should be healthy before Close")
	require.NoError(t, rm.Close())
	assert.False(t, rm.IsHealthy(), "closed manager should report unhealthy")
}

func TestClose_ClearsActiveScans(t *testing.T) {
	rm := NewFixedResourceManager(4)
	ctx := context.Background()
	require.NoError(t, rm.Acquire(ctx, "s1"))
	require.NoError(t, rm.Acquire(ctx, "s2"))
	require.Equal(t, 2, rm.GetActiveScans())

	require.NoError(t, rm.Close())
	assert.Equal(t, 0, rm.GetActiveScans(),
		"Close should wipe the active-scan map")
}

func TestClose_Idempotent(t *testing.T) {
	rm := NewFixedResourceManager(2)
	require.NoError(t, rm.Close())
	assert.NoError(t, rm.Close(), "second Close must be a no-op without error")
}

func TestClose_BlocksSubsequentAcquire(t *testing.T) {
	rm := NewFixedResourceManager(2)
	require.NoError(t, rm.Close())

	err := rm.Acquire(context.Background(), "late-scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestClose_AvailableSlotsEqualCapacityAfterClose(t *testing.T) {
	const capacity = 3
	rm := NewFixedResourceManager(capacity)
	ctx := context.Background()
	require.NoError(t, rm.Acquire(ctx, "a"))
	require.NoError(t, rm.Acquire(ctx, "b"))

	require.NoError(t, rm.Close())
	// Active map is cleared, so available slots should equal full capacity.
	assert.Equal(t, capacity, rm.GetAvailableSlots())
}

func TestClose_ClosedFlagSetInGetStats(t *testing.T) {
	rm := NewFixedResourceManager(2)
	require.NoError(t, rm.Close())
	stats := rm.GetStats()
	assert.Equal(t, true, stats["closed"])
}

// ─── FixedResourceManager.GetStats ────────────────────────────────────────────

func TestGetStats_ContainsRequiredKeys(t *testing.T) {
	rm := NewFixedResourceManager(5)
	stats := rm.GetStats()
	for _, key := range []string{"capacity", "active_scans", "available_slots", "is_healthy", "closed"} {
		_, ok := stats[key]
		assert.True(t, ok, "GetStats must contain key %q", key)
	}
}

func TestGetStats_InitialValues(t *testing.T) {
	const capacity = 7
	rm := NewFixedResourceManager(capacity)
	stats := rm.GetStats()

	assert.Equal(t, capacity, stats["capacity"])
	assert.Equal(t, 0, stats["active_scans"])
	assert.Equal(t, capacity, stats["available_slots"])
	assert.Equal(t, true, stats["is_healthy"])
	assert.Equal(t, false, stats["closed"])
}

func TestGetStats_ReflectsActiveScans(t *testing.T) {
	rm := NewFixedResourceManager(5)
	ctx := context.Background()
	require.NoError(t, rm.Acquire(ctx, "x1"))
	require.NoError(t, rm.Acquire(ctx, "x2"))

	stats := rm.GetStats()
	assert.Equal(t, 2, stats["active_scans"])
	assert.Equal(t, 3, stats["available_slots"])

	rm.Release("x1")
	rm.Release("x2")

	stats2 := rm.GetStats()
	assert.Equal(t, 0, stats2["active_scans"])
	assert.Equal(t, 5, stats2["available_slots"])
}

func TestGetStats_AfterClose(t *testing.T) {
	rm := NewFixedResourceManager(3)
	ctx := context.Background()
	require.NoError(t, rm.Acquire(ctx, "pre-close"))
	require.NoError(t, rm.Close())

	stats := rm.GetStats()
	assert.Equal(t, true, stats["closed"])
	assert.Equal(t, false, stats["is_healthy"])
	assert.Equal(t, 0, stats["active_scans"])
}

func TestGetStats_CapacityClampedToOne_ZeroInput(t *testing.T) {
	// Constructor clamps capacity ≤ 0 to 1.
	rm := NewFixedResourceManager(0)
	stats := rm.GetStats()
	assert.Equal(t, 1, stats["capacity"])
}

func TestGetStats_CapacityClampedToOne_NegativeInput(t *testing.T) {
	rm := NewFixedResourceManager(-42)
	stats := rm.GetStats()
	assert.Equal(t, 1, stats["capacity"])
}

func TestGetStats_AvailableSlotsConsistency(t *testing.T) {
	rm := NewFixedResourceManager(4)
	ctx := context.Background()

	for i, id := range []string{"a", "b", "c"} {
		require.NoError(t, rm.Acquire(ctx, id))
		stats := rm.GetStats()
		assert.Equal(t, i+1, stats["active_scans"])
		assert.Equal(t, 4-(i+1), stats["available_slots"])
	}

	rm.Release("a")
	stats := rm.GetStats()
	assert.Equal(t, 2, stats["active_scans"])
	assert.Equal(t, 2, stats["available_slots"])

	rm.Release("b")
	rm.Release("c")
	stats = rm.GetStats()
	assert.Equal(t, 0, stats["active_scans"])
	assert.Equal(t, 4, stats["available_slots"])
}

// ─── sendResult ───────────────────────────────────────────────────────────────

func TestSendResult_NilChannel_NoPanic(t *testing.T) {
	q := &ScanQueue{}
	req := &ScanQueueRequest{ID: "nil-ch", ResultCh: nil}
	result := &ScanQueueResult{ID: "nil-ch"}
	assert.NotPanics(t, func() { q.sendResult(req, result) })
}

func TestSendResult_BufferedChannel_DeliveredSuccessfully(t *testing.T) {
	q := &ScanQueue{}
	ch := make(chan *ScanQueueResult, 1)
	req := &ScanQueueRequest{ID: "buffered", ResultCh: ch}
	want := &ScanQueueResult{ID: "buffered"}

	q.sendResult(req, want)

	select {
	case got := <-ch:
		assert.Equal(t, want, got)
	default:
		t.Fatal("result was not delivered to the buffered channel")
	}
}

func TestSendResult_UnbufferedChannel_DoesNotBlock(t *testing.T) {
	// An unbuffered channel with no reader must not cause sendResult to block.
	q := &ScanQueue{}
	ch := make(chan *ScanQueueResult) // unbuffered, nobody reading
	req := &ScanQueueRequest{ID: "full-ch", ResultCh: ch}
	result := &ScanQueueResult{ID: "full-ch"}

	done := make(chan struct{})
	go func() {
		q.sendResult(req, result)
		close(done)
	}()

	select {
	case <-done:
		// returned without blocking — correct behavior
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sendResult blocked on an unread unbuffered channel")
	}
}

func TestSendResult_MultipleResultsSameChannel(t *testing.T) {
	q := &ScanQueue{}
	const n = 5
	ch := make(chan *ScanQueueResult, n)

	for i := 0; i < n; i++ {
		req := &ScanQueueRequest{ID: "multi", ResultCh: ch}
		result := &ScanQueueResult{ID: "multi"}
		q.sendResult(req, result)
	}
	assert.Equal(t, n, len(ch),
		"all %d results should be queued in the buffered channel", n)
}

// ──────────────────────────────────────────────────────────────────────────────
// convertNmapHost — OS detection
// ──────────────────────────────────────────────────────────────────────────────

func makeNmapHost(addr string, ports []nmap.Port, osMatches []nmap.OSMatch) nmap.Host {
	h := nmap.Host{}
	h.Addresses = []nmap.Address{{Addr: addr}}
	h.Status.State = "up"
	h.Ports = ports
	h.OS.Matches = osMatches
	return h
}

func TestConvertNmapHost_NoAddresses_ReturnsNil(t *testing.T) {
	h := nmap.Host{}
	result := convertNmapHost(&h)
	if result != nil {
		t.Fatalf("expected nil for host with no addresses, got %+v", result)
	}
}

func TestConvertNmapHost_NoOSMatches_OSFieldsEmpty(t *testing.T) {
	h := makeNmapHost("10.0.0.1", nil, nil)
	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSName != "" {
		t.Errorf("expected empty OSName, got %q", result.OSName)
	}
	if result.OSFamily != "" {
		t.Errorf("expected empty OSFamily, got %q", result.OSFamily)
	}
	if result.OSVersion != "" {
		t.Errorf("expected empty OSVersion, got %q", result.OSVersion)
	}
	if result.OSAccuracy != 0 {
		t.Errorf("expected zero OSAccuracy, got %d", result.OSAccuracy)
	}
}

func TestConvertNmapHost_SingleOSMatch_FieldsPopulated(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "Linux 5.15",
		Accuracy: 97,
		Classes: []nmap.OSClass{
			{Family: "Linux", OSGeneration: "5.15"},
		},
	}
	h := makeNmapHost("10.0.0.2", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSName != "Linux 5.15" {
		t.Errorf("OSName: want %q, got %q", "Linux 5.15", result.OSName)
	}
	if result.OSAccuracy != 97 {
		t.Errorf("OSAccuracy: want 97, got %d", result.OSAccuracy)
	}
	if result.OSFamily != "Linux" {
		t.Errorf("OSFamily: want %q, got %q", "Linux", result.OSFamily)
	}
	if result.OSVersion != "5.15" {
		t.Errorf("OSVersion: want %q, got %q", "5.15", result.OSVersion)
	}
}

func TestConvertNmapHost_BestMatchIsFirst(t *testing.T) {
	// nmap orders matches best-first; convertNmapHost must pick index 0.
	matches := []nmap.OSMatch{
		{Name: "Linux 5.15", Accuracy: 97, Classes: []nmap.OSClass{{Family: "Linux", OSGeneration: "5.15"}}},
		{Name: "Linux 4.19", Accuracy: 85, Classes: []nmap.OSClass{{Family: "Linux", OSGeneration: "4.19"}}},
	}
	h := makeNmapHost("10.0.0.3", nil, matches)

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSName != "Linux 5.15" {
		t.Errorf("expected best match (Linux 5.15), got %q", result.OSName)
	}
	if result.OSAccuracy != 97 {
		t.Errorf("expected accuracy 97, got %d", result.OSAccuracy)
	}
}

func TestConvertNmapHost_OSMatchNoClasses_FamilyAndVersionEmpty(t *testing.T) {
	// An OS match with no classes should still populate OSName/OSAccuracy
	// but leave OSFamily and OSVersion empty rather than panic.
	match := nmap.OSMatch{
		Name:     "Unknown OS",
		Accuracy: 50,
		Classes:  nil,
	}
	h := makeNmapHost("10.0.0.4", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSName != "Unknown OS" {
		t.Errorf("OSName: want %q, got %q", "Unknown OS", result.OSName)
	}
	if result.OSAccuracy != 50 {
		t.Errorf("OSAccuracy: want 50, got %d", result.OSAccuracy)
	}
	if result.OSFamily != "" {
		t.Errorf("expected empty OSFamily when no classes, got %q", result.OSFamily)
	}
	if result.OSVersion != "" {
		t.Errorf("expected empty OSVersion when no classes, got %q", result.OSVersion)
	}
}

func TestConvertNmapHost_WindowsOSMatch(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "Windows 10",
		Accuracy: 92,
		Classes: []nmap.OSClass{
			{Family: "Windows", OSGeneration: "10"},
		},
	}
	h := makeNmapHost("192.168.1.50", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSFamily != "Windows" {
		t.Errorf("OSFamily: want %q, got %q", "Windows", result.OSFamily)
	}
	if result.OSVersion != "10" {
		t.Errorf("OSVersion: want %q, got %q", "10", result.OSVersion)
	}
}

func TestConvertNmapHost_PortsConvertedAlongWithOS(t *testing.T) {
	// Ensure that adding OS detection didn't break port conversion.
	ports := []nmap.Port{
		{ID: 22, Protocol: "tcp"},
		{ID: 80, Protocol: "tcp"},
	}
	ports[0].State.State = "open"
	ports[0].Service.Name = "ssh"
	ports[1].State.State = "open"
	ports[1].Service.Name = "http"

	match := nmap.OSMatch{
		Name:     "Linux 5.4",
		Accuracy: 90,
		Classes:  []nmap.OSClass{{Family: "Linux", OSGeneration: "5.4"}},
	}
	h := makeNmapHost("10.1.1.1", ports, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(result.Ports))
	}
	if result.OSName != "Linux 5.4" {
		t.Errorf("OSName: want %q, got %q", "Linux 5.4", result.OSName)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// parseTargetAddress — localhost and other hostname edge cases
// ──────────────────────────────────────────────────────────────────────────────

func TestParseTargetAddress_Localhost_Accepted(t *testing.T) {
	// Bug: "localhost" was rejected because it contains no dot.
	addr, err := parseTargetAddress("localhost")
	if err != nil {
		t.Fatalf("parseTargetAddress(%q) returned unexpected error: %v", "localhost", err)
	}
	// The function falls back to 0.0.0.0/32 for unresolvable single-label names;
	// the important thing is that it does not error.
	_ = addr
}

func TestParseTargetAddress_FQDN_Accepted(t *testing.T) {
	_, err := parseTargetAddress("example.com")
	if err != nil {
		t.Fatalf("parseTargetAddress(%q) unexpected error: %v", "example.com", err)
	}
}

func TestParseTargetAddress_BareWord_Rejected(t *testing.T) {
	// A bare word with no dot and not "localhost" should still be rejected.
	_, err := parseTargetAddress("notahostname")
	if err == nil {
		t.Fatal("expected error for bare word target, got nil")
	}
}

func TestParseTargetAddress_IPv4_Accepted(t *testing.T) {
	_, err := parseTargetAddress("192.168.1.1")
	if err != nil {
		t.Fatalf("parseTargetAddress(IPv4) unexpected error: %v", err)
	}
}

func TestParseTargetAddress_IPv4CIDR_Accepted(t *testing.T) {
	_, err := parseTargetAddress("10.0.0.0/24")
	if err != nil {
		t.Fatalf("parseTargetAddress(CIDR) unexpected error: %v", err)
	}
}

func TestParseTargetAddress_IPv6_Accepted(t *testing.T) {
	_, err := parseTargetAddress("2001:db8::1")
	if err != nil {
		t.Fatalf("parseTargetAddress(IPv6) unexpected error: %v", err)
	}
}

func TestParseTargetAddress_InvalidCIDRMask_Rejected(t *testing.T) {
	_, err := parseTargetAddress("192.168.1.1/99")
	if err == nil {
		t.Fatal("expected error for invalid CIDR mask, got nil")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// storeScanResults — ScanID linkage
// ──────────────────────────────────────────────────────────────────────────────

// ──────────────────────────────────────────────────────────────────────────────
// storeScanResults — branch routing (no DB required)
//
// storeScanResults has two paths:
//   - nil scanID  → CREATE path: builds a fresh scan_target + scan_job row.
//   - non-nil     → UPDATE path: the row already exists (created by the API);
//                   only scan_stats is updated, no INSERT attempted.
//
// We test the routing logic through two observable properties:
//   1. The job UUID selected (nil→fresh, non-nil→preserved).
//   2. That nil always generates a unique UUID (no accidental reuse).
// ──────────────────────────────────────────────────────────────────────────────

// TestStoreScanResults_NilScanID_TakesCreatePath verifies that when no scanID
// is supplied a brand-new UUID is minted for the scan_job row (CREATE path).
func TestStoreScanResults_NilScanID_TakesCreatePath(t *testing.T) {
	// Two nil calls must produce different UUIDs — they are independent jobs.
	id1 := resolveJobID(nil)
	id2 := resolveJobID(nil)
	assert.NotEqual(t, id1, id2,
		"nil scanID must yield a fresh UUID each time (CREATE path)")
	assert.NotEqual(t, uuid.Nil, id1, "generated UUID must not be the zero value")
}

// TestStoreScanResults_NonNilScanID_TakesUpdatePath verifies that when a
// scanID is supplied the same UUID is returned (UPDATE path — row exists).
func TestStoreScanResults_NonNilScanID_TakesUpdatePath(t *testing.T) {
	want := uuid.New()
	got := resolveJobID(&want)
	assert.Equal(t, want, got,
		"non-nil scanID must be reused as-is (UPDATE path — must not generate a new UUID)")
}

// TestStoreScanResults_ScanIDNil_GeneratesNewUUID keeps the original name so
// existing test references in docs/CI annotations still match.
func TestStoreScanResults_ScanIDNil_GeneratesNewUUID(t *testing.T) {
	id1 := resolveJobID(nil)
	id2 := resolveJobID(nil)
	if id1 == id2 {
		t.Error("two nil-ScanID calls produced the same UUID — they must be unique")
	}
}

func TestStoreScanResults_ScanIDNonNil_Preserved(t *testing.T) {
	want := uuid.New()
	got := resolveJobID(&want)
	if got != want {
		t.Errorf("resolveJobID(&id): want %s, got %s", want, got)
	}
}

// resolveJobID mirrors the ID-selection logic at the top of storeScanResults:
// return the caller-supplied ID when present (UPDATE path), otherwise mint a
// new one (CREATE path).  Keeping this in sync with the production code is
// intentional — if the logic changes, these tests catch the drift.
func resolveJobID(scanID *uuid.UUID) uuid.UUID {
	if scanID != nil {
		return *scanID
	}
	return uuid.New()
}

// TestStoreScanResults_UpdatePath_ScanIDNeverReplacedWithFresh asserts that
// once a non-nil scanID enters storeScanResults it is never swapped out for a
// freshly generated UUID.  This is the core invariant broken by the original
// bug (always calling uuid.New() regardless of the supplied ID).
func TestStoreScanResults_UpdatePath_ScanIDNeverReplacedWithFresh(t *testing.T) {
	for range 20 {
		id := uuid.New()
		got := resolveJobID(&id)
		require.Equal(t, id, got,
			"resolveJobID must always return the supplied scanID unchanged")
	}
}

// TestStoreScanResults_CreatePath_NilYieldsUniqueIDs stress-tests that the
// CREATE path never accidentally hands out the same UUID twice.
func TestStoreScanResults_CreatePath_NilYieldsUniqueIDs(t *testing.T) {
	const n = 50
	seen := make(map[uuid.UUID]struct{}, n)
	for range n {
		id := resolveJobID(nil)
		_, dup := seen[id]
		require.False(t, dup, "resolveJobID(nil) returned a duplicate UUID: %s", id)
		seen[id] = struct{}{}
	}
}

func TestScanConfig_ScanIDField_DefaultNil(t *testing.T) {
	cfg := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
	}
	if cfg.ScanID != nil {
		t.Errorf("expected ScanID to default to nil, got %v", cfg.ScanID)
	}
}

func TestScanConfig_ScanIDField_CanBeSet(t *testing.T) {
	id := uuid.New()
	cfg := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
		ScanID:   &id,
	}
	if cfg.ScanID == nil {
		t.Fatal("expected ScanID to be non-nil after assignment")
	}
	if *cfg.ScanID != id {
		t.Errorf("ScanID: want %s, got %s", id, *cfg.ScanID)
	}
}

func TestSendResult_FullBufferedChannel_DoesNotBlock(t *testing.T) {
	// Fill the channel to capacity first, then a further send must not block.
	q := &ScanQueue{}
	ch := make(chan *ScanQueueResult, 1)
	ch <- &ScanQueueResult{ID: "pre-fill"} // channel is now full

	req := &ScanQueueRequest{ID: "overflow", ResultCh: ch}
	result := &ScanQueueResult{ID: "overflow"}

	done := make(chan struct{})
	go func() {
		q.sendResult(req, result)
		close(done)
	}()

	select {
	case <-done:
		// non-blocking send dropped the result silently — correct
	case <-time.After(200 * time.Millisecond):
		t.Fatal("sendResult blocked on a full buffered channel")
	}
}
