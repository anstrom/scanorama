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
	retryInterval         = 100 * time.Millisecond
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
	go e.runDiscovery(ctx, job, config)

	return job, nil
}

// WaitForCompletion waits for a discovery job to complete or timeout.
func (e *Engine) WaitForCompletion(ctx context.Context, jobID uuid.UUID, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	log.Printf("DEBUG: WaitForCompletion starting for job %s with timeout %v", jobID.String(), timeout)

	for time.Now().Before(deadline) {
		// Check job status
		var status string
		query := `SELECT status FROM discovery_jobs WHERE id = $1`
		err := e.db.QueryRowContext(ctx, query, jobID).Scan(&status)
		if err != nil {
			// Job might not exist yet due to background goroutine timing
			if err.Error() == "sql: no rows in result set" {
				log.Printf("DEBUG: Discovery job %s not found yet, waiting...", jobID.String())
				time.Sleep(retryInterval)
				continue
			}
			log.Printf("DEBUG: Failed to query discovery job %s: %v", jobID.String(), err)
			return fmt.Errorf("failed to check job status: %w", err)
		}

		log.Printf("DEBUG: Discovery job %s status: %s", jobID.String(), status)

		switch status {
		case db.DiscoveryJobStatusCompleted:
			log.Printf("DEBUG: Discovery job %s completed successfully", jobID.String())
			return nil
		case db.DiscoveryJobStatusFailed:
			log.Printf("DEBUG: Discovery job %s failed", jobID.String())
			return fmt.Errorf("discovery job failed")
		case db.DiscoveryJobStatusRunning:
			// Still running, continue waiting
			time.Sleep(retryInterval)
		default:
			log.Printf("DEBUG: Discovery job %s has unknown status: %s", jobID.String(), status)
			return fmt.Errorf("unknown job status: %s", status)
		}
	}

	log.Printf("DEBUG: Discovery job %s timed out after %v", jobID.String(), timeout)
	return fmt.Errorf("discovery job did not complete within %v", timeout)
}

// runDiscovery executes the actual discovery process using nmap.
func (e *Engine) runDiscovery(ctx context.Context, job *db.DiscoveryJob, config Config) {
	defer e.finalizeDiscoveryJob(ctx, job)

	// Check if context is already canceled
	select {
	case <-ctx.Done():
		job.Status = db.DiscoveryJobStatusFailed
		return
	default:
	}

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
	// Check if context is canceled before finalizing
	select {
	case <-ctx.Done():
		return // Don't attempt database operations if context is canceled
	default:
	}

	now := time.Now()
	job.CompletedAt = &now
	if job.Status == db.DiscoveryJobStatusRunning {
		job.Status = db.DiscoveryJobStatusCompleted
	}
	if err := e.saveDiscoveryJob(ctx, job); err != nil {
		fmt.Printf("Error saving discovery job completion: %v\n", err)
	}
}

// buildNmapOptions constructs nmap options based on network and timeout configuration.
func buildNmapOptions(network string, timeout time.Duration) []nmap.Option {
	options := []nmap.Option{
		nmap.WithTargets(network),
		nmap.WithPingScan(), // Host discovery only, no port scan
	}

	// Add timing based on timeout
	if timeout <= 5*time.Second {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
	} else if timeout <= 15*time.Second {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingNormal))
	} else {
		options = append(options, nmap.WithTimingTemplate(nmap.TimingPolite))
	}

	return options
}

// convertNmapHostToDBHost converts an nmap host result to database host format.
func (e *Engine) convertNmapHostToDBHost(host *nmap.Host, config Config) *db.Host {
	if len(host.Addresses) == 0 || host.Status.State != "up" {
		return nil
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

	return dbHost
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
	options := buildNmapOptions(network, timeout)

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
	discoveredHosts := make([]*db.Host, 0, len(result.Hosts))
	for i := range result.Hosts {
		host := &result.Hosts[i]
		dbHost := e.convertNmapHostToDBHost(host, config)
		if dbHost == nil {
			continue
		}

		// Store the host in database
		result := &Result{
			IPAddress: dbHost.IPAddress.IP,
			Status:    dbHost.Status,
			Method:    config.Method,
		}
		log.Printf("DEBUG: Attempting to save discovered host %s with method=%s, status=%s",
			result.IPAddress.String(), result.Method, result.Status)

		if err := e.saveDiscoveredHost(ctx, result, uuid.Nil); err != nil {
			// Check if context was canceled
			if ctx.Err() != nil {
				return discoveredHosts, ctx.Err()
			}
			log.Printf("Failed to save discovered host %s: %v", dbHost.IPAddress.String(), err)
			continue
		}

		log.Printf("DEBUG: Successfully saved discovered host %s", result.IPAddress.String())

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
	// Check if context is canceled before database operations
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if host already exists
	var existingHost db.Host
	query := `SELECT id, discovery_count FROM hosts WHERE ip_address = $1`
	err := e.db.QueryRowContext(ctx, query, db.IPAddr{IP: result.IPAddress}).Scan(
		&existingHost.ID, &existingHost.DiscoveryCount)

	now := time.Now()
	responseTimeMS := int(result.ResponseTime.Milliseconds())

	if err != nil {
		log.Printf("DEBUG: Creating new host for %s with discovery_method=%s",
			result.IPAddress.String(), result.Method)
		return e.createNewHost(ctx, result, now, responseTimeMS)
	}
	log.Printf("DEBUG: Updating existing host %s (id=%s) with discovery_method=%s",
		result.IPAddress.String(), existingHost.ID.String(), result.Method)
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

	log.Printf("DEBUG: Creating new host with ID=%s, IP=%s, DiscoveryMethod=%v",
		host.ID.String(), host.IPAddress.String(),
		func() string {
			if host.DiscoveryMethod != nil {
				return *host.DiscoveryMethod
			} else {
				return "NULL"
			}
		}())

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
	log.Printf("DEBUG: Updating existing host ID=%s with discovery_method=%s",
		existingHost.ID.String(), result.Method)

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
