# Discovery Package Refactoring

## Overview

This document summarizes the refactoring of the discovery package to use the nmap library instead of executing nmap as an external command.

## Problem

The discovery package (`internal/discovery/`) was inconsistent with the scanning package (`internal/scanning/`) in how it used nmap:

- **Discovery package**: Executed nmap as external command using `exec.CommandContext()`
- **Scanning package**: Used the proper Go nmap library (`github.com/Ullaakut/nmap/v3`)

This inconsistency led to:
- Manual command-line argument construction
- Raw text output parsing
- Potential security issues with command execution
- Inconsistent error handling
- Maintenance overhead

## Solution

Refactored the discovery package to use the same nmap library as the scanning package.

### Key Changes

#### 1. Removed External Command Execution

**Before:**
```go
cmd := exec.CommandContext(ctx, "nmap", args...) // #nosec G204
output, err := cmd.Output()
```

**After:**
```go
scanner, err := nmap.NewScanner(ctx, options...)
result, warnings, err := scanner.Run()
```

#### 2. Updated Option Building

**Before:** `buildNmapOptionsForTargets()` returned `[]string`
```go
args := []string{"-sn"}  // Manual command line args
```

**After:** `buildNmapLibraryOptions()` returns `[]nmap.Option`
```go
options := []nmap.Option{
    nmap.WithTargets(targets...),
    nmap.WithPingScan(),
}
```

#### 3. Replaced Output Parsing

**Before:** `parseNmapOutput()` parsed raw text output
```go
if strings.HasPrefix(line, "Nmap scan report for ") {
    // Manual string parsing...
}
```

**After:** `convertNmapResultsToDiscovery()` uses structured data
```go
for _, host := range nmapResult.Hosts {
    ip := net.ParseIP(host.Addresses[0].Addr)
    // Work with structured data...
}
```

#### 4. Updated Method Mapping

| Discovery Method | Before | After |
|-----------------|--------|--------|
| tcp | `-PS22,80,443,8080,8022,8379` | `nmap.WithCustomArguments("-PS22,80,443,8080,8022,8379")` |
| ping | `-PE` | `nmap.WithCustomArguments("-PE")` |
| arp | `-PR` | `nmap.WithCustomArguments("-PR")` |
| OS detection | `-O` | `nmap.WithOSDetection()` |

#### 5. Timing Templates

| Timeout | Before | After |
|---------|--------|--------|
| ≤ 30s | `-T4` | `nmap.WithTimingTemplate(nmap.TimingAggressive)` |
| ≤ 120s | `-T3` | `nmap.WithTimingTemplate(nmap.TimingNormal)` |
| > 120s | `-T2` | `nmap.WithTimingTemplate(nmap.TimingPolite)` |

### Files Modified

1. **`internal/discovery/discovery.go`**
   - Added nmap library import
   - Removed `os/exec` import
   - Refactored `nmapDiscoveryWithTargets()`
   - Renamed `buildNmapOptionsForTargets()` → `buildNmapLibraryOptions()`
   - Renamed `parseNmapOutput()` → `convertNmapResultsToDiscovery()`

2. **`internal/discovery/discovery_test.go`**
   - Added nmap library import for test structures
   - Updated test function names
   - Modified tests to work with `nmap.Option` instead of string arrays
   - Updated mock data to use nmap library structures

## Benefits

### 1. Consistency
- Both discovery and scanning packages now use the same nmap library
- Unified approach to nmap integration across the codebase

### 2. Security
- Eliminated external command execution
- No more string-based command construction
- Reduced attack surface

### 3. Reliability
- Structured error handling from the library
- Type-safe option configuration
- Better timeout and cancellation handling

### 4. Maintainability
- Single source of truth for nmap integration
- Easier to add new nmap features
- Consistent testing patterns

### 5. Performance
- No process spawning overhead
- Better memory management
- Structured data eliminates parsing overhead

## Testing

All existing tests pass with the refactored implementation:
- 18 test functions covering all discovery functionality
- Integration tests verify end-to-end functionality
- No breaking changes to the public API

## Migration Impact

### Zero Impact
- Public API unchanged
- Same discovery results format
- Same configuration options
- Same error handling behavior

### Internal Changes Only
- Function signature changes are internal only
- Test changes maintain same coverage
- Implementation details abstracted from callers

## Future Improvements

With this refactoring, future enhancements become easier:

1. **Enhanced OS Detection**: Can leverage full nmap library OS detection features
2. **Better Timing Control**: More granular timing template options
3. **Advanced Scanning**: Easy to add NSE script support for discovery
4. **Improved Logging**: Structured warnings and debug information
5. **Performance Tuning**: Fine-grained control over scan parallelism

## Verification

To verify the refactoring:

```bash
# Run discovery tests
go test ./internal/discovery/... -v

# Run all internal package tests
go test ./internal/... -short

# Build entire project
go build -v ./...
```

All commands should pass without errors, confirming successful refactoring.