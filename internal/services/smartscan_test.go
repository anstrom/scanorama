// Package services - unit tests for SmartScanService.
package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// ── mock implementations ──────────────────────────────────────────────────────

type mockSmartHostRepo struct {
	getHostFn   func(ctx context.Context, id uuid.UUID) (*db.Host, error)
	listHostsFn func(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
}

func (m *mockSmartHostRepo) GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error) {
	if m.getHostFn != nil {
		return m.getHostFn(ctx, id)
	}
	return nil, errors.New("mockSmartHostRepo: GetHost not configured")
}

func (m *mockSmartHostRepo) ListHosts(
	ctx context.Context, f *db.HostFilters, offset, limit int,
) ([]*db.Host, int64, error) {
	if m.listHostsFn != nil {
		return m.listHostsFn(ctx, f, offset, limit)
	}
	return nil, 0, errors.New("mockSmartHostRepo: ListHosts not configured")
}

func (m *mockSmartHostRepo) RecalculateKnowledgeScore(_ context.Context, _ uuid.UUID) error {
	return nil
}

type mockSmartScanRepo struct {
	createScanFn func(ctx context.Context, input db.CreateScanInput) (*db.Scan, error)
	startScanFn  func(ctx context.Context, id uuid.UUID) error
	stopScanFn   func(ctx context.Context, id uuid.UUID, msg ...string) error
}

func (m *mockSmartScanRepo) CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error) {
	if m.createScanFn != nil {
		return m.createScanFn(ctx, input)
	}
	return nil, errors.New("mockSmartScanRepo: CreateScan not configured")
}

func (m *mockSmartScanRepo) StartScan(ctx context.Context, id uuid.UUID) error {
	if m.startScanFn != nil {
		return m.startScanFn(ctx, id)
	}
	return nil
}

func (m *mockSmartScanRepo) StopScan(ctx context.Context, id uuid.UUID, msg ...string) error {
	if m.stopScanFn != nil {
		return m.stopScanFn(ctx, id, msg...)
	}
	return nil
}

func (m *mockSmartScanRepo) CompleteScan(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockSmartScanRepo) DeleteScan(_ context.Context, _ uuid.UUID) error   { return nil }
func (m *mockSmartScanRepo) ListScans(_ context.Context, _ db.ScanFilters, _, _ int) ([]*db.Scan, int64, error) {
	return nil, 0, nil
}
func (m *mockSmartScanRepo) GetScan(_ context.Context, _ uuid.UUID) (*db.Scan, error) {
	return nil, nil
}
func (m *mockSmartScanRepo) UpdateScan(_ context.Context, _ uuid.UUID, _ db.UpdateScanInput) (*db.Scan, error) {
	return nil, nil
}
func (m *mockSmartScanRepo) GetScanResults(_ context.Context, _ uuid.UUID, _, _ int) ([]*db.ScanResult, int64, error) {
	return nil, 0, nil
}
func (m *mockSmartScanRepo) GetScanSummary(_ context.Context, _ uuid.UUID) (*db.ScanSummary, error) {
	return nil, nil
}
func (m *mockSmartScanRepo) GetProfile(_ context.Context, _ string) (*db.ScanProfile, error) {
	return nil, nil
}

// ── test helpers ──────────────────────────────────────────────────────────────

// newTestService creates a SmartScanService with injected has-open-ports and
// has-services functions so tests never touch a real database.
func newTestService(hasOpenPorts, hasServices bool) *SmartScanService {
	svc := &SmartScanService{
		profileManager: nil,
		logger:         discardLogger(),
	}
	svc.hasOpenPortsFn = func(_ context.Context, _ uuid.UUID) (bool, error) {
		return hasOpenPorts, nil
	}
	svc.hasServicesFn = func(_ context.Context, _ uuid.UUID) (bool, error) {
		return hasServices, nil
	}
	return svc
}

func strPtr(s string) *string { return &s }

func mustParseIP(s string) db.IPAddr {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid IP: " + s)
	}
	return db.IPAddr{IP: ip}
}

// hostUp returns a minimal up host seen recently (not stale).
func hostUp(ip string) *db.Host {
	return &db.Host{
		ID:        uuid.New(),
		IPAddress: mustParseIP(ip),
		Status:    "up",
		LastSeen:  time.Now(),
	}
}

// hostStale returns an up host whose last_seen is beyond the stale threshold.
func hostStale(ip string) *db.Host {
	h := hostUp(ip)
	h.LastSeen = time.Now().Add(-40 * 24 * time.Hour)
	return h
}

// ── EvaluateHost: skip conditions ────────────────────────────────────────────

func TestEvaluateHost_SkipsGoneHost(t *testing.T) {
	svc := newTestService(false, false)
	host := hostUp("10.0.0.1")
	host.Status = "gone"

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "skip", stage.Stage)
}

func TestEvaluateHost_SkipsIgnoredHost(t *testing.T) {
	svc := newTestService(false, false)
	host := hostUp("10.0.0.1")
	host.IgnoreScanning = true

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "skip", stage.Stage)
}

// ── EvaluateHost: OS detection ────────────────────────────────────────────────

func TestEvaluateHost_OSDetectionWhenNoOSAndUp(t *testing.T) {
	svc := newTestService(false, false)
	host := hostUp("10.0.0.2")
	// OSFamily is nil — no OS known

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "os_detection", stage.Stage)
	assert.True(t, stage.OSDetection, "OS detection flag must be set")
	assert.Equal(t, "syn", stage.ScanType, "OS detection requires SYN scan for raw-socket access")
	assert.NotEmpty(t, stage.Ports)
}

func TestEvaluateHost_NoOSOnDownHostSkips(t *testing.T) {
	svc := newTestService(false, false)
	host := hostUp("10.0.0.3")
	host.Status = "down"
	// No OS, but host is down — os_detection case requires status=="up".

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "skip", stage.Stage)
}

// ── EvaluateHost: port expansion ─────────────────────────────────────────────

func TestEvaluateHost_PortExpansionWhenOSKnownButNoPorts(t *testing.T) {
	svc := newTestService(false /*hasOpenPorts*/, false)
	host := hostUp("10.0.0.4")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "port_expansion", stage.Stage)
	assert.NotEmpty(t, stage.Ports)
}

// ── EvaluateHost: service scan ────────────────────────────────────────────────

func TestEvaluateHost_ServiceScanWhenPortsButNoServices(t *testing.T) {
	svc := newTestService(true /*hasOpenPorts*/, false /*hasServices*/)
	host := hostUp("10.0.0.5")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "service_scan", stage.Stage)
	assert.NotEmpty(t, stage.Ports)
}

// ── EvaluateHost: stale refresh ───────────────────────────────────────────────

func TestEvaluateHost_RefreshWhenStale(t *testing.T) {
	// A host with complete knowledge (OS + open ports + services) but last seen
	// over 30 days ago should be refreshed, not skipped.
	svc := newTestService(true /*hasOpenPorts*/, true /*hasServices*/)
	host := hostStale("10.0.0.6")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "refresh", stage.Stage)
}

func TestEvaluateHost_NotStaleWhenRecentlySeen(t *testing.T) {
	svc := newTestService(true /*hasOpenPorts*/, true /*hasServices*/)
	host := hostUp("10.0.0.6")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "skip", stage.Stage)
}

// ── EvaluateHost: skip when complete ─────────────────────────────────────────

func TestEvaluateHost_SkipWhenComplete(t *testing.T) {
	svc := newTestService(true /*hasOpenPorts*/, true /*hasServices*/)
	host := hostUp("10.0.0.7")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "skip", stage.Stage)
}

// ── EvaluateHost: DB error tolerance ─────────────────────────────────────────

func TestEvaluateHost_FallsBackOnOpenPortsError(t *testing.T) {
	// If the open-ports query fails, hasOpenPorts is treated as false.
	// A host with OS but a failing ports query should route to port_expansion.
	svc := &SmartScanService{
		logger: discardLogger(),
		hasOpenPortsFn: func(_ context.Context, _ uuid.UUID) (bool, error) {
			return false, errors.New("db error")
		},
		hasServicesFn: func(_ context.Context, _ uuid.UUID) (bool, error) {
			return false, nil
		},
	}
	host := hostUp("10.0.0.8")
	host.OSFamily = strPtr("linux")

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "port_expansion", stage.Stage)
}

// ── QueueBatch ────────────────────────────────────────────────────────────────

func TestQueueBatch_RespectsLimit(t *testing.T) {
	createCalls := 0
	scanRepo := &mockSmartScanRepo{
		createScanFn: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			createCalls++
			return &db.Scan{ID: uuid.New()}, nil
		},
	}

	// 10 hosts, all needing os_detection (no OS, no ports).
	hosts := make([]*db.Host, 10)
	for i := range hosts {
		hosts[i] = hostUp(fmt.Sprintf("10.0.0.%d", i+1))
	}
	hostRepo := &mockSmartHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			return hosts, int64(len(hosts)), nil
		},
	}

	svc := &SmartScanService{
		hostRepo:       hostRepo,
		scanRepo:       scanRepo,
		logger:         discardLogger(),
		hasOpenPortsFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:  func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}

	result, err := svc.QueueBatch(context.Background(), BatchFilter{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, 3, result.Queued, "should stop queuing after limit")
	assert.Equal(t, 3, createCalls, "CreateScan should not be called beyond the limit")
}

func TestQueueBatch_StageFilter(t *testing.T) {
	scanRepo := &mockSmartScanRepo{
		createScanFn: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			return &db.Scan{ID: uuid.New()}, nil
		},
	}

	// Two hosts: one needing os_detection, one needing port_expansion.
	noOSHost := hostUp("10.0.0.1")
	hasOSHost := hostUp("10.0.0.2")
	hasOSHost.OSFamily = strPtr("linux")

	hostRepo := &mockSmartHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			return []*db.Host{noOSHost, hasOSHost}, 2, nil
		},
	}

	svc := &SmartScanService{
		hostRepo:       hostRepo,
		scanRepo:       scanRepo,
		logger:         discardLogger(),
		hasOpenPortsFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:  func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}

	result, err := svc.QueueBatch(context.Background(), BatchFilter{Stage: "port_expansion", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Queued, "only the host needing port_expansion should be queued")
	assert.Equal(t, 1, result.Skipped, "the os_detection host should be skipped by stage filter")
	require.Len(t, result.Details, 1)
	assert.Equal(t, "port_expansion", result.Details[0].Stage)
}
