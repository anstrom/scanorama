package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
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

// scanCmd represents the scan command
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
  scanorama scan --live-hosts --type comprehensive
  scanorama scan --os-family windows --type intense`,
	Run: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)

	// Define flags
	scanCmd.Flags().StringVar(&scanTargets, "targets", "", "Comma-separated list of targets to scan")
	scanCmd.Flags().BoolVar(&scanLiveHosts, "live-hosts", false, "Scan only discovered live hosts")
	scanCmd.Flags().StringVar(&scanPorts, "ports", "22,80,443,8080,8443", "Ports to scan (comma-separated)")
	scanCmd.Flags().StringVar(&scanType, "type", "connect", "Scan type: connect, syn, version, comprehensive, intense, stealth")
	scanCmd.Flags().StringVar(&scanProfile, "profile", "", "Scan profile to use (overrides scan type)")
	scanCmd.Flags().IntVar(&scanTimeout, "timeout", 300, "Scan timeout in seconds")
	scanCmd.Flags().StringVar(&scanOSFamily, "os-family", "", "Scan only hosts with specific OS family (windows, linux, macos)")

	// Make targets and live-hosts mutually exclusive
	scanCmd.MarkFlagsMutuallyExclusive("targets", "live-hosts")

	// Add detailed flag descriptions
	scanCmd.Flags().Lookup("targets").Usage = "Specific targets to scan (e.g., '192.168.1.1,192.168.1.10' or '192.168.1.1-10')"
	scanCmd.Flags().Lookup("live-hosts").Usage = "Scan all hosts discovered as 'up' in previous discovery"
	scanCmd.Flags().Lookup("ports").Usage = "Port specification: '80,443' or '1-1000' or 'T:' for top ports"
	scanCmd.Flags().Lookup("type").Usage = "Scan type: connect (default), syn (requires root), version, comprehensive, intense, stealth"
	scanCmd.Flags().Lookup("profile").Usage = "Use predefined scan profile (windows-server, linux-server, etc.)"
	scanCmd.Flags().Lookup("timeout").Usage = "Maximum time to wait for scan completion"
	scanCmd.Flags().Lookup("os-family").Usage = "Filter targets by OS family when using --live-hosts"
}

func runScan(cmd *cobra.Command, args []string) {
	// Validate arguments
	if !scanLiveHosts && scanTargets == "" {
		fmt.Fprintf(os.Stderr, "Error: either --targets or --live-hosts must be specified\n\n")
		cmd.Help()
		os.Exit(1)
	}

	// Validate scan type
	validTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"version":       true,
		"comprehensive": true,
		"intense":       true,
		"stealth":       true,
	}
	if !validTypes[scanType] {
		fmt.Fprintf(os.Stderr, "Error: invalid scan type '%s'\n", scanType)
		fmt.Fprintf(os.Stderr, "Valid types: connect, syn, version, comprehensive, intense, stealth\n")
		os.Exit(1)
	}

	// Validate ports
	if err := validatePorts(scanPorts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid port specification '%s': %v\n", scanPorts, err)
		os.Exit(1)
	}

	// Setup database connection
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Connect(context.Background(), &cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create scan configuration
	scanConfig := internal.ScanConfig{
		Targets:    []string{},
		Ports:      scanPorts,
		ScanType:   scanType,
		TimeoutSec: scanTimeout,
	}

	if verbose {
		fmt.Printf("Scan configuration: %+v\n", scanConfig)
	}

	// Create scanner - using internal scan functionality
	// Note: This will need to be adapted to match the actual internal API

	// Run scan based on mode
	if scanLiveHosts {
		runLiveHostsScan(database, scanConfig)
	} else {
		runTargetsScan(database, scanConfig, scanTargets)
	}
}

func runLiveHostsScan(database *db.DB, config internal.ScanConfig) error {
	fmt.Println("Scanning discovered live hosts...")

	if scanOSFamily != "" {
		fmt.Printf("Filtering by OS family: %s\n", scanOSFamily)
	}

	// TODO: Implement live hosts scanning using internal package
	fmt.Println("Live hosts scanning not yet implemented with new CLI")
	return nil
}

func runTargetsScan(database *db.DB, config internal.ScanConfig, targets string) error {
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

	// Update scan config with targets
	config.Targets = targetList

	// TODO: Implement target scanning using internal package
	fmt.Printf("Target scanning not yet fully implemented with new CLI for targets: %v\n", targetList)
	return nil
}

func displayScanResults(result *internal.ScanResult) {
	fmt.Printf("\nScan completed successfully!\n")
	fmt.Printf("Duration: %v\n", result.Duration)
	fmt.Printf("Hosts up: %d\n", result.Stats.Up)
	fmt.Printf("Hosts down: %d\n", result.Stats.Down)
	fmt.Printf("Total hosts: %d\n", result.Stats.Total)
	fmt.Println("\nUse 'scanorama hosts' to view all discovered hosts")
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

		// Check for range (e.g., "80-443")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return fmt.Errorf("invalid port range: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil || start < 1 || start > 65535 {
				return fmt.Errorf("invalid start port in range: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil || end < 1 || end > 65535 {
				return fmt.Errorf("invalid end port in range: %s", rangeParts[1])
			}

			if start > end {
				return fmt.Errorf("start port cannot be greater than end port: %s", part)
			}
		} else {
			// Single port
			port, err := strconv.Atoi(part)
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("invalid port: %s", part)
			}
		}
	}

	return nil
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
