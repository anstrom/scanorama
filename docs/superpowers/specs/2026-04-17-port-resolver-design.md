# Dynamic Port Lists & Port Definition Expansion

**Date:** 2026-04-17
**Issue:** #742
**Status:** Approved

## Problem

SmartScan's non-profile stages (`os_detection`, `identity_enrichment`, `refresh`) carry hardcoded port strings in Go constants. `port_expansion` and `service_scan` consult scan profiles but fall back to `1-1024` when no profile matches. The fleet's historical open-port data in `port_scans` is never consulted, and the `port_definitions` curated catalog has coverage gaps in modern infrastructure categories.

## Goals

1. **Proposal A** — Feed fleet history back into stage port lists via a SQL aggregate over `port_scans`.
2. **Proposal B** — Use the curated `port_definitions` table (filtered by `os_families`) to inject OS-relevant ports into stages that previously ignored OS context.
3. **Proposal C** — Make per-stage base port lists operator-configurable via the existing `settings` table, with current hardcoded values as defaults.
4. **Expand `port_definitions`** — Add ~150 entries covering Prometheus exporters, popular OSS products, virtualization, service mesh, and modern data infrastructure.

## Out of Scope

- Changing how `port_expansion` and `service_scan` stages select ports — they already use `profiles.Manager`.
- Per-network-segment fleet queries (fleet-wide is sufficient; segment scoping can be added later if data shows benefit).
- Frontend UI for editing per-stage settings (settings are operator-managed via API or DB).

## Architecture

### New component: `PortListResolver`

**File:** `internal/services/portresolver.go`

```go
type PortListResolver struct{ db *db.DB }

func NewPortListResolver(database *db.DB) *PortListResolver

func (r *PortListResolver) Resolve(ctx context.Context, stage string, host *db.Host) (string, error)
```

Single responsibility: given a stage name and host, return a merged, deduplicated, capped nmap-compatible port string.

Resolution order (applied on every call):

1. **Base ports (C)** — read `smartscan.<stage>.ports` from `settings`; fall back to the hardcoded constant if the key is missing or empty.
2. **OS augmentation (B)** — if `host.OSFamily != nil`, query `port_definitions WHERE os_families @> ARRAY[$1] AND protocol = 'tcp' ORDER BY is_standard DESC, port ASC`; take up to `smartscan.top_ports_limit` rows.
3. **Fleet top-N (A)** — `SELECT port FROM port_scans WHERE state = 'open' GROUP BY port ORDER BY COUNT(DISTINCT host_id) DESC LIMIT $n` fleet-wide.
4. **Merge + cap** — union all three port sets, deduplicate, sort numerically, truncate at `smartscan.top_ports_limit` (default 256; treat 0 or missing as 256). Return as comma-separated string.

### Integration with `SmartScanService`

`SmartScanService` gains an optional `portListResolver portListResolverIface` field (interface with a single `Resolve` method, so tests can stub it). When non-nil, `stageOSDetection`, `stageIdentityEnrichment`, and the `refresh` branch in `EvaluateHost` call `Resolve` instead of returning the hardcoded port string. When nil (default, including all existing tests), behaviour is unchanged.

`stageWithProfile` is **not changed** — profiles already own `port_expansion` and `service_scan`.

A new constructor option `WithPortListResolver(r *PortListResolver)` wires the resolver in production. The main `api.go` (or wherever `NewSmartScanService` is called) passes `NewPortListResolver(db)`.

### Settings keys (seeded in migration 028)

| Key | Default value | Type | Description |
|---|---|---|---|
| `smartscan.os_detection.ports` | `22,80,135,443,445,3389` | string | Base ports for OS fingerprint stage |
| `smartscan.identity_enrichment.ports` | `22,80,161,443` | string | Base ports for identity enrichment stage |
| `smartscan.refresh.ports` | `1-1024` | string | Base ports for refresh stage |
| `smartscan.top_ports_limit` | `256` | int | Maximum merged port count across all sources |

### Port definitions expansion (migration 028)

New entries added with `ON CONFLICT DO NOTHING`. Categories and OS family tags follow existing conventions.

**Prometheus exporters** (`category = 'monitoring'`, `os_families = '{linux}'`, `is_standard = false`):

| Port | Service |
|---|---|
| 9091 | Prometheus Pushgateway |
| 9101 | HAProxy Prometheus exporter |
| 9102 | StatsD exporter |
| 9113 | nginx Prometheus exporter |
| 9115 | Blackbox exporter |
| 9116 | SNMP exporter |
| 9121 | Redis exporter |
| 9216 | MongoDB exporter |
| 9308 | Kafka exporter |
| 9419 | RabbitMQ exporter |
| 8085 | cAdvisor metrics |
| 9999 | JMX exporter (default) |

**Identity / auth** (`category = 'security'`):

| Port | Service | OS families |
|---|---|---|
| 8080 | Keycloak HTTP | `{linux}` |
| 8443 | Keycloak HTTPS | `{linux}` |
| 9000 | Authentik HTTP | `{linux}` |
| 5556 | Dex OIDC provider | `{linux}` |

**Virtualization** (`category = 'virtualization'`):

| Port | Service | OS families |
|---|---|---|
| 8006 | Proxmox VE web UI (HTTPS) | `{}` |
| 9443 | VMware vSphere / Proxmox alternate | `{}` |
| 8080 | oVirt / RHV web UI | `{}` |
| 5405 | Proxmox cluster (corosync, UDP) | `{}` |

**Modern data infrastructure** (`category = 'database'` or `'messaging'`):

| Port | Service |
|---|---|
| 8123 | ClickHouse HTTP interface |
| 9440 | ClickHouse HTTPS interface |
| 26257 | CockroachDB SQL wire / HTTP admin |
| 4222 | NATS client |
| 8222 | NATS HTTP monitoring |
| 6222 | NATS cluster routing |
| 6650 | Apache Pulsar broker |
| 9001 | MinIO web console |
| 6333 | Qdrant vector DB HTTP |
| 19530 | Milvus gRPC |
| 8428 | VictoriaMetrics HTTP |

**DevOps / GitOps** (`category = 'devops'`):

| Port | Service |
|---|---|
| 8080 | Jenkins / ArgoCD / generic CI |
| 8081 | Nexus Repository / Artifactory |
| 3000 | Gitea web UI |
| 4646 | HashiCorp Nomad HTTP |
| 4647 | HashiCorp Nomad RPC |
| 4648 | HashiCorp Nomad Serf |
| 2746 | Argo Workflows server |
| 8075 | Gitaly (GitLab Git service) |

**Service mesh / proxy** (`category = 'network'`):

| Port | Service |
|---|---|
| 15012 | Istio pilot gRPC |
| 15021 | Istio health check |
| 9901 | Envoy admin interface |
| 4140 | Linkerd proxy inbound |
| 4191 | Linkerd admin |
| 1936 | HAProxy stats page |
| 10902 | Thanos sidecar gRPC |

**Storage** (`category = 'storage'`):

| Port | Service |
|---|---|
| 6789 | Ceph Monitor |
| 24007 | GlusterFS daemon |

## Testing

### `portresolver_test.go` (unit, go-sqlmock)

- Base ports from settings key → used as-is
- Settings key missing → hardcoded fallback returned
- OS family set → `port_definitions` rows merged in
- Fleet aggregate returns rows → merged in
- Overlapping ports across all three sources → output deduplicated, ≤ `top_ports_limit`
- All three sources empty → hardcoded base returned unchanged
- `top_ports_limit = 0` in settings → falls back to default 256

### `smartscan_test.go` additions

Inject a stub resolver via `portListResolverIface` interface. New test cases for:
- `stageOSDetection` calls resolver when resolver is non-nil
- `stageIdentityEnrichment` calls resolver when resolver is non-nil
- `refresh` branch calls resolver when resolver is non-nil
- Resolver returning error → stage uses hardcoded fallback (fail-open)

### Migration

No test needed — pure `INSERT … ON CONFLICT DO NOTHING`.

## Files Changed

| File | Change |
|---|---|
| `internal/db/028_port_expansion.sql` | New migration: expanded port_definitions + settings keys |
| `internal/services/portresolver.go` | New file: `PortListResolver` |
| `internal/services/portresolver_test.go` | New file: unit tests |
| `internal/services/smartscan.go` | Add `portListResolver` field + `WithPortListResolver` + wire into three stage methods |
| `internal/services/smartscan_test.go` | New test cases for resolver integration |
| `cmd/server/main.go` (or equivalent wiring point) | Pass `NewPortListResolver(db)` to `WithPortListResolver` |
