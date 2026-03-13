// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
