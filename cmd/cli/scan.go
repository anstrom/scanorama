// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/spf13/cobra"
)

const (
	// Scan operation constants.
	defaultScanTimeout = 300 // default scan timeout in seconds
)

var (
	scanTargets   string
	scanLiveHosts bool
	scanPorts     string
	scanType      string
	scanProfile   string
	scanTimeout   int
	scanOSFamily  string
)

// scanCmd represents the scan command.
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan hosts for open ports and services",
	Long: `Scan discovered hosts or specific targets for open ports,
running services, and other network information.

You can either scan specific targets using --targets, or scan all
discovered live hosts using --live-hosts. Various scan types are
available depending on your needs and privileges.`,
	Example: `  scanorama scan --live-hosts
  scanorama scan --targets 192.168.1.10-20
  scanorama scan --targets "192.168.1.1,192.168.1.10" --ports "22,80,443"
  scanorama scan --targets localhost --type version
  scanorama scan --live-hosts --type aggressive
  scanorama scan --os-family windows --type aggressive`,
	Run: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)

	// Define flags
	scanCmd.Flags().StringVar(&scanTargets, "targets", "", "Comma-separated list of targets to scan")
	scanCmd.Flags().BoolVar(&scanLiveHosts, "live-hosts", false, "Scan only discovered live hosts")
	scanCmd.Flags().StringVar(&scanPorts, "ports", "22,80,443,8080,8443", "Ports to scan (comma-separated)")
	scanCmd.Flags().StringVar(&scanType, "type", "connect",
		"Scan type: connect, syn, version, aggressive, stealth")
	scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scan profile to use (overrides scan type)")
	scanCmd.Flags().IntVar(&scanTimeout, "timeout", defaultScanTimeout, "Scan timeout in seconds")
	scanCmd.Flags().StringVar(&scanOSFamily, "os-family", "",
		"Scan only hosts with specific OS family (windows, linux, macos)")

	// Make targets and live-hosts mutually exclusive
	scanCmd.MarkFlagsMutuallyExclusive("targets", "live-hosts")

	// Add detailed flag descriptions
	scanCmd.Flags().Lookup("targets").Usage = "Specific targets to scan " +
		"(e.g., '192.168.1.1,192.168.1.10' or '192.168.1.1-10')"
	scanCmd.Flags().Lookup("live-hosts").Usage = "Scan all hosts discovered as 'up' in previous discovery"
	scanCmd.Flags().Lookup("ports").Usage = "Port specification: '80,443' or '1-1000' or 'T:' for top ports"
	scanCmd.Flags().Lookup("type").Usage = "Scan type: connect (default), syn (requires root), " +
		"version, comprehensive, aggressive, stealth"
	scanCmd.Flags().Lookup("profile").Usage = "Use predefined scan profile (windows-server, linux-server, etc.)"
	scanCmd.Flags().Lookup("timeout").Usage = "Maximum time to wait for scan completion"
	scanCmd.Flags().Lookup("os-family").Usage = "Filter targets by OS family when using --live-hosts"
}

func runScan(cmd *cobra.Command, _ []string) {
	// Validate arguments
	if !scanLiveHosts && scanTargets == "" {
		logging.Error("Either --targets or --live-hosts must be specified")
		if helpErr := cmd.Help(); helpErr != nil {
			logging.Warn("Failed to display help", "error", helpErr)
		}
		os.Exit(1)
	}

	// Validate scan type
	validTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"version":       true,
		"comprehensive": true,
		"aggressive":    true,
		"stealth":       true,
	}
	if !validTypes[scanType] {
		logging.Error("Invalid scan type specified",
			"scan_type", scanType,
			"valid_types", "connect, syn, version, comprehensive, aggressive, stealth")
		os.Exit(1)
	}

	// Validate ports
	if err := validatePorts(scanPorts); err != nil {
		logging.Error("Invalid port specification", "ports", scanPorts, "error", err)
		os.Exit(1)
	}

	// Setup database connection
	cfg, err := config.Load("config.yaml")
	if err != nil {
		logging.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	database, err := db.Connect(context.Background(), &cfg.Database)
	if err != nil {
		logging.ErrorDatabase("Failed to connect to database", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			logging.Warn("Failed to close database connection", "error", closeErr)
		}
	}()

	// Create scan configuration
	scanConfig := scanning.ScanConfig{
		Targets:    []string{},
		Ports:      scanPorts,
		ScanType:   scanType,
		TimeoutSec: scanTimeout,
	}

	// Log scan configuration
	logging.Info("Starting scan operation",
		"scan_type", scanType,
		"ports", scanPorts,
		"timeout", scanTimeout,
		"live_hosts", scanLiveHosts)

	// Create scanner - using internal scan functionality
	// Note: This will need to be adapted to match the actual internal API

	// Run scan based on mode
	if scanLiveHosts {
		logging.Info("Scanning all live hosts")
		runLiveHostsScan(database, &scanConfig)
	} else {
		logging.InfoScan("Scanning specific targets", scanTargets)
		runTargetsScan(database, &scanConfig, scanTargets)
	}
}

func runLiveHostsScan(database *db.DB, scanConfig *scanning.ScanConfig) {
	fmt.Println("Scanning discovered live hosts...")

	// Query for live hosts
	var liveHosts []struct {
		IPAddress string `db:"ip_address"`
		OSFamily  string `db:"os_family"`
	}

	query := `SELECT ip_address, COALESCE(os_family, 'unknown') as os_family
	          FROM hosts
	          WHERE status = 'up' AND (ignore_scanning IS NULL OR ignore_scanning = false)`
	args := []interface{}{}

	// Add OS family filter if specified
	if scanOSFamily != "" {
		fmt.Printf("Filtering by OS family: %s\n", scanOSFamily)
		query += " AND LOWER(COALESCE(os_family, '')) = LOWER($1)"
		args = append(args, scanOSFamily)
	}

	query += " ORDER BY last_seen DESC"

	err := database.SelectContext(context.Background(), &liveHosts, query, args...)
	if err != nil {
		logging.ErrorDatabase("Failed to query live hosts", err)
		fmt.Printf("Error: Failed to query live hosts: %v\n", err)
		os.Exit(1)
	}

	if len(liveHosts) == 0 {
		if scanOSFamily != "" {
			fmt.Printf("No live hosts found with OS family '%s'\n", scanOSFamily)
		} else {
			fmt.Println("No live hosts found to scan")
		}
		return
	}

	fmt.Printf("Found %d live host(s) to scan\n", len(liveHosts))

	// Convert live hosts to target strings
	targets := make([]string, len(liveHosts))
	for i, host := range liveHosts {
		targets[i] = host.IPAddress
	}

	// Set targets in scan config
	scanConfig.Targets = targets

	// Run the scan using internal package
	result, err := scanning.RunScanWithDB(scanConfig, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan failed: %v\n", err)
		os.Exit(1)
	}

	// Calculate total ports
	totalPorts := 0
	for _, host := range result.Hosts {
		totalPorts += len(host.Ports)
	}

	// Display results
	fmt.Printf("\nScan completed successfully!\n")
	fmt.Printf("Hosts scanned: %d\n", len(result.Hosts))
	fmt.Printf("Total ports found: %d\n", totalPorts)

	// Show summary of open ports found
	openPorts := 0
	for _, host := range result.Hosts {
		for _, port := range host.Ports {
			if port.State == "open" {
				openPorts++
			}
		}
	}
	fmt.Printf("Open ports found: %d\n", openPorts)
}

func runTargetsScan(database *db.DB, scanConfig *scanning.ScanConfig, targets string) {
	fmt.Printf("Scanning targets: %s\n", targets)

	// Parse targets
	targetList := parseTargets(targets)
	if len(targetList) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no valid targets found in '%s'\n", targets)
		os.Exit(1)
	}

	if verbose {
		fmt.Printf("Parsed %d targets: %v\n", len(targetList), targetList)
	}

	// Set targets in scan config
	scanConfig.Targets = targetList

	// Run the scan using internal package
	fmt.Printf("Starting scan of %d target(s)...\n", len(targetList))
	result, err := scanning.RunScanWithDB(scanConfig, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan failed: %v\n", err)
		os.Exit(1)
	}

	// Calculate total ports
	totalPorts := 0
	for _, host := range result.Hosts {
		totalPorts += len(host.Ports)
	}

	// Display results
	fmt.Printf("\nScan completed successfully!\n")
	fmt.Printf("Targets scanned: %d\n", len(result.Hosts))
	fmt.Printf("Total ports found: %d\n", totalPorts)

	// Show summary of open ports found
	openPorts := 0
	for _, host := range result.Hosts {
		for _, port := range host.Ports {
			if port.State == "open" {
				openPorts++
			}
		}
	}
	fmt.Printf("Open ports found: %d\n", openPorts)
}

func validatePorts(ports string) error {
	if ports == "" {
		return fmt.Errorf("empty port specification")
	}

	// Handle special cases
	if strings.HasPrefix(ports, "T:") {
		return nil // Top ports specification
	}

	// Split by commas and validate each part
	parts := strings.Split(ports, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if err := validatePortPart(part); err != nil {
			return err
		}
	}

	return nil
}

func validatePortPart(part string) error {
	// Check for range (e.g., "80-443")
	if strings.Contains(part, "-") {
		return validatePortRange(part)
	}
	// Single port
	return validateSinglePort(part)
}

func validatePortRange(part string) error {
	rangeParts := strings.Split(part, "-")
	if len(rangeParts) != 2 {
		return fmt.Errorf("invalid port range: %s", part)
	}

	start, err := parsePort(rangeParts[0])
	if err != nil {
		return fmt.Errorf("invalid start port in range: %s", rangeParts[0])
	}

	end, err := parsePort(rangeParts[1])
	if err != nil {
		return fmt.Errorf("invalid end port in range: %s", rangeParts[1])
	}

	if start > end {
		return fmt.Errorf("start port cannot be greater than end port: %s", part)
	}

	return nil
}

func validateSinglePort(part string) error {
	_, err := parsePort(part)
	if err != nil {
		return fmt.Errorf("invalid port: %s", part)
	}
	return nil
}

func parsePort(portStr string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return port, nil
}

func parseTargets(targets string) []string {
	if targets == "" {
		return nil
	}

	var result []string
	parts := strings.Split(targets, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle range notation (e.g., "192.168.1.1-10")
		if strings.Contains(part, "-") && strings.Count(part, ".") >= 3 {
			// This is an IP range, expand it
			expanded := expandIPRange(part)
			result = append(result, expanded...)
		} else {
			result = append(result, part)
		}
	}

	return result
}

func expandIPRange(ipRange string) []string {
	// Simple implementation for IP ranges like "192.168.1.1-10"
	// For now, just return the original range - the scan engine should handle expansion
	return []string{ipRange}
}
