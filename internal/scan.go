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
	// Scan type constants.
	scanTypeConnect       = "connect"
	scanTypeVersion       = "version"
	scanTypeIntense       = "intense"
	scanTypeComprehensive = "comprehensive"
	scanTypeStealth       = "stealth"

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
	case scanTypeConnect:
		options = append(options, nmap.WithConnectScan())
	case "syn":
		options = append(options, nmap.WithSYNScan())
	case scanTypeVersion:
		options = append(options,
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
		)
	case scanTypeIntense:
		options = append(options,
			nmap.WithConnectScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithAggressiveScan(),
		)
	case scanTypeStealth:
		options = append(options,
			nmap.WithConnectScan(),
			nmap.WithTimingTemplate(nmap.TimingPolite),
		)
	case scanTypeComprehensive:
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
	// Use single transaction for all operations to prevent race conditions
	tx, err := database.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Create a scan job record - for now we'll create a minimal scan target
	scanTarget, err := createOrGetScanTargetTx(ctx, tx, config)
	if err != nil {
		return fmt.Errorf("failed to create scan target: %w", err)
	}

	// Create scan job within same transaction
	scanJob := &db.ScanJob{
		ID:       uuid.New(),
		TargetID: scanTarget.ID,
		Status:   db.ScanJobStatusCompleted,
	}

	now := time.Now()
	scanJob.StartedAt = &result.StartTime
	scanJob.CompletedAt = &now

	// Store scan statistics
	statsJSON := fmt.Sprintf(
		`{"hosts_up": %d, "hosts_down": %d, "total_hosts": %d, "duration_seconds": %d}`,
		result.Stats.Up, result.Stats.Down, result.Stats.Total, int(result.Duration.Seconds()))
	scanJob.ScanStats = db.JSONB(statsJSON)

	// Create scan job in transaction
	createJobQuery := `
		INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at, scan_stats)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = tx.ExecContext(ctx, createJobQuery, scanJob.ID, scanJob.TargetID, scanJob.Status,
		scanJob.StartedAt, scanJob.CompletedAt, scanJob.ScanStats)
	if err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}

	// Store host and port scan results in same transaction
	err = storeHostResultsTx(ctx, tx, scanJob.ID, result.Hosts)
	if err != nil {
		return err
	}

	// Commit all operations atomically
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit scan results transaction: %w", err)
	}

	return nil
}

// createOrGetScanTargetTx creates or retrieves a scan target within a transaction.
func createOrGetScanTargetTx(ctx context.Context, tx *sqlx.Tx, config *ScanConfig) (*db.ScanTarget, error) {
	// Try to find existing target by checking if any target contains our first IP
	if len(config.Targets) == 0 {
		return nil, fmt.Errorf("no targets specified")
	}

	firstTarget := config.Targets[0]
	network := resolveTargetToNetwork(firstTarget)
	return createAdhocScanTargetTx(ctx, tx, network, config)
}

// resolveTargetToNetwork converts a target (IP or hostname) to CIDR network notation.
func resolveTargetToNetwork(target string) string {
	ip := net.ParseIP(target)
	if ip == nil {
		return resolveHostnameToNetwork(target)
	}
	return ipToNetworkString(ip)
}

// resolveHostnameToNetwork resolves a hostname to CIDR network notation.
func resolveHostnameToNetwork(hostname string) string {
	ips, err := net.LookupIP(hostname)
	if err != nil || len(ips) == 0 {
		// If hostname resolution fails, use a placeholder network
		return "127.0.0.1/32"
	}
	return ipToNetworkString(ips[0])
}

// ipToNetworkString converts an IP address to CIDR network notation with /32 or /128.
func ipToNetworkString(ip net.IP) string {
	if ip.To4() != nil {
		return ip.String() + "/32"
	}
	return ip.String() + "/128"
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

// createAdhocScanTargetTx creates an ad-hoc scan target within a transaction.
func createAdhocScanTargetTx(ctx context.Context, tx *sqlx.Tx, target string,
	config *ScanConfig) (*db.ScanTarget, error) {
	// Parse target address
	networkAddr, err := parseTargetAddress(target)
	if err != nil {
		return nil, err
	}

	// Map scan types to database-compatible values
	dbScanType := config.ScanType
	switch config.ScanType {
	case scanTypeComprehensive, scanTypeIntense:
		dbScanType = scanTypeVersion // Map complex scan types to version detection
	case scanTypeStealth:
		dbScanType = scanTypeConnect // Map stealth to basic connect scan
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

	// Create target within transaction
	query := `
		INSERT INTO scan_targets (id, name, network, description, scan_interval_seconds, scan_ports, scan_type, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	err = tx.QueryRowContext(ctx, query, scanTarget.ID, scanTarget.Name, scanTarget.Network,
		scanTarget.Description, scanTarget.ScanIntervalSeconds, scanTarget.ScanPorts,
		scanTarget.ScanType, scanTarget.Enabled).Scan(&scanTarget.CreatedAt,
		&scanTarget.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create scan target: %w", err)
	}

	return scanTarget, nil
}

// processHostForScanTx processes a single host for scanning within a transaction.
func processHostForScanTx(ctx context.Context, tx *sqlx.Tx, host Host, jobID uuid.UUID) ([]*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}

	// Get or create host within transaction
	dbHost, err := getOrCreateHostSafelyTx(ctx, tx, ipAddr, host)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create host %s: %w", host.Address, err)
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

// getOrCreateHostSafelyTx gets or creates a host within a transaction.
func getOrCreateHostSafelyTx(ctx context.Context, tx *sqlx.Tx, ipAddr db.IPAddr, host Host) (*db.Host, error) {
	// Try to find existing host
	var existingHost db.Host
	query := `SELECT * FROM hosts WHERE ip_address = $1`
	err := tx.QueryRowContext(ctx, query, ipAddr).Scan(
		&existingHost.ID, &existingHost.IPAddress, &existingHost.Hostname,
		&existingHost.MACAddress, &existingHost.Vendor, &existingHost.OSFamily,
		&existingHost.OSName, &existingHost.OSVersion, &existingHost.OSConfidence,
		&existingHost.OSDetectedAt, &existingHost.OSMethod, &existingHost.OSDetails,
		&existingHost.DiscoveryMethod, &existingHost.ResponseTimeMS,
		&existingHost.DiscoveryCount, &existingHost.IgnoreScanning,
		&existingHost.FirstSeen, &existingHost.LastSeen, &existingHost.Status)

	if err == nil {
		// Found existing host - update status and return
		existingHost.Status = host.Status
		return &existingHost, nil
	}

	// Host not found - create new one
	if err.Error() == "sql: no rows in result set" {
		newHost := &db.Host{
			ID:        uuid.New(),
			IPAddress: ipAddr,
			Status:    host.Status,
		}

		// Insert new host within transaction
		insertQuery := `
			INSERT INTO hosts (id, ip_address, status, first_seen, last_seen)
			VALUES ($1, $2, $3, NOW(), NOW())
			RETURNING first_seen, last_seen`

		err = tx.QueryRowContext(ctx, insertQuery, newHost.ID, newHost.IPAddress, newHost.Status).Scan(
			&newHost.FirstSeen, &newHost.LastSeen)
		if err != nil {
			return nil, fmt.Errorf("failed to create host: %w", err)
		}

		return newHost, nil
	}

	return nil, fmt.Errorf("failed to query host: %w", err)
}

// storeHostResultsTx stores host and port scan results within a transaction.
func storeHostResultsTx(ctx context.Context, tx *sqlx.Tx, jobID uuid.UUID, hosts []Host) error {
	var allPortScans []*db.PortScan

	for _, host := range hosts {
		portScans, err := processHostForScanTx(ctx, tx, host, jobID)
		if err != nil {
			log.Printf("Failed to process host %s: %v", host.Address, err)
			continue
		}
		allPortScans = append(allPortScans, portScans...)
	}

	// Batch insert all port scans within transaction
	if len(allPortScans) > 0 {
		for _, scan := range allPortScans {
			if scan.ID == uuid.Nil {
				scan.ID = uuid.New()
			}

			query := `
				INSERT INTO port_scans (
					id, job_id, host_id, port, protocol, state,
					service_name, service_version, service_product, banner
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

			_, err := tx.ExecContext(ctx, query, scan.ID, scan.JobID, scan.HostID,
				scan.Port, scan.Protocol, scan.State, scan.ServiceName,
				scan.ServiceVersion, scan.ServiceProduct, scan.Banner)
			if err != nil {
				return fmt.Errorf("failed to create port scan: %w", err)
			}
		}
	}

	return nil
}
