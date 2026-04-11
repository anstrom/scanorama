// Package scanning provides core scanning functionality and shared types for scanorama.
// It contains scan execution logic, result processing, XML handling,
// and common data structures used throughout the application.
package scanning

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	internaldns "github.com/anstrom/scanorama/internal/dns"
	"github.com/anstrom/scanorama/internal/enrichment"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Constants for scan configuration validation.
const (
	minTimeoutSeconds           = 60
	minTimeoutSecondsScripted   = 300 // aggressive/comprehensive run NSE scripts
	mediumTimeoutSeconds        = 300
	maxConcurrency              = 20
	ipv6CIDRBits                = 128
	defaultTargetCapacity       = 10
	nullMethodValue             = "NULL"
	scanTypeConnect             = "connect"
	scanTypeSYN                 = "syn"
	scanTypeACK                 = "ack"
	scanTypeUDP                 = "udp"
	scanTypeAggressive          = "aggressive"
	scanTypeComprehensive       = "comprehensive"
	udpTimeoutMultiplier        = 4
	scriptedOverheadNumerator   = 3
	scriptedOverheadDenominator = 2
)

const (
	// Database null value representation.
	nullValue = "NULL"

	// Output formatting constants.
	outputSeparatorLength = 80

	// Port state constants — must match the port_scans_state_check DB constraint.
	portStateOpen     = "open"
	portStateClosed   = "closed"
	portStateFiltered = "filtered"
	portStateUnknown  = "unknown"
)

// CalculateTimeout estimates a reasonable scan timeout based on the number of
// ports, targets, and scan type.
//
// Baseline: 1 second per port per target for TCP connect.
// UDP is ~4× slower due to retransmits and lack of RST responses.
// A floor of minTimeoutSeconds and a ceiling of 1 hour are applied.
func CalculateTimeout(ports string, targetCount int, scanType string) int {
	portCount := countPorts(ports)
	if portCount <= 0 {
		portCount = 1000 // nmap default
	}
	if targetCount <= 0 {
		targetCount = 1
	}

	// Base: ~1s per port per target for TCP connect at T3 timing.
	seconds := portCount * targetCount

	// UDP is significantly slower — retransmit delays add up.
	hasUDP := strings.Contains(ports, "U:")
	if scanType == scanTypeUDP || hasUDP {
		seconds *= udpTimeoutMultiplier
	}

	// Aggressive/comprehensive scan types do service detection and scripting —
	// add 50% overhead on the base count, and use a higher floor because NSE
	// scripts routinely take longer than 60s even on a small number of ports.
	isScripted := scanType == scanTypeAggressive || scanType == scanTypeComprehensive
	if isScripted {
		seconds = seconds * scriptedOverheadNumerator / scriptedOverheadDenominator
	}

	floor := minTimeoutSeconds
	if isScripted {
		floor = minTimeoutSecondsScripted
	}
	if seconds < floor {
		seconds = floor
	}
	const maxTimeoutSeconds = 3600
	if seconds > maxTimeoutSeconds {
		seconds = maxTimeoutSeconds
	}
	return seconds
}

// countPorts counts the total number of ports in a port specification,
// handling comma-separated values, ranges (e.g. "1-1024"), and T:/U: prefixes.
func countPorts(ports string) int {
	total := 0
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		// Strip protocol prefix.
		for _, prefix := range []string{"T:", "U:", "t:", "u:"} {
			if strings.HasPrefix(part, prefix) {
				part = part[len(prefix):]
				break
			}
		}
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 == nil && err2 == nil && hi >= lo {
				total += hi - lo + 1
			} else {
				total++ // malformed range — count as 1
			}
		} else {
			total++
		}
	}
	return total
}

// RunScan is a convenience wrapper around RunScanWithContext that uses a background context.

// stripProtocolPrefixes removes nmap mixed-protocol prefixes (T: and U:) from a
// port specification, returning a plain comma-separated port list suitable for a
// TCP-only connect scan.  Example: "T:22,80,U:53,161" → "22,80,53,161".
func stripProtocolPrefixes(ports string) string {
	var out []string
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		for _, prefix := range []string{"T:", "U:", "t:", "u:"} {
			if strings.HasPrefix(part, prefix) {
				part = part[len(prefix):]
				break
			}
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}

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
		"ports", config.Ports,
		"timeout_sec", config.TimeoutSec)

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

		// Detect privilege errors: several nmap features require raw sockets
		// (root / CAP_NET_RAW). Surface a clear, actionable message rather than
		// the generic "scanner execution failed".
		errStr := err.Error()
		if strings.Contains(errStr, "Operation not permitted") ||
			strings.Contains(errStr, "requires root privileges") ||
			strings.Contains(errStr, "requires elevated privileges") ||
			strings.Contains(errStr, "raw sockets") ||
			strings.Contains(errStr, "You requested a scan type which requires") {
			metrics.GetGlobalMetrics().IncrementScanErrors(config.ScanType, "permission_denied")
			logging.Error("Scanner requires elevated privileges",
				"scan_type", config.ScanType, "os_detection", config.OSDetection, "error", err)
			return nil, errors.WrapScanError(errors.CodeScanFailed, privilegeErrorMessage(config), err)
		}

		metrics.GetGlobalMetrics().IncrementScanErrors(config.ScanType, "execution_failed")
		logging.Error("Scanner execution failed", "scan_type", config.ScanType, "error", err)
		return nil, errors.WrapScanError(errors.CodeScanFailed, "scanner execution failed", err)
	}

	// Convert results to our format
	convertNmapResults(result, scanResult)

	// Store results in database if provided
	if database != nil {
		if err := storeScanResults(ctx, database, config, scanResult, config.ScanID); err != nil {
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
		return nil, &ExecError{Op: "create scanner", Err: err}
	}

	// Run the scan
	result, warnings, err := scanner.Run()
	if err != nil {
		return nil, &ExecError{Op: "run scan", Err: err}
	}

	if warnings != nil && len(*warnings) > 0 {
		logging.Warn("Scan completed with warnings", "warnings", *warnings)
	}

	return result, nil
}

// buildScanOptions creates nmap options based on scan configuration.
func buildScanOptions(config *ScanConfig) []nmap.Option {
	// Mixed-protocol support: if the port spec contains UDP ports (e.g. "T:22,80,U:53,161"),
	// add an explicit UDP scan so nmap handles both TCP and UDP in a single run.
	// Skip when using connect scan (-sT): UDP scanning always requires root (-sU),
	// and combining -sU with -sT produces no results when running unprivileged.
	// Strip the T:/U: protocol prefixes from the port spec in that case so nmap
	// doesn't complain about the mixed-protocol syntax without a UDP scan mode.
	hasUDP := strings.Contains(config.Ports, "U:")
	if hasUDP && config.ScanType == scanTypeConnect {
		// Resolve before building the options slice to avoid index mutation.
		config.Ports = stripProtocolPrefixes(config.Ports)
		hasUDP = false
	}

	options := []nmap.Option{
		nmap.WithTargets(config.Targets...),
		nmap.WithPorts(config.Ports),
	}

	if hasUDP {
		options = append(options, nmap.WithUDPScan())
	}

	// Add scan type options with enhanced capabilities
	switch config.ScanType {
	case scanTypeConnect:
		options = append(options, nmap.WithConnectScan())
	case scanTypeSYN:
		options = append(options, nmap.WithSYNScan())
	case scanTypeACK:
		options = append(options, nmap.WithACKScan())
	case scanTypeUDP:
		options = append(options, nmap.WithUDPScan())
	case scanTypeAggressive:
		options = append(options,
			nmap.WithSYNScan(),
			nmap.WithServiceInfo(),
			nmap.WithVersionAll(),
			nmap.WithAggressiveScan(),
		)
	case scanTypeComprehensive:
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

	// Tell nmap to assume it is fully privileged when the scan requires raw
	// sockets (SYN/ACK/UDP/OS-detection). This is necessary when the nmap
	// binary runs via SUID root or Linux capabilities (CAP_NET_RAW): without
	// --privileged nmap checks its own EUID and may abort before even trying,
	// even though it actually has the OS-level access it needs.
	// Connect-only scans without OS detection never need raw sockets, so we
	// leave --privileged off to avoid unnecessary permission escalation.
	if scanNeedsPrivileged(config, hasUDP) {
		options = append(options, nmap.WithPrivileged())
	}

	// Add nmap timing template. The Timing field (set from the scan profile) takes
	// precedence. If not set, fall back to a concurrency-based heuristic.
	if opt := buildTimingOption(config.Timing); opt != nil {
		options = append(options, opt)
	} else if config.Concurrency > maxConcurrency {
		// No explicit timing — apply T4 when high concurrency is requested,
		// otherwise leave nmap to use its own default (T3).
		options = append(options, nmap.WithTimingTemplate(nmap.TimingAggressive))
	}

	// Pass --host-timeout to nmap so it enforces its own deadline even if the
	// Go context is cancelled or the parent process dies. Use the same value
	// as the scan's TimeoutSec so the two are in sync.
	if config.TimeoutSec > 0 {
		options = append(options, nmap.WithHostTimeout(time.Duration(config.TimeoutSec)*time.Second))
	}

	// Add host discovery options for better reliability and useful scanning options
	options = append(options,
		nmap.WithSkipHostDiscovery(), // Skip ping and go straight to port scan
		nmap.WithVerbosity(1),        // Basic verbosity for better debugging
	)

	return options
}

// buildTimingOption maps a timing string from the scan profile to the
// corresponding nmap timing-template option. It returns nil when the timing
// value is empty or unrecognized so the caller can apply its own fallback.
func buildTimingOption(timing string) nmap.Option {
	switch timing {
	case "paranoid":
		return nmap.WithTimingTemplate(nmap.TimingSlowest)
	case "polite":
		return nmap.WithTimingTemplate(nmap.TimingSneaky)
	case "normal":
		return nmap.WithTimingTemplate(nmap.TimingNormal)
	case scanTypeAggressive:
		return nmap.WithTimingTemplate(nmap.TimingAggressive)
	case "insane":
		return nmap.WithTimingTemplate(nmap.TimingFastest)
	default:
		return nil
	}
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
			State:       normalizePortState(p.State.State),
			Service:     p.Service.Name,
			Version:     p.Service.Version,
			ServiceInfo: p.Service.Product,
		}
		host.Ports = append(host.Ports, port)
	}

	// Capture OS detection data from the best match (highest accuracy).
	if len(h.OS.Matches) > 0 {
		best := h.OS.Matches[0]
		host.OSName = best.Name
		host.OSAccuracy = best.Accuracy
		if len(best.Classes) > 0 {
			host.OSFamily = best.Classes[0].Family
			host.OSVersion = best.Classes[0].OSGeneration
		}
	}

	// Derive per-host scan duration from the nmap XML start/end timestamps.
	// nmap.Timestamp is an alias for time.Time, so a zero value means the
	// attribute was absent in the XML (host was down or timing not reported).
	start := time.Time(h.StartTime)
	end := time.Time(h.EndTime)
	if !start.IsZero() && !end.IsZero() && end.After(start) {
		ms := int(end.Sub(start).Milliseconds())
		host.ScanDurationMs = &ms
	}

	return host
}

// normalizePortState maps nmap compound states to the four values allowed by
// the port_scans_state_check DB constraint: open, closed, filtered, unknown.
//
// nmap can return "open|filtered" for UDP/firewall scenarios where it cannot
// distinguish between the two — we treat that as "open" (conservative: assume
// the port may be reachable). "closed|filtered" is treated as "filtered".
// Any other unrecognized value falls back to "unknown".
func normalizePortState(state string) string {
	switch state {
	case portStateOpen, portStateClosed, portStateFiltered, portStateUnknown:
		return state
	case "open|filtered":
		return portStateOpen
	case "closed|filtered":
		return portStateFiltered
	default:
		return portStateUnknown
	}
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

// scanNeedsPrivileged reports whether the scan configuration requires raw
// socket access (CAP_NET_RAW / SUID root). Used to decide whether to pass
// --privileged to nmap so it does not abort its own EUID pre-check when
// running via SUID or Linux capabilities.
func scanNeedsPrivileged(config *ScanConfig, hasUDP bool) bool {
	return config.OSDetection ||
		hasUDP ||
		config.ScanType == scanTypeSYN ||
		config.ScanType == scanTypeACK ||
		config.ScanType == scanTypeUDP ||
		config.ScanType == scanTypeAggressive ||
		config.ScanType == scanTypeComprehensive
}

// privilegeErrorMessage returns an actionable error string explaining which
// nmap feature triggered the raw-socket requirement and how to resolve it.
// Three distinct cases in priority order:
//  1. OS detection (-O) — can be disabled in the profile.
//  2. UDP ports in the port list — UDP always needs CAP_NET_RAW.
//  3. A privileged TCP scan type (syn, ack, …) — suggest 'connect'.
func privilegeErrorMessage(config *ScanConfig) string {
	switch {
	case config.OSDetection:
		return "OS detection (-O) requires root privileges (CAP_NET_RAW); " +
			"disable OS detection in the scan profile or run the daemon as root"
	case strings.Contains(config.Ports, "U:"):
		return "UDP port scanning requires root privileges (CAP_NET_RAW); " +
			"remove UDP ports from the port list or run the daemon as root"
	default:
		return "scan type '" + config.ScanType + "' requires root privileges (CAP_NET_RAW); " +
			"switch to scan type 'connect' or run the daemon as root"
	}
}

// storeScanResults stores scan results in the database.
// storeScanResults persists the results of a scan run.
// When scanID is non-nil it is reused as the scan_jobs row ID so that
// GetScanResults (which queries port_scans by job_id) can find the data.
// When scanID is nil a fresh UUID is generated (legacy / CLI path).
func storeScanResults(
	ctx context.Context, database *db.DB, config *ScanConfig, result *ScanResult, scanID *uuid.UUID,
) error {
	logging.Debug("Storing scan results", "targets", config.Targets)

	statsJSON := fmt.Sprintf(`{"hosts_up": %d, "hosts_down": %d, "total_hosts": %d, "duration_seconds": %d}`,
		result.Stats.Up, result.Stats.Down, result.Stats.Total, int(result.Duration.Seconds()))

	jobRepo := db.NewScanJobRepository(database)

	var jobID uuid.UUID

	if scanID != nil {
		// The scan_job row was already created by CreateScan/StartScan.
		// Updating stats on the existing row avoids a PK conflict on insert.
		jobID = *scanID
		logging.Debug("Updating existing scan job stats", "job_id", jobID)
		if _, err := database.ExecContext(ctx,
			`UPDATE scan_jobs SET scan_stats = $1 WHERE id = $2`,
			db.JSONB(statsJSON), jobID,
		); err != nil {
			// Non-fatal: log and continue so port results are still stored.
			logging.Warn("Failed to update scan job stats", "job_id", jobID, "error", err)
		}
	} else {
		// No caller-supplied ID — standalone (non-API) scan run.
		// Find or create a network entry and create a brand-new scan_job row.
		networkID, err := findOrCreateNetwork(ctx, database, config)
		if err != nil {
			logging.Error("Failed to find or create network", "error", err)
			return fmt.Errorf("failed to find or create network: %w", err)
		}
		logging.Debug("Found/created network", "network_id", networkID)

		jobID = uuid.New()
		now := time.Now()
		scanJob := &db.ScanJob{
			ID:        jobID,
			NetworkID: &networkID,
			Status:    db.ScanJobStatusCompleted,
		}
		scanJob.StartedAt = &result.StartTime
		scanJob.CompletedAt = &now
		scanJob.ScanStats = db.JSONB(statsJSON)

		logging.Debug("Creating scan job", "job_id", scanJob.ID, "network_id", scanJob.NetworkID)
		if err := jobRepo.Create(ctx, scanJob); err != nil {
			logging.Error("Failed to create scan job", "error", err)
			return fmt.Errorf("failed to create scan job: %w", err)
		}
		logging.Debug("Successfully created scan job", "job_id", scanJob.ID)
	}

	// Store host and port scan results.
	logging.Debug("Storing host results", "host_count", len(result.Hosts))
	storedHosts, err := storeHostResults(ctx, database, jobID, result.Hosts)
	if err != nil {
		logging.Error("Failed to store host results", "error", err)
		return err
	}
	logging.Debug("Successfully stored all scan results")

	// Run DNS enrichment in the background — failures are non-fatal.
	if len(storedHosts) > 0 {
		go runDNSEnrichment(database, storedHosts)
	}

	// Launch banner enrichment in the background — best-effort, non-blocking.
	go runBannerEnrichment(database, result.Hosts)
	go runSNMPEnrichment(database, result.Hosts, config.SNMPCommunity)

	return nil
}

// findOrCreateNetwork finds an existing network by CIDR or creates a new one
// for ad-hoc (non-API) scan runs.  Returns the network UUID to store as
// scan_jobs.network_id.
func findOrCreateNetwork(ctx context.Context, database *db.DB, config *ScanConfig) (uuid.UUID, error) {
	if len(config.Targets) == 0 {
		return uuid.Nil, fmt.Errorf("no targets specified")
	}

	firstTarget := config.Targets[0]

	// Normalise to a CIDR string.
	cidr := firstTarget
	ip := net.ParseIP(firstTarget)
	if ip != nil {
		if ip.To4() != nil {
			cidr = ip.String() + "/32"
		} else {
			cidr = ip.String() + "/128"
		}
	}

	// Reuse the network if this CIDR is already known.
	var id uuid.UUID
	err := database.QueryRowContext(ctx,
		`SELECT id FROM networks WHERE cidr = $1`, cidr).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !stderrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("failed to look up network by CIDR: %w", err)
	}

	// Not found — determine scan type and create an ephemeral network entry
	// (is_active=false, scan_enabled=false so it won't appear in scheduled scans).
	dbScanType := config.ScanType
	switch config.ScanType {
	case "comprehensive", scanTypeAggressive:
		dbScanType = scanTypeConnect
	case "stealth":
		dbScanType = scanTypeConnect
	}

	id = uuid.New()
	name := fmt.Sprintf("Ad-hoc: %s", cidr)
	_, err = database.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr,
			scan_ports, scan_type,
			is_active, scan_enabled, discovery_method
		) VALUES (
			$1, $2, $3, $4, $5, false, false, 'tcp'
		)
	`, id, name, cidr, config.Ports, dbScanType)
	if err != nil {
		// Name collision — fall back to the CIDR itself as the name.
		_, err = database.ExecContext(ctx, `
			INSERT INTO networks (
				id, name, cidr,
				scan_ports, scan_type,
				is_active, scan_enabled, discovery_method
			) VALUES (
				$1, $2, $3, $4, $5, false, false, 'tcp'
			)
		`, id, cidr, cidr, config.Ports, dbScanType)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to create network for ad-hoc scan: %w", err)
		}
	}

	logging.Debug("Created ad-hoc network", "network_id", id, "cidr", cidr)
	return id, nil
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
		// Accept hostnames: either containing a dot (FQDNs like "example.com")
		// or well-known single-label names like "localhost".
		// Reject bare words that look neither like an IP nor a plausible hostname.
		isHostname := strings.Contains(target, ".") || target == "localhost"
		if !isHostname {
			return networkAddr, fmt.Errorf("invalid target address: %q is not a valid IP, CIDR, or hostname", target)
		}
		// For hostnames, create a placeholder network; nmap will resolve it.
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

// processHostForScan processes a single host for scanning, preserving discovery data.
// Uses transaction-safe approach to handle race conditions between discovery and scan.
func processHostForScan(ctx context.Context, database *db.DB, hostRepo *db.HostRepository,
	host *Host, jobID uuid.UUID) (*db.Host, []*db.PortScan, error) {
	ipAddr := db.IPAddr{IP: net.ParseIP(host.Address)}

	dbHost, err := getOrCreateHostSafely(ctx, database, hostRepo, ipAddr, host)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get or create host %s: %w", host.Address, err)
	}

	// Persist OS detection data when nmap returned results.
	if host.OSName != "" || host.OSFamily != "" {
		persistOSData(ctx, hostRepo, dbHost, host)
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
			ID:             uuid.New(),
			JobID:          jobID,
			HostID:         dbHost.ID,
			Port:           int(port.Number),
			Protocol:       port.Protocol,
			State:          port.State,
			ScanDurationMs: host.ScanDurationMs,
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

	return dbHost, portScans, nil
}

// persistOSData writes OS detection fields from a scan Host onto the db.Host record.
func persistOSData(ctx context.Context, hostRepo *db.HostRepository, dbHost *db.Host, host *Host) {
	if host.OSFamily != "" {
		dbHost.OSFamily = &host.OSFamily
	}
	if host.OSName != "" {
		dbHost.OSName = &host.OSName
	}
	if host.OSVersion != "" {
		dbHost.OSVersion = &host.OSVersion
	}
	if host.OSAccuracy > 0 {
		dbHost.OSConfidence = &host.OSAccuracy
	}
	if updateErr := hostRepo.CreateOrUpdate(ctx, dbHost); updateErr != nil {
		logging.Warn("Failed to update OS detection data for host",
			"address", host.Address, "error", updateErr)
	}
}

// getOrCreateHostSafely atomically upserts a host by IP address, eliminating
// the TOCTOU race of the previous get-then-create pattern.
func getOrCreateHostSafely(ctx context.Context, _ *db.DB, hostRepo *db.HostRepository,
	ipAddr db.IPAddr, host *Host) (*db.Host, error) {
	dbHost, err := hostRepo.UpsertForScan(ctx, ipAddr, host.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert host %s: %w", ipAddr, err)
	}
	logging.Debug("Host upserted for scan",
		"ip", ipAddr.String(),
		"id", dbHost.ID.String())
	return dbHost, nil
}

// storeHostResults stores host and port scan results in the database.
// storeHostResults stores host and port scan results in the database,
// updating OS fields on the host record when OS detection data is present.
// It returns the slice of db.Host records that were successfully stored,
// so callers can run post-scan enrichment on them.
func storeHostResults(ctx context.Context, database *db.DB, jobID uuid.UUID, hosts []Host) ([]*db.Host, error) {
	hostRepo := db.NewHostRepository(database)
	portRepo := db.NewPortScanRepository(database)

	var allPortScans []*db.PortScan
	var storedHosts []*db.Host

	for i := range hosts {
		dbHost, portScans, err := processHostForScan(ctx, database, hostRepo, &hosts[i], jobID)
		if err != nil {
			logging.Error("Failed to process host", "address", hosts[i].Address, "error", err)
			continue
		}
		allPortScans = append(allPortScans, portScans...)
		if dbHost != nil {
			storedHosts = append(storedHosts, dbHost)
		}
	}

	// Batch insert all port scans
	if len(allPortScans) > 0 {
		if err := portRepo.CreateBatch(ctx, allPortScans); err != nil {
			return nil, fmt.Errorf("failed to store port scan results: %w", err)
		}
	}

	return storedHosts, nil
}

// runDNSEnrichment creates a short-lived DNS enricher and enriches the given
// hosts. It is intended to be called in a goroutine after scan results are
// stored. Any error is only logged, never returned.
func runDNSEnrichment(database *db.DB, hosts []*db.Host) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("panic in DNS enrichment goroutine", "error", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resolver := internaldns.New(database)
	dnsRepo := db.NewDNSRepository(database)
	hostRepo := db.NewHostRepository(database)
	enricher := enrichment.NewDNSEnricher(resolver, dnsRepo, hostRepo)
	enricher.EnrichHosts(ctx, hosts)
}

// bannerEnrichmentTimeout is the maximum wall-clock time allowed for a full
// banner-grabbing pass after a scan completes.
const bannerEnrichmentTimeout = 5 * time.Minute

// runBannerEnrichment looks up host IDs for all scanned hosts with open ports
// and runs banner grabbing asynchronously. Errors are logged, not propagated.
func runBannerEnrichment(database *db.DB, hosts []Host) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("panic in banner enrichment goroutine", "error", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), bannerEnrichmentTimeout)
	defer cancel()

	hostRepo := db.NewHostRepository(database)
	bannerRepo := db.NewBannerRepository(database)
	grabber := enrichment.NewBannerGrabber(bannerRepo, slog.Default())

	var targets []enrichment.BannerTarget
	for _, h := range hosts {
		if h.Status != "up" {
			continue
		}

		var openPorts []int
		for _, p := range h.Ports {
			if p.State == portStateOpen && p.Protocol == db.ProtocolTCP {
				openPorts = append(openPorts, int(p.Number))
			}
		}
		if len(openPorts) == 0 {
			continue
		}

		ip := net.ParseIP(h.Address)
		if ip == nil {
			continue
		}
		dbHost, err := hostRepo.GetByIP(ctx, db.IPAddr{IP: ip})
		if err != nil {
			logging.Warn("banner enrichment: host not found by IP", "ip", h.Address, "error", err)
			continue
		}

		targets = append(targets, enrichment.BannerTarget{
			HostID: dbHost.ID,
			IP:     h.Address,
			Ports:  openPorts,
		})
	}

	if len(targets) > 0 {
		grabber.EnrichHosts(ctx, targets)
	}
}

const (
	snmpEnrichmentTimeout = 5 * time.Minute
	snmpPort              = 161
)

// runSNMPEnrichment probes hosts that had port 161 open during the scan.
// Runs in a goroutine; errors are logged but not propagated.
func runSNMPEnrichment(database *db.DB, hosts []Host, community string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("panic in SNMP enrichment goroutine", "error", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), snmpEnrichmentTimeout)
	defer cancel()

	hostRepo := db.NewHostRepository(database)
	snmpRepo := db.NewSNMPRepository(database)
	enricher := enrichment.NewSNMPEnricher(snmpRepo, slog.Default())

	for i := range hosts {
		h := &hosts[i]
		if !hasSNMPPort(h.Ports) {
			continue
		}
		dbHost, err := hostRepo.GetByIP(ctx, db.IPAddr{IP: net.ParseIP(h.Address)})
		if err != nil {
			slog.Default().Warn("snmp enrichment: host lookup failed", "ip", h.Address, "err", err)
			continue
		}
		target := enrichment.SNMPTarget{
			HostID:    dbHost.ID,
			IP:        h.Address,
			Community: community,
		}
		if err := enricher.EnrichHost(ctx, target); err != nil {
			slog.Default().Warn("snmp enrichment failed", "ip", h.Address, "err", err)
		}
	}
}

// hasSNMPPort reports whether port 161 UDP is open in a port list.
func hasSNMPPort(ports []Port) bool {
	for _, p := range ports {
		if p.Number == snmpPort && p.Protocol == "udp" && p.State == portStateOpen {
			return true
		}
	}
	return false
}
