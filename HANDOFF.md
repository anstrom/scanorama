# Handoff — 2026-04-09

## Branch / main state

`main` is clean and green. Most recent merge: `feat: advanced host filtering (#607)`.

---

## v0.23 milestone — 11 / 11 closed (100 %) ✅

All issues closed. Milestone complete.

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
| #607 | Advanced filtering with compound conditions |

---

## What was done this session

### Closed issues #606 and #607

Both issues were still open despite their PRs (#626, #627) having been merged. Verified every acceptance criterion against the implementation before closing.

**#606 criteria verified:**
- User can select two discovery runs and see the comparison (`ComparePanel`, two dropdowns)
- Comparison shows new/gone/changed hosts (`DiffSection` reused)
- Toast notification on discovery completion with summary (`DiscoveryNotifications` in `RootLayout`)
- Clicking the toast navigates to `/discovery?job={id}`
- Edge cases handled: same-run short-circuits to all-unchanged; different-network returns 422

**#607 criteria verified:**
- Filter builder allows adding multiple conditions (`addCondition` / `addGroup`)
- AND/OR toggle at both top-level and sub-group (`OpToggle`)
- Server-side filtering: `ParseFilterExpr` → `TranslateFilterExpr` → parameterised SQL WHERE clause
- URL persistence: `serializeFilter` / `deserializeFilter` via base64url `?filter=` param
- Saved presets: `PresetsDropdown` with localStorage, capped at 20
- Port-based filtering: `open_port` field → correlated `EXISTS` subquery against `port_scans`
- Invalid filters: UI typed inputs prevent invalid construction; backend silently drops malformed JSON (intentional, consistent with other filter params)
- Responsive: `flex-wrap`, `hidden sm:inline`, `sm:ml-auto` throughout the builder

### README rewrite

`README.md` was comprehensively rewritten to describe the current system. Covers:
- Feature overview (discovery, scanning, host tracking, discovery changelog, advanced filtering, table power-ups, real-time, admin)
- Full requirements table (Go 1.26+, PostgreSQL 14+, nmap 7+, Node 20+)
- Quick start and development setup (`make dev`)
- Frontend section: stack table (React 19, Vite 6, TypeScript 5, Tailwind 4, TanStack Router/Query, Recharts, Zod, Vitest), all routes described
- CLI reference (apikeys, discover, scan, schedule, daemon)
- Full config example
- Complete API reference (all endpoints across System, Hosts, Scans, Discovery, Profiles, Schedules, Networks, Exclusions, API Keys, WebSocket)
- Advanced filter expression format documented with JSON example
- curl examples: submit a scan, compound filter query
- Docker compose stack
- Database migrations table (001–007)
- Project structure tree

---

## v0.24 — Tags, Groups & Organization (next)

**Theme:** Give users the tools to organise their infrastructure the way they think about it. The backend already stores a `tags` field on hosts and scans, but it is invisible in the UI.

### Feature scope

| Feature | Effort | Notes |
|---|---|---|
| Tag management UI | M | Tag input with autocomplete; add/remove tags on hosts and scans from detail panels and inline |
| Tag-based filtering | S | Filter any list view by tag; combine with existing filters |
| Host groups | M | Named groups (e.g. "Production web servers", "DMZ") either manually or by filter rule |
| Clone profile button | S | Backend `/profiles/{id}/clone` endpoint exists; just needs the UI button + rename dialog |
| Bulk tag from filter | S | "Tag all matching hosts" action when a filter is active |

### Backend work required

- `GET /api/v1/tags` endpoint returning all known tags with counts (needed for autocomplete)
- Tag filtering on `GET /api/v1/hosts` — extend `HostFilters` with a `Tags []string` field and add an `ANY($N)` clause to `buildHostFilters`
- Possibly a `host_groups` table + migration if groups need persistence beyond filter-based virtual groups

### Frontend work required

- `TagInput` component: comma / enter to add, backspace to remove, autocomplete via `useAllTags` hook
- Tags display in host list (chip list, truncated) and host detail panel
- `useAllTags` hook: `GET /api/v1/tags`
- `useUpdateHostTags` mutation (reuse existing `useUpdateHost`)
- Tag filter chip in the host filter bar (alongside status, OS, vendor)
- Host groups page or section (decision: separate route or a tab on Networks page?)
- Clone profile button in profile detail panel / profile list row action

### Suggested start

Tag input component + host detail integration is the smallest shippable increment — it unblocks tag filtering and bulk-tag, and doesn't require a new table. Do that first, then tag filtering, then groups.

---

## Dev environment notes

- **DB**: `make dev-db-up` starts `scanorama-dev-postgres` via `docker/docker-compose.dev.yml`.
- **nmap / privileges**: `make dev` runs the backend as root via `sudo -v` + `sudo -E`. No SUID needed. Set `daemon.user` / `daemon.group` in `config.yaml` to drop privileges post-init; leave blank in dev.
- **Frontend**: Vite dev server at `http://localhost:5173`, proxies `/api` and `/ws` to `:8080`.
- **WS discovery notifications**: fire only when the backend is running and a discovery job completes; safe to ignore in unit tests (the WS manager is `null` in test environments).
- **Test count**: 835 frontend tests (Vitest), all Go tests pass.

---

## Migrations on main

| # | File | Description |
|---|---|---|
| 001 | `001_initial_schema.sql` | Base schema |
| 002 | `002_host_targets.sql` | Host target associations |
| 003 | `003_discovery_network_link.sql` | Discovery ↔ network link |
| 004 | `004_host_status_model.sql` | `gone` status, `host_status_events` trigger |
| 005 | `005_response_time.sql` | RTT min/max/avg columns on hosts |
| 006 | `006_scan_duration.sql` | `scan_duration_ms` on `port_scans` |
| 007 | `007_timeout_events.sql` | `host_timeout_events` table |