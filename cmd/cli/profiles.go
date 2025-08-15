// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
)

const (
	profileDetailSeparator   = 50
	maxProfilesLimit         = 100
	profileListSeparatorLen  = 100
	maxOSFamilyDisplayLen    = 15
	maxDescriptionDisplayLen = 40
	maxPortsBeforeTruncate   = 5
)

var (
	profileTestTarget string
	profileTestDryRun bool
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
	Long: `Display a list of all available scan profiles with their
basic information including name, description, and target OS family.`,
	Run: runProfilesList,
}

// profilesShowCmd represents the profiles show command.
var profilesShowCmd = &cobra.Command{
	Use:   "show <profile-name>",
	Short: "Show details of a specific scan profile",
	Long: `Display detailed information about a specific scan profile
including all configuration options, port specifications, and usage examples.`,
	Args: cobra.ExactArgs(1),
	Run:  runProfilesShow,
}

// profilesTestCmd represents the profiles test command.
var profilesTestCmd = &cobra.Command{
	Use:   "test <profile-name>",
	Short: "Test a scan profile against a target",
	Long: `Test a scan profile configuration against a specific target host.
This allows you to verify the profile works as expected before using it
in production scans or scheduled jobs.`,
	Args: cobra.ExactArgs(1),
	Run:  runProfilesTest,
}

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
	profilesTestCmd.Flags().Lookup("dry-run").Usage = "Show scan configuration without actually running the scan"
}

func runProfilesList(_ *cobra.Command, _ []string) {
	withDatabaseOrExit(func(database *db.DB) {
		ctx := context.Background()
		profiles, _, err := database.ListProfiles(ctx, db.ProfileFilters{}, 0, maxProfilesLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying profiles: %v\n", err)
			os.Exit(1)
		}

		displayProfiles(profiles)
	})
}

func runProfilesShow(_ *cobra.Command, args []string) {
	profileName := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		profile, err := getProfileByName(database, profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying profile '%s': %v\n", profileName, err)
			os.Exit(1)
		}

		displayProfileDetails(profile)
	})
}

func runProfilesTest(_ *cobra.Command, args []string) {
	profileName := args[0]

	// Validate target IP
	if err := validateIP(profileTestTarget); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid target IP '%s': %v\n", profileTestTarget, err)
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		profile, err := getProfileByName(database, profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying profile '%s': %v\n", profileName, err)
			os.Exit(1)
		}

		if profileTestDryRun {
			displayTestConfiguration(profile, profileTestTarget)
		} else {
			runTestScan(database, profile, profileTestTarget)
		}
	})
}

func getProfileByName(database *db.DB, name string) (*db.ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, ports, scan_type,
		       timing, scripts, options, priority, built_in,
		       created_at, updated_at
		FROM scan_profiles
		WHERE name = $1`

	profile := &db.ScanProfile{}
	var osFamily, scripts interface{}
	var options []byte

	err := database.QueryRow(query, name).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Description,
		&osFamily,
		&profile.Ports,
		&profile.ScanType,
		&profile.Timing,
		&scripts,
		&options,
		&profile.Priority,
		&profile.BuiltIn,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("profile '%s' not found: %w", name, err)
	}

	// Handle array fields
	if osFamily != nil {
		if arr, ok := osFamily.([]interface{}); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					profile.OSFamily = append(profile.OSFamily, str)
				}
			}
		}
	}

	if scripts != nil {
		if arr, ok := scripts.([]interface{}); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					profile.Scripts = append(profile.Scripts, str)
				}
			}
		}
	}

	// Handle JSONB options
	if len(options) > 0 {
		profile.Options = db.JSONB(options)
	}

	return profile, nil
}

func displayProfiles(profiles []*db.ScanProfile) {
	if len(profiles) == 0 {
		fmt.Println("No scan profiles found")
		return
	}

	fmt.Printf("Found %d scan profile(s):\n\n", len(profiles))

	// Header
	fmt.Printf("%-20s %-12s %-15s %-40s %s\n", "Name", "Scan Type", "OS Family", "Description", "Built-in")
	fmt.Println(strings.Repeat("-", profileListSeparatorLen))

	// Display each profile
	for _, profile := range profiles {
		osFamily := "Any"
		if len(profile.OSFamily) > 0 {
			osFamily = strings.Join(profile.OSFamily, ",")
			if len(osFamily) > maxOSFamilyDisplayLen {
				osFamily = osFamily[:12] + "..."
			}
		}

		description := profile.Description
		if len(description) > maxDescriptionDisplayLen {
			description = description[:37] + "..."
		}

		builtIn := "No"
		if profile.BuiltIn {
			builtIn = "Yes"
		}

		fmt.Printf("%-20s %-12s %-15s %-40s %s\n",
			profile.Name,
			profile.ScanType,
			osFamily,
			description,
			builtIn)
	}

	fmt.Printf("\nUse 'scanorama profiles show <name>' to view profile details\n")
	fmt.Printf("Use 'scanorama profiles test <name> --target <host>' to test a profile\n")
}

func displayProfileDetails(profile *db.ScanProfile) {
	fmt.Printf("Profile: %s\n", profile.Name)
	fmt.Println(strings.Repeat("=", profileDetailSeparator))
	fmt.Printf("ID: %s\n", profile.ID)
	fmt.Printf("Description: %s\n", profile.Description)

	if len(profile.OSFamily) > 0 {
		fmt.Printf("OS Family: %s\n", strings.Join(profile.OSFamily, ", "))
	} else {
		fmt.Printf("OS Family: Any\n")
	}

	if len(profile.OSPattern) > 0 {
		fmt.Printf("OS Patterns: %s\n", strings.Join(profile.OSPattern, ", "))
	}

	fmt.Printf("Scan Type: %s\n", profile.ScanType)
	fmt.Printf("Ports: %s\n", profile.Ports)
	fmt.Printf("Timing: %s\n", profile.Timing)
	fmt.Printf("Priority: %d\n", profile.Priority)
	fmt.Printf("Built-in: %t\n", profile.BuiltIn)
	fmt.Printf("Created: %s\n", profile.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", profile.UpdatedAt.Format("2006-01-02 15:04:05"))

	if len(profile.Scripts) > 0 {
		fmt.Printf("Scripts: %s\n", strings.Join(profile.Scripts, ", "))
	}

	if len(profile.Options) > 0 {
		fmt.Printf("Options: %+v\n", profile.Options)
	}

	// Parse and display port information
	if profile.Ports != "" {
		fmt.Println("\nPort Configuration:")
		displayPortList(profile.Ports)
	}

	fmt.Printf("\nTo use this profile:\n")
	fmt.Printf("  scanorama scan --targets <host> --profile %s\n", profile.Name)
	fmt.Printf("  scanorama profiles test %s --target <host>\n", profile.Name)
}

func displayPortList(ports string) {
	// Parse port specification and provide helpful information
	if strings.Contains(ports, ",") {
		portList := strings.Split(ports, ",")
		fmt.Printf("  Specific ports: %d ports specified\n", len(portList))

		// Show first few ports as example
		if len(portList) <= 10 {
			fmt.Printf("  Ports: %s\n", ports)
		} else {
			first5 := strings.Join(portList[:5], ",")
			fmt.Printf("  Ports: %s... (and %d more)\n", first5, len(portList)-maxPortsBeforeTruncate)
		}
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

func displayTestConfiguration(profile *db.ScanProfile, target string) {
	fmt.Printf("Test Configuration for Profile: %s\n", profile.Name)
	fmt.Println(strings.Repeat("=", profileDetailSeparator))
	fmt.Printf("Target: %s\n", target)
	fmt.Printf("Scan Type: %s\n", profile.ScanType)
	fmt.Printf("Ports: %s\n", profile.Ports)
	fmt.Printf("Timing: %s\n", profile.Timing)

	if len(profile.Scripts) > 0 {
		fmt.Printf("Scripts: %s\n", strings.Join(profile.Scripts, ", "))
	}

	fmt.Println("\nThis configuration would perform:")
	displayPortList(profile.Ports)

	fmt.Printf("\nTo actually run this test:\n")
	fmt.Printf("  scanorama profiles test %s --target %s\n", profile.Name, target)
}

func runTestScan(database *db.DB, profile *db.ScanProfile, target string) {
	fmt.Printf("Testing profile '%s' against target '%s'...\n", profile.Name, target)
	fmt.Println(strings.Repeat("-", profileDetailSeparator))

	fmt.Printf("Scan configuration:\n")
	fmt.Printf("  Target: %s\n", target)
	fmt.Printf("  Profile: %s (%s)\n", profile.Name, profile.Description)
	fmt.Printf("  Scan type: %s\n", profile.ScanType)
	fmt.Printf("  Ports: %s\n", profile.Ports)
	fmt.Printf("  Timing: %s\n", profile.Timing)
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

	fmt.Printf("\nProfile test completed successfully!\n")
	fmt.Printf("Hosts scanned: %d\n", len(result.Hosts))
	fmt.Printf("Open ports found: %d\n", totalPorts)

	// Display results
	if len(result.Hosts) > 0 {
		fmt.Printf("\nScan Results:\n")
		internal.PrintResults(result)
	}

	fmt.Printf("\nProfile '%s' test completed\n", profile.Name)
}
