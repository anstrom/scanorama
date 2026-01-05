# PR Plan: Improve Scheduler Test Coverage (11.7% → 60%+)

## Overview

This PR aims to significantly improve test coverage for the `internal/scheduler` package, addressing issue #223. The scheduler is critical infrastructure that manages all scheduled discovery and scan jobs, and currently has only 11.7% test coverage.

## Current State

### Existing Tests (11.7% coverage)
- ✅ `TestPanicRecoveryInCronWrapper` - Tests panic recovery in job execution
- ✅ `TestJobStateCleanupOnPanic` - Tests state cleanup after panics
- ✅ `TestMultipleJobsPanicIsolation` - Tests panic isolation between jobs
- ✅ `TestExecuteDiscoveryJobPanicRecovery` - Tests discovery job panic handling
- ✅ `TestExecuteScanJobPanicRecovery` - Tests scan job panic handling
- ✅ `TestNewScheduler` - Tests scheduler initialization

### Coverage by Function
| Function | Current Coverage | Priority |
|----------|-----------------|----------|
| `NewScheduler` | 100.0% | ✅ Done |
| `addJobToCron` | 94.1% | ✅ Good |
| `prepareJobExecution` | 38.5% | 🟡 Needs work |
| `executeDiscoveryJob` | 21.2% | 🟡 Needs work |
| `executeScanJob` | 23.8% | 🟡 Needs work |
| `Start` | 0.0% | 🔴 Critical |
| `Stop` | 0.0% | 🔴 Critical |
| `AddDiscoveryJob` | 0.0% | 🔴 Critical |
| `AddScanJob` | 0.0% | 🔴 Critical |
| `RemoveJob` | 0.0% | 🔴 Critical |
| `GetJobs` | 0.0% | 🔴 Critical |
| `EnableJob` | 0.0% | 🔴 Critical |
| `DisableJob` | 0.0% | 🔴 Critical |
| All other functions | 0.0% | 🔴 Critical |

## Test Implementation Plan

### Phase 1: Scheduler Lifecycle Tests

**Goal:** Test basic scheduler start/stop functionality

#### Tests to Add:

1. **TestScheduler_Start**
   - ✓ Start scheduler successfully
   - ✓ Load existing jobs from database
   - ✓ Verify cron scheduler starts
   - ✓ Verify running state is set
   - ✓ Error: Scheduler already running
   - ✓ Error: Failed to load jobs from database

2. **TestScheduler_Stop**
   - ✓ Stop running scheduler
   - ✓ Verify cron scheduler stops
   - ✓ Verify context is cancelled
   - ✓ Verify running state is cleared
   - ✓ Handle stopping already stopped scheduler (no-op)

3. **TestScheduler_StartStop_Concurrency**
   - ✓ Concurrent start/stop calls
   - ✓ Verify thread safety with mutex

**Mock Requirements:**
- Mock database repository for loading jobs
- No discovery/profiles mocks needed for lifecycle tests

**Estimated Coverage Gain:** +5%

---

### Phase 2: Job Management Tests

**Goal:** Test adding, removing, and listing jobs

#### Tests to Add:

4. **TestScheduler_AddDiscoveryJob**
   - ✓ Add valid discovery job
   - ✓ Job saved to database
   - ✓ Job added to cron scheduler
   - ✓ Job stored in memory
   - ✓ Error: Invalid cron expression
   - ✓ Error: Database save failure
   - ✓ Error: Invalid job config

5. **TestScheduler_AddScanJob**
   - ✓ Add valid scan job
   - ✓ Job saved to database
   - ✓ Job added to cron scheduler
   - ✓ Job stored in memory
   - ✓ Error: Invalid cron expression
   - ✓ Error: Database save failure
   - ✓ Multiple scan jobs with different configs

6. **TestScheduler_RemoveJob**
   - ✓ Remove existing job
   - ✓ Job removed from cron
   - ✓ Job removed from database
   - ✓ Job removed from memory
   - ✓ Error: Job not found
   - ✓ Error: Database deletion failure

7. **TestScheduler_GetJobs**
   - ✓ Get all jobs from database
   - ✓ Calculate next run times
   - ✓ Empty job list
   - ✓ Multiple jobs returned
   - ✓ Handle database error gracefully

8. **TestScheduler_EnableDisableJob**
   - ✓ Enable disabled job
   - ✓ Disable enabled job
   - ✓ Error: Job not found
   - ✓ Error: Database update failure
   - ✓ Verify job state changes

**Mock Requirements:**
- Mock `db.Repository` interface:
  - `CreateScheduledJob(ctx, job) error`
  - `GetScheduledJobs(ctx) ([]*db.ScheduledJob, error)`
  - `GetScheduledJob(ctx, id) (*db.ScheduledJob, error)`
  - `UpdateScheduledJob(ctx, job) error`
  - `DeleteScheduledJob(ctx, id) error`
  - `UpdateScheduledJobEnabled(ctx, id, enabled) error`

**Estimated Coverage Gain:** +20%

---

### Phase 3: Job Execution Tests

**Goal:** Test job execution logic with mocked dependencies

#### Tests to Add:

9. **TestScheduler_ExecuteDiscoveryJob_Complete**
   - ✓ Execute discovery job successfully
   - ✓ Verify job state: not running → running → not running
   - ✓ Verify prepareJobExecution called
   - ✓ Verify discovery service called with correct params
   - ✓ Verify cleanupJobExecution called
   - ✓ Verify last run time updated
   - ✓ Job disabled - early return
   - ✓ Job already running - skip execution

10. **TestScheduler_ExecuteDiscoveryJob_Errors**
    - ✓ Discovery service returns error
    - ✓ Database error on last run update
    - ✓ State cleanup on error

11. **TestScheduler_ExecuteScanJob_Complete**
    - ✓ Execute scan job successfully
    - ✓ Fetch hosts to scan from database
    - ✓ Process hosts and select profiles
    - ✓ Execute scans (mocked)
    - ✓ State management throughout execution
    - ✓ Live hosts only filter
    - ✓ Network filter applied
    - ✓ OS family filter applied
    - ✓ Max age filter applied

12. **TestScheduler_ExecuteScanJob_Errors**
    - ✓ Database error fetching hosts
    - ✓ No hosts to scan
    - ✓ Profile selection error
    - ✓ Scan execution error

13. **TestScheduler_ProcessHostsForScanning**
    - ✓ Build host scan query correctly
    - ✓ Apply all filters (live hosts, networks, OS, max age)
    - ✓ Execute query and scan hosts
    - ✓ Select appropriate profiles
    - ✓ Handle missing profile gracefully

**Mock Requirements:**
- Mock `discovery.Service` interface:
  - `DiscoverNetwork(ctx, network, method, detectOS, timeout, concurrency) error`
- Mock `profiles.Service` interface:
  - `GetProfile(ctx, id) (*profiles.Profile, error)`
  - `GetDefaultProfile(ctx) (*profiles.Profile, error)`
- Mock database queries for host fetching
- Mock scan execution (likely via db repository)

**Estimated Coverage Gain:** +25%

---

### Phase 4: Helper Functions & Edge Cases

**Goal:** Test internal helper functions and edge cases

#### Tests to Add:

14. **TestScheduler_LoadScheduledJobs**
    - ✓ Load jobs from database
    - ✓ Parse job configs correctly
    - ✓ Add each job to cron
    - ✓ Handle empty job list
    - ✓ Handle malformed job config
    - ✓ Handle database error

15. **TestScheduler_CreateScheduledJob**
    - ✓ Create job with valid config
    - ✓ Marshal config to JSONB
    - ✓ Save to database
    - ✓ Return job object
    - ✓ Error: Invalid config
    - ✓ Error: Database error

16. **TestScheduler_PrepareAndCleanupJobExecution**
    - ✓ Prepare: Check if job exists
    - ✓ Prepare: Check if job enabled
    - ✓ Prepare: Check if already running
    - ✓ Prepare: Set running state
    - ✓ Cleanup: Clear running state
    - ✓ Cleanup: Thread safety

17. **TestScheduler_UpdateJobLastRun**
    - ✓ Update last run time in database
    - ✓ Handle database error
    - ✓ Verify correct timestamp

18. **TestScheduler_SaveAndDeleteScheduledJob**
    - ✓ Save job to database
    - ✓ Delete job from database
    - ✓ Handle database errors

19. **TestScheduler_HostScanQuery**
    - ✓ Build query with no filters
    - ✓ Build query with live hosts filter
    - ✓ Build query with networks filter
    - ✓ Build query with OS family filter
    - ✓ Build query with max age filter
    - ✓ Build query with all filters combined

20. **TestScheduler_SelectProfileForHost**
    - ✓ Use specified profile ID
    - ✓ Use OS family profile
    - ✓ Fall back to default profile
    - ✓ Handle profile not found
    - ✓ Handle profiles service error

**Estimated Coverage Gain:** +15%

---

## Mock Implementation Strategy

### Mock Database Repository

```go
type mockDBRepository struct {
    mock.Mock
}

func (m *mockDBRepository) CreateScheduledJob(ctx context.Context, job *db.ScheduledJob) error {
    args := m.Called(ctx, job)
    return args.Error(0)
}

func (m *mockDBRepository) GetScheduledJobs(ctx context.Context) ([]*db.ScheduledJob, error) {
    args := m.Called(ctx)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).([]*db.ScheduledJob), args.Error(1)
}

// ... additional methods
```

### Mock Discovery Service

```go
type mockDiscoveryService struct {
    mock.Mock
}

func (m *mockDiscoveryService) DiscoverNetwork(ctx context.Context, network, method string, detectOS bool, timeout, concurrency int) error {
    args := m.Called(ctx, network, method, detectOS, timeout, concurrency)
    return args.Error(0)
}
```

### Mock Profiles Service

```go
type mockProfilesService struct {
    mock.Mock
}

func (m *mockProfilesService) GetProfile(ctx context.Context, id uuid.UUID) (*profiles.Profile, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*profiles.Profile), args.Error(1)
}

func (m *mockProfilesService) GetDefaultProfile(ctx context.Context) (*profiles.Profile, error) {
    args := m.Called(ctx)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*profiles.Profile), args.Error(1)
}
```

---

## Testing Best Practices

1. **Use Table-Driven Tests** where appropriate
2. **Test Both Happy and Error Paths** for each function
3. **Verify State Changes** (running flags, job counts, etc.)
4. **Test Concurrency** with goroutines where relevant
5. **Clean Up Resources** (stop schedulers, cancel contexts)
6. **Use Meaningful Test Names** that describe the scenario
7. **Mock External Dependencies** to keep tests fast and reliable
8. **Assert on Mock Expectations** to verify correct interactions
9. **Test Edge Cases** (empty lists, nil values, boundary conditions)
10. **Keep Tests Focused** - one aspect per test function

---

## Expected Outcomes

### Coverage Targets
- **Current:** 11.7%
- **Target:** 60%+
- **Estimated Final:** 65-70%

### Coverage Breakdown by Phase
- Phase 1 (Lifecycle): 11.7% → 16.7%
- Phase 2 (Job Management): 16.7% → 36.7%
- Phase 3 (Job Execution): 36.7% → 61.7%
- Phase 4 (Helpers/Edge Cases): 61.7% → 70%+

### Code Quality Improvements
- ✅ Better documentation through test examples
- ✅ Improved confidence in scheduler reliability
- ✅ Easier to refactor with comprehensive test suite
- ✅ Clearer understanding of error handling paths
- ✅ Validation of concurrent access patterns

---

## Implementation Checklist

### Pre-PR
- [ ] Review scheduler.go implementation thoroughly
- [ ] Identify all dependencies that need mocking
- [ ] Set up mock implementations using testify/mock
- [ ] Create helper functions for common test setup

### During Implementation
- [ ] Implement Phase 1 tests (lifecycle)
- [ ] Verify coverage improvement after Phase 1
- [ ] Implement Phase 2 tests (job management)
- [ ] Verify coverage improvement after Phase 2
- [ ] Implement Phase 3 tests (execution)
- [ ] Verify coverage improvement after Phase 3
- [ ] Implement Phase 4 tests (helpers/edge cases)
- [ ] Verify final coverage >60%

### Pre-Merge
- [ ] Run all tests: `make test`
- [ ] Check coverage: `make coverage`
- [ ] Verify no flaky tests (run 10x)
- [ ] Update documentation if needed
- [ ] Review test names and descriptions
- [ ] Ensure all mocks are properly cleaned up
- [ ] Verify tests run quickly (<30s for unit tests)

---

## Related Issues

- Addresses: #223 (test coverage improvement)
- Contributes to: Overall project quality and maintainability
- Enables: Future refactoring with confidence

---

## Notes

- **No External Dependencies Required:** All tests use mocks
- **Fast Test Execution:** Unit tests with mocks run in milliseconds
- **Maintainable:** Clear test names and structure
- **Comprehensive:** Covers happy paths, errors, and edge cases
- **Thread-Safe:** Tests verify concurrent access patterns

---

## Success Criteria

✅ Coverage increases from 11.7% to >60%  
✅ All critical functions have test coverage  
✅ Tests are fast (<30s for full suite)  
✅ No flaky tests  
✅ Tests follow project conventions  
✅ Mock setup is clean and reusable  
✅ PR passes CI pipeline  
✅ Code review approved