// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file contains database query and display helper functions for the networks subcommands.
package cli

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

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
