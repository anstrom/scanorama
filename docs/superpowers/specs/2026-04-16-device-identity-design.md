# Device Identity — Design Spec

**Issue:** #713  
**Milestone:** v0.27 — UX Polish & Quality of Life  
**Date:** 2026-04-16  

## Problem

MAC address randomization is on by default on iOS, Android, and Windows. When a device
rotates its MAC, Scanorama creates a new host record and loses all prior scan history,
tags, and notes. This feature introduces a stable `Device` concept that survives MAC and
IP churn permanently.

## Design

### 1. Data Model

Four schema changes in migration `026_devices.sql`:

```sql
-- Stable device identity above raw host records
CREATE TABLE devices (
    id         UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(255) NOT NULL,
    notes      TEXT,
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    updated_at TIMESTAMPTZ  DEFAULT NOW()
);

-- One row per MAC address ever seen for a device.
-- UNIQUE on mac_address: a MAC can only belong to one device.
CREATE TABLE device_known_macs (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id   UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    mac_address MACADDR     NOT NULL,
    first_seen  TIMESTAMPTZ DEFAULT NOW(),
    last_seen   TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_mac UNIQUE (mac_address)
);

-- Stable names ever associated with a device.
-- source ∈ {mdns, dns, snmp, netbios, user}
-- UNIQUE on (name, source): same name from same source = one row.
CREATE TABLE device_known_names (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id  UUID        NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    source     VARCHAR(20) NOT NULL
                   CHECK (source IN ('mdns', 'dns', 'snmp', 'netbios', 'user')),
    first_seen TIMESTAMPTZ DEFAULT NOW(),
    last_seen  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_device_name UNIQUE (name, source)
);

-- Low-confidence match candidates surfaced for user review
CREATE TABLE device_suggestions (
    id                UUID    PRIMARY KEY DEFAULT uuid_generate_v4(),
    host_id           UUID    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    device_id         UUID    NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    confidence_score  INTEGER NOT NULL,
    confidence_reason TEXT,
    dismissed         BOOLEAN DEFAULT FALSE,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_suggestion UNIQUE (host_id, device_id)
);

-- hosts gains two nullable columns
ALTER TABLE hosts
    ADD COLUMN device_id  UUID REFERENCES devices(id) ON DELETE SET NULL,
    ADD COLUMN mdns_name  TEXT;
```

`hosts.device_id = NULL` means unidentified. `hosts.mdns_name` caches the most recently
resolved mDNS name for this specific host/IP — separate from the durable
`device_known_names` records.

### 2. mDNS Enrichment

New file: `internal/enrichment/mdns.go`

Adds a lightweight enrichment step to the existing pipeline (after banner, alongside SNMP).
Sends a **unicast DNS PTR query** directly to `<host-ip>:5353` — no multicast, no listener:

```
query:       <reversed-ip>.in-addr.arpa. PTR
destination: <host-ip>:5353
timeout:     2s
library:     github.com/miekg/dns
```

If the host responds (Apple, Android, Linux/avahi all do), the resolved `.local` name is
written to `hosts.mdns_name` via `HostRepository.UpdateMDNSName`. The enricher interface:

```go
type MDNSEnricher struct {
    timeout time.Duration
    logger  *slog.Logger
}

func (e *MDNSEnricher) Enrich(ctx context.Context, hostID uuid.UUID, ip string) (name string, err error)
```

### 3. DNS Name Quality Filter

DNS PTR names (from `hosts.hostname`) are included as identity signals only if they pass
a quality filter at write time. Names are rejected if they:

- Contain an IP address literally (e.g. `192-168-1-50`, `10.0.0.1`)
- Match DHCP-generated prefixes: `dhcp-*`, `host-*`, `ip-*`, `client-*`
- Match ISP-generated suffixes: `*.dynamic.*`, `*.dhcp.*`, `*.broadband.*`
- Consist entirely of numeric labels

Names that pass are stored in `device_known_names` with `source = 'dns'` and participate
in matching identically to other name sources.

### 4. Device Matcher

New service: `internal/services/device_matcher.go`

Runs **post-discovery** (not during scan), called at the end of each discovery run with the
list of host IDs updated. For each host, it scores every existing device using the
weighted signal model below, then acts on the highest-scoring match.

#### Signal Weights

| Signal | Weight | Notes |
|--------|--------|-------|
| Non-randomized MAC | 3 | IEEE globally-administered: `mac[0] & 0x02 == 0` |
| mDNS/Bonjour name | 3 | Self-asserted by device, stable across MAC/IP change |
| SNMP sysName | 3 | Manually configured, very stable |
| NetBIOS name | 3 | Device-set, stable |
| User-assigned name | 3 | Explicit |
| Filtered DNS name | 2 | Semi-stable (dynamic DNS environments) |
| Banner fingerprint | 2 | Matching service:version strings from port_banners |
| Locally-administered MAC | 1 | Randomized — minimal signal |
| Historical IP | 1 | Current IP already exists on a host attached to device D |
| OS family | 1 | Broad |
| Vendor OUI | 1 | Narrows to manufacturer |
| Port set | 1 | Identical set of open (port, protocol) pairs from port_scans |
| Banner fingerprint | 2 | Matching service:version strings from port_banners (e.g. `OpenSSH_8.9p1`) |

#### Thresholds

- **Score ≥ 3** → auto-attach host to device, learn new signals
- **Score 1–2** → create `device_suggestions` row, surface in discovery diff
- **Score 0** → no action
- **Tie (two devices both score ≥ 3)** → create suggestions for both, do not auto-attach

#### Learning

When a host is auto-attached, the matcher inserts any new MAC or name signals into
`device_known_macs` / `device_known_names` so future occurrences of those signals
also trigger auto-attach.

### 5. API

#### New Device Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/devices` | List devices (name, MAC count, host count) |
| `POST` | `/api/v1/devices` | Create device manually |
| `GET` | `/api/v1/devices/{id}` | Detail — name, notes, known MACs, known names, attached hosts |
| `PUT` | `/api/v1/devices/{id}` | Update name / notes |
| `DELETE` | `/api/v1/devices/{id}` | Delete (attached hosts set device_id → NULL) |
| `POST` | `/api/v1/devices/{id}/hosts/{host_id}` | Manually attach host |
| `DELETE` | `/api/v1/devices/{id}/hosts/{host_id}` | Detach host |
| `POST` | `/api/v1/devices/suggestions/{id}/accept` | Accept → attach |
| `POST` | `/api/v1/devices/suggestions/{id}/dismiss` | Dismiss suggestion |

#### Existing Endpoint Changes

- `GET /api/v1/hosts/{id}` — response gains `device_id`, `device_name`, `mdns_name`
- `GET /api/v1/hosts` — list response gains `device_name` (null when unidentified)
- `GET /api/v1/discovery/{id}/diff` — response gains `suggestions` array:
  `[{ host_id, device_id, device_name, confidence_score, confidence_reason }]`

### 6. Frontend

#### Device Detail Page (`/devices/:id`)

New route. Shows: name (inline editable), notes, known MACs list, known names list with
source badges, attached hosts table with links to host detail. Attach/detach controls.

#### Host Detail

New "Device" card. If attached: device name as link + detach button. If unattached:
"Attach to device" control (pick existing or create new).

#### Host List

Optional `Device` column (off by default in column toggle). Shows device name where
identified, blank otherwise.

#### Discovery Diff

Suggestion cards below new/gone/changed sections. Each card shows: device name,
confidence score, confidence reason (e.g. "Vendor: Apple · OS: iOS · Score: 2"),
Accept and Dismiss buttons.

## Acceptance Criteria

- [ ] `devices`, `device_known_macs`, `device_known_names`, `device_suggestions` tables and migration
- [ ] `hosts.device_id` and `hosts.mdns_name` columns and migration
- [ ] mDNS enricher (unicast PTR query, `miekg/dns`, 2s timeout)
- [ ] DNS name quality filter applied before writing to `device_known_names`
- [ ] `DeviceMatcher` service with weighted scoring, runs post-discovery
- [ ] Auto-attach on score ≥ 3, suggestion on score 1–2, tie → suggest both
- [ ] Signal learning: new MACs/names recorded on auto-attach
- [ ] Device CRUD API endpoints
- [ ] Manual attach/detach API endpoints
- [ ] Suggestion accept/dismiss API endpoints
- [ ] Host list and host detail responses include `device_id`, `device_name`, `mdns_name`
- [ ] Discovery diff response includes `suggestions` array
- [ ] Device detail page (`/devices/:id`)
- [ ] Host detail "Device" card with attach/detach UI
- [ ] Discovery diff suggestion cards with accept/dismiss

## Out of Scope

- Tags on devices (tags stay on hosts per issue spec)
- Email/push notifications when a device is auto-attached
- mDNS service browsing (multicast browse) — only unicast PTR queries
- Merging host scan history across attached hosts (display only, not deduplicated)
