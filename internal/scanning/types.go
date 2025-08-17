package scanning

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// Port validation constants.
	expectedPortRangeParts = 2
)

// ScanError represents error types for scan operations.
type ScanError struct {
	Op   string // Operation that failed
	Err  error  // Original error
	Host string // Host where the error occurred, if applicable
	Port uint16 // Port where the error occurred, if applicable
}

func (e *ScanError) Error() string {
	if e.Host != "" && e.Port > 0 {
		return fmt.Sprintf("%s failed for %s:%d: %v", e.Op, e.Host, e.Port, e.Err)
	}
	if e.Host != "" {
		return fmt.Sprintf("%s failed for %s: %v", e.Op, e.Host, e.Err)
	}
	return fmt.Sprintf("%s failed: %v", e.Op, e.Err)
}

func (e *ScanError) Unwrap() error {
	return e.Err
}

// ScanConfig represents the configuration for a network scan.
type ScanConfig struct {
	// Targets is a list of targets to scan (IPs, hostnames, CIDR ranges)
	Targets []string
	// Ports specifies which ports to scan (e.g., "80,443" or "1-1000")
	Ports string
	// ScanType determines the type of scan: "connect", "syn", or "version"
	ScanType string
	// TimeoutSec specifies scan timeout in seconds (0 = default timeout)
	TimeoutSec int
	// Concurrency specifies the number of concurrent scans (0 = auto)
	Concurrency int
	// RetryCount specifies the number of retry attempts for failed scans
	RetryCount int
	// RetryDelay specifies the delay between retries
	RetryDelay time.Duration
}

// Validate checks if the scan configuration is valid.
func (c *ScanConfig) Validate() error {
	if len(c.Targets) == 0 {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("no targets specified")}
	}
	if c.Ports == "" {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("no ports specified")}
	}
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"version":       true,
		"aggressive":    true,
		"stealth":       true,
		"comprehensive": true,
	}
	if !validScanTypes[c.ScanType] {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid scan type: %s", c.ScanType)}
	}

	return c.validatePorts()
}

// validatePorts validates the port specification.
func (c *ScanConfig) validatePorts() error {
	parts := strings.Split(c.Ports, ",")
	for _, part := range parts {
		if err := c.validatePortPart(part); err != nil {
			return err
		}
	}
	return nil
}

// validatePortPart validates a single port or port range.
func (c *ScanConfig) validatePortPart(part string) error {
	if strings.Contains(part, "-") {
		return c.validatePortRange(part)
	}
	return c.validateSinglePort(part)
}

// validatePortRange validates a port range (e.g., "80-100").
func (c *ScanConfig) validatePortRange(part string) error {
	rangeParts := strings.Split(part, "-")
	if len(rangeParts) != expectedPortRangeParts {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid port range format: %s", part)}
	}

	start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
	if err != nil {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid start port: %s", rangeParts[0])}
	}
	end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
	if err != nil {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid end port: %s", rangeParts[1])}
	}

	if start < 0 || start > 65535 || end < 0 || end > 65535 {
		return &ScanError{
			Op:  "validate config",
			Err: fmt.Errorf("invalid port range: %s (must be 0-65535)", part),
		}
	}
	if start > end {
		return &ScanError{
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
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid port: %s", part)}
	}
	if port < 0 || port > 65535 {
		return &ScanError{Op: "validate config", Err: fmt.Errorf("invalid port: %d (must be 0-65535)", port)}
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
