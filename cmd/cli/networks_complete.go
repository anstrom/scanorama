// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file contains shell completion helper functions for the networks subcommands.
package cli

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/db"
)

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
