package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/profiles"
	"github.com/anstrom/scanorama/internal/scheduler"
)

// Version information - populated by build flags
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "discover":
		runDiscovery(os.Args[2:])
	case "scan":
		runScan(os.Args[2:])
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

For detailed help on any command, use: scanorama <command> --help
`)
}

func setupDatabase() (*db.DB, error) {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to database
	ctx := context.Background()
	database, err := db.Connect(ctx, cfg.Database)
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
	defer database.Close()

	// Create discovery engine
	discoveryEngine := discovery.NewEngine(database)

	// Run discovery
	ctx := context.Background()
	config := discovery.Config{
		Network:     network,
		Method:      method,
		DetectOS:    detectOS,
		Timeout:     3 * time.Second,
		Concurrency: 50,
	}

	fmt.Printf("Starting discovery of %s (method: %s, OS detection: %v)\n", network, method, detectOS)

	job, err := discoveryEngine.Discover(ctx, config)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	fmt.Printf("Discovery job started: %s\n", job.ID)
	fmt.Println("Discovery running in background. Use 'scanorama hosts' to view results.")
}

func runScan(args []string) {
	fmt.Println("Scan command - implementation coming soon")
	// TODO: Implement scanning with profile selection
}

func runHosts(args []string) {
	fmt.Println("Hosts command - implementation coming soon")
	// TODO: Implement host management commands
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
	defer database.Close()

	profileManager := profiles.NewManager(database)
	ctx := context.Background()

	switch args[0] {
	case "list":
		profiles, err := profileManager.GetAll(ctx)
		if err != nil {
			log.Fatalf("Failed to get profiles: %v", err)
		}

		fmt.Printf("Available Scan Profiles:\n\n")
		for _, profile := range profiles {
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

	case "show":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: scanorama profiles show <profile-id>\n")
			os.Exit(1)
		}

		profile, err := profileManager.GetByID(ctx, args[1])
		if err != nil {
			log.Fatalf("Failed to get profile: %v", err)
		}

		fmt.Printf("Profile: %s\n", profile.Name)
		fmt.Printf("ID: %s\n", profile.ID)
		fmt.Printf("Description: %s\n", profile.Description)
		fmt.Printf("OS Family: %v\n", profile.OSFamily)
		fmt.Printf("OS Patterns: %v\n", profile.OSPattern)
		fmt.Printf("Ports: %s\n", profile.Ports)
		fmt.Printf("Scan Type: %s\n", profile.ScanType)
		fmt.Printf("Timing: %s\n", profile.Timing)
		fmt.Printf("Scripts: %v\n", profile.Scripts)
		fmt.Printf("Priority: %d\n", profile.Priority)
		fmt.Printf("Built-in: %v\n", profile.BuiltIn)

	default:
		fmt.Fprintf(os.Stderr, "Unknown profiles command: %s\n", args[0])
		os.Exit(1)
	}
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
	defer database.Close()

	discoveryEngine := discovery.NewEngine(database)
	profileManager := profiles.NewManager(database)
	sched := scheduler.NewScheduler(database, discoveryEngine, profileManager)

	ctx := context.Background()

	switch args[0] {
	case "list":
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

	case "add-discovery":
		if len(args) < 4 {
			fmt.Fprintf(os.Stderr, "Usage: scanorama schedule add-discovery <name> <cron> <network> [--detect-os]\n")
			fmt.Fprintf(os.Stderr, "Example: scanorama schedule add-discovery \"weekly-sweep\" \"0 2 * * 0\" \"10.0.0.0/8\" --detect-os\n")
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

		config := scheduler.DiscoveryJobConfig{
			Network:     network,
			Method:      "ping",
			DetectOS:    detectOS,
			Timeout:     3,
			Concurrency: 50,
		}

		if err := sched.AddDiscoveryJob(ctx, name, cronExpr, config); err != nil {
			log.Fatalf("Failed to add discovery job: %v", err)
		}

		fmt.Printf("Discovery job '%s' scheduled successfully\n", name)

	case "add-scan":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: scanorama schedule add-scan <name> <cron> [--live-hosts] [--profile <id>]\n")
			fmt.Fprintf(os.Stderr, "Example: scanorama schedule add-scan \"daily-scan\" \"0 */6 * * *\" --live-hosts --profile auto\n")
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

		config := scheduler.ScanJobConfig{
			LiveHostsOnly: liveHostsOnly,
			ProfileID:     profileID,
			MaxAge:        24,
		}

		if err := sched.AddScanJob(ctx, name, cronExpr, config); err != nil {
			log.Fatalf("Failed to add scan job: %v", err)
		}

		fmt.Printf("Scan job '%s' scheduled successfully\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown schedule command: %s\n", args[0])
		os.Exit(1)
	}
}

func runDaemon(args []string) {
	fmt.Println("Starting Scanorama daemon...")

	// Setup database
	database, err := setupDatabase()
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer database.Close()

	// Create components
	discoveryEngine := discovery.NewEngine(database)
	profileManager := profiles.NewManager(database)
	sched := scheduler.NewScheduler(database, discoveryEngine, profileManager)

	// Start scheduler
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}
	defer sched.Stop()

	fmt.Println("Daemon started. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down daemon...")
}
