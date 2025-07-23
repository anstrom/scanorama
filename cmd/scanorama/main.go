package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/anstrom/scanorama/internal"
)

type scanOptions struct {
	targets  string
	ports    string
	scanType string
	output   string
}

func main() {
	opts := parseFlags()

	// Validate inputs
	if opts.targets == "" {
		log.Fatal("Target hosts are required. Use -targets flag.")
	}

	// Create scanner config
	config := internal.ScanConfig{
		Targets:  strings.Split(opts.targets, ","),
		Ports:    opts.ports,
		ScanType: opts.scanType,
	}

	// Run the scan
	results, err := internal.RunScan(config)
	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	// Handle output
	if opts.output != "" {
		if err := internal.SaveResults(results, opts.output); err != nil {
			log.Fatalf("Failed to save results: %v", err)
		}
	} else {
		// Print to stdout
		internal.PrintResults(results)
	}
}

func parseFlags() scanOptions {
	opts := scanOptions{}

	flag.StringVar(&opts.targets, "targets", "", "Comma-separated list of targets (IPs, hostnames, CIDR ranges)")
	flag.StringVar(&opts.ports, "ports", "1-1000", "Port specification (e.g., '80,443' or '1-1000')")
	flag.StringVar(&opts.scanType, "type", "syn", "Scan type: syn, connect, version")
	flag.StringVar(&opts.output, "output", "", "Output file path (XML format)")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nScanorama - Network Scanner\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  %s -targets 192.168.1.0/24 -ports 80,443\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -targets localhost -type version -aggressive\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	return opts
}
