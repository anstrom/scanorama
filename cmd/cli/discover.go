// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/spf13/cobra"
)

const (
	// Discovery operation constants.
	defaultDiscoveryTimeout = 30 // seconds for discovery timeout
)

var (
	discoverDetectOS    bool
	discoverAllNetworks bool
	discoverMethod      string
	discoverTimeout     int
)

// discoverCmd represents the discover command.
var discoverCmd = &cobra.Command{
	Use:   "discover [network]",
	Short: "Perform network discovery",
	Long: `Discover active hosts on the specified network using various methods
like ping sweeps, ARP discovery, or TCP probes.

The network argument should be in CIDR notation (e.g., 192.168.1.0/24).
If --all-networks is specified, the network argument is optional and
local networks will be auto-discovered.`,
	Example: `  scanorama discover 192.168.1.0/24
  scanorama discover 10.0.0.0/8 --detect-os
  scanorama discover --all-networks --method arp
  scanorama discover 172.16.0.0/12 --method tcp --timeout 60`,
	Args: func(cmd *cobra.Command, args []string) error {
		// If --all-networks is specified, network argument is optional
		if discoverAllNetworks {
			return cobra.MaximumNArgs(1)(cmd, args)
		}
		// Otherwise, exactly one network argument is required
		return cobra.ExactArgs(1)(cmd, args)
	},
	Run: runDiscovery,
}

func init() {
	rootCmd.AddCommand(discoverCmd)

	// Define flags
	discoverCmd.Flags().BoolVar(&discoverDetectOS, "detect-os", false, "Enable OS detection during discovery")
	discoverCmd.Flags().BoolVar(&discoverAllNetworks, "all-networks", false, "Discover and scan all local networks")
	discoverCmd.Flags().StringVar(&discoverMethod, "method", "tcp", "Discovery method: tcp, ping, or arp")
	discoverCmd.Flags().IntVar(&discoverTimeout, "timeout", defaultDiscoveryTimeout, "Discovery timeout in seconds")

	// Add flag descriptions
	discoverCmd.Flags().Lookup("detect-os").Usage = "Enable OS fingerprinting during host discovery"
	discoverCmd.Flags().Lookup("all-networks").Usage = "Auto-discover all local network interfaces and scan them"
	discoverCmd.Flags().Lookup("method").Usage = "Discovery method: tcp (TCP connect), ping (ICMP), or arp (ARP scan)"
	discoverCmd.Flags().Lookup("timeout").Usage = "Maximum time to wait for discovery completion"
}

func runDiscovery(cmd *cobra.Command, args []string) {
	// Determine target network
	var network string
	if len(args) > 0 {
		network = args[0]
	} else if discoverAllNetworks {
		fmt.Println("Auto-discovering local networks...")
		// This would be handled by the discovery engine
		network = ""
	} else {
		fmt.Fprintf(os.Stderr, "Error: network argument required when --all-networks is not specified\n")
		if helpErr := cmd.Help(); helpErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to display help: %v\n", helpErr)
		}
		os.Exit(1)
	}

	// Validate method
	validMethods := map[string]bool{
		"tcp":  true,
		"ping": true,
		"arp":  true,
	}
	if !validMethods[discoverMethod] {
		fmt.Fprintf(os.Stderr, "Error: invalid discovery method '%s'. Valid methods: tcp, ping, arp\n", discoverMethod)
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Create discovery configuration
		discoverConfig := discovery.Config{
			Network:     network,
			Method:      discoverMethod,
			DetectOS:    discoverDetectOS,
			Timeout:     time.Duration(discoverTimeout) * time.Second,
			Concurrency: 50,
		}

		if verbose {
			fmt.Printf("Starting discovery with config: %+v\n", discoverConfig)
		}

		// Run discovery
		fmt.Printf("Discovering hosts on %s using %s method...\n", network, discoverMethod)

		// TODO: Implement discovery using actual internal API
		fmt.Printf("Discovery functionality not yet fully implemented with new CLI\n")
		fmt.Printf("Network: %s, Method: %s, DetectOS: %v\n", network, discoverMethod, discoverDetectOS)

		fmt.Println("\nUse 'scanorama hosts' to view discovered hosts")
		fmt.Println("Use 'scanorama scan --live-hosts' to scan discovered hosts")
	})
}
