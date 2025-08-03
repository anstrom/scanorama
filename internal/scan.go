package internal

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ullaakut/nmap/v3"
)

// RunScan is a convenience wrapper around RunScanWithContext that uses a background context.
func RunScan(config ScanConfig) (*ScanResult, error) {
	return RunScanWithContext(context.Background(), config)
}

// RunScanWithContext performs a network scan based on the provided configuration and context.
// It uses nmap to scan the specified targets and ports, returning detailed results
// about discovered hosts and services.
func RunScanWithContext(ctx context.Context, config ScanConfig) (*ScanResult, error) {
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

	options := []nmap.Option{
		nmap.WithTargets(config.Targets...),
		nmap.WithPorts(config.Ports),
	}

	// Add scan type options
	switch config.ScanType {
	case "connect":
		options = append(options, nmap.WithConnectScan())
	case "syn":
		options = append(options, nmap.WithSYNScan())
	case "version":
		options = append(options, nmap.WithServiceInfo())
	}

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

	// Convert nmap results to our format
	scanResult.Stats = HostStats{
		Up:    result.Stats.Hosts.Up,
		Down:  result.Stats.Hosts.Down,
		Total: result.Stats.Hosts.Total,
	}
	scanResult.Hosts = make([]Host, 0, len(result.Hosts))

	for _, h := range result.Hosts {
		if len(h.Addresses) == 0 {
			continue
		}

		host := Host{
			Address: h.Addresses[0].Addr,
			Status:  h.Status.State,
			Ports:   make([]Port, 0, len(h.Ports)),
		}

		for _, p := range h.Ports {
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

		scanResult.Hosts = append(scanResult.Hosts, host)
	}

	return scanResult, nil
}

// validateScanConfig verifies that all scan parameters are valid.
// It checks target specification, port ranges, and scan type.
func validateScanConfig(config ScanConfig) error {
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
		fmt.Printf("%s\n", strings.Repeat("-", 80))

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
