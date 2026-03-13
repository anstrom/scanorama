// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file contains the command run functions for the networks subcommands.
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
