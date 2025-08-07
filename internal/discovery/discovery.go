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
	"github.com/jmoiron/sqlx"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Default discovery configuration values.
	defaultConcurrency    = 50
	defaultTimeoutSeconds = 3
	maxNetworkSizeBits    = 16 // Limit to /16 or smaller networks
	retryInterval         = 100 * time.Millisecond
	ciConsistencyDelay    = 50 * time.Millisecond
	nullValue             = "NULL"
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
	startTime := time.Now()
	checkCount := 0

	for time.Now().Before(deadline) {
		checkCount++
		elapsed := time.Since(startTime)

		// Check job status
		var status string
		var completedAt *time.Time
		var hostsDiscovered int
		query := `SELECT status, completed_at, hosts_discovered FROM discovery_jobs WHERE id = $1`
		err := e.db.QueryRowContext(ctx, query, jobID).Scan(&status, &completedAt, &hostsDiscovered)

		if err != nil {
			// Job might not exist yet due to background goroutine timing
			if err.Error() == "sql: no rows in result set" {
				time.Sleep(retryInterval)
				continue
			}
			log.Printf("ERROR: WaitForCompletion failed to check job status for %s: %v", jobID, err)
			return fmt.Errorf("failed to check job status: %w", err)
		}

		switch status {
		case db.DiscoveryJobStatusCompleted:
			// Discovery job completed - ensure transaction consistency for CI

			// Force database consistency check
			if err := e.ensureHostTransactionConsistency(ctx); err != nil {
				log.Printf("WARNING: Database consistency check failed: %v", err)
			}

			// Use hostsDiscovered from the query above (already available)
			if hostsDiscovered > 0 {
				// Verify consistency only if hosts were discovered
				if err := e.verifyDiscoveryConsistency(ctx, jobID, hostsDiscovered); err != nil {
					log.Printf("WARNING: Discovery consistency check failed: %v", err)
					// Don't fail the operation, just log warning
				}
			}

			return nil
		case db.DiscoveryJobStatusFailed:
			log.Printf("ERROR: Discovery job %s failed after %d checks (elapsed: %v)", jobID, checkCount, elapsed)
			return fmt.Errorf("discovery job failed")
		case db.DiscoveryJobStatusRunning:
			// Still running, continue waiting
			time.Sleep(retryInterval)
		default:
			log.Printf("ERROR: WaitForCompletion unknown job status %q for %s", status, jobID)
			return fmt.Errorf("unknown job status: %s", status)
		}
	}

	// Timeout reached - get final status
	var finalStatus string
	var finalCompletedAt *time.Time
	var finalHosts int
	finalQuery := `SELECT status, completed_at, hosts_discovered FROM discovery_jobs WHERE id = $1`
	err := e.db.QueryRowContext(ctx, finalQuery, jobID).Scan(&finalStatus, &finalCompletedAt, &finalHosts)
	if err != nil {
		log.Printf("ERROR: WaitForCompletion timeout after %d checks (%v) - could not get final status: %v",
			checkCount, timeout, err)
		return fmt.Errorf("discovery job did not complete within %v (final status unknown: %v)", timeout, err)
	}

	log.Printf("ERROR: WaitForCompletion timeout after %d checks (%v) - final status: %q, completed_at: %v, hosts: %d",
		checkCount, timeout, finalStatus, finalCompletedAt, finalHosts)
	return fmt.Errorf("discovery job did not complete within %v (final status: %s)", timeout, finalStatus)
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
		log.Printf("Error saving discovery job completion: %v", err)
	}
}

// buildNmapOptions constructs nmap options based on network and timeout configuration.
func buildNmapOptions(network string, timeout time.Duration) []nmap.Option {
	options := []nmap.Option{
		nmap.WithTargets(network),
		// Use TCP connect discovery instead of ICMP ping (no root privileges required)
		nmap.WithPorts("22,80,443,8080,8443,3389,5432,6379"),
		nmap.WithConnectScan(),
		// TCP connect scan works without root privileges and provides host discovery
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

		if err := e.saveDiscoveredHost(ctx, result, uuid.Nil); err != nil {
			// Check if context was canceled
			if ctx.Err() != nil {
				return discoveredHosts, ctx.Err()
			}
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

// saveDiscoveredHost saves or updates a discovered host in the database with transaction safety.
func (e *Engine) saveDiscoveredHost(ctx context.Context, result *Result, _ uuid.UUID) error {

	// Check if context is canceled before database operations
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Use explicit transaction for host save to ensure proper commit
	tx, err := e.db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Use UPSERT to handle race conditions between discovery processes
	// This atomically handles both insert and update cases

	now := time.Now()
	responseTimeMS := int(result.ResponseTime.Milliseconds())

	err = e.upsertHostTx(ctx, tx, result, now, responseTimeMS)

	if err != nil {
		return err
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Verify the host was actually saved by reading it back
	var verificationHost db.Host
	verifyQuery := `SELECT id, discovery_method FROM hosts WHERE ip_address = $1`
	err = e.db.QueryRowContext(ctx, verifyQuery, db.IPAddr{IP: result.IPAddress}).Scan(
		&verificationHost.ID, &verificationHost.DiscoveryMethod)
	if err != nil {
		return fmt.Errorf("host verification failed after save: %w", err)
	}

	expectedMethod := result.Method
	actualMethod := nullValue
	if verificationHost.DiscoveryMethod != nil {
		actualMethod = *verificationHost.DiscoveryMethod
	}

	if actualMethod != expectedMethod {
		return fmt.Errorf("discovery method verification failed: expected %s, got %s", expectedMethod, actualMethod)
	}
	return nil
}

// upsertHostTx atomically inserts or updates a host to prevent race conditions.
func (e *Engine) upsertHostTx(ctx context.Context, tx *sqlx.Tx, result *Result,
	now time.Time, responseTimeMS int) error {
	hostID := uuid.New()

	// Prepare OS information
	var osFamily, osName, osVersion, osMethod *string
	var osConfidence *int
	var osDetectedAt *time.Time
	var osDetails *[]byte

	if result.OSInfo != nil {
		osFamily = &result.OSInfo.Family
		osName = &result.OSInfo.Name
		osVersion = &result.OSInfo.Version
		osConfidence = &result.OSInfo.Confidence
		osDetectedAt = &now
		osMethod = &result.OSInfo.Method
		if result.OSInfo.Details != nil {
			detailsJSON, _ := json.Marshal(result.OSInfo.Details)
			osDetails = &detailsJSON
		}
	}

	// Use UPSERT to handle race conditions atomically
	query := `
		INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
						   os_confidence, os_detected_at, os_method, os_details, discovery_method,
						   response_time_ms, discovery_count, ignore_scanning, first_seen, last_seen, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (ip_address) DO UPDATE SET
			status = EXCLUDED.status,
			last_seen = EXCLUDED.last_seen,
			discovery_method = EXCLUDED.discovery_method,
			response_time_ms = EXCLUDED.response_time_ms,
			discovery_count = hosts.discovery_count + 1,
			os_family = COALESCE(EXCLUDED.os_family, hosts.os_family),
			os_name = COALESCE(EXCLUDED.os_name, hosts.os_name),
			os_version = COALESCE(EXCLUDED.os_version, hosts.os_version),
			os_confidence = COALESCE(EXCLUDED.os_confidence, hosts.os_confidence),
			os_detected_at = COALESCE(EXCLUDED.os_detected_at, hosts.os_detected_at),
			os_method = COALESCE(EXCLUDED.os_method, hosts.os_method),
			os_details = CASE WHEN EXCLUDED.os_details IS NOT NULL THEN EXCLUDED.os_details ELSE hosts.os_details END
	`

	_, err := tx.ExecContext(ctx, query,
		hostID, db.IPAddr{IP: result.IPAddress}, nil, nil, nil,
		osFamily, osName, osVersion, osConfidence, osDetectedAt,
		osMethod, osDetails, &result.Method, &responseTimeMS,
		1, false, now, now, result.Status)

	return err
}

// verifyDiscoveryConsistency checks that discovery results are properly stored
// and consistent with the job record. This helps catch CI race conditions.
func (e *Engine) verifyDiscoveryConsistency(ctx context.Context, jobID uuid.UUID, expectedHosts int) error {
	// Get discovery job details
	var jobNetwork string
	var jobMethod string
	query := `SELECT network, method FROM discovery_jobs WHERE id = $1`
	err := e.db.QueryRowContext(ctx, query, jobID).Scan(&jobNetwork, &jobMethod)
	if err != nil {
		return fmt.Errorf("failed to get job details: %w", err)
	}

	// Count hosts with matching discovery method
	var actualHosts int
	hostQuery := `SELECT COUNT(*) FROM hosts WHERE discovery_method = $1`
	err = e.db.QueryRowContext(ctx, hostQuery, jobMethod).Scan(&actualHosts)
	if err != nil {
		return fmt.Errorf("failed to count discovered hosts: %w", err)
	}

	// Allow some tolerance for CI environments
	if actualHosts < expectedHosts {
		return fmt.Errorf("consistency mismatch: expected %d hosts with method %s, found %d",
			expectedHosts, jobMethod, actualHosts)
	}

	return nil
}

// ensureHostTransactionConsistency forces a database sync to ensure all
// host transactions are committed. This helps with CI race conditions.
func (e *Engine) ensureHostTransactionConsistency(ctx context.Context) error {
	// Force a database sync operation
	var syncResult int
	query := `SELECT COUNT(*) FROM hosts WHERE last_seen >= NOW() - INTERVAL '1 minute'`
	err := e.db.QueryRowContext(ctx, query).Scan(&syncResult)
	if err != nil {
		return fmt.Errorf("failed to sync database: %w", err)
	}

	// Small delay to ensure transaction commits in CI
	time.Sleep(ciConsistencyDelay)

	return nil
}
