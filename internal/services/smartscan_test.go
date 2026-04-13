// Package services - unit tests for SmartScanService.
package services

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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

// ── applyKnowledgeFilter ─────────────────────────────────────────────────────

func TestApplyKnowledgeFilter(t *testing.T) {
	now := time.Now()
	highScore := &db.Host{ID: uuid.New(), KnowledgeScore: 90, LastSeen: now, IPAddress: mustParseIP("10.0.0.1")}
	lowScore := &db.Host{ID: uuid.New(), KnowledgeScore: 50, LastSeen: now, IPAddress: mustParseIP("10.0.0.2")}
	staleHost := &db.Host{
		ID:             uuid.New(),
		KnowledgeScore: 90,
		LastSeen:       now.Add(-50 * time.Hour),
		IPAddress:      mustParseIP("10.0.0.3"),
	}
	lowScoreAndStale := &db.Host{
		ID:             uuid.New(),
		KnowledgeScore: 40,
		LastSeen:       now.Add(-50 * time.Hour),
		IPAddress:      mustParseIP("10.0.0.4"),
	}
	all := []*db.Host{highScore, lowScore, staleHost, lowScoreAndStale}

	tests := []struct {
		name      string
		filter    BatchFilter
		wantIDs   []uuid.UUID
		wantCount int
	}{
		{
			name:      "score threshold only — includes low-score hosts",
			filter:    BatchFilter{ScoreThreshold: 80},
			wantIDs:   []uuid.UUID{lowScore.ID, lowScoreAndStale.ID},
			wantCount: 2,
		},
		{
			name:      "staleness only — includes hosts not seen within 24h",
			filter:    BatchFilter{MaxStalenessHours: 24},
			wantIDs:   []uuid.UUID{staleHost.ID, lowScoreAndStale.ID},
			wantCount: 2,
		},
		{
			name:      "OR logic — score OR staleness",
			filter:    BatchFilter{ScoreThreshold: 80, MaxStalenessHours: 24},
			wantIDs:   []uuid.UUID{lowScore.ID, staleHost.ID, lowScoreAndStale.ID},
			wantCount: 3,
		},
		{
			name:      "both zero — no filter applied, all hosts returned",
			filter:    BatchFilter{},
			wantCount: 0, // applyKnowledgeFilter is not called when both zero; passing here to show empty result
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.filter.ScoreThreshold == 0 && tt.filter.MaxStalenessHours == 0 {
				// Both zero: guard in resolveHosts prevents the call; verify contract
				got := applyKnowledgeFilter(all, tt.filter)
				assert.Empty(t, got, "both-zero filter should return no hosts (caller guards this)")
				return
			}
			got := applyKnowledgeFilter(all, tt.filter)
			assert.Equal(t, tt.wantCount, len(got))
			gotIDs := make(map[uuid.UUID]bool, len(got))
			for _, h := range got {
				gotIDs[h.ID] = true
			}
			for _, id := range tt.wantIDs {
				assert.True(t, gotIDs[id], "expected host %s in result", id)
			}
		})
	}
}

// ── WithAutoProgression ───────────────────────────────────────────────────────

func TestWithAutoProgression_DefaultValues(t *testing.T) {
	svc := &SmartScanService{logger: discardLogger()}
	// Pass zero for all optional parameters — should fall back to defaults.
	got := svc.WithAutoProgression(0, 0, 0)

	assert.Same(t, svc, got, "must return receiver for chaining")
	assert.True(t, svc.autoProgressEnabled)
	assert.Equal(t, AutoProgressDefaultThreshold, svc.autoProgressThreshold)
	assert.Equal(t, AutoProgressDefaultMaxPerWindow, svc.autoProgressMaxPerWin)
	assert.Equal(t, time.Duration(AutoProgressDefaultWindowHours)*time.Hour, svc.autoProgressWindow)
}

func TestWithAutoProgression_ExplicitValues(t *testing.T) {
	svc := &SmartScanService{logger: discardLogger()}
	got := svc.WithAutoProgression(60, 5, 48)

	assert.Same(t, svc, got)
	assert.True(t, svc.autoProgressEnabled)
	assert.Equal(t, 60, svc.autoProgressThreshold)
	assert.Equal(t, 5, svc.autoProgressMaxPerWin)
	assert.Equal(t, 48*time.Hour, svc.autoProgressWindow)
}

// ── QueueBatch with ScoreThreshold / MaxStalenessHours ───────────────────────

func TestQueueBatch_ScoreThresholdFilter(t *testing.T) {
	createCalls := 0
	scanRepo := &mockSmartScanRepo{
		createScanFn: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			createCalls++
			return &db.Scan{ID: uuid.New()}, nil
		},
	}

	// Two hosts: one high-score (should be skipped), one low-score (should be queued).
	// Both have no OS so EvaluateHost → os_detection.
	highScore := &db.Host{
		ID: uuid.New(), KnowledgeScore: 90,
		Status:    "up",
		LastSeen:  time.Now(),
		IPAddress: mustParseIP("10.0.0.1"),
	}
	lowScore := &db.Host{
		ID: uuid.New(), KnowledgeScore: 30,
		Status:    "up",
		LastSeen:  time.Now(),
		IPAddress: mustParseIP("10.0.0.2"),
	}

	hostRepo := &mockSmartHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			return []*db.Host{highScore, lowScore}, 2, nil
		},
	}
	svc := &SmartScanService{
		hostRepo:       hostRepo,
		scanRepo:       scanRepo,
		logger:         discardLogger(),
		hasOpenPortsFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:  func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}

	result, err := svc.QueueBatch(context.Background(), BatchFilter{ScoreThreshold: 80, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Queued, "only low-score host should be queued")
	assert.Equal(t, 1, createCalls)
}

func TestQueueBatch_StalenessFilter(t *testing.T) {
	createCalls := 0
	scanRepo := &mockSmartScanRepo{
		createScanFn: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			createCalls++
			return &db.Scan{ID: uuid.New()}, nil
		},
	}

	recent := &db.Host{
		ID:        uuid.New(),
		Status:    "up",
		LastSeen:  time.Now(),
		IPAddress: mustParseIP("10.0.0.1"),
	}
	stale := &db.Host{
		ID:        uuid.New(),
		Status:    "up",
		LastSeen:  time.Now().Add(-72 * time.Hour),
		IPAddress: mustParseIP("10.0.0.2"),
	}

	hostRepo := &mockSmartHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			return []*db.Host{recent, stale}, 2, nil
		},
	}
	svc := &SmartScanService{
		hostRepo:       hostRepo,
		scanRepo:       scanRepo,
		logger:         discardLogger(),
		hasOpenPortsFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:  func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}

	result, err := svc.QueueBatch(context.Background(), BatchFilter{MaxStalenessHours: 48, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Queued, "only stale host should be queued")
	assert.Equal(t, 1, createCalls)
}

// ── ReEvaluateHosts auto-progression paths ────────────────────────────────────

func newAutoProgressSvc(
	t *testing.T, threshold, maxPerWin int,
	createFn func(context.Context, db.CreateScanInput) (*db.Scan, error),
) *SmartScanService {
	t.Helper()
	scanRepo := &mockSmartScanRepo{createScanFn: createFn}
	svc := &SmartScanService{
		logger:                discardLogger(),
		scanRepo:              scanRepo,
		autoProgressEnabled:   true,
		autoProgressThreshold: threshold,
		autoProgressMaxPerWin: maxPerWin,
		autoProgressWindow:    24 * time.Hour,
		hasOpenPortsFn:        func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:         func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}
	return svc
}

func TestReEvaluateHosts_AutoProgressionDisabled(t *testing.T) {
	queued := 0
	scanRepo := &mockSmartScanRepo{
		createScanFn: func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
			queued++
			return &db.Scan{ID: uuid.New()}, nil
		},
	}
	host := hostUp("10.0.1.1")
	hostRepo := &mockSmartHostRepo{
		getHostFn: func(_ context.Context, _ uuid.UUID) (*db.Host, error) { return host, nil },
	}
	svc := &SmartScanService{
		logger:              discardLogger(),
		hostRepo:            hostRepo,
		scanRepo:            scanRepo,
		autoProgressEnabled: false, // disabled
		hasOpenPortsFn:      func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		hasServicesFn:       func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
	}

	svc.ReEvaluateHosts(nil, []uuid.UUID{host.ID})
	assert.Equal(t, 0, queued, "no scan should be created when auto-progression is disabled")
}

func TestReEvaluateHosts_AutoProgressionSkipsHighScoreHost(t *testing.T) {
	queued := 0
	host := hostUp("10.0.1.2")
	host.KnowledgeScore = 95 // above threshold

	hostRepo := &mockSmartHostRepo{
		getHostFn: func(_ context.Context, _ uuid.UUID) (*db.Host, error) { return host, nil },
	}
	svc := newAutoProgressSvc(t, 80, 0, func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
		queued++
		return &db.Scan{ID: uuid.New()}, nil
	})
	svc.hostRepo = hostRepo

	svc.ReEvaluateHosts(nil, []uuid.UUID{host.ID})
	assert.Equal(t, 0, queued, "host with score >= threshold must not be re-queued")
}

func TestReEvaluateHosts_AutoProgressionSkipsSkipStage(t *testing.T) {
	queued := 0
	// A host that evaluates to skip: all knowledge present and not stale.
	host := hostUp("10.0.1.3")
	host.KnowledgeScore = 30
	host.OSFamily = strPtr("linux")

	hostRepo := &mockSmartHostRepo{
		getHostFn: func(_ context.Context, _ uuid.UUID) (*db.Host, error) { return host, nil },
	}
	svc := newAutoProgressSvc(t, 80, 0, func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
		queued++
		return &db.Scan{ID: uuid.New()}, nil
	})
	svc.hostRepo = hostRepo
	// Force EvaluateHost to return skip by making port and service checks return true.
	svc.hasOpenPortsFn = func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }
	svc.hasServicesFn = func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }

	svc.ReEvaluateHosts(nil, []uuid.UUID{host.ID})
	assert.Equal(t, 0, queued, "skip stage must not trigger a re-queue")
}

func TestReEvaluateHosts_AutoProgressionQueuesLowScoreHost(t *testing.T) {
	queued := 0
	host := hostUp("10.0.1.4")
	host.KnowledgeScore = 30 // below threshold

	hostRepo := &mockSmartHostRepo{
		getHostFn: func(_ context.Context, _ uuid.UUID) (*db.Host, error) { return host, nil },
	}
	// maxPerWin=0 skips the exceedsAutoQueueLimit DB query so no real DB needed.
	svc := newAutoProgressSvc(t, 80, 0, func(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
		queued++
		return &db.Scan{ID: uuid.New()}, nil
	})
	svc.hostRepo = hostRepo

	svc.ReEvaluateHosts(nil, []uuid.UUID{host.ID})
	assert.Equal(t, 1, queued, "low-score host below threshold must be re-queued")
}

// ── exceedsAutoQueueLimit ─────────────────────────────────────────────────────

// newSmartScanMockDB returns a *db.DB backed by sqlmock for exceedsAutoQueueLimit tests.
func newSmartScanMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

func TestExceedsAutoQueueLimit_BelowLimit(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	svc := &SmartScanService{
		database:              database,
		autoProgressMaxPerWin: 3,
		autoProgressWindow:    24 * time.Hour,
	}

	exceeded := svc.exceedsAutoQueueLimit(context.Background(), "10.0.0.1")
	assert.False(t, exceeded, "count 1 < limit 3: must not be exceeded")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExceedsAutoQueueLimit_AtOrAboveLimit(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	svc := &SmartScanService{
		database:              database,
		autoProgressMaxPerWin: 3,
		autoProgressWindow:    24 * time.Hour,
	}

	exceeded := svc.exceedsAutoQueueLimit(context.Background(), "10.0.0.1")
	assert.True(t, exceeded, "count 3 == limit 3: must be exceeded")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExceedsAutoQueueLimit_DBErrorFailsOpen(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	mock.ExpectQuery("SELECT COUNT").
		WillReturnError(sql.ErrConnDone)

	svc := &SmartScanService{
		database:              database,
		autoProgressMaxPerWin: 3,
		autoProgressWindow:    24 * time.Hour,
	}

	exceeded := svc.exceedsAutoQueueLimit(context.Background(), "10.0.0.1")
	assert.False(t, exceeded, "DB error must fail open (allow the queue)")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ── filterByOSFamily ──────────────────────────────────────────────────────────

func TestFilterByOSFamily_MatchesCaseInsensitive(t *testing.T) {
	linux := strPtr("linux")
	windows := strPtr("Windows")
	empty := strPtr("")
	hosts := []*db.Host{
		{ID: uuid.New(), OSFamily: linux, IPAddress: mustParseIP("10.0.0.1")},
		{ID: uuid.New(), OSFamily: windows, IPAddress: mustParseIP("10.0.0.2")},
		{ID: uuid.New(), OSFamily: empty, IPAddress: mustParseIP("10.0.0.3")},
		{ID: uuid.New(), OSFamily: nil, IPAddress: mustParseIP("10.0.0.4")},
	}

	got := filterByOSFamily(hosts, "Linux")
	require.Len(t, got, 1)
	assert.Equal(t, hosts[0].ID, got[0].ID)
}

func TestFilterByOSFamily_EmptyFamilyReturnsNone(t *testing.T) {
	// A nil OSFamily host must not match even when the filter is "".
	// filterByOSFamily is only called with non-empty filter values, but ensure
	// the nil-check is robust.
	hosts := []*db.Host{{ID: uuid.New(), OSFamily: nil, IPAddress: mustParseIP("10.0.0.1")}}
	got := filterByOSFamily(hosts, "")
	// nil-check: no os_family match for ""
	assert.Empty(t, got)
}

// ── GetProfileRecommendations ─────────────────────────────────────────────────

func TestGetProfileRecommendations_NilProfileManager_ReturnsNil(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	// DB query still runs; profileManager nil causes early return.
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "host_count"}).
			AddRow("linux", 5))

	svc := &SmartScanService{database: database, profileManager: nil}
	recs, err := svc.GetProfileRecommendations(context.Background())
	require.NoError(t, err)
	assert.Nil(t, recs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetProfileRecommendations_DBError_ReturnsError(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	mock.ExpectQuery("SELECT os_family").
		WillReturnError(fmt.Errorf("connection reset"))

	svc := &SmartScanService{database: database}
	_, err := svc.GetProfileRecommendations(context.Background())
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetProfileRecommendations_NoHosts_ReturnsEmpty(t *testing.T) {
	database, mock := newSmartScanMockDB(t)
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "host_count"}))

	svc := &SmartScanService{database: database, profileManager: nil}
	recs, err := svc.GetProfileRecommendations(context.Background())
	require.NoError(t, err)
	assert.Nil(t, recs)
	require.NoError(t, mock.ExpectationsWereMet())
}
