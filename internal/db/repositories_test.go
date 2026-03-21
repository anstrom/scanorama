//go:build integration

package db

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── Scan repository ───────────────────────────────────────────────────────────

func TestScanRepository_CreateAndGet(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.1/32'")
	input := map[string]interface{}{
		"name":      "test-scan-create",
		"targets":   []string{"127.0.0.1"},
		"scan_type": "connect",
		"ports":     "22,80",
	}

	scan, err := db.CreateScan(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, scan)
	assert.NotEqual(t, uuid.Nil, scan.ID)
	assert.Equal(t, "test-scan-create", scan.Name)
	assert.Equal(t, "connect", scan.ScanType)
	assert.Equal(t, "22,80", scan.Ports)

	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	got, err := db.GetScan(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, scan.ID, got.ID)
	assert.Equal(t, scan.Name, got.Name)
	assert.Equal(t, "pending", got.Status)
	// PortsScanned should be populated from the ports field.
	require.NotNil(t, got.PortsScanned)
	assert.Equal(t, "22,80", *got.PortsScanned)
	// ErrorMessage should be nil for a freshly created scan.
	assert.Nil(t, got.ErrorMessage)
	// DurationStr should be nil until both timestamps are set.
	assert.Nil(t, got.DurationStr)
}

func TestScanRepository_GetScan_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.GetScan(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestScanRepository_ListScans(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.2/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-list",
		"targets":   []string{"127.0.0.2"},
		"scan_type": "connect",
		"ports":     "443",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	scans, total, err := db.ListScans(ctx, ScanFilters{}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.GreaterOrEqual(t, len(scans), 1)

	// Find our scan in the list and check the new derived fields.
	var found *Scan
	for _, s := range scans {
		if s.ID == scan.ID {
			found = s
			break
		}
	}
	require.NotNil(t, found, "created scan should appear in ListScans")
	require.NotNil(t, found.PortsScanned)
	assert.Equal(t, "443", *found.PortsScanned)
	assert.Nil(t, found.ErrorMessage)
}

func TestScanRepository_UpdateScan(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.1/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-update",
		"targets":   []string{"127.0.0.1"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	updated, err := db.UpdateScan(ctx, scan.ID, map[string]interface{}{
		"ports": "22,443",
	})
	require.NoError(t, err)
	assert.Equal(t, "22,443", updated.Ports)
}

func TestScanRepository_UpdateScan_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.UpdateScan(context.Background(), uuid.New(), map[string]interface{}{
		"ports": "80",
	})
	require.Error(t, err)
}

func TestScanRepository_DeleteScan(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.3/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-delete",
		"targets":   []string{"127.0.0.3"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)

	require.NoError(t, db.DeleteScan(ctx, scan.ID))

	_, err = db.GetScan(ctx, scan.ID)
	require.Error(t, err)
}

func TestScanRepository_DeleteScan_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	err := db.DeleteScan(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestScanRepository_DeleteScan_Running(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.5/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-running",
		"targets":   []string{"127.0.0.5"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	require.NoError(t, db.StartScan(ctx, scan.ID))
	t.Cleanup(func() { _ = db.StopScan(ctx, scan.ID) })

	// Deleting a running scan should return a conflict error.
	err = db.DeleteScan(ctx, scan.ID)
	require.Error(t, err)
	assert.True(t, errors.IsConflict(err), "expected conflict error, got: %v", err)
}

func TestScanRepository_StartCompleteScan(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.4/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-lifecycle",
		"targets":   []string{"127.0.0.4"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	require.NoError(t, db.StartScan(ctx, scan.ID))

	started, err := db.GetScan(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", started.Status)
	assert.NotNil(t, started.StartedAt)

	require.NoError(t, db.CompleteScan(ctx, scan.ID))

	completed, err := db.GetScan(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", completed.Status)
	assert.NotNil(t, completed.CompletedAt)
	// DurationStr should now be populated since both timestamps are set.
	assert.NotNil(t, completed.DurationStr)
}

func TestScanRepository_StopScan(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.1/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-stop",
		"targets":   []string{"127.0.0.1"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	require.NoError(t, db.StartScan(ctx, scan.ID))
	require.NoError(t, db.StopScan(ctx, scan.ID))

	stopped, err := db.GetScan(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stopped.Status)
}

func TestScanRepository_GetScanResults(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.1/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-results",
		"targets":   []string{"127.0.0.1"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	// No results yet — should return empty slice, not error.
	results, total, err := db.GetScanResults(ctx, scan.ID, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, results)
}

func TestScanRepository_GetScanSummary(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '127.0.0.1/32'")
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "test-scan-summary",
		"targets":   []string{"127.0.0.1"},
		"scan_type": "connect",
		"ports":     "22",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	summary, err := db.GetScanSummary(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, scan.ID, summary.ScanID)
	assert.Equal(t, 0, summary.TotalPorts)
}

// ── ScanJob repository ────────────────────────────────────────────────────────

func TestScanJobRepository_CreateAndGet(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE name = 'job-repo-target'")
	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '10.0.0.1/32'")
	networkID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', true, false, 0, '22', 'connect'
		)`,
		networkID, "job-repo-target", "10.0.0.1/32")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", networkID)
	})

	jobRepo := NewScanJobRepository(db)
	jobID := uuid.New()
	job := &ScanJob{ID: jobID, NetworkID: networkID, Status: ScanJobStatusPending}
	require.NoError(t, jobRepo.Create(ctx, job))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", jobID)
	})

	got, err := jobRepo.GetByID(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, jobID, got.ID)
	assert.Equal(t, ScanJobStatusPending, got.Status)
}

func TestScanJobRepository_UpdateStatus(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE name = 'job-status-target'")
	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '10.0.0.2/32'")
	networkID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', true, false, 0, '22', 'connect'
		)`,
		networkID, "job-status-target", "10.0.0.2/32")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", networkID)
	})

	jobRepo := NewScanJobRepository(db)
	jobID := uuid.New()
	require.NoError(t, jobRepo.Create(ctx, &ScanJob{
		ID: jobID, NetworkID: networkID, Status: ScanJobStatusPending,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", jobID)
	})

	require.NoError(t, jobRepo.UpdateStatus(ctx, jobID, ScanJobStatusRunning, nil))

	got, err := jobRepo.GetByID(ctx, jobID)
	require.NoError(t, err)
	assert.Equal(t, ScanJobStatusRunning, got.Status)
}

// ── PortScan repository ───────────────────────────────────────────────────────

func TestPortScanRepository_CreateBatchAndGetByHost(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert a host to attach port scans to.
	hostRepo := NewHostRepository(db)
	hostID := uuid.New()
	ip := IPAddr{IP: net.ParseIP("10.0.1.1")}
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "10.0.1.1")
	require.NoError(t, hostRepo.CreateOrUpdate(ctx, &Host{
		ID:        hostID,
		IPAddress: ip,
		Status:    HostStatusUp,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "10.0.1.1")
	})

	// Insert a network and scan job to attach port scans to.
	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE name = 'port-scan-batch-target'")
	_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr = '10.0.1.1/32'")
	networkID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', true, false, 0, '22,80', 'connect'
		)`,
		networkID, "port-scan-batch-target", "10.0.1.1/32")
	require.NoError(t, err)
	jobRepo := NewScanJobRepository(db)
	jobID := uuid.New()
	require.NoError(t, jobRepo.Create(ctx, &ScanJob{
		ID: jobID, NetworkID: networkID, Status: ScanJobStatusPending,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", jobID)
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", networkID)
	})

	portRepo := NewPortScanRepository(db)
	batch := []*PortScan{
		{ID: uuid.New(), JobID: jobID, HostID: hostID, Port: 22, Protocol: "tcp", State: "open"},
		{ID: uuid.New(), JobID: jobID, HostID: hostID, Port: 80, Protocol: "tcp", State: "open"},
	}
	require.NoError(t, portRepo.CreateBatch(ctx, batch))

	got, err := portRepo.GetByHost(ctx, hostID)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

// ── Network repository (raw SQL) ──────────────────────────────────────────────

func TestNetworkRepository_CRUD(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()
	id := uuid.New()

	// 1. Insert
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', true, true, 3600, '22', 'connect'
		)`,
		id, "crud-network", "192.168.99.0/24")
	require.NoError(t, err)

	// 2. Select back
	var got Network
	err = db.GetContext(ctx, &got,
		"SELECT id, name, cidr, is_active, scan_enabled, scan_interval_seconds, scan_ports, scan_type, discovery_method, host_count, active_host_count, created_at, updated_at FROM networks WHERE id = $1",
		id)
	require.NoError(t, err)

	// 3. Verify
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "crud-network", got.Name)
	assert.True(t, got.IsActive)
	assert.True(t, got.ScanEnabled)
	assert.Equal(t, 3600, got.ScanIntervalSeconds)
	assert.Equal(t, "22", got.ScanPorts)
	assert.Equal(t, "connect", got.ScanType)
	assert.Equal(t, "tcp", got.DiscoveryMethod)

	// 4. Delete
	_, err = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", id)
	require.NoError(t, err)

	// Confirm deletion
	err = db.GetContext(ctx, &got, "SELECT id FROM networks WHERE id = $1", id)
	require.Error(t, err)
}

// ── Profile repository ────────────────────────────────────────────────────────

func TestProfileRepository_CRUD(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create.
	profile, err := db.CreateProfile(ctx, map[string]interface{}{
		"name":      "test-profile-crud",
		"scan_type": "connect",
		"ports":     "22,80,443",
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "test-profile-crud", profile.Name)
	t.Cleanup(func() { _ = db.DeleteProfile(ctx, profile.ID) })

	// Get.
	got, err := db.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, profile.ID, got.ID)
	assert.Equal(t, "connect", got.ScanType)
	assert.Equal(t, "22,80,443", got.Ports)

	// List.
	profiles, total, err := db.ListProfiles(ctx, ProfileFilters{}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	var found bool
	for _, p := range profiles {
		if p.ID == profile.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "created profile should appear in ListProfiles")

	// List with filter.
	filtered, _, err := db.ListProfiles(ctx, ProfileFilters{ScanType: "connect"}, 0, 100)
	require.NoError(t, err)
	for _, p := range filtered {
		assert.Equal(t, "connect", p.ScanType)
	}

	// Update.
	updated, err := db.UpdateProfile(ctx, profile.ID, map[string]interface{}{
		"ports": "22,443",
	})
	require.NoError(t, err)
	assert.Equal(t, "22,443", updated.Ports)

	// Delete.
	require.NoError(t, db.DeleteProfile(ctx, profile.ID))
	_, err = db.GetProfile(ctx, profile.ID)
	require.Error(t, err)
}

func TestProfileRepository_CreateProfile_DuplicateName(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM scan_profiles WHERE name = 'test-duplicate-profile' AND is_builtin = false")
	profile, err := db.CreateProfile(ctx, map[string]interface{}{
		"name":      "test-duplicate-profile",
		"scan_type": "connect",
		"ports":     "80,443",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteProfile(ctx, profile.ID) })

	// Creating a second profile with the same name should return a conflict error.
	_, err = db.CreateProfile(ctx, map[string]interface{}{
		"name":      "test-duplicate-profile",
		"scan_type": "connect",
		"ports":     "22",
	})
	require.Error(t, err)
	assert.True(t, errors.IsConflict(err), "expected conflict error, got: %v", err)
}

func TestProfileRepository_GetProfile_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.GetProfile(context.Background(), "nonexistent-profile-id")
	require.Error(t, err)
}

func TestProfileRepository_UpdateBuiltIn_Rejected(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Find a built-in profile (seeded by migrations).
	var builtInID string
	err := db.QueryRowContext(ctx,
		"SELECT id FROM scan_profiles WHERE built_in = true LIMIT 1").Scan(&builtInID)
	if err != nil {
		t.Skip("no built-in profiles found, skipping")
	}

	_, err = db.UpdateProfile(ctx, builtInID, map[string]interface{}{"ports": "9999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}

func TestProfileRepository_DeleteBuiltIn_Rejected(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	var builtInID string
	err := db.QueryRowContext(ctx,
		"SELECT id FROM scan_profiles WHERE built_in = true LIMIT 1").Scan(&builtInID)
	if err != nil {
		t.Skip("no built-in profiles found, skipping")
	}

	err = db.DeleteProfile(ctx, builtInID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}

// ── Schedule repository ───────────────────────────────────────────────────────

func TestScheduleRepository_CRUD(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create.
	schedule, err := db.CreateSchedule(ctx, map[string]interface{}{
		"name":            "test-schedule-crud",
		"job_type":        "discovery",
		"cron_expression": "0 * * * *",
		"enabled":         true,
		"job_config": map[string]interface{}{
			"network": "10.0.0.0/24",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, schedule)
	assert.Equal(t, "test-schedule-crud", schedule.Name)
	assert.Equal(t, "discovery", schedule.JobType)
	assert.True(t, schedule.Enabled)
	t.Cleanup(func() { _ = db.DeleteSchedule(ctx, schedule.ID) })

	// Get.
	got, err := db.GetSchedule(ctx, schedule.ID)
	require.NoError(t, err)
	assert.Equal(t, schedule.ID, got.ID)
	assert.Equal(t, "0 * * * *", got.CronExpression)

	// List.
	schedules, total, err := db.ListSchedules(ctx, ScheduleFilters{}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	var found bool
	for _, s := range schedules {
		if s.ID == schedule.ID {
			found = true
			break
		}
	}
	assert.True(t, found)

	// List with job_type filter.
	filtered, _, err := db.ListSchedules(ctx, ScheduleFilters{JobType: "discovery"}, 0, 100)
	require.NoError(t, err)
	for _, s := range filtered {
		assert.Equal(t, "discovery", s.JobType)
	}

	// Update.
	updated, err := db.UpdateSchedule(ctx, schedule.ID, map[string]interface{}{
		"cron_expression": "30 * * * *",
	})
	require.NoError(t, err)
	assert.Equal(t, "30 * * * *", updated.CronExpression)

	// Disable / Enable.
	require.NoError(t, db.DisableSchedule(ctx, schedule.ID))
	disabled, err := db.GetSchedule(ctx, schedule.ID)
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)

	require.NoError(t, db.EnableSchedule(ctx, schedule.ID))
	enabled, err := db.GetSchedule(ctx, schedule.ID)
	require.NoError(t, err)
	assert.True(t, enabled.Enabled)

	// Delete.
	require.NoError(t, db.DeleteSchedule(ctx, schedule.ID))
	_, err = db.GetSchedule(ctx, schedule.ID)
	require.Error(t, err)
}

func TestScheduleRepository_GetSchedule_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.GetSchedule(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestScheduleRepository_UpdateSchedule_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.UpdateSchedule(context.Background(), uuid.New(), map[string]interface{}{
		"name": "ghost",
	})
	require.Error(t, err)
}

func TestScheduleRepository_DeleteSchedule_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	err := db.DeleteSchedule(context.Background(), uuid.New())
	require.Error(t, err)
}

// ── Discovery job repository ──────────────────────────────────────────────────

func TestDiscoveryJobRepository_CRUD(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Create.
	job, err := db.CreateDiscoveryJob(ctx, map[string]interface{}{
		"networks": []string{"10.10.0.0/24"},
		"method":   "tcp",
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.NotEqual(t, uuid.Nil, job.ID)
	assert.Equal(t, "pending", job.Status)
	t.Cleanup(func() { _ = db.DeleteDiscoveryJob(ctx, job.ID) })

	// Get.
	got, err := db.GetDiscoveryJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, "tcp", got.Method)

	// List.
	jobs, total, err := db.ListDiscoveryJobs(ctx, DiscoveryFilters{}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	var found bool
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	assert.True(t, found)

	// List with filters.
	filtered, _, err := db.ListDiscoveryJobs(ctx, DiscoveryFilters{Status: "pending"}, 0, 100)
	require.NoError(t, err)
	for _, j := range filtered {
		assert.Equal(t, "pending", j.Status)
	}

	// Start → running.
	require.NoError(t, db.StartDiscoveryJob(ctx, job.ID))
	running, err := db.GetDiscoveryJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", running.Status)

	// Stop → failed.
	require.NoError(t, db.StopDiscoveryJob(ctx, job.ID))
	stopped, err := db.GetDiscoveryJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stopped.Status)

	// Update.
	now := time.Now().UTC()
	updated, err := db.UpdateDiscoveryJob(ctx, job.ID, map[string]interface{}{
		"hosts_discovered": 5,
		"hosts_responsive": 3,
		"completed_at":     now,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, updated.HostsDiscovered)
	assert.Equal(t, 3, updated.HostsResponsive)

	// Delete.
	require.NoError(t, db.DeleteDiscoveryJob(ctx, job.ID))
	_, err = db.GetDiscoveryJob(ctx, job.ID)
	require.Error(t, err)
}

func TestDiscoveryJobRepository_GetDiscoveryJob_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.GetDiscoveryJob(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestDiscoveryJobRepository_DeleteDiscoveryJob_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	err := db.DeleteDiscoveryJob(context.Background(), uuid.New())
	require.Error(t, err)
}

// ── Host repository (extended) ────────────────────────────────────────────────

func TestHostRepository_GetActiveHosts(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.50")}
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.50")

	repo := NewHostRepository(db)
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:        uuid.New(),
		IPAddress: ip,
		Status:    HostStatusUp,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.50")
	})

	hosts, err := repo.GetActiveHosts(ctx)
	require.NoError(t, err)

	var found bool
	for _, h := range hosts {
		if h.IPAddress.String() == "203.0.113.50" {
			found = true
			assert.Equal(t, HostStatusUp, h.Status)
			break
		}
	}
	assert.True(t, found, "newly inserted host should appear in GetActiveHosts")
}

func TestHostRepository_ListHosts(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	ip := IPAddr{IP: net.ParseIP("203.0.113.51")}
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.51")

	repo := NewHostRepository(db)
	require.NoError(t, repo.CreateOrUpdate(ctx, &Host{
		ID:        uuid.New(),
		IPAddress: ip,
		Status:    HostStatusUp,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.51")
	})

	hosts, total, err := db.ListHosts(ctx, &HostFilters{}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.GreaterOrEqual(t, len(hosts), 1)
}

func TestHostRepository_CreateGetUpdateDelete(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.52")

	// CreateHost.
	host, err := db.CreateHost(ctx, map[string]interface{}{
		"ip_address": "203.0.113.52",
		"status":     "up",
	})
	require.NoError(t, err)
	require.NotNil(t, host)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", "203.0.113.52")
	})

	// GetHost.
	got, err := db.GetHost(ctx, host.ID)
	require.NoError(t, err)
	assert.Equal(t, host.ID, got.ID)
	assert.Equal(t, "up", got.Status)

	// UpdateHost.
	updated, err := db.UpdateHost(ctx, host.ID, map[string]interface{}{
		"status": "down",
	})
	require.NoError(t, err)
	assert.Equal(t, "down", updated.Status)

	// DeleteHost.
	require.NoError(t, db.DeleteHost(ctx, host.ID))
	_, err = db.GetHost(ctx, host.ID)
	require.Error(t, err)
}

func TestHostRepository_GetHost_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	_, err := db.GetHost(context.Background(), uuid.New())
	require.Error(t, err)
}

// ── NetworkSummary repository ─────────────────────────────────────────────────

func TestNetworkSummaryRepository_GetAll(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	repo := NewNetworkSummaryRepository(db)

	// network_summary is a view — just verify it doesn't error.
	summaries, err := repo.GetAll(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, summaries)
}

// ── Additional ScanRepository tests ──────────────────────────────────────────

func TestScanRepository_CreateScan_ReuseNetwork(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	scan1, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "reuse-network-test-1",
		"targets":   []string{"10.201.0.1"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	require.NotNil(t, scan1)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan1.ID) })

	var networkID1 uuid.UUID
	err = db.QueryRowContext(ctx,
		"SELECT network_id FROM scan_jobs WHERE id = $1", scan1.ID).Scan(&networkID1)
	require.NoError(t, err)

	scan2, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "reuse-network-test-2",
		"targets":   []string{"10.201.0.1"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	require.NotNil(t, scan2)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan2.ID) })

	var networkID2 uuid.UUID
	err = db.QueryRowContext(ctx,
		"SELECT network_id FROM scan_jobs WHERE id = $1", scan2.ID).Scan(&networkID2)
	require.NoError(t, err)

	assert.Equal(t, networkID1, networkID2, "both scan jobs should reference the same network (CIDR reuse path)")
}

func TestScanRepository_CreateScan_NameCollision(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Pre-insert a network whose name will collide with the scan name below.
	collisionNetID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', false, false, 0, '80', 'connect'
		)`,
		collisionNetID, "collision-name-test", "10.250.0.0/24")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", collisionNetID)
	})

	// CreateScan with the same name — findOrCreateNetwork should fall back to the CIDR as the name.
	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "collision-name-test",
		"targets":   []string{"10.250.100.5"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	require.NotNil(t, scan)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	// GetScan reads n.name from the DB — it should be the CIDR fallback.
	got, err := db.GetScan(ctx, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, "10.250.100.5", got.Name)
}

func TestScanRepository_CreateScan_MultipleTargets(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "multi-target-test",
		"targets":   []string{"10.202.0.1", "10.202.0.2"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	require.NotNil(t, scan)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM scan_jobs WHERE created_at >= $1", scan.CreatedAt)
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE cidr IN ('10.202.0.1/32','10.202.0.2/32')")
	})

	var count int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM scan_jobs WHERE created_at >= $1", scan.CreatedAt).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 2)
}

func TestScanRepository_ListScans_WithScanTypeFilter(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	scan, err := db.CreateScan(ctx, map[string]interface{}{
		"name":      "filter-scan-type-test",
		"targets":   []string{"10.203.0.1"},
		"scan_type": "connect",
		"ports":     "80",
	})
	require.NoError(t, err)
	require.NotNil(t, scan)
	t.Cleanup(func() { _ = db.DeleteScan(ctx, scan.ID) })

	scans, total, err := db.ListScans(ctx, ScanFilters{ScanType: "connect"}, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.GreaterOrEqual(t, len(scans), 1)

	var found bool
	for _, s := range scans {
		if s.ID == scan.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "created scan should appear in ListScans with scan_type filter")
}

// ── Additional HostRepository tests ──────────────────────────────────────────

func TestHostRepository_GetHostScans(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Delete any leftover host from a previous run.
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = '203.0.113.60'::inet")

	// Insert host — register cleanup first so it runs last (LIFO).
	hostRepo := NewHostRepository(db)
	hostID := uuid.New()
	ip := IPAddr{IP: net.ParseIP("203.0.113.60")}
	require.NoError(t, hostRepo.CreateOrUpdate(ctx, &Host{
		ID:        hostID,
		IPAddress: ip,
		Status:    HostStatusUp,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = '203.0.113.60'::inet")
	})

	// Insert network.
	networkID := uuid.New()
	_, err := db.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			discovery_method, is_active, scan_enabled,
			scan_interval_seconds, scan_ports, scan_type
		) VALUES (
			$1, $2, $3,
			'tcp', false, false, 0, '80', 'connect'
		)`,
		networkID, "host-scans-test-net", "203.0.113.60/32")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM networks WHERE id = $1", networkID)
	})

	// Insert scan job — register cleanup last so it runs first (LIFO).
	jobRepo := NewScanJobRepository(db)
	jobID := uuid.New()
	require.NoError(t, jobRepo.Create(ctx, &ScanJob{
		ID:        jobID,
		NetworkID: networkID,
		Status:    ScanJobStatusPending,
	}))
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", jobID)
	})

	scans, total, err := db.GetHostScans(ctx, hostID, 0, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.GreaterOrEqual(t, len(scans), 1)
	assert.Equal(t, jobID, scans[0].ID)
}

// ── Migrator tests ────────────────────────────────────────────────────────────

func TestMigrator_Reset(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()

	ctx := context.Background()

	m := NewMigrator(db.DB)

	// Reset drops all schema objects and immediately re-applies all migrations
	// via an internal Up() call, so the schema is fully restored on return.
	// Register a safety-net cleanup first in case the test panics mid-way.
	t.Cleanup(func() { _ = m.Up(ctx) })

	require.NoError(t, m.Reset(ctx))

	// After Reset the internal Up() has re-created every table.
	// Verify the networks table is present and all migrations are recorded.
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT FROM information_schema.tables WHERE table_name='networks' AND table_schema='public')").
		Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists, "networks table should exist after Reset (Up is called internally)")

	// All migrations should be present in schema_migrations.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "schema_migrations should be populated after Reset")
}
