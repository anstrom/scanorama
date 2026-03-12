# Scanorama Improvement Plan

> **Status:** Draft — do not commit.
> **Last updated:** 2026-03-12
> **Branch context:** `fix/bug-fixes-pr1` (current PR finishes DB stubs + test coverage)

---

## Current State

| Package | Coverage | Notes |
|---|---:|---|
| `internal/services` | 8.2% | Almost all methods need a live DB; no service interfaces |
| `cmd/cli` | 14.6% | CLI commands largely untested |
| `internal/db` | 22.4% | New CRUD implementations from this PR still lack tests |
| `internal/profiles` | 25.7% | Profile CRUD untested |
| `internal/daemon` | 32.3% | Lifecycle methods untested |
| `internal/scanning` | 48.3% | Error/edge paths missing |
| `internal/auth` | 51.5% | |
| `internal/discovery` | 51.0% | |
| `internal/api/handlers` | 73.6% | Helper/validator coverage good; HTTP handler coverage relies on integration tests that skip without a DB |
| `internal/scheduler` | 71.5% | `processHostsForScanning` implemented but has no concurrency guards |

### Key Deficiencies

1. **Placeholder response converters** — `scheduleToResponse`, `discoveryToResponse`, `scanToResponse`, and `resultToResponse` return hardcoded/partial data instead of mapping from real DB types.
2. **Placeholder admin operations** — `stopWorker`, `updateConfig`, `getLogs` in `admin.go` are stubs that log and sleep.
3. **No service-layer interfaces** — handlers take concrete `*services.NetworkService`, `*db.DB`, etc., making unit testing impossible without a live database.
4. **No scan concurrency controls** — nothing prevents resource exhaustion when many scans run at once (#221).
5. **Large files** — `admin.go` (1 283 lines), `cmd/cli/networks.go` (1 139 lines), `server.go` (881 lines).
6. **Missing docs** — no deployment guide, no architecture diagrams.

---

## Phase 0 — Finish Current PR

**Goal:** Land `fix/bug-fixes-pr1` with clean history.

What remains:

- [x] Implement discovery job DB CRUD (`networks.go`)
- [x] Implement schedule DB CRUD (`scheduled_jobs.go`)
- [x] Refactor `UpdateScan` + harden `extractScanData`
- [x] Unit tests for `validateCreateNetworkRequest`, `validateUpdateNetworkRequest`
- [x] Unit tests for `getScanFilters`, `scanToHostScanResponse`
- [x] Extended `validateNetworkConfig` coverage (method + exclusion branches)
- [x] Fix pre-existing `DeleteNetwork_NotFound` assertion

---

## Phase 1 — Critical Functional Gaps

**Issues:** #333
**Goal:** Make features that look complete actually work.
**Estimate:** 1 week

### 1a. Fix placeholder response converters

The following functions return fake data and must map from real DB types:

| File | Function | What's wrong |
|---|---|---|
| `handlers/schedule.go` | `scheduleToResponse` | Ignores input entirely, returns hardcoded values |
| `handlers/discovery.go` | `discoveryToResponse` | Same — ignores input |
| `handlers/scan.go` | `scanToResponse` | Partially maps `db.Scan` but drops `Targets`, hardcodes `Progress` |
| `handlers/scan.go` | `resultToResponse` | Drops `HostIP`, `Hostname`, `Version`, `Banner`; fakes `ScanTime` |

Each of these is a one-function fix. Write the mapping, add a unit test.

### 1b. Wire up admin stubs

`handlers/admin.go` has three placeholder helpers:

- `stopWorker` — sleeps and logs. Needs to call into the worker manager.
- `updateConfig` — logs and returns nil. Needs to call `config.Update` + apply hot-reloadable values.
- `getLogs` — returns canned data. Needs to read from the structured log sink.

These can be deferred if admin endpoints aren't user-facing yet, but they should at least return `501 Not Implemented` instead of pretending to succeed.

### 1c. Complete scan scheduling (#333) — CRITICAL

`processHostsForScanning` is implemented (scans run), but the scheduler still has issues:

- No concurrency limit on how many hosts are scanned in parallel within a single job.
- No scan result persistence linked back to the scheduled job.
- `getNextRunTime` in `cmd/cli/schedule.go` is a stub that always returns `now + 1h`.

Deliverables:
- Batch host scanning with configurable parallelism (e.g., `config.Scanning.MaxConcurrent`).
- Persist scan results to the DB linked to the originating scheduled job.
- Implement `getNextRunTime` using the cron library already in `go.mod`.

---

## Phase 2 — Testability & Service Interfaces

**Issues:** #335, #342, #223
**Goal:** Enable unit testing of handlers without a live database.
**Estimate:** 1–2 weeks

### 2a. Extract service interfaces

Define interfaces in the handler package (or a shared `ports` package) for each service dependency:

```go
// internal/api/handlers/interfaces.go

type NetworkServicer interface {
    ListNetworks(ctx context.Context, activeOnly bool) ([]*db.Network, error)
    GetNetworkByID(ctx context.Context, id uuid.UUID) (*services.NetworkWithExclusions, error)
    CreateNetwork(ctx context.Context, name, cidr, description, method string, active, scanEnabled bool) (*db.Network, error)
    UpdateNetwork(ctx context.Context, id uuid.UUID, name, cidr, description, method string, active bool) (*db.Network, error)
    DeleteNetwork(ctx context.Context, id uuid.UUID) error
    // ... etc
}
```

Update `NetworkHandler` to depend on `NetworkServicer` instead of `*services.NetworkService`. Do the same for scan, host, discovery, schedule, and profile handlers.

### 2b. Generate mocks

Use `go generate` + `mockgen` (already in `go.mod` via `go.uber.org/mock`) to produce mock implementations. Place under `internal/api/handlers/mocks/`.

### 2c. Convert integration-only handler tests to unit tests

With mocks available, rewrite the skipped integration tests as unit tests that run without a database. Keep the integration tests as a separate `_integration_test.go` build-tagged suite.

### 2d. Service-layer unit tests

With the interfaces defined, also mock `*db.DB` at the service layer and write unit tests for `services.NetworkService` methods (currently 8.2% → target 60%+).

---

## Phase 3 — Code Organization & Splitting

**Issues:** #334, #337, #338, #340
**Goal:** Reduce file sizes, improve navigability, reduce merge conflicts.
**Estimate:** 1 week

### 3a. Split `handlers/admin.go` (1 283 lines) → #337

- `admin_workers.go` — `GetWorkerStatus`, `StopWorker`, `stopWorker`
- `admin_config.go` — `GetConfig`, `UpdateConfig`, `validateConfigUpdate`, `validateConfig*`, `extractConfig*`, `updateConfig`, `getCurrentConfig`, `isRestartRequired`
- `admin_logging.go` — `GetLogs`, `getLogs`, `matchesFilters`, `containsRecursive`, `contains`

### 3b. Split `cmd/cli/networks.go` (1 139 lines) → #338

- `networks_list.go` — `listNetworksCmd`
- `networks_add.go` — `addNetworkCmd`
- `networks_delete.go` — `deleteNetworkCmd`
- `networks_scan.go` — `scanNetworkCmd`

### 3c. Extract routing from `server.go` → #340

- `server.go` — server lifecycle, middleware setup
- `routes.go` — `registerRoutes`, route group definitions

### 3d. Simplify `sanitizeDBError` → #336

Replace the 75-line switch with a map lookup:

```go
var pgErrorMap = map[string]struct {
    code errors.ErrorCode
    msg  string
}{
    "23505": {errors.CodeConflict, "resource already exists"},
    "23503": {errors.CodeValidation, "referenced resource not found"},
    // ...
}
```

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

## Phase 5 — Test Coverage Push

**Issues:** #223, #342, #339
**Goal:** All critical packages ≥ 60%, no package below 40%.
**Estimate:** 2 weeks (can overlap with other phases)

### Priority targets (current → target)

| Package | Current | Target | Strategy |
|---|---:|---:|---|
| `internal/services` | 8% | 60% | Depends on Phase 2 interfaces/mocks |
| `internal/db` | 22% | 40% | Integration tests with test DB; unit tests for helpers |
| `internal/profiles` | 26% | 50% | Unit tests for validation; integration for CRUD |
| `internal/daemon` | 32% | 50% | Mock DB + config; test lifecycle methods |
| `internal/scanning` | 48% | 60% | Error paths, timeout scenarios, result parsing |
| `internal/auth` | 52% | 60% | Edge cases in token validation, middleware |
| `internal/discovery` | 51% | 60% | Additional discovery methods, large-network edge cases |

### Split large test files → #339

- `scheduler_test.go` (2 215 lines) → `scheduler_test.go` + `scheduler_discovery_test.go` + `scheduler_scan_test.go`
- `migration_test.go` (1 639 lines) → `migration_test.go` + `migration_schema_test.go`
- `profile_test.go` (1 529 lines) → `profile_test.go` + `profile_validation_test.go`
- `common_test.go` (1 512 lines) → `common_test.go` + `common_pagination_test.go` + `common_crud_test.go`

---

## Phase 6 — Configuration & Documentation

**Issues:** #220, #341, #343, #344
**Goal:** Centralize config validation; create deployment and architecture docs.
**Estimate:** 1–2 weeks

### 6a. Centralize configuration validation (#220)

- Create `internal/config/validate.go` with validators for each section (API, DB, scanning, daemon, logging).
- Apply at all entry points: config file load, CLI flags, API admin endpoints.
- Fill in defaults and normalize values (e.g., durations, paths).

### 6b. Consolidate configuration documentation (#341)

- Document precedence: defaults → config file → env vars → CLI flags → API overrides.
- Create `docs/CONFIGURATION.md` with a full reference of every setting.

### 6c. Deployment guide (#344)

- `docs/DEPLOYMENT.md`: Docker, binary, systemd
- Database setup, migrations, backup
- Security hardening, TLS, API key rotation

### 6d. Architecture diagrams (#343)

- `docs/technical/architecture/system-overview.md` — component diagram
- `docs/technical/architecture/data-flow.md` — scan lifecycle, discovery pipeline
- `docs/technical/architecture/scheduling-flow.md` — cron → job → scan → results

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
Phase 0  ██████████  Current PR (done)
Phase 1  ██████████  Critical functional gaps       ~1 week
Phase 2  ██████████  Service interfaces + mocks     ~1–2 weeks
Phase 3  ██████████  File splitting / reorg         ~1 week (can overlap with 2)
Phase 4  ██████████  Scan concurrency queue         ~1–2 weeks
Phase 5  ██████████  Test coverage push             ~2 weeks (overlaps with 2–4)
Phase 6  ██████████  Config + docs                  ~1–2 weeks (overlaps with 4–5)
Phase 7  ██████████  Observability                  ~1–2 weeks (when stable)
```

Phases 1–3 are the immediate next steps after the current PR lands.
Phase 4 is the most impactful feature work.
Phases 5–7 are ongoing and can be parallelized.

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