// Package internal provides core scanning functionality and shared types for scanorama.
// It contains scan execution logic, result processing, XML handling,
// and common data structures used throughout the application.
package internal

import (
	"context"
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
		nmap.WithVerbosity(1), // Basic verbosity for better debugging
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

	// Start a transaction to ensure scan job and port scans are created atomically
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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

	// Create scan job within the transaction
	jobQuery := `
		INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at, scan_stats)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at`

	err = tx.QueryRowContext(ctx, jobQuery,
		scanJob.ID, scanJob.TargetID, scanJob.Status,
		scanJob.StartedAt, scanJob.CompletedAt, scanJob.ScanStats).Scan(&scanJob.CreatedAt)
	if err != nil {
		log.Printf("DEBUG: Scan failed to create scan job: %v", err)
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	log.Printf("DEBUG: Scan successfully created scan job ID=%s", scanJob.ID)

	// Store host and port scan results within the same transaction
	log.Printf("DEBUG: Scan storing results for %d hosts", len(result.Hosts))
	err = storeHostResultsInTransaction(ctx, tx, scanJob.ID, result.Hosts)
	if err != nil {
		log.Printf("DEBUG: Scan failed to store host results: %v", err)
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit scan results transaction: %w", err)
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
		log.Printf("DEBUG: Scan failed to create scan target: %v", err)
		return nil, fmt.Errorf("failed to create scan target: %w", err)
	}
	log.Printf("DEBUG: Scan successfully created scan target ID=%s, name=%s", scanTarget.ID, scanTarget.Name)

	return scanTarget, nil
}

// processHostForScanInTransaction processes a single host for scanning within an existing transaction.
func processHostForScanInTransaction(ctx context.Context, tx *sqlx.Tx,
	host Host, jobID uuid.UUID) ([]*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}

	// Get or create host within the transaction
	var dbHost *db.Host
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM hosts WHERE ip_address = $1)`
	err := tx.QueryRowContext(ctx, checkQuery, ipAddr).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check host existence: %w", err)
	}

	if exists {
		// Get existing host
		getQuery := `SELECT id, ip_address, hostname, mac_address, vendor, os_family, os_name,
			os_version, os_confidence, os_detected_at, os_method, os_details, discovery_method,
			response_time_ms, discovery_count, ignore_scanning, first_seen, last_seen, status
			FROM hosts WHERE ip_address = $1`
		dbHost = &db.Host{}
		err = tx.QueryRowContext(ctx, getQuery, ipAddr).Scan(
			&dbHost.ID, &dbHost.IPAddress, &dbHost.Hostname, &dbHost.MACAddress,
			&dbHost.Vendor, &dbHost.OSFamily, &dbHost.OSName, &dbHost.OSVersion,
			&dbHost.OSConfidence, &dbHost.OSDetectedAt, &dbHost.OSMethod, &dbHost.OSDetails,
			&dbHost.DiscoveryMethod, &dbHost.ResponseTimeMS, &dbHost.DiscoveryCount,
			&dbHost.IgnoreScanning, &dbHost.FirstSeen, &dbHost.LastSeen, &dbHost.Status)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing host: %w", err)
		}
	} else {
		// Create new host within transaction
		dbHost = &db.Host{
			ID:        uuid.New(),
			IPAddress: ipAddr,
			Status:    host.Status,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}

		insertQuery := `INSERT INTO hosts (id, ip_address, status, first_seen, last_seen)
			VALUES ($1, $2, $3, $4, $5) RETURNING created_at`
		err = tx.QueryRowContext(ctx, insertQuery,
			dbHost.ID, dbHost.IPAddress, dbHost.Status, dbHost.FirstSeen, dbHost.LastSeen).Scan(&dbHost.FirstSeen)
		if err != nil {
			return nil, fmt.Errorf("failed to create host: %w", err)
		}
	}

	log.Printf("DEBUG: Scan using host %s (ID=%s)", host.Address, dbHost.ID.String())

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

// createPortScansInTransaction creates port scans within an existing transaction.
func createPortScansInTransaction(ctx context.Context, tx *sqlx.Tx, scans []*db.PortScan) error {
	if len(scans) == 0 {
		return nil
	}

	query := `
		INSERT INTO port_scans (
			id, job_id, host_id, port, protocol, state,
			service_name, service_version, service_product, banner
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		ON CONFLICT (job_id, host_id, port, protocol)
		DO UPDATE SET
			state = EXCLUDED.state,
			service_name = EXCLUDED.service_name,
			service_version = EXCLUDED.service_version,
			service_product = EXCLUDED.service_product,
			banner = EXCLUDED.banner,
			scanned_at = NOW()`

	for _, scan := range scans {
		if scan.ID == uuid.Nil {
			scan.ID = uuid.New()
		}

		_, err := tx.ExecContext(ctx, query,
			scan.ID, scan.JobID, scan.HostID, scan.Port, scan.Protocol, scan.State,
			scan.ServiceName, scan.ServiceVersion, scan.ServiceProduct, scan.Banner)
		if err != nil {
			return fmt.Errorf("failed to create port scan for host %s port %d: %w",
				scan.HostID, scan.Port, err)
		}
	}

	return nil
}

// storeHostResultsInTransaction stores host and port scan results within an existing transaction.
func storeHostResultsInTransaction(ctx context.Context, tx *sqlx.Tx, jobID uuid.UUID, hosts []Host) error {
	var allPortScans []*db.PortScan

	for _, host := range hosts {
		portScans, err := processHostForScanInTransaction(ctx, tx, host, jobID)
		if err != nil {
			log.Printf("Failed to process host %s: %v", host.Address, err)
			continue
		}
		allPortScans = append(allPortScans, portScans...)
	}

	// Batch insert all port scans within the transaction
	if len(allPortScans) > 0 {
		if err := createPortScansInTransaction(ctx, tx, allPortScans); err != nil {
			return fmt.Errorf("failed to store port scan results: %w", err)
		}
	}

	return nil
}
