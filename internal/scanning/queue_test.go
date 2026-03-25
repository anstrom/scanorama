package scanning

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ----------------------------------------------------------------

// mockJob is a minimal Job implementation used throughout queue tests.
// The execute function controls what happens when the job runs; onDone is
// called (if non-nil) after execution so tests can synchronize on completion.
type mockJob struct {
	id      string
	jobType string
	target  string
	execute func(context.Context) error
	onDone  func(err error)
}

func (m *mockJob) ID() string     { return m.id }
func (m *mockJob) Type() string   { return m.jobType }
func (m *mockJob) Target() string { return m.target }
func (m *mockJob) Execute(ctx context.Context) error {
	err := m.execute(ctx)
	if m.onDone != nil {
		m.onDone(err)
	}
	return err
}

// newSuccessJob builds a mockJob that completes immediately without error.
func newSuccessJob(id string) *mockJob {
	return &mockJob{
		id:      id,
		jobType: "scan",
		target:  "127.0.0.1",
		execute: func(_ context.Context) error {
			return nil
		},
	}
}

// newBlockingJob builds a mockJob that signals started then blocks on unblock.
func newBlockingJob(id string, started, unblock chan struct{}) *mockJob {
	var once sync.Once
	return &mockJob{
		id:      id,
		jobType: "scan",
		target:  "127.0.0.1",
		execute: func(ctx context.Context) error {
			once.Do(func() { close(started) })
			select {
			case <-unblock:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

// waitForCondition polls fn every tick until it returns true or the timeout
// elapses. Returns whether the condition was met.
func waitForCondition(timeout, tick time.Duration, fn func() bool) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		if fn() {
			return true
		}
		select {
		case <-deadline:
			return fn() // one last check
		case <-ticker.C:
		}
	}
}

// --- tests ------------------------------------------------------------------

func TestNewScanQueue(t *testing.T) {
	t.Run("custom values", func(t *testing.T) {
		q := NewScanQueue(4, 100)
		require.NotNil(t, q)

		stats := q.Stats()
		assert.Equal(t, 4, stats.MaxConcurrent, "MaxConcurrent should match constructor arg")
		assert.Equal(t, 100, stats.MaxQueueSize, "MaxQueueSize should match constructor arg")
		assert.Equal(t, 0, stats.QueueDepth, "QueueDepth should start at zero")
		assert.Equal(t, 0, stats.ActiveScans, "ActiveScans should start at zero")
		assert.Equal(t, int64(0), stats.TotalSubmitted)
		assert.Equal(t, int64(0), stats.TotalCompleted)
		assert.Equal(t, int64(0), stats.TotalRejected)
		assert.Equal(t, int64(0), stats.TotalFailed)
	})

	t.Run("minimum / default values", func(t *testing.T) {
		// Passing 0 or negative values – the constructor should still produce a
		// usable queue (typically clamped to 1).
		q := NewScanQueue(0, 0)
		require.NotNil(t, q)

		stats := q.Stats()
		assert.GreaterOrEqual(t, stats.MaxConcurrent, 1,
			"MaxConcurrent should be at least 1 even when 0 is requested")
		assert.GreaterOrEqual(t, stats.MaxQueueSize, 1,
			"MaxQueueSize should be at least 1 even when 0 is requested")
	})

	t.Run("negative values handled gracefully", func(t *testing.T) {
		q := NewScanQueue(-5, -10)
		require.NotNil(t, q)

		stats := q.Stats()
		assert.GreaterOrEqual(t, stats.MaxConcurrent, 1)
		assert.GreaterOrEqual(t, stats.MaxQueueSize, 1)
	})
}

func TestScanQueue_SubmitBeforeStart(t *testing.T) {
	q := NewScanQueue(2, 10)
	require.NotNil(t, q)

	// Submit before calling Start – items should either be buffered or we get
	// a defined error. Both are acceptable; a panic or deadlock is not.
	job := newSuccessJob("before-start-1")
	err := q.Submit(job)

	if err != nil {
		assert.Error(t, err, "Submit before Start should return a meaningful error if not buffered")
		t.Logf("Submit before Start returned error (acceptable): %v", err)
	} else {
		stats := q.Stats()
		assert.Equal(t, int64(1), stats.TotalSubmitted,
			"Buffered submit should increment TotalSubmitted")
		assert.GreaterOrEqual(t, stats.QueueDepth, 1,
			"QueueDepth should reflect the buffered item")
	}

	// Clean-up: start + stop to drain anything that was buffered so goroutines
	// don't leak.
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	cancel()
	q.Stop()
}

func TestScanQueue_SubmitAfterStop(t *testing.T) {
	q := NewScanQueue(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	cancel()
	q.Stop()

	// After Stop, submitting should return ErrQueueClosed.
	err := q.Submit(newSuccessJob("after-stop-1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueClosed, "Submit after Stop must return ErrQueueClosed")

	// TotalRejected should have been incremented.
	stats := q.Stats()
	assert.Equal(t, int64(1), stats.TotalRejected,
		"Rejected submission should increment TotalRejected")
}

func TestScanQueue_QueueFull(t *testing.T) { //nolint:cyclop
	const maxQueue = 2
	const maxConcurrent = 1

	q := NewScanQueue(maxConcurrent, maxQueue)
	require.NotNil(t, q)

	// We do NOT start the queue so that nothing is consumed – items simply
	// accumulate in the buffer until it is full.  If the implementation
	// requires Start for Submit to work, we start it but use a scanFunc that
	// blocks forever so items pile up.
	//
	// Strategy: try without Start first. If Submit fails with a non-full
	// error, start the queue with a blocking scanFunc override.

	// Submit a blocking job first and wait until the worker picks it up,
	// so the worker slot is provably occupied before we fill the buffer.
	workerStarted := make(chan struct{})
	blockCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.Start(ctx)

	firstJob := newBlockingJob("full-blocking-first", workerStarted, blockCh)
	require.NoError(t, q.Submit(firstJob), "first job must be accepted")

	// Wait until the worker goroutine has actually picked up the first job.
	select {
	case <-workerStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start the first job in time")
	}

	// Now fill the queue buffer (maxQueue slots).
	for i := 0; i < maxQueue; i++ {
		job := newBlockingJob("full-blocking-"+string(rune('A'+i)), make(chan struct{}), blockCh)
		err := q.Submit(job)
		require.NoError(t, err, "submit %d should succeed while buffer has room", i)
	}

	// This submit should exceed the buffer capacity.
	err := q.Submit(newSuccessJob("overflow"))
	require.Error(t, err, "Submit should fail when queue is full")
	assert.ErrorIs(t, err, ErrQueueFull, "Error should be ErrQueueFull")

	stats := q.Stats()
	assert.GreaterOrEqual(t, stats.TotalRejected, int64(1),
		"Rejected count should reflect the overflow")

	close(blockCh) // unblock workers
}

func TestScanQueue_Stats(t *testing.T) {
	q := NewScanQueue(2, 10)
	require.NotNil(t, q)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	const numJobs = 5
	for i := 0; i < numJobs; i++ {
		err := q.Submit(newSuccessJob("stats-" + string(rune('A'+i))))
		require.NoError(t, err)
	}

	// Wait for all results (with timeout).
	ok := waitForCondition(5*time.Second, 25*time.Millisecond, func() bool {
		s := q.Stats()
		return s.TotalCompleted+s.TotalFailed >= numJobs
	})
	require.True(t, ok, "timed out waiting for jobs to complete")

	stats := q.Stats()
	assert.Equal(t, int64(numJobs), stats.TotalSubmitted,
		"TotalSubmitted should equal the number of submissions")
	assert.Equal(t, int64(numJobs), stats.TotalCompleted+stats.TotalFailed,
		"TotalCompleted+TotalFailed should equal submissions")
	assert.Equal(t, int64(0), stats.TotalRejected,
		"No submissions should have been rejected")
	assert.Equal(t, 0, stats.ActiveScans,
		"No scans should be active after all complete")
	assert.Equal(t, 0, stats.QueueDepth,
		"Queue should be drained")

	cancel()
	q.Stop()
}

func TestScanQueue_GracefulShutdown(t *testing.T) {
	const numJobs = 3
	q := NewScanQueue(1, 10)
	require.NotNil(t, q)

	var completed int64

	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)

	for i := 0; i < numJobs; i++ {
		job := &mockJob{
			id: "shutdown-" + string(rune('A'+i)), jobType: "scan", target: "127.0.0.1",
			execute: func(ctx context.Context) error {
				select {
				case <-time.After(50 * time.Millisecond):
				case <-ctx.Done():
				}
				atomic.AddInt64(&completed, 1)
				return nil
			},
		}
		err := q.Submit(job)
		require.NoError(t, err)
	}

	// Give a moment for at least one scan to start.
	time.Sleep(20 * time.Millisecond)

	// Signal shutdown.
	cancel()

	// Stop should block until in-flight work finishes.
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned – good.
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds – possible deadlock")
	}

	stats := q.Stats()
	// After graceful shutdown the completed + failed count should account for
	// everything that was actively being processed when Stop was called.
	// At a minimum the in-flight scan(s) must have finished.
	assert.GreaterOrEqual(t, stats.TotalCompleted+stats.TotalFailed, int64(1),
		"At least the in-flight scan should have completed during graceful shutdown")
}

func TestScanQueue_ConcurrencyLimit(t *testing.T) {
	const maxConcurrent = 3
	const numJobs = 10

	q := NewScanQueue(maxConcurrent, numJobs+5)
	require.NotNil(t, q)

	var (
		active    int64
		peakSeen  int64
		peakMu    sync.Mutex
		completed int64
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	for i := 0; i < numJobs; i++ {
		job := &mockJob{
			id: "conc-" + string(rune('A'+i)), jobType: "scan", target: "127.0.0.1",
			execute: func(ctx context.Context) error {
				cur := atomic.AddInt64(&active, 1)
				peakMu.Lock()
				if cur > peakSeen {
					peakSeen = cur
				}
				peakMu.Unlock()
				select {
				case <-time.After(30 * time.Millisecond):
				case <-ctx.Done():
				}
				atomic.AddInt64(&active, -1)
				atomic.AddInt64(&completed, 1)
				return nil
			},
		}
		err := q.Submit(job)
		require.NoError(t, err)
	}

	// Wait for all work to finish.
	ok := waitForCondition(10*time.Second, 25*time.Millisecond, func() bool {
		return atomic.LoadInt64(&completed) >= numJobs
	})
	require.True(t, ok, "timed out waiting for all jobs to finish")

	peakMu.Lock()
	peak := peakSeen
	peakMu.Unlock()

	assert.LessOrEqual(t, peak, int64(maxConcurrent),
		"Peak active scans (%d) must not exceed maxConcurrent (%d)", peak, maxConcurrent)
	assert.GreaterOrEqual(t, peak, int64(1),
		"At least one scan should have been active")

	// With enough jobs and a holding time, we expect the queue to actually
	// use more than one concurrent slot (but this is an optimistic check –
	// scheduling is non-deterministic).
	if peak < int64(maxConcurrent) {
		t.Logf("Note: peak concurrency was %d (max allowed %d); scheduling may vary", peak, maxConcurrent)
	}

	cancel()
	q.Stop()
}

func TestScanQueue_ContextCancellation(t *testing.T) {
	q := NewScanQueue(2, 10)
	require.NotNil(t, q)

	scanStarted := make(chan struct{}, 1)
	scanBlocked := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)

	job := &mockJob{
		id: "ctx-cancel-1", jobType: "scan", target: "127.0.0.1",
		execute: func(ctx context.Context) error {
			select {
			case scanStarted <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-scanBlocked:
				return nil
			}
		},
	}
	err := q.Submit(job)
	require.NoError(t, err)

	// Wait until the job has actually started.
	select {
	case <-scanStarted:
	case <-time.After(2 * time.Second):
		// Job may finish quickly without the mock — that's acceptable.
	}

	// Cancel the parent context.
	cancel()

	// The queue should shut down promptly.
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good – Stop returned.
	case <-time.After(5 * time.Second):
		close(scanBlocked) // unblock just in case
		t.Fatal("Stop() did not return within 5 seconds after context cancellation")
	}

	// After cancellation + Stop, further submits should be rejected.
	err = q.Submit(newSuccessJob("ctx-cancel-2"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueClosed,
		"Submit after context cancellation + Stop should return ErrQueueClosed")
}

// TestScanQueue_StatsIntegrity verifies that stat counters remain consistent
// under concurrent submissions and completions.
func TestScanQueue_StatsIntegrity(t *testing.T) {
	const maxConcurrent = 4
	const numJobs = 20

	q := NewScanQueue(maxConcurrent, numJobs+5)
	require.NotNil(t, q)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	for i := 0; i < numJobs; i++ {
		job := &mockJob{
			id: "integrity-" + string(rune('A'+(i%26))), jobType: "scan", target: "127.0.0.1",
			execute: func(_ context.Context) error {
				time.Sleep(5 * time.Millisecond)
				return nil
			},
		}
		err := q.Submit(job)
		require.NoError(t, err)
	}

	ok := waitForCondition(10*time.Second, 25*time.Millisecond, func() bool {
		s := q.Stats()
		return s.TotalCompleted+s.TotalFailed >= numJobs
	})
	require.True(t, ok, "timed out waiting for jobs")

	stats := q.Stats()
	assert.Equal(t, int64(numJobs), stats.TotalSubmitted)
	assert.Equal(t, int64(numJobs), stats.TotalCompleted+stats.TotalFailed)
	assert.Equal(t, int64(0), stats.TotalRejected)
	assert.Equal(t, 0, stats.QueueDepth)
	assert.Equal(t, 0, stats.ActiveScans)

	cancel()
	q.Stop()
}

// TestScanQueue_MultipleStopCalls ensures calling Stop multiple times does not
// panic or deadlock.
func TestScanQueue_MultipleStopCalls(t *testing.T) {
	q := NewScanQueue(1, 5)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	cancel()

	// First stop – should work normally.
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("First Stop() call did not return in time")
	}

	// Second stop – must not panic or deadlock.
	done2 := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Second Stop() panicked: %v", r)
			}
			close(done2)
		}()
		q.Stop()
	}()
	select {
	case <-done2:
	case <-time.After(3 * time.Second):
		t.Fatal("Second Stop() call did not return in time")
	}
}

// ─── Snapshot tests ──────────────────────────────────────────────────────────

func TestScanQueue_Snapshot_BeforeStart(t *testing.T) {
	q := NewScanQueue(3, 20)

	snaps := q.Snapshot()

	assert.Len(t, snaps, 3, "Snapshot should return one entry per worker")
	for i, snap := range snaps {
		assert.Equal(t, fmt.Sprintf("worker-%d", i), snap.ID)
		assert.Equal(t, "idle", snap.Status)
		assert.Empty(t, snap.JobID)
		assert.Empty(t, snap.JobType)
		assert.Empty(t, snap.JobTarget)
		assert.Nil(t, snap.JobStartedAt)
		assert.Zero(t, snap.JobsDone)
		assert.Zero(t, snap.JobsFailed)
	}
}

func TestScanQueue_Snapshot_WorkerStartedAt(t *testing.T) {
	q := NewScanQueue(2, 10)

	before := time.Now()
	q.Start(context.Background())
	defer q.Stop()
	after := time.Now()

	snaps := q.Snapshot()

	require.Len(t, snaps, 2)
	for _, snap := range snaps {
		assert.False(t, snap.WorkerStartedAt.IsZero(), "WorkerStartedAt should be set after Start")
		assert.True(t, !snap.WorkerStartedAt.Before(before) && !snap.WorkerStartedAt.After(after),
			"WorkerStartedAt should be within the window around Start()")
	}
}

func TestScanQueue_Snapshot_ActiveWorker(t *testing.T) {
	q := NewScanQueue(1, 10)

	started := make(chan struct{})
	unblock := make(chan struct{})
	done := make(chan struct{})

	q.Start(context.Background())
	defer q.Stop()

	job := &mockJob{
		id: "snap-active-1", jobType: "scan", target: "192.168.1.1",
		execute: func(ctx context.Context) error {
			close(started)
			select {
			case <-unblock:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		onDone: func(_ error) { close(done) },
	}
	require.NoError(t, q.Submit(job))

	// Wait until the worker has picked up the job.
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start the job in time")
	}

	snaps := q.Snapshot()
	require.Len(t, snaps, 1)
	snap := snaps[0]

	assert.Equal(t, "active", snap.Status)
	assert.Equal(t, "snap-active-1", snap.JobID)
	assert.Equal(t, "scan", snap.JobType)
	assert.Equal(t, "192.168.1.1", snap.JobTarget)
	assert.NotNil(t, snap.JobStartedAt)

	close(unblock)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not complete in time")
	}

	// Worker should be idle again.
	ok := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		return q.Snapshot()[0].Status == "idle"
	})
	assert.True(t, ok, "worker should return to idle after job completes")
}

func TestScanQueue_Snapshot_IdleAfterCompletion(t *testing.T) {
	q := NewScanQueue(2, 10)
	q.Start(context.Background())
	defer q.Stop()

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		job := &mockJob{
			id: fmt.Sprintf("snap-idle-%d", i), jobType: "scan", target: "127.0.0.1",
			execute: func(_ context.Context) error { return nil },
			onDone:  func(_ error) { wg.Done() },
		}
		require.NoError(t, q.Submit(job))
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("jobs did not complete in time")
	}

	ok := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		for _, s := range q.Snapshot() {
			if s.Status != "idle" {
				return false
			}
		}
		return true
	})
	assert.True(t, ok, "all workers should be idle after jobs complete")
}

func TestScanQueue_Snapshot_CountersIncrementOnSuccess(t *testing.T) {
	q := NewScanQueue(1, 10)
	q.Start(context.Background())
	defer q.Stop()

	const jobs = 3
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Add(1)
		job := &mockJob{
			id: fmt.Sprintf("snap-ok-%d", i), jobType: "scan", target: "127.0.0.1",
			execute: func(_ context.Context) error { return nil },
			onDone:  func(_ error) { wg.Done() },
		}
		require.NoError(t, q.Submit(job))
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("jobs did not complete in time")
	}

	ok := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		return q.Snapshot()[0].JobsDone == jobs
	})
	assert.True(t, ok, "JobsDone should reflect completed jobs")

	snap := q.Snapshot()[0]
	assert.Equal(t, int64(jobs), snap.JobsDone)
	assert.Equal(t, int64(0), snap.JobsFailed)
}

func TestScanQueue_Snapshot_CountersIncrementOnFailure(t *testing.T) {
	q := NewScanQueue(1, 10)
	q.Start(context.Background())
	defer q.Stop()

	done := make(chan struct{})
	job := &mockJob{
		id: "snap-fail-1", jobType: "scan", target: "127.0.0.1",
		execute: func(_ context.Context) error { return fmt.Errorf("simulated failure") },
		onDone:  func(_ error) { close(done) },
	}
	require.NoError(t, q.Submit(job))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not complete in time")
	}

	ok := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		return q.Snapshot()[0].JobsFailed == 1
	})
	assert.True(t, ok, "JobsFailed should be 1 after a failed job")

	snap := q.Snapshot()[0]
	assert.Equal(t, int64(0), snap.JobsDone)
	assert.Equal(t, int64(1), snap.JobsFailed)
}

func TestScanQueue_Snapshot_LastJobAt(t *testing.T) {
	q := NewScanQueue(1, 10)
	q.Start(context.Background())
	defer q.Stop()

	assert.True(t, q.Snapshot()[0].LastJobAt.IsZero(), "LastJobAt should be zero before any job")

	before := time.Now()
	done := make(chan struct{})
	require.NoError(t, q.Submit(&mockJob{
		id: "snap-lastjob-1", jobType: "scan", target: "127.0.0.1",
		execute: func(_ context.Context) error { return nil },
		onDone:  func(_ error) { close(done) },
	}))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not complete in time")
	}
	after := time.Now()

	ok := waitForCondition(3*time.Second, 20*time.Millisecond, func() bool {
		return !q.Snapshot()[0].LastJobAt.IsZero()
	})
	require.True(t, ok, "LastJobAt should be set after a completed job")

	lastJobAt := q.Snapshot()[0].LastJobAt
	assert.True(t, !lastJobAt.Before(before) && !lastJobAt.After(after),
		"LastJobAt should fall within the job execution window")
}

func TestScanQueue_Snapshot_MultiTargetJob(t *testing.T) {
	// ScanJob (not mockJob) is used here to verify that Target() joins
	// multiple targets correctly via the real ScanJob implementation.
	q := NewScanQueue(1, 10)

	started := make(chan struct{})
	unblock := make(chan struct{})
	done := make(chan struct{})

	q.Start(context.Background())
	defer q.Stop()

	cfg := &ScanConfig{
		Targets:  []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		Ports:    "22",
		ScanType: "connect",
	}
	var startOnce sync.Once
	job := NewScanJob(
		"snap-multi-1",
		cfg,
		nil,
		func(_ context.Context, _ *ScanConfig, _ *db.DB) (*ScanResult, error) {
			startOnce.Do(func() { close(started) })
			<-unblock
			return &ScanResult{}, nil
		},
		func(_ *ScanResult, _ error) { close(done) },
	)
	require.NoError(t, q.Submit(job))

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start in time")
	}

	snap := q.Snapshot()[0]
	assert.Equal(t, "10.0.0.1, 10.0.0.2, 10.0.0.3", snap.JobTarget,
		"multiple targets should be joined with ', '")

	close(unblock)
	<-done
}
