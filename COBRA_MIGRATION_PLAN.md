# Cobra + Viper Migration Plan

## Executive Summary

Migrate from manual CLI argument parsing to Cobra/Viper framework to resolve current parsing issues, improve maintainability, and provide better user experience.

**Current Issues:**
- Schedule command argument parsing failures
- PostgreSQL array scanning display issues (CLI-related)
- Manual flag parsing complexity leading to bugs
- Poor help text generation
- Single 1000+ line main.go file

**Benefits of Migration:**
- Automatic help generation and validation
- Better error messages and user experience
- Cleaner, maintainable code architecture
- Built-in shell completion
- Robust configuration management with Viper

## Phase 1A: Cobra/Viper Migration (1-2 weeks)

This migration should be completed **before** Phase 1 (Enhanced Scanning Engine) as it will:
1. Fix immediate CLI parsing issues
2. Provide better foundation for new commands
3. Make adding service detection commands much easier

## Current vs. Future Architecture

### Current Structure
```
cmd/scanorama/main.go (1000+ lines)
├── Manual argument parsing with switch statements
├── Custom configuration loading
├── Mixed command logic and parsing
└── Manual help text generation
```

### Target Structure
```
cmd/
├── root.go           # Root command and global flags
├── discover.go       # scanorama discover
├── scan.go          # scanorama scan
├── hosts.go         # scanorama hosts
├── profiles.go      # scanorama profiles  
├── schedule.go      # scanorama schedule
├── daemon.go        # scanorama daemon
└── version.go       # scanorama version

internal/config/
├── config.go        # Enhanced with Viper
└── viper.go         # Viper configuration binding
```

## Migration Steps

### Step 1: Add Dependencies (30 minutes)

```bash
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
```

Update `go.mod`:
```go
require (
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    // ... existing dependencies
)
```

### Step 2: Create Root Command (2 hours)

**File: `cmd/root.go`**
```go
package cmd

import (
    "fmt"
    "os"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var (
    cfgFile string
    verbose bool
)

var rootCmd = &cobra.Command{
    Use:   "scanorama",
    Short: "Advanced Network Scanner",
    Long: `Scanorama is a comprehensive network scanning and discovery tool 
designed for continuous network monitoring with OS-aware scanning capabilities.`,
    Version: getVersion(),
}

func Execute() {
    err := rootCmd.Execute()
    if err != nil {
        os.Exit(1)
    }
}

func init() {
    cobra.OnInitialize(initConfig)
    
    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
    
    viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

func initConfig() {
    if cfgFile != "" {
        viper.SetConfigFile(cfgFile)
    } else {
        viper.SetConfigName("config")
        viper.SetConfigType("yaml")
        viper.AddConfigPath(".")
    }

    viper.AutomaticEnv()

    if err := viper.ReadInConfig(); err == nil {
        if verbose {
            fmt.Println("Using config file:", viper.ConfigFileUsed())
        }
    }
}
```

### Step 3: Migrate Discovery Command (3 hours)

**File: `cmd/discover.go`**
```go
package cmd

import (
    "fmt"
    "github.com/spf13/cobra"
    "github.com/anstrom/scanorama/internal/discovery"
)

var discoverCmd = &cobra.Command{
    Use:   "discover [network]",
    Short: "Perform network discovery",
    Long: `Discover active hosts on the specified network using various methods
like ping sweeps, ARP discovery, or TCP probes.`,
    Example: `  scanorama discover 192.168.1.0/24
  scanorama discover 10.0.0.0/8 --detect-os
  scanorama discover --all-networks --method arp`,
    Args: cobra.RangeArgs(0, 1),
    Run:  runDiscovery,
}

var (
    discoverDetectOS    bool
    discoverAllNetworks bool
    discoverMethod      string
    discoverTimeout     int
)

func init() {
    rootCmd.AddCommand(discoverCmd)
    
    discoverCmd.Flags().BoolVar(&discoverDetectOS, "detect-os", false, "Enable OS detection")
    discoverCmd.Flags().BoolVar(&discoverAllNetworks, "all-networks", false, "Scan all local networks")
    discoverCmd.Flags().StringVar(&discoverMethod, "method", "tcp", "Discovery method (tcp, ping, arp)")
    discoverCmd.Flags().IntVar(&discoverTimeout, "timeout", 30, "Timeout in seconds")
}

func runDiscovery(cmd *cobra.Command, args []string) {
    // Validate arguments
    if !discoverAllNetworks && len(args) == 0 {
        cmd.Help()
        fmt.Println("\nError: network argument required when --all-networks is not specified")
        os.Exit(1)
    }
    
    // Implementation from current runDiscovery function
    // ... (convert existing logic)
}
```

### Step 4: Migrate Scan Command (3 hours)

**File: `cmd/scan.go`**
```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/anstrom/scanorama/internal/scan"
)

var scanCmd = &cobra.Command{
    Use:   "scan",
    Short: "Scan hosts for open ports and services",
    Long: `Scan discovered hosts or specific targets for open ports,
running services, and other network information.`,
    Example: `  scanorama scan --live-hosts
  scanorama scan --targets 192.168.1.10-20
  scanorama scan --targets host --type version`,
    Run: runScan,
}

var (
    scanTargets   string
    scanLiveHosts bool
    scanPorts     string
    scanType      string
    scanProfile   string
    scanTimeout   int
)

func init() {
    rootCmd.AddCommand(scanCmd)
    
    scanCmd.Flags().StringVar(&scanTargets, "targets", "", "Comma-separated list of targets")
    scanCmd.Flags().BoolVar(&scanLiveHosts, "live-hosts", false, "Scan only discovered live hosts")
    scanCmd.Flags().StringVar(&scanPorts, "ports", "22,80,443,8080,8443", "Ports to scan")
    scanCmd.Flags().StringVar(&scanType, "type", "connect", "Scan type (connect, syn, version, etc.)")
    scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scan profile to use")
    scanCmd.Flags().IntVar(&scanTimeout, "timeout", 300, "Scan timeout in seconds")
    
    // Make targets and live-hosts mutually exclusive
    scanCmd.MarkFlagsMutuallyExclusive("targets", "live-hosts")
}
```

### Step 5: Migrate Remaining Commands (4 hours)

Continue with:
- `cmd/hosts.go` - Host management commands
- `cmd/profiles.go` - Profile management  
- `cmd/schedule.go` - **This will fix the current parsing issues**
- `cmd/daemon.go` - Daemon mode
- `cmd/version.go` - Version command

### Step 6: Enhanced Configuration with Viper (2 hours)

**File: `internal/config/viper.go`**
```go
package config

import (
    "github.com/spf13/viper"
)

// BindViperConfig sets up Viper configuration binding
func BindViperConfig() error {
    // Database configuration
    viper.SetDefault("database.host", "localhost")
    viper.SetDefault("database.port", 5432)
    viper.SetDefault("database.database", "scanorama")
    viper.SetDefault("database.username", "scanorama")
    viper.SetDefault("database.ssl_mode", "require")
    
    // Scanning configuration
    viper.SetDefault("scanning.worker_pool_size", 10)
    viper.SetDefault("scanning.default_scan_type", "connect")
    viper.SetDefault("scanning.default_ports", "22,80,443,8080,8443")
    viper.SetDefault("scanning.max_concurrent_targets", 100)
    
    // Logging configuration
    viper.SetDefault("logging.level", "info")
    viper.SetDefault("logging.format", "text")
    viper.SetDefault("logging.output", "stdout")
    
    return nil
}

// LoadFromViper creates a Config from Viper settings
func LoadFromViper() (*Config, error) {
    var cfg Config
    
    err := viper.Unmarshal(&cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }
    
    return &cfg, nil
}
```

### Step 7: Update Main Function (1 hour)

**File: `cmd/scanorama/main.go`** (Simplified)
```go
package main

import (
    "github.com/anstrom/scanorama/cmd"
)

var (
    version   = "dev"
    commit    = "none"
    buildTime = "unknown"
)

func main() {
    cmd.Execute()
}

func getVersion() string {
    return fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildTime)
}
```

## Migration Benefits

### 1. Fixes Current Issues

**Before (Schedule Command):**
```go
// Current broken parsing
func handleScheduleAddDiscovery(args []string) {
    if len(args) < 3 {  // This logic is fragile
        fmt.Println("Usage: schedule add-discovery...")
        return
    }
    // Manual parsing prone to errors
}
```

**After (Schedule Command):**
```go
var scheduleAddDiscoveryCmd = &cobra.Command{
    Use:   "add-discovery [name] [cron] [network]",
    Short: "Add scheduled discovery job",
    Args:  cobra.ExactArgs(3),  // Automatic validation
    Run:   runScheduleAddDiscovery,
}
```

### 2. Better User Experience

**Rich Help System:**
```bash
$ ./scanorama scan --help
Scan hosts for open ports and services

Usage:
  scanorama scan [flags]

Examples:
  scanorama scan --live-hosts
  scanorama scan --targets 192.168.1.10-20
  scanorama scan --targets host --type version

Flags:
      --live-hosts           Scan only discovered live hosts
      --ports string         Ports to scan (default "22,80,443,8080,8443")
      --profile string       Scan profile to use
      --targets string       Comma-separated list of targets
      --timeout int          Scan timeout in seconds (default 300)
      --type string          Scan type (default "connect")

Global Flags:
      --config string   config file (default is ./config.yaml)
  -v, --verbose         verbose output
```

**Shell Completion:**
```bash
# Enable bash completion
./scanorama completion bash > /etc/bash_completion.d/scanorama

# Tab completion now works
./scanorama sc<TAB>  → scan schedule
./scanorama scan --<TAB>  → shows all available flags
```

### 3. Configuration Management

**Environment Variable Binding:**
```bash
# Automatic environment variable support
export SCANORAMA_DATABASE_HOST=production-db
export SCANORAMA_SCANNING_WORKER_POOL_SIZE=20
export SCANORAMA_VERBOSE=true

./scanorama discover 10.0.0.0/8  # Uses env vars automatically
```

**Multiple Config Formats:**
```yaml
# config.yaml
database:
  host: localhost
  port: 5432
  
scanning:
  worker_pool_size: 15
  default_ports: "22,80,443,3389"
```

## Implementation Timeline

### Week 1: Core Migration
- **Day 1-2**: Add dependencies, create root command
- **Day 3-4**: Migrate discover and scan commands
- **Day 5**: Migrate hosts and profiles commands

### Week 2: Complete Migration  
- **Day 1-2**: Migrate schedule and daemon commands (fixes parsing issues)
- **Day 3**: Enhanced Viper configuration
- **Day 4**: Testing and refinement
- **Day 5**: Documentation and shell completion

## Risk Mitigation

### 1. Incremental Migration
- Keep old main.go as backup until migration complete
- Test each command as it's migrated
- Maintain backward compatibility during transition

### 2. Testing Strategy
```bash
# Test each migrated command
make test
./scanorama discover --help
./scanorama scan --help
# Verify all existing functionality works
```

### 3. Feature Parity Check
- [ ] All current commands work identically
- [ ] All current flags are supported  
- [ ] Error messages are as good or better
- [ ] Performance is maintained

## Post-Migration Benefits

### 1. Easier Feature Addition
Adding new scanning types becomes trivial:
```go
scanCmd.Flags().StringVar(&scanVulnCheck, "vuln-check", "", "Vulnerability check type")
```

### 2. Better Testing
```go
func TestScanCommand(t *testing.T) {
    cmd := &cobra.Command{}
    cmd.SetArgs([]string{"--targets", "localhost", "--ports", "80"})
    // Test command directly
}
```

### 3. Plugin Architecture Ready
Cobra makes it easy to add plugin commands later:
```go
// Future plugin support
rootCmd.AddCommand(pluginManager.GetCommands()...)
```

## Success Criteria

- [ ] All current functionality works identically
- [ ] Schedule command parsing issues resolved
- [ ] Help system is comprehensive and user-friendly
- [ ] Shell completion works for bash/zsh
- [ ] Configuration management is more flexible
- [ ] Codebase is more maintainable with clear separation
- [ ] CI tests pass with new CLI structure
- [ ] Performance is maintained or improved

## Next Steps After Migration

With Cobra/Viper in place, Phase 1 (Enhanced Scanning Engine) becomes much easier:

1. **Service Detection Commands** - Easy to add with Cobra
2. **Advanced Reporting** - Better flag handling for output formats
3. **Configuration Management** - Viper makes complex configs simple
4. **Plugin System** - Cobra provides excellent plugin foundations

**Estimated Timeline: 1-2 weeks**  
**Recommended Priority: High** (do this before Phase 1 service detection work)

This migration will solve the immediate CLI issues and provide a much better foundation for all future development.