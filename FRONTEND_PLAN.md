# Scanorama Frontend Development Plan

> **Status:** Draft — not for git commit  
> **Date:** 2026-03-14  
> **Starting point:** v0.13.0 (pre-frontend milestone)

---

## Table of Contents

1. [Goals & Constraints](#1-goals--constraints)
2. [Framework Decision](#2-framework-decision)
3. [Look & Feel](#3-look--feel)
4. [Project Structure](#4-project-structure)
5. [API Client Strategy](#5-api-client-strategy)
6. [Local Development & Testing](#6-local-development--testing)
7. [Iteration Plan](#7-iteration-plan)
8. [Open Questions](#8-open-questions)

---

## 1. Goals & Constraints

### What the frontend needs to do

Scanorama is a network scanning and discovery tool. The frontend is an **operator dashboard** — used by network admins and security engineers to monitor their infrastructure, kick off scans, and investigate results. It is not a consumer-facing app with millions of users; it's a professional tool that prioritizes **clarity, density, and speed** over flash.

### Core use cases

| Priority | Use Case |
|----------|----------|
| P0 | View discovered hosts and their open ports/services |
| P0 | Start/stop scans and monitor progress in real time |
| P0 | View and manage networks (add, exclude ranges, enable/disable) |
| P1 | Manage scan profiles and schedules |
| P1 | Dashboard with aggregate stats (host count, scan activity, network health) |
| P1 | View scan history and compare results over time |
| P2 | Admin panel (worker status, config, logs) |
| P2 | WebSocket-powered live scan progress |

### Constraints

- **Single-page app** served by the Go backend (embedded in the binary for production)
- **No SSR needed** — this is a tool, not a content site
- **API is the contract** — 48 REST endpoints already documented in OpenAPI/Swagger 2.0
- **Auth is optional** — API key auth exists but is off by default in dev
- **Target browsers:** Latest Chrome, Firefox, Safari (no IE, no legacy)
- **Solo developer / small team** — framework choice should minimize boilerplate and maximize velocity

---

## 2. Framework Decision

### Candidates evaluated

| | React + Vite | Vue 3 + Vite | Svelte 5 + Vite |
|---|---|---|---|
| **Ecosystem size** | Largest | Large | Growing |
| **Component libraries** | Extensive (shadcn, Radix, etc.) | Good (PrimeVue, Naive UI) | Limited |
| **TypeScript** | First-class | First-class | First-class |
| **Bundle size** | ~45 KB (React + ReactDOM) | ~33 KB | ~8 KB |
| **Learning curve** | Moderate (hooks, JSX) | Low (SFC, template syntax) | Low (runes) |
| **Data table ecosystem** | TanStack Table (excellent) | TanStack Table / AG Grid | TanStack Table |
| **Existing project assets** | Reference React hooks in `docs/api/client.js` | Reference Vue composables in `docs/api/client.js` | None |
| **OpenAPI codegen** | `openapi-typescript` + `openapi-fetch` | Same tooling works | Same tooling works |
| **Go embed compatibility** | `vite build` → `dist/` → `embed.FS` | Same | Same |

### Decision: **React 19 + Vite 6 + TypeScript**

**Rationale:**

1. **Component library:** We're going with [shadcn/ui](https://ui.shadcn.com/) — it gives us unstyled, composable Radix-based primitives with Tailwind styling. Perfect for a data-dense dashboard. No runtime dependency, components are copied into the project and owned by us.

2. **Data tables:** TanStack Table v8 has first-class React support and handles sorting, filtering, pagination, column resizing, and virtual scrolling — all critical for host/port/scan lists that can have thousands of rows.

3. **Existing assets:** The reference client in `docs/api/client.js` already has React hooks (`useScans`, `useHosts`) and the quickstart guide has React component examples. Less throwaway work.

4. **Ecosystem depth:** For a data-heavy dashboard, React's ecosystem is unmatched — charting (Recharts), maps, virtualization, accessibility.

5. **Hiring/collaboration:** If the team grows, React developers are the easiest to find.

### Key dependencies

| Package | Purpose | Version |
|---------|---------|---------|
| `react` / `react-dom` | UI framework | 19.x |
| `vite` | Build tool / dev server | 6.x |
| `typescript` | Type safety | 5.x |
| `tailwindcss` | Utility-first CSS | 4.x |
| `@tanstack/react-router` | File-based routing, type-safe | 1.x |
| `@tanstack/react-query` | Server state management, caching, polling | 5.x |
| `@tanstack/react-table` | Headless data tables | 8.x |
| `openapi-typescript` | Generate types from Swagger spec | 7.x |
| `openapi-fetch` | Type-safe fetch client from generated types | 0.x |
| `recharts` | Charting (dashboard stats) | 2.x |
| `lucide-react` | Icons | latest |
| `tailwind-merge` + `clsx` | Conditional class merging (shadcn pattern) | latest |
| `zod` | Form validation | 3.x |

**Not using:** Redux (TanStack Query handles server state; React context handles the little local state we need), Next.js (no SSR needed), Storybook (overkill for our scale at this stage).

---

## 3. Look & Feel

### Design direction: **Dense, professional, dark-first**

This is a network operations tool. Think Grafana, Datadog, or pgAdmin — not a marketing site.

### Design principles

1. **Information density over whitespace.** Operators want to see data, not padding. Tables should be tight. Cards should pack information.

2. **Dark mode is the default.** Network tools are used in SOCs and server rooms. Light mode available but dark is primary.

3. **Color is functional, not decorative.** Green = healthy/up/open. Red = failed/down/critical. Yellow = warning/pending. Blue = informational/running. Gray = inactive/unknown.

4. **Typography is for scanning, not reading.** Monospace for IPs, ports, CIDRs, timestamps. Sans-serif (Inter) for labels and headings. Small font sizes are fine — this audience is comfortable with dense UIs.

5. **Progressive disclosure.** Summary → detail. Table row → slide-out panel or detail page. Don't dump everything on screen at once, but don't hide it behind too many clicks either.

### Color system

```
Background:     hsl(222, 47%, 6%)     — near-black navy
Surface:        hsl(222, 47%, 9%)     — card/panel background  
Surface raised: hsl(222, 47%, 12%)    — hover states, modals
Border:         hsl(222, 20%, 18%)    — subtle borders

Text primary:   hsl(210, 40%, 96%)    — almost white
Text secondary: hsl(215, 20%, 60%)    — muted labels
Text muted:     hsl(215, 15%, 40%)    — timestamps, metadata

Accent:         hsl(217, 91%, 60%)    — primary actions, links
Success:        hsl(142, 71%, 45%)    — up, healthy, open
Warning:        hsl(38, 92%, 50%)     — pending, degraded
Danger:         hsl(0, 84%, 60%)      — down, failed, error
Info:           hsl(199, 89%, 48%)    — running, informational
```

These will be defined as CSS custom properties and mapped to Tailwind's color config so shadcn/ui components inherit them automatically.

### Layout

```
┌──────────────────────────────────────────────────┐
│  ┌──────┐  Scanorama          [search] [?] [⚙]  │  ← Top bar (48px)
├──┤      ├────────────────────────────────────────┤
│  │ Nav  │                                        │
│  │      │  Main content area                     │
│  │ □ Dash│  (tables, detail views, forms)        │
│  │ □ Scan│                                       │
│  │ □ Host│                                       │  ← Sidebar (220px, collapsible to 48px icons)
│  │ □ Net │                                       │
│  │ □ Disc│                                       │
│  │ □ Prof│                                       │
│  │ □ Schd│                                       │
│  │      │                                        │
│  │ ──── │                                        │
│  │ □ Admn│                                       │
│  └──────┘                                        │
└──────────────────────────────────────────────────┘
```

- **Sidebar navigation** with icon + label, collapsible to icon-only
- **Top bar** with global search, help, and settings
- **Content area** fills remaining space, scrolls independently
- **No footer** — every pixel is for content

### Key UI patterns

| Pattern | When to use | Implementation |
|---------|-------------|----------------|
| **Data table** | Lists of hosts, scans, networks, etc. | TanStack Table + custom columns |
| **Stat cards** | Dashboard KPIs (total hosts, active scans, etc.) | Grid of small cards with icon + number + trend |
| **Detail panel** | Viewing a single host, scan, or network | Either slide-over panel or dedicated route |
| **Status badge** | Scan status, host status, port state | Colored pill with label |
| **Action menu** | Row-level actions (start, stop, delete, etc.) | Dropdown menu on row hover or ⋮ button |
| **Empty state** | No data yet | Illustration + CTA to create first item |
| **Loading skeleton** | Data fetching | Animated shimmer placeholders matching layout |
| **Toast notifications** | Action confirmations, errors | Bottom-right stack, auto-dismiss |
| **Command palette** | Power-user quick navigation | Cmd+K modal with fuzzy search |

---

## 4. Project Structure

```
frontend/
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.ts
├── postcss.config.js
├── components.json                 ← shadcn/ui config
│
├── public/
│   └── favicon.svg
│
├── src/
│   ├── main.tsx                    ← Entry point
│   ├── app.tsx                     ← Root layout + router
│   ├── globals.css                 ← Tailwind directives + CSS vars
│   │
│   ├── api/
│   │   ├── client.ts              ← openapi-fetch configured instance
│   │   ├── types.ts               ← Generated from OpenAPI spec (do not edit)
│   │   └── hooks/
│   │       ├── use-scans.ts       ← TanStack Query wrappers
│   │       ├── use-hosts.ts
│   │       ├── use-networks.ts
│   │       ├── use-discovery.ts
│   │       ├── use-profiles.ts
│   │       ├── use-schedules.ts
│   │       └── use-system.ts      ← health, status, version
│   │
│   ├── components/
│   │   ├── ui/                    ← shadcn/ui primitives (button, input, table, etc.)
│   │   ├── layout/
│   │   │   ├── sidebar.tsx
│   │   │   ├── topbar.tsx
│   │   │   └── root-layout.tsx
│   │   ├── data-table/
│   │   │   ├── data-table.tsx     ← Reusable TanStack Table wrapper
│   │   │   ├── column-header.tsx
│   │   │   ├── pagination.tsx
│   │   │   └── toolbar.tsx
│   │   ├── status-badge.tsx
│   │   ├── stat-card.tsx
│   │   └── command-palette.tsx
│   │
│   ├── routes/                    ← One file per route (TanStack Router)
│   │   ├── __root.tsx
│   │   ├── index.tsx              ← Dashboard
│   │   ├── scans/
│   │   │   ├── index.tsx          ← Scan list
│   │   │   └── $scanId.tsx        ← Scan detail
│   │   ├── hosts/
│   │   │   ├── index.tsx
│   │   │   └── $hostId.tsx
│   │   ├── networks/
│   │   │   ├── index.tsx
│   │   │   └── $networkId.tsx
│   │   ├── discovery/
│   │   │   └── index.tsx
│   │   ├── profiles/
│   │   │   └── index.tsx
│   │   ├── schedules/
│   │   │   └── index.tsx
│   │   └── admin/
│   │       └── index.tsx
│   │
│   ├── lib/
│   │   ├── utils.ts               ← cn() helper, formatters
│   │   ├── constants.ts           ← Status colors, port labels, etc.
│   │   └── ws.ts                  ← WebSocket connection manager
│   │
│   └── hooks/
│       ├── use-theme.ts           ← Dark/light toggle
│       └── use-debounce.ts
│
└── tests/
    ├── setup.ts                   ← Vitest + Testing Library config
    ├── msw/
    │   ├── handlers.ts            ← MSW mock handlers for API
    │   └── server.ts              ← MSW server setup
    └── components/
        └── ...                    ← Component tests
```

### Go embed integration (production)

The Go server will embed the built frontend:

```go
//go:embed frontend/dist/*
var frontendFS embed.FS

// Serve at root, with SPA fallback to index.html for client-side routing
```

This is a later iteration — during development, Vite's dev server proxies API calls to the Go backend.

---

## 5. API Client Strategy

### Type generation from OpenAPI spec

```bash
# Generate TypeScript types from the existing Swagger spec
npx openapi-typescript ../docs/swagger/swagger.yaml -o src/api/types.ts
```

This gives us fully typed request/response interfaces derived directly from the backend's Swagger annotations. When the backend changes, we regenerate.

### Type-safe fetch client

```typescript
// src/api/client.ts
import createClient from 'openapi-fetch';
import type { paths } from './types';

export const api = createClient<paths>({
  baseUrl: import.meta.env.VITE_API_BASE_URL ?? '/api/v1',
});
```

Usage is fully typed — autocomplete on paths, params, and response shapes:

```typescript
const { data, error } = await api.GET('/hosts', {
  params: { query: { page: 1, page_size: 20 } },
});
// data is typed as PaginatedHostsResponse
```

### TanStack Query wrappers

Every API resource gets a hooks file that wraps the fetch client with TanStack Query for caching, polling, and mutations:

```typescript
// src/api/hooks/use-hosts.ts
export function useHosts(params: HostListParams) {
  return useQuery({
    queryKey: ['hosts', params],
    queryFn: () => api.GET('/hosts', { params: { query: params } }),
    select: (res) => res.data,
  });
}

export function useHost(id: string) {
  return useQuery({
    queryKey: ['hosts', id],
    queryFn: () => api.GET('/hosts/{id}', { params: { path: { id } } }),
    select: (res) => res.data,
  });
}
```

### Why this approach

- **Single source of truth:** Backend Swagger annotations → generated types → typed client → typed hooks. No manual type duplication.
- **Regeneration is cheap:** One command after backend changes. Compiler catches mismatches.
- **No code generation runtime:** `openapi-fetch` is ~5 KB, uses native `fetch`, and the types are purely compile-time.

---

## 6. Local Development & Testing

### Development environment setup

```bash
# Terminal 1: Start backend + database
cd /path/to/scanorama
make db-up                          # Postgres via Docker
go run ./cmd/scanorama api \
  --config config/environments/config.local.yaml

# Terminal 2: Start frontend dev server
cd frontend
npm install
npm run dev                         # Vite on port 5173, proxies /api → :8080
```

Vite config for API proxying:

```typescript
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
});
```

### Testing strategy — three layers

#### Layer 1: Component tests (Vitest + Testing Library)

Fast, isolated, no backend needed. Mock API responses with MSW (Mock Service Worker).

```bash
npm run test              # Run all tests
npm run test:watch        # Watch mode during development
npm run test:coverage     # Coverage report
```

What we test:
- Components render correctly with various data states (empty, loading, error, data)
- User interactions (click buttons, fill forms, sort tables)
- Conditional rendering (status badges show correct colors, disabled states)

MSW intercepts `fetch` calls and returns canned responses — tests hit the same code paths as production but with deterministic data.

```typescript
// tests/msw/handlers.ts
import { http, HttpResponse } from 'msw';

export const handlers = [
  http.get('/api/v1/hosts', () =>
    HttpResponse.json({
      data: [{ id: '...', ip_address: '192.168.1.1', status: 'up', ... }],
      pagination: { page: 1, page_size: 20, total_items: 1, total_pages: 1 },
    })
  ),
  // ... handlers for each endpoint
];
```

#### Layer 2: Integration tests (Playwright)

End-to-end browser tests against the real frontend + real (or mocked) backend.

```bash
npm run test:e2e          # Headless Chromium
npm run test:e2e:ui       # Interactive Playwright UI
```

For local development, these run against Vite dev server + Go backend + test database. In CI, we can run against a Docker Compose stack or use MSW in the browser for hermetic tests.

What we test:
- Full user flows (navigate to hosts page → click host → see detail → go back)
- Form submissions (create network → verify it appears in list)
- Pagination and filtering work end-to-end
- Error states display correctly when API returns errors

#### Layer 3: Visual smoke tests (manual, supported by Playwright screenshots)

Not automated initially, but Playwright can capture screenshots for manual review:

```bash
npm run test:e2e -- --update-snapshots
```

Useful when refactoring layout or theme without wanting to break visual appearance.

### Testing conventions

- **Filename pattern:** `*.test.tsx` for component tests, `*.spec.ts` for e2e
- **One test file per component/route** — mirrors `src/` structure in `tests/`
- **Minimum coverage target:** 60% for components, 80% for hooks/utils
- **CI integration:** Component tests run on every PR (fast, no infra needed). E2e tests run on merge to main.

### Seed data for local development

The Go backend should have a `make seed` target (or we create one) that populates the dev database with realistic test data:

- 3–5 networks (10.0.0.0/24, 192.168.1.0/24, 172.16.0.0/16, etc.)
- 50–100 hosts with varied statuses, OS fingerprints, and port profiles
- 10–20 completed scans with results
- A few active schedules
- Some discovery jobs in various states

This makes local frontend development immediately useful — you see real-looking data, not empty states.

---

## 7. Iteration Plan

Each iteration is a **shippable increment** — it adds visible, testable functionality. Iterations are scoped to roughly 2–4 days of work each.

### Iteration 0: Scaffold & Prove the Stack

**Goal:** Empty app that builds, runs, proxies to the backend, and proves every layer of the stack works.

- [ ] `npm create vite@latest frontend -- --template react-ts`
- [ ] Install Tailwind 4, configure with dark theme color system
- [ ] Install and configure shadcn/ui (`npx shadcn@latest init`)
- [ ] Set up Vite proxy to Go backend
- [ ] Generate TypeScript types from Swagger spec (`openapi-typescript`)
- [ ] Set up `openapi-fetch` client with one test call
- [ ] Set up TanStack Query provider
- [ ] Set up TanStack Router with root layout (sidebar placeholder + content area)
- [ ] Add Vitest + Testing Library + MSW
- [ ] One smoke test: root layout renders, health endpoint returns data
- [ ] `npm run build` produces a `dist/` that can be served

**Deliverable:** App shell with sidebar nav (links but placeholder pages), dark theme, hitting `/api/v1/health` and showing the result.

---

### Iteration 1: Dashboard

**Goal:** Landing page with aggregate stats and at-a-glance system health.

- [ ] `StatCard` component (icon, label, value, optional trend)
- [ ] Fetch `/health`, `/status`, `/version` — display in top cards
- [ ] Fetch `/networks/stats` — display network/host/exclusion counts
- [ ] Fetch `/scans?page_size=5&sort=-created_at` — show recent scans table
- [ ] Fetch `/hosts?status=up&page_size=1` — show active host count
- [ ] Auto-refresh via TanStack Query `refetchInterval` (30s)
- [ ] Loading skeletons for all cards
- [ ] Tests for StatCard, dashboard data fetching

**Deliverable:** A dashboard that immediately tells you "the system is healthy, there are N hosts across M networks, and here are the last 5 scans."

---

### Iteration 2: Hosts — The Primary View

**Goal:** The most-used page. Browse, search, sort, and inspect hosts.

- [ ] Reusable `DataTable` component (wrapping TanStack Table)
  - [ ] Column sorting (click headers)
  - [ ] Pagination controls (page size selector, prev/next/jump)
  - [ ] Column visibility toggle
  - [ ] Loading state with skeletons
- [ ] Hosts list page using `DataTable`
  - [ ] Columns: IP, hostname, status, OS, MAC, open ports count, last seen, first seen
  - [ ] `StatusBadge` component (up = green, down = red, unknown = gray)
  - [ ] Search/filter by IP, hostname, status
  - [ ] Click row → navigate to host detail
- [ ] Host detail page
  - [ ] Header with IP, hostname, status, OS info
  - [ ] Port/service table (from scan results)
  - [ ] Scan history for this host
  - [ ] Host metadata (vendor, MAC, response time, discovery count)
- [ ] Tests for DataTable, StatusBadge, host list, host detail

**Deliverable:** You can browse all hosts, search for one, click into it, and see its ports and scan history.

---

### Iteration 3: Scans — CRUD & Real-Time Progress

**Goal:** View scans, create new ones, and monitor running scans.

- [ ] Scan list page (DataTable with status, targets, progress, timestamps)
- [ ] Create scan form
  - [ ] Target input (multi-value: IPs, CIDRs, hostnames)
  - [ ] Profile selector (dropdown populated from `/profiles`)
  - [ ] Name and description fields
  - [ ] Zod validation
- [ ] Scan detail page
  - [ ] Status + progress bar (for running scans)
  - [ ] Results table (hosts found, ports scanned)
  - [ ] Start/stop action buttons
  - [ ] Error display for failed scans
- [ ] Polling for running scans (`refetchInterval: 2000` when status === 'running')
- [ ] Toast notifications for scan state changes
- [ ] Tests for scan form validation, scan list, progress display

**Deliverable:** Full scan lifecycle — create, monitor, view results, start/stop.

---

### Iteration 4: Networks & Exclusions ✅ DONE

**Goal:** Manage networks and their exclusion ranges.

- [x] Networks list page (CIDR, host count, active hosts, last discovery, status)
- [x] Create network form (name, CIDR, discovery method, description)
- [x] Network detail page
  - [x] Network info card
  - [ ] Hosts in this network (filtered host table) — skipped: API has no `network_id` host filter
  - [x] Exclusions sub-table
  - [x] Enable/disable toggle
  - [x] Rename action (inline in panel header)
- [x] Exclusion management
  - [x] Add exclusion form (CIDR, reason)
  - [x] Delete exclusion (with confirmation dialog)
  - [x] Global exclusions page at `/exclusions`
- [x] Tests for network CRUD, exclusion management

**Deliverable:** Complete network management — add networks, define exclusions, see which hosts belong to which network.

**Implementation notes:**
- `src/api/hooks/use-networks.ts` — 13 hooks (5 queries + 8 mutations)
- `src/components/add-network-modal.tsx` — Create network modal
- `src/components/add-exclusion-modal.tsx` — Add exclusion modal (network-scoped or global)
- `src/routes/networks.tsx` — Full Networks page with `NetworkDetailPanel` and `ExclusionsSection`
- `src/routes/exclusions.tsx` — Global exclusions page at `/exclusions`
- Exclusions nav item added to sidebar (`ShieldOff` icon)
- 430 tests passing across 20 test files

---

### Iteration 5: Discovery, Profiles & Schedules

**Goal:** Fill in the remaining CRUD resources.

- [ ] Discovery jobs page (list, create, start/stop, view results)
- [ ] Profiles page (list, create, edit, delete)
  - [ ] Profile form with scan type, timing, ports, scripts, OS targeting
- [ ] Schedules page (list, create, edit, delete, enable/disable)
  - [ ] Cron expression input with human-readable preview
  - [ ] Next-run display
  - [ ] Link to associated profile and targets
- [ ] Tests for each resource page

**Deliverable:** All seven resource domains are accessible and manageable from the frontend.

---

### Iteration 6: Polish & Power Features

**Goal:** Quality-of-life features that make the tool feel professional.

- [ ] Command palette (Cmd+K) — fuzzy search across all resources
- [ ] Global search in top bar
- [ ] Light/dark theme toggle (persist in localStorage)
- [ ] Keyboard shortcuts (n = new, / = search, etc.)
- [ ] Responsive sidebar (collapse to icons on narrow screens)
- [ ] Empty states with illustrations and CTAs
- [ ] Breadcrumbs on detail pages
- [ ] Relative timestamps ("3 minutes ago") with absolute on hover
- [ ] Monospace formatting for IPs, CIDRs, ports, MACs everywhere

**Deliverable:** A polished, professional tool that feels good to use daily.

---

### Iteration 7: Real-Time & Admin

**Goal:** WebSocket integration and admin panel.

- [ ] WebSocket connection manager (`src/lib/ws.ts`)
  - [ ] Auto-reconnect with exponential backoff
  - [ ] Connection status indicator in top bar
- [ ] Live scan progress via WebSocket (replace polling)
- [ ] Admin panel
  - [ ] Worker status (active/idle, queue depth, processed jobs)
  - [ ] System config viewer
  - [ ] Log viewer (streaming or paginated)
- [ ] Tests for WebSocket reconnection logic, admin panel rendering

**Deliverable:** Real-time updates and full admin visibility.

---

### Iteration 8: Production Integration

**Goal:** Embed in Go binary, optimize, and ship.

- [ ] Go `embed.FS` integration for `frontend/dist/`
- [ ] SPA fallback handler (serve `index.html` for unknown routes)
- [ ] Production build optimization (code splitting, chunk hashing, compression)
- [ ] Content-Security-Policy headers for embedded frontend
- [ ] Update Dockerfile to include frontend build step
- [ ] Update Makefile with `make frontend` and `make build-all` targets
- [ ] Playwright e2e tests running in CI against Docker Compose stack
- [ ] Performance audit (Lighthouse, bundle size analysis)

**Deliverable:** `go build` produces a single binary that serves both the API and the frontend. Ready for release.

---

## 8. Open Questions

These need decisions before or during early iterations:

| # | Question | Options | Leaning |
|---|----------|---------|---------|
| 1 | **Router choice:** TanStack Router vs React Router v7? | TanStack has type-safe routes and better DX. React Router is more established. | TanStack Router — type safety from route params through to components is worth it for a dashboard with many `:id` routes |
| 2 | **Swagger 2.0 vs OpenAPI 3.x?** The backend currently generates Swagger 2.0. `openapi-typescript` works with both but OpenAPI 3.x is better supported everywhere. | Convert spec to 3.x during frontend setup, or update `swag` config to emit 3.x. | Convert to OpenAPI 3.x — one-time effort, better tooling support going forward |
| 3 | **Where does the frontend live?** `frontend/` subdir (monorepo) or separate repo? | Monorepo (same repo as backend) — simpler for Go embed, single PR for full-stack changes, easier to keep spec in sync. | Monorepo in `frontend/` |
| 4 | **State management for WebSocket data?** TanStack Query can't naturally handle push data. | Option A: WS messages invalidate query cache (trigger refetch). Option B: WS messages directly update query cache via `queryClient.setQueryData`. | Start with A (simpler), move to B if latency matters |
| 5 | **Should we add a `make seed` command?** Need realistic data for frontend dev. | Yes — Go command or SQL script that populates dev database with sample data. | Yes, do this in Iteration 0 or 1 |
| 6 | **Sidebar persistence.** Should collapsed/expanded state persist? | localStorage | Yes, localStorage |
| 7 | **Table preferences.** Should column visibility, sort order, and page size persist per-table? | URL params (shareable) vs localStorage (sticky). | URL params for sort/filter/page (shareable links), localStorage for column visibility |

---

## Summary

The plan is **8 iterations from empty scaffold to production-embedded release**, with each iteration producing a testable, demoable increment. The stack is React 19 + Vite 6 + TypeScript + Tailwind + shadcn/ui + TanStack (Query + Router + Table), with types generated directly from the backend's OpenAPI spec.

The design direction is a **dark-first, information-dense operator dashboard** — professional and functional, built for people who stare at IP addresses all day and need clarity over aesthetics.

Every iteration includes tests. Local development requires only `npm run dev` + a running Go backend (which is already one command). No heavyweight infrastructure needed to start building.