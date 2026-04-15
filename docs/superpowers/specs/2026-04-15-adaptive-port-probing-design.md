# Adaptive Port Probing — Design Spec

**Issue:** #671 (scoped)
**Date:** 2026-04-15
**Status:** Approved

---

## Problem

When nmap finds an open port but cannot identify the service running on it, the
banner grabber falls back to a plain TCP read. Many services wait for the client
to speak first (HTTP, HTTPS, SSH), so a raw TCP read returns nothing useful.
These ports stay permanently unidentified even though zgrab2 could name them
with a targeted probe.

## Scope

This spec covers the smarter probing step only. The `port_observations`
aggregate table, UI badges, and Smart Scan profile integration are out of scope
and will be addressed separately once the probing foundation is in place.

---

## Design

### Overview

When `grabOne` reaches an open port with no nmap-detected service name, it
checks whether extended probing has already been attempted for that
(host, port) pair. If not, it runs a fixed probe sequence — HTTP → HTTPS →
SSH → plain TCP — stopping at the first successful identification, then
permanently marks the pair as probed. On subsequent scans the extended sequence
is skipped, preventing redundant network traffic to ports that already have a
definitive result (or a confirmed non-identification).

### DB migration

New file: `internal/db/024_port_banners_extended_probe.sql`

```sql
ALTER TABLE port_banners
    ADD COLUMN IF NOT EXISTS extended_probe_done BOOLEAN NOT NULL DEFAULT FALSE;
```

The column defaults to `FALSE` so existing rows are unaffected — they will be
eligible for extended probing on the next scan cycle.

### Repository

`BannerRepository` (`internal/db/repository_banners.go`) gains one method:

```go
// IsExtendedProbeDone reports whether extended probing has already been
// attempted for the given host/port pair. Returns false if no banner row
// exists yet.
func (r *BannerRepository) IsExtendedProbeDone(
    ctx context.Context, hostID uuid.UUID, port int,
) (bool, error)
```

`UpsertPortBanner` is updated to include `extended_probe_done` in the
`ON CONFLICT DO UPDATE` clause so callers can set it alongside other fields.

`PortBanner` model gains the corresponding field:

```go
ExtendedProbeDone bool `db:"extended_probe_done" json:"-"`
```

### Enrichment logic

In `internal/enrichment/banner.go`, the `default` branch of `grabOne` becomes:

```go
default:
    if pi.Service == "" {
        done, _ := g.repo.IsExtendedProbeDone(ctx, t.HostID, pi.Number)
        if !done {
            g.probeUnknown(ctx, t, pi)
            return
        }
    }
    g.grabPlain(ctx, t, pi.Number, addr)
```

`probeUnknown` runs the probe sequence:

1. Try `grabZGrabHTTP` — if it stores a banner with a non-empty service, done.
2. Try `grabZGrabHTTPS` — same check.
3. Try `grabZGrabSSH` — same check.
4. Fall back to `grabPlain`.

After the sequence completes (regardless of outcome), upsert a `port_banners`
row with `extended_probe_done = true`. This ensures the flag is set even when
no service is identified, preventing repeated probing on future scans.

`probeUnknown` runs within the existing `bannerConcurrency` semaphore — no
additional concurrency controls are needed.

### Rate limiting

The issue specifies a cap of 20 extended probes per host per scan cycle. This
is enforced by counting the number of `probeUnknown` calls per host within
`EnrichHosts` before dispatching goroutines. Ports beyond the cap proceed
directly to `grabPlain` and are not flagged (`extended_probe_done` remains
`false`), making them eligible for extended probing in a future cycle.

---

## Error handling

- `IsExtendedProbeDone` errors are treated as `false` (probe proceeds). A DB
  error should not suppress enrichment.
- Individual probe attempts within `probeUnknown` follow the existing pattern:
  log and continue to the next probe in the sequence.
- If setting `extended_probe_done = true` fails, log a warning. The probe
  results are still stored.

---

## Testing

### `BannerRepository`
- `IsExtendedProbeDone`: returns `false` when no row exists; returns the stored
  value when a row exists. Uses `go-sqlmock`.
- `UpsertPortBanner` with `ExtendedProbeDone = true`: verify the column is
  written and survives a conflict update.

### `grabOne` (unit, mock repo)
- When `pi.Service == ""` and `IsExtendedProbeDone` returns `false`:
  `probeUnknown` is called, `grabPlain` is not.
- When `pi.Service == ""` and `IsExtendedProbeDone` returns `true`:
  `probeUnknown` is not called, `grabPlain` is called.
- When `pi.Service != ""` (identified by nmap): existing dispatch path,
  unchanged.

### `probeUnknown` (unit, mock repo)
- Stops after the first probe that identifies a service (does not attempt
  subsequent probes).
- Always upserts with `extended_probe_done = true`, even when no service is
  identified.
- Respects the 20-probe-per-host cap: ports beyond the cap skip `probeUnknown`.

---

## Out of scope

- `port_observations` aggregate table
- "Exploring..." badge in host detail UI
- "Local" tab in port browser
- Smart Scan profile suggestion integration

These will be addressed in a follow-up once the probing foundation has shipped
and its value is validated.
