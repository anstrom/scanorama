# Host ↔ Network Membership Redesign

**Date:** 2026-04-15
**Status:** Design approved, pending implementation plan
**Context:** Follow-up to PR #719 (hostname CIDR / JSONB corruption fix)

## Problem

PR #719 made ad-hoc scans (CLI `scanorama scan <target>`) work for hostnames and
IP targets by resolving them to a `/32` or `/128` CIDR and auto-inserting a row
into the `networks` table. This fixed the immediate bug but revealed a deeper
modelling issue:

- The `networks` table was originally meant to hold **user-registered CIDR
  ranges** (e.g. `10.0.0.0/24`) — a curated inventory of what the operator
  considers a network.
- Ad-hoc single-host scans now pollute that table with `/32` pseudo-networks
  named `"Ad-hoc: <cidr>"`. Over time the table becomes a mix of real subnets
  and per-host scan targets.
- Migration `002_host_targets.sql` already made `scan_jobs.network_id` nullable
  precisely to support scans that don't belong to a network, but the CLI
  ad-hoc path never took advantage of it.

The host↔network relationship is conceptually **membership via CIDR
containment**, not a stored link. Postgres's `inet`/`cidr` types, `<<`/`<<=`
operators, and GIST indexes make this cheap to compute on the fly.

## Design decisions

### 1. `networks` is user-curated only

A row exists in `networks` if and only if a user explicitly registered it
(via `POST /networks` or a CLI `networks create`). Scans never auto-create
network rows.

**Rationale:** clean separation of concerns — the catalog is what the operator
cares about, scan history is independent.

### 2. Host↔network membership is derived, not stored

Membership is a SQL query, not a join table. A host is "in" a registered
network iff `host.ip_address <<= network.cidr`. One host can be a member of
multiple registered networks (e.g. both `10.0.0.0/16` and `10.0.0.0/24`).

**Rationale:** no denormalised cache to keep in sync, no triggers to maintain,
indexed lookups via existing GIST indexes.

### 3. Scan attachment uses longest-prefix match

`scan_jobs.network_id` is a scalar FK, so each scan attaches to at most one
network. When multiple registered networks contain the target IP, the
most-specific (highest `masklen`) wins. Ties (implausible) resolve
arbitrarily; no policy needed.

**Rationale:** conventional routing semantics, intuitive ("this scan is part
of the /24, which is inside the /16"). Single FK means simpler queries and
no ambiguity in scan reports.

### 4. No new schema (apart from a view)

- `hosts.ip_address` and `networks.cidr` already carry the authoritative
  relationship via the `<<=` operator.
- `scan_jobs.network_id` is already nullable (migration 002).
- `discovery_jobs.network_id` is already nullable (migration 003).
- The existing `update_network_host_counts` trigger already maintains
  `networks.host_count` correctly via containment.

The only DB change is a new **view** `host_network_memberships` that exposes
the derived membership for easy joining.

### 5. Existing `/32` ad-hoc rows cleaned up interactively

Rather than a destructive one-shot migration, add a CLI command
`scanorama networks cleanup-adhoc` that lists candidates (`name LIKE 'Ad-hoc: %'`),
prompts for confirmation, and deletes. Supports `--dry-run` and `--yes`.

**Rationale:** operator visibility; no surprise deletes at deploy time. The
prefix-based match is specific enough that user-registered networks are safe.

### 6. Scan history preserved during cleanup

Before deleting a network row, NULL out any `scan_jobs.network_id` pointing to
it. The FK has `ON DELETE CASCADE`, so unconditional deletion would wipe scan
history — not acceptable. Detach first, then delete.

## Architecture summary

| Relationship | How it's expressed |
|---|---|
| Host → all containing networks | `host_network_memberships` view (many-to-many, derived) |
| Scan → primary network | `scan_jobs.network_id`, longest-prefix match (single, nullable) |
| Network → host count | Existing `update_network_host_counts` trigger |

## Code changes

### DB migration: `NNN_host_network_memberships.sql`

```sql
CREATE OR REPLACE VIEW host_network_memberships AS
SELECT h.id  AS host_id,
       n.id  AS network_id,
       masklen(n.cidr) AS mask_len
FROM hosts h
JOIN networks n ON h.ip_address <<= n.cidr;

COMMENT ON VIEW host_network_memberships IS
    'Derived host↔network membership via CIDR containment. '
    'A host is a member of every registered network whose CIDR contains its IP.';
```

No tables changed, no data migrated.

### `internal/scanning/scan.go`

- Replace `findOrCreateNetwork(ctx, database, config)` with
  `findContainingNetwork(ctx, database, ip)` — pure lookup, no writes.
- Returns `uuid.Nil` when no registered network contains the target. Not an
  error: most scans have no container.
- Keep the `normaliseToCIDR` helper from PR #719. Still useful for the lookup
  (DNS → IP → query).
- Scan-job insert uses `sql.NullUUID{UUID: id, Valid: id != uuid.Nil}` so NULL
  is stored when there's no container.

Query:

```sql
SELECT id FROM networks
WHERE $1::inet <<= cidr
ORDER BY masklen(cidr) DESC
LIMIT 1;
```

### `internal/api/handlers/networks.go`

On successful `POST /networks`, backfill `host_count` from any pre-existing
hosts that fall inside the new CIDR:

```sql
UPDATE networks
SET host_count = (
        SELECT COUNT(*) FROM hosts WHERE ip_address <<= $1
    ),
    active_host_count = (
        SELECT COUNT(*) FROM hosts WHERE ip_address <<= $1 AND status = 'up'
    ),
    updated_at = NOW()
WHERE id = $2;
```

This closes the gap where registering a network after hosts are scanned leaves
its count at zero.

### New endpoint: `GET /hosts/{id}/networks`

Returns the list of registered networks containing the host, ordered by
longest prefix first. Backed by the view. Surfaced in the host-detail page in
the frontend as a "Member of" section.

### New CLI command: `scanorama networks cleanup-adhoc`

Flags: `--dry-run`, `--yes`.

Flow:
1. Query candidates:
   `SELECT id, cidr, name, created_at, host_count FROM networks WHERE name LIKE 'Ad-hoc: %'`.
2. If `--dry-run`: print the list and exit.
3. Otherwise: prompt per-row (`y/n/a/q`) unless `--yes`.
4. For each confirmed row, in a transaction:
   - `UPDATE scan_jobs SET network_id = NULL WHERE network_id = $1`
   - `UPDATE discovery_jobs SET network_id = NULL WHERE network_id = $1`
   - `DELETE FROM networks WHERE id = $1`
5. Print summary: N networks deleted, M scan_jobs detached.

## Testing strategy

### Backend unit tests

- `findContainingNetwork`: returns Nil for no match, correct UUID for single
  match, longest-prefix UUID for overlapping matches.
- Scan paths: ad-hoc scan produces `scan_jobs.network_id = NULL` when no
  registered container; attaches when one exists.
- Network-create handler: backfills `host_count` and `active_host_count`
  correctly when hosts pre-exist in the CIDR.
- Cleanup command:
  - `--dry-run` reports candidates and touches nothing.
  - Real run detaches `scan_jobs` / `discovery_jobs` then deletes networks.
  - Scan history preserved (scan_jobs still exist with NULL network_id).

### DB/integration tests

- The `host_network_memberships` view returns correct rows for overlapping
  CIDRs, IPv4, and IPv6.
- Registering a `/24` after scanning five hosts inside it produces
  `host_count = 5`.

### Manual verification (test plan)

- `scanorama scan hegre.uninett.no` — confirm no new row in `networks`;
  `scan_jobs.network_id IS NULL` in DB.
- Register `10.0.0.0/24`, scan `10.0.0.5` — confirm `scan_jobs.network_id`
  points to the `/24`.
- Host-detail page shows "Member of: 10.0.0.0/24, 10.0.0.0/16" for a host
  inside both.
- `scanorama networks cleanup-adhoc --dry-run` lists pre-existing `/32`
  pseudo-networks.

## Out of scope (follow-ups)

- Renaming / restructuring the `networks` table beyond the view
- Replacing the trigger-maintained `host_count` with a view-based computation
  (the trigger still works; changing it is unrelated to this design)
- Frontend UI for explicit "move host to network" workflows — membership is
  derived, not editable
- Ignoring ad-hoc `/32` rows via `is_adhoc` flag instead of deleting — rejected
  in favour of interactive cleanup
