# TODO Items - Scanorama Project

This file tracks outstanding TODO items identified in the codebase. Items are categorized by priority and component.

## High Priority

### Daemon Component (`internal/daemon/daemon.go`)

#### Configuration Reload Support
- **Location**: `internal/daemon/daemon.go:278`
- **Description**: Implement configuration reload functionality for SIGHUP signal
- **Current State**: Signal handler exists but functionality is not implemented
- **Impact**: High - Would allow runtime configuration changes without service restart
- **Estimate**: Medium effort

```go
// TODO: Implement configuration reload
case syscall.SIGHUP:
    d.logger.Println("Received SIGHUP - configuration reload not implemented")
```

#### Database Reconnection Logic
- **Location**: `internal/daemon/daemon.go:401`
- **Description**: Implement automatic database reconnection when health checks fail
- **Current State**: Health check detects failures but doesn't attempt reconnection
- **Impact**: High - Critical for service reliability
- **Estimate**: Medium effort

```go
// TODO: Implement reconnection logic
if err := d.database.Ping(d.ctx); err != nil {
    d.logger.Printf("Database health check failed: %v", err)
    // TODO: Implement reconnection logic
}
```

## Medium Priority

### Daemon Component (`internal/daemon/daemon.go`)

#### Custom Signal Actions
- **Location**: `internal/daemon/daemon.go:283` and `internal/daemon/daemon.go:285`
- **Description**: Implement custom actions for SIGUSR1 and SIGUSR2 signals
- **Current State**: Signal handlers exist but no actions are implemented
- **Impact**: Medium - Would enhance operational capabilities
- **Estimate**: Low-Medium effort

**SIGUSR1 - Status Dump**
```go
// TODO: Implement custom action (e.g., status dump)
case syscall.SIGUSR1:
    d.logger.Println("Received SIGUSR1 - custom action not implemented")
```

**SIGUSR2 - Debug Mode Toggle**
```go
// TODO: Implement custom action (e.g., toggle debug mode)
case syscall.SIGUSR2:
    d.logger.Println("Received SIGUSR2 - custom action not implemented")
```

#### Enhanced Health Checks
- **Location**: `internal/daemon/daemon.go:405`
- **Description**: Add comprehensive health checks beyond database connectivity
- **Current State**: Only database health check is implemented
- **Impact**: Medium - Would improve monitoring and diagnostics
- **Estimate**: Medium effort

```go
// TODO: Add more health checks
// - Check scanning workers status
// - Check memory usage
// - Check disk space
// - Check network connectivity
```

### Scheduler Component (`internal/scheduler/scheduler.go`)

#### Scanning Logic Implementation
- **Location**: `internal/scheduler/scheduler.go:448`
- **Description**: Implement actual scanning logic integration in the scheduler
- **Current State**: Placeholder logic that only logs what would be scanned
- **Impact**: Medium - Required for automated scanning functionality
- **Estimate**: High effort

```go
// TODO: Implement actual scanning logic here
// This would integrate with the existing scan functionality
func (s *Scheduler) processHostsForScanning(ctx context.Context, hosts []*db.Host, config *ScanJobConfig) {
    // Currently just logs, needs real implementation
}
```

## Implementation Guidelines

### For Configuration Reload
1. Add configuration validation
2. Implement hot-reload mechanism
3. Update logging configuration
4. Notify components of config changes
5. Add proper error handling

### For Database Reconnection
1. Implement exponential backoff
2. Add connection pooling improvements
3. Log reconnection attempts
4. Update health check status
5. Notify monitoring systems

### For Signal Handlers
1. **SIGUSR1**: Implement status dump including:
   - Current scan status
   - Database connection status
   - Memory usage
   - Active workers count
   - Recent error summary

2. **SIGUSR2**: Implement debug toggle including:
   - Runtime log level changes
   - Detailed request/response logging
   - Performance metrics collection

### For Health Checks
1. Add worker pool status monitoring
2. Implement resource usage checks
3. Add external dependency checks
4. Create health check endpoint for monitoring
5. Implement alerting thresholds

### For Scheduler Scanning
1. Integrate with existing scanning engine
2. Add proper error handling and retry logic
3. Implement scan result processing
4. Add progress tracking
5. Create scan job management interface

## Notes

- All TODO items should be tracked as GitHub issues when work begins
- Consider breaking larger items into smaller, manageable tasks
- Ensure proper testing is implemented for each completed TODO
- Update this file as items are completed or new ones are identified
- Consider API compatibility when implementing changes

## Completed Items

*This section will be updated as TODO items are resolved*

---

Last Updated: August 17, 2025
Generated during refactoring analysis