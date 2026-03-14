# Scanorama Configuration Reference

This document is the authoritative reference for every configuration setting in Scanorama. For deployment-specific guidance (building, running, systemd, database setup), see [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Table of Contents

- [Overview](#overview)
- [Configuration Precedence](#configuration-precedence)
- [Environment Variables](#environment-variables)
- [Full Configuration Reference](#full-configuration-reference)
  - [Daemon](#daemon)
  - [Database](#database)
  - [Scanning](#scanning)
  - [API](#api)
  - [Discovery](#discovery)
  - [Logging](#logging)
- [Validation Rules](#validation-rules)
- [Hot-Reloadable Settings](#hot-reloadable-settings)
- [Example Configurations](#example-configurations)
  - [Minimal](#minimal-configuration)
  - [Production](#production-configuration)
  - [Development](#development-configuration)

---

## Overview

Scanorama uses a layered configuration system. Settings are defined as Go structs in `internal/config/config.go` and loaded through multiple sources that are merged in a well-defined order. Configuration files are parsed as YAML (`.yaml`, `.yml`) or JSON (`.json`).

The configuration is loaded at startup via `config.Load(path)`, which:

1. Initialises hardcoded defaults (including environment variable overrides).
2. Reads and merges the config file on top.
3. Validates the final merged configuration.

The config file path defaults to `./config.yaml` and can be overridden with the `--config` CLI flag.

### File Security

Scanorama enforces strict file security on the configuration file:

- **Maximum file size:** 10 MB
- **Maximum content size:** 5 MB
- **Permissions:** Must not be world-readable (`0o044`) or group-writable (`0o020`). A file with permissions broader than `0600` for sensitive configs will be rejected. Use `chmod 600 config.yaml` or `chmod 640 config.yaml` (with the owning group set appropriately).
- **Path validation:** Directory traversal (`..`), null bytes, and unsupported file extensions are rejected.
- **Binary detection:** Files that appear to contain binary data are rejected.

---

## Configuration Precedence

Settings are resolved in the following order, from **lowest** to **highest** priority:

| Priority | Source | Description |
|----------|--------|-------------|
| 1 (lowest) | **Hardcoded defaults** | Values returned by `config.Default()` in `internal/config/config.go`. |
| 2 | **Environment variables** | `SCANORAMA_*` variables read during `config.Default()` construction. These are baked into the default struct *before* the config file is parsed. |
| 3 | **Config file** | YAML or JSON file specified via `--config` flag (default: `./config.yaml`). Values in the file overwrite the defaults+env layer. |
| 4 | **CLI flags** | Command-line flags such as `--host`, `--port`, `--pid-file` override the corresponding config values. |
| 5 (highest) | **Admin API endpoint** | A subset of settings can be changed at runtime via `PUT /api/v1/admin/config`. These changes apply immediately but are **not** persisted to disk. |

> **Note:** Environment variables are evaluated once at startup. Changing an environment variable after the process has started has no effect unless the daemon is restarted.

---

## Environment Variables

All environment variables use the `SCANORAMA_` prefix. They are read during construction of the default configuration and serve as the baseline before the config file is applied.

### Database

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SCANORAMA_DB_HOST` | string | `localhost` | PostgreSQL hostname or IP address |
| `SCANORAMA_DB_PORT` | int | `5432` | PostgreSQL port |
| `SCANORAMA_DB_NAME` | string | *(empty)* | Database name (**required** — validation fails if empty) |
| `SCANORAMA_DB_USER` | string | *(empty)* | Database username (**required**) |
| `SCANORAMA_DB_PASSWORD` | string | *(empty)* | Database password |
| `SCANORAMA_DB_SSLMODE` | string | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `SCANORAMA_DB_MAX_OPEN_CONNS` | int | `25` | Maximum number of open connections in the pool |
| `SCANORAMA_DB_MAX_IDLE_CONNS` | int | `5` | Maximum number of idle connections in the pool |
| `SCANORAMA_DB_CONN_MAX_LIFETIME` | duration | `5m` | Maximum lifetime of a connection (Go duration string) |
| `SCANORAMA_DB_CONN_MAX_IDLE_TIME` | duration | `5m` | Maximum time a connection can sit idle (Go duration string) |

### Daemon

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SCANORAMA_PID_FILE` | string | `/var/run/scanorama.pid` | Path to the PID file |
| `SCANORAMA_WORK_DIR` | string | `/var/lib/scanorama` | Working directory for the daemon process |
| `SCANORAMA_USER` | string | *(empty)* | User to drop privileges to after startup |
| `SCANORAMA_GROUP` | string | *(empty)* | Group to drop privileges to after startup |

> **Tip:** For secrets like `SCANORAMA_DB_PASSWORD`, use a root-owned `EnvironmentFile` in your systemd unit rather than putting the password in the YAML config. See the [systemd section in DEPLOYMENT.md](DEPLOYMENT.md#systemd-service-recommended-for-production).

---

## Full Configuration Reference

### Daemon

Top-level key: `daemon`

Controls process lifecycle settings: PID file, working directory, privilege dropping, and shutdown behaviour.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| PID File | `pid_file` | string | `/var/run/scanorama.pid` | Path to the PID file written on startup. Env: `SCANORAMA_PID_FILE`. |
| Working Directory | `work_dir` | string | `/var/lib/scanorama` | Directory the daemon `chdir`s into. Env: `SCANORAMA_WORK_DIR`. |
| User | `user` | string | *(empty)* | Drop privileges to this OS user after startup. Env: `SCANORAMA_USER`. |
| Group | `group` | string | *(empty)* | Drop privileges to this OS group after startup. Env: `SCANORAMA_GROUP`. |
| Daemonize | `daemonize` | bool | `false` | Fork to background (detach from terminal). |
| Shutdown Timeout | `shutdown_timeout` | duration | `30s` | Graceful shutdown window. After this duration, in-flight work is abandoned. |

```yaml
daemon:
  pid_file: /var/run/scanorama/scanorama.pid
  work_dir: /var/lib/scanorama
  user: scanorama
  group: scanorama
  daemonize: false
  shutdown_timeout: 30s
```

---

### Database

Top-level key: `database`

PostgreSQL connection and pool settings. The struct is defined in `internal/db/database.go` as `db.Config`.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Host | `host` | string | `localhost` | PostgreSQL server hostname or IP. |
| Port | `port` | int | `5432` | PostgreSQL server port. |
| Database | `database` | string | *(empty)* | Database name. **Required.** |
| Username | `username` | string | *(empty)* | Connection username. **Required.** |
| Password | `password` | string | *(empty)* | Connection password. Consider using env var `SCANORAMA_DB_PASSWORD` instead. |
| SSL Mode | `ssl_mode` | string | `disable` | PostgreSQL SSL mode: `disable`, `require`, `verify-ca`, `verify-full`. |
| Max Open Conns | `max_open_conns` | int | `25` | Maximum open connections in the pool. |
| Max Idle Conns | `max_idle_conns` | int | `5` | Maximum idle connections retained in the pool. |
| Conn Max Lifetime | `conn_max_lifetime` | duration | `5m` | Maximum total lifetime of a connection before it is closed and replaced. |
| Conn Max Idle Time | `conn_max_idle_time` | duration | `5m` | Maximum time a connection can remain idle before being closed. |

```yaml
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: changeme
  ssl_mode: require
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 5m
```

> **Security:** Use `ssl_mode: require` (or stricter) in production. See [DEPLOYMENT.md — Database Credentials](DEPLOYMENT.md#database-credentials) for best practices.

---

### Scanning

Top-level key: `scanning`

Controls the scan execution engine: worker pool, timeouts, default scan parameters, retry logic, and rate limiting.

#### Core Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Worker Pool Size | `worker_pool_size` | int | `10` | Number of concurrent scanning worker goroutines. |
| Default Interval | `default_interval` | duration | `1h` | Default re-scan interval for targets without an explicit schedule. |
| Max Scan Timeout | `max_scan_timeout` | duration | `10m` | Maximum allowed timeout for a single scan operation. |
| Default Ports | `default_ports` | string | `22,80,443,8080,8443` | Comma-separated list of ports to scan when no ports are specified. |
| Default Scan Type | `default_scan_type` | string | `connect` | Default nmap scan type. Valid values: `connect`, `syn`, `version`. |
| Max Concurrent Targets | `max_concurrent_targets` | int | `100` | Maximum number of targets scanned concurrently within a single job. |
| Enable Service Detection | `enable_service_detection` | bool | `true` | Run nmap service/version detection probes on open ports. |
| Enable OS Detection | `enable_os_detection` | bool | `false` | Run nmap OS fingerprinting (requires elevated privileges). |

#### Retry Settings

Nested under `scanning.retry`. Controls automatic retry of failed scan operations with exponential backoff.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Max Retries | `max_retries` | int | `3` | Maximum number of retry attempts for a failed scan. |
| Retry Delay | `retry_delay` | duration | `30s` | Base delay between retries. |
| Backoff Multiplier | `backoff_multiplier` | float | `2.0` | Multiplier applied to the delay after each successive retry. |

#### Rate Limit Settings

Nested under `scanning.rate_limit`. Controls the rate at which scan requests are dispatched to nmap.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Enabled | `enabled` | bool | `true` | Enable scan rate limiting. |
| Requests Per Second | `requests_per_second` | int | `100` | Maximum scan operations initiated per second. |
| Burst Size | `burst_size` | int | `200` | Maximum burst above the steady-state rate. |

```yaml
scanning:
  worker_pool_size: 10
  default_interval: 1h
  max_scan_timeout: 10m
  default_ports: "22,80,443,8080,8443"
  default_scan_type: connect
  max_concurrent_targets: 100
  enable_service_detection: true
  enable_os_detection: false

  retry:
    max_retries: 3
    retry_delay: 30s
    backoff_multiplier: 2.0

  rate_limit:
    enabled: true
    requests_per_second: 100
    burst_size: 200
```

---

### API

Top-level key: `api`

REST API server settings including bind address, timeouts, TLS, authentication, CORS, and rate limiting.

#### Server Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Enabled | `enabled` | bool | `true` | Enable the REST API server. |
| Host | `host` | string | `127.0.0.1` | Bind address. Use `0.0.0.0` to listen on all interfaces. |
| Port | `port` | int | `8080` | TCP port to listen on. Must be 1–65535. |
| Read Timeout | `read_timeout` | duration | `10s` | Maximum duration for reading the entire request (headers + body). |
| Write Timeout | `write_timeout` | duration | `10s` | Maximum duration before timing out writes of the response. |
| Idle Timeout | `idle_timeout` | duration | `60s` | Maximum time to wait for the next request on a keep-alive connection. |
| Max Header Bytes | `max_header_bytes` | int | `1048576` (1 MB) | Maximum size of request headers in bytes. |
| Request Timeout | `request_timeout` | duration | `30s` | Per-request context timeout applied by middleware. |
| Max Request Size | `max_request_size` | int | `1048576` (1 MB) | Maximum request body size in bytes. |

#### TLS Settings

Nested under `api.tls`.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Enabled | `enabled` | bool | `false` | Enable TLS termination in the API server. |
| Certificate File | `cert_file` | string | *(empty)* | Path to the PEM-encoded server certificate. **Required** when TLS is enabled. |
| Key File | `key_file` | string | *(empty)* | Path to the PEM-encoded private key. **Required** when TLS is enabled. |
| CA File | `ca_file` | string | *(empty)* | Path to a CA certificate for mutual TLS client authentication (optional). |

#### Authentication Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Auth Enabled | `auth_enabled` | bool | `false` | Require API key authentication on all endpoints. |
| API Keys | `api_keys` | []string | `[]` | Static API keys loaded from config. Clients send keys via `X-API-Key` header. At least one key is **required** when `auth_enabled` is `true`. |

#### CORS Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Enable CORS | `enable_cors` | bool | `true` | Enable Cross-Origin Resource Sharing headers. |
| CORS Origins | `cors_origins` | []string | `["*"]` | Allowed origins. Restrict in production. |

#### Rate Limiting Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Rate Limit Enabled | `rate_limit_enabled` | bool | `true` | Enable per-IP API rate limiting. |
| Rate Limit Requests | `rate_limit_requests` | int | `100` | Maximum requests allowed within the window. |
| Rate Limit Window | `rate_limit_window` | duration | `1m` | Time window for the request counter. |

```yaml
api:
  enabled: true
  host: 127.0.0.1
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 60s
  max_header_bytes: 1048576
  request_timeout: 30s
  max_request_size: 1048576

  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""

  auth_enabled: false
  api_keys: []

  enable_cors: true
  cors_origins:
    - "*"

  rate_limit_enabled: true
  rate_limit_requests: 100
  rate_limit_window: 1m
```

---

### Discovery

Top-level key: `discovery`

Network discovery engine configuration: which networks to scan, exclusion lists, default discovery parameters, and auto-seeding.

#### Top-Level Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Networks | `networks` | []NetworkConfig | `[]` | List of predefined networks for scheduled discovery. See [Network Entry](#network-entry) below. |
| Global Exclusions | `global_exclusions` | []string | `[]` | IP addresses excluded from **all** discovery runs (e.g., gateways, critical infrastructure). |
| Auto Seed | `auto_seed` | bool | `true` | Seed the `networks` database table from this config on startup. |

#### Default Discovery Settings

Nested under `discovery.defaults`. Applied to any network that does not specify its own value.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Method | `method` | string | `ping` | Discovery method: `ping`, `tcp`, `arp`. |
| Timeout | `timeout` | string | `30s` | Timeout per discovery operation (as a string parsed by the discovery engine). |
| Schedule | `schedule` | string | `0 */12 * * *` | Cron expression for automatic discovery (default: twice daily). |
| Ports | `ports` | string | `22,80,443,8080,8443,3389,5432,6379` | Ports used for TCP-based discovery. |

#### Network Entry

Each entry under `discovery.networks` defines a network to discover.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Name | `name` | string | — | Unique name for this network. **Required.** |
| CIDR | `cidr` | string | — | Network in CIDR notation (e.g., `192.168.1.0/24`). **Required.** |
| Method | `method` | string | *(inherits default)* | Discovery method override for this network. |
| Schedule | `schedule` | string | *(inherits default)* | Cron schedule override. |
| Description | `description` | string | *(empty)* | Human-readable description. |
| Exclusions | `exclusions` | []string | `[]` | Network-specific IP exclusions (merged with global exclusions). |
| Enabled | `enabled` | bool | — | Enable or disable this network for discovery. |
| Ports | `ports` | string | *(inherits default)* | Ports override for TCP-based discovery. |

```yaml
discovery:
  auto_seed: true
  global_exclusions:
    - "10.0.0.1"

  defaults:
    method: ping
    timeout: "30s"
    schedule: "0 */12 * * *"
    ports: "22,80,443,8080,8443,3389,5432,6379"

  networks:
    - name: office-lan
      cidr: 192.168.1.0/24
      method: ping
      schedule: "0 */6 * * *"
      description: "Office LAN"
      enabled: true
      exclusions:
        - "192.168.1.1"

    - name: server-vlan
      cidr: 10.10.0.0/24
      method: tcp
      schedule: "0 */4 * * *"
      description: "Server VLAN"
      enabled: true
      ports: "22,80,443,3306,5432,6379,8080"
```

---

### Logging

Top-level key: `logging`

Controls log level, format, output destination, rotation, and request logging.

#### Core Settings

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Level | `level` | string | `info` | Minimum log level: `debug`, `info`, `warn`, `error`. |
| Format | `format` | string | `text` | Log output format: `text` (human-readable) or `json` (structured). |
| Output | `output` | string | `stdout` | Log destination: `stdout`, `stderr`, or an absolute file path. |
| Structured | `structured` | bool | `false` | Enable structured key-value logging. |
| Request Logging | `request_logging` | bool | `true` | Log every HTTP request (method, path, status, duration). |

#### Rotation Settings

Nested under `logging.rotation`. Only applicable when `output` is a file path.

| Setting | YAML Key | Type | Default | Description |
|---------|----------|------|---------|-------------|
| Enabled | `enabled` | bool | `false` | Enable automatic log file rotation. |
| Max Size MB | `max_size_mb` | int | `100` | Maximum log file size in megabytes before rotation. |
| Max Backups | `max_backups` | int | `5` | Number of rotated log files to retain. |
| Max Age Days | `max_age_days` | int | `30` | Maximum age (in days) of rotated log files before deletion. |
| Compress | `compress` | bool | `true` | Compress rotated log files with gzip. |

```yaml
logging:
  level: info
  format: text
  output: stdout
  structured: false
  request_logging: true

  rotation:
    enabled: false
    max_size_mb: 100
    max_backups: 5
    max_age_days: 30
    compress: true
```

---

## Validation Rules

Configuration is validated by `Config.Validate()` after loading. If any rule fails, startup is aborted with a descriptive error message.

### Database Validation (`validateDatabase`)

| Rule | Error |
|------|-------|
| `host` must not be empty | `database host is required (set SCANORAMA_DB_HOST or configure in file)` |
| `database` must not be empty | `database name is required (set SCANORAMA_DB_NAME or configure in file)` |
| `username` must not be empty | `database username is required (set SCANORAMA_DB_USER or configure in file)` |

### Scanning Validation (`validateScanning`)

| Rule | Error |
|------|-------|
| `worker_pool_size` must be > 0 | `worker pool size must be positive` |
| `max_concurrent_targets` must be > 0 | `max concurrent targets must be positive` |
| `default_interval` must be > 0 | `default scan interval must be positive` |
| `default_scan_type` must be one of `connect`, `syn`, `version` | `invalid default scan type: <value>` |

### API Validation (`validateAPI`)

Skipped entirely when `api.enabled` is `false`.

| Rule | Error |
|------|-------|
| `port` must be 1–65535 | `API port must be between 1 and 65535` |
| `host` must not be empty | `API host address is required when API is enabled` |
| `read_timeout` must be > 0 | `API read timeout must be positive` |
| `write_timeout` must be > 0 | `API write timeout must be positive` |
| `idle_timeout` must be > 0 | `API idle timeout must be positive` |
| `max_header_bytes` must be > 0 | `API max header bytes must be positive` |
| When `auth_enabled` is true, `api_keys` must have ≥ 1 entry | `at least one API key must be provided when authentication is enabled` |

### API Rate Limiting Validation (`validateAPIRateLimiting`)

Skipped when `rate_limit_enabled` is `false`.

| Rule | Error |
|------|-------|
| `rate_limit_requests` must be > 0 | `rate limit requests must be positive when rate limiting is enabled` |
| `rate_limit_window` must be > 0 | `rate limit window must be positive when rate limiting is enabled` |

### TLS Validation (`validateTLS`)

Skipped when `api.tls.enabled` is `false`.

| Rule | Error |
|------|-------|
| `cert_file` must not be empty | `TLS certificate file is required when TLS is enabled` |
| `key_file` must not be empty | `TLS key file is required when TLS is enabled` |

### Logging Validation (`validateLogging`)

| Rule | Error |
|------|-------|
| `level` must be one of `debug`, `info`, `warn`, `error` | `invalid log level: <value>` |
| `format` must be one of `text`, `json` | `invalid log format: <value>` |

---

## Hot-Reloadable Settings

A subset of configuration can be updated at runtime via the admin API endpoint without restarting the daemon:

```
PUT /api/v1/admin/config
Content-Type: application/json

{
  "section": "<section>",
  "config": { ... }
}
```

```
GET /api/v1/admin/config[?section=<section>]
```

The following sections and fields are available for hot-reload:

### API Section (`"section": "api"`)

| Field | Type | Constraints |
|-------|------|-------------|
| `enabled` | bool | — |
| `host` | string | — |
| `port` | int | 1–65535 |
| `read_timeout` | string (duration) | — |
| `write_timeout` | string (duration) | — |
| `idle_timeout` | string (duration) | — |
| `max_header_bytes` | int | 1024–1048576 |
| `enable_cors` | bool | — |
| `cors_origins` | []string | Individual origins max 255 chars |
| `auth_enabled` | bool | — |
| `rate_limit_enabled` | bool | — |
| `rate_limit_requests` | int | — |
| `rate_limit_window` | string (duration) | — |

### Database Section (`"section": "database"`)

| Field | Type | Constraints |
|-------|------|-------------|
| `host` | string | — |
| `port` | int | 1–65535 |
| `database` | string | 1–63 chars |
| `username` | string | 1–63 chars |
| `ssl_mode` | string | `disable`, `require`, `verify-ca`, `verify-full` |
| `max_open_conns` | int | 1–100 |
| `max_idle_conns` | int | 1–100 |
| `conn_max_lifetime` | string (duration) | — |
| `conn_max_idle_time` | string (duration) | — |

> **Note:** Changing database connection settings at runtime will affect new connections only. Existing pooled connections are not immediately closed.

### Scanning Section (`"section": "scanning"`)

| Field | Type | Constraints |
|-------|------|-------------|
| `worker_pool_size` | int | 1–1000 |
| `default_interval` | string (duration) | — |
| `max_scan_timeout` | string (duration) | — |
| `default_ports` | string | max 1000 chars |
| `default_scan_type` | string | `connect`, `syn`, `ack`, `window`, `fin`, `null`, `xmas`, `maimon` |
| `max_concurrent_targets` | int | 1–10000 |
| `enable_service_detection` | bool | — |
| `enable_os_detection` | bool | — |

### Logging Section (`"section": "logging"`)

| Field | Type | Constraints |
|-------|------|-------------|
| `level` | string | `debug`, `info`, `warn`, `error` |
| `format` | string | `text`, `json` |
| `output` | string | 1–255 chars |
| `structured` | bool | — |
| `request_logging` | bool | — |

### Daemon Section (`"section": "daemon"`)

| Field | Type | Constraints |
|-------|------|-------------|
| `pid_file` | string | 1–255 chars |
| `work_dir` | string | 1–255 chars |
| `user` | string | 1–32 chars |
| `group` | string | 1–32 chars |
| `daemonize` | bool | — |
| `shutdown_timeout` | string (duration) | — |

> **Important:** Hot-reloaded changes are held in memory only. They are **not** written back to the config file. A daemon restart will revert to the values in the config file (plus env vars). To make changes permanent, update the config file.

> **Security:** The admin config endpoint should be protected with authentication (`api.auth_enabled: true`) in production. Sensitive fields like `database.password` are not exposed in `GET` responses — they are redacted to `[REDACTED]`.

---

## Example Configurations

### Minimal Configuration

The bare minimum to get Scanorama running. Everything else uses defaults.

```yaml
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: changeme

logging:
  level: info
```

### Production Configuration

A hardened configuration suitable for production deployments.

```yaml
daemon:
  pid_file: /var/run/scanorama/scanorama.pid
  work_dir: /var/lib/scanorama
  user: scanorama
  group: scanorama
  daemonize: false          # Let systemd manage the process
  shutdown_timeout: 60s

database:
  host: db.internal.example.com
  port: 5432
  database: scanorama
  username: scanorama
  # password: set via SCANORAMA_DB_PASSWORD env var
  ssl_mode: verify-full
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 5m

scanning:
  worker_pool_size: 10
  default_interval: 1h
  max_scan_timeout: 10m
  default_ports: "22,80,443,8080,8443"
  default_scan_type: connect
  max_concurrent_targets: 50
  enable_service_detection: true
  enable_os_detection: false

  retry:
    max_retries: 3
    retry_delay: 30s
    backoff_multiplier: 2.0

  rate_limit:
    enabled: true
    requests_per_second: 50
    burst_size: 100

api:
  enabled: true
  host: 127.0.0.1           # Bind to localhost; reverse proxy handles external traffic
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 60s
  max_header_bytes: 1048576
  request_timeout: 30s
  max_request_size: 1048576

  tls:
    enabled: false           # TLS terminated at reverse proxy

  auth_enabled: true
  api_keys:
    - "sk_prod_your_secret_key_here"

  enable_cors: true
  cors_origins:
    - "https://dashboard.example.com"

  rate_limit_enabled: true
  rate_limit_requests: 100
  rate_limit_window: 1m

discovery:
  auto_seed: true
  global_exclusions:
    - "10.0.0.1"             # Core router
    - "10.0.0.2"             # Firewall

  defaults:
    method: ping
    timeout: "30s"
    schedule: "0 */12 * * *"
    ports: "22,80,443,8080,8443,3389,5432,6379"

  networks:
    - name: office-lan
      cidr: 192.168.1.0/24
      method: ping
      schedule: "0 */6 * * *"
      description: "Office LAN"
      enabled: true
      exclusions:
        - "192.168.1.1"

    - name: server-vlan
      cidr: 10.10.0.0/24
      method: tcp
      schedule: "0 */4 * * *"
      description: "Server VLAN"
      enabled: true
      ports: "22,80,443,3306,5432,6379,8080,8443"

logging:
  level: info
  format: json               # Structured JSON for log aggregation
  output: /var/log/scanorama/scanorama.log
  structured: true
  request_logging: true

  rotation:
    enabled: true
    max_size_mb: 100
    max_backups: 5
    max_age_days: 30
    compress: true
```

### Development Configuration

A permissive configuration for local development and testing.

```yaml
daemon:
  pid_file: /tmp/scanorama.pid
  work_dir: /tmp/scanorama
  daemonize: false
  shutdown_timeout: 5s

database:
  host: localhost
  port: 5432
  database: scanorama_dev
  username: scanorama
  password: devpassword
  ssl_mode: disable
  max_open_conns: 10
  max_idle_conns: 2
  conn_max_lifetime: 5m
  conn_max_idle_time: 5m

scanning:
  worker_pool_size: 3
  default_interval: 5m
  max_scan_timeout: 2m
  default_ports: "22,80,443,8080"
  default_scan_type: connect
  max_concurrent_targets: 10
  enable_service_detection: true
  enable_os_detection: false

  retry:
    max_retries: 1
    retry_delay: 5s
    backoff_multiplier: 1.5

  rate_limit:
    enabled: false

api:
  enabled: true
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  max_header_bytes: 1048576
  request_timeout: 60s
  max_request_size: 1048576

  tls:
    enabled: false

  auth_enabled: false
  api_keys: []

  enable_cors: true
  cors_origins:
    - "*"

  rate_limit_enabled: false

discovery:
  auto_seed: true
  global_exclusions: []

  defaults:
    method: ping
    timeout: "10s"
    schedule: "0 */1 * * *"  # Every hour for faster feedback
    ports: "22,80,443,8080"

  networks:
    - name: local-net
      cidr: 192.168.1.0/24
      method: ping
      schedule: "*/30 * * * *"  # Every 30 minutes
      description: "Local development network"
      enabled: true

logging:
  level: debug
  format: text
  output: stdout
  structured: false
  request_logging: true

  rotation:
    enabled: false
```

---

## See Also

- [DEPLOYMENT.md](DEPLOYMENT.md) — Building, running, systemd setup, database migrations, CLI usage, security, and monitoring.
- [API Documentation](api/) — REST API endpoint reference.
- [Technical Documentation](technical/) — Architecture and internals.