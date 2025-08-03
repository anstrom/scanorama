package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anstrom/scanorama/internal"
)

// Version information - populated by build flags
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// ScanOptions holds command-line options for the scanner.
type ScanOptions struct {
	Targets     string // Comma-separated list of targets (IPs, hostnames, CIDR ranges)
	Ports       string // Port specification (e.g., "80,443" or "1-1000")
	ScanType    string // Type of scan to perform (syn, connect, version)
	Output      string // Output file path for results (optional)
	timeout     int    // Scan timeout in seconds
	showVersion bool   // Display version information
}

func main() {
	opts := ParseFlags()

	if opts.showVersion {
		fmt.Printf("Scanorama %s\nCommit: %s\nBuild Time: %s\n", version, commit, buildTime)
		os.Exit(0)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal. Cleaning up...")
		cancel()
	}()

	// Validate inputs
	if opts.Targets == "" {
		log.Fatal("Target hosts are required. Use -targets flag.")
	}

	// Create scanner config with timeout
	config := internal.ScanConfig{
		Targets:    strings.Split(opts.Targets, ","),
		Ports:      opts.Ports,
		ScanType:   opts.ScanType,
		TimeoutSec: opts.timeout,
	}

	// Run the scan with context
	results, err := internal.RunScanWithContext(ctx, config)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Fatal("Scan was interrupted")
		}
		log.Fatalf("Scan failed: %v", err)
	}

	// Handle output
	if opts.Output != "" {
		if err := internal.SaveResults(results, opts.Output); err != nil {
			log.Fatalf("Failed to save results: %v", err)
		}
		log.Printf("Results saved to: %s", opts.Output)
	} else {
		// Print to stdout
		internal.PrintResults(results)
	}
}

// ParseFlags processes command-line flags and returns the parsed options.
// It also sets up the usage message with examples and flag descriptions.
func ParseFlags() ScanOptions {
	opts := ScanOptions{}

	flag.StringVar(&opts.Targets, "targets", "", "Comma-separated list of targets (IPs, hostnames, CIDR ranges)")
	flag.StringVar(&opts.Ports, "ports", "1-1000", "Port specification (e.g., '80,443' or '1-1000')")
	flag.StringVar(&opts.ScanType, "type", "syn", "Scan type: syn, connect, version")
	flag.StringVar(&opts.Output, "output", "", "Output file path (XML format)")
	flag.IntVar(&opts.timeout, "timeout", 300, "Scan timeout in seconds")
	flag.BoolVar(&opts.showVersion, "version", false, "Show version information")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nScanorama - Network Scanner\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  %s -targets 192.168.1.0/24 -ports 80,443\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -targets localhost -type version\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()
	return opts
}
