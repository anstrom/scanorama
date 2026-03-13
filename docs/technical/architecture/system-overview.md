# Scanorama System Overview

This document describes the high-level architecture of Scanorama: a network scanning and inventory system built in Go.

## What Is Scanorama?

Scanorama is a continuous network monitoring and inventory tool. It uses [nmap](https://nmap.org/) as its scanning engine and stores results in a PostgreSQL database. The system has three main operational modes:

- **Daemon** – a long-running background process that hosts the REST API and executes scheduled scans and discovery jobs.
- **API server** – an HTTP server exposing a REST API (with WebSocket support) for external consumers and the CLI.
- **CLI** – a command-line client (`scanorama`) for managing the daemon, triggering ad-hoc scans, and querying results.

---

## High-Level Component Diagram

```
┌───────────────────────────────────────────────────────────────────┐
│                        scanorama binary                           │
│                                                                   │
│  ┌──────────────┐      ┌────────────────────────────────────────┐ │
│  │   CLI        │      │              Daemon                    │ │
│  │ (cmd/cli/)   │─────▶│          (internal/daemon/)            │ │
│  └──────────────┘      │                                        │ │
│                        │  ┌──────────────┐  ┌───────────────┐  │ │
│                        │  │  API Server  │  │   Scheduler   │  │ │
│                        │  │(internal/api)│  │(internal/     │  │ │
│                        │  │              │  │ scheduler/)   │  │ │
│                        │  └──────┬───────┘  └──────┬────────┘  │ │
│                        │         │                  │           │ │
│                        └─────────┼──────────────────┼───────────┘ │
└─────────────────────────────────┼──────────────────┼─────────────┘
                                  │                  │
             ┌────────────────────┼──────────────────┤
             ▼                    ▼                  ▼
  ┌────────────────┐   ┌──────────────────┐  ┌─────────────────┐
  │   DB Layer     │   │ Scanning Engine  │  │ Discovery Engine│
  │ (internal/db/) │   │(internal/        │  │(internal/       │
  │                │   │ scanning/)       │  │ discovery/)     │
  │  PostgreSQL    │   │  invokes nmap    │  │  invokes nmap   │
  └────────────────┘   └──────────────────┘  └─────────────────┘
             ▲
  ┌──────────┴──────────────────────────────────────────────────┐
  │                    External Systems                         │
  │    PostgreSQL database        Prometheus metrics scraper    │
  │    nmap binary (system)       HTTP clients / frontends      │
  └─────────────────────────────────────────────────────────────┘
```

---

## Package Reference

### `cmd/scanorama/`

Entry point. Sets build-time version metadata (`version`, `commit`, `buildTime`) via ldflags and delegates to `cmd/cli/`.

### `cmd/cli/`

Cobra-based CLI. Registers all subcommands and provides `init`/`Execute` entry points. Key subcommand files:

| File | Commands |
|------|----------|
| `daemon.go` | `daemon start/stop/status/restart` |
| `server.go` | `server start/stop/status/restart/logs` |
| `scan.go` | `scan run`, ad-hoc scan execution |
| `discover.go` | `discover`, ad-hoc discovery |
| `hosts.go` | `hosts list/get/delete` |
| `networks.go` + helpers | `networks` CRUD and management |
| `schedule.go` | `schedule` CRUD |
| `profiles.go` | `profiles` CRUD |
| `apikeys.go` | `apikeys create/list/revoke` |
| `api.go` / `api_client.go` | HTTP client used by CLI subcommands |

The CLI can operate in two ways:

1. **Direct mode** – connects directly to PostgreSQL and runs operations itself (e.g., ad-hoc scans, host queries).
2. **Client mode** – calls the running daemon's REST API via the built-in HTTP client (`api_client.go`).

### `internal/daemon/`

Manages the daemon lifecycle:

1. Validates configuration.
2. Optionally forks to background (`Daemonize: true`).
3. Drops OS privileges if `User`/`Group` are configured.
4. Writes a PID file.
5. Connects to PostgreSQL and runs all pending database migrations.
6. Starts the API server.
7. Runs a health-check loop (database ping every 10 seconds).
8. Handles `SIGTERM`/`SIGINT` (graceful shutdown), `SIGHUP` (config reload), `SIGUSR1` (status dump), `SIGUSR2` (toggle debug).

### `internal/api/`

REST API server built on `gorilla/mux`. Responsibilities:

- Route registration (`routes.go`) – maps URL patterns to handler functions.
- Middleware pipeline (`middleware/middleware.go`) – request logging, authentication, rate limiting, CORS, recovery, security headers, compression.
- Handler implementations (`handlers/`) – one file per resource group.
- WebSocket hub (`handlers/websocket.go`) – broadcasts real-time scan and discovery updates.
- Prometheus metrics endpoint (`/metrics`) served via `prometheus/client_golang`.
- Swagger/OpenAPI UI at `/swagger/` (generated docs embedded at compile time).

### `internal/api/handlers/`

| File | Responsibility |
|------|----------------|
| `scan.go` | Scan CRUD, start/stop actions, result retrieval |
| `host.go` | Host CRUD, host scan history |
| `discovery.go` | Discovery job CRUD, start/stop |
| `profile.go` | Scan profile CRUD |
| `schedule.go` | Scheduled job CRUD, enable/disable |
| `networks.go` | Network CRUD, enable/disable, exclusions |
| `admin.go` | Admin status endpoint |
| `admin_types.go` | Request/response types for admin endpoints |
| `admin_config.go` | Config retrieval and parsing helpers |
| `admin_validate.go` | Validation logic for admin operations |
| `websocket.go` | WebSocket upgrade and broadcast hub |
| `health.go` | Liveness and health check endpoints |
| `common.go` | Shared helpers: `writeJSON`, `writeError`, pagination, request IDs |
| `manager.go` | `HandlerManager` – owns the WebSocket handler, exposes `GeneralWebSocket` |

### `internal/api/middleware/`

Middleware chain applied to all routes (in order):

1. `Recovery` – catches panics and returns HTTP 500.
2. `Logging` – structured request/response logging with request IDs.
3. `CORS` – configurable allowed origins, methods, and headers.
4. `Authentication` – API key validation (checked against config list and/or database).
5. `ContentType` – enforces `application/json` on mutating requests.

Additional middleware available: `RateLimit`, `RequestTimeout`, `SecurityHeaders`, `Compression`.

### `internal/db/`

Database access layer using `jmoiron/sqlx` on top of `database/sql`.

| File | Responsibility |
|------|----------------|
| `database.go` | Connection management, `DB` type, error sanitization |
| `migrate.go` | Embedded SQL migration runner (auto-runs at startup) |
| `models.go` | Shared data model types |
| `hosts.go` | Host CRUD operations |
| `scans.go` | Scan job and result operations |
| `networks.go` | Network and exclusion operations |
| `profiles.go` | Scan profile operations |
| `scheduled_jobs.go` | Scheduled job persistence |
| `001_initial_schema.sql` … `006_api_keys_table.sql` | Versioned SQL migrations (embedded via `//go:embed`) |

Migrations are applied automatically at startup via `ConnectAndMigrate`. A `schema_migrations` tracking table records each applied migration by name and SHA-256 checksum.

### `internal/scheduler/`

Cron-based job scheduler (`robfig/cron`). Manages two job types:

- **Discovery jobs** – trigger the discovery engine for a given network CIDR on a cron schedule.
- **Scan jobs** – query the host table (with optional filters: live hosts only, OS family, network, profile) and run a port scan against each matched host. Bounded concurrency is enforced via a semaphore (`maxConcurrentScans`, default 5).

Jobs are stored in the `scheduled_jobs` PostgreSQL table and reloaded into memory on scheduler start. The scheduler supports runtime `AddDiscoveryJob`, `AddScanJob`, `RemoveJob`, `EnableJob`, and `DisableJob` operations.

### `internal/scanning/`

Core scan execution package. Wraps nmap via the `Ullaakut/nmap` library:

- `ScanConfig` – targets (IPs, CIDRs, hostnames), ports, scan type, timeout, concurrency.
- `RunScanWithContext` – executes a scan with a context, returns `ScanResult`.
- `RunScanWithDB` – executes a scan and persists results to the database.
- `ParseNmapXML` – parses raw nmap XML output into structured Go types.

Supported scan types: `connect`, `syn`, `version`, `aggressive`, `stealth`, `comprehensive`.

> **Note:** SYN scans (`syn`, `stealth`, `aggressive`) require the nmap binary to be run with root privileges or Linux `CAP_NET_RAW`/`CAP_NET_ADMIN` capabilities.

### `internal/discovery/`

Network discovery engine. Also wraps nmap (ping/host-discovery mode, `-sn`):

- Accepts a CIDR, validates its size (maximum `/16`), generates individual target IPs.
- Selects nmap options based on discovery method: `tcp` (SYN ping), `ping` (ICMP echo), `arp`.
- Runs discovery in a background goroutine; creates and updates a `discovery_jobs` database record.
- Saves discovered hosts to the `hosts` table (insert or update with `last_seen`/`discovery_count`).

### `internal/services/`

Thin service layer between handlers and the database. Currently contains `NetworkService` (`networks.go`), which provides higher-level network management operations used by the network handler.

### `internal/auth/`

API key management:

- `apikey.go` – generates `sk_<random>` keys using `crypto/rand`, hashes with bcrypt (cost 12), validates with `bcrypt.CompareHashAndPassword`. Keys longer than 72 bytes are pre-hashed with SHA-256 before bcrypt to avoid truncation.
- `roles.go` – role-based access control types (RBAC groundwork).
- `db_operations.go` – database operations for persisting and querying API keys.

### `internal/config/`

Configuration management:

- Loads YAML or JSON config files with security checks (max 10 MB, restrictive file permissions).
- Falls back to `Default()` values when no file is provided.
- Overlays environment variables (all prefixed `SCANORAMA_`) onto defaults before file parsing.
- Validates the final configuration before returning it to the caller.

### `internal/workers/`

Generic worker pool for concurrent job execution:

- Configurable pool size, queue size, retry count, retry delay, shutdown timeout, and rate limit.
- Implements `Job` interface (`Execute`, `ID`, `Type`).
- Pre-built job types: `ScanJob`, `DiscoveryJob`.
- Emits structured log messages and Prometheus counters/histograms for every job.

### `internal/logging/`

Structured logging built on `log/slog`. Supports `text` and `json` formats, configurable output (stdout, stderr, or file path), log levels, optional source location, and log rotation (`RotationConfig`).

### `internal/metrics/`

In-process metrics registry plus Prometheus integration. The `Registry` type collects counters, gauges, and histograms. `PrometheusMetrics` registers the same data with the `prometheus/client_golang` library and serves it on `/metrics`.

### `internal/profiles/`

Scan profile management. Profiles define reusable sets of scan parameters (scan type, ports, timing, OS/service detection flags) that can be associated with scheduled scan jobs.

### `internal/errors/`

Typed error hierarchy. `DatabaseError`, `ScanError`, and other domain errors carry an `ErrorCode`, a user-safe message, and an internal `Cause`. Handlers use these to return appropriate HTTP status codes without leaking internal details.

---

## How the Daemon, API Server, and CLI Relate

```
scanorama daemon start
       │
       ▼
  daemon.New(cfg).Start()
       │
       ├─ db.ConnectAndMigrate()   ← runs pending SQL migrations
       ├─ api.New(cfg, db)         ← creates HTTP server + routes
       └─ d.run()
              │
              ├─ go apiServer.Start(ctx)   ← listens on API_HOST:API_PORT
              └─ health-check loop (10 s)

scanorama server start             ← alternative: just the API server
scanorama scan run --target ...    ← direct DB access, no daemon required
scanorama daemon stop              ← sends SIGTERM to daemon PID
```

The CLI's `daemon` and `server` subcommands manage long-running processes via PID files. Subcommands such as `scan`, `hosts`, and `networks` can work either directly against the database or through the API, depending on how they are implemented.

---

## External Dependencies

| Dependency | Role | Notes |
|------------|------|-------|
| **nmap** | Port scanning and host discovery | Must be installed separately; SYN scans require root or Linux capabilities |
| **PostgreSQL** | Primary data store | All scan results, hosts, jobs, and configuration are stored here |
| **Prometheus** | Metrics scraping | The daemon exposes `/metrics` in Prometheus exposition format; Prometheus is not bundled |

### Key Go Library Dependencies

| Library | Purpose |
|---------|---------|
| `gorilla/mux` | HTTP router |
| `gorilla/websocket` | WebSocket support |
| `gorilla/handlers` | CORS and other HTTP middleware |
| `Ullaakut/nmap/v3` | Go wrapper for the nmap binary |
| `jmoiron/sqlx` | Extended `database/sql` helpers |
| `lib/pq` | PostgreSQL driver |
| `robfig/cron/v3` | Cron expression scheduling |
| `prometheus/client_golang` | Prometheus metrics |
| `spf13/cobra` + `spf13/viper` | CLI framework and configuration |
| `google/uuid` | UUID generation for job and resource IDs |
| `golang.org/x/crypto` | bcrypt for API key hashing |
| `swaggo/http-swagger` | Embedded Swagger UI |

---

## Configuration Overview

All configuration is in a single YAML (or JSON) file, divided into six sections:

| Section | Key Settings |
|---------|-------------|
| `daemon` | PID file, working directory, user/group for privilege drop, daemonize flag, shutdown timeout |
| `database` | Host, port, database name, username, password, SSL mode, connection pool limits |
| `scanning` | Worker pool size, default interval, max scan timeout, default ports/scan type, concurrency, retry, rate limit |
| `api` | Enabled flag, host/port, TLS, auth/API keys, CORS, rate limiting, timeouts, max request size |
| `discovery` | Named networks (CIDR, method, schedule, exclusions), global exclusions, defaults, auto-seed |
| `logging` | Level, format (text/json), output, rotation, structured mode, request logging |

Environment variables prefixed with `SCANORAMA_` override defaults for all settings. Database credentials specifically use `SCANORAMA_DB_HOST`, `SCANORAMA_DB_PORT`, `SCANORAMA_DB_NAME`, `SCANORAMA_DB_USER`, `SCANORAMA_DB_PASSWORD`, and `SCANORAMA_DB_SSLMODE`.

---

## Related Documentation

- [`data-flow.md`](./data-flow.md) – request lifecycle and data flow diagrams
- [`logging.md`](./logging.md) – logging, metrics, and worker pool details
- [`../../DEPLOYMENT.md`](../../DEPLOYMENT.md) – deployment guide, environment variables, security
- [`../../api/`](../../api/) – REST API reference