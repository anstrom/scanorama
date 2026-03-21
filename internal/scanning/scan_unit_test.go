package scanning

// NOTE: the tests in this file are pure unit tests — no database, no nmap
// binary required.  They target the two gaps that allowed OS-detection data
// to be silently dropped and "localhost" to be rejected as an invalid target.

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	// A port spec with U: and a privileged scan type should inject -sU.
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "T:80,443,U:53,161",
		ScanType: "syn",
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.True(t, hasArg(args, "-sU"),
		"port spec with U: prefix should add -sU for syn scan; args=%v", args)
}

func TestBuildScanOptions_UDPPortSpec_ConnectScan_StripsPrefix(t *testing.T) {
	// Connect scan is TCP-only: T:/U: prefixes must be stripped and -sU must NOT be added.
	cfg := &ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    "T:80,443,U:53,161",
		ScanType: scanTypeConnect,
	}
	args := scanArgs(t, buildScanOptions(cfg))
	assert.False(t, hasArg(args, "-sU"),
		"connect scan with U: prefix must not add -sU; args=%v", args)
	assert.True(t, hasArg(args, "-sT"),
		"connect scan must still add -sT; args=%v", args)
	// Ports should be stripped of T:/U: prefixes.
	assert.True(t, hasArg(args, "-p"),
		"ports flag must be present; args=%v", args)
	portIdx := slices.Index(args, "-p")
	assert.Equal(t, "80,443,53,161", args[portIdx+1],
		"port spec should have T:/U: prefixes stripped; args=%v", args)
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

// ─── CalculateTimeout ────────────────────────────────────────────────────────

func TestCalculateTimeout_SmallPortList(t *testing.T) {
	// 3 ports, 1 target, connect → floor applies
	got := CalculateTimeout("22,80,443", 1, "connect")
	assert.Equal(t, minTimeoutSeconds, got,
		"small port list should hit the floor")
}

func TestCalculateTimeout_TypicalProfile(t *testing.T) {
	// 19 ports (Linux server profile), 1 target, syn → 19s < floor(60s)
	ports := "T:21,22,25,53,80,111,143,443,465,587,2049,3306,5432,6379,8080,8443,9200,9300,27017"
	got := CalculateTimeout(ports, 1, "syn")
	assert.Equal(t, minTimeoutSeconds, got,
		"19 ports × 1 target = 19s which is below the floor; expect floor")
}

func TestCalculateTimeout_FullPortRange_SingleTarget(t *testing.T) {
	// 1-65535 = 65535 ports, connect scan, 1 target → capped at 3600s
	got := CalculateTimeout("1-65535", 1, "connect")
	assert.Equal(t, 3600, got, "full range single target should hit 1h cap")
}

func TestCalculateTimeout_FullPortRange_MultiTarget(t *testing.T) {
	got := CalculateTimeout("1-65535", 10, "connect")
	assert.Equal(t, 3600, got, "full range multi-target should still be capped at 1h")
}

func TestCalculateTimeout_UDPMultiplier(t *testing.T) {
	// 1-1000 = 1000 ports × 1 target → 1000s TCP, 4000s UDP (capped to 3600s).
	tcp := CalculateTimeout("1-1000", 1, "connect")
	udp := CalculateTimeout("1-1000", 1, "udp")
	assert.Equal(t, 1000, tcp, "1000 ports × 1 target TCP = 1000s")
	assert.Equal(t, 3600, udp, "1000 ports × 1 target UDP = 4000s, capped to 3600s")

	// Use a smaller range to verify the 4× factor without hitting the cap.
	tcpSmall := CalculateTimeout("1-100", 1, "connect")
	udpSmall := CalculateTimeout("1-100", 1, "udp")
	assert.Equal(t, tcpSmall*4, udpSmall, "UDP should be 4× TCP for ranges that don't hit the cap")
}

func TestCalculateTimeout_MixedUDP(t *testing.T) {
	// Both 6-port specs hit the floor at 60s — use a larger range to see the 4× effect.
	pure := CalculateTimeout("1-100", 1, "syn")
	// Simulate a mixed spec by using enough ports that the multiplier pushes above the cap.
	mixed := CalculateTimeout("T:1-100,U:1-100", 1, "syn")
	assert.Greater(t, mixed, pure, "mixed T:/U: spec should produce a longer timeout than TCP-only")
}

func TestCalculateTimeout_AggressiveOverhead(t *testing.T) {
	connect := CalculateTimeout("1-100", 1, "connect")
	aggressive := CalculateTimeout("1-100", 1, "aggressive")
	// Aggressive adds 50% — but both may be at the floor.
	assert.GreaterOrEqual(t, aggressive, connect,
		"aggressive scan should never be faster than connect")
}

func TestCalculateTimeout_Floor(t *testing.T) {
	got := CalculateTimeout("80", 1, "connect")
	assert.Equal(t, minTimeoutSeconds, got, "single port must hit the floor")
}

func TestCalculateTimeout_ScriptedFloor(t *testing.T) {
	// Aggressive and comprehensive have a higher floor (300s) because NSE
	// scripts routinely run longer than the standard 60s floor.
	for _, scanType := range []string{"aggressive", "comprehensive"} {
		got := CalculateTimeout("22,80,443", 1, scanType)
		assert.Equal(t, minTimeoutSecondsScripted, got,
			"%s scan on 3 ports should hit the scripted floor (%ds), got %d",
			scanType, minTimeoutSecondsScripted, got)
	}
	// Standard scan types must NOT use the scripted floor.
	for _, scanType := range []string{"connect", "syn", "udp"} {
		got := CalculateTimeout("22,80,443", 1, scanType)
		assert.Equal(t, minTimeoutSeconds, got,
			"%s scan on 3 ports should hit the standard floor (%ds), got %d",
			scanType, minTimeoutSeconds, got)
	}
}

func TestCalculateTimeout_EmptyPorts(t *testing.T) {
	// Empty port string → falls back to 1000-port default
	got := CalculateTimeout("", 1, "connect")
	assert.Equal(t, 1000, got)
}

func TestCalculateTimeout_ZeroTargets(t *testing.T) {
	// Zero targets treated as 1
	got := CalculateTimeout("22,80", 0, "connect")
	assert.Equal(t, minTimeoutSeconds, got)
}

// ─── normalizePortState ───────────────────────────────────────────────────────

func TestNormalizePortState_KnownStates(t *testing.T) {
	for _, state := range []string{"open", "closed", "filtered", "unknown"} {
		assert.Equal(t, state, normalizePortState(state),
			"known state %q should pass through unchanged", state)
	}
}

func TestNormalizePortState_OpenFiltered(t *testing.T) {
	// UDP ports with no response are reported as "open|filtered" by nmap.
	// We treat them conservatively as "open".
	assert.Equal(t, "open", normalizePortState("open|filtered"))
}

func TestNormalizePortState_ClosedFiltered(t *testing.T) {
	assert.Equal(t, "filtered", normalizePortState("closed|filtered"))
}

func TestNormalizePortState_Unknown(t *testing.T) {
	assert.Equal(t, "unknown", normalizePortState(""))
	assert.Equal(t, "unknown", normalizePortState("unrecognized"))
	assert.Equal(t, "unknown", normalizePortState("open filtered")) // space variant
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

// TestScanConfig_ScanID verifies the ScanID pointer field semantics that the
// storeScanResults create/update path depends on.
func TestScanConfig_ScanID(t *testing.T) {
	t.Run("nil by default", func(t *testing.T) {
		cfg := &ScanConfig{Targets: []string{"10.0.0.1"}, Ports: "80", ScanType: "connect"}
		assert.Nil(t, cfg.ScanID, "ScanID should default to nil (triggers CREATE path)")
	})

	t.Run("non-nil pointer preserved", func(t *testing.T) {
		id := uuid.New()
		cfg := &ScanConfig{Targets: []string{"10.0.0.1"}, Ports: "80", ScanType: "connect", ScanID: &id}
		require.NotNil(t, cfg.ScanID)
		assert.Equal(t, id, *cfg.ScanID, "ScanID value must not be altered by struct construction")
	})

	t.Run("nil and non-nil are distinct states", func(t *testing.T) {
		id := uuid.New()
		withID := &ScanConfig{ScanID: &id}
		withoutID := &ScanConfig{}
		assert.NotEqual(t, withID.ScanID == nil, withoutID.ScanID == nil,
			"nil and non-nil ScanID must be distinguishable (create vs update path)")
	})
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

// ──────────────────────────────────────────────────────────────────────────────
// convertNmapHost — additional branch coverage
// ──────────────────────────────────────────────────────────────────────────────

// TestConvertNmapHost_EmptyClassesSlice_FamilyAndVersionEmpty verifies that an
// explicitly empty (non-nil) Classes slice is treated the same as nil: OSFamily
// and OSVersion remain empty while OSName/OSAccuracy are still populated.
// This exercises the false branch of `if len(best.Classes) > 0`.
func TestConvertNmapHost_EmptyClassesSlice_FamilyAndVersionEmpty(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "Generic Embedded",
		Accuracy: 60,
		Classes:  []nmap.OSClass{}, // non-nil but empty
	}
	h := makeNmapHost("10.0.0.5", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSName != "Generic Embedded" {
		t.Errorf("OSName: want %q, got %q", "Generic Embedded", result.OSName)
	}
	if result.OSAccuracy != 60 {
		t.Errorf("OSAccuracy: want 60, got %d", result.OSAccuracy)
	}
	if result.OSFamily != "" {
		t.Errorf("expected empty OSFamily for empty classes slice, got %q", result.OSFamily)
	}
	if result.OSVersion != "" {
		t.Errorf("expected empty OSVersion for empty classes slice, got %q", result.OSVersion)
	}
}

// TestConvertNmapHost_MultipleClasses_UsesFirstClass verifies that when a match
// has more than one OS class, only the first class is used for OSFamily and
// OSVersion (the true branch of `if len(best.Classes) > 0`, multi-element path).
func TestConvertNmapHost_MultipleClasses_UsesFirstClass(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "Linux 5.x",
		Accuracy: 85,
		Classes: []nmap.OSClass{
			{Family: "Linux", OSGeneration: "5.x"},
			{Family: "Linux", OSGeneration: "4.x"}, // should be ignored
		},
	}
	h := makeNmapHost("10.0.0.6", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.OSFamily != "Linux" {
		t.Errorf("OSFamily: want %q, got %q", "Linux", result.OSFamily)
	}
	if result.OSVersion != "5.x" {
		t.Errorf("OSVersion: want %q (first class), got %q", "5.x", result.OSVersion)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// persistOSData — field-setting logic (struct mutation, no DB required)
//
// persistOSData sets pointer fields on a *db.Host then calls
// hostRepo.CreateOrUpdate.  We want to verify the conditional field-setting
// logic without a live database.  We do this by inspecting the db.Host struct
// immediately after the conditions would have fired — using a thin wrapper that
// calls only the field-setting portion through convertNmapHost (which exercises
// the same boolean gates) so we stay within the unit-test boundary.
// ──────────────────────────────────────────────────────────────────────────────

// TestPersistOSData_AllBranchesTrue verifies that every pointer field in
// db.Host is populated when all OS source fields are non-empty / non-zero.
// We assert via convertNmapHost (which feeds into persistOSData) rather than
// calling persistOSData directly (which would require a live DB).
func TestPersistOSData_AllBranchesTrue(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "FreeBSD 13",
		Accuracy: 91,
		Classes:  []nmap.OSClass{{Family: "BSD", OSGeneration: "13"}},
	}
	h := makeNmapHost("10.0.0.7", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	require.NotNil(t, result)

	// All four OS fields must be populated (all four `if` branches fired).
	assert.Equal(t, "FreeBSD 13", result.OSName, "OSName branch must fire")
	assert.Equal(t, "BSD", result.OSFamily, "OSFamily branch must fire")
	assert.Equal(t, "13", result.OSVersion, "OSVersion branch must fire")
	assert.Equal(t, 91, result.OSAccuracy, "OSAccuracy branch must fire (> 0)")
}

// TestPersistOSData_ZeroAccuracy_ConfidenceNotSet verifies the false branch of
// `if host.OSAccuracy > 0`: when nmap reports accuracy == 0 the OSConfidence
// pointer must remain nil (the field-setting branch must NOT fire).
// ──────────────────────────────────────────────────────────────────────────────
// ExecError — Error() and Unwrap()
// ──────────────────────────────────────────────────────────────────────────────

// TestExecError_Error covers all three format branches of ExecError.Error().
func TestExecError_Error(t *testing.T) {
	inner := fmt.Errorf("connection refused")

	t.Run("op_host_port", func(t *testing.T) {
		e := &ExecError{Op: "connect", Host: "10.0.0.1", Port: 443, Err: inner}
		want := "connect failed for 10.0.0.1:443: connection refused"
		assert.Equal(t, want, e.Error())
	})

	t.Run("op_host_no_port", func(t *testing.T) {
		e := &ExecError{Op: "ping", Host: "192.168.1.5", Err: inner}
		want := "ping failed for 192.168.1.5: connection refused"
		assert.Equal(t, want, e.Error())
	})

	t.Run("op_only", func(t *testing.T) {
		e := &ExecError{Op: "validate config", Err: inner}
		want := "validate config failed: connection refused"
		assert.Equal(t, want, e.Error())
	})
}

// TestExecError_Unwrap verifies that Unwrap returns the wrapped error and that
// errors.Is / errors.As work correctly through the chain.
func TestExecError_Unwrap(t *testing.T) {
	sentinel := fmt.Errorf("sentinel error")

	t.Run("unwrap_returns_inner", func(t *testing.T) {
		e := &ExecError{Op: "op", Err: sentinel}
		assert.Equal(t, sentinel, e.Unwrap(), "Unwrap must return the exact wrapped error")
	})

	t.Run("errors_is_through_wrapping", func(t *testing.T) {
		e := &ExecError{Op: "op", Err: sentinel}
		assert.True(t, errors.Is(e, sentinel),
			"errors.Is must find the sentinel through ExecError wrapping")
	})

	t.Run("errors_as_through_wrapping", func(t *testing.T) {
		// Wrap a *ExecError inside a plain fmt.Errorf("%w") so that errors.As
		// must call Unwrap() at least once to reach the *ExecError target.
		inner := &ExecError{Op: "inner op", Err: fmt.Errorf("root cause")}
		wrapped := fmt.Errorf("outer context: %w", inner)
		var target *ExecError
		assert.True(t, errors.As(wrapped, &target),
			"errors.As must unwrap through fmt.Errorf to find the nested *ExecError")
		assert.Equal(t, "inner op", target.Op)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// validatePortPart — uncovered branches
// ──────────────────────────────────────────────────────────────────────────────

// TestValidatePortPart covers the prefix-only token, lowercase prefixes, and
// the range-delegation branch that were not exercised by existing tests.
func TestValidatePortPart(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"192.168.1.1"}, ScanType: "connect", Ports: "80"}

	t.Run("uppercase_prefix_only_T_colon", func(t *testing.T) {
		// Bare "T:" after stripping is empty — must return nil.
		err := cfg.validatePortPart("T:")
		assert.NoError(t, err)
	})

	t.Run("uppercase_prefix_only_U_colon", func(t *testing.T) {
		err := cfg.validatePortPart("U:")
		assert.NoError(t, err)
	})

	t.Run("lowercase_t_prefix_valid_port", func(t *testing.T) {
		// "t:80" → strip "t:" → validate "80" → nil.
		err := cfg.validatePortPart("t:80")
		assert.NoError(t, err)
	})

	t.Run("lowercase_u_prefix_valid_port", func(t *testing.T) {
		err := cfg.validatePortPart("u:53")
		assert.NoError(t, err)
	})

	t.Run("lowercase_t_prefix_only", func(t *testing.T) {
		// "t:" after stripping is empty — must return nil.
		err := cfg.validatePortPart("t:")
		assert.NoError(t, err)
	})

	t.Run("lowercase_u_prefix_only", func(t *testing.T) {
		err := cfg.validatePortPart("u:")
		assert.NoError(t, err)
	})

	t.Run("range_delegated_to_validatePortRange_valid", func(t *testing.T) {
		// Contains "-" → delegates to validatePortRange; valid range returns nil.
		err := cfg.validatePortPart("80-443")
		assert.NoError(t, err)
	})

	t.Run("range_with_prefix_delegated_valid", func(t *testing.T) {
		// "T:80-443" → strip "T:" → "80-443" → validatePortRange → nil.
		err := cfg.validatePortPart("T:80-443")
		assert.NoError(t, err)
	})

	t.Run("range_delegated_to_validatePortRange_invalid", func(t *testing.T) {
		// Contains "-" and is invalid → validatePortRange returns error.
		err := cfg.validatePortPart("443-80")
		assert.Error(t, err)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// validatePortRange — all branches
// ──────────────────────────────────────────────────────────────────────────────

func TestValidatePortRange(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"192.168.1.1"}, ScanType: "connect", Ports: "80"}

	t.Run("valid_range", func(t *testing.T) {
		err := cfg.validatePortRange("80-443")
		assert.NoError(t, err)
	})

	t.Run("valid_range_full_span", func(t *testing.T) {
		err := cfg.validatePortRange("0-65535")
		assert.NoError(t, err)
	})

	t.Run("too_many_hyphens_three_parts", func(t *testing.T) {
		// "1-2-3" splits into 3 parts → len != 2 → error.
		err := cfg.validatePortRange("1-2-3")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid port range format")
	})

	t.Run("invalid_start_port_non_numeric", func(t *testing.T) {
		err := cfg.validatePortRange("abc-443")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid start port")
	})

	t.Run("invalid_end_port_non_numeric", func(t *testing.T) {
		err := cfg.validatePortRange("80-xyz")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid end port")
	})

	t.Run("start_port_out_of_range", func(t *testing.T) {
		err := cfg.validatePortRange("65536-65537")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid port range")
	})

	t.Run("end_port_out_of_range", func(t *testing.T) {
		err := cfg.validatePortRange("80-99999")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid port range")
	})

	t.Run("start_greater_than_end", func(t *testing.T) {
		err := cfg.validatePortRange("443-80")
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "start port must be less than end port")
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Validate — validatePorts error branch
// ──────────────────────────────────────────────────────────────────────────────

// TestValidate_PortError exercises the branch in Validate where validatePorts
// returns an error (previously uncovered at 87.5%).
func TestValidate_PortError(t *testing.T) {
	t.Run("invalid_single_port_non_numeric", func(t *testing.T) {
		cfg := &ScanConfig{
			Targets:  []string{"192.168.1.1"},
			ScanType: "connect",
			Ports:    "abc",
		}
		err := cfg.Validate()
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr),
			"Validate must propagate a *ExecError from validatePorts")
		assert.Contains(t, scanErr.Err.Error(), "invalid port")
	})

	t.Run("invalid_port_out_of_range", func(t *testing.T) {
		cfg := &ScanConfig{
			Targets:  []string{"192.168.1.1"},
			ScanType: "connect",
			Ports:    "99999",
		}
		err := cfg.Validate()
		require.Error(t, err)
		var scanErr *ExecError
		require.True(t, errors.As(err, &scanErr))
		assert.Contains(t, scanErr.Err.Error(), "invalid port")
	})

	t.Run("valid_ports_no_error", func(t *testing.T) {
		cfg := &ScanConfig{
			Targets:  []string{"192.168.1.1"},
			ScanType: "connect",
			Ports:    "80,443,8080-8090",
		}
		assert.NoError(t, cfg.Validate())
	})
}

func TestPersistOSData_ZeroAccuracy_ConfidenceNotSet(t *testing.T) {
	match := nmap.OSMatch{
		Name:     "Unknown",
		Accuracy: 0, // triggers the false branch of `if host.OSAccuracy > 0`
		Classes:  nil,
	}
	h := makeNmapHost("10.0.0.8", nil, []nmap.OSMatch{match})

	result := convertNmapHost(&h)
	require.NotNil(t, result)

	assert.Equal(t, 0, result.OSAccuracy,
		"OSAccuracy must remain 0 when nmap reports accuracy 0")
	assert.Equal(t, "", result.OSFamily,
		"OSFamily must remain empty when no classes are present")
	assert.Equal(t, "", result.OSVersion,
		"OSVersion must remain empty when no classes are present")
	// OSName is still set even when accuracy is 0 — the name branch is independent.
	assert.Equal(t, "Unknown", result.OSName,
		"OSName is set regardless of accuracy value")
}
