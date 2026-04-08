# Handoff — 2026-04-08

## Branch / main state

`main` is clean and green. Most recent merge: `chore(deps): lock file maintenance`
(includes #603 column-visibility + keyboard nav + Go toolchain bump to 1.26.2).

---

## v0.23 milestone progress — 10 / 11 closed (91 %)

### Closed
| # | Title |
|---|---|
| #597 | Host status model: "gone" hosts and status transitions |
| #598 | Response time capture during discovery and scans |
| #599 | OUI vendor lookup integration |
| #600 | Universal table sorting for all list views |
| #601 | Multi-select + bulk delete |
| #602 | Bulk scan from host selection |
| #603 | Column visibility + keyboard navigation |
| #604 | Discovery diff view: new, gone, and changed hosts |
| #605 | Response time display + slow host detection |
| #606 | Discovery history comparison + notifications |

### Open
| # | Title | Effort | Notes |
|---|---|---|---|
| #607 | Advanced filtering | L | unblocked |

---

## What was done this session

### #606 — Discovery history comparison and completion notifications
Branch: `feat/606-discovery-compare-notifications` (ready to PR, not yet merged)

#### Backend
- **`internal/db/models.go`** — added `DiscoveryCompareDiff` struct (`RunAID`, `RunBID`, `NewHosts`, `GoneHosts`, `ChangedHosts`, `UnchangedCount`).
- **`internal/db/repository_discovery.go`** — added `queryDiffHostsTwo` helper (identical to `queryDiffHosts` but takes two UUIDs for queries referencing both runs); added `CompareDiscoveryRuns(ctx, jobA, jobB)`:
  - Loads both runs; `ErrNotFound` if either is missing.
  - Returns a descriptive error if networks differ (handler maps to 422).
  - Short-circuits to all-unchanged when `jobA == jobB`.
  - Uses `COALESCE(completed_at, created_at)` as the A-run boundary.
  - Derives `UnchangedCount = total − new − gone − changed` (clamped ≥ 0).
- **`internal/api/handlers/interfaces.go`** — `CompareDiscoveryRuns` added to `DiscoveryStore`.
- **`internal/api/handlers/discovery.go`** — `GetDiscoveryCompare` handler: validates `run_a`/`run_b` query params, parses UUIDs, maps different-network error → 422.
- **`internal/api/routes.go`** — `GET /discovery/compare` registered **before** `{id}` wildcards so gorilla/mux doesn't swallow "compare" as an ID.
- **`internal/api/handlers/mocks/mock_discovery_store.go`** — typed mock for `CompareDiscoveryRuns` added manually.
- **`internal/api/handlers/discovery_diff_mock_test.go`** — `TestDiscoveryHandler_GetDiscoveryCompare` with 9 subtests (missing params, invalid UUIDs, different networks → 422, not found → 404, DB error → 500, happy path → 200).

#### Frontend
- **`frontend/src/api/hooks/use-discovery.ts`** — added `DiscoveryCompareDiff` type and `useDiscoveryCompare(runA, runB, enabled)` hook (raw fetch to `/api/v1/discovery/compare`).
- **`frontend/src/router.tsx`** — `/discovery` route now renders `DiscoveryPage` (was `DiscoveryRedirect`); Zod `validateSearch` schema added for optional `job` query param so toast-click navigation can pre-select a job.
- **`frontend/src/routes/discovery.tsx`** — compare UI added:
  - "Compare runs" secondary button in toolbar (toggles `ComparePanel`).
  - `ComparePanel` component: two `<select>` dropdowns (completed jobs only, A = baseline, B = current), summary counts (new/gone/changed/unchanged), diff sections reusing existing `DiffSection`.
  - `DiscoveryPage` reads `search.job` URL param on mount to auto-open the detail panel for the specified job (used by toast click-through).
- **`frontend/src/components/layout/sidebar.tsx`** — "Discovery" nav item added between Hosts and Networks (icon: `ScanSearch`).
- **`frontend/src/components/toast-provider.tsx`** — `ToastItem` and `toast.success`/`toast.error` extended with optional `onClick?: () => void`; `ToastCard` is clickable (`cursor-pointer`) when `onClick` is provided.
- **`frontend/src/components/layout/root-layout.tsx`** — `DiscoveryNotifications` component added inside `<ToastProvider>`; subscribes to `discovery_update` WS messages, fires a clickable success toast on `status === "completed"` summarising new/gone/changed counts; clicking the toast navigates to `/discovery?job={id}` which auto-opens the Changes tab for that run.
- **`frontend/src/routes/discovery.test.tsx`** — 4 new tests: renders Compare button, shows/hides panel, shows run selectors.

All 793 frontend tests and all Go tests pass.

---

## Next: #607 Advanced filtering (L)

### Scope recap
**Frontend:**
- Filter builder on the hosts page: compound AND/OR conditions, fields (status, os_family, vendor, response_time, open port, first/last seen range, scan_count), per-condition operator (is / is not / contains / gt / lt / between).
- Filter state serialised into URL query params (shareable links).
- "Save filter" named presets (localStorage).
- Same pattern on the scans page (status, scan_type, date range, target).

**Backend:**
- `filter` query param on `GET /api/v1/hosts` accepting a structured JSON filter expression.
- Parse, validate, translate to SQL WHERE clauses.
- Port-based filtering requires joining scan results.

### Suggested approach
Start with the frontend filter builder UI (no backend needed yet) against the existing simple filters, then extend the backend to accept structured JSON. The builder is the largest single chunk; doing it first lets you validate the UX before wiring up the SQL.

---

## Dev environment notes

- **DB**: `make dev-db-up` starts `scanorama-dev-postgres` via `docker/docker-compose.dev.yml`.
- **nmap / privileges**: `make dev` runs the daemon as root via `sudo -v` + `sudo -E`. No SUID needed. Set `daemon.user` / `daemon.group` in `config.yaml` to drop privileges post-init; leave blank in dev.
- **WS discovery notifications**: fire only when the backend is running and a discovery job completes; safe to ignore in unit tests (the WS manager is `null` in test environments).

---

## Migrations on main
| # | File | Description |
|---|---|---|
| 001 | `001_initial_schema.sql` | Base schema |
| 002 | `002_host_targets.sql` | Host targets |
| 003 | `003_discovery_network_link.sql` | Discovery↔network link |
| 004 | `004_host_status_model.sql` | "gone" status, `host_status_events` trigger |
| 005 | `005_response_time.sql` | RTT min/max/avg columns on hosts |
| 006 | `006_scan_duration.sql` | `scan_duration_ms` on `port_scans` |
| 007 | `007_timeout_events.sql` | `host_timeout_events` table |