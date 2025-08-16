// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	// Network management constants.
	networksSeparatorLength = 140 // characters for network list separator
	maxDescriptionLength    = 30  // max description length before truncation
	defaultDiscoveryMethod  = "ping"
	// Display formatting constants
	exclusionsSeparatorLength = 100 // characters for exclusions list separator
	statsHeaderLength         = 50  // characters for stats section headers
	subnetMaskThreshold       = 31  // threshold for subnet mask display
)

var (
	networksShowInactive bool
	networksMethod       string
	networksName         string
	networksDescription  string
	networksCIDR         string
	networksActive       bool
	networksScanEnabled  bool

	// Exclusions flags
	exclusionsNetworkName string
	exclusionsGlobal      bool
	exclusionsCIDR        string
	exclusionsReason      string
)

// networksCmd represents the networks command.
var networksCmd = &cobra.Command{
	Use:   "networks",
	Short: "Manage network discovery targets",
	Long: `View and manage network discovery targets. Networks define CIDR ranges
that can be automatically discovered and scanned. You can configure discovery
methods, enable/disable networks, and view network statistics.`,
	Example: `  scanorama networks list
  scanorama networks add --name "corp-lan" --cidr 192.168.1.0/24
  scanorama networks remove corp-lan
  scanorama networks show corp-lan`,
}

// networksListCmd represents the networks list command.
var networksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured networks",
	Long: `List all configured network discovery targets with their status,
host counts, and last discovery/scan times.`,
	Example: `  scanorama networks list
  scanorama networks list --show-inactive`,
	Run: runNetworksList,
}

// networksAddCmd represents the networks add command.
var networksAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new network discovery target",
	Long: `Add a new network CIDR range for discovery and scanning.
The network will be validated and added to the database with the
specified configuration.`,
	Example: `  scanorama networks add --name "corp-lan" --cidr 192.168.1.0/24
  scanorama networks add --name "dmz-servers" --cidr 10.0.1.0/24 --method tcp --description "DMZ servers"
  scanorama networks add --name "lab-net" --cidr 172.16.0.0/16 --method arp --scan=false`,
	Run: runNetworksAdd,
}

// networksRemoveCmd represents the networks remove command.
var networksRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a network discovery target",
	Long: `Remove a network discovery target by name. This will also remove
all associated discovery history but will not remove discovered hosts.`,
	Example: `  scanorama networks remove corp-lan
  scanorama networks remove dmz-servers`,
	Args:              cobra.ExactArgs(1),
	Run:               runNetworksRemove,
	ValidArgsFunction: completeNetworkNames,
}

// networksShowCmd represents the networks show command.
var networksShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show detailed information about a network",
	Long: `Display detailed information about a specific network including
configuration, statistics, and recent activity.`,
	Example: `  scanorama networks show corp-lan
  scanorama networks show dmz-servers`,
	Args:              cobra.ExactArgs(1),
	Run:               runNetworksShow,
	ValidArgsFunction: completeNetworkNames,
}

// networksEnableCmd represents the networks enable command.
var networksEnableCmd = &cobra.Command{
	Use:   "enable [name]",
	Short: "Enable a network for discovery and scanning",
	Long: `Enable a network for automatic discovery and scanning operations.
This sets both is_active and scan_enabled to true.`,
	Example: `  scanorama networks enable corp-lan
  scanorama networks enable dmz-servers`,
	Args:              cobra.ExactArgs(1),
	Run:               runNetworksEnable,
	ValidArgsFunction: completeNetworkNames,
}

// networksDisableCmd represents the networks disable command.
var networksDisableCmd = &cobra.Command{
	Use:   "disable [name]",
	Short: "Disable a network from discovery and scanning",
	Long: `Disable a network from automatic discovery and scanning operations.
This sets both is_active and scan_enabled to false.`,
	Example: `  scanorama networks disable corp-lan
  scanorama networks disable dmz-servers`,
	Args:              cobra.ExactArgs(1),
	Run:               runNetworksDisable,
	ValidArgsFunction: completeNetworkNames,
}

// networksExclusionsCmd represents the networks exclusions command.
var networksExclusionsCmd = &cobra.Command{
	Use:   "exclusions",
	Short: "Manage network exclusions",
	Long: `Manage IP addresses and ranges that should be excluded from discovery
and scanning operations. Exclusions can be global (apply to all networks)
or network-specific.`,
	Example: `  scanorama networks exclusions list
  scanorama networks exclusions add --cidr 192.168.1.1/32 --reason "Router"
  scanorama networks exclusions add --network corp-lan --cidr 192.168.1.0/29 --reason "Management subnet"`,
}

// networksExclusionsListCmd represents the exclusions list command.
var networksExclusionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List network exclusions",
	Long: `List all configured network exclusions. Shows both global exclusions
and network-specific exclusions with their reasons and status.`,
	Example: `  scanorama networks exclusions list
  scanorama networks exclusions list --network corp-lan
  scanorama networks exclusions list --global`,
	Run: runNetworkExclusionsList,
}

// networksExclusionsAddCmd represents the exclusions add command.
var networksExclusionsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a network exclusion",
	Long: `Add an IP address or CIDR range to exclude from discovery and scanning.
Exclusions can be global (apply to all networks) or specific to a network.`,
	Example: `  scanorama networks exclusions add --cidr 192.168.1.1/32 --reason "Router"
  scanorama networks exclusions add --network corp-lan --cidr 192.168.1.0/29 --reason "Management subnet"
  scanorama networks exclusions add --cidr 10.0.0.0/8 --reason "Private range" --global`,
	Run: runNetworkExclusionsAdd,
}

// networksExclusionsRemoveCmd represents the exclusions remove command.
var networksExclusionsRemoveCmd = &cobra.Command{
	Use:   "remove [exclusion-id]",
	Short: "Remove a network exclusion",
	Long: `Remove a network exclusion by its ID. Use 'exclusions list' to see
exclusion IDs.`,
	Example: `  scanorama networks exclusions remove f47ac10b-58cc-4372-a567-0e02b2c3d479`,
	Args:    cobra.ExactArgs(1),
	Run:     runNetworkExclusionsRemove,
}

// networksRenameCmd represents the networks rename command.
var networksRenameCmd = &cobra.Command{
	Use:   "rename [current-name] [new-name]",
	Short: "Rename a network",
	Long: `Rename an existing network discovery target. The network will keep
all its configuration, exclusions, and statistics but with a new name.`,
	Example: `  scanorama networks rename corp-lan corporate-network
  scanorama networks rename "old name" "new name"`,
	Args:              cobra.ExactArgs(2),
	Run:               runNetworksRename,
	ValidArgsFunction: completeNetworkNames,
}

// networksStatsCmd represents the networks stats command.
var networksStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display network statistics",
	Long: `Display comprehensive statistics about configured networks including
total networks, active networks, host counts, and exclusion statistics.`,
	Example: `  scanorama networks stats`,
	Run:     runNetworksStats,
}

func init() {
	rootCmd.AddCommand(networksCmd)
	networksCmd.AddCommand(networksListCmd)
	networksCmd.AddCommand(networksAddCmd)
	networksCmd.AddCommand(networksRemoveCmd)
	networksCmd.AddCommand(networksShowCmd)
	networksCmd.AddCommand(networksEnableCmd)
	networksCmd.AddCommand(networksDisableCmd)
	networksCmd.AddCommand(networksRenameCmd)
	networksCmd.AddCommand(networksStatsCmd)
	networksCmd.AddCommand(networksExclusionsCmd)

	// Add exclusions subcommands
	networksExclusionsCmd.AddCommand(networksExclusionsListCmd)
	networksExclusionsCmd.AddCommand(networksExclusionsAddCmd)
	networksExclusionsCmd.AddCommand(networksExclusionsRemoveCmd)

	// List command flags
	networksListCmd.Flags().BoolVar(&networksShowInactive, "show-inactive", false, "Include inactive networks")

	// Add command flags
	networksAddCmd.Flags().StringVar(&networksName, "name", "", "Network name (required)")
	networksAddCmd.Flags().StringVar(&networksCIDR, "cidr", "", "Network CIDR (e.g., 192.168.1.0/24) (required)")
	networksAddCmd.Flags().StringVar(&networksMethod, "method", defaultDiscoveryMethod, "Discovery method")
	networksAddCmd.Flags().StringVar(&networksDescription, "description", "", "Network description")
	networksAddCmd.Flags().BoolVar(&networksActive, "active", true, "Enable network for discovery")
	networksAddCmd.Flags().BoolVar(&networksScanEnabled, "scan", true, "Enable network for scanning")

	// Mark required flags
	if err := networksAddCmd.MarkFlagRequired("name"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark name flag as required: %v\n", err)
	}
	if err := networksAddCmd.MarkFlagRequired("cidr"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark cidr flag as required: %v\n", err)
	}

	// Add flag descriptions
	networksAddCmd.Flags().Lookup("method").Usage = "Discovery method: tcp, ping, arp, icmp (default: ping)"
	networksAddCmd.Flags().Lookup("active").Usage = "Enable network for discovery operations"
	networksAddCmd.Flags().Lookup("scan").Usage = "Enable network for detailed scanning"

	// Add shell completion for discovery methods
	if err := networksAddCmd.RegisterFlagCompletionFunc("method", completeDiscoveryMethods); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register completion for method flag: %v\n", err)
	}

	// Exclusions command flags
	networksExclusionsListCmd.Flags().StringVar(&exclusionsNetworkName, "network", "",
		"Show exclusions for specific network")
	networksExclusionsListCmd.Flags().BoolVar(&exclusionsGlobal, "global", false, "Show only global exclusions")

	networksExclusionsAddCmd.Flags().StringVar(&exclusionsNetworkName, "network", "",
		"Network name for network-specific exclusion")
	networksExclusionsAddCmd.Flags().BoolVar(&exclusionsGlobal, "global", false,
		"Create global exclusion (applies to all networks)")
	networksExclusionsAddCmd.Flags().StringVar(&exclusionsCIDR, "cidr", "",
		"IP address or CIDR range to exclude (required)")
	networksExclusionsAddCmd.Flags().StringVar(&exclusionsReason, "reason", "", "Reason for exclusion")

	// Mark required flags for add command
	if err := networksExclusionsAddCmd.MarkFlagRequired("cidr"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark cidr flag as required: %v\n", err)
	}

	// Add completion for network names in exclusions commands
	if err := networksExclusionsListCmd.RegisterFlagCompletionFunc("network", completeNetworkNames); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register completion for network flag: %v\n", err)
	}
	if err := networksExclusionsAddCmd.RegisterFlagCompletionFunc("network", completeNetworkNames); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to register completion for network flag: %v\n", err)
	}
}

func runNetworksList(cmd *cobra.Command, args []string) {
	withDatabaseOrExit(func(database *db.DB) {
		networks, err := queryNetworks(database, networksShowInactive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying networks: %v\n", err)
			os.Exit(1)
		}

		displayNetworks(networks)
	})
}

func runNetworksAdd(cmd *cobra.Command, args []string) {
	// Validate CIDR
	_, ipnet, err := net.ParseCIDR(networksCIDR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid CIDR '%s': %v\n", networksCIDR, err)
		os.Exit(1)
	}

	// Validate discovery method
	validMethods := map[string]bool{
		"tcp":  true,
		"ping": true,
		"arp":  true,
		"icmp": true,
	}
	if !validMethods[networksMethod] {
		fmt.Fprintf(os.Stderr, "Error: invalid discovery method '%s'. Valid methods: tcp, ping, arp, icmp\n",
			networksMethod)
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Check if network with same name or CIDR already exists
		var existingCount int
		checkQuery := `SELECT COUNT(*) FROM networks WHERE name = $1 OR cidr = $2`
		err := database.QueryRow(checkQuery, networksName, ipnet.String()).Scan(&existingCount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking existing networks: %v\n", err)
			os.Exit(1)
		}

		if existingCount > 0 {
			fmt.Fprintf(os.Stderr, "Error: network with name '%s' or CIDR '%s' already exists\n",
				networksName, ipnet.String())
			os.Exit(1)
		}

		// Create network
		network := &db.Network{
			ID:              uuid.New(),
			Name:            networksName,
			CIDR:            db.NetworkAddr{IPNet: *ipnet},
			DiscoveryMethod: networksMethod,
			IsActive:        networksActive,
			ScanEnabled:     networksScanEnabled,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if networksDescription != "" {
			network.Description = &networksDescription
		}

		// Insert network
		insertQuery := `
			INSERT INTO networks (id, name, cidr, description, discovery_method,
				is_active, scan_enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

		_, err = database.Exec(insertQuery,
			network.ID,
			network.Name,
			network.CIDR.String(),
			network.Description,
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

		fmt.Printf("Network '%s' (%s) added successfully\n", networksName, ipnet.String())
		if verbose {
			fmt.Printf("Network ID: %s\n", network.ID)
			fmt.Printf("Discovery method: %s\n", networksMethod)
			fmt.Printf("Active: %t, Scan enabled: %t\n", networksActive, networksScanEnabled)
		}
	})
}

func runNetworksRemove(cmd *cobra.Command, args []string) {
	networkName := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		// Check if network exists
		var networkID uuid.UUID
		var cidr string
		checkQuery := `SELECT id, cidr FROM networks WHERE name = $1`
		err := database.QueryRow(checkQuery, networkName).Scan(&networkID, &cidr)
		if err != nil {
			if err.Error() == sqlNoRowsError {
				fmt.Fprintf(os.Stderr, "Error: network '%s' not found\n", networkName)
			} else {
				fmt.Fprintf(os.Stderr, "Error querying network: %v\n", err)
			}
			os.Exit(1)
		}

		// Remove network
		deleteQuery := `DELETE FROM networks WHERE id = $1`
		result, err := database.Exec(deleteQuery, networkID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing network: %v\n", err)
			os.Exit(1)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking deletion result: %v\n", err)
			os.Exit(1)
		}

		if rowsAffected == 0 {
			fmt.Fprintf(os.Stderr, "Error: network '%s' was not removed\n", networkName)
			os.Exit(1)
		}

		fmt.Printf("Network '%s' (%s) removed successfully\n", networkName, cidr)
	})
}

func runNetworksShow(cmd *cobra.Command, args []string) {
	networkName := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		network, err := getNetworkByName(database, networkName)
		if err != nil {
			if err.Error() == sqlNoRowsError {
				fmt.Fprintf(os.Stderr, "Error: network '%s' not found\n", networkName)
			} else {
				fmt.Fprintf(os.Stderr, "Error querying network: %v\n", err)
			}
			os.Exit(1)
		}

		displayNetworkDetails(network)
	})
}

func runNetworksEnable(cmd *cobra.Command, args []string) {
	networkName := args[0]
	updateNetworkStatus(networkName, true, true)
}

func runNetworksDisable(cmd *cobra.Command, args []string) {
	networkName := args[0]
	updateNetworkStatus(networkName, false, false)
}

func updateNetworkStatus(networkName string, active, scanEnabled bool) {
	withDatabaseOrExit(func(database *db.DB) {
		updateQuery := `
			UPDATE networks
			SET is_active = $1, scan_enabled = $2, updated_at = NOW()
			WHERE name = $3`

		result, err := database.Exec(updateQuery, active, scanEnabled, networkName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error updating network: %v\n", err)
			os.Exit(1)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking update result: %v\n", err)
			os.Exit(1)
		}

		if rowsAffected == 0 {
			fmt.Fprintf(os.Stderr, "Error: network '%s' not found\n", networkName)
			os.Exit(1)
		}

		status := "disabled"
		if active {
			status = "enabled"
		}
		fmt.Printf("Network '%s' %s successfully\n", networkName, status)
	})
}

func runNetworksRename(cmd *cobra.Command, args []string) {
	currentName := args[0]
	newName := args[1]

	withDatabaseOrExit(func(database *db.DB) {
		// Check if current network exists
		var networkID uuid.UUID
		var cidr string
		checkQuery := `SELECT id, cidr FROM networks WHERE name = $1`
		err := database.QueryRow(checkQuery, currentName).Scan(&networkID, &cidr)
		if err != nil {
			if err.Error() == sqlNoRowsError {
				fmt.Fprintf(os.Stderr, "Error: network '%s' not found\n", currentName)
			} else {
				fmt.Fprintf(os.Stderr, "Error querying network: %v\n", err)
			}
			os.Exit(1)
		}

		// Check if new name already exists
		var existingCount int
		nameCheckQuery := `SELECT COUNT(*) FROM networks WHERE name = $1 AND id != $2`
		err = database.QueryRow(nameCheckQuery, newName, networkID).Scan(&existingCount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking new name: %v\n", err)
			os.Exit(1)
		}

		if existingCount > 0 {
			fmt.Fprintf(os.Stderr, "Error: network with name '%s' already exists\n", newName)
			os.Exit(1)
		}

		// Rename the network
		updateQuery := `UPDATE networks SET name = $1, updated_at = NOW() WHERE id = $2`
		result, err := database.Exec(updateQuery, newName, networkID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error renaming network: %v\n", err)
			os.Exit(1)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking rename result: %v\n", err)
			os.Exit(1)
		}

		if rowsAffected == 0 {
			fmt.Fprintf(os.Stderr, "Error: network was not renamed\n")
			os.Exit(1)
		}

		fmt.Printf("Network '%s' renamed to '%s' successfully\n", currentName, newName)
		fmt.Printf("Network ID: %s, CIDR: %s\n", networkID, cidr)
	})
}

func queryNetworks(database *db.DB, showInactive bool) ([]db.Network, error) {
	query := `
		SELECT id, name, cidr, description, discovery_method, is_active, scan_enabled,
		       last_discovery, last_scan, host_count, active_host_count, created_at, updated_at, created_by
		FROM networks`

	args := []interface{}{}
	if !showInactive {
		query += " WHERE is_active = true"
	}

	query += " ORDER BY name"

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var networks []db.Network
	for rows.Next() {
		var network db.Network
		var cidrStr string

		err := rows.Scan(
			&network.ID,
			&network.Name,
			&cidrStr,
			&network.Description,
			&network.DiscoveryMethod,
			&network.IsActive,
			&network.ScanEnabled,
			&network.LastDiscovery,
			&network.LastScan,
			&network.HostCount,
			&network.ActiveHostCount,
			&network.CreatedAt,
			&network.UpdatedAt,
			&network.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		// Parse CIDR
		_, ipnet, err := net.ParseCIDR(cidrStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR in database: %w", err)
		}
		network.CIDR = db.NetworkAddr{IPNet: *ipnet}

		networks = append(networks, network)
	}

	return networks, nil
}

func getNetworkByName(database *db.DB, name string) (*db.Network, error) {
	query := `
		SELECT id, name, cidr, description, discovery_method, is_active, scan_enabled,
		       last_discovery, last_scan, host_count, active_host_count, created_at, updated_at, created_by
		FROM networks WHERE name = $1`

	var network db.Network
	var cidrStr string

	err := database.QueryRow(query, name).Scan(
		&network.ID,
		&network.Name,
		&cidrStr,
		&network.Description,
		&network.DiscoveryMethod,
		&network.IsActive,
		&network.ScanEnabled,
		&network.LastDiscovery,
		&network.LastScan,
		&network.HostCount,
		&network.ActiveHostCount,
		&network.CreatedAt,
		&network.UpdatedAt,
		&network.CreatedBy,
	)

	if err != nil {
		return nil, err
	}

	// Parse CIDR
	_, ipnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR in database: %w", err)
	}
	network.CIDR = db.NetworkAddr{IPNet: *ipnet}

	return &network, nil
}

func displayNetworks(networks []db.Network) {
	if len(networks) == 0 {
		fmt.Println("No networks found")
		return
	}

	// Print header
	fmt.Printf("Found %d network(s):\n\n", len(networks))
	fmt.Printf("%-36s %-20s %-18s %-8s %-8s %-8s %-6s %-6s %-30s\n",
		"ID", "Name", "CIDR", "Method", "Active", "Scan", "Hosts", "Up", "Description")
	fmt.Println(strings.Repeat("-", networksSeparatorLength))

	// Print networks
	for i := range networks {
		network := &networks[i]
		description := ""
		if network.Description != nil {
			description = *network.Description
			if len(description) > maxDescriptionLength {
				description = description[:maxDescriptionLength-3] + "..."
			}
		}

		activeStr := "No"
		if network.IsActive {
			activeStr = yesText
		}

		scanStr := "No"
		if network.ScanEnabled {
			scanStr = yesText
		}

		fmt.Printf("%-36s %-20s %-18s %-8s %-8s %-8s %-6d %-6d %-30s\n",
			network.ID.String(),
			network.Name,
			network.CIDR.String(),
			network.DiscoveryMethod,
			activeStr,
			scanStr,
			network.HostCount,
			network.ActiveHostCount,
			description)
	}

	// Print summary
	activeCount := 0
	totalHosts := 0
	totalActiveHosts := 0
	for i := range networks {
		network := &networks[i]
		if network.IsActive {
			activeCount++
		}
		totalHosts += network.HostCount
		totalActiveHosts += network.ActiveHostCount
	}

	fmt.Printf("\nSummary: %d total networks, %d active, %d total hosts, %d active hosts\n",
		len(networks), activeCount, totalHosts, totalActiveHosts)
}

func displayNetworkDetails(network *db.Network) {
	fmt.Printf("=== Network Details ===\n\n")
	fmt.Printf("ID: %s\n", network.ID)
	fmt.Printf("Name: %s\n", network.Name)
	fmt.Printf("CIDR: %s\n", network.CIDR.String())

	if network.Description != nil && *network.Description != "" {
		fmt.Printf("Description: %s\n", *network.Description)
	}

	fmt.Printf("Discovery Method: %s\n", network.DiscoveryMethod)
	fmt.Printf("Active: %t\n", network.IsActive)
	fmt.Printf("Scan Enabled: %t\n", network.ScanEnabled)

	fmt.Printf("\n=== Statistics ===\n")
	fmt.Printf("Total Hosts: %d\n", network.HostCount)
	fmt.Printf("Active Hosts: %d\n", network.ActiveHostCount)

	fmt.Printf("\n=== Timestamps ===\n")
	fmt.Printf("Created: %s\n", network.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", network.UpdatedAt.Format(time.RFC3339))

	if network.LastDiscovery != nil {
		fmt.Printf("Last Discovery: %s\n", network.LastDiscovery.Format(time.RFC3339))
	} else {
		fmt.Printf("Last Discovery: Never\n")
	}

	if network.LastScan != nil {
		fmt.Printf("Last Scan: %s\n", network.LastScan.Format(time.RFC3339))
	} else {
		fmt.Printf("Last Scan: Never\n")
	}

	if network.CreatedBy != nil && *network.CreatedBy != "" {
		fmt.Printf("Created By: %s\n", *network.CreatedBy)
	}

	// Calculate network information
	ones, bits := network.CIDR.Mask.Size()
	if bits == 32 { // IPv4
		totalAddresses := 1 << (32 - ones)
		if ones < subnetMaskThreshold {
			totalAddresses -= 2 // Subtract network and broadcast
		}
		fmt.Printf("\n=== Network Information ===\n")
		fmt.Printf("Network: %s\n", network.CIDR.IP.Mask(network.CIDR.Mask))
		fmt.Printf("Netmask: %s\n", net.IP(network.CIDR.Mask))
		fmt.Printf("Broadcast: %s\n", getBroadcastAddr(network.CIDR.IPNet))
		fmt.Printf("Total Addresses: %d\n", totalAddresses)
		if ones < subnetMaskThreshold {
			fmt.Printf("Usable Addresses: %d\n", totalAddresses)
		}
	}
}

func getBroadcastAddr(ipnet net.IPNet) net.IP {
	ip := ipnet.IP.Mask(ipnet.Mask)
	for i := 0; i < len(ip); i++ {
		ip[i] |= ^ipnet.Mask[i]
	}
	return ip
}

// Shell completion functions
func completeNetworkNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var networkNames []string

	// Try to get network names from database (ignore errors for completion)
	_ = withDatabase(func(database *db.DB) error {
		query := `SELECT name FROM networks WHERE name ILIKE $1 ORDER BY name LIMIT 20`
		rows, err := database.Query(query, toComplete+"%")
		if err != nil {
			return nil // Silent error in completion
		}
		defer func() {
			_ = rows.Close() // Ignore error in completion
		}()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				continue
			}
			networkNames = append(networkNames, name)
		}
		return nil
	})

	return networkNames, cobra.ShellCompDirectiveNoFileComp
}

func completeDiscoveryMethods(cmd *cobra.Command, args []string, toComplete string) (
	[]string, cobra.ShellCompDirective) {
	methods := []string{"tcp", "ping", "arp", "icmp"}
	var matches []string

	for _, method := range methods {
		if strings.HasPrefix(method, toComplete) {
			matches = append(matches, method)
		}
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

func runNetworkExclusionsList(cmd *cobra.Command, args []string) {
	withDatabaseOrExit(func(database *db.DB) {
		exclusions, err := queryNetworkExclusions(database, exclusionsNetworkName, exclusionsGlobal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying exclusions: %v\n", err)
			os.Exit(1)
		}

		displayNetworkExclusions(exclusions)
	})
}

// validateAndNormalizeCIDR validates a CIDR string or IP address and normalizes it to CIDR format.
func validateAndNormalizeCIDR(cidr string) (string, error) {
	// Try parsing as CIDR first
	if _, _, err := net.ParseCIDR(cidr); err == nil {
		return cidr, nil
	}

	// Try as single IP
	if ip := net.ParseIP(cidr); ip != nil {
		if strings.Contains(cidr, ":") {
			return cidr + "/128", nil // IPv6
		}
		return cidr + "/32", nil // IPv4
	}

	return "", fmt.Errorf("invalid CIDR or IP address")
}

func runNetworkExclusionsAdd(cmd *cobra.Command, args []string) {
	// Validate and normalize CIDR
	normalizedCIDR, err := validateAndNormalizeCIDR(exclusionsCIDR)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s '%s'\n", err.Error(), exclusionsCIDR)
		os.Exit(1)
	}
	exclusionsCIDR = normalizedCIDR

	// Validate mutual exclusivity of network and global flags
	if exclusionsNetworkName != "" && exclusionsGlobal {
		fmt.Fprintf(os.Stderr, "Error: cannot specify both --network and --global flags\n")
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		var networkID *uuid.UUID

		// Get network ID if network name is specified
		if exclusionsNetworkName != "" {
			network, err := getNetworkByName(database, exclusionsNetworkName)
			if err != nil {
				if err.Error() == sqlNoRowsError {
					fmt.Fprintf(os.Stderr, "Error: network '%s' not found\n", exclusionsNetworkName)
				} else {
					fmt.Fprintf(os.Stderr, "Error querying network: %v\n", err)
				}
				os.Exit(1)
			}
			networkID = &network.ID
		}

		// Create exclusion
		exclusion := &db.NetworkExclusion{
			ID:           uuid.New(),
			NetworkID:    networkID,
			ExcludedCIDR: exclusionsCIDR,
			Enabled:      true,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if exclusionsReason != "" {
			exclusion.Reason = &exclusionsReason
		}

		// Insert into database
		insertQuery := `
			INSERT INTO network_exclusions (id, network_id, excluded_cidr, reason, enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`

		_, err := database.Exec(insertQuery,
			exclusion.ID,
			exclusion.NetworkID,
			exclusion.ExcludedCIDR,
			exclusion.Reason,
			exclusion.Enabled,
			exclusion.CreatedAt,
			exclusion.UpdatedAt,
		)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding exclusion: %v\n", err)
			os.Exit(1)
		}

		scope := "global"
		if exclusionsNetworkName != "" {
			scope = fmt.Sprintf("network '%s'", exclusionsNetworkName)
		}

		fmt.Printf("Exclusion added successfully\n")
		fmt.Printf("CIDR: %s\n", exclusionsCIDR)
		fmt.Printf("Scope: %s\n", scope)
		if exclusionsReason != "" {
			fmt.Printf("Reason: %s\n", exclusionsReason)
		}
		if verbose {
			fmt.Printf("Exclusion ID: %s\n", exclusion.ID)
		}
	})
}

func runNetworkExclusionsRemove(cmd *cobra.Command, args []string) {
	exclusionIDStr := args[0]

	// Validate UUID
	exclusionID, err := uuid.Parse(exclusionIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid exclusion ID '%s': %v\n", exclusionIDStr, err)
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Check if exclusion exists and get details
		var cidr, reason string
		var networkID *uuid.UUID
		checkQuery := `SELECT excluded_cidr, reason, network_id FROM network_exclusions WHERE id = $1`
		err := database.QueryRow(checkQuery, exclusionID).Scan(&cidr, &reason, &networkID)
		if err != nil {
			if err.Error() == sqlNoRowsError {
				fmt.Fprintf(os.Stderr, "Error: exclusion with ID '%s' not found\n", exclusionIDStr)
			} else {
				fmt.Fprintf(os.Stderr, "Error querying exclusion: %v\n", err)
			}
			os.Exit(1)
		}

		// Remove exclusion
		deleteQuery := `DELETE FROM network_exclusions WHERE id = $1`
		result, err := database.Exec(deleteQuery, exclusionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing exclusion: %v\n", err)
			os.Exit(1)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking deletion result: %v\n", err)
			os.Exit(1)
		}

		if rowsAffected == 0 {
			fmt.Fprintf(os.Stderr, "Error: exclusion was not removed\n")
			os.Exit(1)
		}

		scope := "global"
		if networkID != nil {
			scope = "network-specific"
		}

		fmt.Printf("Exclusion removed successfully\n")
		fmt.Printf("CIDR: %s\n", cidr)
		fmt.Printf("Scope: %s\n", scope)
		if reason != "" {
			fmt.Printf("Reason: %s\n", reason)
		}
	})
}

func queryNetworkExclusions(database *db.DB, networkName string, globalOnly bool) ([]db.NetworkExclusion, error) {
	query := `
		SELECT ne.id, ne.network_id, ne.excluded_cidr, ne.reason, ne.enabled,
		       ne.created_at, ne.updated_at, ne.created_by
		FROM network_exclusions ne
		WHERE ne.enabled = true`

	args := []interface{}{}
	argIndex := 1

	if globalOnly {
		query += " AND ne.network_id IS NULL"
	} else if networkName != "" {
		query += fmt.Sprintf(" AND ne.network_id = (SELECT id FROM networks WHERE name = $%d)", argIndex)
		args = append(args, networkName)
	}
	query += " ORDER BY ne.created_at DESC"

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var exclusions []db.NetworkExclusion
	for rows.Next() {
		var exclusion db.NetworkExclusion
		err := rows.Scan(
			&exclusion.ID,
			&exclusion.NetworkID,
			&exclusion.ExcludedCIDR,
			&exclusion.Reason,
			&exclusion.Enabled,
			&exclusion.CreatedAt,
			&exclusion.UpdatedAt,
			&exclusion.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		exclusions = append(exclusions, exclusion)
	}

	return exclusions, nil
}

func displayNetworkExclusions(exclusions []db.NetworkExclusion) {
	if len(exclusions) == 0 {
		fmt.Println("No network exclusions found")
		return
	}

	fmt.Printf("Found %d exclusion(s):\n\n", len(exclusions))
	fmt.Printf("%-36s %-18s %-12s %-20s %s\n", "ID", "CIDR", "Scope", "Created", "Reason")
	fmt.Println(strings.Repeat("-", exclusionsSeparatorLength))

	for _, exclusion := range exclusions {
		scope := "Global"
		if exclusion.NetworkID != nil {
			scope = "Network"
		}

		reason := ""
		if exclusion.Reason != nil {
			reason = *exclusion.Reason
			if len(reason) > maxDescriptionLength {
				reason = reason[:27] + "..."
			}
		}

		fmt.Printf("%-36s %-18s %-12s %-20s %s\n",
			exclusion.ID.String(),
			exclusion.ExcludedCIDR,
			scope,
			exclusion.CreatedAt.Format("2006-01-02 15:04"),
			reason)
	}

	// Summary
	globalCount := 0
	networkCount := 0
	for _, exclusion := range exclusions {
		if exclusion.NetworkID == nil {
			globalCount++
		} else {
			networkCount++
		}
	}

	fmt.Printf("\nSummary: %d global exclusions, %d network-specific exclusions\n", globalCount, networkCount)
}

// runNetworksStats displays comprehensive network statistics.
func runNetworksStats(cmd *cobra.Command, args []string) {
	withDatabaseOrExit(func(database *db.DB) {
		// Get network statistics
		var stats struct {
			TotalNetworks       int `db:"total_networks"`
			ActiveNetworks      int `db:"active_networks"`
			ScanEnabledNetworks int `db:"scan_enabled_networks"`
			TotalHosts          int `db:"total_hosts"`
			TotalActiveHosts    int `db:"total_active_hosts"`
		}

		statsQuery := `
			SELECT
				COUNT(*) as total_networks,
				COUNT(*) FILTER (WHERE is_active = true) as active_networks,
				COUNT(*) FILTER (WHERE scan_enabled = true) as scan_enabled_networks,
				COALESCE(SUM(host_count), 0) as total_hosts,
				COALESCE(SUM(active_host_count), 0) as total_active_hosts
			FROM networks`

		err := database.Get(&stats, statsQuery)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get network statistics: %v\n", err)
			os.Exit(1)
		}

		// Get exclusion statistics
		var exclusionStats struct {
			TotalExclusions   int `db:"total_exclusions"`
			GlobalExclusions  int `db:"global_exclusions"`
			NetworkExclusions int `db:"network_exclusions"`
		}

		exclusionQuery := `
			SELECT
				COUNT(*) as total_exclusions,
				COUNT(*) FILTER (WHERE network_id IS NULL) as global_exclusions,
				COUNT(*) FILTER (WHERE network_id IS NOT NULL) as network_exclusions
			FROM network_exclusions
			WHERE enabled = true`

		err = database.Get(&exclusionStats, exclusionQuery)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get exclusion statistics: %v\n", err)
			os.Exit(1)
		}

		// Display statistics
		fmt.Println("Network Statistics")
		fmt.Println(strings.Repeat("=", statsHeaderLength))
		fmt.Printf("Total Networks:        %d\n", stats.TotalNetworks)
		fmt.Printf("Active Networks:       %d\n", stats.ActiveNetworks)
		fmt.Printf("Scan Enabled:          %d\n", stats.ScanEnabledNetworks)
		fmt.Printf("Inactive Networks:     %d\n", stats.TotalNetworks-stats.ActiveNetworks)
		fmt.Println()

		fmt.Println("Host Statistics")
		fmt.Println(strings.Repeat("=", statsHeaderLength))
		fmt.Printf("Total Hosts:           %d\n", stats.TotalHosts)
		fmt.Printf("Active Hosts:          %d\n", stats.TotalActiveHosts)
		fmt.Printf("Inactive Hosts:        %d\n", stats.TotalHosts-stats.TotalActiveHosts)
		fmt.Println()

		fmt.Println("Exclusion Statistics")
		fmt.Println(strings.Repeat("=", statsHeaderLength))
		fmt.Printf("Total Exclusions:      %d\n", exclusionStats.TotalExclusions)
		fmt.Printf("Global Exclusions:     %d\n", exclusionStats.GlobalExclusions)
		fmt.Printf("Network Exclusions:    %d\n", exclusionStats.NetworkExclusions)
	})
}
