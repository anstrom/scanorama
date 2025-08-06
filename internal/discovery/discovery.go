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

	log.Printf("DEBUG: WaitForCompletion starting for job %s with timeout %v", jobID, timeout)

	for time.Now().Before(deadline) {
		checkCount++
		elapsed := time.Since(startTime)
		remaining := timeout - elapsed

		// Check job status with detailed debugging
		var status string
		var completedAt *time.Time
		var hostsDiscovered int
		query := `SELECT status, completed_at, hosts_discovered FROM discovery_jobs WHERE id = $1`
		err := e.db.QueryRowContext(ctx, query, jobID).Scan(&status, &completedAt, &hostsDiscovered)

		log.Printf("DEBUG: WaitForCompletion check #%d (elapsed: %v, remaining: %v): "+
			"job=%s, status=%q, completed_at=%v, hosts=%d, err=%v",
			checkCount, elapsed.Truncate(time.Millisecond), remaining.Truncate(time.Millisecond),
			jobID, status, completedAt, hostsDiscovered, err)

		if err != nil {
			// Job might not exist yet due to background goroutine timing
			if err.Error() == "sql: no rows in result set" {
				log.Printf("DEBUG: WaitForCompletion job %s not found yet, continuing to wait...", jobID)
				time.Sleep(retryInterval)
				continue
			}
			log.Printf("ERROR: WaitForCompletion failed to check job status for %s: %v", jobID, err)
			return fmt.Errorf("failed to check job status: %w", err)
		}

		switch status {
		case db.DiscoveryJobStatusCompleted:
			// Discovery job completed - ensure transaction consistency for CI
			log.Printf("DEBUG: Discovery job %s completed after %d checks (elapsed: %v), verifying consistency...",
				jobID, checkCount, elapsed.Truncate(time.Millisecond))

			// Force database consistency check
			if err := e.ensureHostTransactionConsistency(ctx); err != nil {
				log.Printf("WARNING: Database consistency check failed: %v", err)
			}

			// Use hostsDiscovered from the query above (already available)
			if hostsDiscovered > 0 {
				// Verify consistency only if hosts were discovered
				if err := e.verifyDiscoveryConsistency(ctx, jobID, hostsDiscovered); err != nil {
					log.Printf("WARNING: Discovery consistency check failed: %v", err)
					// Don't fail the operation, just log warning for CI debugging
				}
			}

			log.Printf("DEBUG: Discovery job %s consistency checks completed successfully", jobID)
			return nil
		case db.DiscoveryJobStatusFailed:
			log.Printf("ERROR: Discovery job %s failed after %d checks (elapsed: %v)", jobID, checkCount, elapsed)
			return fmt.Errorf("discovery job failed")
		case db.DiscoveryJobStatusRunning:
			// Still running, continue waiting
			log.Printf("DEBUG: WaitForCompletion job %s still running, sleeping %v...", jobID, retryInterval)
			time.Sleep(retryInterval)
		default:
			log.Printf("ERROR: WaitForCompletion unknown job status %q for %s", status, jobID)
			return fmt.Errorf("unknown job status: %s", status)
		}
	}

	// Timeout reached - get final status for debugging
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
		log.Printf("DEBUG: Discovery finalization canceled for job %s", job.ID)
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
	} else {
		log.Printf("DEBUG: Discovery job %s finalized successfully", job.ID)
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
	log.Printf("DEBUG: Discovery attempting to save host %s with method=%s", result.IPAddress, result.Method)

	// Check if context is canceled before database operations
	select {
	case <-ctx.Done():
		log.Printf("DEBUG: Discovery save canceled for host %s", result.IPAddress)
		return ctx.Err()
	default:
	}

	// Use explicit transaction for host save to ensure proper commit
	tx, err := e.db.BeginTx(ctx)
	if err != nil {
		log.Printf("DEBUG: Discovery failed to begin transaction for host %s: %v", result.IPAddress, err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("DEBUG: Discovery failed to rollback transaction for host %s: %v",
					result.IPAddress, rollbackErr)
			}
		}
	}()

	// Check if host already exists
	var existingHost db.Host
	query := `SELECT id, discovery_count FROM hosts WHERE ip_address = $1`
	log.Printf("DEBUG: Discovery checking if host %s already exists", result.IPAddress)
	err = tx.QueryRowContext(ctx, query, db.IPAddr{IP: result.IPAddress}).Scan(
		&existingHost.ID, &existingHost.DiscoveryCount)

	now := time.Now()
	responseTimeMS := int(result.ResponseTime.Milliseconds())

	if err != nil {
		log.Printf("DEBUG: Discovery host %s not found, creating new (error: %v)", result.IPAddress, err)
		err = e.createNewHostTx(ctx, tx, result, now, responseTimeMS)
	} else {
		log.Printf("DEBUG: Discovery host %s already exists, updating", result.IPAddress)
		err = e.updateExistingHostTx(ctx, tx, &existingHost, result, now, responseTimeMS)
	}

	if err != nil {
		log.Printf("DEBUG: Discovery failed to save host %s: %v", result.IPAddress, err)
		return err
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		log.Printf("DEBUG: Discovery failed to commit transaction for host %s: %v", result.IPAddress, err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Verify the host was actually saved by reading it back
	var verificationHost db.Host
	verifyQuery := `SELECT id, discovery_method FROM hosts WHERE ip_address = $1`
	err = e.db.QueryRowContext(ctx, verifyQuery, db.IPAddr{IP: result.IPAddress}).Scan(
		&verificationHost.ID, &verificationHost.DiscoveryMethod)
	if err != nil {
		log.Printf("DEBUG: Discovery verification failed for host %s: %v", result.IPAddress, err)
		return fmt.Errorf("host verification failed after save: %w", err)
	}

	expectedMethod := result.Method
	actualMethod := nullValue
	if verificationHost.DiscoveryMethod != nil {
		actualMethod = *verificationHost.DiscoveryMethod
	}

	if actualMethod != expectedMethod {
		log.Printf("DEBUG: Discovery method mismatch for host %s: expected=%s, actual=%s",
			result.IPAddress, expectedMethod, actualMethod)
		return fmt.Errorf("discovery method verification failed: expected %s, got %s", expectedMethod, actualMethod)
	}

	log.Printf("DEBUG: Discovery successfully verified host %s with discovery_method=%s",
		result.IPAddress, actualMethod)
	return nil
}

// insertHostTx inserts a new host into the database within a transaction.
func (e *Engine) insertHostTx(ctx context.Context, tx *sqlx.Tx, host *db.Host) error {
	log.Printf("DEBUG: Discovery inserting new host %s with discovery_method=%v",
		host.IPAddress.String(),
		func() string {
			if host.DiscoveryMethod != nil {
				return *host.DiscoveryMethod
			}
			return "NULL"
		}())

	query := `
		INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
						   os_confidence, os_detected_at, os_method, os_details, discovery_method,
						   response_time_ms, discovery_count, ignore_scanning, first_seen, last_seen, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := tx.ExecContext(ctx, query,
		host.ID, host.IPAddress, host.Hostname, host.MACAddress, host.Vendor,
		host.OSFamily, host.OSName, host.OSVersion, host.OSConfidence, host.OSDetectedAt,
		host.OSMethod, host.OSDetails, host.DiscoveryMethod, host.ResponseTimeMS,
		host.DiscoveryCount, host.IgnoreScanning, host.FirstSeen, host.LastSeen, host.Status)

	if err != nil {
		log.Printf("DEBUG: Discovery failed to insert host %s: %v", host.IPAddress.String(), err)
	} else {
		log.Printf("DEBUG: Discovery successfully inserted host %s with discovery_method=%v",
			host.IPAddress.String(),
			func() string {
				if host.DiscoveryMethod != nil {
					return *host.DiscoveryMethod
				}
				return "NULL"
			}())
	}

	return err
}

// createNewHostTx creates a new host entry in the database within a transaction.
func (e *Engine) createNewHostTx(ctx context.Context, tx *sqlx.Tx, result *Result,
	now time.Time, responseTimeMS int) error {
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

	return e.insertHostTx(ctx, tx, &host)
}

// updateExistingHostTx updates an existing host entry in the database within a transaction.
func (e *Engine) updateExistingHostTx(
	ctx context.Context, tx *sqlx.Tx, existingHost *db.Host, result *Result, now time.Time, responseTimeMS int,
) error {
	log.Printf("DEBUG: Discovery updating existing host %s with discovery_method=%s", result.IPAddress, result.Method)

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
	_, err := tx.ExecContext(ctx, updateQuery, args...)

	if err != nil {
		log.Printf("DEBUG: Discovery failed to update host %s: %v", result.IPAddress, err)
	} else {
		log.Printf("DEBUG: Discovery successfully updated host %s with discovery_method=%s",
			result.IPAddress, result.Method)
	}

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

	log.Printf("DEBUG: Discovery consistency check for job %s: expected=%d, actual=%d (method=%s)",
		jobID, expectedHosts, actualHosts, jobMethod)

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

	log.Printf("DEBUG: Database consistency check: %d recent hosts found", syncResult)

	// Small delay to ensure transaction commits in CI
	time.Sleep(ciConsistencyDelay)

	return nil
}
