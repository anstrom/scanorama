# Example Test Implementation for Scheduler Coverage

This document shows concrete examples of how the scheduler tests will be implemented.

## Mock Setup

```go
package scheduler

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"

    "github.com/anstrom/scanorama/internal/db"
)

// mockDBRepository mocks the database repository interface
type mockDBRepository struct {
    mock.Mock
}

func (m *mockDBRepository) CreateScheduledJob(ctx context.Context, job *db.ScheduledJob) error {
    args := m.Called(ctx, job)
    if args.Get(0) != nil {
        // Simulate database assigning an ID
        job.ID = uuid.New()
    }
    return args.Error(0)
}

func (m *mockDBRepository) GetScheduledJobs(ctx context.Context) ([]*db.ScheduledJob, error) {
    args := m.Called(ctx)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]*db.ScheduledJob), args.Error(1)
}

func (m *mockDBRepository) GetScheduledJob(ctx context.Context, id uuid.UUID) (*db.ScheduledJob, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*db.ScheduledJob), args.Error(1)
}

func (m *mockDBRepository) UpdateScheduledJob(ctx context.Context, job *db.ScheduledJob) error {
    args := m.Called(ctx, job)
    return args.Error(0)
}

func (m *mockDBRepository) DeleteScheduledJob(ctx context.Context, id uuid.UUID) error {
    args := m.Called(ctx, id)
    return args.Error(0)
}

func (m *mockDBRepository) UpdateScheduledJobEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
    args := m.Called(ctx, id, enabled)
    return args.Error(0)
}

// mockDiscoveryService mocks the discovery service
type mockDiscoveryService struct {
    mock.Mock
}

func (m *mockDiscoveryService) DiscoverNetwork(ctx context.Context, network, method string, detectOS bool, timeout, concurrency int) error {
    args := m.Called(ctx, network, method, detectOS, timeout, concurrency)
    return args.Error(0)
}

// mockProfilesService mocks the profiles service
type mockProfilesService struct {
    mock.Mock
}

func (m *mockProfilesService) GetProfile(ctx context.Context, id uuid.UUID) (*Profile, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Profile), args.Error(1)
}

func (m *mockProfilesService) GetDefaultProfile(ctx context.Context) (*Profile, error) {
    args := m.Called(ctx)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Profile), args.Error(1)
}

// Helper to create test profile
type Profile struct {
    ID   uuid.UUID
    Name string
}
```

---

## Phase 1: Lifecycle Tests

```go
// TestScheduler_Start tests starting the scheduler
func TestScheduler_Start(t *testing.T) {
    tests := []struct {
        name           string
        setupMocks     func(*mockDBRepository)
        existingJobs   []*db.ScheduledJob
        wantErr        bool
        errContains    string
        wantJobCount   int
    }{
        {
            name: "start_successfully_with_no_jobs",
            setupMocks: func(m *mockDBRepository) {
                m.On("GetScheduledJobs", mock.Anything).Return([]*db.ScheduledJob{}, nil)
            },
            existingJobs: []*db.ScheduledJob{},
            wantErr:      false,
            wantJobCount: 0,
        },
        {
            name: "start_successfully_with_existing_jobs",
            setupMocks: func(m *mockDBRepository) {
                jobs := []*db.ScheduledJob{
                    {
                        ID:             uuid.New(),
                        Name:           "test-discovery",
                        Type:           db.ScheduledJobTypeDiscovery,
                        CronExpression: "0 0 * * *",
                        Enabled:        true,
                        Config:         db.JSONB(`{"network": "192.168.1.0/24"}`),
                    },
                    {
                        ID:             uuid.New(),
                        Name:           "test-scan",
                        Type:           db.ScheduledJobTypeScan,
                        CronExpression: "0 1 * * *",
                        Enabled:        true,
                        Config:         db.JSONB(`{"live_hosts_only": true}`),
                    },
                }
                m.On("GetScheduledJobs", mock.Anything).Return(jobs, nil)
            },
            wantErr:      false,
            wantJobCount: 2,
        },
        {
            name: "error_loading_jobs_from_database",
            setupMocks: func(m *mockDBRepository) {
                m.On("GetScheduledJobs", mock.Anything).Return(nil, assert.AnError)
            },
            wantErr:     true,
            errContains: "failed to load scheduled jobs",
        },
        {
            name: "error_scheduler_already_running",
            setupMocks: func(m *mockDBRepository) {
                // No mock expectations - should fail before DB call
            },
            wantErr:     true,
            errContains: "already running",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            if tt.setupMocks != nil {
                tt.setupMocks(mockDB)
            }

            s := NewScheduler(mockDB, nil, nil)
            
            // For "already running" test, start scheduler first
            if tt.name == "error_scheduler_already_running" {
                mockDB.On("GetScheduledJobs", mock.Anything).Return([]*db.ScheduledJob{}, nil).Once()
                require.NoError(t, s.Start())
                defer s.Stop()
            }

            // Execute
            err := s.Start()

            // Assert
            if tt.wantErr {
                require.Error(t, err)
                if tt.errContains != "" {
                    assert.Contains(t, err.Error(), tt.errContains)
                }
            } else {
                require.NoError(t, err)
                assert.True(t, s.running)
                assert.Equal(t, tt.wantJobCount, len(s.jobs))
                
                // Clean up
                s.Stop()
            }

            mockDB.AssertExpectations(t)
        })
    }
}

// TestScheduler_Stop tests stopping the scheduler
func TestScheduler_Stop(t *testing.T) {
    tests := []struct {
        name          string
        startFirst    bool
        setupMocks    func(*mockDBRepository)
    }{
        {
            name:       "stop_running_scheduler",
            startFirst: true,
            setupMocks: func(m *mockDBRepository) {
                m.On("GetScheduledJobs", mock.Anything).Return([]*db.ScheduledJob{}, nil)
            },
        },
        {
            name:       "stop_already_stopped_scheduler_noop",
            startFirst: false,
            setupMocks: func(m *mockDBRepository) {
                // No mock expectations needed
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            if tt.setupMocks != nil {
                tt.setupMocks(mockDB)
            }

            s := NewScheduler(mockDB, nil, nil)

            if tt.startFirst {
                require.NoError(t, s.Start())
                assert.True(t, s.running)
            }

            // Execute
            s.Stop()

            // Assert
            assert.False(t, s.running)

            // Verify context was cancelled if started
            if tt.startFirst {
                select {
                case <-s.ctx.Done():
                    // Context cancelled as expected
                default:
                    t.Error("Context was not cancelled")
                }
            }

            mockDB.AssertExpectations(t)
        })
    }
}

// TestScheduler_StartStop_Concurrency tests concurrent start/stop
func TestScheduler_StartStop_Concurrency(t *testing.T) {
    mockDB := new(mockDBRepository)
    mockDB.On("GetScheduledJobs", mock.Anything).Return([]*db.ScheduledJob{}, nil)

    s := NewScheduler(mockDB, nil, nil)
    defer s.Stop()

    // Try to start/stop concurrently
    done := make(chan bool)
    
    go func() {
        for i := 0; i < 10; i++ {
            _ = s.Start()
            time.Sleep(10 * time.Millisecond)
        }
        done <- true
    }()

    go func() {
        for i := 0; i < 10; i++ {
            s.Stop()
            time.Sleep(10 * time.Millisecond)
        }
        done <- true
    }()

    // Wait for both goroutines
    <-done
    <-done

    // No assertion needed - test passes if no race condition detected
    mockDB.AssertExpectations(t)
}
```

---

## Phase 2: Job Management Tests

```go
// TestScheduler_AddDiscoveryJob tests adding discovery jobs
func TestScheduler_AddDiscoveryJob(t *testing.T) {
    tests := []struct {
        name        string
        jobName     string
        cronExpr    string
        config      DiscoveryJobConfig
        setupMocks  func(*mockDBRepository)
        wantErr     bool
        errContains string
    }{
        {
            name:     "add_valid_discovery_job",
            jobName:  "nightly-discovery",
            cronExpr: "0 0 * * *",
            config: DiscoveryJobConfig{
                Network:     "192.168.1.0/24",
                Method:      "ping",
                DetectOS:    true,
                Timeout:     30,
                Concurrency: 50,
            },
            setupMocks: func(m *mockDBRepository) {
                m.On("CreateScheduledJob", mock.Anything, mock.MatchedBy(func(job *db.ScheduledJob) bool {
                    return job.Name == "nightly-discovery" &&
                           job.Type == db.ScheduledJobTypeDiscovery &&
                           job.CronExpression == "0 0 * * *"
                })).Return(nil)
            },
            wantErr: false,
        },
        {
            name:     "error_invalid_cron_expression",
            jobName:  "bad-cron",
            cronExpr: "invalid cron",
            config: DiscoveryJobConfig{
                Network: "192.168.1.0/24",
            },
            setupMocks: func(m *mockDBRepository) {
                m.On("CreateScheduledJob", mock.Anything, mock.Anything).Return(nil)
            },
            wantErr:     true,
            errContains: "invalid cron expression",
        },
        {
            name:     "error_database_failure",
            jobName:  "db-fail",
            cronExpr: "0 0 * * *",
            config: DiscoveryJobConfig{
                Network: "192.168.1.0/24",
            },
            setupMocks: func(m *mockDBRepository) {
                m.On("CreateScheduledJob", mock.Anything, mock.Anything).Return(assert.AnError)
            },
            wantErr:     true,
            errContains: "failed to create scheduled job",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            if tt.setupMocks != nil {
                tt.setupMocks(mockDB)
            }

            s := NewScheduler(mockDB, nil, nil)
            ctx := context.Background()

            // Execute
            err := s.AddDiscoveryJob(ctx, tt.jobName, tt.cronExpr, tt.config)

            // Assert
            if tt.wantErr {
                require.Error(t, err)
                if tt.errContains != "" {
                    assert.Contains(t, err.Error(), tt.errContains)
                }
            } else {
                require.NoError(t, err)
                
                // Verify job was added to memory
                s.mu.RLock()
                jobCount := len(s.jobs)
                s.mu.RUnlock()
                assert.Equal(t, 1, jobCount)
            }

            mockDB.AssertExpectations(t)
        })
    }
}

// TestScheduler_RemoveJob tests removing jobs
func TestScheduler_RemoveJob(t *testing.T) {
    tests := []struct {
        name        string
        setupJob    bool
        jobID       uuid.UUID
        setupMocks  func(*mockDBRepository, uuid.UUID)
        wantErr     bool
        errContains string
    }{
        {
            name:     "remove_existing_job",
            setupJob: true,
            setupMocks: func(m *mockDBRepository, id uuid.UUID) {
                m.On("DeleteScheduledJob", mock.Anything, id).Return(nil)
            },
            wantErr: false,
        },
        {
            name:     "error_job_not_found",
            setupJob: false,
            jobID:    uuid.New(),
            setupMocks: func(m *mockDBRepository, id uuid.UUID) {
                // No expectations - should fail before DB call
            },
            wantErr:     true,
            errContains: "job not found",
        },
        {
            name:     "error_database_deletion_failed",
            setupJob: true,
            setupMocks: func(m *mockDBRepository, id uuid.UUID) {
                m.On("DeleteScheduledJob", mock.Anything, id).Return(assert.AnError)
            },
            wantErr:     true,
            errContains: "failed to delete from database",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            s := NewScheduler(mockDB, nil, nil)
            ctx := context.Background()

            var jobID uuid.UUID
            if tt.setupJob {
                // Manually add a job to the scheduler
                jobID = uuid.New()
                nextRun := time.Now().Add(time.Hour)
                s.jobs[jobID] = &ScheduledJob{
                    ID: jobID,
                    Config: &db.ScheduledJob{
                        ID:             jobID,
                        Name:           "test-job",
                        Type:           db.ScheduledJobTypeDiscovery,
                        CronExpression: "0 0 * * *",
                        NextRun:        &nextRun,
                    },
                    CronID: s.cron.Schedule(cron.New().Every(time.Hour), cron.FuncJob(func() {})),
                }
            } else if tt.jobID != uuid.Nil {
                jobID = tt.jobID
            }

            if tt.setupMocks != nil {
                tt.setupMocks(mockDB, jobID)
            }

            // Execute
            err := s.RemoveJob(ctx, jobID)

            // Assert
            if tt.wantErr {
                require.Error(t, err)
                if tt.errContains != "" {
                    assert.Contains(t, err.Error(), tt.errContains)
                }
            } else {
                require.NoError(t, err)
                
                // Verify job was removed from memory
                s.mu.RLock()
                _, exists := s.jobs[jobID]
                s.mu.RUnlock()
                assert.False(t, exists)
            }

            mockDB.AssertExpectations(t)
        })
    }
}

// TestScheduler_GetJobs tests listing all jobs
func TestScheduler_GetJobs(t *testing.T) {
    tests := []struct {
        name         string
        setupMocks   func(*mockDBRepository)
        wantJobCount int
    }{
        {
            name: "get_empty_job_list",
            setupMocks: func(m *mockDBRepository) {
                m.On("GetScheduledJobs", mock.Anything).Return([]*db.ScheduledJob{}, nil)
            },
            wantJobCount: 0,
        },
        {
            name: "get_multiple_jobs",
            setupMocks: func(m *mockDBRepository) {
                jobs := []*db.ScheduledJob{
                    {
                        ID:             uuid.New(),
                        Name:           "job-1",
                        Type:           db.ScheduledJobTypeDiscovery,
                        CronExpression: "0 0 * * *",
                    },
                    {
                        ID:             uuid.New(),
                        Name:           "job-2",
                        Type:           db.ScheduledJobTypeScan,
                        CronExpression: "0 1 * * *",
                    },
                }
                m.On("GetScheduledJobs", mock.Anything).Return(jobs, nil)
            },
            wantJobCount: 2,
        },
        {
            name: "handle_database_error_gracefully",
            setupMocks: func(m *mockDBRepository) {
                m.On("GetScheduledJobs", mock.Anything).Return(nil, assert.AnError)
            },
            wantJobCount: 0, // Should return empty list on error
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            if tt.setupMocks != nil {
                tt.setupMocks(mockDB)
            }

            s := NewScheduler(mockDB, nil, nil)

            // Execute
            jobs := s.GetJobs()

            // Assert
            assert.Len(t, jobs, tt.wantJobCount)
            
            // Verify next run times are calculated
            for _, job := range jobs {
                assert.False(t, job.NextRun.IsZero(), "NextRun should be calculated")
            }

            mockDB.AssertExpectations(t)
        })
    }
}
```

---

## Phase 3: Job Execution Tests

```go
// TestScheduler_ExecuteDiscoveryJob tests discovery job execution
func TestScheduler_ExecuteDiscoveryJob(t *testing.T) {
    tests := []struct {
        name           string
        jobEnabled     bool
        setupMocks     func(*mockDBRepository, *mockDiscoveryService)
        wantExecuted   bool
    }{
        {
            name:       "execute_discovery_job_successfully",
            jobEnabled: true,
            setupMocks: func(mDB *mockDBRepository, mDisc *mockDiscoveryService) {
                mDisc.On("DiscoverNetwork", 
                    mock.Anything, 
                    "192.168.1.0/24", 
                    "ping", 
                    true, 
                    30, 
                    50,
                ).Return(nil)
                mDB.On("UpdateScheduledJob", mock.Anything, mock.Anything).Return(nil)
            },
            wantExecuted: true,
        },
        {
            name:       "skip_disabled_job",
            jobEnabled: false,
            setupMocks: func(mDB *mockDBRepository, mDisc *mockDiscoveryService) {
                // No mock expectations - job should not execute
            },
            wantExecuted: false,
        },
        {
            name:       "handle_discovery_service_error",
            jobEnabled: true,
            setupMocks: func(mDB *mockDBRepository, mDisc *mockDiscoveryService) {
                mDisc.On("DiscoverNetwork", 
                    mock.Anything, 
                    mock.Anything, 
                    mock.Anything, 
                    mock.Anything, 
                    mock.Anything, 
                    mock.Anything,
                ).Return(assert.AnError)
                // Should still update last run even on error
                mDB.On("UpdateScheduledJob", mock.Anything, mock.Anything).Return(nil)
            },
            wantExecuted: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockDB := new(mockDBRepository)
            mockDisc := new(mockDiscoveryService)
            
            if tt.setupMocks != nil {
                tt.setupMocks(mockDB, mockDisc)
            }

            s := NewScheduler(mockDB, mockDisc, nil)

            // Setup job
            jobID := uuid.New()
            nextRun := time.Now().Add(time.Hour)
            s.jobs[jobID] = &ScheduledJob{
                ID: jobID,
                Config: &db.ScheduledJob{
                    ID:             jobID,
                    Name:           "test-discovery",
                    Type:           db.ScheduledJobTypeDiscovery,
                    Enabled:        tt.jobEnabled,
                    CronExpression: "0 0 * * *",
                    NextRun:        &nextRun,
                    Config:         db.JSONB(`{"network": "192.168.1.0/24", "method": "ping", "detect_os": true, "timeout": 30, "concurrency": 50}`),
                },
            }

            config := DiscoveryJobConfig{
                Network:     "192.168.1.0/24",
                Method:      "ping",
                DetectOS:    true,
                Timeout:     30,
                Concurrency: 50,
            }

            // Execute
            s.executeDiscoveryJob(jobID, config)

            // Give it time to complete
            time.Sleep(100 * time.Millisecond)

            // Assert
            s.mu.RLock()
            job := s.jobs[jobID]
            isRunning := job.Running
            s.mu.RUnlock()

            assert.False(t, isRunning, "Job should not be marked as running after completion")

            mockDB.AssertExpectations(t)
            mockDisc.AssertExpectations(t)
        })
    }
}
```

---

## Running the Tests

```bash
# Run all scheduler tests
go test -v ./internal/scheduler/...

# Run with coverage
go test -coverprofile=coverage.out ./internal/scheduler/...
go tool cover -html=coverage.out

# Run specific test
go test -v -run TestScheduler_Start ./internal/scheduler/...

# Run with race detection
go test -race ./internal/scheduler/...
```

---

## Notes

- All tests use mocks to avoid external dependencies
- Tests verify both success and error paths
- State management (running flags) is verified
- Thread safety is ensured with mutex usage
- Tests are fast (<30s for full suite)
- Clear test names describe the scenario being tested