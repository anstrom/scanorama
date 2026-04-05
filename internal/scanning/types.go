package scanning

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// Port validation constants.
	expectedPortRangeParts = 2
)

// ExecError represents error types for scan operations.
type ExecError struct {
	Op   string // Operation that failed
	Err  error  // Original error
	Host string // Host where the error occurred, if applicable
	Port uint16 // Port where the error occurred, if applicable
}

func (e *ExecError) Error() string {
	if e.Host != "" && e.Port > 0 {
		return fmt.Sprintf("%s failed for %s:%d: %v", e.Op, e.Host, e.Port, e.Err)
	}
	if e.Host != "" {
		return fmt.Sprintf("%s failed for %s: %v", e.Op, e.Host, e.Err)
	}
	return fmt.Sprintf("%s failed: %v", e.Op, e.Err)
}

func (e *ExecError) Unwrap() error {
	return e.Err
}

// ScanConfig represents the configuration for a network scan.
type ScanConfig struct {
	// Targets is a list of targets to scan (IPs, hostnames, CIDR ranges)
	Targets []string
	// Ports specifies which ports to scan (e.g., "80,443" or "1-1000")
	Ports string
	// ScanType determines the type of scan: "connect", "syn", "ack", "udp", "aggressive", or "comprehensive"
	ScanType string
	// Timing sets the nmap timing template explicitly: "paranoid", "polite", "normal", "aggressive", "insane".
	// When set, this takes precedence over any timing derived from TimeoutSec.
	Timing string
	// OSDetection enables nmap OS fingerprinting (-O)
	OSDetection bool
	// TimeoutSec specifies scan timeout in seconds (0 = default timeout)
	TimeoutSec int
	// Concurrency specifies the number of concurrent scans (0 = auto)
	Concurrency int
	// RetryCount specifies the number of retry attempts for failed scans
	RetryCount int
	// RetryDelay specifies the delay between retries
	RetryDelay time.Duration
	// ScanID is the UUID of the existing scan_jobs row that triggered this scan.
	// When set, storeScanResults reuses this ID so that port_scans rows are
	// linked to the same UUID exposed to the API client via GetScanResults.
	// When nil a fresh UUID is generated (CLI / legacy path).
	ScanID *uuid.UUID
}

// Validate checks if the scan configuration is valid.
func (c *ScanConfig) Validate() error {
	if len(c.Targets) == 0 {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("no targets specified")}
	}
	if c.Ports == "" {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("no ports specified")}
	}
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"ack":           true,
		"udp":           true,
		"aggressive":    true,
		"comprehensive": true,
	}
	if !validScanTypes[c.ScanType] {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid scan type: %s", c.ScanType)}
	}

	if err := c.validateTargets(); err != nil {
		return err
	}
	return c.validatePorts()
}

// validateTargets checks that no target looks like a nmap flag (starts with '-').
// The HTTP handler already validates targets as IPs/CIDRs; this defense-in-depth
// check also covers CLI and internal callers that bypass the handler.
func (c *ScanConfig) validateTargets() error {
	for _, target := range c.Targets {
		if strings.HasPrefix(target, "-") {
			return &ExecError{
				Op:  "validate config",
				Err: fmt.Errorf("invalid target %q: targets must not start with '-'", target),
			}
		}
	}
	return nil
}

// validatePorts validates the port specification.
// Supports nmap mixed-protocol syntax e.g. "T:22,80,U:53,161".
func (c *ScanConfig) validatePorts() error {
	parts := strings.Split(c.Ports, ",")
	for _, part := range parts {
		if err := c.validatePortPart(part); err != nil {
			return err
		}
	}
	return nil
}

// validatePortPart validates a single port or port range, stripping any
// leading protocol prefix (T: or U:) before parsing.
func (c *ScanConfig) validatePortPart(part string) error {
	// Strip nmap protocol prefixes T: and U:
	for _, prefix := range []string{"T:", "U:", "t:", "u:"} {
		if strings.HasPrefix(part, prefix) {
			part = part[len(prefix):]
			break
		}
	}
	// A prefix-only token (e.g. bare "T:") after stripping is empty — skip it.
	if part == "" {
		return nil
	}
	if strings.Contains(part, "-") {
		return c.validatePortRange(part)
	}
	return c.validateSinglePort(part)
}

// validatePortRange validates a port range (e.g., "80-100").
func (c *ScanConfig) validatePortRange(part string) error {
	rangeParts := strings.Split(part, "-")
	if len(rangeParts) != expectedPortRangeParts {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid port range format: %s", part)}
	}

	start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
	if err != nil {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid start port: %s", rangeParts[0])}
	}
	end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
	if err != nil {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid end port: %s", rangeParts[1])}
	}

	if start < 0 || start > 65535 || end < 0 || end > 65535 {
		return &ExecError{
			Op:  "validate config",
			Err: fmt.Errorf("invalid port range: %s (must be 0-65535)", part),
		}
	}
	if start > end {
		return &ExecError{
			Op:  "validate config",
			Err: fmt.Errorf("invalid port range: start port must be less than end port"),
		}
	}
	return nil
}

// validateSinglePort validates a single port.
func (c *ScanConfig) validateSinglePort(part string) error {
	port, err := strconv.Atoi(strings.TrimSpace(part))
	if err != nil {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid port: %s", part)}
	}
	if port < 0 || port > 65535 {
		return &ExecError{Op: "validate config", Err: fmt.Errorf("invalid port: %d (must be 0-65535)", port)}
	}
	return nil
}

// ScanResult contains the complete results of a network scan.
type ScanResult struct {
	// Hosts contains all scanned hosts and their findings
	Hosts []Host
	// Stats contains summary statistics about the scan
	Stats HostStats
	// StartTime is when the scan started
	StartTime time.Time
	// EndTime is when the scan completed
	EndTime time.Time
	// Duration is how long the scan took
	Duration time.Duration
	// Error is any error that occurred during the scan
	Error error
}

// NewScanResult creates a new scan result with the current time as start time.
func NewScanResult() *ScanResult {
	return &ScanResult{
		StartTime: time.Now(),
		Hosts:     make([]Host, 0),
	}
}

// Complete marks the scan as complete and calculates duration.
func (r *ScanResult) Complete() {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
}

// Host represents a scanned host and its findings.
type Host struct {
	// Address is the IP address or hostname of the scanned host
	Address string
	// Status indicates whether the host is "up" or "down"
	Status string
	// Ports contains information about all scanned ports
	Ports []Port
	// OSName is the detected operating system name, if available
	OSName string
	// OSFamily is the detected operating system family (e.g. "Linux", "Windows")
	OSFamily string
	// OSVersion is the detected operating system version, if available
	OSVersion string
	// OSAccuracy is the confidence percentage (0-100) of the OS detection
	OSAccuracy int
}

// Port represents the scan results for a single port.
type Port struct {
	// Number is the port number (1-65535)
	Number uint16
	// Protocol is the transport protocol ("tcp" or "udp")
	Protocol string
	// State indicates whether the port is "open", "closed", or "filtered"
	State string
	// Service is the name of the detected service, if any
	Service string
	// Version is the version of the detected service, if available
	Version string
	// ServiceInfo contains additional service details, if available
	ServiceInfo string
}

// HostStats contains summary statistics about a network scan.
type HostStats struct {
	// Up is the number of hosts that were up
	Up int
	// Down is the number of hosts that were down
	Down int
	// Total is the total number of hosts scanned
	Total int
}
