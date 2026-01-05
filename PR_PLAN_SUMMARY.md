# PR Plan Summary: Scheduler Test Coverage Improvement

## 🎯 Objective

Increase test coverage for `internal/scheduler` package from **11.7% → 60%+** to address issue #223.

## 📊 Current State

### Existing Coverage: 11.7%
- ✅ Panic recovery tests (well covered)
- ✅ Scheduler initialization test
- ❌ No lifecycle tests (Start/Stop)
- ❌ No job management tests (Add/Remove/List)
- ❌ No job execution tests (Discovery/Scan)
- ❌ No database interaction tests

### Critical Gaps (0% Coverage)
- `Start()`, `Stop()` - Scheduler lifecycle
- `AddDiscoveryJob()`, `AddScanJob()` - Job creation
- `RemoveJob()`, `GetJobs()` - Job management
- `EnableJob()`, `DisableJob()` - Job state
- `loadScheduledJobs()` - Database loading
- `executeDiscoveryJob()`, `executeScanJob()` - Only 21-23% covered

## 🚀 Implementation Strategy

### Phase 1: Lifecycle Tests (~5% gain)
**Target:** Start/Stop functionality
- Test scheduler start with job loading
- Test scheduler stop with cleanup
- Test concurrent start/stop (thread safety)
- Test error handling (already running, load failures)

**Mocks Needed:** Database repository

### Phase 2: Job Management Tests (~20% gain)
**Target:** CRUD operations for jobs
- Add discovery/scan jobs with validation
- Remove jobs with database cleanup
- List all jobs with next run calculation
- Enable/disable jobs with state management
- Error handling (invalid cron, db failures, not found)

**Mocks Needed:** Database repository

### Phase 3: Job Execution Tests (~25% gain)
**Target:** Job execution logic
- Execute discovery jobs with discovery service
- Execute scan jobs with host selection
- Process hosts for scanning with filters
- Profile selection (specified, OS-based, default)
- State management (running flags, last run updates)
- Error scenarios (service failures, no hosts, etc.)

**Mocks Needed:** 
- Database repository
- Discovery service
- Profiles service

### Phase 4: Helpers & Edge Cases (~15% gain)
**Target:** Internal functions and boundaries
- Load jobs from database with parsing
- Create scheduled jobs with marshaling
- Prepare/cleanup job execution
- Build host scan queries with filters
- Save/delete/update database operations
- Edge cases (empty lists, nil values, malformed data)

**Mocks Needed:** All previous mocks

## 📋 Testing Approach

### Test Structure
```go
// Example test structure
func TestScheduler_AddDiscoveryJob(t *testing.T) {
    tests := []struct {
        name        string
        cronExpr    string
        config      DiscoveryJobConfig
        setupMocks  func(*mockDBRepository)
        wantErr     bool
        errContains string
    }{
        {
            name:     "valid_discovery_job",
            cronExpr: "0 0 * * *",
            config:   DiscoveryJobConfig{Network: "192.168.1.0/24", ...},
            setupMocks: func(m *mockDBRepository) {
                m.On("CreateScheduledJob", mock.Anything, mock.Anything).Return(nil)
            },
            wantErr: false,
        },
        {
            name:     "invalid_cron_expression",
            cronExpr: "invalid",
            wantErr:  true,
        },
        // ... more test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            // Execute test
            // Assert results
        })
    }
}
```

### Mock Setup
- Use `testify/mock` for all external dependencies
- Create reusable mock helpers
- Verify mock expectations after each test
- Clean up resources in defer statements

## 📈 Expected Results

### Coverage Progression
| Phase | Coverage | Gain |
|-------|----------|------|
| Current | 11.7% | - |
| Phase 1 | 16.7% | +5% |
| Phase 2 | 36.7% | +20% |
| Phase 3 | 61.7% | +25% |
| Phase 4 | 70%+ | +15% |

### Quality Metrics
- ✅ All critical paths covered
- ✅ Both happy and error paths tested
- ✅ Thread safety verified
- ✅ Fast execution (<30s for full suite)
- ✅ No flaky tests
- ✅ Clear, maintainable test code

## ✅ Success Criteria

- [ ] Coverage >60% achieved
- [ ] All 0% functions have >50% coverage
- [ ] Tests pass consistently (10+ runs)
- [ ] Tests complete in <30 seconds
- [ ] No new external dependencies
- [ ] Mocks properly cleaned up
- [ ] CI pipeline passes
- [ ] Code review approved

## 🔗 Related

- **Issue:** #223 - Improve test coverage across codebase
- **Priority:** P1 - Critical Infrastructure
- **Impact:** High - Scheduler manages all scheduled operations
- **Dependencies:** None (all mocked)

## 📝 Notes

- Tests will be **unit tests only** - no integration tests needed
- All dependencies mocked for fast, reliable execution
- No external services required (no DB, no nmap, etc.)
- Thread safety is critical - verify mutex usage
- Panic recovery already well tested - focus on business logic

## 🎬 Next Steps

1. Create feature branch: `test/scheduler-coverage`
2. Implement Phase 1 (lifecycle)
3. Verify coverage gain
4. Implement Phase 2 (job management)
5. Verify coverage gain
6. Implement Phase 3 (execution)
7. Verify coverage gain
8. Implement Phase 4 (helpers)
9. Final verification and PR submission

---

**Estimated Effort:** 2-3 days
**Risk:** Low (no production code changes)
**Impact:** High (critical infrastructure testing)