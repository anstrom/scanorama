// Package scanning provides core scanning functionality and shared types for scanorama.
// It contains scan execution logic, result processing, XML handling,
// and common data structures used throughout the application.
package scanning

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/google/uuid"
)

// Constants for scan configuration validation.
const (
	minTimeoutSeconds     = 5
	mediumTimeoutSeconds  = 15
	maxConcurrency        = 20
	ipv6CIDRBits          = 128
	defaultTargetCapacity = 10
	nullMethodValue       = "NULL"
	scanTypeConnect       = "connect"
	scanTypeAggressive    = "aggressive"
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

	// Resolve scan type, downgrading to connect scan when root is required but not available.
	config.ScanType = resolveScanType(config.ScanType)

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
		logging.Warn("Scan completed with warnings", "warnings", *warnings)
	}

	return result, nil
}

// buildScanOptions creates nmap options based on scan configuration.
func buildScanOptions(config *ScanConfig) []nmap.Option {
	options := []nmap.Option{
		nmap.WithTargets(config.Targets...),
		nmap.WithPorts(config.Ports),
	}

	// Mixed-protocol support: if the port spec contains UDP ports (e.g. "T:22,80,U:53,161"),
	// add an explicit UDP scan so nmap handles both TCP and UDP in a single run.
	if strings.Contains(config.Ports, "U:") {
		options = append(options, nmap.WithUDPScan())
	}

	// Add scan type options with enhanced capabilities
	switch config.ScanType {
	case scanTypeConnect:
		options = append(options, nmap.WithConnectScan())
	case "syn":
		options = append(options, nmap.WithSYNScan())
	case "ack":
		options = append(options, nmap.WithACKScan())
	case "udp":
		options = append(options, nmap.WithUDPScan())
	case scanTypeAggressive:
		options = append(options,
			nmap.WithSYNScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithAggressiveScan(),
		)
	case "comprehensive":
		options = append(options,
			nmap.WithSYNScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithDefaultScript(),
		)
	}

	if config.OSDetection {
		options = append(options, nmap.WithOSDetection())
	}

	// Add nmap timing template. The Timing field (set from the scan profile) takes
	// precedence. If not set, fall back to a concurrency-based heuristic.
	switch config.Timing {
	case "paranoid":
		options = append(options, nmap.WithTimingTemplate(nmap.TimingSlowest))
	case "polite":
		options = append(options, nmap.WithTimingTemplate(nmap.TimingSneaky))
	case "normal":
		options = append(options, nmap.WithTimingTemplate(nmap.TimingNormal))
	case scanTypeAggressive:
		options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
	case "insane":
		options = append(options, nmap.WithTimingTemplate(nmap.TimingFastest))
	default:
		// No explicit timing — apply T4 when high concurrency is requested,
		// otherwise leave nmap to use its own default (T3).
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

// resolveScanType returns the scan type to use, falling back to "connect" when the
// requested type requires raw-socket / root privileges and the process is not root.
func resolveScanType(requested string) string {
	rootRequired := map[string]bool{
		"syn":              true,
		scanTypeAggressive: true,
		"comprehensive":    true,
	}
	if rootRequired[requested] && os.Getuid() != 0 {
		logging.Warn("scan type requires root privileges, falling back to connect scan",
			"requested", requested,
			"effective", scanTypeConnect)
		return scanTypeConnect
	}
	return requested
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
	logging.Debug("Storing scan results", "targets", config.Targets)

	// Create a scan job record - for now we'll create a minimal scan target
	scanTarget, err := createOrGetScanTarget(ctx, database, config)
	if err != nil {
		logging.Error("Failed to create scan target", "error", err)
		return fmt.Errorf("failed to create scan target: %w", err)
	}
	logging.Debug("Created/retrieved scan target", "target_id", scanTarget.ID, "target_name", scanTarget.Name)

	// Create scan job
	scanJob := &db.ScanJob{
		ID:       uuid.New(),
		TargetID: scanTarget.ID,
		Status:   db.ScanJobStatusCompleted,
	}
	logging.Debug("Creating scan job", "job_id", scanJob.ID, "target_id", scanJob.TargetID)

	now := time.Now()
	scanJob.StartedAt = &result.StartTime
	scanJob.CompletedAt = &now

	// Store scan statistics
	statsJSON := fmt.Sprintf(`{"hosts_up": %d, "hosts_down": %d, "total_hosts": %d, "duration_seconds": %d}`,
		result.Stats.Up, result.Stats.Down, result.Stats.Total, int(result.Duration.Seconds()))
	scanJob.ScanStats = db.JSONB(statsJSON)

	jobRepo := db.NewScanJobRepository(database)
	if err := jobRepo.Create(ctx, scanJob); err != nil {
		logging.Error("Failed to create scan job", "error", err)
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	logging.Debug("Successfully created scan job", "job_id", scanJob.ID)

	// Store host and port scan results
	logging.Debug("Storing host results", "host_count", len(result.Hosts))
	err = storeHostResults(ctx, database, scanJob.ID, result.Hosts)
	if err != nil {
		logging.Error("Failed to store host results", "error", err)
		return err
	}
	logging.Debug("Successfully stored all scan results")
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
		// Require at least one dot so bare words like "not-an-ip-address"
		// are rejected while FQDNs like "example.com" are accepted.
		if !strings.Contains(target, ".") {
			return networkAddr, fmt.Errorf("invalid target address: %q is not a valid IP, CIDR, or hostname", target)
		}
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
	logging.Debug("Creating ad-hoc scan target", "target", target)

	// Parse target address
	networkAddr, err := parseTargetAddress(target)
	if err != nil {
		logging.Error("Failed to parse target address", "target", target, "error", err)
		return nil, err
	}
	logging.Debug("Parsed target address", "target", target, "network", networkAddr.String())

	// Map scan types to database-compatible values
	dbScanType := config.ScanType
	switch config.ScanType {
	case "comprehensive", scanTypeAggressive:
		dbScanType = "version" // Map complex scan types to version detection
	case "stealth":
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

	if err := targetRepo.Create(ctx, scanTarget); err != nil {
		logging.Error("Failed to create scan target", "error", err)
		return nil, fmt.Errorf("failed to create scan target: %w", err)
	}
	logging.Debug("Successfully created scan target", "target_id", scanTarget.ID, "target_name", scanTarget.Name)

	return scanTarget, nil
}

// debugHostLookup performs detailed debugging of host lookup process.
func debugHostLookup(ctx context.Context, database *db.DB, hostAddress string, ipAddr db.IPAddr) {
	logging.Debug("Looking up host", "address", hostAddress, "ip", ipAddr.IP)

	// Debug: Check what hosts actually exist in database
	var hostCount int
	countQuery := `SELECT COUNT(*) FROM hosts WHERE ip_address::text LIKE '%' || $1 || '%'`
	if err := database.QueryRowContext(ctx, countQuery, hostAddress).Scan(&hostCount); err == nil {
		logging.Debug("Found hosts with IP", "count", hostCount, "address", hostAddress)
	}
}

// processHostForScan processes a single host for scanning, preserving discovery data.
// Uses transaction-safe approach to handle race conditions between discovery and scan.
func processHostForScan(ctx context.Context, database *db.DB, hostRepo *db.HostRepository,
	host Host, jobID uuid.UUID) ([]*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}
	debugHostLookup(ctx, database, host.Address, ipAddr)

	dbHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, host)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create host %s: %w", host.Address, err)
	}

	logging.Debug("Using host for scan",
		"address", host.Address,
		"id", dbHost.ID.String(),
		"discovery_method", func() string {
			if dbHost.DiscoveryMethod != nil {
				return *dbHost.DiscoveryMethod
			}
			return nullValue
		}())

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

// getOrCreateHostSafely looks up a host by IP address and creates it if it does
// not yet exist. It uses the repository's GetByIP (sqlx tag-based scanning) so
// there is no dependency on physical column order in the hosts table.
func getOrCreateHostSafely(ctx context.Context, _ *db.DB, hostRepo *db.HostRepository,
	ipAddr db.IPAddr, host Host) (*db.Host, error) {
	logging.Debug("Looking up host by IP", "ip", ipAddr.String())

	existing, err := hostRepo.GetByIP(ctx, ipAddr)
	if err == nil {
		// Host already exists — update status but preserve all discovery data.
		existing.Status = host.Status
		logging.Debug("Found existing host",
			"ip", ipAddr.String(),
			"id", existing.ID.String(),
			"discovery_method", func() string {
				if existing.DiscoveryMethod != nil {
					return *existing.DiscoveryMethod
				}
				return nullValue
			}())
		return existing, nil
	}

	// Any error other than not-found is unexpected.
	if !isNotFoundError(err) {
		return nil, fmt.Errorf("failed to look up host %s: %w", ipAddr, err)
	}

	// Host does not exist yet — create it.
	logging.Debug("Creating new host", "ip", ipAddr.String())
	newHost := &db.Host{
		ID:        uuid.New(),
		IPAddress: ipAddr,
		Status:    host.Status,
	}
	if createErr := hostRepo.CreateOrUpdate(ctx, newHost); createErr != nil {
		return nil, fmt.Errorf("failed to create new host %s: %w", ipAddr, createErr)
	}

	logging.Debug("Successfully created new host", "ip", ipAddr.String())
	return newHost, nil
}

// isNotFoundError reports whether err represents a "not found" condition
// returned by the db layer (wraps sql.ErrNoRows or our own CodeNotFound error).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Our sanitizeDBError wraps sql.ErrNoRows as errors.DatabaseError with CodeNotFound.
	var dbErr *errors.DatabaseError
	if stderrors.As(err, &dbErr) {
		return dbErr.Code == errors.CodeNotFound
	}
	return false
}

// storeHostResults stores host and port scan results in the database.
func storeHostResults(ctx context.Context, database *db.DB, jobID uuid.UUID, hosts []Host) error {
	hostRepo := db.NewHostRepository(database)
	portRepo := db.NewPortScanRepository(database)

	var allPortScans []*db.PortScan

	for _, host := range hosts {
		portScans, err := processHostForScan(ctx, database, hostRepo, host, jobID)
		if err != nil {
			logging.Error("Failed to process host", "address", host.Address, "error", err)
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
