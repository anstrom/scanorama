// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/spf13/cobra"
)

const (
	// Discovery operation constants.
	defaultDiscoveryTimeout = 30 // seconds for discovery timeout
	defaultConcurrency      = 50 // default concurrency for discovery
	timeoutBufferSeconds    = 30 // extra timeout buffer for operations
	tableHeaderSeparatorLen = 70 // length of table header separator
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

// waitForDiscoveryCompletion waits for a discovery job to complete and shows progress.
func waitForDiscoveryCompletion(ctx context.Context, database *db.DB, jobID interface{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastStatus := ""

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("discovery canceled")
		default:
		}

		var status string
		var completedAt *time.Time
		var hostsDiscovered, hostsResponsive int

		query := `SELECT status, completed_at, hosts_discovered, hosts_responsive
		         FROM discovery_jobs WHERE id = $1`
		err := database.QueryRowContext(ctx, query, jobID).Scan(
			&status, &completedAt, &hostsDiscovered, &hostsResponsive)

		if err != nil {
			return fmt.Errorf("failed to check job status: %w", err)
		}

		// Show status updates
		if status != lastStatus {
			switch status {
			case db.DiscoveryJobStatusRunning:
				fmt.Printf("Discovery in progress...\n")
			case db.DiscoveryJobStatusCompleted:
				fmt.Printf("Discovery completed successfully!\n")
				fmt.Printf("Found %d responsive hosts out of %d discovered\n", hostsResponsive, hostsDiscovered)
				return nil
			case db.DiscoveryJobStatusFailed:
				return fmt.Errorf("discovery job failed")
			}
			lastStatus = status
		}

		if status == db.DiscoveryJobStatusCompleted {
			return nil
		}

		if status == db.DiscoveryJobStatusFailed {
			return fmt.Errorf("discovery job failed")
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("discovery job did not complete within %v", timeout)
}

// showDiscoveryResults displays the results of a completed discovery job.
func showDiscoveryResults(database *db.DB, jobID interface{}) {
	// Get job details
	var hostsDiscovered, hostsResponsive int
	var completedAt *time.Time
	var network string

	query := `SELECT hosts_discovered, hosts_responsive, completed_at, network
	         FROM discovery_jobs WHERE id = $1`
	err := database.QueryRow(query, jobID).Scan(&hostsDiscovered, &hostsResponsive, &completedAt, &network)

	if err != nil {
		fmt.Printf("Warning: Could not retrieve discovery results: %v\n", err)
		return
	}

	fmt.Println("\n=== Discovery Results ===")
	fmt.Printf("Network: %s\n", network)
	fmt.Printf("Total hosts discovered: %d\n", hostsDiscovered)
	fmt.Printf("Responsive hosts: %d\n", hostsResponsive)

	if completedAt != nil {
		fmt.Printf("Completed at: %s\n", completedAt.Format(time.RFC3339))
	}

	// Show recently discovered hosts
	if hostsResponsive > 0 {
		fmt.Println("\nRecently discovered hosts:")
		showRecentHosts(database, network)
	}
}

// showRecentHosts displays recently discovered hosts from the network.
func showRecentHosts(database *db.DB, networkStr string) {
	// Parse the network to get the CIDR
	_, _, err := net.ParseCIDR(networkStr)
	if err != nil {
		fmt.Printf("Warning: Could not parse network CIDR: %v\n", err)
		return
	}

	// Query for hosts in this network discovered recently (last hour)
	query := `
		SELECT ip_address, status, COALESCE(os_family, 'unknown') as os_family,
		       COALESCE(discovery_method, '') as discovery_method, last_seen
		FROM hosts
		WHERE ip_address <<= $1
		  AND last_seen > NOW() - INTERVAL '1 hour'
		ORDER BY last_seen DESC
		LIMIT 10`

	rows, err := database.Query(query, networkStr)
	if err != nil {
		fmt.Printf("Warning: Could not query discovered hosts: %v\n", err)
		return
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	fmt.Printf("%-15s %-8s %-12s %-8s %s\n", "IP Address", "Status", "OS Family", "Method", "Last Seen")
	fmt.Println(strings.Repeat("-", tableHeaderSeparatorLen))

	count := 0
	for rows.Next() {
		var ip, status, osFamily, method string
		var lastSeen time.Time

		err := rows.Scan(&ip, &status, &osFamily, &method, &lastSeen)
		if err != nil {
			continue
		}

		fmt.Printf("%-15s %-8s %-12s %-8s %s\n",
			ip, status, osFamily, method, lastSeen.Format("15:04:05"))
		count++
	}

	if count == 0 {
		fmt.Println("No hosts found in database for this network")
	}
}

// discoverLocalNetworks discovers local network interfaces for --all-networks option.
func discoverLocalNetworks() ([]string, error) {
	var networks []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					// IPv4 address
					networks = append(networks, ipnet.String())
				}
			}
		}
	}

	return networks, nil
}

func runDiscovery(cmd *cobra.Command, args []string) {
	// Determine target network(s)
	networks := determineTargetNetworks(cmd, args)

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
		// Create discovery engine
		engine := discovery.NewEngine(database)
		engine.SetConcurrency(defaultConcurrency)
		engine.SetTimeout(time.Duration(discoverTimeout) * time.Second)

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(),
			time.Duration(discoverTimeout+timeoutBufferSeconds)*time.Second)
		defer cancel()

		// Process each network
		for i, network := range networks {
			if len(networks) > 1 {
				fmt.Printf("\n=== Discovering Network %d/%d: %s ===\n", i+1, len(networks), network)
			}

			// Create discovery configuration
			discoverConfig := &discovery.Config{
				Network:     network,
				Method:      discoverMethod,
				DetectOS:    discoverDetectOS,
				Timeout:     time.Duration(discoverTimeout) * time.Second,
				Concurrency: defaultConcurrency,
				MaxHosts:    10000,
			}

			if verbose {
				fmt.Printf("Starting discovery with config: %+v\n", discoverConfig)
			}

			// Run discovery
			fmt.Printf("Discovering hosts on %s using %s method...\n", network, discoverMethod)

			job, err := engine.Discover(ctx, discoverConfig)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to start discovery for %s: %v\n", network, err)
				continue
			}

			fmt.Printf("Discovery job started (ID: %s)\n", job.ID)

			// Wait for completion with progress updates
			err = waitForDiscoveryCompletion(ctx, database, job.ID, time.Duration(discoverTimeout)*time.Second)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Discovery failed for %s: %v\n", network, err)
				continue
			}

			// Show final results
			showDiscoveryResults(database, job.ID)
		}

		fmt.Println("\nDiscovery complete!")
		fmt.Println("Use 'scanorama hosts' to view all discovered hosts")
		fmt.Println("Use 'scanorama scan --live-hosts' to scan discovered hosts")
	})
}

// determineTargetNetworks determines the target networks based on flags and arguments.
func determineTargetNetworks(cmd *cobra.Command, args []string) []string {
	if discoverAllNetworks {
		return handleAllNetworksDiscovery()
	}
	return handleSingleNetworkDiscovery(cmd, args)
}

// handleAllNetworksDiscovery handles the --all-networks option.
func handleAllNetworksDiscovery() []string {
	fmt.Println("Auto-discovering local networks...")
	localNets, err := discoverLocalNetworks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to discover local networks: %v\n", err)
		os.Exit(1)
	}

	if len(localNets) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No local networks found\n")
		os.Exit(1)
	}

	fmt.Printf("Discovered %d local networks: %s\n", len(localNets), strings.Join(localNets, ", "))
	return localNets
}

// handleSingleNetworkDiscovery handles single network discovery from arguments.
func handleSingleNetworkDiscovery(cmd *cobra.Command, args []string) []string {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: network argument required when --all-networks is not specified\n")
		if helpErr := cmd.Help(); helpErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to display help: %v\n", helpErr)
		}
		os.Exit(1)
	}
	return []string{args[0]}
}
