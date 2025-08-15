// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
)

const (
	// Host management constants.
	hostsSeparatorLength = 100 // characters for host list separator
	maxOSNameLength      = 18  // max OS name length before truncation
	ipAddressParts       = 4   // expected parts in IP address
	hoursPerDay          = 24  // hours in a day for duration calculation
)

var (
	hostsStatus      string
	hostsOSFamily    string
	hostsLastSeen    string
	hostsShowIgnored bool
)

// hostsCmd represents the hosts command.
var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage discovered hosts",
	Long: `View and manage hosts that have been discovered during network scans.
You can filter hosts by status (up/down), operating system family,
last seen time, and whether to include ignored hosts.`,
	Example: `  scanorama hosts
  scanorama hosts --status up
  scanorama hosts --os windows --last-seen 24h
  scanorama hosts --status down --show-ignored
  scanorama hosts ignore 192.168.1.1`,
	Run: runHosts,
}

// hostsIgnoreCmd represents the hosts ignore command.
var hostsIgnoreCmd = &cobra.Command{
	Use:   "ignore [IP]",
	Short: "Ignore a host from future scans",
	Long: `Mark a host as ignored so it will be excluded from future
automatic scans. The host will still appear in host listings
unless --show-ignored is used.`,
	Example: `  scanorama hosts ignore 192.168.1.1
  scanorama hosts ignore 10.0.0.1`,
	Args: cobra.ExactArgs(1),
	Run:  runHostsIgnore,
}

// hostsUnignoreCmd represents the hosts unignore command.
var hostsUnignoreCmd = &cobra.Command{
	Use:   "unignore [IP]",
	Short: "Remove ignore flag from a host",
	Long: `Remove the ignore flag from a host so it will be included
in future automatic scans again.`,
	Example: `  scanorama hosts unignore 192.168.1.1
  scanorama hosts unignore 10.0.0.1`,
	Args: cobra.ExactArgs(1),
	Run:  runHostsUnignore,
}

func init() {
	rootCmd.AddCommand(hostsCmd)
	hostsCmd.AddCommand(hostsIgnoreCmd)
	hostsCmd.AddCommand(hostsUnignoreCmd)

	// Define flags for the main hosts command
	hostsCmd.Flags().StringVar(&hostsStatus, "status", "", "Filter by host status: up, down")
	hostsCmd.Flags().StringVar(&hostsOSFamily, "os", "", "Filter by OS family: windows, linux, macos, unknown")
	hostsCmd.Flags().StringVar(&hostsLastSeen, "last-seen", "", "Filter by last seen time (e.g., 1h, 24h, 7d)")
	hostsCmd.Flags().BoolVar(&hostsShowIgnored, "show-ignored", false, "Include ignored hosts in results")

	// Add detailed flag descriptions
	hostsCmd.Flags().Lookup("status").Usage = "Show only hosts with specific status " +
		"(up = responsive, down = not responding)"
	hostsCmd.Flags().Lookup("os").Usage = "Filter by detected operating system family"
	hostsCmd.Flags().Lookup("last-seen").Usage = "Show hosts seen within time period (1h, 6h, 24h, 7d, 30d)"
	hostsCmd.Flags().Lookup("show-ignored").Usage = "Include hosts marked as ignored in the results"
}

func runHosts(_ *cobra.Command, _ []string) {
	// Validate last-seen format if provided
	if hostsLastSeen != "" {
		if err := validateDuration(hostsLastSeen); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid last-seen format '%s': %v\n", hostsLastSeen, err)
			fmt.Fprintf(os.Stderr, "Valid formats: 1h, 6h, 24h, 7d, 30d\n")
			os.Exit(1)
		}
	}

	// Validate status if provided
	if hostsStatus != "" {
		validStatuses := map[string]bool{
			"up":   true,
			"down": true,
		}
		if !validStatuses[hostsStatus] {
			fmt.Fprintf(os.Stderr, "Error: invalid status '%s'. Valid statuses: up, down\n", hostsStatus)
			os.Exit(1)
		}
	}

	// Validate OS family if provided
	if hostsOSFamily != "" {
		validOSFamilies := map[string]bool{
			"windows": true,
			"linux":   true,
			"macos":   true,
			"unknown": true,
		}
		if !validOSFamilies[hostsOSFamily] {
			fmt.Fprintf(os.Stderr, "Error: invalid OS family '%s'. "+
				"Valid families: windows, linux, macos, unknown\n", hostsOSFamily)
			os.Exit(1)
		}
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Build query filters
		filters := buildHostsFilters()

		if verbose {
			fmt.Printf("Querying hosts with filters: %+v\n", filters)
		}

		// Query hosts
		hosts, err := queryHosts(database, filters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying hosts: %v\n", err)
			os.Exit(1)
		}

		// Display results
		displayHosts(hosts)
	})
}

func runHostsIgnore(_ *cobra.Command, args []string) {
	ip := args[0]

	withHostDatabaseOperation(ip, func(database *db.DB) error {
		// Set ignore flag
		err := setHostIgnoreFlag(database, ip, true)
		if err != nil {
			return fmt.Errorf("ignoring host %s: %v", ip, err)
		}

		fmt.Printf("Host %s has been marked as ignored\n", ip)
		fmt.Println("It will be excluded from future automatic scans")
		return nil
	})
}

func runHostsUnignore(_ *cobra.Command, args []string) {
	ip := args[0]

	withHostDatabaseOperation(ip, func(database *db.DB) error {
		// Remove ignore flag
		err := setHostIgnoreFlag(database, ip, false)
		if err != nil {
			return fmt.Errorf("unignoring host %s: %v", ip, err)
		}

		fmt.Printf("Host %s is no longer ignored\n", ip)
		fmt.Println("It will be included in future automatic scans")
		return nil
	})
}

// HostFilters represents the filters for querying hosts.
type HostFilters struct {
	Status      string
	OSFamily    string
	LastSeenDur time.Duration
	ShowIgnored bool
}

// Host represents a discovered host.
type Host struct {
	IP              string
	Status          string
	OSFamily        string
	OSName          string
	LastSeen        time.Time
	FirstSeen       time.Time
	IsIgnored       bool
	OpenPorts       int
	TotalScans      int
	DiscoveryMethod string
}

func buildHostsFilters() HostFilters {
	filters := HostFilters{
		Status:      hostsStatus,
		OSFamily:    hostsOSFamily,
		ShowIgnored: hostsShowIgnored,
	}

	if hostsLastSeen != "" {
		dur, _ := parseDuration(hostsLastSeen)
		filters.LastSeenDur = dur
	}

	return filters
}

func queryHosts(database *db.DB, filters HostFilters) ([]Host, error) {
	// Build SQL query based on filters
	query := `
		SELECT
			h.ip_address,
			h.status,
			COALESCE(h.os_family, 'unknown') as os_family,
			COALESCE(h.os_name, 'Unknown') as os_name,
			h.last_seen,
			h.first_seen,
			COALESCE(h.ignore_scanning, false) as ignore_scanning,
			COALESCE(h.discovery_method, '') as discovery_method,
			COUNT(DISTINCT ps.id) as open_ports,
			COUNT(DISTINCT sj.id) as total_scans
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id AND ps.state = 'open'
		LEFT JOIN scan_jobs sj ON h.id = sj.target_id
		WHERE 1=1`

	args := []interface{}{}
	argIndex := 1

	// Add status filter
	if filters.Status != "" {
		query += fmt.Sprintf(" AND h.status = $%d", argIndex)
		args = append(args, filters.Status)
		argIndex++
	}

	// Add OS family filter
	if filters.OSFamily != "" {
		query += fmt.Sprintf(" AND h.os_family = $%d", argIndex)
		args = append(args, filters.OSFamily)
		argIndex++
	}

	// Add last seen filter
	if filters.LastSeenDur > 0 {
		query += fmt.Sprintf(" AND h.last_seen >= $%d", argIndex)
		args = append(args, time.Now().Add(-filters.LastSeenDur))
	}

	// Add ignored filter
	if !filters.ShowIgnored {
		query += " AND (h.ignore_scanning IS NULL OR h.ignore_scanning = false)"
	}

	query += `
		GROUP BY h.id, h.ip_address, h.status, h.os_family, h.os_name, h.last_seen, h.first_seen,
			h.ignore_scanning, h.discovery_method
		ORDER BY h.last_seen DESC, h.ip_address`

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var hosts []Host
	for rows.Next() {
		var h Host
		err := rows.Scan(
			&h.IP,
			&h.Status,
			&h.OSFamily,
			&h.OSName,
			&h.LastSeen,
			&h.FirstSeen,
			&h.IsIgnored,
			&h.DiscoveryMethod,
			&h.OpenPorts,
			&h.TotalScans,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		hosts = append(hosts, h)
	}

	return hosts, nil
}

func displayHosts(hosts []Host) {
	if len(hosts) == 0 {
		fmt.Println("No hosts found matching the specified criteria")
		return
	}

	// Print header
	fmt.Printf("Found %d host(s):\n\n", len(hosts))
	fmt.Printf("%-15s %-8s %-12s %-20s %-12s %-8s %-8s %s\n",
		"IP Address", "Status", "OS Family", "OS Name", "Last Seen", "Method", "Ports", "Ignored")
	fmt.Println(strings.Repeat("-", hostsSeparatorLength))

	// Print hosts
	for i := range hosts {
		host := &hosts[i]
		lastSeenStr := formatDuration(time.Since(host.LastSeen))
		ignoredStr := ""
		if host.IsIgnored {
			ignoredStr = "YES"
		}

		osName := host.OSName
		if len(osName) > maxOSNameLength {
			osName = osName[:maxOSNameLength-3] + "..."
		}

		fmt.Printf("%-15s %-8s %-12s %-20s %-12s %-8s %-8d %s\n",
			host.IP,
			host.Status,
			host.OSFamily,
			osName,
			lastSeenStr,
			host.DiscoveryMethod,
			host.OpenPorts,
			ignoredStr)
	}

	// Print summary
	upCount := 0
	downCount := 0
	ignoredCount := 0
	for i := range hosts {
		host := &hosts[i]
		if host.Status == "up" {
			upCount++
		} else {
			downCount++
		}
		if host.IsIgnored {
			ignoredCount++
		}
	}

	fmt.Printf("\nSummary: %d up, %d down", upCount, downCount)
	if ignoredCount > 0 {
		fmt.Printf(", %d ignored", ignoredCount)
	}
	fmt.Println()
}

func setHostIgnoreFlag(database *db.DB, ip string, ignored bool) error {
	query := `UPDATE hosts SET ignore_scanning = $1 WHERE ip_address = $2`
	result, err := database.Exec(query, ignored, ip)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("host %s not found in database", ip)
	}

	return nil
}

func validateDuration(duration string) error {
	_, err := parseDuration(duration)
	return err
}

func parseDuration(duration string) (time.Duration, error) {
	// Handle common duration formats
	duration = strings.ToLower(strings.TrimSpace(duration))

	// Parse using Go's standard duration parsing first
	if dur, err := time.ParseDuration(duration); err == nil {
		return dur, nil
	}

	// Handle day notation (e.g., "7d", "30d")
	if strings.HasSuffix(duration, "d") {
		daysStr := strings.TrimSuffix(duration, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day format: %s", duration)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return 0, fmt.Errorf("invalid duration format: %s", duration)
}

func validateIP(ip string) error {
	// Simple IP validation - in real implementation you'd use net.ParseIP
	parts := strings.Split(ip, ".")
	if len(parts) != ipAddressParts {
		return fmt.Errorf("invalid IP format")
	}

	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return fmt.Errorf("invalid IP octet: %s", part)
		}
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	days := int(d.Hours() / hoursPerDay)
	return fmt.Sprintf("%dd", days)
}
