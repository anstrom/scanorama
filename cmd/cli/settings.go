// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements settings management commands for reading and updating
// server configuration via the admin API.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

const (
	settingsMaxValueLen = 40
)

var (
	settingsKey    string
	settingsValue  string
	settingsOutput string
)

// Setting represents a single server configuration entry returned by the API.
type Setting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Type        string `json:"type"`
	UpdatedAt   string `json:"updated_at"`
}

// settingsCmd is the parent command for all settings subcommands.
var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage server configuration settings",
	Long: `View and update server configuration settings via the admin API.

Settings are key-value pairs that control server behavior at runtime.
Values must be valid JSON — use quoted strings for text values.

Examples:
  # List all settings as a table
  scanorama settings get

  # List all settings as JSON
  scanorama settings get --output json

  # Update a boolean setting
  scanorama settings update --key some.feature.enabled --value true

  # Update a numeric setting
  scanorama settings update --key scan.timeout --value 30

  # Update a string setting (value must be a JSON string literal)
  scanorama settings update --key log.level --value '"debug"'`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// settingsGetCmd retrieves all server settings.
var settingsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get all server settings",
	Long: `Retrieve all server configuration settings from the admin API.

Use --output json to get machine-readable output.

Examples:
  scanorama settings get
  scanorama settings get --output json`,
	Run: runSettingsGet,
}

// settingsUpdateCmd updates a single server setting.
var settingsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a server setting",
	Long: `Update a single server configuration setting via the admin API.

The --value flag must be a valid JSON value:
  - Boolean:  --value true
  - Number:   --value 42
  - String:   --value '"text"'   (note the inner quotes)

Examples:
  scanorama settings update --key scan.timeout --value 30
  scanorama settings update --key log.level --value '"debug"'
  scanorama settings update --key feature.enabled --value true`,
	Run: runSettingsUpdate,
}

func runSettingsGet(cmd *cobra.Command, args []string) {
	err := WithAPIClient("get settings", executeSettingsGet)
	if err != nil {
		os.Exit(1)
	}
}

func runSettingsUpdate(cmd *cobra.Command, args []string) {
	err := WithAPIClient("update setting", executeSettingsUpdate)
	if err != nil {
		os.Exit(1)
	}
}

func executeSettingsGet(client *APIClient) error {
	resp, err := client.Get("/admin/settings")
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}

	settings, err := parseSettingsResponse(resp.Data)
	if err != nil {
		return fmt.Errorf("failed to parse settings response: %w", err)
	}

	if settingsOutput == outputFormatJSON {
		displaySettingsJSON(settings)
	} else {
		displaySettingsTable(settings)
	}

	return nil
}

func executeSettingsUpdate(client *APIClient) error {
	payload := map[string]string{
		"key":   settingsKey,
		"value": settingsValue,
	}

	resp, err := client.Put("/admin/settings", payload)
	if err != nil {
		return fmt.Errorf("failed to update setting: %w", err)
	}

	result, err := parseUpdateResponse(resp.Data)
	if err != nil {
		return fmt.Errorf("failed to parse update response: %w", err)
	}

	if updated, ok := result["updated"].(bool); ok && updated {
		fmt.Printf("Setting %q updated successfully.\n", settingsKey)
	} else {
		fmt.Printf("Setting %q update response received (key: %v).\n",
			settingsKey, result["key"])
	}

	return nil
}

// parseSettingsResponse re-marshals the untyped API response data into a
// typed []Setting slice via JSON round-trip.
func parseSettingsResponse(data interface{}) ([]Setting, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var wrapper struct {
		Settings []Setting `json:"settings"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if wrapper.Settings == nil {
		return make([]Setting, 0), nil
	}

	return wrapper.Settings, nil
}

// parseUpdateResponse re-marshals the untyped API response data into a
// map[string]interface{} for flexible field access.
func parseUpdateResponse(data interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return result, nil
}

// truncateValue shortens a string to at most maxLen characters, appending "..."
// if truncation occurred.
func truncateValue(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// displaySettingsTable renders settings as an ASCII table to stdout.
func displaySettingsTable(settings []Setting) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("Key", "Type", "Value", "Description", "Updated")

	for i := range settings {
		s := &settings[i]
		_ = table.Append([]string{
			s.Key,
			s.Type,
			truncateValue(s.Value, settingsMaxValueLen),
			s.Description,
			s.UpdatedAt,
		})
	}

	_ = table.Render()
}

// displaySettingsJSON renders settings as indented JSON to stdout.
func displaySettingsJSON(settings []Setting) {
	output := struct {
		Settings []Setting `json:"settings"`
		Count    int       `json:"count"`
	}{
		Settings: settings,
		Count:    len(settings),
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

func init() {
	rootCmd.AddCommand(settingsCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsUpdateCmd)

	// get flags
	settingsGetCmd.Flags().StringVarP(&settingsOutput, "output", "o", "table",
		"Output format (table, json)")

	// update flags
	settingsUpdateCmd.Flags().StringVar(&settingsKey, "key", "",
		"Setting key to update (required)")
	settingsUpdateCmd.Flags().StringVar(&settingsValue, "value", "",
		"New value as valid JSON (required) — e.g. true, 42, or '\"text\"'")
	_ = settingsUpdateCmd.MarkFlagRequired("key")
	_ = settingsUpdateCmd.MarkFlagRequired("value")
}
