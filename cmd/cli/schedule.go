// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/spf13/cobra"
)

const (
	// Schedule management constants.
	scheduleArgsCount       = 3   // required args for add-discovery command
	scheduleScanArgsCount   = 5   // required args for add-scan command
	scheduleDefaultTimeout  = 300 // default timeout in seconds
	scheduleSeparatorLength = 85  // characters for schedule list separator
	scheduleDetailSeparator = 50  // characters for schedule detail separator
	maxJobNameLength        = 20  // max job name length before truncation
	maxPortsDisplay         = 5   // max ports to show before truncation
)

// scheduleCmd represents the schedule command.
var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage scheduled jobs",
	Long: `Manage scheduled discovery and scan jobs using cron expressions.
You can add, list, and remove scheduled jobs that will run automatically
at specified intervals.`,
	Example: `  scanorama schedule list
  scanorama schedule add-discovery "weekly-sweep" "0 2 * * 0" "10.0.0.0/8"
  scanorama schedule add-scan "daily-live" "0 */6 * * *" --live-hosts
  scanorama schedule remove "weekly-sweep"`,
}

// scheduleListCmd represents the schedule list command.
var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scheduled jobs",
	Long: `Display all currently scheduled jobs with their schedules,
next run times, and job details.`,
	Run: runScheduleList,
}

// scheduleAddDiscoveryCmd represents the schedule add-discovery command.
var scheduleAddDiscoveryCmd = &cobra.Command{
	Use:   "add-discovery [name] [cron] [network]",
	Short: "Add a scheduled discovery job",
	Long: `Add a new scheduled discovery job that will run at specified intervals.
The cron expression follows standard cron format (minute hour day month weekday).`,
	Example: `  scanorama schedule add-discovery "weekly-sweep" "0 2 * * 0" "10.0.0.0/8"
  scanorama schedule add-discovery "daily-local" "0 1 * * *" "192.168.0.0/16"
  scanorama schedule add-discovery "hourly-critical" "0 * * * *" "10.0.1.0/24"`,
	Args: cobra.ExactArgs(scheduleArgsCount),
	Run:  runScheduleAddDiscovery,
}

// scheduleAddScanCmd represents the schedule add-scan command.
var scheduleAddScanCmd = &cobra.Command{
	Use:   "add-scan [name] [cron]",
	Short: "Add a scheduled scan job",
	Long: `Add a new scheduled scan job that will run at specified intervals.
You can schedule scans for live hosts or specific targets.`,
	Example: `  scanorama schedule add-scan "daily-live" "0 */6 * * *" --live-hosts
  scanorama schedule add-scan "weekly-servers" "0 3 * * 1" --targets "10.0.1.1,10.0.1.2"
  scanorama schedule add-scan "nightly-comprehensive" "0 22 * * *" --live-hosts --type comprehensive`,
	Args: cobra.ExactArgs(2),
	Run:  runScheduleAddScan,
}

// scheduleRemoveCmd represents the schedule remove command.
var scheduleRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a scheduled job",
	Long: `Remove a scheduled job by name. The job will no longer
run automatically after removal.`,
	Example: `  scanorama schedule remove "weekly-sweep"
  scanorama schedule remove "daily-live"`,
	Args: cobra.ExactArgs(1),
	Run:  runScheduleRemove,
}

// scheduleShowCmd represents the schedule show command.
var scheduleShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show details of a scheduled job",
	Long: `Display detailed information about a specific scheduled job
including its configuration, schedule, and execution history.`,
	Example: `  scanorama schedule show "weekly-sweep"
  scanorama schedule show "daily-live"`,
	Args: cobra.ExactArgs(1),
	Run:  runScheduleShow,
}

var (
	// Flags for add-scan command.
	scheduleTargets   string
	scheduleLiveHosts bool
	schedulePorts     string
	scheduleScanType  string
	scheduleProfile   string
	scheduleTimeout   int
	scheduleOSFamily  string

	// Flags for add-discovery command.
	scheduleDetectOS bool
	scheduleMethod   string
)

func init() {
	rootCmd.AddCommand(scheduleCmd)
	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleAddDiscoveryCmd)
	scheduleCmd.AddCommand(scheduleAddScanCmd)
	scheduleCmd.AddCommand(scheduleRemoveCmd)
	scheduleCmd.AddCommand(scheduleShowCmd)

	// Flags for add-scan command
	scheduleAddScanCmd.Flags().StringVar(&scheduleTargets, "targets", "", "Comma-separated list of targets to scan")
	scheduleAddScanCmd.Flags().BoolVar(&scheduleLiveHosts, "live-hosts", false, "Scan only discovered live hosts")
	scheduleAddScanCmd.Flags().StringVar(&schedulePorts, "ports", "22,80,443,8080,8443", "Ports to scan")
	scheduleAddScanCmd.Flags().StringVar(&scheduleScanType, "type", "connect", "Scan type")
	scheduleAddScanCmd.Flags().StringVar(&scheduleProfile, "profile", "", "Scan profile to use")
	scheduleAddScanCmd.Flags().IntVar(&scheduleTimeout, "timeout", scheduleDefaultTimeout, "Scan timeout in seconds")
	scheduleAddScanCmd.Flags().StringVar(&scheduleOSFamily, "os-family", "", "Scan only specific OS family")

	// Make targets and live-hosts mutually exclusive
	scheduleAddScanCmd.MarkFlagsMutuallyExclusive("targets", "live-hosts")

	// Flags for add-discovery command
	scheduleAddDiscoveryCmd.Flags().BoolVar(&scheduleDetectOS, "detect-os", false, "Enable OS detection")
	scheduleAddDiscoveryCmd.Flags().StringVar(&scheduleMethod, "method", "tcp", "Discovery method")

	// Add detailed descriptions
	scheduleAddScanCmd.Flags().Lookup("targets").Usage = "Specific targets to scan in scheduled job"
	scheduleAddScanCmd.Flags().Lookup("live-hosts").Usage = "Scan all discovered live hosts in scheduled job"
	scheduleAddScanCmd.Flags().Lookup("type").Usage = "Scan type: connect, syn, version, " +
		"comprehensive, intense, stealth"
}

func runScheduleList(cmd *cobra.Command, args []string) {
	withDatabaseOrExit(func(database *db.DB) {
		// Query scheduled jobs
		jobs, err := queryScheduledJobs(database)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying scheduled jobs: %v\n", err)
			os.Exit(1)
		}

		displayScheduledJobs(jobs)
	})
}

func runScheduleAddDiscovery(cmd *cobra.Command, args []string) {
	name := args[0]
	cronExpr := args[1]
	network := args[2]

	// Validate cron expression
	if err := validateCronExpression(cronExpr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid cron expression '%s': %v\n", cronExpr, err)
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Create discovery job
		job := DiscoveryJob{
			Name:     name,
			CronExpr: cronExpr,
			Network:  network,
			Method:   scheduleMethod,
			DetectOS: scheduleDetectOS,
		}

		if verbose {
			fmt.Printf("Creating discovery job: %+v\n", job)
		}

		err := createScheduledDiscoveryJob(database, job)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating scheduled discovery job: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully created scheduled discovery job '%s'\n", name)
		fmt.Printf("Schedule: %s (next run: %s)\n", cronExpr, getNextRunTime(cronExpr))
		fmt.Printf("Network: %s\n", network)
		fmt.Printf("Method: %s\n", scheduleMethod)
		if scheduleDetectOS {
			fmt.Printf("OS Detection: enabled\n")
		}
	})
}

func runScheduleAddScan(cmd *cobra.Command, args []string) {
	name := args[0]
	cronExpr := args[1]

	// Validate cron expression
	if err := validateCronExpression(cronExpr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid cron expression '%s': %v\n", cronExpr, err)
		os.Exit(1)
	}

	// Validate that either targets or live-hosts is specified
	if !scheduleLiveHosts && scheduleTargets == "" {
		fmt.Fprintf(os.Stderr, "Error: either --targets or --live-hosts must be specified\n")
		os.Exit(1)
	}

	withDatabaseOrExit(func(database *db.DB) {
		// Create scan job
		job := ScanJob{
			Name:      name,
			CronExpr:  cronExpr,
			Targets:   scheduleTargets,
			LiveHosts: scheduleLiveHosts,
			Ports:     schedulePorts,
			ScanType:  scheduleScanType,
			Profile:   scheduleProfile,
			Timeout:   scheduleTimeout,
			OSFamily:  scheduleOSFamily,
		}

		if verbose {
			fmt.Printf("Creating scan job: %+v\n", job)
		}

		err := createScheduledScanJob(database, &job)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating scheduled scan job: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully created scheduled scan job '%s'\n", name)
		fmt.Printf("Schedule: %s (next run: %s)\n", cronExpr, getNextRunTime(cronExpr))
		if scheduleLiveHosts {
			fmt.Println("Target: all live hosts")
		} else {
			fmt.Printf("Targets: %s\n", scheduleTargets)
		}
		fmt.Printf("Scan type: %s\n", scheduleScanType)
		fmt.Printf("Ports: %s\n", schedulePorts)
	})
}

func runScheduleRemove(cmd *cobra.Command, args []string) {
	name := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		// Remove scheduled job
		err := removeScheduledJob(database, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing scheduled job '%s': %v\n", name, err)
			os.Exit(1)
		}

		fmt.Printf("Successfully removed scheduled job '%s'\n", name)
	})
}

func runScheduleShow(cmd *cobra.Command, args []string) {
	name := args[0]

	withDatabaseOrExit(func(database *db.DB) {
		// Query job details
		job, err := getScheduledJob(database, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting scheduled job: %v\n", err)
			os.Exit(1)
		}

		displayJobDetails(job)
	})
}

// ScheduledJob represents a scheduled job.
type ScheduledJob struct {
	ID        string
	Name      string
	JobType   string
	CronExpr  string
	IsActive  bool
	CreatedAt time.Time
	LastRun   *time.Time
	NextRun   time.Time
	RunCount  int
	Config    map[string]interface{}
}

// DiscoveryJob represents a discovery job configuration.
type DiscoveryJob struct {
	Name     string
	CronExpr string
	Network  string
	Method   string
	DetectOS bool
}

// ScanJob represents a scan job configuration.
type ScanJob struct {
	Name      string
	CronExpr  string
	Targets   string
	LiveHosts bool
	Ports     string
	ScanType  string
	Profile   string
	Timeout   int
	OSFamily  string
}

func queryScheduledJobs(database *db.DB) ([]ScheduledJob, error) {
	query := `
		SELECT
			id, name, job_type, cron_expression, is_active,
			created_at, last_run, run_count, configuration
		FROM scheduled_jobs
		ORDER BY created_at DESC`

	rows, err := database.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close rows: %v\n", closeErr)
		}
	}()

	var jobs []ScheduledJob
	for rows.Next() {
		var job ScheduledJob
		var configJSON string

		err := rows.Scan(
			&job.ID,
			&job.Name,
			&job.JobType,
			&job.CronExpr,
			&job.IsActive,
			&job.CreatedAt,
			&job.LastRun,
			&job.RunCount,
			&configJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		// Calculate next run time
		job.NextRun = getNextRunTime(job.CronExpr)

		jobs = append(jobs, job)
	}

	return jobs, nil
}

func displayScheduledJobs(jobs []ScheduledJob) {
	if len(jobs) == 0 {
		fmt.Println("No scheduled jobs found")
		return
	}

	fmt.Printf("Found %d scheduled job(s):\n\n", len(jobs))
	fmt.Printf("%-20s %-12s %-15s %-8s %-20s %-5s\n",
		"Name", "Type", "Schedule", "Active", "Next Run", "Runs")
	fmt.Println(strings.Repeat("-", scheduleSeparatorLength))

	for i := range jobs {
		job := &jobs[i]
		activeStr := "No"
		if job.IsActive {
			activeStr = "Yes"
		}

		nextRunStr := job.NextRun.Format("2006-01-02 15:04")

		fmt.Printf("%-20s %-12s %-15s %-8s %-20s %-5d\n",
			truncateString(job.Name, maxJobNameLength),
			job.JobType,
			job.CronExpr,
			activeStr,
			nextRunStr,
			job.RunCount)
	}
}

func createScheduledDiscoveryJob(database *db.DB, job DiscoveryJob) error {
	query := `
		INSERT INTO scheduled_jobs (name, job_type, cron_expression, configuration, is_active)
		VALUES ($1, 'discovery', $2, $3, true)`

	configJSON := fmt.Sprintf(`{"network":%q,"method":%q,"detect_os":%t}`,
		job.Network, job.Method, job.DetectOS)

	_, err := database.Exec(query, job.Name, job.CronExpr, configJSON)
	if err != nil {
		return fmt.Errorf("failed to create scheduled job: %w", err)
	}

	return nil
}

func createScheduledScanJob(database *db.DB, job *ScanJob) error {
	query := `
		INSERT INTO scheduled_jobs (name, job_type, cron_expression, configuration, is_active)
		VALUES ($1, 'scan', $2, $3, true)`

	configJSON := fmt.Sprintf(
		`{"targets":%q,"live_hosts":%t,"ports":%q,"scan_type":%q,"profile":%q,"timeout":%d,"os_family":%q}`,
		job.Targets, job.LiveHosts, job.Ports, job.ScanType, job.Profile, job.Timeout, job.OSFamily)

	_, err := database.Exec(query, job.Name, job.CronExpr, configJSON)
	if err != nil {
		return fmt.Errorf("failed to create scheduled job: %w", err)
	}

	return nil
}

func removeScheduledJob(database *db.DB, name string) error {
	query := `DELETE FROM scheduled_jobs WHERE name = $1`
	result, err := database.Exec(query, name)
	if err != nil {
		return fmt.Errorf("failed to remove job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("could not get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("job '%s' not found", name)
	}

	return nil
}

func getScheduledJob(database *db.DB, name string) (*ScheduledJob, error) {
	query := `
		SELECT
			id, name, job_type, cron_expression, is_active,
			created_at, last_run, run_count, configuration
		FROM scheduled_jobs
		WHERE name = $1`

	var job ScheduledJob
	var configJSON string

	err := database.QueryRow(query, name).Scan(
		&job.ID,
		&job.Name,
		&job.JobType,
		&job.CronExpr,
		&job.IsActive,
		&job.CreatedAt,
		&job.LastRun,
		&job.RunCount,
		&configJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("job '%s' not found: %w", name, err)
	}

	job.NextRun = getNextRunTime(job.CronExpr)

	return &job, nil
}

func displayJobDetails(job *ScheduledJob) {
	fmt.Printf("Scheduled Job Details: %s\n", job.Name)
	fmt.Println(strings.Repeat("=", scheduleDetailSeparator))
	fmt.Printf("ID: %s\n", job.ID)
	fmt.Printf("Type: %s\n", job.JobType)
	fmt.Printf("Schedule: %s\n", job.CronExpr)
	fmt.Printf("Active: %t\n", job.IsActive)
	fmt.Printf("Created: %s\n", job.CreatedAt.Format("2006-01-02 15:04:05"))
	if job.LastRun != nil {
		fmt.Printf("Last Run: %s\n", job.LastRun.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Println("Last Run: Never")
	}
	fmt.Printf("Next Run: %s\n", job.NextRun.Format("2006-01-02 15:04:05"))
	fmt.Printf("Run Count: %d\n", job.RunCount)
}

func validateCronExpression(cronExpr string) error {
	parts := strings.Fields(cronExpr)
	if len(parts) != scheduleScanArgsCount {
		return fmt.Errorf("cron expression must have 5 fields (minute hour day month weekday)")
	}
	return nil
}

func getNextRunTime(_ string) time.Time {
	// Simple implementation - in real code you'd use the cron library
	// to calculate the actual next run time
	return time.Now().Add(time.Hour) // Placeholder
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
