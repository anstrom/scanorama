# Scanorama

[![CI](https://github.com/anstrom/scanorama/actions/workflows/main.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/main.yml)
[![Security](https://github.com/anstrom/scanorama/actions/workflows/security.yml/badge.svg)](https://github.com/anstrom/scanorama/actions/workflows/security.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anstrom/scanorama)](https://goreportcard.com/report/github.com/anstrom/scanorama)
[![codecov](https://codecov.io/gh/anstrom/scanorama/branch/main/graph/badge.svg)](https://codecov.io/gh/anstrom/scanorama)

Scanorama is a self-hosted network scanning and discovery tool. It discovers hosts on your networks, tracks port states, OS fingerprints, MAC vendors, and response times over time, and surfaces everything in a dense operator dashboard. A Go backend handles scanning, scheduling, and persistence; a React frontend provides the UI.

---

## Features

### Discovery & Scanning

- **Network discovery** via nmap — ping sweep, TCP probe, or ARP, with optional OS fingerprinting
- **Port scanning** with five scan modes: connect (TCP), SYN stealth, version detection, aggressive, and comprehensive full-range
- **Scan profiles** — named, reusable configurations for ports, scan type, and options; profiles are cloneable
- **Scheduled jobs** — cron-based scheduling for both discovery and scan jobs, with enable/disable and next-run preview
- **Worker pool** — configurable number of concurrent scan workers; admin panel shows live worker status and queue depth
- **Network exclusions** — per-network or global IP/CIDR exclusion lists respected during all scan and discovery operations

### Host Tracking

- **Persistent host inventory** with full scan history per host
- **Status model** — four states: `up`, `down`, `unknown`, `gone`; state transitions (e.g. up → gone) are recorded with timestamps
- **Response time capture** — min, max, and average RTT per host, measured during discovery and scan probes; slow hosts flagged
- **Scan duration per host** — how long each scan took for each individual host
- **Timeout tracking** — timeouts recorded as distinct events, separate from clean "down" states
- **MAC vendor lookup** — OUI resolution via MAC address during discovery; vendor name (Cisco, Raspberry Pi, Dell, etc.) displayed on the host list and in host detail
- **Hostname management** — inline editing of hostnames from the host detail panel

### Discovery Changelog

- **Diff view** — after every discovery run, see exactly which hosts are new, gone, or changed (status change, new ports, etc.)
- **"Gone" host retention** — hosts that stop responding are marked `gone` with a last-seen timestamp, not deleted
- **History comparison** — compare any two completed discovery runs side by side: what appeared between run A and run B, what disappeared, what changed; edge cases handled (same-run short-circuits, different-network returns a clear error)
- **Discovery notifications** — WebSocket notification fires when a scheduled discovery completes; a clickable toast summarises new/gone/changed counts and links to the diff view
- **Dashboard widget** — Recent Discovery Changes card shows the diff from the latest completed run

### Advanced Filtering

- **Filter builder UI** on the hosts page with compound AND/OR logic
- **Fields**: status, OS family, vendor, hostname, response time (ms), first seen, last seen, open port, scan count
- **Operators**: is, is not, contains (text), >, <, between (numeric/date)
- **Sub-groups** — one level of nesting for complex expressions like `(status is up AND os_family is Linux) OR open_port is 443`
- **Server-side SQL translation** — filters are translated to parameterised SQL; port-based filtering uses a correlated `EXISTS` subquery against scan results
- **URL persistence** — active filter serialised as base64url in `?filter=` for shareable links
- **Saved presets** — named filter presets stored in localStorage, loaded from a dropdown; up to 20 presets

### Table Power-Ups (all list views)

- **Universal server-side sorting** — every list view (Hosts, Scans, Networks, Schedules, Profiles, Exclusions) supports sortable columns
- **Multi-select** — checkbox column with shift-click range selection and select-all-on-page
- **Bulk delete** — delete multiple hosts in one action with confirmation
- **Bulk scan** — select multiple hosts and launch a scan against the selection in one click
- **Column visibility** — show/hide table columns per user preference (persisted in localStorage)
- **Keyboard navigation** — arrow keys move between rows, Enter opens the detail panel, Space toggles selection, Escape dismisses

### Real-Time & Admin

- **WebSocket streams** — three channels: `GET /api/v1/ws` (general events including discovery), `GET /api/v1/ws/scans` (scan progress), `GET /api/v1/ws/logs` (live log tail)
- **Admin panel** — system health, database status, uptime, build info, worker status table, and a streaming log viewer
- **Prometheus metrics** — scan throughput, error rates, and operational counters exposed at `/metrics`
- **Structured logging** — slog-based, text or JSON format, configurable level

### API

- Full REST API with `X-API-Key` authentication
- Interactive Swagger UI at `/swagger/`
- API key management via CLI or API

---

## Requirements

| Dependency | Version | Notes |
|---|---|---|
| **Go** | 1.26+ | Backend build |
| **PostgreSQL** | 14+ | Persistence |
| **nmap** | 7.0+ | Required at runtime |
| **Node.js** | 20+ | Frontend dev only |

---

## Quick Start

```bash
git clone https://github.com/anstrom/scanorama.git
cd scanorama

# Build the Go binary
make build

# Copy and edit config
cp config/environments/config.example.yaml config.yaml
# Set: database.host, database.username, database.password, api.api_keys

# Start a PostgreSQL instance (or use your own)
make dev-db-up

# Run the API server
./build/scanorama api
```

The API is available at `http://localhost:8080` and the Swagger UI at `http://localhost:8080/swagger/`.

---

## Development Setup

`make dev` starts everything — it builds the binary, ensures the dev database is running, starts the backend as root (required for nmap SYN scans), and launches the Vite dev server.

```bash
# First-time setup (installs Go tools + npm deps)
make dev-setup

# Start backend + frontend (hot reload on both)
make dev

# Frontend is served at http://localhost:5173 (proxies /api and /ws to :8080)
# Backend API is at http://localhost:8080
```

Other useful targets:

```bash
make test          # Run all Go tests
make test-unit     # Unit tests only (no DB required)
make lint          # golangci-lint
make check         # lint + tests in one pass
make dev-db-shell  # psql shell to the dev database
make dev-nuke      # Tear down the dev DB and delete all data
```

### Frontend

The frontend is a React 19 + Vite 6 + TypeScript 5 single-page app, styled with Tailwind CSS 4, using TanStack Router and TanStack Query for routing and data fetching.

```bash
cd frontend
npm install
npm run dev      # Dev server at http://localhost:5173
npm test         # Vitest unit tests (835 tests)
npm run build    # Production build to frontend/dist/
```

**Stack:**

| Layer | Library |
|---|---|
| Framework | React 19 |
| Build | Vite 6 |
| Language | TypeScript 5 |
| Styling | Tailwind CSS 4 |
| Routing | TanStack Router 1 |
| Data fetching | TanStack Query 5 |
| Charts | Recharts 3 |
| Icons | Lucide React |
| Validation | Zod 4 |
| Testing | Vitest 4 + Testing Library + MSW 2 |

The design direction is dark-first and information-dense — built for people who stare at IP addresses all day.

---

## Frontend Pages

| Route | Page | Description |
|---|---|---|
| `/` | Dashboard | Stat cards (active hosts, networks, scans), 7-day scan activity chart, recent discovery changes widget |
| `/hosts` | Hosts | Full host inventory with advanced filter builder, response times, vendor info, bulk ops, detail panel |
| `/scans` | Scans | Scan job list with status, launch new scans, stop/delete, results detail panel |
| `/networks` | Networks | Network CIDR management, enable/disable, per-network discovery and scan triggers |
| `/discovery` | Discovery | Discovery run list, diff view per run, history comparison panel, toast notifications |
| `/schedules` | Schedules | Cron-based scheduled discovery and scan jobs; create/edit/enable/disable |
| `/profiles` | Profiles | Scan profile CRUD; clone existing profiles as a starting point |
| `/exclusions` | Exclusions | Global and per-network IP/CIDR exclusion lists |
| `/admin` | Admin | System health, worker status table, live log viewer |

---

## CLI Reference

### API Keys

```bash
# Create an API key
scanorama apikeys create --name "my-key"
# → sk_live_abc123...

export SCANORAMA_API_KEY=sk_live_abc123...

scanorama apikeys list
scanorama apikeys delete <id>
```

### Discovery

```bash
# Discover a CIDR
scanorama discover 192.168.1.0/24

# Save the network and run discovery
scanorama discover 192.168.1.0/24 --add --name "home-lan"

# Discover all configured networks
scanorama discover --configured-networks

# Auto-detect and discover all local interfaces
scanorama discover --all-networks

# Discovery method (default: ping)
scanorama discover 10.0.0.0/24 --method tcp    # tcp probe
scanorama discover 10.0.0.0/24 --method arp    # ARP (local only)

# Enable OS fingerprinting during discovery (requires root)
scanorama discover 192.168.1.0/24 --detect-os
```

### Scanning

```bash
# Scan a specific target
scanorama scan --targets 192.168.1.1

# Scan with explicit ports
scanorama scan --targets "192.168.1.1,192.168.1.10" --ports "22,80,443"

# Scan all known live hosts
scanorama scan --live-hosts

# Filter live hosts by OS family
scanorama scan --live-hosts --os-family windows

# Use a saved scan profile
scanorama scan --targets 192.168.1.1 --profile linux-server

# Scan types
scanorama scan --targets 192.168.1.1 --type connect       # TCP connect (default, no root)
scanorama scan --targets 192.168.1.1 --type syn           # SYN stealth (requires root)
scanorama scan --targets 192.168.1.1 --type version       # Service version detection
scanorama scan --targets 192.168.1.1 --type aggressive    # OS + version + scripts
scanorama scan --targets 192.168.1.1 --type comprehensive # Full 65535-port range
```

### Scheduling

```bash
scanorama schedule list

# Schedule weekly discovery (every Sunday at 02:00)
scanorama schedule add-discovery "weekly-sweep" "0 2 * * 0" "10.0.0.0/8"

# Schedule a scan every 6 hours against live hosts
scanorama schedule add-scan "frequent-scan" "0 */6 * * *" --live-hosts

scanorama schedule remove weekly-sweep
```

### Running the Server

```bash
scanorama api              # API server in the foreground
scanorama daemon start     # Start background daemon with API server
scanorama daemon status    # Check daemon status
scanorama daemon stop      # Stop daemon
```

---

## Configuration

```bash
cp config/environments/config.example.yaml config.yaml
```

```yaml
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: your_secure_password
  ssl_mode: prefer            # disable | require | verify-ca | verify-full | prefer
  max_open_conns: 25
  max_idle_conns: 5

scanning:
  worker_pool_size: 10
  default_ports: "22,80,443,8080,8443"
  default_scan_type: connect  # connect | syn | version | aggressive | comprehensive
  enable_os_detection: false  # requires root / CAP_NET_RAW
  max_scan_timeout: "10m"
  rate_limit:
    enabled: true
    requests_per_second: 100

api:
  enabled: true
  listen_addr: "127.0.0.1"
  port: 8080
  auth_enabled: true
  api_keys:
    - "your-api-key-here"
  cors:
    enabled: true
    allowed_origins: ["*"]

daemon:
  pid_file: "/var/run/scanorama.pid"
  work_dir: "/var/lib/scanorama"
  user: ""    # drop privileges to this user after binding (Linux)
  group: ""
  shutdown_timeout: "30s"

logging:
  level: info     # debug | info | warn | error
  format: text    # text | json
  output: stdout
```

### Environment Variables

All configuration values can be overridden with environment variables:

| Variable | Config key |
|---|---|
| `SCANORAMA_DB_HOST` | `database.host` |
| `SCANORAMA_DB_PORT` | `database.port` |
| `SCANORAMA_DB_NAME` | `database.database` |
| `SCANORAMA_DB_USER` | `database.username` |
| `SCANORAMA_DB_PASSWORD` | `database.password` |
| `SCANORAMA_DB_SSLMODE` | `database.ssl_mode` |
| `SCANORAMA_API_KEY` | API key for CLI / client authentication |
| `SCANORAMA_LOG_LEVEL` | `logging.level` |
| `SCANORAMA_LOG_FORMAT` | `logging.format` |
| `SCANORAMA_USER` | `daemon.user` |
| `SCANORAMA_GROUP` | `daemon.group` |
| `SCANORAMA_PID_FILE` | `daemon.pid_file` |

---

## API Reference

All endpoints (except `/api/v1/health` and `/api/v1/liveness`) require an `X-API-Key` header.

### System

```
GET  /api/v1/health
GET  /api/v1/liveness
GET  /api/v1/status
GET  /api/v1/version
GET  /api/v1/metrics
GET  /api/v1/admin/status
GET  /api/v1/admin/workers
GET  /api/v1/admin/logs
```

### Hosts

```
GET    /api/v1/hosts                   List hosts (filtering, sorting, pagination, advanced filter)
POST   /api/v1/hosts                   Create host
DELETE /api/v1/hosts                   Bulk delete hosts (body: {"ids": [...]})
GET    /api/v1/hosts/{id}              Get host
PUT    /api/v1/hosts/{id}              Update host
DELETE /api/v1/hosts/{id}              Delete host
GET    /api/v1/hosts/{id}/scans        Get scan history for a host
```

**Host list query parameters:**

| Parameter | Description |
|---|---|
| `page`, `page_size` | Pagination |
| `sort_by`, `sort_order` | Column sort (`asc`/`desc`) |
| `status` | Filter by status (`up`/`down`/`unknown`/`gone`) |
| `os` | Filter by OS family |
| `vendor` | Filter by MAC vendor |
| `search` | Full-text search on IP and hostname |
| `filter` | JSON-encoded advanced filter expression (see below) |

**Advanced filter expression format:**

A filter is a JSON group node with `op` (`AND`/`OR`) and `conditions` — each condition is either a leaf or a nested group (max 3 levels deep, max 20 conditions per group).

```json
{
  "op": "AND",
  "conditions": [
    { "field": "status", "cmp": "is", "value": "up" },
    { "field": "os_family", "cmp": "contains", "value": "Linux" },
    {
      "op": "OR",
      "conditions": [
        { "field": "open_port", "cmp": "is", "value": "22" },
        { "field": "open_port", "cmp": "is", "value": "2222" }
      ]
    }
  ]
}
```

Supported fields: `status`, `os_family`, `vendor`, `hostname`, `response_time_ms`, `first_seen`, `last_seen`, `open_port`, `scan_count`.  
Supported operators: `is`, `is_not`, `contains` (text fields), `gt`, `lt`, `between` (numeric/date fields).

### Scans

```
GET    /api/v1/scans                   List scans (filtering, sorting, pagination)
POST   /api/v1/scans                   Create scan job
GET    /api/v1/scans/{id}              Get scan
PUT    /api/v1/scans/{id}              Update scan
DELETE /api/v1/scans/{id}             Delete scan
GET    /api/v1/scans/{id}/results      Get scan results (per-host port data)
POST   /api/v1/scans/{id}/start        Start a pending scan
POST   /api/v1/scans/{id}/stop         Stop a running scan
```

### Discovery

```
GET    /api/v1/discovery                              List discovery jobs
POST   /api/v1/discovery                              Create discovery job
GET    /api/v1/discovery/compare?run_a={id}&run_b={id} Compare two discovery runs
GET    /api/v1/discovery/{id}                         Get discovery job
GET    /api/v1/discovery/{id}/diff                    Get new/gone/changed hosts for a run
POST   /api/v1/discovery/{id}/start                   Start discovery
POST   /api/v1/discovery/{id}/stop                    Stop discovery
```

The `/compare` endpoint returns a `DiscoveryCompareDiff` object with `new_hosts`, `gone_hosts`, `changed_hosts`, and `unchanged_count`. Comparing a run with itself short-circuits to all-unchanged. Comparing runs from different networks returns `422`.

### Profiles

```
GET    /api/v1/profiles                List scan profiles
POST   /api/v1/profiles                Create profile
GET    /api/v1/profiles/{id}           Get profile
PUT    /api/v1/profiles/{id}           Update profile
DELETE /api/v1/profiles/{id}           Delete profile
POST   /api/v1/profiles/{id}/clone     Clone profile (returns new profile)
```

### Schedules

```
GET    /api/v1/schedules               List scheduled jobs
POST   /api/v1/schedules               Create schedule
GET    /api/v1/schedules/{id}          Get schedule
PUT    /api/v1/schedules/{id}          Update schedule
DELETE /api/v1/schedules/{id}          Delete schedule
POST   /api/v1/schedules/{id}/enable   Enable schedule
POST   /api/v1/schedules/{id}/disable  Disable schedule
GET    /api/v1/schedules/{id}/next-run Next scheduled execution time
```

### Networks

```
GET    /api/v1/networks                         List networks
POST   /api/v1/networks                         Create network
GET    /api/v1/networks/stats                   Aggregated stats across all networks
GET    /api/v1/networks/{id}                    Get network
PUT    /api/v1/networks/{id}                    Update network
DELETE /api/v1/networks/{id}                    Delete network
POST   /api/v1/networks/{id}/enable             Enable network
POST   /api/v1/networks/{id}/disable            Disable network
PUT    /api/v1/networks/{id}/rename             Rename network
POST   /api/v1/networks/{id}/discover           Start a discovery run on this network
GET    /api/v1/networks/{id}/discovery          List discovery jobs for this network
POST   /api/v1/networks/{id}/scan               Start a scan against this network's hosts
GET    /api/v1/networks/{id}/exclusions         List exclusions for this network
POST   /api/v1/networks/{id}/exclusions         Add an exclusion to this network
```

### Exclusions

```
GET    /api/v1/exclusions              List global exclusions
POST   /api/v1/exclusions              Create global exclusion
DELETE /api/v1/exclusions/{id}         Delete exclusion
```

### API Keys

```
GET    /api/v1/apikeys                 List API keys
POST   /api/v1/apikeys                 Create API key
DELETE /api/v1/apikeys/{id}            Revoke API key
```

### WebSocket

```
GET  /api/v1/ws          General event stream (discovery updates, host changes)
GET  /api/v1/ws/scans    Scan progress stream
GET  /api/v1/ws/logs     Live log tail stream
```

Full interactive documentation at `http://localhost:8080/swagger/`.

### Example: submit a scan and poll for results

```bash
# Create and start a scan
curl -s -X POST http://localhost:8080/api/v1/scans \
  -H "X-API-Key: $SCANORAMA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "quick-check",
    "targets": ["192.168.1.0/24"],
    "scan_type": "connect",
    "ports": "22,80,443"
  }' | jq .id

# Start it
curl -s -X POST http://localhost:8080/api/v1/scans/<id>/start \
  -H "X-API-Key: $SCANORAMA_API_KEY"

# Poll for status
curl -s http://localhost:8080/api/v1/scans/<id> \
  -H "X-API-Key: $SCANORAMA_API_KEY" | jq .status

# Fetch results
curl -s "http://localhost:8080/api/v1/scans/<id>/results?limit=50" \
  -H "X-API-Key: $SCANORAMA_API_KEY" | jq .
```

### Example: filter hosts with compound conditions

```bash
# Linux hosts that are up and have port 22 open
FILTER=$(printf '{"op":"AND","conditions":[{"field":"status","cmp":"is","value":"up"},{"field":"os_family","cmp":"is","value":"Linux"},{"field":"open_port","cmp":"is","value":"22"}]}')

curl -s "http://localhost:8080/api/v1/hosts?filter=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$FILTER")" \
  -H "X-API-Key: $SCANORAMA_API_KEY" | jq '.data | length'
```

---

## Docker

The production Docker Compose stack runs Scanorama, PostgreSQL, and Nginx. Optional profiles add Prometheus and Grafana.

```bash
cd docker

# Create secrets
echo "strong_db_password" > secrets/postgres_password.txt
echo "your-api-key"       > secrets/api_key.txt

# Start core services
docker compose up -d

# With monitoring (Prometheus + Grafana)
docker compose --profile monitoring up -d

# With Redis cache
docker compose --profile cache up -d
```

| Service | Port | Notes |
|---|---|---|
| Nginx | 80 / 443 | Reverse proxy with TLS |
| Scanorama API | 8080 | Direct access |
| PostgreSQL | 5432 | Internal only |
| Prometheus | 9090 | `--profile monitoring` |
| Grafana | 3000 | `--profile monitoring` |

---

## Database Migrations

Migrations run automatically on startup. The schema is managed via numbered SQL files in `internal/db/`.

| Migration | Description |
|---|---|
| `001_initial_schema.sql` | Base schema (hosts, scans, networks, profiles, schedules, exclusions) |
| `002_host_targets.sql` | Host target associations for scan jobs |
| `003_discovery_network_link.sql` | Link between discovery jobs and networks |
| `004_host_status_model.sql` | `gone` status, `host_status_events` trigger for transition tracking |
| `005_response_time.sql` | `response_time_min_ms`, `response_time_max_ms`, `response_time_avg_ms` on hosts |
| `006_scan_duration.sql` | `scan_duration_ms` on `port_scans` |
| `007_timeout_events.sql` | `host_timeout_events` table for timeout frequency tracking |

---

## Project Structure

```
scanorama/
├── cmd/
│   └── scanorama/          Entry point; Cobra CLI commands
├── internal/
│   ├── api/
│   │   ├── handlers/       HTTP handlers, request/response types, mocks
│   │   └── routes.go       Route registration
│   ├── db/                 Repository layer, models, migrations, filter expression engine
│   ├── scanning/           nmap integration, scan worker pool, result parsing
│   ├── discovery/          Discovery engine (ping/TCP/ARP, diff calculation)
│   ├── scheduler/          Cron-based job scheduler (robfig/cron)
│   ├── services/           Business logic layer between handlers and repositories
│   ├── metrics/            Prometheus metrics definitions
│   ├── logging/            slog wrapper
│   └── ws/                 WebSocket hub and broadcast logic
├── frontend/
│   ├── src/
│   │   ├── api/            Type-safe fetch client, TanStack Query hooks
│   │   ├── components/     Shared UI components (FilterBuilder, StatusBadge, etc.)
│   │   ├── lib/            Utilities (filter-expr, date formatting, WebSocket)
│   │   └── routes/         Page components, one file per route
│   └── package.json
├── config/environments/    Example and environment-specific config files
├── docker/                 Production Docker Compose stack
├── docs/
│   ├── planning/           ROADMAP.md, FRONTEND_PLAN.md
│   └── swagger/            Generated OpenAPI spec
└── test/                   Integration test helpers
```

---

## License

MIT — see [LICENSE](LICENSE) for details.