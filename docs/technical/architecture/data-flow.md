# Scanorama Data Flow

This document describes how data moves through Scanorama for each major operation: HTTP request handling, scan execution, network discovery, and WebSocket updates.

---

## 1. HTTP Request Lifecycle

Every inbound HTTP request passes through the same middleware pipeline before reaching a handler.

```
Client
  │
  │  HTTP request
  ▼
gorilla/mux router
  │
  ├─ Recovery middleware       ← catches panics, returns HTTP 500
  ├─ Logging middleware        ← assigns request ID, logs method/path/status/duration
  ├─ CORS middleware           ← sets Access-Control-* headers (if enabled)
  ├─ Authentication middleware ← validates X-API-Key header (if auth enabled)
  └─ ContentType middleware    ← requires application/json on POST/PUT
  │
  ▼
Handler function (e.g., ScanHandler.CreateScan)
  │
  ├─ Parse and validate request body / path params
  ├─ Call db.* or service layer
  └─ writeJSON(w, statusCode, responseBody)
  │
  ▼
Client receives JSON response
```

### Request ID Propagation

The logging middleware generates a unique request ID (UUID) and stores it in the request context under `RequestIDKey`. All structured log lines emitted during the request include this ID. It is also returned to the client in the `X-Request-ID` response header.

### Authentication

When `api.auth_enabled: true`, the `Authentication` middleware checks every request for a valid API key:

1. Reads the `X-API-Key` header.
2. Checks the key against the static list in `api.api_keys` (config file).
3. If not found in the config list, queries the `api_keys` database table and validates with bcrypt.
4. Stores `APIKeyInfo` in the request context on success; returns HTTP 401 on failure.

---

## 2. Scan Trigger Flow (HTTP → Scheduler → Scanning Engine → DB)

### 2a. Ad-hoc Scan via API

```
POST /api/v1/scans/{id}/start
  │
  ▼
ScanHandler.StartScan
  │
  ├─ db.GetScan(id)              ← load scan configuration from database
  ├─ validate scan is not already running
  ├─ go scanning.RunScanWithDB(config, db)   ← launch in goroutine
  │       │
  │       ├─ build nmap arguments from ScanConfig
  │       │    (targets, ports, scan type, timeout, concurrency)
  │       │
  │       ├─ exec nmap binary (via Ullaakut/nmap library)
  │       │    └─ nmap writes XML to stdout
  │       │
  │       ├─ ParseNmapXML(output)
  │       │    └─ unmarshal XML into NmapRun → []Host → []Port
  │       │
  │       ├─ db.SaveScanResults(scanID, results)
  │       │    ├─ INSERT INTO port_scans (...)
  │       │    └─ UPDATE scan_jobs SET status = 'completed', ...
  │       │
  │       └─ update scan status in DB (running → completed / failed)
  │
  └─ writeJSON(w, 202, {"status": "started"})
```

### 2b. Scheduled Scan via Scheduler

```
cron trigger (e.g., "0 */6 * * *")
  │
  ▼
Scheduler.executeScanJob(ctx, jobConfig)
  │
  ├─ prepareJobExecution()      ← mark job as Running in memory
  │
  ├─ getHostsToScan(jobConfig)
  │    └─ SELECT hosts FROM hosts WHERE ...
  │         filters: live_hosts_only, os_family, network, max_age_hours
  │
  ├─ for each host (bounded by semaphore, default max 5 concurrent):
  │    │
  │    ├─ selectProfileForHost(host)
  │    │    └─ db.GetProfile(profileID) or use defaults
  │    │
  │    └─ go scanning.RunScanWithContext(ctx, &ScanConfig{...}, db)
  │              │
  │              └─ (same nmap → XML → DB path as 2a above)
  │
  ├─ wait for all goroutines via sync.WaitGroup
  ├─ updateJobLastRun() in DB
  └─ cleanupJobExecution()      ← mark job as not Running
```

---

## 3. Discovery Flow (HTTP → Discovery Engine → DB)

### 3a. Ad-hoc Discovery via API

```
POST /api/v1/discovery/{id}/start
  │
  ▼
DiscoveryHandler.StartDiscovery
  │
  ├─ db.GetDiscoveryJob(id)     ← load job configuration
  │
  └─ discoveryEngine.Discover(ctx, &Config{Network: cidr, Method: method})
          │
          ├─ net.ParseCIDR(network)
          ├─ validateNetworkSize()  ← reject if prefix < /16
          │
          ├─ db INSERT discovery_jobs (status=running)
          │
          └─ go runDiscovery(ctx, job, config)   ← background goroutine
                  │
                  ├─ generateTargetsFromCIDR()
                  │    └─ enumerate individual IPs from CIDR block
                  │         (respects /31 RFC 3021, skips network/broadcast)
                  │
                  ├─ calculateDynamicTimeout(len(targets), baseTimeout)
                  │
                  ├─ buildNmapLibraryOptions(targets, config, timeout)
                  │    ├─ nmap.WithPingScan()       ← -sn (no port scan)
                  │    ├─ method "tcp"  → WithSYNDiscovery(ports...)
                  │    ├─ method "ping" → WithICMPEchoDiscovery()
                  │    └─ method "arp"  → WithCustomArguments("-PR")
                  │
                  ├─ nmap.NewScanner(ctx, options...).Run()
                  │    └─ nmap binary executes host discovery
                  │
                  ├─ convertNmapResultsToDiscovery()
                  │    └─ filter to hosts with status "up"
                  │
                  ├─ saveDiscoveredHosts(ctx, results)
                  │    └─ for each discovered host:
                  │         ├─ SELECT id FROM hosts WHERE ip_address = $1
                  │         ├─ if exists: UPDATE hosts SET last_seen, discovery_count++
                  │         └─ if new:    INSERT INTO hosts (ip_address, status, ...)
                  │
                  └─ finalizeDiscoveryJob()
                       └─ UPDATE discovery_jobs SET status='completed', completed_at, hosts_discovered
```

### 3b. Scheduled Discovery via Scheduler

```
cron trigger (e.g., "0 */12 * * *")
  │
  ▼
Scheduler.executeDiscoveryJob(ctx, jobConfig)
  │
  ├─ prepareJobExecution()
  │
  └─ discovery.Engine.Discover(ctx, &Config{
         Network: jobConfig.Network,
         Method:  jobConfig.Method,
         Timeout: jobConfig.Timeout,
     })
       │
       └─ (same runDiscovery goroutine path as 3a above)
```

---

## 4. WebSocket Update Flow

The WebSocket hub in `internal/api/handlers/websocket.go` uses a hub-and-spoke pattern where a single goroutine (`run()`) owns all client maps and channels to avoid lock contention.

### Connection Setup

```
Client
  │  GET /api/v1/ws   (or /api/v1/scans/{id}/ws, /api/v1/discovery/{id}/ws)
  ▼
WebSocketHandler.GeneralWebSocket (or ScanWebSocket / DiscoveryWebSocket)
  │
  ├─ upgrader.Upgrade(w, r, nil)   ← HTTP → WebSocket upgrade
  │
  ├─ conn.SetReadLimit(512)
  ├─ conn.SetReadDeadline(now + 60s)
  ├─ conn.SetPongHandler(...)       ← refresh deadline on pong
  │
  ├─ register <- &clientRegistration{conn, connType}
  │    └─ hub goroutine adds conn to scanClients or discoveryClients map
  │
  ├─ go writePump(conn)            ← sends periodic pings (every ~54s)
  └─ readPump(conn)                ← blocks; cleans up on disconnect
```

### Broadcasting an Update

Updates are broadcast by any component that has access to the `WebSocketHandler`. For example, a scan handler or background worker calls `BroadcastScanUpdate`:

```
scanning.RunScanWithContext completes (or status changes)
  │
  └─ handler or scheduler calls:
       wsHandler.BroadcastScanUpdate(&ScanUpdateMessage{
           ScanID:   id,
           Status:   "completed",
           Progress: 1.0,
       })
         │
         ├─ json.Marshal(WebSocketMessage{Type: "scan_update", ...})
         └─ scanBroadcast <- data    ← non-blocking send to buffered channel (cap 256)
                │
                ▼
         hub goroutine (run loop)
                │
                └─ broadcastToClients(scanClients, data, "scan")
                     └─ for each conn in scanClients:
                          conn.SetWriteDeadline(now + 10s)
                          conn.WriteMessage(TextMessage, data)
```

If the broadcast channel is full (256 messages), the message is dropped and a warning is logged — the hub never blocks a caller.

### WebSocket Message Structure

All messages share a common envelope:

```json
{
  "type": "scan_update",
  "timestamp": "2026-01-01T12:00:00Z",
  "data": { ... }
}
```

| `type` value | `data` schema | Sent to |
|---|---|---|
| `scan_update` | `ScanUpdateMessage` (scan_id, status, progress, results_count, …) | scan clients + general clients |
| `discovery_update` | `DiscoveryUpdateMessage` (job_id, status, progress, hosts_found, …) | discovery clients + general clients |
| `system_*` | `{"message": "..."}` | all clients |

---

## 5. Database Migration Flow (Startup)

```
daemon.Start()
  │
  └─ db.ConnectAndMigrate(ctx, &dbConfig)
          │
          ├─ db.Connect()             ← open connection pool (sqlx)
          │
          └─ migrator.Up(ctx)
                  │
                  ├─ CREATE TABLE IF NOT EXISTS schema_migrations (...)
                  │
                  ├─ SELECT name FROM schema_migrations  ← already-applied set
                  │
                  ├─ fs.WalkDir(embed.FS, ".")           ← reads *.sql files
                  │   sorted lexicographically:
                  │   001_initial_schema.sql
                  │   002_performance_improvements.sql
                  │   003_networks_table.sql
                  │   004_network_exclusions.sql
                  │   005_fix_scan_types.sql
                  │   006_api_keys_table.sql
                  │
                  └─ for each pending file:
                       BEGIN TRANSACTION
                         exec migration SQL
                         INSERT INTO schema_migrations (name, checksum)
                       COMMIT
```

Migrations are embedded in the binary at compile time (`//go:embed *.sql`) so no external migration tool or separate migration files are needed at runtime.

---

## 6. Metrics Flow

```
Any handler or package
  │
  ├─ metrics.Counter("scan_total", labels)
  ├─ metrics.Gauge("active_connections", value, labels)
  └─ metrics.NewTimer("scan_duration_seconds", labels)
          │
          ▼
  internal/metrics.Registry   ← in-memory store
          │
          ▼ (also registered with)
  prometheus/client_golang Registry
          │
  GET /metrics
          │
          ▼
  promhttp.HandlerFor(registry, ...)   ← Prometheus text exposition format
          │
          ▼
  Prometheus scraper
```

The daemon also runs a background goroutine (`prom.StartPeriodicUpdates`, every 5 seconds) that refreshes system-level gauges such as Go runtime memory stats.

---

## 7. Configuration Load Flow (Startup)

```
cli.initConfig()
  │
  ├─ viper.SetConfigFile(path) or search for ./config.yaml
  ├─ viper.SetEnvPrefix("SCANORAMA")
  ├─ viper.AutomaticEnv()          ← SCANORAMA_* env vars overlay config
  └─ viper.ReadInConfig()
          │
          ▼
config.Load(path)
  │
  ├─ validateConfigPath()          ← path traversal and length checks
  ├─ validateConfigPermissions()   ← must not be world-readable (0o600 / 0o640)
  ├─ config.Default()              ← baseline with env var overrides
  │    └─ getDatabaseConfigFromEnv()
  │         reads SCANORAMA_DB_HOST, _PORT, _NAME, _USER, _PASSWORD, _SSLMODE
  ├─ safeYAMLUnmarshal(data, cfg)  ← strict decode, rejects unknown fields
  └─ cfg.Validate()                ← semantic validation (ports, timeouts, TLS, …)
          │
          ▼
  *config.Config returned to caller
```

---

## Summary Table

| Operation | Entry Point | Key Packages | Storage |
|---|---|---|---|
| HTTP API request | `gorilla/mux` router | `api`, `api/middleware`, `api/handlers` | PostgreSQL via `db` |
| Ad-hoc scan | `POST /scans/{id}/start` | `api/handlers`, `scanning` | `scan_jobs`, `port_scans` |
| Scheduled scan | cron trigger | `scheduler`, `scanning` | `scan_jobs`, `port_scans`, `hosts` |
| Ad-hoc discovery | `POST /discovery/{id}/start` | `api/handlers`, `discovery` | `discovery_jobs`, `hosts` |
| Scheduled discovery | cron trigger | `scheduler`, `discovery` | `discovery_jobs`, `hosts` |
| WebSocket update | `GET /api/v1/ws` | `api/handlers/websocket` | In-memory broadcast only |
| DB migration | daemon startup | `db/migrate` | `schema_migrations` |
| Metrics scrape | `GET /metrics` | `metrics` | In-memory + Prometheus registry |