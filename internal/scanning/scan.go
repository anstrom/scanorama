// Package scanning provides core scanning functionality and shared types for scanorama.
// It contains scan execution logic, result processing, XML handling,
// and common data structures used throughout the application.
package scanning

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
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
	nullMethodValue       = "NULL"
)

const (
	// Database null value representation.
	nullValue = "NULL"

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
    // Start timing for Prometheus metrics
    scanStart := time.Now()
    defer func() {
        metrics.GetGlobalMetrics().RecordScanDuration(config.ScanType, time.Since(scanStart))
    }()

	// Log scan start
	logging.Info("Starting scan operation",
		"scan_type", config.ScanType,
		"target_count", len(config.Targets),
		"ports", config.Ports)

	// Validate the configuration
	if err := validateScanConfig(config); err != nil {
        metrics.GetGlobalMetrics().IncrementScanErrors(config.ScanType, "config_invalid")
        return nil, errors.WrapScanError(errors.CodeValidation, "invalid configuration", err)
	}

	// Apply timeout if specified
	if config.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(config.TimeoutSec)*time.Second)
		defer cancel()
	}

	// Initialize scan result with start time
	scanResult := NewScanResult()
	defer func() {
		scanResult.Complete()
		// Record scan completion metrics
        status := "success"
        if scanResult.Error != nil {
            status = "error"
        }
        metrics.GetGlobalMetrics().IncrementScansTotal(config.ScanType, status)
		logging.Info("Scan operation completed",
			"scan_type", config.ScanType,
			"duration", scanResult.Duration,
			"hosts_scanned", len(scanResult.Hosts),
			"status", status)
	}()

	// Create and run scanner
	result, err := createAndRunScanner(ctx, config)
	if err != nil {
		// Check if this is a timeout error and preserve that information
		if strings.Contains(err.Error(), "timed out") || ctx.Err() == context.DeadlineExceeded {
            metrics.GetGlobalMetrics().IncrementScanErrors(config.ScanType, "timeout")
			logging.Error("Scanner execution timed out", "scan_type", config.ScanType, "error", err)
			return nil, errors.WrapScanError(errors.CodeTimeout, "scan operation timed out", err)
		}

        metrics.GetGlobalMetrics().IncrementScanErrors(config.ScanType, "execution_failed")
		logging.Error("Scanner execution failed", "scan_type", config.ScanType, "error", err)
		return nil, errors.WrapScanError(errors.CodeScanFailed, "scanner execution failed", err)
	}

	// Convert results to our format
	convertNmapResults(result, scanResult)

	// Store results in database if provided
	if database != nil {
		if err := storeScanResults(ctx, database, config, scanResult); err != nil {
			logging.ErrorDatabase("Failed to store scan results", err,
				"scan_type", config.ScanType,
				"host_count", len(scanResult.Hosts))
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
	case "aggressive":
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

	// Add performance optimizations
	if config.Concurrency > 0 {
		// nmap library doesn't directly expose parallelism, but we can use timing
		if config.Concurrency > maxConcurrency {
			options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
		}
	}

	// Add host discovery options for better reliability and useful scanning options
	options = append(options,
		nmap.WithSkipHostDiscovery(), // Skip ping and go straight to port scan
		nmap.WithVerbosity(1),        // Basic verbosity for better debugging
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
	log.Printf("DEBUG: Scan storing results for targets %v", config.Targets)

	// Create a scan job record - for now we'll create a minimal scan target
	scanTarget, err := createOrGetScanTarget(ctx, database, config)
	if err != nil {
		log.Printf("DEBUG: Scan failed to create scan target: %v", err)
		return fmt.Errorf("failed to create scan target: %w", err)
	}
	log.Printf("DEBUG: Scan created/retrieved scan target ID=%s, name=%s", scanTarget.ID, scanTarget.Name)

	// Create scan job
	scanJob := &db.ScanJob{
		ID:       uuid.New(),
		TargetID: scanTarget.ID,
		Status:   db.ScanJobStatusCompleted,
	}
	log.Printf("DEBUG: Scan creating scan job ID=%s for target ID=%s", scanJob.ID, scanJob.TargetID)

	now := time.Now()
	scanJob.StartedAt = &result.StartTime
	scanJob.CompletedAt = &now

	// Store scan statistics
	statsJSON := fmt.Sprintf(`{"hosts_up": %d, "hosts_down": %d, "total_hosts": %d, "duration_seconds": %d}`,
		result.Stats.Up, result.Stats.Down, result.Stats.Total, int(result.Duration.Seconds()))
	scanJob.ScanStats = db.JSONB(statsJSON)

	jobRepo := db.NewScanJobRepository(database)
	if err := jobRepo.Create(ctx, scanJob); err != nil {
		log.Printf("DEBUG: Scan failed to create scan job: %v", err)
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	log.Printf("DEBUG: Scan successfully created scan job ID=%s", scanJob.ID)

	// Store host and port scan results
	log.Printf("DEBUG: Scan storing results for %d hosts", len(result.Hosts))
	err = storeHostResults(ctx, database, scanJob.ID, result.Hosts)
	if err != nil {
		log.Printf("DEBUG: Scan failed to store host results: %v", err)
		return err
	}
	log.Printf("DEBUG: Scan successfully stored all results")
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
	log.Printf("DEBUG: Scan creating ad-hoc scan target for %s", target)

	// Parse target address
	networkAddr, err := parseTargetAddress(target)
	if err != nil {
		log.Printf("DEBUG: Scan failed to parse target address %s: %v", target, err)
		return nil, err
	}
	log.Printf("DEBUG: Scan parsed target %s as network %s", target, networkAddr.String())

	// Map scan types to database-compatible values
	dbScanType := config.ScanType
	switch config.ScanType {
	case "comprehensive", "aggressive":
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
		log.Printf("DEBUG: Scan failed to create scan target: %v", err)
		return nil, fmt.Errorf("failed to create scan target: %w", err)
	}
	log.Printf("DEBUG: Scan successfully created scan target ID=%s, name=%s", scanTarget.ID, scanTarget.Name)

	return scanTarget, nil
}

// debugHostLookup performs detailed debugging of host lookup process.
func debugHostLookup(ctx context.Context, database *db.DB, hostAddress string, ipAddr db.IPAddr) {
	log.Printf("DEBUG: Scan looking up host %s with parsed IP %v (type: %T)",
		hostAddress, ipAddr.IP, ipAddr.IP)

	// Debug: Check what hosts actually exist in database
	var hostCount int
	countQuery := `SELECT COUNT(*) FROM hosts WHERE ip_address::text LIKE '%' || $1 || '%'`
	if err := database.QueryRowContext(ctx, countQuery, hostAddress).Scan(&hostCount); err == nil {
		log.Printf("DEBUG: Scan found %d hosts containing IP %s", hostCount, hostAddress)
	}
}

// debugListExistingHosts lists current hosts in database for debugging.
func debugListExistingHosts(ctx context.Context, database *db.DB) {
	var debugHosts []struct {
		IP     string  `db:"ip_address"`
		Method *string `db:"discovery_method"`
		ID     string  `db:"id"`
	}
	debugQuery := `SELECT ip_address::text, discovery_method, id::text FROM hosts LIMIT 5`
	if err := database.SelectContext(ctx, &debugHosts, debugQuery); err == nil {
		log.Printf("DEBUG: Scan - Current hosts in database:")
		for _, h := range debugHosts {
			method := nullValue
			if h.Method != nil {
				method = *h.Method
			}
			log.Printf("DEBUG: Scan -   Host IP=%s, Method=%s, ID=%s", h.IP, method, h.ID)
		}
	}
}

// processHostForScan processes a single host for scanning, preserving discovery data.
// Uses transaction-safe approach to handle race conditions between discovery and scan.
func processHostForScan(ctx context.Context, database *db.DB, hostRepo *db.HostRepository,
	host Host, jobID uuid.UUID) ([]*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}
	debugHostLookup(ctx, database, host.Address, ipAddr)

	// Use transaction-safe host lookup with retries for CI consistency
	dbHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, host)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create host %s: %w", host.Address, err)
	}

	log.Printf("DEBUG: Scan using host %s (ID=%s) with discovery_method=%v",
		host.Address, dbHost.ID.String(),
		func() string {
			if dbHost.DiscoveryMethod != nil {
				return *dbHost.DiscoveryMethod
			}
			return nullValue
		}())

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
	const maxRetries = 3
	const retryDelay = 100 * time.Millisecond

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("DEBUG: Scan attempting host lookup %s (attempt %d/%d)", ipAddr.String(), attempt, maxRetries)

		// Use explicit transaction for consistent reads
		tx, err := database.BeginTx(ctx)
		if err != nil {
			log.Printf("DEBUG: Scan failed to begin transaction for %s: %v", ipAddr.String(), err)
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to begin transaction after %d attempts: %w", maxRetries, err)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Try to find existing host within transaction
		var existingHost db.Host
		query := `SELECT * FROM hosts WHERE ip_address = $1`
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
			result, commitErr := handleHostCommit(tx, &existingHost, host, ipAddr)
			if commitErr != nil {
				if attempt == maxRetries {
					return nil, commitErr
				}
				time.Sleep(retryDelay)
				continue
			}
			return result, nil
		}

		// Host not found - rollback read transaction
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Printf("DEBUG: Scan failed to rollback read transaction for %s: %v", ipAddr.String(), rollbackErr)
		}

		if attempt == maxRetries {
			// Create new host if all retries exhausted
			log.Printf("DEBUG: Scan creating new host for %s after %d lookup attempts", ipAddr.String(), maxRetries)
			debugListExistingHosts(ctx, database)

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

			log.Printf("DEBUG: Scan successfully created new host %s", ipAddr.String())
			return newHost, nil
		}

		log.Printf("DEBUG: Scan host lookup attempt %d failed for %s, retrying...", attempt, ipAddr.String())
		time.Sleep(retryDelay)
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

	log.Printf("DEBUG: Scan verified host %s exists in database", hostID.String())
	return nil
}

// handleHostCommit handles the transaction commit for found hosts.
func handleHostCommit(tx *sqlx.Tx, existingHost *db.Host, host Host, ipAddr db.IPAddr) (*db.Host, error) {
	if commitErr := tx.Commit(); commitErr != nil {
		log.Printf("DEBUG: Scan failed to commit read transaction for %s: %v", ipAddr.String(), commitErr)
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Printf("DEBUG: Scan failed to rollback after commit error for %s: %v",
				ipAddr.String(), rollbackErr)
		}
		return nil, fmt.Errorf("failed to commit read transaction: %w", commitErr)
	}

	log.Printf("DEBUG: Scan found existing host %s (ID=%s) with discovery_method=%v",
		ipAddr.String(), existingHost.ID.String(),
		func() string {
			if existingHost.DiscoveryMethod != nil {
				return *existingHost.DiscoveryMethod
			}
			return nullValue
		}())

	// Update status but preserve discovery data
	existingHost.Status = host.Status
	return existingHost, nil
}

// storeHostResults stores host and port scan results in the database.
func storeHostResults(ctx context.Context, database *db.DB, jobID uuid.UUID, hosts []Host) error {
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

	// Batch insert all port scans
	if len(allPortScans) > 0 {
		if err := portRepo.CreateBatch(ctx, allPortScans); err != nil {
			return fmt.Errorf("failed to store port scan results: %w", err)
		}
	}

	return nil
}
