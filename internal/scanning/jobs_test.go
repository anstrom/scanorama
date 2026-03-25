// Package scanning provides core scanning functionality and shared types for scanorama.
package scanning

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ─── ScanJob ─────────────────────────────────────────────────────────────────

func TestScanJob_ID(t *testing.T) {
	job := NewScanJob("scan-abc", nil, nil, nil, nil)
	assert.Equal(t, "scan-abc", job.ID())
}

func TestScanJob_Type(t *testing.T) {
	job := NewScanJob("x", nil, nil, nil, nil)
	assert.Equal(t, "scan", job.Type())
}

func TestScanJob_Target_SingleTarget(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{"192.168.1.1"}, Ports: "80", ScanType: "connect"}
	job := NewScanJob("x", cfg, nil, nil, nil)
	assert.Equal(t, "192.168.1.1", job.Target())
}

func TestScanJob_Target_MultipleTargets(t *testing.T) {
	cfg := &ScanConfig{
		Targets:  []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		Ports:    "22",
		ScanType: "connect",
	}
	job := NewScanJob("x", cfg, nil, nil, nil)
	got := job.Target()
	assert.Equal(t, "10.0.0.1, 10.0.0.2, 10.0.0.3", got)
	assert.True(t, strings.Contains(got, ", "), "targets should be separated by ', '")
}

func TestScanJob_Target_NilConfig(t *testing.T) {
	job := NewScanJob("x", nil, nil, nil, nil)
	assert.Equal(t, "", job.Target())
}

func TestScanJob_Target_EmptyTargets(t *testing.T) {
	cfg := &ScanConfig{Targets: []string{}, Ports: "80", ScanType: "connect"}
	job := NewScanJob("x", cfg, nil, nil, nil)
	assert.Equal(t, "", job.Target())
}

func TestScanJob_Execute_CallsExecutorAndOnDone(t *testing.T) {
	wantResult := &ScanResult{Hosts: []Host{{Address: "10.0.0.1", Status: "up"}}}
	executorCalled := false
	onDoneCalled := false
	var gotResult *ScanResult
	var gotErr error

	job := NewScanJob(
		"exec-1",
		&ScanConfig{Targets: []string{"10.0.0.1"}, Ports: "80", ScanType: "connect"},
		nil,
		func(_ context.Context, _ *ScanConfig, _ *db.DB) (*ScanResult, error) {
			executorCalled = true
			return wantResult, nil
		},
		func(result *ScanResult, err error) {
			onDoneCalled = true
			gotResult = result
			gotErr = err
		},
	)

	err := job.Execute(context.Background())

	require.NoError(t, err)
	assert.True(t, executorCalled, "executor must be called")
	assert.True(t, onDoneCalled, "onDone must be called")
	assert.Equal(t, wantResult, gotResult)
	assert.NoError(t, gotErr)
}

func TestScanJob_Execute_PropagatesExecutorError(t *testing.T) {
	wantErr := errors.New("nmap: binary not found")
	var gotErr error

	job := NewScanJob(
		"exec-err",
		nil,
		nil,
		func(_ context.Context, _ *ScanConfig, _ *db.DB) (*ScanResult, error) {
			return nil, wantErr
		},
		func(_ *ScanResult, err error) {
			gotErr = err
		},
	)

	err := job.Execute(context.Background())

	assert.ErrorIs(t, err, wantErr)
	assert.ErrorIs(t, gotErr, wantErr, "onDone must receive the same error")
}

func TestScanJob_Execute_NilOnDone_DoesNotPanic(t *testing.T) {
	job := NewScanJob(
		"exec-nil-done",
		nil,
		nil,
		func(_ context.Context, _ *ScanConfig, _ *db.DB) (*ScanResult, error) {
			return &ScanResult{}, nil
		},
		nil,
	)
	assert.NotPanics(t, func() { _ = job.Execute(context.Background()) })
}

func TestScanJob_Execute_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	job := NewScanJob(
		"exec-ctx",
		nil,
		nil,
		func(ctx context.Context, _ *ScanConfig, _ *db.DB) (*ScanResult, error) {
			return nil, ctx.Err()
		},
		nil,
	)

	err := job.Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// ─── Job interface compliance ─────────────────────────────────────────────────

// TestScanJob_ImplementsJobInterface verifies that *ScanJob satisfies the Job
// interface at compile time via a type assertion.
func TestScanJob_ImplementsJobInterface(t *testing.T) {
	var _ Job = (*ScanJob)(nil)
}
