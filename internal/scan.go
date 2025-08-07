// Package internal provides core scanning functionality and shared types for scanorama.
// It contains scan execution logic, result processing, XML handling,
// and common data structures used throughout the application.
package internal

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Constants for scan configuration validation.
const (
	minTimeoutSeconds     = 5
	mediumTimeoutSeconds  = 15
	maxConcurrency        = 20
	ipv6CIDRBits          = 128
	defaultTargetCapacity = 10
)

const (
	// Database null value representation.

	// Output formatting constants.
	outputSeparatorLength = 80
)

// RunScan is a convenience wrapper around RunScanWithContext that uses a background context.
func RunScan(config *ScanConfig) (*ScanResult, error) {
	return RunScanWithContext(context.Background(), config, nil)
}

// RunScanWithDB is a convenience wrapper that includes database storage.
func RunScanWithDB(config *ScanConfig, database *db.DB) (*ScanResult, error) {
	return RunScanWithContext(context.Background(), config, database)
}

// RunScanWithContext performs a network scan based on the provided configuration and context.
// It uses nmap to scan the specified targets and ports, returning detailed results
// about discovered hosts and services. If database is provided, results are stored.
func RunScanWithContext(ctx context.Context, config *ScanConfig, database *db.DB) (*ScanResult, error) {
	// Validate the configuration
	if err := validateScanConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply timeout if specified
	if config.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(config.TimeoutSec)*time.Second)
		defer cancel()
	}

	// Initialize scan result with start time
	scanResult := NewScanResult()
	defer scanResult.Complete()

	// Create and run scanner
	result, err := createAndRunScanner(ctx, config)
	if err != nil {
		return nil, err
	}

	// Convert results to our format
	convertNmapResults(result, scanResult)

	// Store results in database if provided
	if database != nil {
		if err := storeScanResults(ctx, database, config, scanResult); err != nil {
			log.Printf("Failed to store scan results in database: %v", err)
			// Don't fail the scan if database storage fails
		}
	}

	return scanResult, nil
}

// createAndRunScanner creates an nmap scanner with the given config and runs it.
func createAndRunScanner(ctx context.Context, config *ScanConfig) (*nmap.Run, error) {
	options := buildScanOptions(config)

	scanner, err := nmap.NewScanner(ctx, options...)
	if err != nil {
		return nil, &ScanError{Op: "create scanner", Err: err}
	}

	// Run the scan
	result, warnings, err := scanner.Run()
	if err != nil {
		return nil, &ScanError{Op: "run scan", Err: err}
	}

	if warnings != nil && len(*warnings) > 0 {
		log.Printf("Scan completed with warnings: %v", *warnings)
	}

	return result, nil
}

// buildScanOptions creates nmap options based on scan configuration.
func buildScanOptions(config *ScanConfig) []nmap.Option {
	options := []nmap.Option{
		nmap.WithTargets(config.Targets...),
		nmap.WithPorts(config.Ports),
	}

	// Add scan type options with enhanced capabilities
	switch config.ScanType {
	case "connect":
		options = append(options, nmap.WithConnectScan())
	case "syn":
		options = append(options, nmap.WithSYNScan())
	case "version":
		options = append(options,
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
		)
	case "intense":
		options = append(options,
			nmap.WithConnectScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithAggressiveScan(),
		)
	case "stealth":
		options = append(options,
			nmap.WithConnectScan(),
			nmap.WithTimingTemplate(nmap.TimingPolite),
		)
	case "comprehensive":
		options = append(options,
			nmap.WithConnectScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithDefaultScript(),
		)
	}

	// Add timing template based on configuration
	if config.TimeoutSec > 0 {
		if config.TimeoutSec <= minTimeoutSeconds {
			options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
		} else if config.TimeoutSec <= mediumTimeoutSeconds {
			options = append(options, nmap.WithTimingTemplate(nmap.TimingNormal))
		} else {
			options = append(options, nmap.WithTimingTemplate(nmap.TimingPolite))
		}
	}

	// Add host discovery options for better reliability
	options = append(options,
		nmap.WithSkipHostDiscovery(), // Skip ping and go straight to port scan
	)

	// Add performance optimizations
	if config.Concurrency > 0 {
		// nmap library doesn't directly expose parallelism, but we can use timing
		if config.Concurrency > maxConcurrency {
			options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
		}
	}

	// Add useful scanning options for better results
	options = append(options,
		nmap.WithVerbosity(1),
	)

	return options
}

// convertNmapResults converts nmap results to our internal format.
func convertNmapResults(result *nmap.Run, scanResult *ScanResult) {
	// Convert stats
	scanResult.Stats = HostStats{
		Up:    result.Stats.Hosts.Up,
		Down:  result.Stats.Hosts.Down,
		Total: result.Stats.Hosts.Total,
	}

	// Convert hosts
	scanResult.Hosts = make([]Host, 0, len(result.Hosts))
	for i := range result.Hosts {
		if host := convertNmapHost(&result.Hosts[i]); host != nil {
			scanResult.Hosts = append(scanResult.Hosts, *host)
		}
	}
}

// convertNmapHost converts a single nmap host to our format.
func convertNmapHost(h *nmap.Host) *Host {
	if len(h.Addresses) == 0 {
		return nil
	}

	host := &Host{
		Address: h.Addresses[0].Addr,
		Status:  h.Status.State,
		Ports:   make([]Port, 0, len(h.Ports)),
	}

	for j := range h.Ports {
		p := &h.Ports[j]
		port := Port{
			Number:      p.ID,
			Protocol:    p.Protocol,
			State:       p.State.State,
			Service:     p.Service.Name,
			Version:     p.Service.Version,
			ServiceInfo: p.Service.Product,
		}
		host.Ports = append(host.Ports, port)
	}

	return host
}

// validateScanConfig verifies that all scan parameters are valid.
// It checks target specification, port ranges, and scan type.
func validateScanConfig(config *ScanConfig) error {
	return config.Validate()
}

// PrintResults displays scan results in a human-readable format on stdout.
// The output includes host status, open ports, and detected services.
func PrintResults(result *ScanResult) {
	if result == nil {
		fmt.Println("No results available")
		return
	}

	fmt.Println("Scan Results:")
	fmt.Println("=============")
	fmt.Printf("Scan started: %s\n", result.StartTime.Format(time.RFC3339))
	fmt.Printf("Scan duration: %v\n", result.Duration)
	fmt.Printf("Total hosts: %d, Up: %d, Down: %d\n\n",
		result.Stats.Total, result.Stats.Up, result.Stats.Down)

	for _, host := range result.Hosts {
		fmt.Printf("Host: %s (%s)\n", host.Address, host.Status)
		if host.Status != "up" {
			continue
		}

		if len(host.Ports) == 0 {
			fmt.Println("No open ports found")
			continue
		}

		fmt.Println("Open Ports:")
		fmt.Printf("%-6s %-10s %-15s %-20s %s\n",
			"Port", "Protocol", "State", "Service", "Version")
		fmt.Printf("%s\n", strings.Repeat("-", outputSeparatorLength))

		for _, port := range host.Ports {
			version := port.Version
			if port.ServiceInfo != "" {
				if version != "" {
					version += " "
				}
				version += port.ServiceInfo
			}
			fmt.Printf("%-6d %-10s %-15s %-20s %s\n",
				port.Number, port.Protocol, port.State,
				port.Service, version)
		}
		fmt.Println()
	}
}

// storeScanResults stores scan results in the database.
func storeScanResults(ctx context.Context, database *db.DB, config *ScanConfig, result *ScanResult) error {
	// Create a scan job record - for now we'll create a minimal scan target
	scanTarget, err := createOrGetScanTarget(ctx, database, config)
	if err != nil {
		return fmt.Errorf("failed to create scan target: %w", err)
	}

	// Verify scan target exists before proceeding
	var targetExists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM scan_targets WHERE id = $1)`
	err = database.QueryRowContext(ctx, checkQuery, scanTarget.ID).Scan(&targetExists)
	if err != nil {
		return fmt.Errorf("failed to verify scan target existence: %w", err)
	}
	if !targetExists {
		return fmt.Errorf("scan target %s does not exist", scanTarget.ID)
	}

	// Create scan job
	scanJob := &db.ScanJob{
		ID:       uuid.New(),
		TargetID: scanTarget.ID,
		Status:   db.ScanJobStatusCompleted,
	}

	now := time.Now()
	scanJob.StartedAt = &result.StartTime
	scanJob.CompletedAt = &now

	// Store scan statistics
	statsJSON := fmt.Sprintf(`{"hosts_up": %d, "hosts_down": %d, "total_hosts": %d, "duration_seconds": %d}`,
		result.Stats.Up, result.Stats.Down, result.Stats.Total, int(result.Duration.Seconds()))
	scanJob.ScanStats = db.JSONB(statsJSON)

	jobRepo := db.NewScanJobRepository(database)
	if err := jobRepo.Create(ctx, scanJob); err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}

	// Store host and port scan results
	err = storeHostResults(ctx, database, scanJob.ID, result.Hosts)
	if err != nil {
		return err
	}

	return nil
}

// createOrGetScanTarget creates or retrieves a scan target for the given configuration.
func createOrGetScanTarget(ctx context.Context, database *db.DB, config *ScanConfig) (*db.ScanTarget, error) {
	targetRepo := db.NewScanTargetRepository(database)

	// Try to find existing target by checking if any target contains our first IP
	if len(config.Targets) == 0 {
		return nil, fmt.Errorf("no targets specified")
	}

	firstTarget := config.Targets[0]
	ip := net.ParseIP(firstTarget)
	if ip == nil {
		// If it's not an IP, treat it as hostname for now
		// For simplicity, create a /32 network for the resolved IP
		return createAdhocScanTarget(ctx, targetRepo, firstTarget, config)
	}

	// Create /32 network for single IP
	var network string
	if ip.To4() != nil {
		network = ip.String() + "/32"
	} else {
		network = ip.String() + "/128"
	}

	return createAdhocScanTarget(ctx, targetRepo, network, config)
}

// parseTargetAddress parses a target string as CIDR, IP address, or hostname.
func parseTargetAddress(target string) (db.NetworkAddr, error) {
	var networkAddr db.NetworkAddr

	if strings.Contains(target, "/") {
		_, ipnet, err := net.ParseCIDR(target)
		if err != nil {
			return networkAddr, fmt.Errorf("invalid CIDR notation: %w", err)
		}
		networkAddr.IPNet = *ipnet
		return networkAddr, nil
	}

	// Try to parse as IP address first
	ip := net.ParseIP(target)
	if ip == nil {
		// For hostnames, create a placeholder network
		// This will be resolved during scanning
		ip = net.ParseIP("0.0.0.0")
	}

	// Create /32 or /128 mask for single IP
	var mask net.IPMask
	if ip.To4() != nil {
		mask = net.CIDRMask(32, 32)
	} else {
		mask = net.CIDRMask(ipv6CIDRBits, ipv6CIDRBits)
	}
	networkAddr.IPNet = net.IPNet{IP: ip, Mask: mask}
	return networkAddr, nil
}

// createAdhocScanTarget creates a temporary scan target for ad-hoc scans.
func createAdhocScanTarget(ctx context.Context, targetRepo *db.ScanTargetRepository,
	target string, config *ScanConfig) (*db.ScanTarget, error) {
	// Parse target address
	networkAddr, err := parseTargetAddress(target)
	if err != nil {
		return nil, err
	}

	// Map scan types to database-compatible values
	dbScanType := config.ScanType
	switch config.ScanType {
	case "comprehensive", "intense":
		dbScanType = "version" // Map complex scan types to version detection
	case "stealth":
		dbScanType = "connect" // Map stealth to basic connect scan
	}

	scanTarget := &db.ScanTarget{
		ID:                  uuid.New(),
		Name:                fmt.Sprintf("Ad-hoc scan: %s", target),
		Network:             networkAddr,
		ScanIntervalSeconds: 0, // Ad-hoc scans don't repeat
		ScanPorts:           config.Ports,
		ScanType:            dbScanType,
		Enabled:             false, // Ad-hoc targets are not scheduled
	}

	if err := targetRepo.Create(ctx, scanTarget); err != nil {
		return nil, fmt.Errorf("failed to create scan target: %w", err)
	}

	return scanTarget, nil
}

// processHostForScan processes a single host for scanning, preserving discovery data.
// Uses transaction-safe approach to handle race conditions between discovery and scan.
func processHostForScan(ctx context.Context, database *db.DB, hostRepo *db.HostRepository,
	host Host, jobID uuid.UUID) ([]*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}

	// Use transaction-safe host lookup with retries for CI consistency
	dbHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, host)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create host %s: %w", host.Address, err)
	}

	// Final verification that host exists before creating port scans
	if err := verifyHostExists(ctx, database, dbHost.ID); err != nil {
		return nil, fmt.Errorf("host verification failed for %s: %w", host.Address, err)
	}

	// Create port scan records
	portScans := make([]*db.PortScan, 0, len(host.Ports))
	for _, port := range host.Ports {
		portScan := &db.PortScan{
			ID:       uuid.New(),
			JobID:    jobID,
			HostID:   dbHost.ID,
			Port:     int(port.Number),
			Protocol: port.Protocol,
			State:    port.State,
		}

		if port.Service != "" {
			portScan.ServiceName = &port.Service
		}
		if port.Version != "" {
			portScan.ServiceVersion = &port.Version
		}
		if port.ServiceInfo != "" {
			portScan.ServiceProduct = &port.ServiceInfo
		}

		portScans = append(portScans, portScan)
	}

	return portScans, nil
}

// getOrCreateHostSafely performs transaction-safe host lookup with retries
// to handle race conditions between discovery and scan operations.
func getOrCreateHostSafely(ctx context.Context, database *db.DB, hostRepo *db.HostRepository,
	ipAddr db.IPAddr, host Host) (*db.Host, error) {
	const maxRetries = 5                      // Increased for CI stability
	const retryDelay = 200 * time.Millisecond // Increased delay for CI

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Use serializable isolation for CI consistency
		tx, err := database.BeginTxx(ctx, &sql.TxOptions{
			Isolation: sql.LevelSerializable,
		})
		if err != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to begin transaction after %d attempts: %w", maxRetries, err)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Try to find existing host within transaction with row lock
		var existingHost db.Host
		query := `SELECT * FROM hosts WHERE ip_address = $1 FOR UPDATE`
		err = tx.QueryRowContext(ctx, query, ipAddr).Scan(
			&existingHost.ID, &existingHost.IPAddress, &existingHost.Hostname,
			&existingHost.MACAddress, &existingHost.Vendor, &existingHost.OSFamily,
			&existingHost.OSName, &existingHost.OSVersion, &existingHost.OSConfidence,
			&existingHost.OSDetectedAt, &existingHost.OSMethod, &existingHost.OSDetails,
			&existingHost.DiscoveryMethod, &existingHost.ResponseTimeMS,
			&existingHost.DiscoveryCount, &existingHost.IgnoreScanning,
			&existingHost.FirstSeen, &existingHost.LastSeen, &existingHost.Status)

		if err == nil {
			// Found existing host - handle commit and return
			result, commitErr := handleHostCommit(tx, &existingHost, host)
			if commitErr != nil {
				if attempt == maxRetries {
					return nil, commitErr
				}
				time.Sleep(retryDelay * time.Duration(attempt)) // Exponential backoff
				continue
			}
			return result, nil
		}

		// Host not found - rollback read transaction and create new host
		_ = tx.Rollback()

		if err.Error() == "sql: no rows in result set" {
			if attempt == maxRetries {
				// Create new host if all retries exhausted
				newHost := &db.Host{
					ID:        uuid.New(),
					IPAddress: ipAddr,
					Status:    host.Status,
					// Note: discovery_method will be NULL for hosts created during scan
				}

				// Create the new host with CreateOrUpdate to handle potential race condition
				if createErr := hostRepo.CreateOrUpdate(ctx, newHost); createErr != nil {
					return nil, fmt.Errorf("failed to create new host: %w", createErr)
				}

				return newHost, nil
			}
		}

		// Other error - rollback and retry
		_ = tx.Rollback()
		if attempt == maxRetries {
			return nil, fmt.Errorf("failed to query host after %d attempts: %w", maxRetries, err)
		}
		time.Sleep(retryDelay * time.Duration(attempt))
	}

	return nil, fmt.Errorf("failed to get or create host after %d attempts", maxRetries)
}

// verifyHostExists ensures the host record exists before creating port scans
// to prevent foreign key constraint violations.
func verifyHostExists(ctx context.Context, database *db.DB, hostID uuid.UUID) error {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)`

	err := database.QueryRowContext(ctx, query, hostID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to verify host existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("host with ID %s does not exist", hostID.String())
	}

	return nil
}

// handleHostCommit handles the transaction commit for found hosts.
func handleHostCommit(tx *sqlx.Tx, existingHost *db.Host, host Host) (*db.Host, error) {
	if commitErr := tx.Commit(); commitErr != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("failed to commit read transaction: %w", commitErr)
	}

	// Update status but preserve discovery data
	existingHost.Status = host.Status
	return existingHost, nil
}

// storeHostResults stores host and port scan results in the database.
func storeHostResults(ctx context.Context, database *db.DB, jobID uuid.UUID, hosts []Host) error {
	// Verify job exists before processing hosts
	var jobExists bool
	jobCheckQuery := `SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)`
	err := database.QueryRowContext(ctx, jobCheckQuery, jobID).Scan(&jobExists)
	if err != nil {
		return fmt.Errorf("failed to verify scan job existence: %w", err)
	}
	if !jobExists {
		return fmt.Errorf("scan job %s does not exist, cannot store host results", jobID)
	}

	hostRepo := db.NewHostRepository(database)
	portRepo := db.NewPortScanRepository(database)

	var allPortScans []*db.PortScan

	for _, host := range hosts {
		portScans, err := processHostForScan(ctx, database, hostRepo, host, jobID)
		if err != nil {
			log.Printf("Failed to process host %s: %v", host.Address, err)
			continue
		}
		allPortScans = append(allPortScans, portScans...)
	}

	// Batch insert all port scans with additional validation
	if len(allPortScans) > 0 {
		// Final verification that all required foreign keys exist
		for _, portScan := range allPortScans {
			// Verify job still exists
			err := database.QueryRowContext(ctx, jobCheckQuery, portScan.JobID).Scan(&jobExists)
			if err != nil || !jobExists {
				return fmt.Errorf("scan job %s no longer exists before port scan creation", portScan.JobID)
			}

			// Verify host exists
			var hostExists bool
			hostCheckQuery := `SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)`
			err = database.QueryRowContext(ctx, hostCheckQuery, portScan.HostID).Scan(&hostExists)
			if err != nil || !hostExists {
				return fmt.Errorf("host %s does not exist before port scan creation", portScan.HostID)
			}
		}

		if err := portRepo.CreateBatch(ctx, allPortScans); err != nil {
			return fmt.Errorf("failed to store port scan results: %w", err)
		}
	}

	return nil
}
