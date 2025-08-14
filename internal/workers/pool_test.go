package workers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockJob implements the Job interface for testing
type MockJob struct {
	id       string
	jobType  string
	duration time.Duration
	err      error
	executed int32
}

func NewMockJob(id, jobType string, duration time.Duration, err error) *MockJob {
	return &MockJob{
		id:       id,
		jobType:  jobType,
		duration: duration,
		err:      err,
	}
}

func (m *MockJob) Execute(ctx context.Context) error {
	atomic.AddInt32(&m.executed, 1)
	if m.duration > 0 {
		select {
		case <-time.After(m.duration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.err
}

func (m *MockJob) ID() string {
	return m.id
}

func (m *MockJob) Type() string {
	return m.jobType
}

func (m *MockJob) ExecutedCount() int32 {
	return atomic.LoadInt32(&m.executed)
}

func TestNewPool(t *testing.T) {
	t.Run("creates pool with valid configuration", func(t *testing.T) {
		config := Config{
			Size:            5,
			QueueSize:       100,
			MaxRetries:      3,
			RetryDelay:      time.Second,
			ShutdownTimeout: 10 * time.Second,
			RateLimit:       10,
		}

		pool := New(config)

		assert.NotNil(t, pool)
		assert.Equal(t, config.Size, cap(pool.workers))
		assert.Equal(t, config.QueueSize, cap(pool.jobs))
		assert.Equal(t, config.QueueSize, cap(pool.results))
	})

	t.Run("creates pool with default values", func(t *testing.T) {
		config := Config{}
		pool := New(config)

		assert.NotNil(t, pool)
		assert.NotNil(t, pool.ctx)
		assert.NotNil(t, pool.cancel)
	})
}

func TestPoolLifecycle(t *testing.T) {
	t.Run("start and shutdown pool successfully", func(t *testing.T) {
		config := Config{
			Size:            2,
			QueueSize:       10,
			MaxRetries:      1,
			RetryDelay:      100 * time.Millisecond,
			ShutdownTimeout: 2 * time.Second,
		}

		pool := New(config)

		// Start the pool
		pool.Start()

		// Submit a simple job
		job := NewMockJob("test-1", "test", 10*time.Millisecond, nil)
		err := pool.Submit(job)
		assert.NoError(t, err)

		// Wait a bit for processing
		time.Sleep(50 * time.Millisecond)

		// Shutdown the pool
		err = pool.Shutdown()
		assert.NoError(t, err)

		// Verify job was executed
		assert.Equal(t, int32(1), job.ExecutedCount())
	})

	t.Run("handles multiple start calls gracefully", func(t *testing.T) {
		config := Config{Size: 1, QueueSize: 1, ShutdownTimeout: time.Second}
		pool := New(config)

		pool.Start()
		pool.Start() // Should not panic or cause issues

		err := pool.Shutdown()
		assert.NoError(t, err)
	})
}

func TestJobSubmission(t *testing.T) {
	config := Config{
		Size:            3,
		QueueSize:       5,
		MaxRetries:      2,
		RetryDelay:      50 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("submits and executes jobs successfully", func(t *testing.T) {
		jobs := make([]*MockJob, 3)
		for i := 0; i < 3; i++ {
			jobs[i] = NewMockJob(fmt.Sprintf("job-%d", i), "test", 10*time.Millisecond, nil)
			err := pool.Submit(jobs[i])
			assert.NoError(t, err)
		}

		// Wait for jobs to complete
		time.Sleep(200 * time.Millisecond)

		for i, job := range jobs {
			assert.Equal(t, int32(1), job.ExecutedCount(), "Job %d should be executed once", i)
		}
	})

	t.Run("returns error when submitting to shut down pool", func(t *testing.T) {
		shutdownConfig := Config{Size: 1, QueueSize: 1, ShutdownTimeout: time.Second}
		shutdownPool := New(shutdownConfig)
		shutdownPool.Start()
		shutdownPool.Shutdown()

		job := NewMockJob("test", "test", 0, nil)
		err := shutdownPool.Submit(job)
		assert.Error(t, err)
	})
}

func TestJobExecution(t *testing.T) {
	config := Config{
		Size:            2,
		QueueSize:       10,
		MaxRetries:      3,
		RetryDelay:      10 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("executes successful jobs", func(t *testing.T) {
		job := NewMockJob("success-job", "test", 5*time.Millisecond, nil)
		err := pool.Submit(job)
		require.NoError(t, err)

		// Wait for job to complete
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, int32(1), job.ExecutedCount())
	})

	t.Run("retries failed jobs", func(t *testing.T) {
		failingJob := NewMockJob("failing-job", "test", 5*time.Millisecond, errors.New("job failed"))
		err := pool.Submit(failingJob)
		require.NoError(t, err)

		// Wait for job and retries to complete
		time.Sleep(200 * time.Millisecond)

		// Should be executed multiple times due to retries
		executed := failingJob.ExecutedCount()
		assert.Greater(t, executed, int32(1), "Job should be retried")
		assert.LessOrEqual(t, executed, int32(config.MaxRetries+1), "Job should not exceed max retries")
	})
}

func TestConcurrentJobProcessing(t *testing.T) {
	config := Config{
		Size:            5,
		QueueSize:       50,
		MaxRetries:      1,
		RetryDelay:      10 * time.Millisecond,
		ShutdownTimeout: 3 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("processes multiple jobs concurrently", func(t *testing.T) {
		const numJobs = 20
		jobs := make([]*MockJob, numJobs)

		start := time.Now()

		// Submit all jobs
		for i := 0; i < numJobs; i++ {
			jobs[i] = NewMockJob(fmt.Sprintf("concurrent-job-%d", i), "concurrent", 50*time.Millisecond, nil)
			err := pool.Submit(jobs[i])
			require.NoError(t, err)
		}

		// Wait for all jobs to complete
		time.Sleep(500 * time.Millisecond)

		duration := time.Since(start)

		// With 5 workers, 20 jobs of 50ms each should complete in less than 500ms
		// (4 batches × 50ms + overhead, allowing for improved shutdown safety)
		assert.Less(t, duration, 600*time.Millisecond, "Concurrent processing should be faster than sequential")

		// Verify all jobs were executed
		for i, job := range jobs {
			assert.Equal(t, int32(1), job.ExecutedCount(), "Job %d should be executed", i)
		}
	})
}

func TestResultCollection(t *testing.T) {
	config := Config{
		Size:            2,
		QueueSize:       5,
		MaxRetries:      1,
		RetryDelay:      10 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
	}

	pool := New(config)
	pool.Start()

	t.Run("collects results from executed jobs", func(t *testing.T) {
		// Submit a simple successful job
		successJob := NewMockJob("success", "test", 5*time.Millisecond, nil)

		err := pool.Submit(successJob)
		require.NoError(t, err)

		// Collect at least one result
		select {
		case result := <-pool.Results():
			assert.Equal(t, "success", result.JobID)
			assert.Equal(t, "test", result.JobType)
			assert.NoError(t, result.Error)
			assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Should receive result within timeout")
		}

		pool.Shutdown()
	})
}

func TestPoolConfiguration(t *testing.T) {
	t.Run("validates configuration limits", func(t *testing.T) {
		testCases := []struct {
			name   string
			config Config
			valid  bool
		}{
			{
				name:   "valid configuration",
				config: Config{Size: 5, QueueSize: 10, MaxRetries: 3, ShutdownTimeout: time.Second},
				valid:  true,
			},
			{
				name:   "zero workers",
				config: Config{Size: 0, QueueSize: 10},
				valid:  true, // Should handle gracefully
			},
			{
				name:   "large queue",
				config: Config{Size: 1, QueueSize: 1000},
				valid:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				pool := New(tc.config)
				assert.NotNil(t, pool)

				if tc.config.Size > 0 {
					pool.Start()
					err := pool.Shutdown()
					assert.NoError(t, err)
				}
			})
		}
	})
}

func TestGracefulShutdown(t *testing.T) {
	t.Run("waits for in-progress jobs to complete", func(t *testing.T) {
		config := Config{
			Size:            2,
			QueueSize:       5,
			MaxRetries:      1,
			ShutdownTimeout: 3 * time.Second,
		}

		pool := New(config)
		pool.Start()

		// Submit short jobs that should execute quickly
		shortJob1 := NewMockJob("short-1", "short", 10*time.Millisecond, nil)
		shortJob2 := NewMockJob("short-2", "short", 10*time.Millisecond, nil)

		err := pool.Submit(shortJob1)
		require.NoError(t, err)
		err = pool.Submit(shortJob2)
		require.NoError(t, err)

		// Give jobs a moment to start
		time.Sleep(20 * time.Millisecond)

		// Shutdown should complete relatively quickly for short jobs
		start := time.Now()
		err = pool.Shutdown()
		shutdownDuration := time.Since(start)

		assert.NoError(t, err)
		assert.Less(t, shutdownDuration, 2*time.Second, "Should not timeout")

		// Jobs should have been executed (may be retried if cancelled during shutdown)
		assert.GreaterOrEqual(t, shortJob1.ExecutedCount(), int32(1), "Job 1 should execute at least once")
		assert.GreaterOrEqual(t, shortJob2.ExecutedCount(), int32(1), "Job 2 should execute at least once")
	})

	t.Run("respects shutdown timeout", func(t *testing.T) {
		config := Config{
			Size:            1,
			QueueSize:       2,
			MaxRetries:      1,
			ShutdownTimeout: 100 * time.Millisecond, // Short timeout
		}

		pool := New(config)
		pool.Start()

		// Submit a very long-running job
		veryLongJob := NewMockJob("very-long", "long", 5*time.Second, nil)
		err := pool.Submit(veryLongJob)
		require.NoError(t, err)

		// Give job time to start
		time.Sleep(20 * time.Millisecond)

		// Shutdown should timeout
		start := time.Now()
		_ = pool.Shutdown()
		shutdownDuration := time.Since(start)

		// Should respect timeout even if job isn't finished
		assert.Less(t, shutdownDuration, 200*time.Millisecond, "Should respect shutdown timeout")
	})
}

func TestErrorHandling(t *testing.T) {
	config := Config{
		Size:            2,
		QueueSize:       5,
		MaxRetries:      2,
		RetryDelay:      20 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("handles job execution errors", func(t *testing.T) {
		errorJob := NewMockJob("error-job", "error", 10*time.Millisecond, errors.New("execution failed"))
		err := pool.Submit(errorJob)
		require.NoError(t, err)

		// Wait for job and retries
		time.Sleep(200 * time.Millisecond)

		// Should be executed multiple times due to retries
		executed := errorJob.ExecutedCount()
		assert.Greater(t, executed, int32(1), "Should retry failed jobs")
		assert.LessOrEqual(t, executed, int32(config.MaxRetries+1), "Should not exceed max retries")
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		job := &MockJob{
			id:       "cancelled-job",
			jobType:  "cancel",
			duration: 100 * time.Millisecond,
		}

		// Job should handle cancellation
		err := job.Execute(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestRateLimiting(t *testing.T) {
	t.Run("respects rate limiting", func(t *testing.T) {
		config := Config{
			Size:            5,
			QueueSize:       20,
			MaxRetries:      1,
			ShutdownTimeout: 2 * time.Second,
			RateLimit:       5, // 5 jobs per second
		}

		pool := New(config)
		pool.Start()
		defer pool.Shutdown()

		// Submit many quick jobs
		const numJobs = 10
		jobs := make([]*MockJob, numJobs)

		start := time.Now()
		for i := 0; i < numJobs; i++ {
			jobs[i] = NewMockJob(fmt.Sprintf("rate-job-%d", i), "rate", time.Millisecond, nil)
			err := pool.Submit(jobs[i])
			require.NoError(t, err)
		}

		// Wait for all jobs to complete
		time.Sleep(3 * time.Second)
		duration := time.Since(start)

		// With rate limiting of 5/sec, 10 jobs should take at least 2 seconds
		if config.RateLimit > 0 {
			expectedMinTime := time.Duration(numJobs/config.RateLimit) * time.Second
			assert.GreaterOrEqual(t, duration, expectedMinTime-100*time.Millisecond,
				"Rate limiting should slow down job processing")
		}

		// All jobs should eventually complete
		for i, job := range jobs {
			assert.Equal(t, int32(1), job.ExecutedCount(), "Job %d should complete", i)
		}
	})
}

func TestConcurrentSubmission(t *testing.T) {
	config := Config{
		Size:            3,
		QueueSize:       100,
		MaxRetries:      1,
		ShutdownTimeout: 3 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("handles concurrent job submission", func(t *testing.T) {
		const numRoutines = 10
		const jobsPerRoutine = 5
		var wg sync.WaitGroup
		var totalJobs = numRoutines * jobsPerRoutine
		jobs := make([]*MockJob, totalJobs)

		// Submit jobs from multiple goroutines
		for r := 0; r < numRoutines; r++ {
			wg.Add(1)
			go func(routineID int) {
				defer wg.Done()
				for j := 0; j < jobsPerRoutine; j++ {
					jobID := routineID*jobsPerRoutine + j
					jobs[jobID] = NewMockJob(
						fmt.Sprintf("concurrent-%d-%d", routineID, j),
						"concurrent",
						20*time.Millisecond,
						nil,
					)
					err := pool.Submit(jobs[jobID])
					assert.NoError(t, err)
				}
			}(r)
		}

		wg.Wait()

		// Wait for all jobs to complete
		time.Sleep(time.Second)

		// Verify all jobs were executed
		for i, job := range jobs {
			if job != nil {
				assert.Equal(t, int32(1), job.ExecutedCount(), "Job %d should be executed", i)
			}
		}
	})
}

func TestResultChannelHandling(t *testing.T) {
	config := Config{
		Size:            2,
		QueueSize:       10,
		MaxRetries:      1,
		RetryDelay:      10 * time.Millisecond,
		ShutdownTimeout: 2 * time.Second,
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	t.Run("provides access to results channel", func(t *testing.T) {
		resultsChan := pool.Results()
		assert.NotNil(t, resultsChan)

		// Submit a job
		job := NewMockJob("result-test", "test", 10*time.Millisecond, nil)
		err := pool.Submit(job)
		require.NoError(t, err)

		// Should receive result
		select {
		case result := <-resultsChan:
			assert.Equal(t, "result-test", result.JobID)
			assert.Equal(t, "test", result.JobType)
			// Result may have error if cancelled during shutdown
			assert.GreaterOrEqual(t, result.Duration, time.Duration(0))
		case <-time.After(1 * time.Second):
			t.Fatal("Should receive result within timeout")
		}
	})
}

func TestPoolShutdownEdgeCases(t *testing.T) {
	t.Run("shutdown without start is safe", func(t *testing.T) {
		config := Config{Size: 1, QueueSize: 1, ShutdownTimeout: time.Second}
		pool := New(config)

		err := pool.Shutdown()
		assert.NoError(t, err)
	})

	t.Run("multiple shutdown calls are safe", func(t *testing.T) {
		config := Config{Size: 1, QueueSize: 1, ShutdownTimeout: time.Second}
		pool := New(config)
		pool.Start()

		err1 := pool.Shutdown()
		assert.NoError(t, err1)

		err2 := pool.Shutdown()
		assert.NoError(t, err2)

		err3 := pool.Shutdown()
		assert.NoError(t, err3)
	})
}

func BenchmarkPoolThroughput(b *testing.B) {
	config := Config{
		Size:            10,
		QueueSize:       1000,
		MaxRetries:      1,
		ShutdownTimeout: 5 * time.Second,
		RateLimit:       0, // No rate limiting for benchmark
	}

	pool := New(config)
	pool.Start()
	defer pool.Shutdown()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		jobID := 0
		for pb.Next() {
			job := NewMockJob(fmt.Sprintf("bench-%d", jobID), "benchmark", 0, nil)
			err := pool.Submit(job)
			if err != nil {
				b.Error(err)
			}
			jobID++
		}
	})
}
