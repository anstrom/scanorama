// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements the smart-scan subcommand group for adaptive scanning operations.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// Smart-scan endpoint paths.
const (
	smartScanSuggestionsPath  = "/smart-scan/suggestions"
	smartScanProfileRecsPath  = "/smart-scan/profile-recommendations"
	smartScanHostsBasePath    = "/smart-scan/hosts/"
	smartScanTriggerBatchPath = "/smart-scan/trigger-batch"
	smartScanStageSuffix      = "/stage"
	smartScanTriggerSuffix    = "/trigger"
)

// SuggestionGroup describes a category of smart-scan suggestions.
type SuggestionGroup struct {
	Count       int    `json:"count"`
	Description string `json:"description"`
	Action      string `json:"action"`
}

// SuggestionSummary is the response shape for GET /smart-scan/suggestions.
type SuggestionSummary struct {
	NoOSInfo    SuggestionGroup `json:"no_os_info"`
	NoPorts     SuggestionGroup `json:"no_ports"`
	NoServices  SuggestionGroup `json:"no_services"`
	Stale       SuggestionGroup `json:"stale"`
	WellKnown   SuggestionGroup `json:"well_known"`
	TotalHosts  int             `json:"total_hosts"`
	GeneratedAt string          `json:"generated_at"`
}

// ProfileRecommendation is one element of the GET /smart-scan/profile-recommendations array.
type ProfileRecommendation struct {
	OSFamily    string `json:"os_family"`
	HostCount   int    `json:"host_count"`
	ProfileID   string `json:"profile_id"`
	ProfileName string `json:"profile_name"`
	Action      string `json:"action"`
}

// ScanStage is the response shape for GET /smart-scan/hosts/{id}/stage.
type ScanStage struct {
	Stage       string  `json:"stage"`
	ScanType    string  `json:"scan_type"`
	Ports       string  `json:"ports"`
	OSDetection bool    `json:"os_detection"`
	ProfileID   *string `json:"profile_id,omitempty"`
	Reason      string  `json:"reason"`
}

var (
	smartScanOutput       string
	smartScanTriggerStage string
	smartScanTriggerLimit int
)

// smartScanCmd is the root of the smart-scan command group.
var smartScanCmd = &cobra.Command{
	Use:   "smart-scan",
	Short: "Adaptive smart-scan operations",
	Long: `Interact with the Scanorama smart-scan subsystem.

Smart-scan analyses the current state of discovered hosts and recommends
the most appropriate next scan action for each host based on what data is
already present.

Examples:
  # View suggestions for which hosts need additional scanning
  scanorama smart-scan suggestions

  # View profile recommendations grouped by OS family
  scanorama smart-scan profile-recommendations

  # Show the recommended scan stage for a specific host
  scanorama smart-scan stage <host-id>

  # Trigger the next recommended scan for a host
  scanorama smart-scan trigger <host-id>

  # Batch-trigger scans for eligible hosts
  scanorama smart-scan trigger-batch --stage initial --limit 50`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// smartScanSuggestionsCmd handles smart-scan suggestions.
var smartScanSuggestionsCmd = &cobra.Command{
	Use:   "suggestions",
	Short: "Show smart-scan suggestions",
	Long:  `Retrieve a summary of hosts that would benefit from additional scanning.`,
	Run:   runSmartScanSuggestions,
}

// smartScanProfileRecsCmd handles smart-scan profile-recommendations.
var smartScanProfileRecsCmd = &cobra.Command{
	Use:   "profile-recommendations",
	Short: "Show per-OS profile recommendations",
	Long:  `Retrieve profile recommendations for each OS family present in the inventory.`,
	Run:   runSmartScanProfileRecs,
}

// smartScanStageCmd handles smart-scan stage <host-id>.
var smartScanStageCmd = &cobra.Command{
	Use:   "stage <host-id>",
	Short: "Show the recommended scan stage for a host",
	Long:  `Retrieve the recommended next scan stage for the specified host UUID.`,
	Args:  cobra.ExactArgs(1),
	Run:   runSmartScanStage,
}

// smartScanTriggerCmd handles smart-scan trigger <host-id>.
var smartScanTriggerCmd = &cobra.Command{
	Use:   "trigger <host-id>",
	Short: "Trigger the next recommended scan for a host",
	Long:  `Enqueue the next recommended scan for the specified host UUID.`,
	Args:  cobra.ExactArgs(1),
	Run:   runSmartScanTrigger,
}

// smartScanTriggerBatchCmd handles smart-scan trigger-batch.
var smartScanTriggerBatchCmd = &cobra.Command{
	Use:   "trigger-batch",
	Short: "Batch-trigger smart scans for eligible hosts",
	Long: `Enqueue smart scans for multiple eligible hosts in one call.

Use --stage to restrict the batch to a specific scan stage.
Use --limit to cap the number of hosts queued (0 = service default).`,
	Run: runSmartScanTriggerBatch,
}

func init() {
	rootCmd.AddCommand(smartScanCmd)

	smartScanCmd.AddCommand(smartScanSuggestionsCmd)
	smartScanCmd.AddCommand(smartScanProfileRecsCmd)
	smartScanCmd.AddCommand(smartScanStageCmd)
	smartScanCmd.AddCommand(smartScanTriggerCmd)
	smartScanCmd.AddCommand(smartScanTriggerBatchCmd)

	smartScanSuggestionsCmd.Flags().StringVarP(
		&smartScanOutput, "output", "o", "table", "Output format (table, json)")
	smartScanProfileRecsCmd.Flags().StringVarP(
		&smartScanOutput, "output", "o", "table", "Output format (table, json)")
	smartScanStageCmd.Flags().StringVarP(
		&smartScanOutput, "output", "o", "table", "Output format (table, json)")

	smartScanTriggerBatchCmd.Flags().StringVar(
		&smartScanTriggerStage, "stage", "", "Restrict batch to this scan stage (optional)")
	smartScanTriggerBatchCmd.Flags().IntVar(
		&smartScanTriggerLimit, "limit", 0, "Maximum hosts to queue (0 = service default)")
	smartScanTriggerBatchCmd.Flags().StringVarP(
		&smartScanOutput, "output", "o", "table", "Output format (table, json)")
}

// --- run functions ---

func runSmartScanSuggestions(_ *cobra.Command, _ []string) {
	if err := WithAPIClient("smart-scan suggestions", func(client *APIClient) error {
		resp, err := client.Get(smartScanSuggestionsPath)
		if err != nil {
			return err
		}
		summary, err := unmarshalAs[SuggestionSummary](resp.Data)
		if err != nil {
			return fmt.Errorf("unexpected response format: %w", err)
		}
		if smartScanOutput == outputFormatJSON {
			displaySuggestionsJSON(summary)
		} else {
			displaySuggestionsTable(summary)
		}
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

func runSmartScanProfileRecs(_ *cobra.Command, _ []string) {
	if err := WithAPIClient("smart-scan profile-recommendations", func(client *APIClient) error {
		resp, err := client.Get(smartScanProfileRecsPath)
		if err != nil {
			return err
		}
		recs, err := unmarshalAs[[]ProfileRecommendation](resp.Data)
		if err != nil {
			return fmt.Errorf("unexpected response format: %w", err)
		}
		if smartScanOutput == outputFormatJSON {
			displayProfileRecommendationsJSON(recs)
		} else {
			displayProfileRecommendationsTable(recs)
		}
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

func runSmartScanStage(_ *cobra.Command, args []string) {
	hostID := args[0]
	endpoint := smartScanHostsBasePath + hostID + smartScanStageSuffix
	if err := WithAPIClient("smart-scan stage", func(client *APIClient) error {
		resp, err := client.Get(endpoint)
		if err != nil {
			return err
		}
		stage, err := unmarshalAs[ScanStage](resp.Data)
		if err != nil {
			return fmt.Errorf("unexpected response format: %w", err)
		}
		if smartScanOutput == outputFormatJSON {
			displayScanStageJSON(stage)
		} else {
			displayScanStageTable(stage)
		}
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

func runSmartScanTrigger(_ *cobra.Command, args []string) {
	hostID := args[0]
	endpoint := smartScanHostsBasePath + hostID + smartScanTriggerSuffix
	if err := WithAPIClient("smart-scan trigger", func(client *APIClient) error {
		resp, err := client.Post(endpoint, nil)
		if err != nil {
			return err
		}
		return printTriggerResult(resp.Data)
	}); err != nil {
		os.Exit(1)
	}
}

// printTriggerResult handles the two valid outcomes of POST /smart-scan/hosts/{id}/trigger:
//   - queued=true  (202): print the scan_id
//   - queued=false (200): host is already up-to-date; print the message field
func printTriggerResult(data interface{}) error {
	m, err := unmarshalAs[map[string]interface{}](data)
	if err != nil {
		return fmt.Errorf("unexpected response format: %w", err)
	}

	queued, _ := m["queued"].(bool)
	if !queued {
		msg, _ := m["message"].(string)
		if msg == "" {
			msg = "no scan queued (host knowledge is already sufficient)"
		}
		fmt.Println(msg)
		return nil
	}

	scanID, ok := m["scan_id"].(string)
	if !ok || scanID == "" {
		return fmt.Errorf("queued=true but scan_id missing in response")
	}
	fmt.Println(scanID)
	return nil
}

func runSmartScanTriggerBatch(_ *cobra.Command, _ []string) {
	payload := map[string]interface{}{
		"stage": smartScanTriggerStage,
		"limit": smartScanTriggerLimit,
	}
	if err := WithAPIClient("smart-scan trigger-batch", func(client *APIClient) error {
		resp, err := client.Post(smartScanTriggerBatchPath, payload)
		if err != nil {
			return err
		}
		result, err := unmarshalAs[map[string]interface{}](resp.Data)
		if err != nil {
			return fmt.Errorf("unexpected response format: %w", err)
		}
		if smartScanOutput == outputFormatJSON {
			displayTriggerBatchJSON(result)
		} else {
			displayTriggerBatchTable(result)
		}
		return nil
	}); err != nil {
		os.Exit(1)
	}
}

// --- display functions ---

func displaySuggestionsJSON(s SuggestionSummary) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func displaySuggestionsTable(s SuggestionSummary) {
	fmt.Printf("Smart-scan suggestions (total hosts: %d, generated: %s)\n\n",
		s.TotalHosts, s.GeneratedAt)

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("Category", "Count", "Description", "Action")

	rows := [][]string{
		{"no_os_info", fmt.Sprintf("%d", s.NoOSInfo.Count), s.NoOSInfo.Description, s.NoOSInfo.Action},
		{"no_ports", fmt.Sprintf("%d", s.NoPorts.Count), s.NoPorts.Description, s.NoPorts.Action},
		{"no_services", fmt.Sprintf("%d", s.NoServices.Count), s.NoServices.Description, s.NoServices.Action},
		{"stale", fmt.Sprintf("%d", s.Stale.Count), s.Stale.Description, s.Stale.Action},
		{"well_known", fmt.Sprintf("%d", s.WellKnown.Count), s.WellKnown.Description, s.WellKnown.Action},
	}
	for _, row := range rows {
		_ = table.Append(row)
	}
	_ = table.Render()
}

func displayProfileRecommendationsJSON(recs []ProfileRecommendation) {
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func displayProfileRecommendationsTable(recs []ProfileRecommendation) {
	if len(recs) == 0 {
		fmt.Println("No profile recommendations available.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("OS Family", "Host Count", "Profile ID", "Profile Name", "Action")

	for i := range recs {
		r := &recs[i]
		_ = table.Append([]string{
			r.OSFamily,
			fmt.Sprintf("%d", r.HostCount),
			r.ProfileID,
			r.ProfileName,
			r.Action,
		})
	}
	_ = table.Render()
}

func displayScanStageJSON(s ScanStage) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func displayScanStageTable(s ScanStage) {
	fmt.Printf("Stage:        %s\n", s.Stage)
	fmt.Printf("Scan type:    %s\n", s.ScanType)
	fmt.Printf("Ports:        %s\n", s.Ports)
	fmt.Printf("OS detection: %t\n", s.OSDetection)
	if s.ProfileID != nil {
		fmt.Printf("Profile ID:   %s\n", *s.ProfileID)
	}
	fmt.Printf("Reason:       %s\n", s.Reason)
}

func displayTriggerBatchJSON(result map[string]interface{}) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func displayTriggerBatchTable(result map[string]interface{}) {
	fmt.Printf("Queued:  %v\n", result["queued"])
	fmt.Printf("Skipped: %v\n", result["skipped"])
}

// --- helpers ---

// unmarshalAs round-trips v (the raw interface{} from APIResponse.Data) through
// JSON encoding to produce a strongly-typed value of T.
func unmarshalAs[T any](v interface{}) (T, error) {
	var zero T
	raw, err := json.Marshal(v)
	if err != nil {
		return zero, fmt.Errorf("marshal: %w", err)
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("unmarshal: %w", err)
	}
	return result, nil
}
