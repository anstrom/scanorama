# Scanorama Improvement Plan

> **Status:** In progress — Phases 0–3 complete, Phase 5/6 partially complete.
> **Last updated:** 2026-03-14
> **Branch context:** `main` (PR landed; subsequent work merged)

---

## Current State

| Package | Coverage (original) | Coverage (current) | Notes |
|---|---:|---:|---|
| `internal/services` | 8.2% | 29.0% | sqlmock-based unit tests added |
| `cmd/cli` | 14.6% | 14.7% | CLI commands largely untested |
| `internal/db` | 22.4% | 14.1% | Integration tests separated; unit-testable surface small |
| `internal/profiles` | 25.7% | 25.7% | Profile CRUD untested |
| `internal/daemon` | 32.3% | 42.9% | Lifecycle unit tests added |
| `internal/scanning` | 48.3% | 48.3% | Error/edge paths missing |
| `internal/auth` | 51.5% | 46.1% | |
| `internal/discovery` | 51.0% | 51.0% | |
| `internal/api/handlers` | 73.6% | 79.2% | Mock-based tests for all 6 handlers; network handler gap closed |
| `internal/scheduler` | 71.5% | 69.7% | Bounded concurrency implemented; test file split |
| `internal/config` | — | 86.0% | New `validate.go` with per-section validators + normalization |

### Key Deficiencies

1. ~~**Placeholder response converters** — `scheduleToResponse`, `discoveryToResponse`, `scanToResponse`, and `resultToResponse` return hardcoded/partial data instead of mapping from real DB types.~~ ✅ Fixed
2. ~~**Placeholder admin operations** — `stopWorker`, `updateConfig`, `getLogs` in `admin.go` are stubs that log and sleep.~~ ✅ Return 501
3. ~~**No service-layer interfaces** — handlers take concrete `*services.NetworkService`, `*db.DB`, etc., making unit testing impossible without a live database.~~ ✅ Interfaces + mocks for all 6 handlers
4. **No scan concurrency controls** — nothing prevents resource exhaustion when many scans run at once (#221). *(scheduler-level concurrency added; API-level queue still needed)*
5. ~~**Large files** — `admin.go` (1 283 lines), `cmd/cli/networks.go` (1 139 lines), `server.go` (881 lines).~~ ✅ All split
6. ~~**Missing docs** — no deployment guide, no architecture diagrams.~~ ✅ DEPLOYMENT.md, CONFIGURATION.md, architecture docs created

---

## Phase 0 — Finish Current PR ✅ COMPLETE

**Goal:** Land `fix/bug-fixes-pr1` with clean history.

- [x] Implement discovery job DB CRUD (`networks.go`)
- [x] Implement schedule DB CRUD (`scheduled_jobs.go`)
- [x] Refactor `UpdateScan` + harden `extractScanData`
- [x] Unit tests for `validateCreateNetworkRequest`, `validateUpdateNetworkRequest`
- [x] Unit tests for `getScanFilters`, `scanToHostScanResponse`
- [x] Extended `validateNetworkConfig` coverage (method + exclusion branches)
- [x] Fix pre-existing `DeleteNetwork_NotFound` assertion

---

## Phase 1 — Critical Functional Gaps ✅ COMPLETE

**Issues:** #333
**Goal:** Make features that look complete actually work.

### 1a. Fix placeholder response converters ✅

- [x] `scheduleToResponse` — maps from real `db.Schedule` fields
- [x] `discoveryToResponse` — maps from real `db.DiscoveryJob` fields
- [x] `scanToResponse` — maps `Targets`, real `Progress`
- [x] `resultToResponse` — maps `HostIP`, `Hostname`, `Version`, `Banner`, real `ScanTime`

### 1b. Wire up admin stubs ✅

- [x] `stopWorker`, `updateConfig`, `getLogs` now return `501 Not Implemented`

### 1c. Complete scan scheduling (#333) ✅

- [x] Bounded concurrency host scanning via channel-based semaphore (default max 5)
- [x] `getNextRunTime` implemented using `cron.ParseStandard`

---

## Phase 2 — Testability & Service Interfaces ✅ COMPLETE

**Issues:** #335, #342, #223
**Goal:** Enable unit testing of handlers without a live database.

### 2a. Extract service interfaces ✅

- [x] 6 narrow interfaces in `interfaces.go`: `ScanStore`, `ScheduleStore`, `DiscoveryStore`, `HostStore`, `ProfileStore`, `NetworkServicer`
- [x] All handlers depend on interfaces, not concrete types

### 2b. Generate mocks ✅

- [x] `go:generate mockgen` directives on all interfaces
- [x] Mocks generated in `internal/api/handlers/mocks/`

### 2c. Convert integration-only handler tests to unit tests ✅

- [x] 25 mock-based tests for Scan, Schedule, Discovery, Host, Profile handlers
- [x] 31 additional mock-based subtests for Network handler (was the last gap)
- [x] Integration tests remain in separate files, skip without a DB

### 2d. Service-layer unit tests 🟡 PARTIAL

- [x] sqlmock-based unit tests added (8.2% → 29.0%)
- [ ] Target 60%+ — still needs more service method coverage

---

## Phase 3 — Code Organization & Splitting ✅ COMPLETE

**Issues:** #334, #337, #338, #340
**Goal:** Reduce file sizes, improve navigability, reduce merge conflicts.

### 3a. Split `handlers/admin.go` → #337 ✅

- [x] `admin_workers.go`, `admin_config.go`, `admin_logging.go` extracted

### 3b. Split `cmd/cli/networks.go` → #338 ✅

- [x] Split by layer: `networks.go` (commands), `networks_handlers.go`, `networks_helpers.go`, `networks_complete.go`

### 3c. Extract routing from `server.go` → #340 ✅

- [x] `routes.go` extracted with route group definitions

### 3d. Simplify `sanitizeDBError` → #336 ✅

- [x] Replaced with map lookup

---

## Phase 4 — Scan Concurrency & Queue (#221)

**Goal:** Prevent resource exhaustion under load.
**Estimate:** 1–2 weeks

### 4a. Scan execution queue

- Add a bounded worker pool wrapping `scanning.RunScanWithContext`.
- Configurable via `config.Scanning.MaxConcurrentScans` (default: 5).
- FIFO queue for pending scan requests.
- API returns `429 Too Many Requests` (or `503 Service Unavailable`) when queue is full.

### 4b. Scheduler integration

- `processHostsForScanning` feeds hosts into the queue instead of calling `RunScanWithContext` directly.
- Scheduled jobs wait for all queued scans to finish before marking the job complete.

### 4c. Observability

- Gauge metric: `scanorama_scan_queue_depth`
- Counter: `scanorama_scans_rejected_total`
- Gauge: `scanorama_scans_active`
- Health endpoint includes queue capacity info.

---

## Phase 5 — Test Coverage Push 🟡 IN PROGRESS

**Issues:** #223, #342, #339
**Goal:** All critical packages ≥ 60%, no package below 40%.
**Estimate:** 2 weeks (can overlap with other phases)

### Priority targets (original → current → target)

| Package | Original | Current | Target | Status |
|---|---:|---:|---:|---|
| `internal/services` | 8% | 29% | 60% | 🟡 sqlmock tests added; needs more |
| `internal/db` | 22% | 14% | 40% | 🟡 Integration tests separated; unit surface small |
| `internal/profiles` | 26% | 26% | 50% | ❌ Not started |
| `internal/daemon` | 32% | 43% | 50% | 🟡 Lifecycle tests added; close to target |
| `internal/scanning` | 48% | 48% | 60% | ❌ Not started |
| `internal/auth` | 52% | 46% | 60% | ❌ Not started |
| `internal/discovery` | 51% | 51% | 60% | ❌ Not started |

### Split large test files → #339

- [x] `scheduler_test.go` → `scheduler_test.go` + `scheduler_discovery_test.go` + `scheduler_scan_test.go`
- [x] `migration_test.go` → `migration_test.go` + `migration_schema_test.go`
- [ ] `profile_test.go` — shrank to 737 lines; arguably no longer needs splitting
- [x] `common_test.go` → `common_test.go` + `common_pagination_test.go` + `common_crud_test.go`

---

## Phase 6 — Configuration & Documentation ✅ COMPLETE

**Issues:** #220, #341, #343, #344
**Goal:** Centralize config validation; create deployment and architecture docs.

### 6a. Centralize configuration validation (#220) ✅

- [x] `internal/config/validate.go` — `ValidateConfig`, `ValidateAndNormalize`, per-section validators
- [x] `ValidationResult` type with errors + warnings (not just first-error-wins)
- [x] Normalization helpers (lowercase, path cleaning, trimming)
- [x] `internal/config/validate_test.go` — 70+ test cases, config coverage 66% → 86%

### 6b. Consolidate configuration documentation (#341) ✅

- [x] `docs/CONFIGURATION.md` — full reference with precedence, env vars, all settings, validation rules, hot-reloadable settings, example configs

### 6c. Deployment guide (#344) ✅

- [x] `docs/DEPLOYMENT.md` — Docker, binary, systemd, DB setup, security hardening

### 6d. Architecture diagrams (#343) ✅

- [x] `docs/technical/architecture/system-overview.md` — component diagram
- [x] `docs/technical/architecture/data-flow.md` — scan lifecycle, discovery pipeline
- [x] `docs/technical/architecture/scheduling-flow.md` — cron → job → scan → results, concurrency model, state machine

---

## Phase 7 — Observability & Hardening

**Issues:** #222
**Goal:** Production-grade monitoring and alerting.
**Estimate:** 1–2 weeks (lower priority, do when other phases are stable)

- Alerting rules for scan failure rate, DB connection loss, high error rate.
- OpenTelemetry tracing integration.
- Grafana dashboard templates in `docs/dashboards/`.
- API latency percentile metrics (p50/p95/p99).
- Database connection pool metrics.

---

## Sequencing Summary

```
Phase 0  ██████████  Current PR                     ✅ COMPLETE
Phase 1  ██████████  Critical functional gaps        ✅ COMPLETE
Phase 2  ████████░░  Service interfaces + mocks      ✅ COMPLETE (2d partial: services 29%)
Phase 3  ██████████  File splitting / reorg          ✅ COMPLETE
Phase 4  ░░░░░░░░░░  Scan concurrency queue          ❌ NOT STARTED
Phase 5  █████░░░░░  Test coverage push              🟡 IN PROGRESS (test splits done, coverage gaps remain)
Phase 6  ██████████  Config + docs                   ✅ COMPLETE
Phase 7  ░░░░░░░░░░  Observability                   ❌ NOT STARTED
```

### Immediate next steps:
1. **Phase 4** — Scan execution queue (most impactful feature work remaining)
2. **Phase 5** — Continue coverage push: `services` (29% → 60%), `daemon` (43% → 50%), `scanning` (48% → 60%)
3. **Phase 7** — Observability (when Phase 4 is stable)

---

## Issue Cross-Reference

| Phase | Issues |
|---|---|
| 1 | #333 |
| 2 | #223, #335, #342 |
| 3 | #334, #336, #337, #338, #340 |
| 4 | #221 |
| 5 | #223, #339, #342 |
| 6 | #220, #341, #343, #344 |
| 7 | #222 |