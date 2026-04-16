# Host Identity Resolution & UI Surfacing

Date: 2026-04-16
Status: approved (brainstormed live, sections 1–7 confirmed by user)

## Context

Scanorama collects multiple name-like signals for every host (mDNS `.local`, SNMP `sys_name`, DNS PTR, TLS cert CN/SAN, HTTP banners, and manual edits), but there is no canonical "display name" per host. The dashboard shows `hosts.hostname` when present and the IP otherwise, which leaves many hosts labeled by IP even when richer signals exist. SmartScan's stage machine (`os_detection → port_expansion → service_scan → refresh`) treats absent names as unremarkable, so hosts with open ports and services — but no name — never get enriched further.

## Goals

1. One canonical `display_name` per host, surfaced everywhere in the UI, with IP as a final fallback.
2. Every name source (automatic and user-defined) is inspectable and promotable.
3. SmartScan actively fills in missing names instead of considering "has ports+services" sufficient.
4. User-defined names live in a dedicated column so they are never clobbered by auto-enrichment.

## Non-goals

- Changes to the `devices` model (orthogonal to identity for now).
- Denormalized candidate lists (compute on read).
- HTTP `Server`/`<title>` as a name source (too weak; skip).

## Data model

Single migration `027_host_identity.sql`:

- `hosts.custom_name VARCHAR(255) NULL` — user-defined override. Set only through the UI's custom-name input. Always wins when non-null.
- `hosts.hostname_source VARCHAR(32) NULL` — provenance tag for the existing `hosts.hostname` value. Migration seeds all existing non-null hostnames with `'ptr'`. The legacy inline-edit path continues writing `hosts.hostname` but will tag it `'manual'` until that affordance is removed from the UI.
- `identity_rank_order` setting stored as a JSONB array on either the existing settings table (to verify during implementation) or a new single-row `identity_settings` table. Default `["mdns","snmp","ptr","cert"]`.

No new tables for candidates — the Identity tab computes them from existing data on every read.

## Resolver (internal/services/identity.go)

```
ResolveDisplayName(host, cfg) -> { name, source, confidence }
  if host.custom_name != null:
    return (custom_name, "custom", 1.0)
  for source in cfg.identity_rank_order:
    if c := lookupCandidate(host, source); c != nil:
      return c
  return (host.ip_address, "ip", 0)
```

`ListCandidates(host) -> []Candidate` reuses the same per-source lookup and returns every value (usable or not) with `{ name, source, usable, not_usable_reason, observed_at }`.

Per-source rules:

- **mdns** — `hosts.mdns_name` non-null and ending in `.local`.
- **snmp** — `host_snmp_data.sys_name` non-empty, printable ASCII, ≤253 chars.
- **ptr** — most recent PTR in `host_dns_records` that passes `DNSNameIsUsable`. Rejected entries keep their row but are marked `usable: false` with `not_usable_reason: "ISP pattern"` (or whichever rule fired).
- **cert** — `subject_cn` or first `sans[]` entry whose forward A/AAAA lookup resolves back to this host's IP. Unmatched certs appear unusable with `not_usable_reason: "no reverse-match"`.

Unknown sources in the config array are logged and skipped — no panic.

## SmartScan stage

New stage `identity_enrichment` slots between `os_detection` and `port_expansion`:

```
if host.status == 'up' && ResolveDisplayName(host).source == "ip":
  return ScanStage{Stage: "identity_enrichment", …}
```

Runs three enrichers in parallel with independent timeouts:

- mDNS unicast probe (existing `MDNSEnricher`).
- SNMP walk (only if `:161` observed open in a prior scan; uses configured communities).
- Fresh DNS lookup — PTR via `DNSEnricher` with the quality filter, plus forward A/AAAA for any candidate cert CNs to establish reverse-match.

Stage succeeds if at least one new usable candidate lands. The existing auto-progression rate limits apply unchanged.

## API

Extend `HostResponse`:

- `display_name` (string, always present — IP when nothing else)
- `display_name_source` (string enum: `custom`, `mdns`, `snmp`, `ptr`, `cert`, `ip`)
- `custom_name` (string, omitempty)
- `name_candidates` — array, always present, `make([]NameCandidateResponse, 0)` when empty

Two new endpoints:

- `PATCH /hosts/{id}/custom-name` — body `{ "custom_name": "..." | null }`. Null clears the override. 400 on empty string, over 255 chars, or disallowed characters. 404 on unknown host id.
- `POST /hosts/{id}/refresh-identity` — enqueues a one-off identity_enrichment run. Returns 202 + scan-job id. 409 if the host has a refresh already in progress.

Swagger regen via `make docs` is required for frontend type sync.

## UI (frontend/src/routes/hosts.tsx)

Two surfaces:

1. **Overview tab — chevron quick-peek.** `▾` next to the name opens a compact read-only panel with the top 3–4 ranked candidates (source + last-observed). Link "Manage in Identity tab →" at the bottom. No edit controls.
2. **Identity tab — full management.** New tab between Overview and Ports. Contents:
   - Custom-name text input + Save (empty clears the override).
   - Full ranked candidate table including unusable rows with their `not_usable_reason` in muted red.
   - "use" link per usable row: one-click promotion to `custom_name`.
   - "Refresh identity now" button calling `POST /hosts/{id}/refresh-identity`; polls the job status.
   - Helper line: "Last refresh: X ago · auto-runs via SmartScan when host lacks a name".
3. **Host list** — primary label becomes `display_name`. Source badge shown on hover.

Components:

- `<HostIdentityPanel mode="compact" | "full">` — one component, two layouts.
- `useHostIdentity(hostId)` — wraps `useHost`, exposes `display_name`, `name_candidates`, and a `setCustomName` mutation.
- `useRefreshIdentity(hostId)` — wraps the POST endpoint.

The existing inline-edit of `hosts.hostname` is removed from Overview; all user edits now go through the Identity tab's `custom_name` input.

## Backfill & rollout

- Migration seeds `hostname_source='ptr'` for existing non-null hostnames; `custom_name` starts null.
- No big-bang enrichment job. SmartScan drains unnamed hosts on its normal cadence. For targeted acceleration operators can use the per-host refresh button or the existing `POST /smart-scan/batch` endpoint.
- Ranking config changes take effect immediately because `display_name` is computed on read.

## Testing

Backend:
- `internal/services/identity_test.go` — table-driven resolver: every rank order, custom override, filtered PTR, unmatched cert, unknown-source-in-config, empty inputs, fallback to IP.
- `internal/services/smartscan_test.go` — new cases for `identity_enrichment` stage selection.
- `internal/api/handlers/host_test.go` — PATCH custom-name (200 / 400 / 404) and POST refresh-identity (202 / 404 / 409).

Frontend:
- `HostIdentityPanel.test.tsx` — compact + full modes, promote-candidate flow, filtered-row reason rendering.
- `use-hosts.test.ts` — extend with `useHostIdentity` and `useRefreshIdentity` (loading/success/error).
- `hosts.test.tsx` — new hooks mocked per the dashboard-test-mocks pattern; Identity tab renders.

All tests assert observable behavior (returned candidate, rendered text, API call body) — no tautological assertions.
