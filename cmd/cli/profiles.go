// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
)

const (
	// Profile management constants.
	profileSeparatorLength = 85 // characters for profile list separator
	maxDescriptionLength   = 35 // max description length before truncation
	profileDetailSeparator = 50 // characters for profile detail separator
	maxPortsToShow         = 5  // maximum ports to show before truncation
)

// profilesCmd represents the profiles command.
var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Manage scan profiles",
	Long: `View and manage scan profiles that define scanning configurations
for different operating systems and use cases. Profiles contain
predefined port lists, scan types, and timing configurations.`,
	Example: `  scanorama profiles list
  scanorama profiles show windows-server
  scanorama profiles test linux-server --target 192.168.1.10`,
}

// profilesListCmd represents the profiles list command.
var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available scan profiles",
	Long: `Display all available scan profiles with their descriptions
and basic configuration information.`,
	Run: runProfilesList,
}

// profilesShowCmd represents the profiles show command.
var profilesShowCmd = &cobra.Command{
	Use:   "show [profile-name]",
	Short: "Show details of a specific scan profile",
	Long: `Display detailed information about a specific scan profile
including its port configuration, scan types, and timing settings.`,
	Example: `  scanorama profiles show windows-server
  scanorama profiles show linux-workstation`,
	Args: cobra.ExactArgs(1),
	Run:  runProfilesShow,
}

// profilesTestCmd represents the profiles test command.
var profilesTestCmd = &cobra.Command{
	Use:   "test [profile-name]",
	Short: "Test a scan profile against a target",
	Long: `Test a scan profile by running it against a specified target
to verify the configuration and see what results it produces.`,
	Example: `  scanorama profiles test windows-server --target 192.168.1.10
  scanorama profiles test linux-server --target localhost`,
	Args: cobra.ExactArgs(1),
	Run:  runProfilesTest,
}

var (
	profileTestTarget string
	profileTestDryRun bool
)

func init() {
	rootCmd.AddCommand(profilesCmd)
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesShowCmd)
	profilesCmd.AddCommand(profilesTestCmd)

	// Flags for test command
	profilesTestCmd.Flags().StringVar(&profileTestTarget, "target", "", "Target host to test profile against")
	profilesTestCmd.Flags().BoolVar(&profileTestDryRun, "dry-run", false, "Show what would be scanned without running")

	// Mark target as required for test command
	if err := profilesTestCmd.MarkFlagRequired("target"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark target flag as required: %v\n", err)
	}

	// Add detailed descriptions
	profilesTestCmd.Flags().Lookup("target").Usage = "Target host or IP address to test the profile against"
	profilesTestCmd.Flags().Lookup("dry-run").Usage = "Display scan configuration without executing the scan"
}

func runProfilesList(cmd *cobra.Command, args []string) {
	withDatabaseOrExit(func(database *db.DB) {
		// Query profiles using safe scanning approach
		profiles, err := queryProfiles(database)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying profiles: %v\n", err)
			os.Exit(1)
		}

		displayProfiles(profiles)
	})
}

func runProfilesShow(cmd *cobra.Command, args []string) {
	profileName := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		profile, err := getProfile(database, profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying profile '%s': %v\n", profileName, err)
			os.Exit(1)
		}

		displayProfileDetails(profile)
	})
}

func runProfilesTest(cmd *cobra.Command, args []string) {
	profileName := args[0]

	// Validate target
	if profileTestTarget == "" {
		fmt.Fprintf(os.Stderr, "Error: target is required for profile testing\n")
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Get profile details
		profile, err := getProfile(database, profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting profile '%s': %v\n", profileName, err)
			os.Exit(1)
		}

		if profileTestDryRun {
			// Show what would be scanned
			displayTestConfiguration(profile, profileTestTarget)
		} else {
			// Actually run the test scan
			runTestScan(database, profile, profileTestTarget)
		}
	})
}

// ScanProfile represents a scan profile.
type ScanProfile struct {
	ID            string
	Name          string
	Description   string
	OSFamily      string
	ScanType      string
	Ports         string
	TimingLevel   int
	IsActive      bool
	CreatedAt     string
	CustomScripts string
}

func queryProfiles(database *db.DB) ([]ScanProfile, error) {
	// Use safe approach to avoid pq.StringArray scanning issues
	query := `
		SELECT
			id, name, description, os_family, scan_type,
			ports, timing_level, is_active, created_at,
			COALESCE(custom_scripts, '') as custom_scripts
		FROM scan_profiles
		WHERE is_active = true
		ORDER BY os_family, name`

	rows, err := database.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var profiles []ScanProfile
	for rows.Next() {
		var profile ScanProfile

		err := rows.Scan(
			&profile.ID,
			&profile.Name,
			&profile.Description,
			&profile.OSFamily,
			&profile.ScanType,
			&profile.Ports,
			&profile.TimingLevel,
			&profile.IsActive,
			&profile.CreatedAt,
			&profile.CustomScripts,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func getProfile(database *db.DB, name string) (*ScanProfile, error) {
	query := `
		SELECT
			id, name, description, os_family, scan_type,
			ports, timing_level, is_active, created_at,
			COALESCE(custom_scripts, '') as custom_scripts
		FROM scan_profiles
		WHERE name = $1`

	var profile ScanProfile

	err := database.QueryRow(query, name).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Description,
		&profile.OSFamily,
		&profile.ScanType,
		&profile.Ports,
		&profile.TimingLevel,
		&profile.IsActive,
		&profile.CreatedAt,
		&profile.CustomScripts,
	)
	if err != nil {
		return nil, fmt.Errorf("profile '%s' not found: %w", name, err)
	}

	return &profile, nil
}

func displayProfiles(profiles []ScanProfile) {
	if len(profiles) == 0 {
		fmt.Println("No scan profiles found")
		return
	}

	fmt.Printf("Found %d scan profile(s):\n\n", len(profiles))
	fmt.Printf("%-25s %-12s %-12s %-15s %s\n",
		"Name", "OS Family", "Scan Type", "Timing", "Description")
	fmt.Println(strings.Repeat("-", profileSeparatorLength))

	currentOSFamily := ""
	for i := range profiles {
		profile := &profiles[i]
		// Group by OS family
		if profile.OSFamily != currentOSFamily {
			if currentOSFamily != "" {
				fmt.Println()
			}
			currentOSFamily = profile.OSFamily
		}

		timingStr := fmt.Sprintf("T%d", profile.TimingLevel)
		description := profile.Description
		if len(description) > maxDescriptionLength {
			description = description[:maxDescriptionLength-3] + "..."
		}

		fmt.Printf("%-25s %-12s %-12s %-15s %s\n",
			profile.Name,
			profile.OSFamily,
			profile.ScanType,
			timingStr,
			description)
	}

	fmt.Printf("\nUse 'scanorama profiles show <name>' for detailed information\n")
	fmt.Printf("Use 'scanorama profiles test <name> --target <host>' to test a profile\n")
}

func displayProfileDetails(profile *ScanProfile) {
	fmt.Printf("Scan Profile: %s\n", profile.Name)
	fmt.Println(strings.Repeat("=", profileDetailSeparator))
	fmt.Printf("ID: %s\n", profile.ID)
	fmt.Printf("Description: %s\n", profile.Description)
	fmt.Printf("OS Family: %s\n", profile.OSFamily)
	fmt.Printf("Scan Type: %s\n", profile.ScanType)
	fmt.Printf("Ports: %s\n", profile.Ports)
	fmt.Printf("Timing Level: T%d\n", profile.TimingLevel)
	fmt.Printf("Active: %t\n", profile.IsActive)
	fmt.Printf("Created: %s\n", profile.CreatedAt)

	if profile.CustomScripts != "" {
		fmt.Printf("Custom Scripts: %s\n", profile.CustomScripts)
	}

	// Parse and display port information
	if profile.Ports != "" {
		fmt.Println("\nPort Configuration:")
		displayPortInfo(profile.Ports)
	}

	fmt.Printf("\nTo use this profile:\n")
	fmt.Printf("  scanorama scan --targets <host> --profile %s\n", profile.Name)
	fmt.Printf("  scanorama profiles test %s --target <host>\n", profile.Name)
}

func displayPortInfo(ports string) {
	// Parse port specification and provide helpful information
	if strings.Contains(ports, ",") {
		displayPortList(ports)
		return
	}

	if strings.Contains(ports, "-") {
		fmt.Printf("  Port range: %s\n", ports)
		return
	}

	if strings.HasPrefix(ports, "T:") {
		fmt.Printf("  Top ports: %s\n", ports)
		return
	}

	fmt.Printf("  Port specification: %s\n", ports)
}

func displayPortList(ports string) {
	portList := strings.Split(ports, ",")
	fmt.Printf("  Specific ports: %d ports specified\n", len(portList))

	// Show first few ports as example
	if len(portList) <= 10 {
		fmt.Printf("  Ports: %s\n", ports)
		return
	}

	first5 := strings.Join(portList[:maxPortsToShow], ",")
	fmt.Printf("  Ports: %s... (and %d more)\n", first5, len(portList)-maxPortsToShow)
}

func displayTestConfiguration(profile *ScanProfile, target string) {
	fmt.Printf("Test Configuration for Profile: %s\n", profile.Name)
	fmt.Println(strings.Repeat("=", profileDetailSeparator))
	fmt.Printf("Target: %s\n", target)
	fmt.Printf("Scan Type: %s\n", profile.ScanType)
	fmt.Printf("Ports: %s\n", profile.Ports)
	fmt.Printf("Timing Level: T%d\n", profile.TimingLevel)

	if profile.CustomScripts != "" {
		fmt.Printf("Custom Scripts: %s\n", profile.CustomScripts)
	}

	fmt.Println("\nThis configuration would perform:")
	displayPortInfo(profile.Ports)

	fmt.Printf("\nTo actually run this test:\n")
	fmt.Printf("  scanorama profiles test %s --target %s\n", profile.Name, target)
}

func runTestScan(database *db.DB, profile *ScanProfile, target string) {
	fmt.Printf("Testing profile '%s' against target '%s'...\n", profile.Name, target)
	fmt.Println(strings.Repeat("-", profileDetailSeparator))

	fmt.Printf("Scan configuration:\n")
	fmt.Printf("  Target: %s\n", target)
	fmt.Printf("  Profile: %s (%s)\n", profile.Name, profile.Description)
	fmt.Printf("  Scan type: %s\n", profile.ScanType)
	fmt.Printf("  Ports: %s\n", profile.Ports)
	fmt.Printf("  Timing: T%d\n", profile.TimingLevel)
	fmt.Println()

	// Create scan configuration from profile
	scanConfig := &internal.ScanConfig{
		Targets:    []string{target},
		Ports:      profile.Ports,
		ScanType:   profile.ScanType,
		TimeoutSec: 300, // Default timeout
	}

	fmt.Printf("Starting profile test scan...\n")
	result, err := internal.RunScanWithDB(scanConfig, database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Profile test scan failed: %v\n", err)
		os.Exit(1)
	}

	// Calculate total ports
	totalPorts := 0
	for _, host := range result.Hosts {
		totalPorts += len(host.Ports)
	}

	// Display results
	fmt.Printf("\nProfile test completed successfully!\n")
	fmt.Printf("Hosts scanned: %d\n", len(result.Hosts))
	fmt.Printf("Total ports found: %d\n", totalPorts)

	// Show summary of open ports found
	openPorts := 0
	for _, host := range result.Hosts {
		for _, port := range host.Ports {
			if port.State == "open" {
				openPorts++
				fmt.Printf("  %s:%d (%s) - %s\n", host.Address, port.Number, port.Protocol, port.Service)
			}
		}
	}

	if openPorts == 0 {
		fmt.Printf("No open ports found on target %s\n", target)
	} else {
		fmt.Printf("Found %d open port(s)\n", openPorts)
	}
}
