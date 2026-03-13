# Scanorama TODO - Remaining Codebase Analysis Tasks

**Last Updated:** March 13, 2026  
**Source:** CODEBASE_ANALYSIS.md  
**Status:** Active Development Tasks

**Recent Cleanup:** ✅ Removed 6 outdated AI-generated analysis documents from docs/ folder

---

## High Priority

### 1. Complete Scan Scheduling Implementation ✅ DONE

**File:** `internal/scheduler/scheduler.go`  
**Resolved in:** `fix(scheduler): implement bounded-concurrency host scanning`

`processHostsForScanning` now selects a scan profile per host, dispatches
goroutines bounded by a counting semaphore (`maxConcurrentScans`), calls
`scanning.RunScanWithContext`, and logs results. Full context-cancellation
support is included.

---

## Medium Priority

### 2. Split Large Database File ✅ DONE

`internal/db/` is already split into focused files:
`database.go`, `hosts.go`, `scans.go`, `networks.go`, `profiles.go`,
`scheduled_jobs.go`, `migrate.go`, `models.go`.

---

### 3. Consolidate Handler Patterns ✅ DONE

`internal/api/handlers/common.go` contains `writeJSON`, `writeError`,
`writePaginatedResponse`, `parseJSON`, `getPaginationParams`, and
`getRequestIDFromContext`. All handlers use these shared helpers.
`health.go` was also updated (PR #441) to use `writeJSON` instead of
four inline `Header/WriteHeader/json.NewEncoder` blocks.

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

### 5. Split Large Admin Handler ✅ DONE (PR #441)

`admin.go` (995 lines) split into four focused files:

- `admin.go` (232 lines) — handler struct, constructor, HTTP methods
- `admin_types.go` (151 lines) — all request/response types
- `admin_config.go` (212 lines) — config retrieval, parsing, extraction
- `admin_validate.go` (419 lines) — all validation and field-level checks

---

### 6. Split Large Networks CLI ✅ DONE (PR #441)

`networks.go` (1,139 lines) split into four focused files:

- `networks.go` (270 lines) — Cobra command vars, flags, `init()` wiring
- `networks_handlers.go` (496 lines) — all 12 `run*` command functions
- `networks_helpers.go` (336 lines) — DB query and display helpers
- `networks_complete.go` (72 lines) — shell completion + CIDR validation

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

### 8. Extract Routing from Server ✅ DONE (PR #441)

`setupRoutes` extracted from `server.go` into `internal/api/routes.go`
and decomposed into eight per-resource helper methods (`setupSystemRoutes`,
`setupScanRoutes`, `setupHostRoutes`, `setupDiscoveryRoutes`,
`setupProfileRoutes`, `setupScheduleRoutes`, `setupNetworkRoutes`,
`setupDocRoutes`). Resolves the `funlen` linter warning on the original
monolithic method.

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
1. ✅ Complete scan scheduling implementation (#1)
2. ✅ Split database.go (#2)
3. ✅ Create shared handler utilities (#3)

### Phase 2: Organization (1 week)
4. ✅ Split admin.go (#5) — done in PR #441
5. ✅ Split networks.go (#6) — done in PR #441
6. Split large test files (#7)
7. ✅ Extract routing (#8) — done in PR #441

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