// Package discovery provides network discovery functionality using nmap.
// This package handles host discovery operations and integrates with the
// database for proper target generation and result storage.
package discovery

import (
    "context"
    "fmt"
    "log"
    "net"
    "strings"
    "time"

    "github.com/Ullaakut/nmap/v3"
    "github.com/google/uuid"

    "github.com/anstrom/scanorama/internal/db"
)

const (
	defaultConcurrency    = 10
	defaultTimeoutSeconds = 60
	maxNetworkSizeBits    = 16
	retryInterval         = 5 * time.Second
	ciConsistencyDelay    = 1 * time.Second
	nullValue             = "NULL"
	maxTimeout            = 300 * time.Second
	minTimeout            = 30 * time.Second
	timeoutMultiplierBase = 6.0
	timeoutMultiplierStep = 2.0
	// SQL error constants
	sqlNoRowsError        = "sql: no rows in result set"
	timeoutMultiplierMax  = 50.0
	timeoutDivisor        = 100.0
	maxHostBits           = 24
	rfc3021NetworkSize    = 31
	singleHostNetworkSize = 32
	minNmapOutputFields   = 5
)

// Engine handles network discovery operations.
type Engine struct {
	db          *db.DB
	concurrency int
	timeout     time.Duration
}

// Config holds discovery configuration parameters.
type Config struct {
	Networks    []string      `json:"networks"`
	Network     string        `json:"network"`
	Method      string        `json:"method"`
	DetectOS    bool          `json:"detect_os"`
	Timeout     time.Duration `json:"timeout"`
	Concurrency int           `json:"concurrency"`
	MaxHosts    int           `json:"max_hosts"`
}

// Result represents a discovery result for a single host.
type Result struct {
	IPAddress    net.IP        `json:"ip_address"`
	Status       string        `json:"status"`
	ResponseTime time.Duration `json:"response_time"`
	OSInfo       string        `json:"os_info"`
	Method       string        `json:"method"`
	Error        string        `json:"error,omitempty"`
}

// NewEngine creates a new discovery engine with the given database.
func NewEngine(database *db.DB) *Engine {
	return &Engine{
		db:          database,
		concurrency: defaultConcurrency,
		timeout:     time.Duration(defaultTimeoutSeconds) * time.Second,
	}
}

// SetConcurrency sets the concurrency level for discovery operations.
func (e *Engine) SetConcurrency(concurrency int) {
	e.concurrency = concurrency
}

// SetTimeout sets the timeout for discovery operations.
func (e *Engine) SetTimeout(timeout time.Duration) {
	e.timeout = timeout
}

// Discover performs network discovery on the specified network.
func (e *Engine) Discover(ctx context.Context, config *Config) (*db.DiscoveryJob, error) {
	// Determine target network
	var network string
	if config.Network != "" {
		network = config.Network
	} else if len(config.Networks) > 0 {
		network = config.Networks[0]
	} else {
		return nil, fmt.Errorf("no network specified for discovery")
	}

	// Parse and validate network
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return nil, fmt.Errorf("invalid network CIDR: %w", err)
	}

	// Check network size limits
	if err := e.validateNetworkSize(*ipnet); err != nil {
		return nil, err
	}

	// Create discovery job
	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: *ipnet},
		Method:    config.Method,
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	// Save initial job state
	if err := e.saveDiscoveryJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to save discovery job: %w", err)
	}

	// Start discovery in background
	go e.runDiscovery(ctx, job, config)

	return job, nil
}

// validateNetworkSize checks if the network is within acceptable size limits.
func (e *Engine) validateNetworkSize(ipnet net.IPNet) error {
	ones, bits := ipnet.Mask.Size()
	if ones < maxNetworkSizeBits {
		return fmt.Errorf("network size too large (/%d), maximum allowed is /%d", ones, maxNetworkSizeBits)
	}
	if bits != singleHostNetworkSize && bits != 128 {
		return fmt.Errorf("unsupported IP version")
	}
	return nil
}

// runDiscovery executes the actual discovery process using nmap.
func (e *Engine) runDiscovery(ctx context.Context, job *db.DiscoveryJob, config *Config) {
	defer e.finalizeDiscoveryJob(ctx, job)

	// Check if context is already canceled
	select {
	case <-ctx.Done():
		job.Status = db.DiscoveryJobStatusFailed
		return
	default:
	}

	// Generate targets from network CIDR
	maxHosts := config.MaxHosts
	if maxHosts <= 0 {
		maxHosts = 10000
	}

	targets, err := e.generateTargetsFromCIDR(job.Network.IPNet, maxHosts)
	if err != nil {
		job.Status = db.DiscoveryJobStatusFailed
		fmt.Printf("Failed to generate targets: %v\n", err)
		return
	}

	if len(targets) == 0 {
		job.Status = db.DiscoveryJobStatusCompleted
		fmt.Printf("No targets to discover.\n")
		return
	}

	// Calculate dynamic timeout based on target count
	dynamicTimeout := e.calculateDynamicTimeout(len(targets), config.Timeout)
	fmt.Printf("Starting nmap discovery with %d targets, method=%s, timeout=%v\n",
		len(targets), config.Method, dynamicTimeout)

	// Use nmap for host discovery with generated targets
	discoveredHosts, err := e.nmapDiscoveryWithTargets(ctx, targets, config, dynamicTimeout)
	if err != nil {
		job.Status = db.DiscoveryJobStatusFailed
		fmt.Printf("Discovery failed: %v\n", err)
		return
	}

	// Save discovered hosts to database
	if len(discoveredHosts) > 0 {
		err = e.saveDiscoveredHosts(ctx, discoveredHosts)
		if err != nil {
			fmt.Printf("Warning: Failed to save some discovered hosts: %v\n", err)
		} else {
			fmt.Printf("Saved %d discovered hosts to database\n", len(discoveredHosts))
		}
	}

	// Update job with results
	job.HostsResponsive = len(discoveredHosts)
	job.HostsDiscovered = len(discoveredHosts)
	fmt.Printf("Discovery completed. Found %d hosts.\n", job.HostsDiscovered)
}

// calculateDynamicTimeout calculates timeout based on network size and base timeout.
func (e *Engine) calculateDynamicTimeout(targetCount int, baseTimeout time.Duration) time.Duration {
	if baseTimeout <= 0 {
		baseTimeout = e.timeout
	}

	// Calculate multiplier: 6x for small networks, scaling up to 50x for large networks
	// Formula: multiplier = 6 + (targetCount / 100) * 2
	multiplier := timeoutMultiplierBase + (float64(targetCount)/timeoutDivisor)*timeoutMultiplierStep
	if multiplier > timeoutMultiplierMax {
		multiplier = timeoutMultiplierMax
	}

	calculatedTimeout := time.Duration(float64(baseTimeout) * multiplier)

	// Apply bounds
	if calculatedTimeout < minTimeout {
		calculatedTimeout = minTimeout
	}
	if calculatedTimeout > maxTimeout {
		calculatedTimeout = maxTimeout
	}

	return calculatedTimeout
}

// generateTargetsFromCIDR generates individual IP addresses from a CIDR block.
func (e *Engine) generateTargetsFromCIDR(ipnet net.IPNet, maxHosts int) ([]string, error) {
	ones, bits := ipnet.Mask.Size()

	// Handle special case of /31 networks (RFC 3021)
	if ones == rfc3021NetworkSize && bits == singleHostNetworkSize {
		ip := ipnet.IP.Mask(ipnet.Mask)
		return []string{
			ip.String(),
			e.nextIP(ip).String(),
		}, nil
	}

	// Handle /32 single host
	if ones == singleHostNetworkSize {
		return []string{ipnet.IP.String()}, nil
	}

	// Calculate network size
	hostBits := bits - ones
	if hostBits > maxHostBits {
		return nil, fmt.Errorf("network too large: /%d (max /8)", ones)
	}

	maxPossibleHosts := 1 << hostBits

	// For regular networks, subtract 2 for network and broadcast addresses
	usableHosts := maxPossibleHosts
	if ones < rfc3021NetworkSize {
		usableHosts = maxPossibleHosts - 2
	}

	// Apply maxHosts limit
	targetCount := usableHosts
	if maxHosts > 0 && maxHosts < usableHosts {
		targetCount = maxHosts
	}

	// Generate target list
	targets := make([]string, 0, targetCount)
	ip := ipnet.IP.Mask(ipnet.Mask)

	// Skip network address for regular networks
	if ones < rfc3021NetworkSize {
		ip = e.nextIP(ip)
	}

	for i := 0; i < targetCount && ipnet.Contains(ip); i++ {
		targets = append(targets, ip.String())
		ip = e.nextIP(ip)
	}

	return targets, nil
}

// nextIP returns the next IP address.
func (e *Engine) nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}

	return next
}

// nmapDiscoveryWithTargets performs nmap discovery on specific targets.
func (e *Engine) nmapDiscoveryWithTargets(ctx context.Context, targets []string, config *Config,
    timeout time.Duration) ([]Result, error) {
    if len(targets) == 0 {
        return []Result{}, nil
    }

    // Build library options and run discovery
    options := e.buildNmapOptionsForLibrary(targets, config, timeout)

    scanner, err := nmap.NewScanner(ctx, options...)
    if err != nil {
        return nil, fmt.Errorf("failed to create nmap scanner: %w", err)
    }

    run, warnings, err := scanner.Run()
    if err != nil {
        return nil, fmt.Errorf("nmap execution failed: %w", err)
    }
    if warnings != nil && len(*warnings) > 0 {
        log.Printf("Discovery completed with warnings: %v", *warnings)
    }

    // Convert hosts marked as up
    var results []Result
    for i := range run.Hosts {
        h := run.Hosts[i]
        if !strings.EqualFold(h.Status.State, "up") {
            continue
        }
        // Prefer IPv4, then IPv6
        var ipStr string
        for _, addr := range h.Addresses {
            if addr.AddrType == addrTypeIPv4 || addr.AddrType == addrTypeIPv6 {
                ipStr = addr.Addr
                if addr.AddrType == addrTypeIPv4 {
                    break
                }
            }
        }
        if ipStr == "" {
            continue
        }
        if ip := net.ParseIP(ipStr); ip != nil {
            results = append(results, Result{
                IPAddress: ip,
                Status:    "up",
                Method:    config.Method,
            })
        }
    }

    return results, nil
}

const (
    addrTypeIPv4 = "ipv4"
    addrTypeIPv6 = "ipv6"
)

// buildNmapOptionsForTargets constructs nmap arguments for target discovery.
func (e *Engine) buildNmapOptionsForTargets(targets []string, config *Config, timeout time.Duration) []string {
    // Kept for legacy tests which assert CLI args; not used by library runner.
    args := []string{"-sn"}
    switch config.Method {
    case "tcp":
        args = append(args, "-PS22,80,443,8080,8022,8379")
    case "ping":
        args = append(args, "-PE")
    case "arp":
        args = append(args, "-PR")
    }
    if config.DetectOS {
        args = append(args, "-O")
    }
    if timeout <= 30*time.Second {
        args = append(args, "-T4")
    } else if timeout <= 120*time.Second {
        args = append(args, "-T3")
    } else {
        args = append(args, "-T2")
    }
    hostTimeout := timeout / time.Duration(len(targets))
    if hostTimeout < time.Second {
        hostTimeout = time.Second
    }
    args = append(args, "--host-timeout", fmt.Sprintf("%ds", int(hostTimeout.Seconds())))
    args = append(args, targets...)
    return args
}

// buildNmapOptionsForLibrary constructs nmap library options for discovery.
func (e *Engine) buildNmapOptionsForLibrary(targets []string, config *Config, timeout time.Duration) []nmap.Option {
    opts := []nmap.Option{
        nmap.WithPingScan(),
        nmap.WithTargets(targets...),
    }
    // Timing template based on timeout
    if timeout <= 30*time.Second {
        opts = append(opts, nmap.WithTimingTemplate(nmap.TimingAggressive))
    } else if timeout <= 120*time.Second {
        opts = append(opts, nmap.WithTimingTemplate(nmap.TimingNormal))
    } else {
        opts = append(opts, nmap.WithTimingTemplate(nmap.TimingPolite))
    }
    // OS detection when requested
    if config.DetectOS {
        opts = append(opts, nmap.WithOSDetection())
    }
    return opts
}

// parseNmapOutput parses legacy human-readable nmap output to extract discovery results.
// Kept for test compatibility; production discovery uses XML parsing.
func (e *Engine) parseNmapOutput(output, method string) []Result {
    var results []Result
    lines := strings.Split(output, "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "Nmap scan report for ") {
            parts := strings.Fields(line)
            if len(parts) >= minNmapOutputFields {
                ipStr := parts[4]
                if ip := net.ParseIP(ipStr); ip != nil {
                    results = append(results, Result{
                        IPAddress: ip,
                        Status:    "up",
                        Method:    method,
                    })
                }
            }
        }
    }
    return results
}

// finalizeDiscoveryJob handles the completion and saving of a discovery job.
func (e *Engine) finalizeDiscoveryJob(ctx context.Context, job *db.DiscoveryJob) {
	// Check if context is canceled before finalizing
	select {
	case <-ctx.Done():
		log.Printf("Discovery finalization canceled for job %s", job.ID)
		return
	default:
	}

	now := time.Now()
	job.CompletedAt = &now
	if job.Status == db.DiscoveryJobStatusRunning {
		job.Status = db.DiscoveryJobStatusCompleted
	}

	if err := e.saveDiscoveryJob(ctx, job); err != nil {
		log.Printf("Error saving discovery job completion: %v", err)
	} else {
		log.Printf("Discovery job %s finalized successfully", job.ID)
	}
}

// saveDiscoveryJob saves or updates a discovery job in the database.
func (e *Engine) saveDiscoveryJob(ctx context.Context, job *db.DiscoveryJob) error {
	query := `
		INSERT INTO discovery_jobs (id, network, method, status, created_at, completed_at,
			hosts_discovered, hosts_responsive)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			completed_at = EXCLUDED.completed_at,
			hosts_discovered = EXCLUDED.hosts_discovered,
			hosts_responsive = EXCLUDED.hosts_responsive`

	_, err := e.db.ExecContext(ctx, query,
		job.ID,
		job.Network.String(),
		job.Method,
		job.Status,
		job.CreatedAt,
		job.CompletedAt,
		job.HostsDiscovered,
		job.HostsResponsive,
	)

	return err
}

// WaitForCompletion waits for a discovery job to complete or timeout.
func (e *Engine) WaitForCompletion(ctx context.Context, jobID uuid.UUID, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var status string
		var completedAt *time.Time
		query := `SELECT status, completed_at FROM discovery_jobs WHERE id = $1`
		err := e.db.QueryRowContext(ctx, query, jobID).Scan(&status, &completedAt)

		if err != nil {
			if err.Error() == sqlNoRowsError {
				time.Sleep(retryInterval)
				continue
			}
			return fmt.Errorf("failed to check job status: %w", err)
		}

		switch status {
		case db.DiscoveryJobStatusCompleted:
			return nil
		case db.DiscoveryJobStatusFailed:
			return fmt.Errorf("discovery job failed")
		case db.DiscoveryJobStatusRunning:
			time.Sleep(retryInterval)
		default:
			return fmt.Errorf("unknown job status: %s", status)
		}
	}

	return fmt.Errorf("discovery job did not complete within %v", timeout)
}

// saveDiscoveredHosts saves discovery results to the hosts table.
func (e *Engine) saveDiscoveredHosts(ctx context.Context, results []Result) error {
	if len(results) == 0 {
		return nil
	}

	var errors []string

	for _, result := range results {
		// Check if host already exists
		var existingID string
		checkQuery := `SELECT id FROM hosts WHERE ip_address = $1`
		err := e.db.QueryRowContext(ctx, checkQuery, result.IPAddress.String()).Scan(&existingID)

		if err != nil && err.Error() != sqlNoRowsError {
			// Some other error occurred
			log.Printf("Error checking existing host %s: %v", result.IPAddress, err)
			errors = append(errors, fmt.Sprintf("failed to check host %s: %v", result.IPAddress, err))
			continue
		}

		if existingID != "" {
			// Host exists, update it
			updateQuery := `
				UPDATE hosts SET
					status = $2,
					discovery_method = $3,
					last_seen = NOW(),
					discovery_count = COALESCE(discovery_count, 0) + 1
				WHERE ip_address = $1`

			_, err = e.db.ExecContext(ctx, updateQuery,
				result.IPAddress.String(),
				result.Status,
				result.Method)

			if err != nil {
				log.Printf("Failed to update host %s: %v", result.IPAddress, err)
				errors = append(errors, fmt.Sprintf("failed to update host %s: %v", result.IPAddress, err))
			}
		} else {
			// Host doesn't exist, create it
			insertQuery := `
				INSERT INTO hosts (ip_address, status, discovery_method, first_seen, last_seen, discovery_count)
				VALUES ($1, $2, $3, NOW(), NOW(), 1)`

			_, err = e.db.ExecContext(ctx, insertQuery,
				result.IPAddress.String(),
				result.Status,
				result.Method)

			if err != nil {
				log.Printf("Failed to insert host %s: %v", result.IPAddress, err)
				errors = append(errors, fmt.Sprintf("failed to insert host %s: %v", result.IPAddress, err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors saving hosts: %s", strings.Join(errors, "; "))
	}

	return nil
}
