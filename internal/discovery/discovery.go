package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

// Engine handles network discovery operations
type Engine struct {
	db          *db.DB
	concurrency int
	timeout     time.Duration
}

// Config represents discovery configuration
type Config struct {
	Network     string        `json:"network"`
	Method      string        `json:"method"`
	DetectOS    bool          `json:"detect_os"`
	Timeout     time.Duration `json:"timeout"`
	Concurrency int           `json:"concurrency"`
}

// Result represents a discovery result for a single host
type Result struct {
	IPAddress    net.IP
	Status       string
	ResponseTime time.Duration
	OSInfo       *db.OSFingerprint
	Method       string
	Error        error
}

// NewEngine creates a new discovery engine
func NewEngine(database *db.DB) *Engine {
	return &Engine{
		db:          database,
		concurrency: 50,
		timeout:     3 * time.Second,
	}
}

// SetConcurrency sets the number of concurrent discovery operations
func (e *Engine) SetConcurrency(concurrency int) {
	e.concurrency = concurrency
}

// SetTimeout sets the timeout for individual host discovery
func (e *Engine) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

// Discover performs network discovery on the specified network
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

// runDiscovery executes the actual discovery process
func (e *Engine) runDiscovery(ctx context.Context, job *db.DiscoveryJob, config Config) {
	defer func() {
		now := time.Now()
		job.CompletedAt = &now
		if job.Status == db.DiscoveryJobStatusRunning {
			job.Status = db.DiscoveryJobStatusCompleted
		}
		if err := e.saveDiscoveryJob(ctx, job); err != nil {
			fmt.Printf("Error saving discovery job completion: %v\n", err)
		}
	}()

	// Generate host list
	hosts := e.generateHostList(job.Network.IPNet)
	if len(hosts) == 0 {
		job.Status = db.DiscoveryJobStatusFailed
		return
	}

	// Set up concurrency control
	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = e.concurrency
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = e.timeout
	}

	// Channel for results
	results := make(chan Result, len(hosts))

	// Worker pool
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)

	// Start workers
	for _, host := range hosts {
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			result := e.discoverHost(ctx, ip, config.Method, config.DetectOS, timeout)
			results <- result
		}(host)
	}

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	hostsFound := 0
	hostsResponsive := 0

	for result := range results {
		if result.Error != nil {
			continue
		}

		hostsFound++
		if result.Status == db.HostStatusUp {
			hostsResponsive++

			// Save or update host in database
			if err := e.saveDiscoveredHost(ctx, result, job.ID); err != nil {
				// Log error but continue processing
				fmt.Printf("Error saving host %s: %v\n", result.IPAddress, err)
			}
		}
	}

	job.HostsDiscovered = hostsFound
	job.HostsResponsive = hostsResponsive
}

// discoverHost discovers a single host
func (e *Engine) discoverHost(ctx context.Context, ip net.IP, method string, detectOS bool, timeout time.Duration) Result {
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

// pingHost discovers a host using ICMP ping
func (e *Engine) pingHost(ctx context.Context, ip net.IP, detectOS bool, timeout time.Duration) Result {
	result := Result{
		IPAddress: ip,
		Method:    db.DiscoveryMethodPing,
		Status:    db.HostStatusDown,
	}

	start := time.Now()

	// Use system ping command
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", fmt.Sprintf("%.0f", timeout.Seconds()*1000), ip.String())
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

// arpHost discovers a host using ARP
func (e *Engine) arpHost(ctx context.Context, ip net.IP, timeout time.Duration) Result {
	result := Result{
		IPAddress: ip,
		Method:    db.DiscoveryMethodARP,
		Status:    db.HostStatusDown,
	}

	start := time.Now()

	// Use system arp command
	cmd := exec.CommandContext(ctx, "arp", "-n", ip.String())
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

// tcpHost discovers a host using TCP connect
func (e *Engine) tcpHost(ctx context.Context, ip net.IP, timeout time.Duration) Result {
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
			conn.Close()
			result.Status = db.HostStatusUp
			result.ResponseTime = time.Since(start)
			break
		}
	}

	return result
}

// detectOS performs OS detection on a discovered host
func (e *Engine) detectOS(ctx context.Context, ip net.IP, timeout time.Duration) *db.OSFingerprint {
	// Use nmap for OS detection
	cmd := exec.CommandContext(ctx, "nmap", "-O", "--osscan-guess",
		fmt.Sprintf("--host-timeout=%ds", int(timeout.Seconds())),
		"-n", ip.String())

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	return e.parseNmapOS(string(output))
}

// parseNmapOS parses nmap OS detection output
func (e *Engine) parseNmapOS(output string) *db.OSFingerprint {
	lines := strings.Split(output, "\n")

	var osInfo *db.OSFingerprint

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for OS details
		if strings.HasPrefix(line, "Running:") {
			osInfo = &db.OSFingerprint{
				Method:  "nmap_os_detection",
				Details: make(map[string]interface{}),
			}

			// Parse running OS line
			running := strings.TrimPrefix(line, "Running:")
			running = strings.TrimSpace(running)

			// Extract OS family and details
			if strings.Contains(strings.ToLower(running), "windows") {
				osInfo.Family = db.OSFamilyWindows
				osInfo.Name = e.extractWindowsVersion(running)
			} else if strings.Contains(strings.ToLower(running), "linux") {
				osInfo.Family = db.OSFamilyLinux
				osInfo.Name = e.extractLinuxVersion(running)
			} else if strings.Contains(strings.ToLower(running), "mac") || strings.Contains(strings.ToLower(running), "darwin") {
				osInfo.Family = db.OSFamilyMacOS
				osInfo.Name = running
			} else if strings.Contains(strings.ToLower(running), "freebsd") {
				osInfo.Family = db.OSFamilyFreeBSD
				osInfo.Name = running
			} else {
				osInfo.Family = db.OSFamilyUnknown
				osInfo.Name = running
			}

			osInfo.Details["running"] = running
		}

		// Look for OS CPE information
		if strings.Contains(line, "OS CPE:") {
			if osInfo != nil {
				cpe := strings.TrimSpace(strings.Split(line, "OS CPE:")[1])
				osInfo.Details["cpe"] = cpe
			}
		}

		// Look for aggressive OS guesses with confidence
		if strings.Contains(line, "%") && (strings.Contains(line, "Windows") || strings.Contains(line, "Linux") || strings.Contains(line, "Mac")) {
			if osInfo == nil {
				osInfo = &db.OSFingerprint{
					Method:  "nmap_os_detection",
					Details: make(map[string]interface{}),
				}
			}

			// Extract confidence percentage
			re := regexp.MustCompile(`(\d+)%`)
			if matches := re.FindStringSubmatch(line); len(matches) > 1 {
				if confidence, err := strconv.Atoi(matches[1]); err == nil {
					osInfo.Confidence = confidence
				}
			}

			// Store the guess
			osInfo.Details["os_guess"] = line
		}
	}

	return osInfo
}

// extractWindowsVersion extracts Windows version from nmap output
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

// extractLinuxVersion extracts Linux version from nmap output
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

// extractPingTime extracts response time from ping output
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

// generateHostList generates a list of IP addresses from a network
func (e *Engine) generateHostList(network net.IPNet) []net.IP {
	var hosts []net.IP

	// Calculate network size
	ones, bits := network.Mask.Size()
	if bits-ones > 16 { // Limit to /16 or smaller networks
		return hosts
	}

	// Generate all possible IPs in the network
	ip := network.IP.Mask(network.Mask)
	for {
		if network.Contains(ip) && !ip.Equal(network.IP) && !isBroadcast(ip, network) {
			hosts = append(hosts, net.ParseIP(ip.String()))
		}

		// Increment IP
		if !incrementIP(ip) {
			break
		}

		// Check if we've gone beyond the network
		if !network.Contains(ip) {
			break
		}
	}

	return hosts
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) bool {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return true
		}
	}
	return false
}

// isBroadcast checks if an IP is the broadcast address for the network
func isBroadcast(ip net.IP, network net.IPNet) bool {
	broadcast := make(net.IP, len(network.IP))
	copy(broadcast, network.IP)

	for i := 0; i < len(broadcast); i++ {
		broadcast[i] |= ^network.Mask[i]
	}

	return ip.Equal(broadcast)
}

// saveDiscoveryJob saves or updates a discovery job in the database
func (e *Engine) saveDiscoveryJob(ctx context.Context, job *db.DiscoveryJob) error {
	query := `
		INSERT INTO discovery_jobs (id, network, method, started_at, completed_at, hosts_discovered, hosts_responsive, status, created_at)
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

// saveDiscoveredHost saves or updates a discovered host in the database
func (e *Engine) saveDiscoveredHost(ctx context.Context, result Result, jobID uuid.UUID) error {
	// Check if host already exists
	var existingHost db.Host
	query := `SELECT id, discovery_count FROM hosts WHERE ip_address = $1`
	err := e.db.QueryRowContext(ctx, query, db.IPAddr{IP: result.IPAddress}).Scan(&existingHost.ID, &existingHost.DiscoveryCount)

	now := time.Now()
	responseTimeMS := int(result.ResponseTime.Milliseconds())

	if err != nil {
		// Host doesn't exist, create new one
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
			host.SetOSFingerprint(result.OSInfo)
		}

		return e.insertHost(ctx, &host)
	} else {
		// Host exists, update it
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
			updateQuery += `, os_family = $6, os_name = $7, os_version = $8, os_confidence = $9, os_detected_at = $10, os_method = $11, os_details = $12`

			var detailsJSON []byte
			if result.OSInfo.Details != nil {
				detailsJSON, _ = json.Marshal(result.OSInfo.Details)
			}

			args = append(args, result.OSInfo.Family, result.OSInfo.Name, result.OSInfo.Version,
				result.OSInfo.Confidence, now, result.OSInfo.Method, detailsJSON)
		}

		updateQuery += " WHERE id = $1"

		_, err = e.db.ExecContext(ctx, updateQuery, args...)
		return err
	}
}

// insertHost inserts a new host into the database
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
