# Scanorama Deployment Guide

This guide covers building Scanorama from source, configuring it for your environment, and running it in production.

---

## Prerequisites

### Required

| Dependency | Minimum Version | Notes |
|------------|-----------------|-------|
| **Go** | 1.22+ | Required to build from source |
| **nmap** | 7.80+ | Must be installed on the host running the daemon; required for all scans and discovery |
| **PostgreSQL** | 14+ | Primary data store; must be accessible from the host running the daemon |

### Optional

| Dependency | Purpose |
|------------|---------|
| **Prometheus** | Scraping the `/metrics` endpoint |
| **make** | Convenience build targets (not strictly required) |

### Installing nmap

```sh
# Debian / Ubuntu
sudo apt-get install nmap

# RHEL / CentOS / Fedora
sudo dnf install nmap

# macOS (Homebrew)
brew install nmap

# Verify
nmap --version
```

> **Important:** SYN scans (`syn`, `stealth`, `aggressive`, `comprehensive` scan types) require
> nmap to run as root or with `CAP_NET_RAW` and `CAP_NET_ADMIN` Linux capabilities.
> `connect`-type scans do not need elevated privileges.

---

## Building from Source

```sh
git clone https://github.com/anstrom/scanorama.git
cd scanorama

# Build the binary (version info injected at build time)
go build \
  -ldflags "-X main.version=$(git describe --tags --always) \
            -X main.commit=$(git rev-parse --short HEAD) \
            -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o scanorama \
  ./cmd/scanorama

# Verify
./scanorama --version
```

The resulting `scanorama` binary is self-contained (migrations are embedded). Copy it to any host that has nmap and PostgreSQL access.

---

## Configuration

Scanorama is configured via a YAML file (default: `./config.yaml`). Pass an alternate path with `--config /path/to/config.yaml`.

### Minimal Configuration

```yaml
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: changeme
  ssl_mode: require

api:
  enabled: true
  host: 127.0.0.1
  port: 8080

logging:
  level: info
  format: text
  output: stdout
```

### Full Configuration Reference

```yaml
# Daemon process settings
daemon:
  pid_file: /var/run/scanorama/scanorama.pid
  work_dir: /var/lib/scanorama
  user: scanorama       # drop privileges to this user after startup (optional)
  group: scanorama      # drop privileges to this group after startup (optional)
  daemonize: false      # fork to background (true = detach from terminal)
  shutdown_timeout: 30s # graceful shutdown window

# PostgreSQL connection
database:
  host: localhost
  port: 5432
  database: scanorama
  username: scanorama
  password: changeme
  ssl_mode: require     # disable | require | verify-ca | verify-full
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 5m

# Scan execution
scanning:
  worker_pool_size: 10
  default_interval: 1h
  max_scan_timeout: 10m
  default_ports: "22,80,443,8080,8443"
  default_scan_type: connect  # connect | syn | version | aggressive | stealth | comprehensive
  max_concurrent_targets: 100
  enable_service_detection: false
  enable_os_detection: false
  retry:
    max_retries: 3
    retry_delay: 30s
    backoff_multiplier: 2.0
  rate_limit:
    enabled: false
    requests_per_second: 100
    burst_size: 200

# REST API server
api:
  enabled: true
  host: 127.0.0.1    # bind address; use 0.0.0.0 to listen on all interfaces
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 60s
  max_header_bytes: 1048576  # 1 MB
  auth_enabled: false
  api_keys: []               # static API keys (plain text stored in config)
  enable_cors: true
  cors_origins:
    - "*"
  rate_limit_enabled: true
  rate_limit_requests: 100
  rate_limit_window: 1m
  request_timeout: 30s
  max_request_size: 1048576  # 1 MB
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""

# Network discovery
discovery:
  auto_seed: true            # seed networks table from config on startup
  global_exclusions:
    - "10.0.0.1"             # IPs excluded from all discovery runs
  defaults:
    method: ping             # ping | tcp | arp
    timeout: 30s
    schedule: "0 */12 * * *" # cron expression (twice daily)
    ports: "22,80,443,8080,8443,3389,5432,6379"
  networks:
    - name: office-lan
      cidr: 192.168.1.0/24
      method: ping
      schedule: "0 */6 * * *"
      description: "Office LAN"
      enabled: true
      exclusions:
        - "192.168.1.1"      # gateway — skip

# Logging
logging:
  level: info                # debug | info | warn | error
  format: text               # text | json
  output: stdout             # stdout | stderr | /path/to/file.log
  structured: false
  request_logging: true
  rotation:
    enabled: false
    max_size_mb: 100
    max_backups: 5
    max_age_days: 30
    compress: true
```

### File Permissions

The config file must not be world-readable. Scanorama rejects config files with permissions broader than `0640`:

```sh
chmod 640 config.yaml
```

---

## Environment Variables

All configuration values can be set or overridden using environment variables prefixed with `SCANORAMA_`. The environment variables are applied on top of built-in defaults before the config file is parsed.

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `SCANORAMA_DB_HOST` | `localhost` | PostgreSQL hostname or IP |
| `SCANORAMA_DB_PORT` | `5432` | PostgreSQL port |
| `SCANORAMA_DB_NAME` | _(none)_ | Database name |
| `SCANORAMA_DB_USER` | _(none)_ | Database username |
| `SCANORAMA_DB_PASSWORD` | _(none)_ | Database password |
| `SCANORAMA_DB_SSLMODE` | `disable` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |
| `SCANORAMA_DB_MAX_OPEN_CONNS` | `25` | Maximum open connections in pool |
| `SCANORAMA_DB_MAX_IDLE_CONNS` | `5` | Maximum idle connections in pool |
| `SCANORAMA_DB_CONN_MAX_LIFETIME` | `5m` | Maximum connection lifetime (Go duration) |
| `SCANORAMA_DB_CONN_MAX_IDLE_TIME` | `5m` | Maximum idle connection time (Go duration) |

### Daemon

| Variable | Default | Description |
|----------|---------|-------------|
| `SCANORAMA_PID_FILE` | `/var/run/scanorama.pid` | Path to PID file |
| `SCANORAMA_WORK_DIR` | `/var/lib/scanorama` | Working directory |
| `SCANORAMA_USER` | _(none)_ | User to drop privileges to |
| `SCANORAMA_GROUP` | _(none)_ | Group to drop privileges to |

> Environment variables take the form `SCANORAMA_<SECTION>_<KEY>` in general, but the database
> variables listed above use their own flat namespace. Refer to `internal/config/config.go` for
> the full mapping.

---

## Database Setup

### Create the Database and User

```sql
-- Run as a PostgreSQL superuser
CREATE USER scanorama WITH PASSWORD 'changeme';
CREATE DATABASE scanorama OWNER scanorama;
GRANT ALL PRIVILEGES ON DATABASE scanorama TO scanorama;
```

### Migrations

**Migrations run automatically at startup.** There is no separate migration step. When the daemon (or `server start`) connects to PostgreSQL, it runs all pending SQL migrations in order before accepting requests.

Migrations are embedded in the binary and tracked in the `schema_migrations` table:

| Migration | Contents |
|-----------|----------|
| `001_initial_schema` | Core tables: `hosts`, `scan_jobs`, `port_scans`, `services`, `discovery_jobs`, `scheduled_jobs`, `scan_profiles` |
| `002_performance_improvements` | Indexes and materialized views |
| `003_networks_table` | `networks` table |
| `004_network_exclusions` | `network_exclusions` table |
| `005_fix_scan_types` | Scan type constraint updates |
| `006_api_keys_table` | `api_keys` and `api_key_roles` tables |

If a migration has already been applied (identified by name and SHA-256 checksum), it is skipped. **Do not modify migration files after they have been applied** — the checksum check will detect the change and the startup will fail.

---

## Running Scanorama

### Foreground (Development / Testing)

Start just the API server in the foreground:

```sh
./scanorama server start --foreground --config config.yaml
```

Start the full daemon (API server + scheduler) in the foreground:

```sh
./scanorama daemon start --background=false --config config.yaml
```

### Background Daemon

```sh
# Start
./scanorama daemon start --config config.yaml

# Check status
./scanorama daemon status

# Stop (sends SIGTERM; waits up to 30 s before SIGKILL)
./scanorama daemon stop

# Restart
./scanorama daemon restart
```

The daemon writes a PID file (default `/tmp/scanorama.pid`, overridden by `daemon.pid_file` in config or `--pid-file` flag).

### API Server Only

Use `server` subcommands when you want only the REST API without the cron scheduler:

```sh
./scanorama server start [--foreground] [--host 0.0.0.0] [--port 8080]
./scanorama server stop
./scanorama server status
./scanorama server logs [--follow]
```

### systemd Service (Recommended for Production)

Create `/etc/systemd/system/scanorama.service`:

```ini
[Unit]
Description=Scanorama Network Scanner Daemon
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=scanorama
Group=scanorama
WorkingDirectory=/var/lib/scanorama
ExecStart=/usr/local/bin/scanorama daemon start --background=false --config /etc/scanorama/config.yaml
ExecStop=/usr/local/bin/scanorama daemon stop
Restart=on-failure
RestartSec=5s

# Environment (alternative to config file for secrets)
EnvironmentFile=-/etc/scanorama/env

# Allow nmap to use raw sockets for SYN scans (remove if using connect scans only)
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable scanorama
sudo systemctl start scanorama
sudo systemctl status scanorama
```

---

## Using the CLI

The `scanorama` binary doubles as a management CLI. All commands accept `--config` to point at a config file and `--verbose` / `-v` for extra output.

```sh
# Ad-hoc scan
./scanorama scan run --target 192.168.1.0/24 --ports 22,80,443 --type connect

# List hosts
./scanorama hosts list

# Network management
./scanorama networks list
./scanorama networks create --name office --cidr 192.168.1.0/24
./scanorama networks enable <id>

# Schedule management
./scanorama schedule list
./scanorama schedule create ...

# API key management
./scanorama apikeys create --name "ci-system"
./scanorama apikeys list
./scanorama apikeys revoke <id>

# Ad-hoc discovery
./scanorama discover --network 192.168.1.0/24 --method ping
```

---

## Security Considerations

### nmap Privileges

nmap requires root or Linux capabilities for raw-socket scan types:

| Scan type | Requires privilege? |
|-----------|-------------------|
| `connect` | No |
| `syn` | Yes (root or `CAP_NET_RAW`/`CAP_NET_ADMIN`) |
| `stealth` | Yes |
| `aggressive` | Yes |
| `version` | No |
| `comprehensive` | Yes |

**Recommended approach:** Run the daemon as a dedicated non-root user (`scanorama`) and grant the binary Linux capabilities:

```sh
sudo setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/scanorama
```

Or use `AmbientCapabilities` in the systemd unit as shown above. Do not run the daemon as root in production.

### API Authentication

By default, `api.auth_enabled` is `false` and the API is unauthenticated. Enable authentication before exposing the API outside localhost:

```yaml
api:
  auth_enabled: true
  api_keys:
    - "sk_yourapikey"
```

API keys can also be managed at runtime via the CLI and are stored in the database as bcrypt hashes. Clients must send the key in the `X-API-Key` request header.

### TLS

For production deployments exposed to untrusted networks, enable TLS:

```yaml
api:
  tls:
    enabled: true
    cert_file: /etc/scanorama/tls/server.crt
    key_file: /etc/scanorama/tls/server.key
```

Alternatively, terminate TLS at a reverse proxy (nginx, Caddy, etc.) and keep Scanorama bound to `127.0.0.1`.

### Database Credentials

Never store database passwords in world-readable files. Use:

- Config file with `chmod 640` owned by the `scanorama` user, or
- Environment variables via a root-owned `EnvironmentFile` in the systemd unit.

### Rate Limiting

The API has built-in per-IP rate limiting (default: 100 requests/minute). Adjust in config:

```yaml
api:
  rate_limit_enabled: true
  rate_limit_requests: 100
  rate_limit_window: 1m
```

### CORS

The default CORS policy allows all origins (`*`). Restrict this in production:

```yaml
api:
  enable_cors: true
  cors_origins:
    - "https://your-frontend.example.com"
```

---

## Monitoring

### Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/liveness` | Liveness probe — returns 200 if the process is alive |
| `GET /api/v1/health` | Readiness probe — checks database connectivity |
| `GET /api/v1/status` | Extended status including uptime, version, component state |
| `GET /api/v1/version` | Build version information |

### Prometheus Metrics

Scanorama exposes Prometheus metrics at `GET /metrics` (also available at `GET /api/v1/metrics`). Add a scrape target to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: scanorama
    static_configs:
      - targets: ["localhost:8080"]
    metrics_path: /metrics
```

Key metric families include scan durations, job counts, worker pool utilisation, and database query counts.

### Logging

Set `logging.level: debug` to see detailed per-request and per-scan traces. For production, `info` is recommended. Use `logging.format: json` and a log aggregator (Loki, Elasticsearch, etc.) for structured log ingestion.

---

## Troubleshooting

### Daemon fails to start: "database connection failed"

- Verify PostgreSQL is running and reachable from the daemon host.
- Check `SCANORAMA_DB_*` environment variables or `database:` section in config.
- Confirm the database user has `CONNECT` and `CREATE TABLE` privileges (needed for migrations).
- Try connecting manually: `psql -h <host> -U <user> -d <database>`.

### Daemon fails to start: "migration failed"

- A migration SQL file may have been modified after it was applied. The SHA-256 checksum no longer matches. Restore the original file or reset the database.
- Check that the database user has `CREATE TABLE`, `CREATE INDEX`, and `CREATE VIEW` privileges.

### API returns 401 Unauthorized

- `api.auth_enabled` is `true`. Supply the key via `X-API-Key: <key>` header.
- Verify the key is listed in `api.api_keys` in the config, or was created via `scanorama apikeys create` and is active in the database.

### nmap scans fail with "permission denied"

- The scan type requires raw socket access. Either:
  - Switch to `default_scan_type: connect` in config, or
  - Grant capabilities: `sudo setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/scanorama`

### nmap not found

- Ensure nmap is installed and on `$PATH` for the user running the daemon.
- Verify with: `which nmap && nmap --version`

### Config file rejected: "insecure config file permissions"

- The config file is too permissive. Fix with: `chmod 640 config.yaml`

### High memory usage

- Reduce `scanning.worker_pool_size` and `scanning.max_concurrent_targets`.
- For large networks, lower `discovery.defaults.timeout` and reduce the number of concurrent scheduled jobs.

### Daemon does not stop gracefully

The `daemon stop` command sends `SIGTERM` and waits up to 30 seconds. If the daemon does not exit in time, it sends `SIGKILL`. If you need a longer shutdown window, increase `daemon.shutdown_timeout` in config.

---

## Directory Layout (Recommended Production)

```
/etc/scanorama/
  config.yaml          # main configuration (chmod 640, owned by scanorama)
  env                  # optional EnvironmentFile for secrets (chmod 600, owned by root)
  tls/
    server.crt
    server.key

/var/lib/scanorama/    # working directory (daemon.work_dir)
/var/run/scanorama/    # PID file directory
/var/log/scanorama/    # log files (if logging.output points here)

/usr/local/bin/scanorama   # binary
```
```

Now let me update the `docs/README.md` to reference the new files: