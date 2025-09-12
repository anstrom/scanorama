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
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const (
	// Discovery operation constants.
	defaultDiscoveryTimeout = 30 // seconds for discovery timeout
	defaultConcurrency      = 50 // default concurrency for discovery
	timeoutBufferSeconds    = 30 // extra timeout buffer for operations
	tableHeaderSeparatorLen = 70 // length of table header separator

	// Dynamic timeout calculation constants - realistic values for network discovery
	minTimeoutSeconds      = 10   // minimum timeout: 10 seconds
	maxTimeoutSeconds      = 1800 // maximum timeout: 30 minutes
	baseTimeoutPerHost     = 0.5  // base seconds per host
	batchTimeoutSeconds    = 15   // base timeout for any network
	timeoutConcurrency     = 50   // default concurrency for timeout calculation
	batchOverheadThreshold = 100  // threshold above which to add batch overhead
	defaultNetworkSize     = 254  // default network size estimate for /24 network
	maxBatchBufferSeconds  = 3600 // maximum batch buffer: 1 hour

	// SQL error constants.
	sqlNoRowsError = "sql: no rows in result set"
)

var (
	discoverDetectOS       bool
	discoverAllNetworks    bool
	discoverConfiguredNets bool
	discoverNetworkName    string
	discoverMethod         string
	discoverTimeout        int
	discoverAdd            bool
	discoverAddName        string
)

// discoverCmd represents the discover command.
var discoverCmd = &cobra.Command{
	Use:   "discover [network]",
	Short: "Perform network discovery",
	Long: `Discover active hosts using various methods like ping sweeps, ARP discovery, or TCP probes.

Discovery can be performed on:
- Specific CIDR networks (e.g., 192.168.1.0/24)
- Configured networks from the database (--configured-networks or --network)
- Auto-discovered local networks (--all-networks)

Network exclusions are automatically applied during discovery.`,
	Example: `  scanorama discover 192.168.1.0/24
  scanorama discover --configured-networks
  scanorama discover --network corp-lan
  scanorama discover 192.168.1.0/24 --add --name "corp-lan"
  scanorama discover --all-networks --method arp`,
	Args: func(cmd *cobra.Command, args []string) error {
		// Count exclusive flags
		flagCount := 0
		if discoverAllNetworks {
			flagCount++
		}
		if discoverConfiguredNets {
			flagCount++
		}
		if discoverNetworkName != "" {
			flagCount++
		}

		// Validate --add flag usage
		if discoverAdd {
			if flagCount > 0 {
				return fmt.Errorf("--add can only be used with specific CIDR networks, " +
					"not with --all-networks, --configured-networks, or --network")
			}
			// --add requires exactly one CIDR argument
			return cobra.ExactArgs(1)(cmd, args)
		}

		// If any network flag is specified, no positional args needed
		if flagCount > 0 {
			if flagCount > 1 {
				return fmt.Errorf("only one of --all-networks, --configured-networks, or --network can be specified")
			}
			return cobra.MaximumNArgs(0)(cmd, args)
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
	discoverCmd.Flags().BoolVar(&discoverAllNetworks, "all-networks", false, "Discover all local network interfaces")
	discoverCmd.Flags().BoolVar(&discoverConfiguredNets, "configured-networks", false,
		"Discover all configured networks")
	discoverCmd.Flags().StringVar(&discoverNetworkName, "network", "", "Discover specific configured network by name")
	discoverCmd.Flags().StringVar(&discoverMethod, "method", "ping", "Discovery method: tcp, ping, arp, or icmp")
	discoverCmd.Flags().IntVar(&discoverTimeout, "timeout", defaultDiscoveryTimeout, "Discovery timeout in seconds")
	discoverCmd.Flags().BoolVar(&discoverAdd, "add", false, "Add the discovered network to configured networks")
	discoverCmd.Flags().StringVar(&discoverAddName, "name", "",
		"Name for the network when using --add (defaults to CIDR if not specified)")

	// Add flag descriptions
	discoverCmd.Flags().Lookup("detect-os").Usage = "Enable OS fingerprinting during host discovery"
	discoverCmd.Flags().Lookup("all-networks").Usage = "Auto-discover all local network interfaces and scan them"
	discoverCmd.Flags().Lookup("configured-networks").Usage = "Discover all active configured networks from database"
	discoverCmd.Flags().Lookup("network").Usage = "Discover specific configured network by name"
	discoverCmd.Flags().Lookup("method").Usage = "Discovery method: ping (ICMP), tcp (TCP connect), " +
		"arp (ARP scan), or icmp (alias for ping)"
	discoverCmd.Flags().Lookup("timeout").Usage = "Maximum time to wait for discovery completion"
	discoverCmd.Flags().Lookup("add").Usage = "Add the discovered network to configured networks after discovery"
	discoverCmd.Flags().Lookup("name").Usage = "Name for the network when using --add (defaults to CIDR)"

	// Add shell completion for network names
	if err := discoverCmd.RegisterFlagCompletionFunc("network", completeNetworkNames); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register completion for network flag: %v\n", err)
	}
	if err := discoverCmd.RegisterFlagCompletionFunc("method", completeDiscoveryMethods); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register completion for method flag: %v\n", err)
	}
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

// calculateDiscoveryTimeout calculates realistic timeout based on network size and user timeout.
// It uses the user's specified timeout as a foundation and adds intelligent scaling based on
// the estimated number of hosts in the target network.
//
// Parameters:
//   - network: CIDR notation network (e.g., "192.168.1.0/24")
//   - baseTimeoutSeconds: User-specified timeout in seconds (must be >= 0)
//
// Returns:
//   - time.Duration representing the calculated timeout
//
// Examples:
//   - Single host (/32): baseTimeout + ~0.5s = ~30s for baseTimeout=30
//   - Small network (/28): baseTimeout + ~8s = ~38s for baseTimeout=30
//   - Standard /24: baseTimeout + ~127s = ~157s for baseTimeout=30
//   - Large network (/16): May reach hours for high user timeouts
func calculateDiscoveryTimeout(network string, baseTimeoutSeconds int) time.Duration {
	// Validate input parameters
	if baseTimeoutSeconds < 0 {
		baseTimeoutSeconds = 0 // Treat negative values as zero
	}

	// Estimate target count from CIDR
	targetCount := estimateNetworkTargets(network)

	// Calculate timeout: user base timeout + scaled time per host batch
	// Formula: baseTimeout + (targetCount * baseTimeoutPerHost) + buffer for concurrency
	// Use user's baseTimeout as the foundation, then add per-host scaling
	scalingTime := int(float64(targetCount) * baseTimeoutPerHost)

	// Prevent integer overflow by capping scaling time
	const maxScalingTime = 1000000 // ~11.5 days max scaling
	if scalingTime > maxScalingTime {
		scalingTime = maxScalingTime
	}

	timeoutSeconds := baseTimeoutSeconds + scalingTime

	// Add buffer for network latency and processing overhead
	if targetCount > batchOverheadThreshold {
		batchBuffer := (targetCount / timeoutConcurrency) * 2 // 2 seconds per batch of hosts
		// Cap batch buffer to prevent excessive timeouts
		if batchBuffer > maxBatchBufferSeconds { // Max 1 hour batch buffer
			batchBuffer = maxBatchBufferSeconds
		}
		timeoutSeconds += batchBuffer
	}

	// Apply reasonable minimum bound
	if timeoutSeconds < minTimeoutSeconds {
		timeoutSeconds = minTimeoutSeconds
	}

	// Only apply maximum timeout ceiling if user hasn't explicitly set a higher timeout
	// This allows users to override the 30-minute ceiling with --timeout flag
	if timeoutSeconds > maxTimeoutSeconds && baseTimeoutSeconds <= maxTimeoutSeconds {
		timeoutSeconds = maxTimeoutSeconds
	}

	return time.Duration(timeoutSeconds) * time.Second
}

// estimateNetworkTargets estimates the number of hosts in a network for timeout calculation.
// It parses CIDR notation and calculates the theoretical host count, applying safety caps
// to prevent excessive timeout calculations for very large networks.
//
// Parameters:
//   - network: CIDR notation string (e.g., "192.168.1.0/24", "10.0.0.0/8")
//
// Returns:
//   - int: Estimated number of targetable hosts (excludes network/broadcast addresses)
//
// Examples:
//   - "192.168.1.1/32" -> 1 host
//   - "192.168.1.0/24" -> 254 hosts
//   - "10.0.0.0/8" -> 65534 hosts (capped at /16 equivalent)
//   - "invalid" -> 254 hosts (default fallback)
func estimateNetworkTargets(network string) int {
	// Validate input - empty strings should use default
	if network == "" {
		return defaultNetworkSize
	}

	_, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		// If we can't parse the CIDR (invalid format, malformed network, etc.),
		// assume a reasonable default equivalent to /24 network for timeout calculation
		return defaultNetworkSize
	}

	ones, bits := ipNet.Mask.Size()

	// Sanity check: ensure we have valid mask data
	if ones < 0 || bits <= 0 || ones > bits {
		return defaultNetworkSize
	}

	hostBits := bits - ones

	// Safety cap: For very large networks (>/16), limit to /16 equivalent
	// This prevents timeout calculations from becoming unreasonably long
	// while still scaling appropriately for realistic network sizes
	const maxHostBits = 16 // Equivalent to /16 network (65534 hosts)
	if hostBits > maxHostBits {
		hostBits = maxHostBits
	}

	// Calculate target count: 2^hostBits - 2 (subtract network and broadcast addresses)
	// Use bit shifting for efficient power-of-2 calculation
	targetCount := (1 << hostBits) - 2

	// Ensure minimum of 1 host (handles /31 and /32 networks)
	if targetCount < 1 {
		targetCount = 1
	}

	return targetCount
}

// calculateDynamicTimeout calculates timeout based on network size and base timeout.
// This mirrors the logic

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
	// Validate method
	validMethods := map[string]bool{
		"tcp":  true,
		"ping": true,
		"arp":  true,
		"icmp": true,
	}
	if !validMethods[discoverMethod] {
		fmt.Fprintf(os.Stderr, "Error: invalid discovery method '%s'. Valid methods: tcp, ping, arp, icmp\n",
			discoverMethod)
		os.Exit(1)
	}

	// Determine target network(s)
	networks := determineTargetNetworks(cmd, args)

	withDatabaseOrExit(func(database *db.DB) {
		// Add network to database if --add flag is specified
		if discoverAdd && len(args) > 0 {
			addNetworkFromDiscovery(database, args[0])
		}
		// Calculate maximum dynamic timeout across all networks for engine and context setup
		var maxDynamicTimeout time.Duration
		for _, network := range networks {
			networkTimeout := calculateDiscoveryTimeout(network, discoverTimeout)
			if networkTimeout > maxDynamicTimeout {
				maxDynamicTimeout = networkTimeout
			}
		}

		// Create discovery engine with dynamic timeout
		engine := discovery.NewEngine(database)
		engine.SetConcurrency(defaultConcurrency)
		engine.SetTimeout(maxDynamicTimeout)

		// Create context with dynamic timeout (plus buffer for overhead)
		ctx, cancel := context.WithTimeout(context.Background(),
			maxDynamicTimeout+time.Duration(timeoutBufferSeconds)*time.Second)
		defer cancel()

		// Process each network
		for i, network := range networks {
			if len(networks) > 1 {
				fmt.Printf("\n=== Discovering Network %d/%d: %s ===\n", i+1, len(networks), network)
			}

			// Calculate dynamic timeout for this specific network
			networkTimeout := calculateDiscoveryTimeout(network, discoverTimeout)
			fmt.Printf("Estimated discovery time: %v (based on network size)\n", networkTimeout)

			// Create discovery configuration with dynamic timeout
			discoverConfig := &discovery.Config{
				Network:     network,
				Method:      discoverMethod,
				DetectOS:    discoverDetectOS,
				Timeout:     networkTimeout,
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

			// Wait for completion with progress updates (using the same dynamic timeout)
			err = waitForDiscoveryCompletion(ctx, database, job.ID, networkTimeout)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Discovery failed for %s: %v\n", network, err)
				continue
			}

			// Show final results
			showDiscoveryResults(database, job.ID)
		}

		fmt.Println("\nDiscovery complete!")
		fmt.Println("Use 'scanorama hosts' to view all discovered hosts")
		fmt.Println("Use 'scanorama networks list' to view network statistics")
		fmt.Println("Use 'scanorama scan --live-hosts' to scan discovered hosts")
	})
}

// determineTargetNetworks determines the target networks based on flags and arguments.
func determineTargetNetworks(cmd *cobra.Command, args []string) []string {
	if discoverAllNetworks {
		return handleAllNetworksDiscovery()
	}
	if discoverConfiguredNets {
		return handleConfiguredNetworksDiscovery()
	}
	if discoverNetworkName != "" {
		return handleSingleConfiguredNetworkDiscovery()
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

// handleConfiguredNetworksDiscovery handles discovery of all configured networks.
func handleConfiguredNetworksDiscovery() []string {
	var networks []string

	err := withDatabase(func(database *db.DB) error {
		query := `SELECT cidr FROM networks WHERE is_active = true ORDER BY name`
		rows, err := database.Query(query)
		if err != nil {
			return fmt.Errorf("failed to query configured networks: %w", err)
		}
		defer func() {
			if closeErr := rows.Close(); closeErr != nil {
				fmt.Printf("Warning: failed to close rows: %v\n", closeErr)
			}
		}()

		for rows.Next() {
			var cidr string
			if err := rows.Scan(&cidr); err != nil {
				fmt.Printf("Warning: failed to scan network: %v\n", err)
				continue
			}
			networks = append(networks, cidr)
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(networks) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No active configured networks found\n")
		fmt.Fprintf(os.Stderr, "Use 'scanorama networks add' to configure networks first\n")
		os.Exit(1)
	}

	fmt.Printf("Discovering %d configured networks: %s\n", len(networks), strings.Join(networks, ", "))
	return networks
}

// handleSingleConfiguredNetworkDiscovery handles discovery of a single configured network by name.
func handleSingleConfiguredNetworkDiscovery() []string {
	var cidr string

	err := withDatabase(func(database *db.DB) error {
		query := `SELECT cidr FROM networks WHERE name = $1 AND is_active = true`
		err := database.QueryRow(query, discoverNetworkName).Scan(&cidr)
		if err != nil {
			if err.Error() == sqlNoRowsError {
				return fmt.Errorf("network '%s' not found or not active", discoverNetworkName)
			}
			return fmt.Errorf("failed to query network '%s': %w", discoverNetworkName, err)
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Use 'scanorama networks list' to see available networks\n")
		os.Exit(1)
	}

	fmt.Printf("Discovering configured network '%s' (%s)\n", discoverNetworkName, cidr)
	return []string{cidr}
}

// addNetworkFromDiscovery adds a network to the database during discovery.
func addNetworkFromDiscovery(database *db.DB, cidrStr string) {
	// Validate CIDR
	_, ipnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid CIDR format '%s': %v\n", cidrStr, err)
		os.Exit(1)
	}

	// Determine network name
	networkName := discoverAddName
	if networkName == "" {
		networkName = cidrStr // Use CIDR as name if no name specified
	}

	// Check if network with same name or CIDR already exists
	var existingCount int
	checkQuery := `SELECT COUNT(*) FROM networks WHERE name = $1 OR cidr = $2`
	err = database.QueryRow(checkQuery, networkName, ipnet.String()).Scan(&existingCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking existing networks: %v\n", err)
		os.Exit(1)
	}

	if existingCount > 0 {
		fmt.Printf("Network '%s' (%s) already exists, skipping addition\n", networkName, ipnet.String())
		return
	}

	// Create network
	network := &db.Network{
		ID:              uuid.New(),
		Name:            networkName,
		CIDR:            db.NetworkAddr{IPNet: *ipnet},
		DiscoveryMethod: discoverMethod,
		IsActive:        true,
		ScanEnabled:     true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Insert into database
	insertQuery := `
		INSERT INTO networks (id, name, cidr, discovery_method, is_active, scan_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = database.Exec(insertQuery,
		network.ID,
		network.Name,
		network.CIDR.String(),
		network.DiscoveryMethod,
		network.IsActive,
		network.ScanEnabled,
		network.CreatedAt,
		network.UpdatedAt,
	)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding network: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Network '%s' (%s) added to configured networks\n", networkName, ipnet.String())
	fmt.Printf("   Discovery method: %s, Active: %t, Scan enabled: %t\n",
		network.DiscoveryMethod, network.IsActive, network.ScanEnabled)
}
