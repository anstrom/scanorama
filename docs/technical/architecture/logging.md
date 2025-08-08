# Logging and Worker Pool Architecture

This document describes the structured logging, metrics collection, and worker pool systems implemented in Scanorama.

## Overview

Scanorama implements modern observability and concurrency patterns through three integrated packages:

- **`internal/logging`** - Structured logging with `slog`
- **`internal/metrics`** - Performance metrics collection
- **`internal/workers`** - Concurrent job execution

## Structured Logging (`internal/logging`)

### Design Goals

- Replace traditional `fmt.Printf` and `log.Printf` with structured logging
- Support both human-readable text and machine-parseable JSON formats
- Provide context-aware logging for different components
- Enable configurable log levels and outputs

### Architecture

```go
type Logger struct {
    *slog.Logger
    config Config
}

type Config struct {
    Level     LogLevel  // debug, info, warn, error
    Format    LogFormat // text, json
    Output    string    // stdout, stderr, or file path
    AddSource bool      // include source file information
}
```

### Integration Points

1. **CLI Initialization**: Logging is initialized in `cmd/cli/root.go` after configuration loading
2. **Configuration**: Logging config is part of main application config
3. **Component-Specific Loggers**: Specialized functions for scans, discovery, database, daemon

### Usage Patterns

```go
// Basic logging
logging.Info("Operation started", "component", "scanner")
logging.Error("Operation failed", "error", err)

// Component-specific logging
logging.InfoScan("Scanning target", "192.168.1.1", "ports", "80,443")
logging.ErrorDatabase("Query failed", err, "table", "hosts")

// Contextual logging
logger := logging.Default().WithComponent("discovery")
logger.Info("Network scan complete", "hosts_found", 42)
```

## Metrics Collection (`internal/metrics`)

### Design Goals

- Collect performance and operational metrics
- Support standard metric types (counters, gauges, histograms)
- Enable monitoring of scan performance and system health
- Provide foundation for future Prometheus integration

### Architecture

```go
type Registry struct {
    metrics map[string]*Metric
    enabled bool
}

type Metric struct {
    Name      string
    Type      MetricType // counter, gauge, histogram
    Value     float64
    Labels    Labels
    Timestamp time.Time
}
```

### Predefined Metrics

- **Scan Metrics**: `scan_duration_seconds`, `scan_total`, `scan_errors_total`
- **Discovery Metrics**: `discovery_duration_seconds`, `hosts_discovered_total`
- **Database Metrics**: `database_queries_total`, `database_connections_active`

### Usage Patterns

```go
// Time operation execution
timer := metrics.NewTimer(metrics.MetricScanDuration, metrics.Labels{
    metrics.LabelScanType: "aggressive",
    metrics.LabelTarget:   "192.168.1.1",
})
defer timer.Stop()

// Count events
metrics.IncrementScanTotal("aggressive", "success")

// Record values
metrics.Gauge("active_connections", 25, nil)
```

## Worker Pool (`internal/workers`)

### Design Goals

- Enable concurrent execution of scan and discovery operations
- Provide job queuing with configurable parallelism
- Implement retry logic for transient failures
- Support rate limiting to avoid overwhelming networks
- Enable graceful shutdown

### Architecture

```go
type Pool struct {
    config   Config
    jobs     chan Job
    results  chan Result
    workers  []*worker
    // ... other fields
}

type Job interface {
    Execute(ctx context.Context) error
    ID() string
    Type() string
}
```

### Job Types

- **ScanJob**: Executes individual host scans
- **DiscoveryJob**: Performs network discovery operations

### Configuration

```go
type Config struct {
    Size            int           // Number of worker goroutines
    QueueSize       int           // Job queue capacity
    MaxRetries      int           // Retry attempts for failed jobs
    RetryDelay      time.Duration // Delay between retries
    ShutdownTimeout time.Duration // Graceful shutdown timeout
    RateLimit       int           // Jobs per second (0 = unlimited)
}
```

### Usage Patterns

```go
// Create and start pool
pool := workers.New(workers.DefaultConfig())
pool.Start()
defer pool.Shutdown()

// Submit jobs
job := workers.NewScanJob("scan-1", "192.168.1.1", "80,443", "aggressive", scanExecutor)
if err := pool.Submit(job); err != nil {
    log.Printf("Failed to submit job: %v", err)
}

// Process results
go func() {
    for result := range pool.Results() {
        if result.Error != nil {
            log.Printf("Job %s failed: %v", result.JobID, result.Error)
        }
    }
}()
```

## Integration Examples

### Scan Operation with Full Observability

```go
func ExecuteScanWithObservability(target string) error {
    // Start timing
    timer := metrics.NewTimer(metrics.MetricScanDuration, metrics.Labels{
        metrics.LabelTarget: target,
        metrics.LabelScanType: "aggressive",
    })
    defer timer.Stop()

    // Create component logger
    logger := logging.Default().WithComponent("scanner").WithTarget(target)
    
    logger.Info("Starting scan operation")
    
    // Execute scan
    if err := performScan(target); err != nil {
        // Record error metrics
        metrics.IncrementScanErrors("aggressive", target, "execution_failed")
        
        // Log structured error
        logger.Error("Scan failed", "error", err)
        
        return errors.WrapScanErrorWithTarget(
            errors.CodeScanFailed, 
            "scan execution failed", 
            target, 
            err,
        )
    }
    
    // Record success
    metrics.IncrementScanTotal("aggressive", "success")
    logger.Info("Scan completed successfully")
    
    return nil
}
```

### Concurrent Discovery with Worker Pool

```go
func DiscoverNetworksWithWorkers(networks []string) error {
    // Create worker pool
    pool := workers.New(workers.Config{
        Size:       5,
        QueueSize:  50,
        MaxRetries: 3,
        RateLimit:  10, // 10 discoveries per second
    })
    
    pool.Start()
    defer pool.Shutdown()
    
    // Submit discovery jobs
    for _, network := range networks {
        job := workers.NewDiscoveryJob(
            fmt.Sprintf("discover-%s", network),
            network,
            "tcp",
            discoveryExecutor,
        )
        
        if err := pool.Submit(job); err != nil {
            logging.Error("Failed to submit discovery job", 
                "network", network, "error", err)
            continue
        }
    }
    
    // Process results
    var errors []error
    for i := 0; i < len(networks); i++ {
        result := <-pool.Results()
        if result.Error != nil {
            errors = append(errors, result.Error)
        }
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("discovery failed for %d networks", len(errors))
    }
    
    return nil
}
```

## Configuration Integration

All observability features are configured through the main application configuration:

```yaml
# config.yaml
logging:
  level: info
  format: text
  output: stdout
  add_source: false

# Worker pool configurations are embedded in scanning config
scanning:
  worker_pool_size: 10
  max_concurrent_targets: 100
  rate_limit:
    enabled: true
    requests_per_second: 100
```

## Performance Considerations

### Logging Performance

- Structured logging adds minimal overhead compared to traditional logging
- JSON format is slightly slower than text but provides better machine parsing
- File output requires disk I/O but provides persistence
- Source location adds overhead and should only be enabled for debugging

### Worker Pool Performance

- Pool size should match available CPU cores for CPU-bound tasks
- For I/O-bound tasks (network scans), higher pool sizes can improve throughput
- Rate limiting prevents overwhelming target networks
- Queue size should accommodate burst workloads

### Metrics Performance

- In-memory metrics collection has minimal overhead
- Histogram calculations are simplified for performance
- Metrics are collected in goroutine-safe manner with minimal locking

## Future Enhancements

### Logging
- Log rotation for file outputs
- Remote log shipping (syslog, fluentd)
- Log sampling for high-volume operations

### Metrics
- Prometheus metrics endpoint
- Histogram buckets for better latency analysis
- Custom metric exporters

### Worker Pools
- Priority job queues
- Dynamic worker scaling
- Job persistence for restart recovery
- Circuit breaker pattern integration

## Troubleshooting

### Common Issues

1. **High Memory Usage**: Reduce worker pool size or queue size
2. **Slow Performance**: Increase worker pool size or disable rate limiting
3. **Missing Logs**: Check log level configuration and output destination
4. **Job Queue Full**: Increase queue size or reduce job submission rate

### Debugging

Enable debug logging to see detailed operation information:

```yaml
logging:
  level: debug
  add_source: true
```

Monitor metrics to identify performance bottlenecks:

```go
metrics := metrics.GetMetrics()
for name, metric := range metrics {
    fmt.Printf("%s: %f\n", name, metric.Value)
}
```
