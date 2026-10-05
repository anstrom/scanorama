# Scheduling Flow

This document describes how the Scanorama scheduler works: how cron expressions drive job execution, how scan and discovery jobs are dispatched, and how bounded concurrency keeps the system stable.

---

## 1. Overview

The scheduler is a background subsystem that executes recurring scan and discovery jobs on cron-based schedules. It is implemented in `internal/scheduler/` and built on top of the [`robfig/cron/v3`](https://github.com/robfig/cron) library.

Its responsibilities are:

- **Load** persisted job definitions from the `scheduled_jobs` PostgreSQL table at startup.
- **Register** each enabled job with the cron runtime using its stored cron expression.
- **Execute** jobs when the cron runtime fires them — discovery jobs invoke the discovery engine; scan jobs query the host table, select profiles, and dispatch bounded-concurrency nmap scans.
- **Persist** run metadata (`last_run`, `next_run`) back to the database after every execution.
- **Manage** jobs at runtime — add, remove, enable, and disable jobs via the API or CLI.

---

## 2. Architecture — Scheduler in the Daemon Lifecycle

The scheduler is created during daemon startup and runs for the lifetime of the process. It depends on the database, the discovery engine, and the profile manager.

```
daemon.Start()
  │
  ├─ db.ConnectAndMigrate()                   ← opens DB, runs migrations
  ├─ discovery.NewEngine(db)                  ← creates discovery engine
  ├─ profiles.NewManager(db)                  ← creates profile manager
  │
  ├─ scheduler.NewScheduler(db, engine, mgr)  ← creates scheduler
  │       │
  │       └─ .WithMaxConcurrentScans(n)       ← optional concurrency override
  │
  ├─ scheduler.Start()
  │       │
  │       ├─ loadScheduledJobs()              ← SELECT enabled jobs from DB
  │       │    └─ for each job:
  │       │         ├─ parse config JSON
  │       │         ├─ cron.AddFunc(expr, executeFunc)
  │       │         └─ store in s.jobs map
  │       │
  │       └─ cron.Start()                     ← starts the cron ticker
  │
  ├─ api.Server.Start()                       ← REST API (can manage jobs)
  └─ health-check loop
```

### Shutdown

When the daemon receives `SIGTERM`/`SIGINT`:

1. `scheduler.Stop()` is called.
2. The cron runtime is stopped (`cron.Stop()`), preventing new job firings.
3. The scheduler's context is canceled (`cancel()`), signaling in-flight jobs to wind down.
4. In-flight scan goroutines observe the canceled context and exit after their current nmap process completes.

---

## 3. Job Types

The scheduler supports two job types, stored in the `type` column of the `scheduled_jobs` table.

| Type | Constant | Config Struct | Executor |
|------|----------|---------------|----------|
| `discovery` | `db.ScheduledJobTypeDiscovery` | `DiscoveryJobConfig` | `executeDiscoveryJob` |
| `scan` | `db.ScheduledJobTypeScan` | `ScanJobConfig` | `executeScanJob` |

### Discovery Job Config (`DiscoveryJobConfig`)

| Field | JSON Key | Description |
|-------|----------|-------------|
| `Network` | `network` | CIDR to discover (e.g., `10.0.0.0/24`) |
| `Method` | `method` | Discovery method: `tcp`, `ping`, or `arp` |
| `DetectOS` | `detect_os` | Enable OS detection during discovery |
| `Timeout` | `timeout_seconds` | Per-discovery timeout in seconds |
| `Concurrency` | `concurrency` | Concurrency hint passed to discovery engine |

### Scan Job Config (`ScanJobConfig`)

| Field | JSON Key | Description |
|-------|----------|-------------|
| `LiveHostsOnly` | `live_hosts_only` | Only scan hosts with `status = 'up'` |
| `Networks` | `networks` | Optional list of CIDRs to filter hosts by |
| `ProfileID` | `profile_id` | Scan profile ID, or `"auto"` for auto-selection |
| `MaxAge` | `max_age_hours` | Only scan hosts seen within this many hours |
| `OSFamily` | `os_family` | Filter hosts by OS family (e.g., `["linux", "windows"]`) |

---

## 4. Cron Expression Handling

Scanorama uses **standard 5-field cron expressions** (minute, hour, day-of-month, month, day-of-week), parsed by `robfig/cron/v3` via `cron.ParseStandard()`.

```
 ┌───────────── minute (0–59)
 │ ┌───────────── hour (0–23)
 │ │ ┌───────────── day of month (1–31)
 │ │ │ ┌───────────── month (1–12)
 │ │ │ │ ┌───────────── day of week (0–6, Sun=0)
 │ │ │ │ │
 * * * * *
```

### Examples

| Expression | Meaning |
|------------|---------|
| `0 2 * * 0` | Every Sunday at 02:00 |
| `0 */6 * * *` | Every 6 hours |
| `0 1 * * *` | Daily at 01:00 |
| `0 * * * *` | Every hour on the hour |
| `*/15 * * * *` | Every 15 minutes |

### Validation

Cron expressions are validated at two levels:

1. **CLI** (`cmd/cli/schedule.go`) — `validateCronExpression()` checks that the expression contains exactly 5 whitespace-separated fields.
2. **Scheduler** (`internal/scheduler/scheduler.go`) — `cron.ParseStandard(cronExpr)` performs full semantic validation when creating or loading a job. Invalid expressions cause the job to be rejected with an error.

The `robfig/cron` library's standard parser does **not** accept the optional 6-field (seconds) format — only the 5-field format is supported.

---

## 5. Job Execution Flow

### 5a. Scan Job — End-to-End

```
cron ticker fires
  │
  ▼
executeScanJob(jobID, config)
  │
  ├─ recover()                           ← panic guard (deferred)
  │
  ├─ prepareJobExecution(jobID)
  │    ├─ RLock: check job exists, is enabled, is not already running
  │    ├─ Lock:  set job.Running = true, job.LastRun = now
  │    └─ return (job, true) or (nil, false) to skip
  │
  ├─ defer cleanupJobExecution(jobID)    ← sets job.Running = false
  ├─ defer updateJobLastRun(ctx, jobID)  ← persists last_run + next_run to DB
  │
  ├─ getHostsToScan(ctx, config)
  │    ├─ buildHostScanQuery(config)     ← base SELECT with ignore_scanning=false
  │    ├─ addHostScanFilters(...)        ← WHERE clauses for live_hosts, max_age,
  │    │                                    os_family, networks
  │    └─ executeHostScanQuery(ctx, ...) ← runs query, scans rows into []*db.Host
  │
  ├─ if len(hosts) == 0 → log, return
  │
  ├─ processHostsForScanning(ctx, hosts, config)
  │    │
  │    ├─ create semaphore channel (cap = maxConcurrentScans)
  │    ├─ create sync.WaitGroup
  │    │
  │    └─ for each host:
  │         ├─ check ctx.Err() → break if canceled
  │         ├─ selectProfileForHost(ctx, host, configProfileID)
  │         │    └─ if "auto": profiles.SelectBestProfile(host)
  │         │       else:      use config profile ID
  │         │
  │         ├─ profiles.GetByID(ctx, profileID) → get ports, scan type, timing
  │         │
  │         ├─ sem <- struct{}{}        ← acquire semaphore (blocks if full)
  │         │    (or break on ctx.Done)
  │         │
  │         ├─ wg.Add(1)
  │         └─ go func(host, profile):
  │              ├─ defer <-sem          ← release semaphore
  │              ├─ defer wg.Done()
  │              ├─ defer recover()      ← panic guard per goroutine
  │              │
  │              ├─ scanning.RunScanWithContext(ctx, &ScanConfig{
  │              │      Targets:    [host.IPAddress],
  │              │      Ports:      profile.Ports,
  │              │      ScanType:   profile.ScanType,
  │              │      TimeoutSec: timingToScanTimeout(profile.Timing),
  │              │  }, db)
  │              │    │
  │              │    ├─ build nmap arguments
  │              │    ├─ exec nmap binary
  │              │    ├─ ParseNmapXML(output)
  │              │    └─ db.SaveScanResults(...)
  │              │
  │              └─ log result or error
  │
  ├─ wg.Wait()                          ← block until all scans finish
  │
  └─ log completion with elapsed time
```

### 5b. Discovery Job — End-to-End

```
cron ticker fires
  │
  ▼
executeDiscoveryJob(jobID, config)
  │
  ├─ recover()                           ← panic guard (deferred)
  │
  ├─ RLock: check job exists, enabled, not already running
  ├─ Lock:  set job.Running = true, job.LastRun = now
  ├─ defer: set job.Running = false
  │
  ├─ build discovery.Config{
  │      Network:     config.Network,
  │      Method:      config.Method,
  │      DetectOS:    config.DetectOS,
  │      Timeout:     config.Timeout * time.Second,
  │      Concurrency: config.Concurrency,
  │  }
  │
  ├─ discovery.Engine.Discover(ctx, &discoveryConfig)
  │    │
  │    ├─ net.ParseCIDR(), validateNetworkSize()
  │    ├─ INSERT discovery_jobs (status=running)
  │    └─ go runDiscovery(ctx, ...)
  │         ├─ generateTargetsFromCIDR()
  │         ├─ nmap -sn (ping scan) with method-specific flags
  │         ├─ save discovered hosts to DB (INSERT or UPDATE)
  │         └─ UPDATE discovery_jobs SET status='completed'
  │
  ├─ updateJobLastRun(ctx, jobID, startTime)
  │    ├─ calculateNextRun(jobID, startTime)
  │    ├─ UPDATE scheduled_jobs SET last_run, next_run
  │    └─ update in-memory NextRun
  │
  └─ log completion or failure with elapsed time
```

---

## 6. Concurrency Model

Scan jobs can target many hosts. To avoid spawning an unbounded number of nmap processes, the scheduler uses a **semaphore-based bounded parallelism** pattern.

### Semaphore Implementation

```go
sem := make(chan struct{}, s.maxConcurrentScans)  // default cap = 5
var wg sync.WaitGroup

for _, host := range hosts {
    sem <- struct{}{}     // acquire slot (blocks when cap reached)
    wg.Add(1)
    go func(h *db.Host) {
        defer func() {
            <-sem           // release slot
            wg.Done()
        }()
        // ... run nmap scan for this host ...
    }(host)
}

wg.Wait()                 // wait for all in-flight scans
```

### How It Works

```
maxConcurrentScans = 3, hosts = [H1, H2, H3, H4, H5]

Time ──────────────────────────────────────────────────────▶

sem slots:   [  ] [  ] [  ]     ← 3 slots available

  H1 ────── acquire ═══════════════ scan H1 ═══════════ release ──
  H2 ────── acquire ═══════════ scan H2 ═══════ release ──────────
  H3 ────── acquire ═══════ scan H3 ═════ release ────────────────
  H4 ─────────────── (blocked) ──── acquire ════ scan H4 ═ release
  H5 ─────────────── (blocked) ────────── acquire ═ scan H5 ═ rel.

            ├── 3 concurrent ──┤    ├── 3 concurrent ──┤
```

### Key Properties

| Property | Value |
|----------|-------|
| Default concurrency limit | `5` (`defaultMaxConcurrentScans`) |
| Override method | `scheduler.WithMaxConcurrentScans(n)` |
| Minimum value | `1` (values ≤ 0 are ignored, default kept) |
| Completion signal | `sync.WaitGroup` — `executeScanJob` blocks until all host goroutines finish |
| Context cancellation | The semaphore acquire uses a `select` with `ctx.Done()` so that a canceled context breaks the dispatch loop immediately |

### Context-Aware Semaphore Acquisition

The acquire step is not a simple channel send — it checks for context cancellation:

```go
select {
case sem <- struct{}{}:
    // acquired — proceed with scan
case <-ctx.Done():
    // context canceled — stop dispatching
    break hostLoop
}
```

This ensures that daemon shutdown or job cancellation does not leave goroutines blocked indefinitely on a full semaphore.

---

## 7. Next Run Time Calculation

The `getNextRunTime` function (in `cmd/cli/schedule.go`) and the `calculateNextRun` method (in `internal/scheduler/scheduler.go`) both use the same approach:

```
cron.ParseStandard(cronExpr)   →   schedule.Next(referenceTime)
```

### CLI — `getNextRunTime`

Used for display purposes when listing jobs or confirming job creation:

```go
func getNextRunTime(cronExpr string) time.Time {
    schedule, err := cron.ParseStandard(cronExpr)
    if err != nil {
        return time.Time{}   // zero time on invalid expression
    }
    return schedule.Next(time.Now())
}
```

### Scheduler — `calculateNextRun`

Used after every job execution to compute and persist the next scheduled time:

```go
func (s *Scheduler) calculateNextRun(jobID uuid.UUID, after time.Time) time.Time {
    // 1. Look up the job's cron expression from the in-memory map
    // 2. cron.ParseStandard(expr)
    // 3. schedule.Next(after)
    // Falls back to zero time if job not found or expression invalid
}
```

### When Next Run Is Updated

```
Job execution completes (success or failure)
  │
  └─ updateJobLastRun(ctx, jobID, startTime)
       │
       ├─ calculateNextRun(jobID, startTime)      ← compute next occurrence
       ├─ UPDATE scheduled_jobs SET last_run=$1, next_run=$2
       └─ updateJobNextRunInMemory(jobID, nextRun) ← keep in-memory state consistent
```

The `next_run` value is persisted to the database so that it can be displayed by the CLI and API even when the scheduler is not running.

---

## 8. Error Handling

Errors are handled at multiple levels to ensure that one failing host or job does not take down the scheduler.

### Job-Level Error Handling

| Scenario | Behavior |
|----------|----------|
| Job not found in memory | Execution silently skipped |
| Job disabled | Execution silently skipped |
| Job already running | Logged and skipped (prevents overlapping runs) |
| Panic in job execution | Caught by `recover()`, logged, `job.Running` reset to `false` |
| `getHostsToScan` fails | Error logged, job returns early; `last_run`/`next_run` still updated |
| No hosts match filters | Logged as informational, job completes normally |
| Discovery engine fails | Error logged, `last_run`/`next_run` still updated |

### Host-Level Error Handling (Scan Jobs)

Each host scan runs in its own goroutine with independent error handling:

```
for each host goroutine:
  │
  ├─ defer recover()           ← catch panics per-host
  │
  ├─ selectProfileForHost()
  │    └─ on error → log, skip host (continue to next)
  │
  ├─ profiles.GetByID()
  │    └─ on error → log, skip host (continue to next)
  │
  └─ scanning.RunScanWithContext()
       ├─ on success → log result summary
       └─ on error   → log error, return (other hosts unaffected)
```

### Key Design Decision

**Individual host scan failures do not fail the overall job.** The `sync.WaitGroup` waits for all goroutines regardless of individual outcomes. The job is always marked as complete, and `last_run` / `next_run` are always updated. This ensures the cron schedule advances even if some or all hosts fail.

---

## 9. Configuration

### Scheduler-Specific Settings

The scheduler's concurrency limit is set programmatically via `WithMaxConcurrentScans()`. The related configuration keys in the `scanning` section of the config file control broader scanning behavior:

| Config Key (YAML) | Type | Default | Description |
|--------------------|------|---------|-------------|
| `scanning.worker_pool_size` | `int` | `10` | Size of the global worker pool for scan jobs |
| `scanning.max_concurrent_targets` | `int` | `100` | Maximum concurrent targets per scan execution |
| `scanning.default_interval` | `duration` | `1h` | Default scan interval for targets |
| `scanning.max_scan_timeout` | `duration` | `10m` | Maximum timeout per individual scan |
| `scanning.default_ports` | `string` | `22,80,443,8080,8443` | Default ports when no profile specifies them |
| `scanning.scan_mode` | `string` | `syn` | Default scan type when no profile specifies one |
| `scanning.enable_service_detection` | `bool` | `true` | Enable nmap service/version detection |
| `scanning.enable_os_detection` | `bool` | `false` | Enable nmap OS detection |

### Scheduler Internal Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `defaultMaxConcurrentScans` | `5` | Max parallel host scans per job execution |
| `scanTimeoutParanoid` | `3600s` (1h) | Timeout for nmap T0 timing |
| `scanTimeoutPolite` | `1800s` (30m) | Timeout for nmap T1 timing |
| `scanTimeoutNormal` | `900s` (15m) | Timeout for nmap T3 timing |
| `scanTimeoutAggressive` | `600s` (10m) | Timeout for nmap T4 timing |
| `scanTimeoutInsane` | `300s` (5m) | Timeout for nmap T5 timing |

### CLI Defaults for Job Creation

When creating jobs via the CLI (`scanorama schedule add-scan`), these defaults apply:

| Flag | Default | Description |
|------|---------|-------------|
| `--ports` | `22,80,443,8080,8443` | Ports to scan |
| `--type` | `connect` | Scan type |
| `--timeout` | `300` (5 min) | Scan timeout in seconds |
| `--method` | `tcp` | Discovery method (for `add-discovery`) |

---

## 10. Database Schema

Jobs are stored in the `scheduled_jobs` table:

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` | Primary key |
| `name` | `TEXT` | Human-readable job name (unique) |
| `type` | `TEXT` | `'discovery'` or `'scan'` |
| `cron_expression` | `TEXT` | 5-field cron expression |
| `config` | `JSONB` | Job-specific configuration (see §3) |
| `enabled` | `BOOLEAN` | Whether the job is active |
| `last_run` | `TIMESTAMPTZ` | When the job last executed (NULL if never) |
| `next_run` | `TIMESTAMPTZ` | Computed next execution time |
| `created_at` | `TIMESTAMPTZ` | Job creation timestamp |
| `last_run_duration_ms` | `INTEGER` | Duration of the last run in milliseconds |

---

## 11. Full System Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                              Scheduler                                   │
│                                                                          │
│  ┌─────────────────┐     ┌──────────────────────────────────────────┐   │
│  │  robfig/cron     │     │          In-Memory Job Registry          │   │
│  │  runtime         │     │       map[uuid.UUID]*ScheduledJob        │   │
│  │                  │     │                                          │   │
│  │  Evaluates cron  │     │  ┌──────────┐ ┌──────────┐ ┌─────────┐ │   │
│  │  expressions     │────▶│  │ Job A    │ │ Job B    │ │ Job C   │ │   │
│  │  every minute    │     │  │ disc/scan│ │ disc/scan│ │disc/scan│ │   │
│  └─────────────────┘     │  │ Running? │ │ Running? │ │Running? │ │   │
│           │               │  └──────────┘ └──────────┘ └─────────┘ │   │
│           │               └──────────────────────────────────────────┘   │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Job Execution                                 │    │
│  │                                                                  │    │
│  │  ┌──────────────────────┐    ┌────────────────────────────────┐ │    │
│  │  │  executeDiscoveryJob │    │  executeScanJob                │ │    │
│  │  │                      │    │                                │ │    │
│  │  │  discovery.Engine    │    │  getHostsToScan()              │ │    │
│  │  │    .Discover(...)    │    │  selectProfileForHost()        │ │    │
│  │  │                      │    │  processHostsForScanning()     │ │    │
│  │  │  ┌────────────────┐  │    │                                │ │    │
│  │  │  │ nmap -sn       │  │    │  ┌─sem──────────────────────┐ │ │    │
│  │  │  │ (ping scan)    │  │    │  │ Bounded concurrency (5)  │ │ │    │
│  │  │  └────────────────┘  │    │  │                          │ │ │    │
│  │  │                      │    │  │ ┌──────┐ ┌──────┐ ┌────┐│ │ │    │
│  │  │  Save hosts to DB   │    │  │ │nmap  │ │nmap  │ │... ││ │ │    │
│  │  └──────────────────────┘    │  │ │host1 │ │host2 │ │    ││ │ │    │
│  │                              │  │ └──────┘ └──────┘ └────┘│ │ │    │
│  │                              │  └──────────────────────────┘ │ │    │
│  │                              │                                │ │    │
│  │                              │  wg.Wait() → all scans done   │ │    │
│  │                              └────────────────────────────────┘ │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│           │                                                              │
│           ▼                                                              │
│  ┌─────────────────────────────────┐                                    │
│  │  updateJobLastRun(jobID, time)  │                                    │
│  │    ├─ calculateNextRun()        │                                    │
│  │    ├─ UPDATE scheduled_jobs     │                                    │
│  │    └─ update in-memory NextRun  │                                    │
│  └─────────────────────────────────┘                                    │
└──────────────────────────────────────────────────────────────────────────┘
          │                    │
          ▼                    ▼
┌──────────────────┐  ┌─────────────────┐
│   PostgreSQL     │  │  nmap binary    │
│                  │  │  (system)       │
│  scheduled_jobs  │  └─────────────────┘
│  hosts           │
│  port_scans      │
│  discovery_jobs  │
└──────────────────┘
```

---

## 12. Job Lifecycle State Machine

A scheduled job moves through the following states during its lifetime:

```
                    ┌──────────────────────────────────────┐
                    │                                      │
                    ▼                                      │
┌────────┐    ┌─────────┐    ┌─────────┐    ┌──────────┐ │
│Created │───▶│ Enabled │───▶│ Running │───▶│Completed │─┘
│        │    │ (idle)  │    │         │    │ (idle)   │
└────────┘    └─────────┘    └─────────┘    └──────────┘
                  │  ▲                           │
                  │  │                           │
                  ▼  │                           │
              ┌──────────┐                       │
              │ Disabled │                       │
              │          │◀──────────────────────┘
              └──────────┘       (can be disabled at any time)

State transitions:
  Created  → Enabled    : Job inserted into DB with enabled=true
  Enabled  → Running    : Cron fires, prepareJobExecution sets Running=true
  Running  → Completed  : Execution finishes, cleanupJobExecution sets Running=false
  Completed→ Enabled    : Job returns to idle, waiting for next cron trigger
  Enabled  ↔ Disabled   : EnableJob() / DisableJob() toggles the enabled flag
  Running  → Enabled    : If job is already running when cron fires, the new
                           invocation is skipped (overlap prevention)
```

---

## 13. Overlap Prevention

The scheduler prevents the same job from running concurrently with itself. Before execution begins, `prepareJobExecution` checks the `Running` flag under a read lock:

```
prepareJobExecution(jobID)
  │
  ├─ RLock
  │   ├─ job exists?        → no  → return (nil, false)
  │   ├─ job.Config.Enabled?→ no  → return (nil, false)
  │   └─ job.Running?       → yes → log "already running, skipping"
  │                                  return (nil, false)
  ├─ RUnlock
  │
  ├─ Lock
  │   ├─ job.Running = true
  │   └─ job.LastRun = time.Now()
  └─ Unlock
      return (job, true)
```

If a job takes longer than its cron interval (e.g., a scan job with `0 */1 * * *` runs for 90 minutes), subsequent cron firings are silently skipped until the current execution completes.

---

## Related Documentation

- [`system-overview.md`](./system-overview.md) – high-level architecture and package reference
- [`data-flow.md`](./data-flow.md) – request lifecycle, scan trigger flow (§2), discovery flow (§3)
- [`logging.md`](./logging.md) – logging, metrics, and worker pool details
- [`../../DEPLOYMENT.md`](../../DEPLOYMENT.md) – deployment guide and environment variables