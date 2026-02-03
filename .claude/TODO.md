# Scanorama TODO - Remaining Codebase Analysis Tasks

**Last Updated:** February 3, 2026  
**Source:** CODEBASE_ANALYSIS.md  
**Status:** Active Development Tasks

**Recent Cleanup:** ✅ Removed 6 outdated AI-generated analysis documents from docs/ folder

---

## High Priority

### 1. Complete Scan Scheduling Implementation ⚠️ CRITICAL

**File:** `internal/scheduler/scheduler.go:484`  
**Issue:** `processHostsForScanning` has TODO placeholder instead of actual implementation

**Current State:**
```go
func (s *Scheduler) processHostsForScanning(ctx context.Context, hosts []*db.Host, config *ScanJobConfig) {
    // TODO: Implement actual scanning logic here
    // This would integrate with the existing scan functionality
    // For now, just log that we would scan these hosts
}
```

**Action Required:**
- Integrate with existing scan service
- Actually trigger scans for scheduled jobs
- Add error handling and logging
- Test scheduled scan execution

**Effort:** 2-4 hours  
**Impact:** Feature completion - scheduled scans don't work without this

---

## Medium Priority

### 2. Split Large Database File

**File:** `internal/db/database.go` (2,591 lines)  
**Recommendation:** Split into logical modules

**Proposed Structure:**
```
internal/db/
  ├── database.go       (core DB connection, migrations, sanitization)
  ├── hosts.go         (host-related operations)
  ├── scans.go         (scan-related operations)
  ├── networks.go      (network-related operations)
  ├── profiles.go      (profile operations)
  └── scheduled_jobs.go (scheduler operations)
```

**Effort:** 1 day  
**Benefit:** Easier navigation, better logical grouping, reduced merge conflicts

---

### 3. Consolidate Handler Patterns

**Location:** `internal/api/handlers/` (11 handler files)  
**Issue:** Repetitive patterns for validation, error handling, response formatting

**Recommendation:**
Create shared handler utilities (e.g., `internal/api/handlers/utils.go`):
- `ValidateAndDecode(r *http.Request, v interface{}) error`
- `RespondJSON(w http.ResponseWriter, status int, data interface{})`
- Common error response helpers

**Effort:** 1-2 days  
**Benefit:** Reduce handler code by ~20%, more consistent patterns

---

### 4. Simplify Database Error Sanitization

**File:** `internal/db/database.go` - `sanitizeDBError` function (75 lines)  
**Issue:** Inline error code handling is verbose

**Recommendation:**
- Create map-based lookup for PostgreSQL error codes
- Reduce inline conditionals
- Easier to maintain and extend

**Effort:** 2 hours  
**Benefit:** More maintainable error handling

---

### 5. Split Large Admin Handler

**File:** `internal/api/handlers/admin.go` (1,283 lines)  
**Recommendation:** Split into separate files:
- `admin_config.go` - Configuration handlers
- `admin_logging.go` - Logging handlers
- `admin_workers.go` - Worker management handlers

**Effort:** 3-4 hours  
**Benefit:** Better organization, easier to find specific handlers

---

### 6. Split Large Networks CLI

**File:** `cmd/cli/networks.go` (1,139 lines)  
**Recommendation:** Split into separate command files:
- `networks_list.go`
- `networks_add.go`
- `networks_delete.go`
- `networks_scan.go`

**Effort:** 2-3 hours  
**Benefit:** Clearer command structure

---

## Low Priority

### 7. Split Large Test Files

**Files with 1,500+ lines:**
- `internal/scheduler/scheduler_test.go` (2,064 lines)
- `internal/db/migration_test.go` (1,609 lines)
- `internal/api/handlers/profile_test.go` (1,529 lines)
- `internal/api/handlers/common_test.go` (1,512 lines)

**Recommendation:**
Split by functionality (e.g., `scheduler_test.go` → `scheduler_test.go`, `scheduler_discovery_test.go`, `scheduler_scan_test.go`)

**Effort:** 1 day  
**Benefit:** Easier to find specific tests

---

### 8. Extract Routing from Server

**File:** `internal/api/server.go` (881 lines)  
**Recommendation:** Extract routing to `routes.go`

**Effort:** 2 hours  
**Benefit:** Clearer separation of concerns

---

### 9. Consolidate Configuration

**Issue:** Configuration spread across multiple locations  
**Recommendation:**
- Keep single source of truth in `internal/config/`
- Document configuration hierarchy
- Clarify environment-specific overrides

**Effort:** 4 hours  
**Benefit:** Clearer configuration management

---

### 10. Improve Test Coverage

**Packages needing improvement:**
- `internal/auth` (46.1%) - Target: >60%
- `internal/scanning` (48.3%) - Target: >60%
- `internal/discovery` (51.0%) - Target: >60%
- `internal/api/handlers` (58.5%) - Target: >70%
- `internal/daemon` (32.3%) - Target: >50%
- `internal/services` (7.1%) - Target: >50%
- `internal/profiles` (25.7%) - Target: >50%
- `internal/db` (17.4%) - Target: >40%

**Note:** Some packages have extensive integration tests not reflected in unit coverage

**Effort:** Ongoing  
**Benefit:** Better test reliability, easier refactoring

---

## Documentation Improvements

### 11. Add Architecture Diagrams

**Location:** `docs/technical/architecture/`  
**Needed:**
- `system-overview.md` - High-level architecture
- `data-flow.md` - How data flows through system
- `scheduling-flow.md` - Cron job execution flow
- `api-design.md` - REST API design decisions

**Effort:** 2 days  
**Benefit:** Easier onboarding, clearer system understanding

---

### 12. Create Deployment Guide

**Location:** `docs/DEPLOYMENT.md`  
**Should Cover:**
- Production deployment options
- Environment variables reference
- Database setup and migrations
- Security considerations
- Monitoring and logging setup

**Effort:** 1 day  
**Benefit:** Easier production deployments

---

## Keep As-Is (Strengths) ✅

**Do NOT change these - they're working well:**
- Package structure and separation of concerns
- Error handling patterns
- Dependency management strategy
- CI/CD pipeline and tooling
- Metrics and logging implementation
- Modern tooling (Dependabot, Renovate, golangci-lint)

---

## Implementation Roadmap

### Phase 1: Critical (1 week)
1. Complete scan scheduling implementation (#1) - **HIGHEST PRIORITY**
2. Split database.go (#2)
3. Create shared handler utilities (#3)

### Phase 2: Organization (1 week)
4. Split admin.go (#5)
5. Split networks.go (#6)
6. Split large test files (#7)
7. Extract routing (#8)

### Phase 3: Documentation (1 week)
8. Add architecture diagrams (#11)
9. Create deployment guide (#12)
10. Document configuration (#9)

### Phase 4: Ongoing
11. Improve test coverage (#10)

---

## Notes

- **Repository cleanup completed:** ✅
  - Removed 57 orphaned coverage files
  - Removed 16 outdated AI-generated analysis documents (10 from root + 6 from docs/)
  - Repository is now clean

- **Documentation folder cleanup:** ✅ (February 3, 2026)
  - **Phase 1 removed** (AI-generated analysis documents):
    - `frontend-readiness-plan.md`
    - `api-documentation-analysis.md`
    - `implementation-recommendations.md`
    - `go-best-practices-analysis.md`
    - `ci-improvements.md`
    - `TESTING_STATUS.md`
  - **Phase 2 removed** (outdated/unimplemented feature docs):
    - `LOCAL_TESTING.md` (referenced non-existent make targets)
    - `QUICK_REFERENCE.md` (referenced non-existent make targets)
    - `server-commands.md` (documented unimplemented health endpoints)
    - `docs/ci/act-testing.md` (referenced non-existent infrastructure)
  - **Kept and verified:**
    - `API_CLI_PARITY.md` (verified against current codebase, added verification date)
    - `README.md` (documentation index)
    - `docs/api/` subdirectory (frontend quickstart, validation rules)
    - `docs/technical/` subdirectory (architecture, testing)
  - **Total removed:** 10 outdated documentation files

- **Recent fixes applied:** ✅
  - npm audit configured to exclude dev dependencies
  - Go compilation errors resolved (uax29 revert)
  - All CI/CD pipelines passing

- **Priority focus:** Complete scan scheduling before other refactoring