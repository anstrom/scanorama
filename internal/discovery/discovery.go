// Package discovery provides network discovery functionality for scanorama.
// It handles host discovery through various methods like ping scanning,
// and manages the discovery engine and worker processes.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Default discovery configuration values.
	defaultConcurrency    = 50
	defaultTimeoutSeconds = 3
	maxNetworkSizeBits    = 16 // Limit to /16 or smaller networks
)

// Constants for discovery operations.
const (
	MillisecondsPerSecond = 1000
)

// Engine handles network discovery operations.
type Engine struct {
	db          *db.DB
	concurrency int
	timeout     time.Duration
}

// Config represents discovery configuration.
type Config struct {
	Network     string        `json:"network"`
	Method      string        `json:"method"`
	DetectOS    bool          `json:"detect_os"`
	Timeout     time.Duration `json:"timeout"`
	Concurrency int           `json:"concurrency"`
}

// Result represents a discovery result for a single host.
type Result struct {
	IPAddress    net.IP
	Status       string
	ResponseTime time.Duration
	OSInfo       *db.OSFingerprint
	Method       string
	Error        error
}

// NewEngine creates a new discovery engine.
func NewEngine(database *db.DB) *Engine {
	return &Engine{
		db:          database,
		concurrency: defaultConcurrency,
		timeout:     defaultTimeoutSeconds * time.Second,
	}
}

// SetConcurrency sets the number of concurrent discovery operations.
func (e *Engine) SetConcurrency(concurrency int) {
	e.concurrency = concurrency
}

// SetTimeout sets the timeout for individual host discovery.
func (e *Engine) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

// Discover performs network discovery on the specified network.
func (e *Engine) Discover(ctx context.Context, config Config) (*db.DiscoveryJob, error) {
	// Parse network
	_, ipnet, err := net.ParseCIDR(config.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network: %w", err)
	}

	// Create discovery job
	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: *ipnet},
		Method:    config.Method,
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	now := time.Now()
	job.StartedAt = &now

	// Save job to database
	if err := e.saveDiscoveryJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to save discovery job: %w", err)
	}

	// Start discovery in background
	go e.runDiscovery(context.Background(), job, config)

	return job, nil
}

// runDiscovery executes the actual discovery process using nmap.
func (e *Engine) runDiscovery(ctx context.Context, job *db.DiscoveryJob, config Config) {
	defer e.finalizeDiscoveryJob(ctx, job)

	// Use nmap for host discovery
	discoveredHosts, err := e.nmapDiscovery(ctx, job.Network.IPNet.String(), config)
	if err != nil {
		job.Status = db.DiscoveryJobStatusFailed
		fmt.Printf("Discovery failed: %v\n", err)
		return
	}

	// Update job with results
	job.HostsResponsive = len(discoveredHosts)
	job.HostsDiscovered = len(discoveredHosts)
	fmt.Printf("Discovery completed. Found %d hosts.\n", job.HostsDiscovered)
}

// finalizeDiscoveryJob handles the completion and saving of a discovery job.
func (e *Engine) finalizeDiscoveryJob(ctx context.Context, job *db.DiscoveryJob) {
	now := time.Now()
	job.CompletedAt = &now
	if job.Status == db.DiscoveryJobStatusRunning {
		job.Status = db.DiscoveryJobStatusCompleted
	}
	if err := e.saveDiscoveryJob(ctx, job); err != nil {
		fmt.Printf("Error saving discovery job completion: %v\n", err)
	}
}

// setupDiscoveryParameters configures concurrency and timeout values.
func (e *Engine) setupDiscoveryParameters(config Config) (int, time.Duration) {
	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = e.concurrency
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = e.timeout
	}

	return concurrency, timeout
}

// runDiscoveryWorkers is deprecated - replaced by nmapDiscovery.
// Kept for compatibility but no longer used.
func (e *Engine) runDiscoveryWorkers(
	ctx context.Context, hosts []net.IP, config Config, concurrency int, timeout time.Duration,
) []*db.Host {
	// This function is deprecated and replaced by nmapDiscovery
	return []*db.Host{}
}

// createHostFromResult creates a database host entry from a discovery result.
func (e *Engine) createHostFromResult(ctx context.Context, result *Result) (*db.Host, error) {
	// Save or update host in database
	if err := e.saveDiscoveredHost(ctx, result, uuid.Nil); err != nil {
		return nil, fmt.Errorf("failed to save discovered host: %w", err)
	}

	// Create host object for return
	host := &db.Host{
		IPAddress: db.IPAddr{IP: result.IPAddress},
		Status:    result.Status,
	}

	return host, nil
}

// discoverHost discovers a single host.
func (e *Engine) discoverHost(
	ctx context.Context, ip net.IP, method string, detectOS bool, timeout time.Duration,
) Result {
	result := Result{
		IPAddress: ip,
		Method:    method,
		Status:    db.HostStatusDown,
	}

	switch method {
	case db.DiscoveryMethodPing:
		return e.pingHost(ctx, ip, detectOS, timeout)
	case db.DiscoveryMethodARP:
		return e.arpHost(ctx, ip, timeout)
	case db.DiscoveryMethodTCP:
		return e.tcpHost(ctx, ip, timeout)
	default:
		result.Error = fmt.Errorf("unsupported discovery method: %s", method)
		return result
	}
}

// pingHost discovers a host using ICMP ping.
func (e *Engine) pingHost(ctx context.Context, ip net.IP, detectOS bool, timeout time.Duration) Result {
	result := Result{
		IPAddress: ip,
		Method:    db.DiscoveryMethodPing,
		Status:    db.HostStatusDown,
	}

	start := time.Now()

	// Use system ping command
	//nolint:gosec // legitimate use of ping with validated IP
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W",
		fmt.Sprintf("%.0f", timeout.Seconds()*MillisecondsPerSecond), ip.String())
	output, err := cmd.Output()

	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err
		return result
	}

	// Parse ping output
	if strings.Contains(string(output), "1 received") || strings.Contains(string(output), "1 packets received") {
		result.Status = db.HostStatusUp

		// Extract response time from ping output
		if rtt := e.extractPingTime(string(output)); rtt > 0 {
			result.ResponseTime = rtt
		}

		// Perform OS detection if requested
		if detectOS {
			result.OSInfo = e.detectOS(ctx, ip, timeout)
		}
	}

	return result
}

// arpHost discovers a host using ARP.
func (e *Engine) arpHost(ctx context.Context, ip net.IP, _ time.Duration) Result {
	result := Result{
		IPAddress: ip,
		Method:    db.DiscoveryMethodARP,
		Status:    db.HostStatusDown,
	}

	start := time.Now()

	// Use system arp command
	cmd := exec.CommandContext(ctx, "arp", "-n", ip.String()) //nolint:gosec // legitimate use of arp with validated IP
	output, err := cmd.Output()

	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err
		return result
	}

	// Check if ARP entry exists and is not incomplete
	if strings.Contains(string(output), ip.String()) && !strings.Contains(string(output), "incomplete") {
		result.Status = db.HostStatusUp
	}

	return result
}

// tcpHost discovers a host using TCP connect.
func (e *Engine) tcpHost(_ context.Context, ip net.IP, timeout time.Duration) Result {
	result := Result{
		IPAddress: ip,
		Method:    db.DiscoveryMethodTCP,
		Status:    db.HostStatusDown,
	}

	// Common ports to try
	ports := []int{80, 443, 22, 23, 21, 25, 53, 135, 139, 445, 3389}

	for _, port := range ports {
		start := time.Now()

		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip.String(), port), timeout)
		if err == nil {
			_ = conn.Close()
			result.Status = db.HostStatusUp
			result.ResponseTime = time.Since(start)
			break
		}
	}

	return result
}

// detectOS performs OS detection on a discovered host.
func (e *Engine) detectOS(ctx context.Context, ip net.IP, timeout time.Duration) *db.OSFingerprint {
	// Use nmap for OS detection
	//nolint:gosec // legitimate use of nmap with validated IP
	cmd := exec.CommandContext(ctx, "nmap", "-O", "--osscan-guess",
		fmt.Sprintf("--host-timeout=%ds", int(timeout.Seconds())),
		"-n", ip.String())

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	return e.parseNmapOS(string(output))
}

// parseNmapOS parses nmap OS detection output.
func (e *Engine) parseNmapOS(output string) *db.OSFingerprint {
	lines := strings.Split(output, "\n")
	var osInfo *db.OSFingerprint

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Running:") {
			osInfo = e.parseRunningLine(line)
			continue
		}

		if e.isConfidenceLine(line) {
			if osInfo == nil {
				osInfo = e.createBasicOSFingerprint()
			}
			e.parseConfidenceLine(osInfo, line)
		}
	}

	return osInfo
}

// parseRunningLine parses a "Running:" line from nmap output.
func (e *Engine) parseRunningLine(line string) *db.OSFingerprint {
	osInfo := e.createBasicOSFingerprint()

	running := strings.TrimPrefix(line, "Running:")
	running = strings.TrimSpace(running)

	// Extract OS family and details
	runningLower := strings.ToLower(running)
	switch {
	case strings.Contains(runningLower, "windows"):
		osInfo.Family = db.OSFamilyWindows
		osInfo.Name = e.extractWindowsVersion(running)
	case strings.Contains(runningLower, "linux"):
		osInfo.Family = db.OSFamilyLinux
		osInfo.Name = e.extractLinuxVersion(running)
	case strings.Contains(runningLower, "mac") || strings.Contains(runningLower, "darwin"):
		osInfo.Family = db.OSFamilyMacOS
		osInfo.Name = running
	case strings.Contains(runningLower, "freebsd"):
		osInfo.Family = db.OSFamilyFreeBSD
		osInfo.Name = running
	case strings.Contains(runningLower, "openbsd"):
		osInfo.Family = db.OSFamilyUnix
		osInfo.Name = running
	case strings.Contains(runningLower, "netbsd"):
		osInfo.Family = db.OSFamilyUnix
		osInfo.Name = running
	default:
		osInfo.Family = db.OSFamilyUnknown
		osInfo.Name = running
	}

	osInfo.Details["running"] = running
	return osInfo
}

// createBasicOSFingerprint creates a basic OS fingerprint structure.
func (e *Engine) createBasicOSFingerprint() *db.OSFingerprint {
	return &db.OSFingerprint{
		Method:  "nmap_os_detection",
		Details: make(map[string]interface{}),
	}
}

// isConfidenceLine checks if a line contains confidence information.
func (e *Engine) isConfidenceLine(line string) bool {
	return strings.Contains(line, "%") && (strings.Contains(line, "Windows") ||
		strings.Contains(line, "Linux") || strings.Contains(line, "Mac"))
}

// parseConfidenceLine extracts confidence information from a line.
func (e *Engine) parseConfidenceLine(osInfo *db.OSFingerprint, line string) {
	parts := strings.Fields(line)
	for _, part := range parts {
		if strings.Contains(part, "%") {
			confidenceStr := strings.TrimSuffix(part, "%")
			if confidence, err := strconv.Atoi(confidenceStr); err == nil {
				osInfo.Confidence = confidence
				osInfo.Details["confidence_line"] = line
				break
			}
		}
	}
}

// extractWindowsVersion extracts Windows version from nmap output.
func (e *Engine) extractWindowsVersion(running string) string {
	// Common patterns for Windows versions
	patterns := []string{
		`Windows Server 2022`,
		`Windows Server 2019`,
		`Windows Server 2016`,
		`Windows Server 2012`,
		`Windows 11`,
		`Windows 10`,
		`Windows 8`,
		`Windows 7`,
		`Windows Server`,
		`Windows`,
	}

	runningLower := strings.ToLower(running)
	for _, pattern := range patterns {
		if strings.Contains(runningLower, strings.ToLower(pattern)) {
			return pattern
		}
	}

	return "Windows (unknown version)"
}

// extractLinuxVersion extracts Linux version from nmap output.
func (e *Engine) extractLinuxVersion(running string) string {
	// Common patterns for Linux distributions
	patterns := []string{
		`Ubuntu`,
		`CentOS`,
		`Red Hat`,
		`RHEL`,
		`Debian`,
		`SUSE`,
		`Fedora`,
		`Alpine`,
	}

	for _, pattern := range patterns {
		if strings.Contains(running, pattern) {
			return pattern + " Linux"
		}
	}

	return "Linux (unknown distribution)"
}

// extractPingTime extracts response time from ping output.
func (e *Engine) extractPingTime(output string) time.Duration {
	// Look for time=X.Xms pattern
	re := regexp.MustCompile(`time=([0-9.]+)\s*ms`)
	matches := re.FindStringSubmatch(output)

	if len(matches) > 1 {
		if ms, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}

	return 0
}

// nmapDiscovery performs host discovery using nmap.
func (e *Engine) nmapDiscovery(ctx context.Context, network string, config Config) ([]*db.Host, error) {
	// Set up discovery timeout
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}

	// Apply timeout to context
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout*10) // Give nmap more time for network scans
	defer cancel()

	// Build nmap options for host discovery
	options := []nmap.Option{
		nmap.WithTargets(network),
		nmap.WithPingScan(), // Host discovery only, no port scan
	}

	// Add timing based on config
	if timeout <= 5*time.Second {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
	} else if timeout <= 15*time.Second {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingNormal))
	} else {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingPolite))
	}

	// Create and run scanner
	scanner, err := nmap.NewScanner(discoveryCtx, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create nmap scanner: %w", err)
	}

	result, warnings, err := scanner.Run()
	if err != nil {
		return nil, fmt.Errorf("nmap discovery failed: %w", err)
	}

	if warnings != nil && len(*warnings) > 0 {
		log.Printf("Discovery completed with warnings: %v", *warnings)
	}

	// Convert nmap results to our host format
	var discoveredHosts []*db.Host
	for i := range result.Hosts {
		host := &result.Hosts[i]
		if len(host.Addresses) == 0 || host.Status.State != "up" {
			continue
		}

		dbHost := &db.Host{
			ID:              uuid.New(),
			IPAddress:       db.IPAddr{IP: net.ParseIP(host.Addresses[0].Addr)},
			Status:          host.Status.State,
			DiscoveryMethod: &config.Method,
			ResponseTimeMS:  new(int),
		}

		// Try to extract hostname
		if len(host.Hostnames) > 0 {
			hostname := host.Hostnames[0].Name
			dbHost.Hostname = &hostname
		}

		// Try to extract MAC address and vendor
		for _, addr := range host.Addresses {
			if addr.AddrType == "mac" {
				macAddr := db.MACAddr{}
				if err := macAddr.Scan(addr.Addr); err == nil {
					dbHost.MACAddress = &macAddr
					if addr.Vendor != "" {
						dbHost.Vendor = &addr.Vendor
					}
				}
				break
			}
		}

		// Store the host in database
		if err := e.saveDiscoveredHost(context.Background(), &Result{
			IPAddress: dbHost.IPAddress.IP,
			Status:    dbHost.Status,
			Method:    config.Method,
		}, uuid.Nil); err != nil {
			log.Printf("Failed to save discovered host %s: %v", dbHost.IPAddress.String(), err)
			continue
		}

		discoveredHosts = append(discoveredHosts, dbHost)
	}

	return discoveredHosts, nil
}

// saveDiscoveryJob saves or updates a discovery job in the database.
func (e *Engine) saveDiscoveryJob(ctx context.Context, job *db.DiscoveryJob) error {
	query := `
		INSERT INTO discovery_jobs (
			id, network, method, started_at, completed_at,
			hosts_discovered, hosts_responsive, status, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			hosts_discovered = EXCLUDED.hosts_discovered,
			hosts_responsive = EXCLUDED.hosts_responsive,
			status = EXCLUDED.status
	`

	_, err := e.db.ExecContext(ctx, query,
		job.ID, job.Network, job.Method, job.StartedAt, job.CompletedAt,
		job.HostsDiscovered, job.HostsResponsive, job.Status, job.CreatedAt)

	return err
}

// saveDiscoveredHost saves or updates a discovered host in the database.
func (e *Engine) saveDiscoveredHost(ctx context.Context, result *Result, _ uuid.UUID) error {
	// Check if host already exists
	var existingHost db.Host
	query := `SELECT id, discovery_count FROM hosts WHERE ip_address = $1`
	err := e.db.QueryRowContext(ctx, query, db.IPAddr{IP: result.IPAddress}).Scan(
		&existingHost.ID, &existingHost.DiscoveryCount)

	now := time.Now()
	responseTimeMS := int(result.ResponseTime.Milliseconds())

	if err != nil {
		return e.createNewHost(ctx, result, now, responseTimeMS)
	}
	return e.updateExistingHost(ctx, &existingHost, result, now, responseTimeMS)
}

// insertHost inserts a new host into the database.
func (e *Engine) insertHost(ctx context.Context, host *db.Host) error {
	query := `
		INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
						   os_confidence, os_detected_at, os_method, os_details, discovery_method,
						   response_time_ms, discovery_count, ignore_scanning, first_seen, last_seen, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := e.db.ExecContext(ctx, query,
		host.ID, host.IPAddress, host.Hostname, host.MACAddress, host.Vendor,
		host.OSFamily, host.OSName, host.OSVersion, host.OSConfidence, host.OSDetectedAt,
		host.OSMethod, host.OSDetails, host.DiscoveryMethod, host.ResponseTimeMS,
		host.DiscoveryCount, host.IgnoreScanning, host.FirstSeen, host.LastSeen, host.Status)

	return err
}

// createNewHost creates a new host entry in the database.
func (e *Engine) createNewHost(ctx context.Context, result *Result, now time.Time, responseTimeMS int) error {
	host := db.Host{
		ID:              uuid.New(),
		IPAddress:       db.IPAddr{IP: result.IPAddress},
		Status:          result.Status,
		DiscoveryMethod: &result.Method,
		ResponseTimeMS:  &responseTimeMS,
		DiscoveryCount:  1,
		FirstSeen:       now,
		LastSeen:        now,
	}

	// Set OS information if detected
	if result.OSInfo != nil {
		if err := host.SetOSFingerprint(result.OSInfo); err != nil {
			log.Printf("Failed to set OS fingerprint: %v", err)
		}
	}

	return e.insertHost(ctx, &host)
}

// updateExistingHost updates an existing host entry in the database.
func (e *Engine) updateExistingHost(
	ctx context.Context, existingHost *db.Host, result *Result, now time.Time, responseTimeMS int,
) error {
	updateQuery := `
		UPDATE hosts SET
			status = $2,
			last_seen = $3,
			discovery_method = $4,
			response_time_ms = $5,
			discovery_count = discovery_count + 1
	`
	args := []interface{}{existingHost.ID, result.Status, now, result.Method, responseTimeMS}

	// Add OS information if detected
	if result.OSInfo != nil {
		updateQuery += `, os_family = $6, os_name = $7, os_version = $8, os_confidence = $9, ` +
			`os_detected_at = $10, os_method = $11, os_details = $12`

		var detailsJSON []byte
		if result.OSInfo.Details != nil {
			detailsJSON, _ = json.Marshal(result.OSInfo.Details)
		}

		args = append(args, result.OSInfo.Family, result.OSInfo.Name, result.OSInfo.Version,
			result.OSInfo.Confidence, now, result.OSInfo.Method, detailsJSON)
	}

	updateQuery += ` WHERE id = $1`
	_, err := e.db.ExecContext(ctx, updateQuery, args...)
	return err
}
