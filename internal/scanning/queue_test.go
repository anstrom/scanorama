package scanning

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ----------------------------------------------------------------

// newTestConfig returns a minimal valid ScanConfig for use in queue tests.
func newTestConfig() *ScanConfig {
	return &ScanConfig{
		Targets:    []string{"127.0.0.1"},
		Ports:      "80",
		ScanType:   "connect",
		TimeoutSec: 5,
	}
}

// newTestRequest builds a ScanQueueRequest with a unique ID and an optional
// result channel. If resultCh is nil one is created automatically.
func newTestRequest(id string, resultCh chan<- *ScanQueueResult) *ScanQueueRequest {
	if resultCh == nil {
		ch := make(chan *ScanQueueResult, 1)
		resultCh = ch
	}
	return &ScanQueueRequest{
		ID:       id,
		Config:   newTestConfig(),
		Database: nil,
		ResultCh: resultCh,
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
		assert.Equal(t, int64(0), stats.TotalSubmitted, "TotalSubmitted should start at zero")
		assert.Equal(t, int64(0), stats.TotalCompleted, "TotalCompleted should start at zero")
		assert.Equal(t, int64(0), stats.TotalRejected, "TotalRejected should start at zero")
		assert.Equal(t, int64(0), stats.TotalFailed, "TotalFailed should start at zero")
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
	req := newTestRequest("before-start-1", nil)
	err := q.Submit(req)

	if err != nil {
		// If the implementation requires Start first, it must return a
		// well-defined error (not a panic).
		assert.Error(t, err, "Submit before Start should return a meaningful error if not buffered")
		t.Logf("Submit before Start returned error (acceptable): %v", err)
	} else {
		// Items were buffered – verify stats reflect the submission.
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
	err := q.Submit(newTestRequest("after-stop-1", nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueClosed, "Submit after Stop must return ErrQueueClosed")

	// TotalRejected should have been incremented.
	stats := q.Stats()
	assert.Equal(t, int64(1), stats.TotalRejected,
		"Rejected submission should increment TotalRejected")
}

func TestScanQueue_QueueFull(t *testing.T) {
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

	firstErr := q.Submit(newTestRequest("full-1", nil))
	if firstErr != nil {
		// Implementation needs Start – use a blocking approach instead.
		// Re-create the queue and start with a context we control.
		q = NewScanQueue(maxConcurrent, maxQueue)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Override the internal scan function to block so items stay queued.
		// If scanFunc is exported or settable, set it; otherwise we rely on
		// the fact that with maxConcurrent=1 and a blocking first scan, the
		// remaining items sit in the queue buffer.
		blockCh := make(chan struct{})
		if setter, ok := interface{}(q).(interface {
			SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
		}); ok {
			setter.SetScanFunc(func(_ context.Context, req *ScanQueueRequest) *ScanQueueResult {
				<-blockCh // block until test is done
				return &ScanQueueResult{ID: req.ID}
			})
		}

		q.Start(ctx)

		// Fill: one item will be picked up by the worker (blocking), the
		// rest should land in the queue buffer.
		for i := 0; i < maxQueue+maxConcurrent; i++ {
			err := q.Submit(newTestRequest("full-blocking-"+string(rune('A'+i)), nil))
			require.NoError(t, err, "submit %d should succeed while queue has room", i)
		}

		// Give workers a moment to pick up items.
		time.Sleep(50 * time.Millisecond)

		// This submit should exceed the capacity.
		err := q.Submit(newTestRequest("overflow", nil))
		require.Error(t, err, "Submit should fail when queue is full")
		assert.ErrorIs(t, err, ErrQueueFull, "Error should be ErrQueueFull")

		stats := q.Stats()
		assert.GreaterOrEqual(t, stats.TotalRejected, int64(1),
			"Rejected count should reflect the overflow")

		close(blockCh) // unblock workers
		cancel()
		q.Stop()
		return
	}

	// If we get here, Submit works without Start (buffered channel approach).
	err := q.Submit(newTestRequest("full-2", nil))
	require.NoError(t, err, "second submit should succeed – queue size is %d", maxQueue)

	// Queue is now at capacity.
	err = q.Submit(newTestRequest("full-overflow", nil))
	require.Error(t, err, "Submit should fail when queue is full")
	assert.ErrorIs(t, err, ErrQueueFull, "Error should be ErrQueueFull")

	stats := q.Stats()
	assert.GreaterOrEqual(t, stats.TotalRejected, int64(1))
}

func TestScanQueue_Stats(t *testing.T) {
	q := NewScanQueue(2, 10)
	require.NotNil(t, q)

	resultCh := make(chan *ScanQueueResult, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wire up a fast no-op scan function if the queue supports it.
	if setter, ok := interface{}(q).(interface {
		SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
	}); ok {
		setter.SetScanFunc(func(_ context.Context, req *ScanQueueRequest) *ScanQueueResult {
			return &ScanQueueResult{
				ID:       req.ID,
				Result:   &ScanResult{},
				Duration: time.Millisecond,
			}
		})
	}

	q.Start(ctx)

	const numJobs = 5
	for i := 0; i < numJobs; i++ {
		req := newTestRequest("stats-"+string(rune('A'+i)), chan<- *ScanQueueResult(resultCh))
		err := q.Submit(req)
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

	// Scan function that takes a bit of time to simulate work.
	if setter, ok := interface{}(q).(interface {
		SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
	}); ok {
		setter.SetScanFunc(func(ctx context.Context, req *ScanQueueRequest) *ScanQueueResult {
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
			}
			atomic.AddInt64(&completed, 1)
			return &ScanQueueResult{ID: req.ID, Result: &ScanResult{}, Duration: 50 * time.Millisecond}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)

	resultCh := make(chan *ScanQueueResult, numJobs)
	for i := 0; i < numJobs; i++ {
		req := newTestRequest("shutdown-"+string(rune('A'+i)), chan<- *ScanQueueResult(resultCh))
		err := q.Submit(req)
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

	// Scan function that tracks concurrency.
	if setter, ok := interface{}(q).(interface {
		SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
	}); ok {
		setter.SetScanFunc(func(ctx context.Context, req *ScanQueueRequest) *ScanQueueResult {
			cur := atomic.AddInt64(&active, 1)

			peakMu.Lock()
			if cur > peakSeen {
				peakSeen = cur
			}
			peakMu.Unlock()

			// Hold the slot for a bit so concurrency can build up.
			select {
			case <-time.After(30 * time.Millisecond):
			case <-ctx.Done():
			}

			atomic.AddInt64(&active, -1)
			atomic.AddInt64(&completed, 1)
			return &ScanQueueResult{ID: req.ID, Result: &ScanResult{}, Duration: 30 * time.Millisecond}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	resultCh := make(chan *ScanQueueResult, numJobs)
	for i := 0; i < numJobs; i++ {
		req := newTestRequest("conc-"+string(rune('A'+i)), chan<- *ScanQueueResult(resultCh))
		err := q.Submit(req)
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

	// Scan function that signals it started and then blocks until context is
	// cancelled.
	if setter, ok := interface{}(q).(interface {
		SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
	}); ok {
		setter.SetScanFunc(func(ctx context.Context, req *ScanQueueRequest) *ScanQueueResult {
			select {
			case scanStarted <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
			case <-scanBlocked:
			}
			return &ScanQueueResult{ID: req.ID, Result: &ScanResult{}, Error: ctx.Err()}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)

	err := q.Submit(newTestRequest("ctx-cancel-1", nil))
	require.NoError(t, err)

	// Wait until the scan function has actually started.
	select {
	case <-scanStarted:
	case <-time.After(2 * time.Second):
		// If scanFunc is not overridden the scan may finish (or fail) before
		// we get here – that's acceptable.
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
	err = q.Submit(newTestRequest("ctx-cancel-2", nil))
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

	if setter, ok := interface{}(q).(interface {
		SetScanFunc(func(context.Context, *ScanQueueRequest) *ScanQueueResult)
	}); ok {
		setter.SetScanFunc(func(_ context.Context, req *ScanQueueRequest) *ScanQueueResult {
			time.Sleep(5 * time.Millisecond) // tiny delay
			return &ScanQueueResult{ID: req.ID, Result: &ScanResult{}, Duration: 5 * time.Millisecond}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	resultCh := make(chan *ScanQueueResult, numJobs)
	for i := 0; i < numJobs; i++ {
		req := newTestRequest("integrity-"+string(rune('A'+(i%26))), chan<- *ScanQueueResult(resultCh))
		err := q.Submit(req)
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
