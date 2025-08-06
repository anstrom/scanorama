package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/profiles"
	"github.com/anstrom/scanorama/internal/scheduler"
)

const (
	// Command line argument requirements.
	minArgsForSubcommand   = 2
	minArgsForProfileShow  = 2
	minArgsForScheduleAdd  = 4
	minArgsForScheduleScan = 4

	// Default timeout and concurrency values.
	defaultTimeoutSeconds = 3

	// Display formatting constants.
	TableSeparatorWidth = 120
	defaultConcurrency  = 50

	// Default values for scheduling.
	defaultMaxAge = 24
)

// Version information - populated by build flags.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) < minArgsForSubcommand {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "discover":
		runDiscovery(os.Args[2:])
	case "scan":
		runScan(os.Args[2:])
	case "migrate":
		runMigrate(os.Args[2:])
	case "hosts":
		runHosts(os.Args[2:])
	case "profiles":
		runProfiles(os.Args[2:])
	case "schedule":
		runSchedule(os.Args[2:])
	case "daemon":
		runDaemon(os.Args[2:])
	case "version":
		fmt.Printf("Scanorama %s\nCommit: %s\nBuild Time: %s\n", version, commit, buildTime)
	case "help", "-h", "--help":
		showUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		showUsage()
		os.Exit(1)
	}
}

func showUsage() {
	fmt.Fprintf(os.Stderr, `Scanorama - Advanced Network Scanner

Usage: scanorama <command> [options]

Commands:
  discover    Perform network discovery
  scan        Scan hosts (existing or specified)
  migrate     Run database migrations
  hosts       Manage discovered hosts
  profiles    Manage scan profiles
  schedule    Manage scheduled jobs
  daemon      Run as daemon service
  version     Show version information
  help        Show this help message

Discovery Commands:
  scanorama discover 192.168.1.0/24                    # Basic ping sweep
  scanorama discover 10.0.0.0/8 --detect-os           # Discovery with OS detection
  scanorama discover --all-networks --method arp       # ARP discovery on all networks

Scanning Commands:
  scanorama scan --live-hosts                          # Scan only discovered live hosts
  scanorama scan --targets 192.168.1.10-20            # Scan specific targets
  scanorama scan --targets host --type version         # Version detection scan
  scanorama scan --targets host --type intense         # Comprehensive scan with OS detection
  scanorama scan --targets host --type stealth         # Stealthy scan with slow timing
  scanorama scan --targets host --type comprehensive   # Full scan with scripts
  scanorama scan --live-hosts --profile auto          # Auto-select profiles by OS
  scanorama scan --os-family windows                  # Scan only Windows hosts

Host Management:
  scanorama hosts --status up                         # Show live hosts
  scanorama hosts --os windows --last-seen 24h       # Filter by OS and recency
  scanorama hosts ignore 192.168.1.1                 # Exclude from scanning

Profile Management:
  scanorama profiles list                             # Show all profiles
  scanorama profiles show windows-server             # Show specific profile
  scanorama profiles test linux-server --target host # Test profile

Scheduling:
  scanorama schedule add-discovery "weekly-sweep" "0 2 * * 0" 10.0.0.0/8
  scanorama schedule add-scan "daily-live" "0 */6 * * *" --live-hosts
  scanorama schedule list                             # Show scheduled jobs

Migration Commands:
  scanorama migrate up                                 # Apply pending migrations
  scanorama migrate reset                              # Reset database (WARNING: destroys data)

For detailed help on any command, use: scanorama <command> --help
	`)
}

func setupDatabase() (*db.DB, error) {
	// Load configuration from default paths
	configPaths := []string{"config.yaml", "config.yml", "scanorama.yaml", "scanorama.yml"}
	var cfg *config.Config
	var err error

	for _, path := range configPaths {
		if _, statErr := os.Stat(path); statErr == nil {
			cfg, err = config.Load(path)
			if err == nil {
				break
			}
		}
	}

	if cfg == nil {
		// Use defaults if no config file found
		cfg = config.Default()
	}

	// Connect to database
	ctx := context.Background()
	database, err := db.Connect(ctx, &cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return database, nil
}

func runDiscovery(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: scanorama discover <network> [options]\n")
		fmt.Fprintf(os.Stderr, "Example: scanorama discover 192.168.1.0/24 --detect-os\n")
		os.Exit(1)
	}

	network := args[0]
	detectOS := false
	method := "ping"

	// Parse additional flags
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--detect-os":
			detectOS = true
		case "--method":
			if i+1 < len(args) {
				method = args[i+1]
				i++
			}
		}
	}

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Create discovery engine
	discoveryEngine := discovery.NewEngine(database)

	// Run discovery
	ctx := context.Background()
	discoveryConfig := discovery.Config{
		Network:     network,
		Method:      method,
		DetectOS:    detectOS,
		Timeout:     defaultTimeoutSeconds * time.Second,
		Concurrency: defaultConcurrency,
	}

	fmt.Printf("Starting discovery of %s (method: %s, OS detection: %v)\n", network, method, detectOS)

	job, err := discoveryEngine.Discover(ctx, discoveryConfig)
	if err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Discovery failed: %v", err) //nolint:gocritic // intentional exit after cleanup
	}

	fmt.Printf("Discovery job started: %s\n", job.ID)
	fmt.Println("Discovery running...")

	// Wait for discovery to complete by polling the job status
	for {
		time.Sleep(2 * time.Second)

		// Check job status in database
		query := `SELECT status, hosts_discovered, completed_at FROM discovery_jobs WHERE id = $1`
		var status string
		var hostsDiscovered int
		var completedAt *time.Time

		err := database.QueryRowContext(ctx, query, job.ID).Scan(&status, &hostsDiscovered, &completedAt)
		if err != nil {
			log.Printf("Error checking discovery status: %v", err)
			break
		}

		if status == "completed" || status == "failed" {
			fmt.Printf("Discovery %s. Found %d hosts.\n", status, hostsDiscovered)
			break
		}

		// Timeout after 60 seconds
		if time.Since(*job.StartedAt) > 60*time.Second {
			fmt.Println("Discovery timed out.")
			break
		}
	}

	fmt.Println("Use 'scanorama hosts' to view discovered hosts.")
}

// scanOptions holds the parsed command line options for scan command.
type scanOptions struct {
	targets    string
	liveHosts  bool
	ports      string
	scanType   string
	profileID  string
	timeoutSec int
}

// parseScanArgs parses command line arguments for the scan command.
func parseScanArgs(args []string) scanOptions {
	opts := scanOptions{
		ports:      "22,80,443,8080,8443",
		scanType:   "connect",
		timeoutSec: defaultTimeoutSeconds,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--targets":
			if i+1 < len(args) {
				opts.targets = args[i+1]
				i++
			}
		case "--live-hosts":
			opts.liveHosts = true
		case "--ports":
			if i+1 < len(args) {
				opts.ports = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				opts.scanType = args[i+1]
				i++
			}
		case "--profile":
			if i+1 < len(args) {
				opts.profileID = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				if t, err := strconv.Atoi(args[i+1]); err == nil && t > 0 {
					opts.timeoutSec = t
				}
				i++
			}
		}
	}

	return opts
}

func runScan(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: scanorama scan [options]\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --live-hosts                    # Scan discovered live hosts\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets 192.168.1.10-20      # Scan specific targets\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets localhost --ports 80,443  # Scan specific ports\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets example.com --type version   # Version detection scan\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets 10.0.0.1 --type intense\n")
		fmt.Fprintf(os.Stderr, "    # Comprehensive scan with OS detection\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets target --type stealth\n")
		fmt.Fprintf(os.Stderr, "    # Stealthy scan with slow timing\n")
		fmt.Fprintf(os.Stderr, "  scanorama scan --targets host --timeout 30           # Scan with 30 second timeout\n")
		os.Exit(1)
	}

	opts := parseScanArgs(args)

	// Validate options
	if !opts.liveHosts && opts.targets == "" {
		fmt.Fprintf(os.Stderr, "Error: Must specify either --live-hosts or --targets\n")
		os.Exit(1)
	}

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	ctx := context.Background()

	if opts.liveHosts {
		fmt.Println("Scanning discovered live hosts...")
		err = scanLiveHosts(ctx, database, opts.ports, opts.scanType, opts.profileID, opts.timeoutSec)
	} else {
		fmt.Printf("Scanning targets: %s\n", opts.targets)
		err = scanTargets(ctx, database, opts.targets, opts.ports, opts.scanType, opts.profileID, opts.timeoutSec)
	}

	if err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Scan failed: %v", err) //nolint:gocritic // intentional exit after cleanup
	}

	fmt.Println("Scan completed successfully")
}

func scanLiveHosts(ctx context.Context, database *db.DB, ports, scanType, _ string, timeoutSec int) error {
	// Get live hosts from database
	query := `
		SELECT ip_address, hostname, os_family
		FROM hosts
		WHERE status = 'up' AND ignore_scanning = false
		ORDER BY last_seen DESC
		LIMIT 100
	`

	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query live hosts: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var hosts []string
	for rows.Next() {
		var ipAddr string
		var hostname, osFamily *string

		if err := rows.Scan(&ipAddr, &hostname, &osFamily); err != nil {
			log.Printf("Failed to scan host row: %v", err)
			continue
		}

		hosts = append(hosts, ipAddr)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over host rows: %w", err)
	}

	if len(hosts) == 0 {
		fmt.Println("No live hosts found to scan")
		return nil
	}

	fmt.Printf("Found %d live hosts to scan\n", len(hosts))
	return performScan(ctx, database, hosts, ports, scanType, timeoutSec)
}

func scanTargets(ctx context.Context, database *db.DB, targets, ports, scanType,
	_ string, timeoutSec int) error {
	// For now, split comma-separated targets
	// TODO: Add support for ranges and CIDR notation
	hostList := strings.Split(targets, ",")

	hosts := make([]string, 0, 10) // Pre-allocate with reasonable capacity
	for _, host := range hostList {
		hosts = append(hosts, strings.TrimSpace(host))
	}

	return performScan(ctx, database, hosts, ports, scanType, timeoutSec)
}

func performScan(ctx context.Context, database *db.DB, hosts []string, ports, scanType string, timeoutSec int) error {
	// Create scan configuration
	scanConfig := &internal.ScanConfig{
		Targets:     hosts,
		Ports:       ports,
		ScanType:    scanType,
		TimeoutSec:  timeoutSec,
		Concurrency: defaultConcurrency,
	}

	fmt.Printf("Starting %s scan of %d target(s)...\n", scanType, len(hosts))
	fmt.Printf("Ports: %s\n", ports)
	fmt.Printf("Timeout: %d seconds\n", timeoutSec)
	fmt.Println()

	// Perform the actual nmap scan
	result, err := internal.RunScanWithContext(ctx, scanConfig, database)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Display results
	internal.PrintResults(result)

	// Print scan summary
	fmt.Printf("\nScan Summary:\n")
	fmt.Printf("  Duration: %v\n", result.Duration)
	fmt.Printf("  Hosts up: %d\n", result.Stats.Up)
	fmt.Printf("  Hosts down: %d\n", result.Stats.Down)
	fmt.Printf("  Total hosts: %d\n", result.Stats.Total)

	return nil
}

// hostsOptions holds the parsed command line options for hosts command.
type hostsOptions struct {
	status        string
	osFamily      string
	lastSeenHours int
	showIgnored   bool
}

// parseHostsArgs parses command line arguments for the hosts command.
func parseHostsArgs(args []string) (hostsOptions, bool) {
	opts := hostsOptions{
		lastSeenHours: 24,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 < len(args) {
				opts.status = args[i+1]
				i++
			}
		case "--os":
			if i+1 < len(args) {
				opts.osFamily = args[i+1]
				i++
			}
		case "--last-seen":
			if i+1 < len(args) {
				switch args[i+1] {
				case "24h":
					opts.lastSeenHours = 24
				case "7d":
					opts.lastSeenHours = 168
				case "30d":
					opts.lastSeenHours = 720
				}
				i++
			}
		case "--include-ignored":
			opts.showIgnored = true
		case "ignore":
			if i+1 < len(args) {
				handleHostIgnore(args[i+1])
				return opts, true // early return handled
			}
		}
	}

	return opts, false
}

func runHosts(args []string) {
	opts, handled := parseHostsArgs(args)
	if handled {
		return
	}

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	ctx := context.Background()
	err = showHosts(ctx, database, opts.status, opts.osFamily, opts.lastSeenHours, opts.showIgnored)
	if err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Failed to show hosts: %v", err) //nolint:gocritic // intentional exit after cleanup
	}
}

// buildHostsQuery constructs the SQL query and arguments for host filtering.
func buildHostsQuery(status, osFamily string, lastSeenHours int, showIgnored bool) (query string, args []interface{}) {
	query = `
		SELECT ip_address, hostname, mac_address, vendor, os_family, os_name,
		       status, last_seen, ignore_scanning, discovery_count
		FROM hosts
		WHERE 1=1
	`
	argIndex := 1

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	if osFamily != "" {
		query += fmt.Sprintf(" AND os_family ILIKE $%d", argIndex)
		args = append(args, "%"+osFamily+"%")
		_ = argIndex // argIndex will be used when more filters are added
	}

	if lastSeenHours > 0 {
		query += fmt.Sprintf(" AND last_seen > NOW() - INTERVAL '%d hours'", lastSeenHours)
	}

	if !showIgnored {
		query += " AND ignore_scanning = false"
	}

	query += " ORDER BY last_seen DESC LIMIT 100"
	return
}

// displayHostsHeader prints the table header for host results.
func displayHostsHeader() {
	fmt.Printf("%-15s %-20s %-17s %-15s %-10s %-10s %-19s %s\n",
		"IP Address", "Hostname", "MAC Address", "OS Family", "Status", "Ignored", "Last Seen", "Count")
	fmt.Println(strings.Repeat("-", TableSeparatorWidth))
}

// formatHostRow scans a database row and formats it for display.
func formatHostRow(rows *sql.Rows) error {
	var ipAddr string
	var hostname, macAddr, vendor, osFamily, osName, status *string
	var lastSeen time.Time
	var ignoreScanning bool
	var discoveryCount int

	err := rows.Scan(&ipAddr, &hostname, &macAddr, &vendor, &osFamily, &osName,
		&status, &lastSeen, &ignoreScanning, &discoveryCount)
	if err != nil {
		return err
	}

	hostnameStr := ""
	if hostname != nil {
		hostnameStr = *hostname
	}

	macAddrStr := ""
	if macAddr != nil {
		macAddrStr = *macAddr
	}

	osFamilyStr := ""
	if osFamily != nil {
		osFamilyStr = *osFamily
	}

	statusStr := ""
	if status != nil {
		statusStr = *status
	}

	ignoredStr := "No"
	if ignoreScanning {
		ignoredStr = "Yes"
	}

	fmt.Printf("%-15s %-20s %-17s %-15s %-10s %-10s %-19s %d\n",
		ipAddr, hostnameStr, macAddrStr, osFamilyStr, statusStr, ignoredStr,
		lastSeen.Format("2006-01-02 15:04:05"), discoveryCount)

	return nil
}

func showHosts(ctx context.Context, database *db.DB, status, osFamily string,
	lastSeenHours int, showIgnored bool) error {
	query, args := buildHostsQuery(status, osFamily, lastSeenHours, showIgnored)

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to query hosts: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	displayHostsHeader()

	count := 0
	for rows.Next() {
		if err := formatHostRow(rows); err != nil {
			log.Printf("Failed to scan host row: %v", err)
			continue
		}
		count++
	}

	if count == 0 {
		fmt.Println("No hosts found matching the criteria")
	} else {
		fmt.Printf("\nTotal: %d hosts\n", count)
	}

	return rows.Err()
}

func handleHostIgnore(ipAddr string) {
	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	ctx := context.Background()
	query := `UPDATE hosts SET ignore_scanning = true WHERE ip_address = $1`
	result, err := database.ExecContext(ctx, query, ipAddr)
	if err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Failed to ignore host: %v", err) //nolint:gocritic // intentional exit after cleanup
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Warning: Could not get affected rows: %v", err)
	}

	if rowsAffected > 0 {
		fmt.Printf("Host %s is now ignored for scanning\n", ipAddr)
	} else {
		fmt.Printf("Host %s not found in database\n", ipAddr)
	}
}

func runMigrate(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: scanorama migrate <command>\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up      Apply pending migrations\n")
		fmt.Fprintf(os.Stderr, "  reset   Reset database (WARNING: destroys all data)\n")
		os.Exit(1)
	}

	command := args[0]

	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		cfg = config.Default()
	}

	// Connect to database (without auto-migration)
	ctx := context.Background()
	database, err := db.Connect(ctx, &cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close database: %v\n", err)
		}
	}()

	migrator := db.NewMigrator(database.DB)

	switch command {
	case "up":
		fmt.Println("Applying pending migrations...")
		if err := migrator.Up(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			return
		}
		fmt.Println("Migrations applied successfully")

	case "reset":
		fmt.Println("WARNING: This will destroy all data in the database!")
		fmt.Print("Type 'yes' to confirm: ")
		var confirmation string
		if _, err := fmt.Scanln(&confirmation); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read confirmation: %v\n", err)
			return
		}
		if confirmation != "yes" {
			fmt.Println("Migration reset canceled")
			return
		}

		if err := migrator.Reset(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Database reset failed: %v\n", err)
			return
		}
		fmt.Println("Database reset and migrations applied successfully")

	default:
		fmt.Fprintf(os.Stderr, "Unknown migration command: %s\n", command)
		fmt.Fprintf(os.Stderr, "Available commands: up, reset\n")
		return
	}
}

func runProfiles(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: scanorama profiles <list|show|create|delete> [options]\n")
		os.Exit(1)
	}

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	profileManager := profiles.NewManager(database)
	ctx := context.Background()

	switch args[0] {
	case "list":
		handleProfilesList(ctx, profileManager, database)
	case "show":
		handleProfilesShow(ctx, profileManager, args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown profiles command: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "Available commands: list, show\n")
		return
	}
}

func handleProfilesList(ctx context.Context, profileManager *profiles.Manager, database *db.DB) {
	profileList, err := profileManager.GetAll(ctx)
	if err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Failed to get profiles: %v", err)
	}

	fmt.Printf("Available Scan Profiles:\n\n")
	for _, profile := range profileList {
		builtIn := ""
		if profile.BuiltIn {
			builtIn = " (built-in)"
		}
		fmt.Printf("%-20s %s%s\n", profile.ID, profile.Name, builtIn)
		fmt.Printf("  Description: %s\n", profile.Description)
		fmt.Printf("  OS Family:   %v\n", profile.OSFamily)
		fmt.Printf("  Scan Type:   %s\n", profile.ScanType)
		fmt.Printf("  Timing:      %s\n", profile.Timing)
		fmt.Printf("  Ports:       %s\n", profile.Ports)
		fmt.Println()
	}
}

func handleProfilesShow(ctx context.Context, profileManager *profiles.Manager, args []string) {
	if len(args) < minArgsForProfileShow {
		fmt.Fprintf(os.Stderr, "Usage: scanorama profiles show <profile-id>\n")
		return
	}

	profile, err := profileManager.GetByID(ctx, args[1])
	if err != nil {
		log.Fatalf("Failed to get profile: %v", err)
	}

	if profile == nil {
		fmt.Printf("Profile '%s' not found\n", args[1])
		return
	}

	fmt.Printf("Profile: %s\n", profile.Name)
	fmt.Printf("ID: %s\n", profile.ID)
	fmt.Printf("Description: %s\n", profile.Description)
	fmt.Printf("OS Family: %v\n", profile.OSFamily)
	fmt.Printf("Scan Type: %s\n", profile.ScanType)
	fmt.Printf("Timing: %s\n", profile.Timing)
	fmt.Printf("Ports: %s\n", profile.Ports)
	fmt.Printf("Built-in: %v\n", profile.BuiltIn)
}

func runSchedule(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: scanorama schedule <list|add-discovery|add-scan|remove> [options]\n")
		os.Exit(1)
	}

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	discoveryEngine := discovery.NewEngine(database)
	profileManager := profiles.NewManager(database)
	sched := scheduler.NewScheduler(database, discoveryEngine, profileManager)

	ctx := context.Background()

	switch args[0] {
	case "list":
		handleScheduleList(sched)
	case "add-discovery":
		handleScheduleAddDiscovery(ctx, sched, args, database)
	case "add-scan":
		handleScheduleAddScan(ctx, sched, args, database)
	default:
		fmt.Fprintf(os.Stderr, "Unknown schedule command: %s\n", args[0])
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		os.Exit(1) //nolint:gocritic // intentional exit after cleanup
	}
}

func handleScheduleList(sched *scheduler.Scheduler) {
	jobs := sched.GetJobs()
	if len(jobs) == 0 {
		fmt.Println("No scheduled jobs found.")
		return
	}

	fmt.Printf("Scheduled Jobs:\n\n")
	for _, job := range jobs {
		status := "enabled"
		if !job.Config.Enabled {
			status = "disabled"
		}
		fmt.Printf("Name: %s (%s)\n", job.Config.Name, status)
		fmt.Printf("Type: %s\n", job.Config.Type)
		fmt.Printf("Schedule: %s\n", job.Config.CronExpression)
		fmt.Printf("Next Run: %s\n", job.NextRun.Format("2006-01-02 15:04:05"))
		if job.Config.LastRun != nil {
			fmt.Printf("Last Run: %s\n", job.Config.LastRun.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}
}

func handleScheduleAddDiscovery(ctx context.Context, sched *scheduler.Scheduler, args []string, database *db.DB) {
	if len(args) < minArgsForScheduleAdd {
		fmt.Fprintln(os.Stderr, "Usage: scanorama schedule add-discovery <name> <cron> <network> [--detect-os]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr,
			"Example: scanorama schedule add-discovery \"weekly-sweep\" \"0 2 * * 0\" \"10.0.0.0/8\" --detect-os\n")
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		os.Exit(1)
	}

	name := args[1]
	cronExpr := args[2]
	network := args[3]
	detectOS := false

	for i := 4; i < len(args); i++ {
		if args[i] == "--detect-os" {
			detectOS = true
		}
	}

	jobConfig := scheduler.DiscoveryJobConfig{
		Network:     network,
		Method:      "ping",
		DetectOS:    detectOS,
		Timeout:     defaultTimeoutSeconds,
		Concurrency: defaultConcurrency,
	}

	if err := sched.AddDiscoveryJob(ctx, name, cronExpr, jobConfig); err != nil {
		log.Fatalf("Failed to add discovery job: %v", err)
	}

	fmt.Printf("Discovery job '%s' scheduled successfully\n", name)
}

func handleScheduleAddScan(ctx context.Context, sched *scheduler.Scheduler, args []string, database *db.DB) {
	if len(args) < minArgsForScheduleScan {
		fmt.Fprintln(os.Stderr, "Usage: scanorama schedule add-scan <name> <cron> [--live-hosts] [--profile <name>]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintf(os.Stderr,
			"Example: scanorama schedule add-scan \"daily-scan\" \"0 */6 * * *\" --live-hosts --profile auto\n")
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		os.Exit(1)
	}

	name := args[1]
	cronExpr := args[2]
	liveHostsOnly := false
	profileID := "auto"

	for i := 3; i < len(args); i++ {
		switch args[i] {
		case "--live-hosts":
			liveHostsOnly = true
		case "--profile":
			if i+1 < len(args) {
				profileID = args[i+1]
				i++
			}
		}
	}

	jobConfig := scheduler.ScanJobConfig{
		LiveHostsOnly: liveHostsOnly,
		ProfileID:     profileID,
		MaxAge:        defaultMaxAge,
	}

	if err := sched.AddScanJob(ctx, name, cronExpr, &jobConfig); err != nil {
		log.Fatalf("Failed to add scan job: %v", err)
	}

	fmt.Printf("Scan job '%s' scheduled successfully\n", name)
}

func runDaemon(_ []string) {
	fmt.Println("Starting Scanorama daemon...")

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Create components
	discoveryEngine := discovery.NewEngine(database)
	profileManager := profiles.NewManager(database)
	sched := scheduler.NewScheduler(database, discoveryEngine, profileManager)

	// Start scheduler
	if err := sched.Start(); err != nil {
		if err := database.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
		log.Fatalf("Failed to start scheduler: %v", err) //nolint:gocritic // intentional exit after cleanup
	}
	defer sched.Stop()

	fmt.Println("Daemon started. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down daemon...")
}
