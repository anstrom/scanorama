# Dynamic Port Lists & Port Definition Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace hardcoded port strings in SmartScan's `os_detection`, `identity_enrichment`, and `refresh` stages with a `PortListResolver` that merges operator-configured defaults (settings table), OS-specific ports (port_definitions table), and fleet-observed top-N ports (port_scans aggregate); also expand port_definitions with ~150 new entries covering modern infrastructure.

**Architecture:** A new `PortListResolver` struct in `internal/services/portresolver.go` exposes a single `Resolve(ctx, stage, host)` method. It reads base ports from `settings`, augments with OS-matched rows from `port_definitions`, and overlays fleet top-N from `port_scans`, then deduplicates and caps at a configurable limit. `SmartScanService` holds an optional `portListResolverIface` field; nil means existing hardcoded behaviour (all current tests pass unchanged). The wiring happens in `internal/api/routes.go`.

**Tech Stack:** Go, PostgreSQL, `go-sqlmock` (DATA-DOG/go-sqlmock), `testify/require` + `testify/assert`.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/db/028_port_expansion.sql` | Create | Migration: new port_definitions rows + settings seed keys |
| `internal/services/portresolver.go` | Create | `PortListResolver` — three-source merge logic |
| `internal/services/portresolver_test.go` | Create | Unit + sqlmock tests for resolver |
| `internal/services/smartscan.go` | Modify | Add interface + field + `WithPortListResolver` + wire three stage methods |
| `internal/services/smartscan_test.go` | Modify | Add stub resolver tests for three stages |
| `internal/api/routes.go` | Modify | Pass `NewPortListResolver(db)` to `WithPortListResolver` |

---

## Task 1: Migration — port_definitions expansion + settings keys

**Files:**
- Create: `internal/db/028_port_expansion.sql`

- [ ] **Step 1: Write the migration file**

```sql
-- Migration 028: expanded port definitions and SmartScan port settings.
-- Adds ~150 curated port/service entries covering Prometheus exporters,
-- modern data infrastructure, virtualization, service mesh, and DevOps tooling.
-- Also seeds per-stage base-port settings keys for operator configurability.

-- ── Prometheus exporters ──────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(9091,  'tcp', 'prometheus-pushgateway', 'Prometheus Pushgateway',              'monitoring', '{linux}', false),
(9101,  'tcp', 'haproxy-exporter',       'HAProxy Prometheus exporter',          'monitoring', '{linux}', false),
(9102,  'tcp', 'statsd-exporter',        'StatsD Prometheus exporter',           'monitoring', '{linux}', false),
(9113,  'tcp', 'nginx-exporter',         'nginx Prometheus exporter',            'monitoring', '{linux}', false),
(9115,  'tcp', 'blackbox-exporter',      'Prometheus Blackbox exporter',         'monitoring', '{linux}', false),
(9116,  'tcp', 'snmp-exporter',          'Prometheus SNMP exporter',             'monitoring', '{linux}', false),
(9121,  'tcp', 'redis-exporter',         'Redis Prometheus exporter',            'monitoring', '{linux}', false),
(9216,  'tcp', 'mongodb-exporter',       'MongoDB Prometheus exporter',          'monitoring', '{linux}', false),
(9308,  'tcp', 'kafka-exporter',         'Kafka Prometheus exporter',            'monitoring', '{linux}', false),
(9419,  'tcp', 'rabbitmq-exporter',      'RabbitMQ Prometheus exporter',         'monitoring', '{linux}', false),
(8085,  'tcp', 'cadvisor',               'cAdvisor container metrics',           'monitoring', '{linux}', false),
(9999,  'tcp', 'jmx-exporter',           'Prometheus JMX exporter (default)',    'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Identity / auth ───────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(5556,  'tcp', 'dex',                    'Dex OIDC identity provider',           'security',   '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Virtualization ────────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(8006,  'tcp', 'proxmox-web',            'Proxmox VE web UI (HTTPS)',             'virtualization', '{}', true),
(9443,  'tcp', 'vsphere-https',          'VMware vSphere / ESXi HTTPS',          'virtualization', '{}', true),
(5405,  'udp', 'corosync',               'Proxmox cluster / Corosync heartbeat', 'virtualization', '{}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Modern data infrastructure ────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(8123,  'tcp', 'clickhouse-http',        'ClickHouse HTTP interface',             'database',   '{linux}', true),
(9440,  'tcp', 'clickhouse-https',       'ClickHouse native TLS interface',       'database',   '{linux}', false),
(26257, 'tcp', 'cockroachdb',            'CockroachDB SQL wire + admin HTTP',    'database',   '{linux}', true),
(4222,  'tcp', 'nats',                   'NATS messaging client port',            'messaging',  '{linux}', true),
(8222,  'tcp', 'nats-monitor',           'NATS HTTP monitoring interface',        'messaging',  '{linux}', false),
(6222,  'tcp', 'nats-cluster',           'NATS cluster routing port',             'messaging',  '{linux}', false),
(6650,  'tcp', 'pulsar',                 'Apache Pulsar broker service',          'messaging',  '{linux}', true),
(9001,  'tcp', 'minio-console',          'MinIO web console',                     'storage',    '{linux}', false),
(6333,  'tcp', 'qdrant',                 'Qdrant vector database HTTP API',       'database',   '{linux}', false),
(19530, 'tcp', 'milvus',                 'Milvus vector database gRPC',           'database',   '{linux}', false),
(8428,  'tcp', 'victoriametrics',        'VictoriaMetrics HTTP API',              'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── DevOps / GitOps ───────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(4646,  'tcp', 'nomad-http',             'HashiCorp Nomad HTTP API',              'devops',     '{linux}', false),
(4647,  'tcp', 'nomad-rpc',              'HashiCorp Nomad RPC',                   'devops',     '{linux}', false),
(4648,  'tcp', 'nomad-serf',             'HashiCorp Nomad Serf',                  'devops',     '{linux}', false),
(2746,  'tcp', 'argo-workflows',         'Argo Workflows server',                 'devops',     '{linux}', false),
(8075,  'tcp', 'gitaly',                 'GitLab Gitaly Git service',             'devops',     '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Service mesh / proxy ──────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(15012, 'tcp', 'istio-pilot',            'Istio pilot discovery gRPC',            'network',    '{linux}', false),
(15021, 'tcp', 'istio-health',           'Istio health check endpoint',           'network',    '{linux}', false),
(9901,  'tcp', 'envoy-admin',            'Envoy proxy admin interface',           'network',    '{linux}', false),
(4140,  'tcp', 'linkerd-proxy',          'Linkerd proxy inbound port',            'network',    '{linux}', false),
(4191,  'tcp', 'linkerd-admin',          'Linkerd admin interface',               'network',    '{linux}', false),
(1936,  'tcp', 'haproxy-stats',          'HAProxy statistics page',               'network',    '{linux}', false),
(10902, 'tcp', 'thanos-sidecar',         'Thanos sidecar gRPC',                   'monitoring', '{linux}', false)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── Storage ───────────────────────────────────────────────────────────────────
INSERT INTO port_definitions (port, protocol, service, description, category, os_families, is_standard) VALUES
(6789,  'tcp', 'ceph-mon',               'Ceph Monitor port',                     'storage',    '{linux}', true),
(24007, 'tcp', 'glusterfs',              'GlusterFS daemon port',                 'storage',    '{linux}', true)
ON CONFLICT (port, protocol) DO NOTHING;

-- ── SmartScan per-stage base port settings ────────────────────────────────────
INSERT INTO settings (key, value, type, description) VALUES
    ('smartscan.os_detection.ports',
     '"22,80,135,443,445,3389"',
     'string',
     'Base port list for the os_detection SmartScan stage'),
    ('smartscan.identity_enrichment.ports',
     '"22,80,161,443"',
     'string',
     'Base port list for the identity_enrichment SmartScan stage'),
    ('smartscan.refresh.ports',
     '"1-1024"',
     'string',
     'Base port list for the refresh SmartScan stage'),
    ('smartscan.top_ports_limit',
     '256',
     'int',
     'Maximum merged port count across all sources (0 = use default 256)')
ON CONFLICT (key) DO NOTHING;
```

- [ ] **Step 2: Verify migration file exists and SQL is valid**

```bash
ls internal/db/028_port_expansion.sql
# should print the path

# Quick syntax check — psql parses but doesn't execute with --dry-run equivalent:
psql "$DATABASE_URL" --set ON_ERROR_STOP=1 -c "\i internal/db/028_port_expansion.sql" 2>&1 | head -5
# Expected: INSERT 0 12 (or similar counts) — no ERROR lines
```

If the database isn't running, skip the psql step; CI integration tests run the migration automatically.

- [ ] **Step 3: Commit the migration**

```bash
git add internal/db/028_port_expansion.sql
git commit -m "feat(db): add port definitions expansion and SmartScan port settings"
```

---

## Task 2: `PortListResolver` — write failing tests first

**Files:**
- Create: `internal/services/portresolver_test.go`
- Create: `internal/services/portresolver.go` (stub only in this step)

- [ ] **Step 1: Write the stub resolver so tests compile**

Create `internal/services/portresolver.go`:

```go
// Package services — PortListResolver merges three port sources for SmartScan stages.
package services

import (
	"context"

	"github.com/anstrom/scanorama/internal/db"
)

// portResolverTimeout caps all three DB queries inside Resolve.
const portResolverTimeout = 3 * time.Second

// defaultTopPortsLimit is used when the settings key is 0 or missing.
const defaultTopPortsLimit = 256

// stageDefaultPorts maps each stage name to its hardcoded fallback port string.
// These match the constants in smartscan.go so the resolver stays in sync.
var stageDefaultPorts = map[string]string{
	"os_detection":        "22,80,135,443,445,3389",
	"identity_enrichment": "22,80,161,443",
	"refresh":             "1-1024",
}

// PortListResolver builds a merged, deduplicated port string for a SmartScan stage
// by combining three sources: operator-configured defaults (settings table),
// OS-matched curated ports (port_definitions table), and fleet top-N (port_scans).
type PortListResolver struct {
	db *db.DB
}

// NewPortListResolver creates a PortListResolver backed by the given database.
func NewPortListResolver(database *db.DB) *PortListResolver {
	return &PortListResolver{db: database}
}

// Resolve returns the merged port string for the given stage and host.
// On any DB error the affected source is silently skipped (fail-open).
// If the resolved base string contains a port range (e.g. "1-1024"), no
// augmentation is applied and the base is returned unchanged.
func (r *PortListResolver) Resolve(ctx context.Context, stage string, host *db.Host) (string, error) {
	panic("not implemented")
}
```

**Note:** Add `"time"` to the import block — it's needed for `portResolverTimeout`.

- [ ] **Step 2: Write the failing tests**

Create `internal/services/portresolver_test.go`:

```go
package services

import (
	"context"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// newResolverMockDB returns a *db.DB backed by sqlmock for resolver tests.
func newResolverMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

// resolverHost builds a minimal host for resolver tests.
func resolverHost(osFamily *string) *db.Host {
	return &db.Host{
		ID:        uuid.New(),
		IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.1")},
		OSFamily:  osFamily,
		LastSeen:  time.Now(),
	}
}

func osPtr(s string) *string { return &s }

// ── base port resolution (Proposal C) ────────────────────────────────────────

func TestPortListResolver_UsesSettingsBaseWhenPresent(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	// settings returns a custom base
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.os_detection.ports", `"22,443,8080"`, "", "string", time.Now()))

	// top_ports_limit
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "256", "", "int", time.Now()))

	// OS augmentation — no OS family set
	// Fleet top-N — returns empty
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result, err := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	require.NoError(t, err)
	assert.Equal(t, "22,443,8080", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_FallsBackToHardcodedDefaultWhenSettingMissing(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	// settings key not found
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}))

	// top_ports_limit — also missing
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}))

	// Fleet top-N — empty
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result, err := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	require.NoError(t, err)
	// hardcoded fallback
	assert.Equal(t, "22,80,135,443,445,3389", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── range base passthrough ────────────────────────────────────────────────────

func TestPortListResolver_ReturnsRangeBaseUnchanged(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	// settings returns a range
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.refresh.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.refresh.ports", `"1-1024"`, "", "string", time.Now()))

	// top_ports_limit — not needed for range, but resolver reads it before deciding
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "256", "", "int", time.Now()))

	// No OS or fleet queries expected because base is a range
	result, err := r.Resolve(context.Background(), "refresh", resolverHost(nil))
	require.NoError(t, err)
	assert.Equal(t, "1-1024", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── OS augmentation (Proposal B) ─────────────────────────────────────────────

func TestPortListResolver_MergesOSPortsWhenOSFamilyKnown(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)
	host := resolverHost(osPtr("linux"))

	// base: just port 22
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.os_detection.ports", `"22"`, "", "string", time.Now()))

	// top_ports_limit
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "256", "", "int", time.Now()))

	// OS augmentation returns ports 111 and 2049
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(111).AddRow(2049))

	// Fleet — empty
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}))

	result, err := r.Resolve(context.Background(), "os_detection", host)
	require.NoError(t, err)
	// merged, sorted: 22, 111, 2049
	assert.Equal(t, "22,111,2049", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── fleet top-N (Proposal A) ─────────────────────────────────────────────────

func TestPortListResolver_MergesFleetTopPorts(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	// base: port 22 only
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.identity_enrichment.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.identity_enrichment.ports", `"22"`, "", "string", time.Now()))

	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "256", "", "int", time.Now()))

	// No OS augmentation (no os family)
	// Fleet returns 80 and 443
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80).AddRow(443))

	result, err := r.Resolve(context.Background(), "identity_enrichment", resolverHost(nil))
	require.NoError(t, err)
	assert.Equal(t, "22,80,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── deduplication and cap ─────────────────────────────────────────────────────

func TestPortListResolver_DeduplicatesOverlappingPorts(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)
	host := resolverHost(osPtr("linux"))

	// base: 22,80
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.os_detection.ports", `"22,80"`, "", "string", time.Now()))

	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "256", "", "int", time.Now()))

	// OS augmentation also returns 80 (duplicate) and 443
	mock.ExpectQuery(`SELECT port FROM port_definitions`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80).AddRow(443))

	// Fleet also returns 22 (duplicate)
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(22))

	result, err := r.Resolve(context.Background(), "os_detection", host)
	require.NoError(t, err)
	// deduplicated, sorted: 22, 80, 443
	assert.Equal(t, "22,80,443", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_CapsAtTopPortsLimit(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	// base: ports 1-5
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.os_detection.ports", `"1,2,3,4,5"`, "", "string", time.Now()))

	// limit of 3
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "3", "", "int", time.Now()))

	// Fleet adds 6,7,8 (but cap means only 3 total)
	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(6).AddRow(7).AddRow(8))

	result, err := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	require.NoError(t, err)
	// capped at 3
	assert.Equal(t, "1,2,3", result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortListResolver_TreatsZeroLimitAsDefault(t *testing.T) {
	database, mock := newResolverMockDB(t)
	r := NewPortListResolver(database)

	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.os_detection.ports").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.os_detection.ports", `"22"`, "", "string", time.Now()))

	// limit = 0 → default 256
	mock.ExpectQuery(`SELECT key, value`).
		WithArgs("smartscan.top_ports_limit").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "description", "type", "updated_at"}).
			AddRow("smartscan.top_ports_limit", "0", "", "int", time.Now()))

	mock.ExpectQuery(`SELECT port FROM port_scans`).
		WillReturnRows(sqlmock.NewRows([]string{"port"}).AddRow(80))

	result, err := r.Resolve(context.Background(), "os_detection", resolverHost(nil))
	require.NoError(t, err)
	// no cap issue — only 2 ports
	assert.Equal(t, "22,80", result)
	require.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 3: Run tests — confirm they fail with "not implemented"**

```bash
go test ./internal/services/ -run TestPortListResolver -v 2>&1 | head -30
```
Expected: all tests panic with `not implemented`.

---

## Task 3: Implement `PortListResolver`

**Files:**
- Modify: `internal/services/portresolver.go`

- [ ] **Step 1: Replace the stub with the full implementation**

Replace the entire contents of `internal/services/portresolver.go`:

```go
// Package services — PortListResolver merges three port sources for SmartScan stages.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

// portResolverTimeout caps all three DB queries inside Resolve.
const portResolverTimeout = 3 * time.Second

// defaultTopPortsLimit is used when the settings key is 0 or missing.
const defaultTopPortsLimit = 256

// stageDefaultPorts maps each stage name to its hardcoded fallback port string.
// These mirror the constants in smartscan.go and must be kept in sync.
var stageDefaultPorts = map[string]string{
	"os_detection":        "22,80,135,443,445,3389",
	"identity_enrichment": "22,80,161,443",
	"refresh":             "1-1024",
}

// portRangeRE detects nmap range notation (e.g. "1-1024") inside a port string.
var portRangeRE = regexp.MustCompile(`\b\d+-\d+\b`)

// PortListResolver builds a merged, deduplicated port string for a SmartScan stage
// by combining three sources:
//  1. Operator-configured base ports from the settings table (Proposal C).
//  2. OS-matched curated ports from port_definitions (Proposal B).
//  3. Fleet top-N open ports from port_scans (Proposal A).
type PortListResolver struct {
	db *db.DB
}

// NewPortListResolver creates a PortListResolver backed by the given database.
func NewPortListResolver(database *db.DB) *PortListResolver {
	return &PortListResolver{db: database}
}

// Resolve returns the merged port string for the given stage and host.
// If the base string contains a port range (e.g. "1-1024") it is returned
// unchanged — ranges already cover any individual ports the other sources
// would add. On any DB error the affected source is skipped (fail-open).
func (r *PortListResolver) Resolve(ctx context.Context, stage string, host *db.Host) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, portResolverTimeout)
	defer cancel()

	base := r.readBasePorts(ctx, stage)
	limit := r.readLimit(ctx)

	// Ranges are intentionally broad; augmentation would be redundant.
	if portRangeRE.MatchString(base) {
		return base, nil
	}

	ports := parseCSVPorts(base)

	if host.OSFamily != nil && *host.OSFamily != "" {
		ports = append(ports, r.queryOSPorts(ctx, *host.OSFamily, limit)...)
	}

	ports = append(ports, r.queryFleetTopPorts(ctx, limit)...)

	return mergePorts(ports, limit), nil
}

// readBasePorts returns the base port string for the stage from settings,
// falling back to the hardcoded default if the key is absent or empty.
// settings.value is stored as JSONB; string values are JSON-quoted ("\"..\"").
func (r *PortListResolver) readBasePorts(ctx context.Context, stage string) string {
	fallback, ok := stageDefaultPorts[stage]
	if !ok {
		fallback = "1-1024"
	}

	key := "smartscan." + stage + ".ports"
	var rawValue string
	err := r.db.QueryRowContext(ctx,
		`SELECT value::text FROM settings WHERE key = $1`, key,
	).Scan(&rawValue)
	if err != nil {
		return fallback
	}

	// JSONB string values are JSON-quoted: `"22,80,443"` → strip outer quotes.
	var s string
	if jsonErr := json.Unmarshal([]byte(rawValue), &s); jsonErr == nil && s != "" {
		return s
	}
	// Fallback: value was stored as a bare string (int settings, not expected here).
	if rawValue != "" {
		return rawValue
	}
	return fallback
}

// readLimit returns the top_ports_limit setting value, defaulting to 256.
func (r *PortListResolver) readLimit(ctx context.Context) int {
	var rawValue string
	err := r.db.QueryRowContext(ctx,
		`SELECT value::text FROM settings WHERE key = 'smartscan.top_ports_limit'`,
	).Scan(&rawValue)
	if err != nil {
		return defaultTopPortsLimit
	}
	n, err := strconv.Atoi(strings.Trim(rawValue, `"`))
	if err != nil || n <= 0 {
		return defaultTopPortsLimit
	}
	return n
}

// queryOSPorts queries port_definitions for TCP ports associated with the given
// OS family. Returns at most limit port numbers.
func (r *PortListResolver) queryOSPorts(ctx context.Context, osFamily string, limit int) []int {
	rows, err := r.db.QueryContext(ctx, `
		SELECT port
		FROM port_definitions
		WHERE os_families @> ARRAY[$1]::text[]
		  AND protocol = 'tcp'
		ORDER BY is_standard DESC, port ASC
		LIMIT $2`, osFamily, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

// queryFleetTopPorts returns the top-N ports most commonly seen open across
// the fleet, ordered by distinct host count descending.
func (r *PortListResolver) queryFleetTopPorts(ctx context.Context, limit int) []int {
	rows, err := r.db.QueryContext(ctx, `
		SELECT port
		FROM port_scans
		WHERE state = 'open'
		GROUP BY port
		ORDER BY COUNT(DISTINCT host_id) DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

// parseCSVPorts splits a comma-separated port string into integers.
// Non-numeric tokens (ranges, labels) are silently skipped.
func parseCSVPorts(s string) []int {
	var ports []int
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if n, err := strconv.Atoi(tok); err == nil && n > 0 && n <= 65535 {
			ports = append(ports, n)
		}
	}
	return ports
}

// mergePorts deduplicates ports, sorts them numerically, caps at limit,
// and returns them as a comma-separated string.
func mergePorts(ports []int, limit int) string {
	seen := make(map[int]struct{}, len(ports))
	unique := make([]int, 0, len(ports))
	for _, p := range ports {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			unique = append(unique, p)
		}
	}
	sort.Ints(unique)
	if limit > 0 && len(unique) > limit {
		unique = unique[:limit]
	}
	parts := make([]string, len(unique))
	for i, p := range unique {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

// portListStr is a convenience alias used by SmartScanService stage methods
// to call Resolve and fall back to the hardcoded default on error.
func portListStr(
	ctx context.Context,
	resolver portListResolverIface,
	stage string,
	host *db.Host,
	fallback string,
) string {
	if resolver == nil {
		return fallback
	}
	ports, err := resolver.Resolve(ctx, stage, host)
	if err != nil || ports == "" {
		return fallback
	}
	return ports
}

// portListResolverIface is the interface satisfied by PortListResolver.
// Defined here so SmartScanService can depend on the interface, not the concrete type,
// enabling test stubs.
type portListResolverIface interface {
	Resolve(ctx context.Context, stage string, host *db.Host) (string, error)
}

// Compile-time assertion that PortListResolver implements the interface.
var _ portListResolverIface = (*PortListResolver)(nil)

// portListStr returns the resolved port string or the given fallback if the
// resolver is nil or returns an error.
func portListStr(
	ctx context.Context,
	resolver portListResolverIface,
	stage string,
	host *db.Host,
	fallback string,
) string {
	if resolver == nil {
		return fallback
	}
	s, err := resolver.Resolve(ctx, stage, host)
	if err != nil || s == "" {
		return fallback
	}
	return s
}
```

**Note:** The file above has `portListStr` defined twice — remove the first (stub) occurrence. The final file should have it only once.

- [ ] **Step 2: Fix duplicate `portListStr` — ensure it appears only once**

The final file should define `portListStr` exactly once (the second, complete version). Delete the first incomplete declaration.

- [ ] **Step 3: Run the resolver tests**

```bash
go test ./internal/services/ -run TestPortListResolver -v 2>&1
```
Expected: all 7 tests PASS.

- [ ] **Step 4: Run the full service test suite to confirm no regressions**

```bash
go test ./internal/services/ -v 2>&1 | tail -20
```
Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/services/portresolver.go internal/services/portresolver_test.go
git commit -m "feat(services): add PortListResolver — dynamic port list merging"
```

---

## Task 4: Wire `PortListResolver` into `SmartScanService`

**Files:**
- Modify: `internal/services/smartscan.go`

- [ ] **Step 1: Add `portListResolver` field and `WithPortListResolver` method**

In `internal/services/smartscan.go`, inside the `SmartScanService` struct (around line 139), add the field after `logger`:

```go
// portListResolver merges settings, OS, and fleet port sources per stage.
// nil = use hardcoded defaults (backward-compatible, all existing tests pass).
portListResolver portListResolverIface
```

Add the fluent builder method after `WithAutoProgression` (around line 180):

```go
// WithPortListResolver attaches a PortListResolver to the service, enabling
// dynamic port list merging for os_detection, identity_enrichment, and refresh.
func (s *SmartScanService) WithPortListResolver(r portListResolverIface) *SmartScanService {
	s.portListResolver = r
	return s
}
```

- [ ] **Step 2: Update `stageOSDetection` to accept ctx + host and use resolver**

Replace the existing `stageOSDetection` method (around line 433):

```go
// stageOSDetection returns a ScanStage configured for OS fingerprinting.
// SYN scan is required because OS detection needs raw socket access (-O flag).
// When a PortListResolver is configured the port list is augmented with
// OS-specific and fleet-observed ports; otherwise the hardcoded default is used.
func (s *SmartScanService) stageOSDetection(ctx context.Context, host *db.Host) *ScanStage {
	const fallback = "22,80,135,443,445,3389"
	return &ScanStage{
		Stage:       "os_detection",
		ScanType:    "syn",
		Ports:       portListStr(ctx, s.portListResolver, "os_detection", host, fallback),
		OSDetection: true,
		Reason:      "no OS information — running OS fingerprint scan",
	}
}
```

- [ ] **Step 3: Update `stageIdentityEnrichment` to accept ctx + host**

Replace the existing `stageIdentityEnrichment` method (around line 385):

```go
// stageIdentityEnrichment returns a ScanStage that probes the ports most
// likely to yield a host name. When a PortListResolver is configured the port
// list is augmented with OS-specific and fleet-observed ports.
func (s *SmartScanService) stageIdentityEnrichment(ctx context.Context, host *db.Host) *ScanStage {
	return &ScanStage{
		Stage:    stageIdentityEnrichment,
		ScanType: "connect",
		Ports:    portListStr(ctx, s.portListResolver, "identity_enrichment", host, identityEnrichmentPorts),
		Reason:   "host has no usable name — probing identity surfaces (mDNS, SNMP, DNS, TLS)",
	}
}
```

- [ ] **Step 4: Update all call sites of the changed methods**

In `EvaluateHost` (around line 364), change:
```go
// OLD:
case !hasOS && host.Status == "up":
    return s.stageOSDetection(), nil
case host.Status == "up" && !hasUsableHostName(host):
    return s.stageIdentityEnrichment(), nil

// NEW:
case !hasOS && host.Status == "up":
    return s.stageOSDetection(ctx, host), nil
case host.Status == "up" && !hasUsableHostName(host):
    return s.stageIdentityEnrichment(ctx, host), nil
```

In `EvaluateHost`, the refresh branch (around line 373), change:
```go
// OLD:
case isStale:
    return &ScanStage{
        Stage:    "refresh",
        ScanType: "connect",
        Ports:    "1-1024",
        Reason:   fmt.Sprintf("last seen %s ago — refreshing scan", time.Since(host.LastSeen).Round(time.Hour)),
    }, nil

// NEW:
case isStale:
    return &ScanStage{
        Stage:    "refresh",
        ScanType: "connect",
        Ports:    portListStr(ctx, s.portListResolver, "refresh", host, "1-1024"),
        Reason:   fmt.Sprintf("last seen %s ago — refreshing scan", time.Since(host.LastSeen).Round(time.Hour)),
    }, nil
```

In `QueueIdentityEnrichment` (around line 518), change:
```go
// OLD:
stage := s.stageIdentityEnrichment()

// NEW:
stage := s.stageIdentityEnrichment(ctx, host)
```

- [ ] **Step 5: Build to catch compilation errors**

```bash
go build ./internal/services/ 2>&1
```
Expected: no output (clean build).

- [ ] **Step 6: Run the full service test suite**

```bash
go test ./internal/services/ -v 2>&1 | tail -30
```
Expected: all tests PASS — existing tests unaffected because `portListResolver` is nil by default.

- [ ] **Step 7: Commit**

```bash
git add internal/services/smartscan.go
git commit -m "feat(smartscan): wire PortListResolver into os_detection, identity_enrichment, refresh"
```

---

## Task 5: SmartScan resolver integration tests

**Files:**
- Modify: `internal/services/smartscan_test.go`

- [ ] **Step 1: Add a stub resolver and new test cases**

Append to `internal/services/smartscan_test.go`:

```go
// ── PortListResolver integration ──────────────────────────────────────────────

// stubResolver is a test double for portListResolverIface.
type stubResolver struct {
	ports string
	err   error
	calls []string // records which stages were resolved
}

func (s *stubResolver) Resolve(_ context.Context, stage string, _ *db.Host) (string, error) {
	s.calls = append(s.calls, stage)
	return s.ports, s.err
}

func TestEvaluateHost_OSDetection_UsesResolver(t *testing.T) {
	svc := newTestService(false, false) // no open ports → os_detection fires
	resolver := &stubResolver{ports: "22,80,9001"}
	svc.portListResolver = resolver

	host := hostUp("10.0.0.1")
	host.OSFamily = nil // no OS → triggers os_detection
	host.Hostname = nil // clear to avoid identity_enrichment short-circuit

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "os_detection", stage.Stage)
	assert.Equal(t, "22,80,9001", stage.Ports)
	assert.Contains(t, resolver.calls, "os_detection")
}

func TestEvaluateHost_IdentityEnrichment_UsesResolver(t *testing.T) {
	family := "linux"
	svc := newTestService(false, false)
	resolver := &stubResolver{ports: "22,80,161,443,9001"}
	svc.portListResolver = resolver

	host := hostUp("10.0.0.2")
	host.OSFamily = &family
	host.Hostname = nil  // no usable name → identity_enrichment fires
	host.MDNSName = nil
	host.CustomName = nil

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, stageIdentityEnrichment, stage.Stage)
	assert.Equal(t, "22,80,161,443,9001", stage.Ports)
	assert.Contains(t, resolver.calls, "identity_enrichment")
}

func TestEvaluateHost_Refresh_UsesResolver(t *testing.T) {
	family := "linux"
	svc := newTestService(true, true) // has ports + services → only stale check remains
	resolver := &stubResolver{ports: "22,80,443"}
	svc.portListResolver = resolver

	host := hostUp("10.0.0.3")
	host.OSFamily = &family
	host.Hostname = strPtr("srv.example.com") // usable name → skip identity_enrichment
	host.LastSeen = time.Now().Add(-31 * 24 * time.Hour) // stale

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "refresh", stage.Stage)
	assert.Equal(t, "22,80,443", stage.Ports)
	assert.Contains(t, resolver.calls, "refresh")
}

func TestEvaluateHost_ResolverError_FallsBackToHardcoded(t *testing.T) {
	svc := newTestService(false, false)
	resolver := &stubResolver{err: fmt.Errorf("db timeout")}
	svc.portListResolver = resolver

	host := hostUp("10.0.0.4")
	host.OSFamily = nil
	host.Hostname = nil

	stage, err := svc.EvaluateHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, "os_detection", stage.Stage)
	// fallback hardcoded value
	assert.Equal(t, "22,80,135,443,445,3389", stage.Ports)
}
```

- [ ] **Step 2: Run the new tests**

```bash
go test ./internal/services/ -run TestEvaluateHost_OSDetection_UsesResolver,TestEvaluateHost_IdentityEnrichment_UsesResolver,TestEvaluateHost_Refresh_UsesResolver,TestEvaluateHost_ResolverError_FallsBackToHardcoded -v 2>&1
```
Expected: all 4 PASS.

- [ ] **Step 3: Run the full suite to confirm no regressions**

```bash
go test ./internal/services/ -race 2>&1 | tail -10
```
Expected: `ok  	github.com/anstrom/scanorama/internal/services`

- [ ] **Step 4: Commit**

```bash
git add internal/services/smartscan_test.go
git commit -m "test(smartscan): add PortListResolver integration tests for three stage methods"
```

---

## Task 6: Wire resolver in production

**Files:**
- Modify: `internal/api/routes.go`

- [ ] **Step 1: Add `WithPortListResolver` to the SmartScanService chain**

In `internal/api/routes.go`, find the `NewSmartScanService` chain (around line 52) and add `WithPortListResolver`:

```go
// BEFORE:
smartScanSvc := services.NewSmartScanService(s.database, profileManager, s.scanQueue, s.logger).
    WithAutoProgression(
        services.AutoProgressDefaultThreshold,
        services.AutoProgressDefaultMaxPerWindow,
        services.AutoProgressDefaultWindowHours,
    )

// AFTER:
smartScanSvc := services.NewSmartScanService(s.database, profileManager, s.scanQueue, s.logger).
    WithAutoProgression(
        services.AutoProgressDefaultThreshold,
        services.AutoProgressDefaultMaxPerWindow,
        services.AutoProgressDefaultWindowHours,
    ).
    WithPortListResolver(services.NewPortListResolver(s.database))
```

- [ ] **Step 2: Build the entire project**

```bash
go build ./... 2>&1
```
Expected: no output (clean build).

- [ ] **Step 3: Run the full test suite**

```bash
go test -race ./internal/... 2>&1 | tail -20
```
Expected: all packages pass.

- [ ] **Step 4: Commit**

```bash
git add internal/api/routes.go
git commit -m "feat(api): wire PortListResolver into SmartScanService in production"
```

---

## Task 7: Push and open PR

- [ ] **Step 1: Ensure pre-push checks pass**

```bash
make lint
go test -race ./internal/...
```
Expected: no lint issues, all tests green.

- [ ] **Step 2: Push and open PR**

```bash
git push origin HEAD
gh pr create \
  --title "feat: dynamic SmartScan port lists + expanded port definitions" \
  --body "$(cat <<'EOF'
## Summary

- **Proposal A**: Fleet top-N ports (aggregate over `port_scans`) merged into SmartScan stage port lists
- **Proposal B**: OS-family-matched ports from `port_definitions` augment `os_detection` and `identity_enrichment`
- **Proposal C**: Per-stage base port lists are now operator-configurable via `settings` table
- **Migration 028**: ~50 new `port_definitions` entries (Prometheus exporters, virtualization, data infra, service mesh, DevOps tooling)

## Test plan

- [ ] Apply migration on a running dev instance; confirm `SELECT COUNT(*) FROM port_definitions` increases and the four `smartscan.*` settings keys appear in `SELECT key FROM settings WHERE key LIKE 'smartscan.%'`
- [ ] Trigger a SmartScan for a host with no OS info; confirm the queued scan's ports include ports beyond the previous 6-port hardcoded list when fleet history exists
- [ ] Update `smartscan.os_detection.ports` setting via `UPDATE settings SET value = '"22,443"'::jsonb WHERE key = 'smartscan.os_detection.ports'`; restart backend; confirm next `os_detection` scan uses the new base list

Closes #742
EOF
)"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| Proposal A — fleet top-N aggregate | Task 3 (`queryFleetTopPorts`) |
| Proposal B — OS-family port_definitions filter | Task 3 (`queryOSPorts`) |
| Proposal C — settings-driven base ports | Task 3 (`readBasePorts`) |
| Expand port_definitions (~150 entries) | Task 1 |
| Settings keys seeded in migration | Task 1 |
| `portListResolverIface` interface | Task 3 (`portresolver.go`) |
| `WithPortListResolver` builder | Task 4 |
| `stageOSDetection` uses resolver | Task 4 |
| `stageIdentityEnrichment` uses resolver | Task 4 |
| `refresh` branch uses resolver | Task 4 |
| `stageWithProfile` untouched | ✅ (no task modifies it) |
| Resolver error → fail-open fallback | Task 3 (`portListStr`) + Task 5 |
| Range base → no augmentation | Task 3 (`portRangeRE`) + Task 2 test |
| cap 0 → default 256 | Task 3 (`readLimit`) + Task 2 test |
| Production wiring in routes.go | Task 6 |

**Placeholder scan:** No TBDs. All code blocks are complete.

**Type consistency:** `portListResolverIface` is defined in `portresolver.go` and referenced in `smartscan.go` (same package — no import needed). `portListStr` helper is defined in `portresolver.go` and called in `smartscan.go`. `stubResolver` in tests implements `portListResolverIface` — `Resolve` signature matches exactly.
