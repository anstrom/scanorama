// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements API key management commands for creating, listing, updating,
// and deleting API keys used for API server authentication.
package cli

// Trigger diagnostics refresh
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/auth"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

var (
	// API key command flags
	apiKeyName         string
	apiKeyExpiresIn    string
	apiKeyNotes        string
	apiKeyShowExpired  bool
	apiKeyShowInactive bool
	apiKeyForce        bool
	apiKeyOutput       string
)

// apiKeysCmd represents the apikeys command group
var apiKeysCmd = &cobra.Command{
	Use:     "apikeys",
	Aliases: []string{"apikey", "keys", "key"},
	Short:   "Manage API keys for client authentication",
	Long: `Manage API keys for client authentication with the Scanorama API server.

API keys allow clients (CLI tools, dashboards, monitoring apps) to authenticate
with the API server. Keys are stored in the database with metadata like usage
tracking, expiration, and access controls.

To use CLI commands that call the API server, set the SCANORAMA_API_KEY
environment variable to one of the keys created here.

Examples:
  # Create an API key for a dashboard application
  scanorama apikeys create --name "Production Dashboard"

  # Create an API key for CLI usage
  scanorama apikeys create --name "CLI Access"
  export SCANORAMA_API_KEY=sk_abc123...

  # List all active API keys
  scanorama apikeys list

  # Create an API key that expires in 30 days
  scanorama apikeys create --name "Testing" --expires-in 30d

  # Show details of a specific API key
  scanorama apikeys show sk_live_abc123...

  # Revoke an API key
  scanorama apikeys revoke sk_live_abc123...

  # Update API key metadata
  scanorama apikeys update sk_live_abc123... --name "Updated Name" --notes "New description"`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if no subcommand is provided
		_ = cmd.Help()
	},
}

// apiKeysListCmd lists all API keys
var apiKeysListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List API keys",
	Long: `List all API keys with their metadata.

By default, only active (non-revoked, non-expired) keys are shown.
Use flags to include expired or inactive keys.

Examples:
  # List active API keys
  scanorama apikeys list

  # List all keys including expired and inactive
  scanorama apikeys list --show-expired --show-inactive

  # Output as JSON
  scanorama apikeys list --output json`,
	Run: runAPIKeysListCommand,
}

// apiKeysCreateCmd creates a new API key
var apiKeysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API key",
	Long: `Create a new API key for client authentication.

The key will be displayed only once during creation for security reasons.
Make sure to save it in a secure location. To use the key with CLI commands,
set it as an environment variable: export SCANORAMA_API_KEY=your_key_here

Examples:
  # Create an API key for an application
  scanorama apikeys create --name "Production System"

  # Create an API key for CLI usage
  scanorama apikeys create --name "CLI Access"
  # Then: export SCANORAMA_API_KEY=sk_abc123...

  # Create a key that expires in 30 days
  scanorama apikeys create --name "Testing" --expires-in 30d

  # Create a key with notes
  scanorama apikeys create --name "Admin Access" --notes "Emergency access key"`,
	Run: runAPIKeysCreateCommand,
}

// apiKeysShowCmd shows details of a specific API key
var apiKeysShowCmd = &cobra.Command{
	Use:     "show <key-id-or-prefix>",
	Aliases: []string{"get", "describe"},
	Short:   "Show API key details",
	Long: `Show detailed information about a specific API key.

You can specify the key by its ID or by its display prefix.

Examples:
  # Show key details by prefix
  scanorama apikeys show sk_live_abc123...

  # Show key details by full ID
  scanorama apikeys show 12345678-1234-1234-1234-123456789012`,
	Args: cobra.ExactArgs(1),
	Run:  runAPIKeysShowCommand,
}

// apiKeysUpdateCmd updates an API key's metadata
var apiKeysUpdateCmd = &cobra.Command{
	Use:     "update <key-id-or-prefix>",
	Aliases: []string{"edit", "modify"},
	Short:   "Update API key metadata",
	Long: `Update the metadata of an existing API key.

You can update the name, notes, and expiration time. The actual key value
cannot be changed - create a new key if you need a different key value.

Examples:
  # Update key name
  scanorama apikeys update sk_live_abc123... --name "New Name"

  # Update notes
  scanorama apikeys update sk_live_abc123... --notes "Updated description"

  # Set expiration (30 days from now)
  scanorama apikeys update sk_live_abc123... --expires-in 30d`,
	Args: cobra.ExactArgs(1),
	Run:  runAPIKeysUpdateCommand,
}

// apiKeysRevokeCmd revokes (deactivates) an API key
var apiKeysRevokeCmd = &cobra.Command{
	Use:     "revoke <key-id-or-prefix>",
	Aliases: []string{"delete", "remove", "disable"},
	Short:   "Revoke an API key",
	Long: `Revoke (deactivate) an API key, making it unusable for authentication.

Revoked keys are kept in the database for audit purposes but cannot be used
for authentication. This action cannot be undone.

Examples:
  # Revoke a key
  scanorama apikeys revoke sk_live_abc123...

  # Force revoke without confirmation
  scanorama apikeys revoke sk_live_abc123... --force`,
	Args: cobra.ExactArgs(1),
	Run:  runAPIKeysRevokeCommand,
}

// runAPIKeysListCommand handles the list subcommand
func runAPIKeysListCommand(cmd *cobra.Command, args []string) {
	if err := executeListAPIKeys(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAPIKeysCreateCommand handles the create subcommand
func runAPIKeysCreateCommand(cmd *cobra.Command, args []string) {
	if err := executeCreateAPIKey(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAPIKeysShowCommand handles the show subcommand
func runAPIKeysShowCommand(cmd *cobra.Command, args []string) {
	keyIdentifier := args[0]
	if err := executeShowAPIKey(keyIdentifier); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAPIKeysUpdateCommand handles the update subcommand
func runAPIKeysUpdateCommand(cmd *cobra.Command, args []string) {
	keyIdentifier := args[0]
	if err := executeUpdateAPIKey(keyIdentifier); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runAPIKeysRevokeCommand handles the revoke subcommand
func runAPIKeysRevokeCommand(cmd *cobra.Command, args []string) {
	keyIdentifier := args[0]
	if err := executeRevokeAPIKey(keyIdentifier); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Helper functions

// setupDatabaseConnection creates a database connection using the config
func setupDatabaseConnection() (*db.DB, error) {
	cfg, err := config.Load(getConfigFilePath())
	if err != nil {
		return nil, fmt.Errorf("error loading config: %v", err)
	}

	dbConfig := cfg.GetDatabaseConfig()
	database, err := db.Connect(context.Background(), &dbConfig)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	return database, nil
}

// parseExpirationDuration parses duration strings like "30d", "1h", "7d"
func parseExpirationDuration(durationStr string) (time.Time, error) {
	duration, err := parseDurationString(durationStr)
	if err != nil {
		return time.Time{}, err
	}

	return time.Now().UTC().Add(duration), nil
}

// parseDurationString parses duration strings with support for days
func parseDurationString(s string) (time.Duration, error) {
	// Handle days separately since time.ParseDuration doesn't support 'd'
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid days format: %s", daysStr)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Use standard duration parsing for other units
	return time.ParseDuration(s)
}

// displayAPIKeysTable displays API keys in a table format
func displayAPIKeysTable(keys []auth.APIKeyInfo) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("ID", "Name", "Prefix", "Status", "Created", "Last Used", "Expires")

	for i := range keys {
		key := &keys[i]

		// Determine status
		status := "Active"
		if !key.IsActive {
			status = "Revoked"
		} else if key.IsExpired() {
			status = "Expired"
		}

		// Format timestamps
		created := key.CreatedAt.Format("2006-01-02 15:04")
		lastUsed := "Never"
		if key.LastUsedAt != nil {
			lastUsed = key.LastUsedAt.Format("2006-01-02 15:04")
		}
		expires := "Never"
		if key.ExpiresAt != nil {
			expires = key.ExpiresAt.Format("2006-01-02 15:04")
		}

		// Format ID - truncate if longer than 8 characters
		displayID := key.ID
		if len(key.ID) > 8 {
			displayID = key.ID[:8] + "..."
		}

		_ = table.Append([]string{
			displayID,
			key.Name,
			key.KeyPrefix,
			status,
			created,
			lastUsed,
			expires,
		})
	}

	_ = table.Render()
}

// displayAPIKeysJSON displays API keys in JSON format
func displayAPIKeysJSON(keys []auth.APIKeyInfo) {
	output := struct {
		APIKeys []auth.APIKeyInfo `json:"api_keys"`
		Count   int               `json:"count"`
	}{
		APIKeys: keys,
		Count:   len(keys),
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

// queryAPIKeys retrieves API keys from database with filters
func queryAPIKeys(database *db.DB, showExpired, showInactive bool) ([]auth.APIKeyInfo, error) {
	repo := auth.NewAPIKeyRepository(database)
	return repo.ListAPIKeys(showExpired, showInactive)
}

func storeAPIKey(database *db.DB, generatedKey *auth.GeneratedAPIKey) (*auth.APIKeyInfo, error) {
	repo := auth.NewAPIKeyRepository(database)
	return repo.CreateAPIKey(generatedKey)
}

func findAPIKey(database *db.DB, identifier string) (*auth.APIKeyInfo, error) {
	repo := auth.NewAPIKeyRepository(database)
	return repo.FindAPIKeyByIdentifier(identifier)
}

func updateAPIKey(database *db.DB, keyID string, updates map[string]interface{}) (*auth.APIKeyInfo, error) {
	repo := auth.NewAPIKeyRepository(database)
	return repo.UpdateAPIKey(keyID, updates)
}

func revokeAPIKey(database *db.DB, keyID string) error {
	repo := auth.NewAPIKeyRepository(database)
	return repo.RevokeAPIKey(keyID)
}

// Helper functions that handle database operations without exitAfterDefer issues

func executeListAPIKeys() error {
	database, err := setupDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to close database connection: %v\n", err)
		}
	}()

	keys, err := queryAPIKeys(database, apiKeyShowExpired, apiKeyShowInactive)
	if err != nil {
		return fmt.Errorf("failed to query API keys: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys found.")
		return nil
	}

	if apiKeyOutput == "json" {
		displayAPIKeysJSON(keys)
	} else {
		displayAPIKeysTable(keys)
	}
	return nil
}

func executeCreateAPIKey() error {
	if apiKeyName == "" {
		return fmt.Errorf("API key name is required")
	}

	generatedKey, err := auth.GenerateAPIKey(apiKeyName)
	if err != nil {
		return fmt.Errorf("failed to generate API key: %w", err)
	}

	// Set additional metadata
	if apiKeyExpiresIn != "" {
		expiration, err := parseExpirationDuration(apiKeyExpiresIn)
		if err != nil {
			return fmt.Errorf("invalid expiration format '%s': %w", apiKeyExpiresIn, err)
		}
		generatedKey.KeyInfo.ExpiresAt = &expiration
	}
	generatedKey.KeyInfo.Notes = apiKeyNotes

	database, err := setupDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to close database connection: %v\n", err)
		}
	}()

	keyInfo, err := storeAPIKey(database, generatedKey)
	if err != nil {
		return fmt.Errorf("failed to store API key: %w", err)
	}

	fmt.Println("🔑 API Key Created Successfully")
	fmt.Println("═══════════════════════════════")
	fmt.Printf("Key ID: %s\n", keyInfo.ID)
	fmt.Printf("Name: %s\n", keyInfo.Name)
	fmt.Printf("Prefix: %s\n", keyInfo.KeyPrefix)
	fmt.Printf("Full Key: %s\n", generatedKey.Key)
	fmt.Printf("Created: %s\n", keyInfo.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	if keyInfo.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", keyInfo.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	} else {
		fmt.Println("Expires: Never")
	}

	fmt.Println()
	fmt.Println("⚠️  IMPORTANT: Save this key now - it will not be shown again!")
	fmt.Println()
	fmt.Println("To use this key with CLI commands:")
	fmt.Printf("  export SCANORAMA_API_KEY=%s\n", generatedKey.Key)
	fmt.Println()
	fmt.Println("Or add it to your shell profile for permanent use:")
	fmt.Printf("  echo 'export SCANORAMA_API_KEY=%s' >> ~/.bashrc\n", generatedKey.Key)
	return nil
}

func executeShowAPIKey(keyIdentifier string) error {
	database, err := setupDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to close database connection: %v\n", err)
		}
	}()

	keyInfo, err := findAPIKey(database, keyIdentifier)
	if err != nil {
		return fmt.Errorf("failed to find API key: %w", err)
	}

	fmt.Println("🔑 API Key Details")
	fmt.Println("═════════════════")
	fmt.Printf("ID: %s\n", keyInfo.ID)
	fmt.Printf("Name: %s\n", keyInfo.Name)
	fmt.Printf("Prefix: %s\n", keyInfo.KeyPrefix)
	status := "Active"
	if !keyInfo.IsActive {
		status = "Revoked"
	} else if keyInfo.ExpiresAt != nil && keyInfo.ExpiresAt.Before(time.Now().UTC()) {
		status = "Expired"
	}
	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Created: %s\n", keyInfo.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	if keyInfo.LastUsedAt != nil {
		fmt.Printf("Last Used: %s\n", keyInfo.LastUsedAt.Format("2006-01-02 15:04:05 MST"))
	} else {
		fmt.Println("Last Used: Never")
	}
	if keyInfo.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", keyInfo.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	} else {
		fmt.Println("Expires: Never")
	}
	if keyInfo.Notes != "" {
		fmt.Printf("Notes: %s\n", keyInfo.Notes)
	}
	return nil
}

func executeUpdateAPIKey(keyIdentifier string) error {
	database, err := setupDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to close database connection: %v\n", err)
		}
	}()

	keyInfo, err := findAPIKey(database, keyIdentifier)
	if err != nil {
		return fmt.Errorf("failed to find API key: %w", err)
	}

	updates := make(map[string]interface{})
	if apiKeyName != "" {
		updates["name"] = apiKeyName
	}
	if apiKeyNotes != "" {
		updates["notes"] = apiKeyNotes
	}

	if len(updates) == 0 {
		fmt.Println("No updates specified.")
		return nil
	}

	updatedKey, err := updateAPIKey(database, keyInfo.ID, updates)
	if err != nil {
		return fmt.Errorf("failed to update API key: %w", err)
	}

	fmt.Println("✅ API Key Updated Successfully")
	fmt.Println("═══════════════════════════════")
	fmt.Printf("ID: %s\n", updatedKey.ID)
	fmt.Printf("Name: %s\n", updatedKey.Name)
	fmt.Printf("Prefix: %s\n", updatedKey.KeyPrefix)
	fmt.Printf("Updated: %s\n", updatedKey.UpdatedAt.Format("2006-01-02 15:04:05 MST"))
	return nil
}

func executeRevokeAPIKey(keyIdentifier string) error {
	database, err := setupDatabaseConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to close database connection: %v\n", err)
		}
	}()

	keyInfo, err := findAPIKey(database, keyIdentifier)
	if err != nil {
		return fmt.Errorf("failed to find API key: %w", err)
	}

	if !apiKeyForce {
		fmt.Printf("⚠️  Are you sure you want to revoke API key '%s' (%s)?\n", keyInfo.Name, keyInfo.KeyPrefix)
		fmt.Println("This action cannot be undone.")
		fmt.Printf("Type 'yes' to confirm: ")

		var confirmation string
		_, _ = fmt.Scanln(&confirmation)
		if !strings.EqualFold(confirmation, "yes") {
			fmt.Println("❌ Revocation cancelled.")
			return nil
		}
	}

	err = revokeAPIKey(database, keyInfo.ID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	fmt.Println("✅ API Key Revoked Successfully")
	fmt.Println("═══════════════════════════════")
	fmt.Printf("Key '%s' (%s) has been revoked and is no longer valid.\n", keyInfo.Name, keyInfo.KeyPrefix)
	return nil
}

// init registers the apikeys commands and flags
func init() {
	// Add apikeys command group to root
	rootCmd.AddCommand(apiKeysCmd)

	// Add subcommands
	apiKeysCmd.AddCommand(apiKeysListCmd)
	apiKeysCmd.AddCommand(apiKeysCreateCmd)
	apiKeysCmd.AddCommand(apiKeysShowCmd)
	apiKeysCmd.AddCommand(apiKeysUpdateCmd)
	apiKeysCmd.AddCommand(apiKeysRevokeCmd)

	// List command flags
	apiKeysListCmd.Flags().BoolVar(&apiKeyShowExpired, "show-expired", false, "Include expired keys in results")
	apiKeysListCmd.Flags().BoolVar(&apiKeyShowInactive, "show-inactive", false,
		"Include revoked/inactive keys in results")
	apiKeysListCmd.Flags().StringVarP(&apiKeyOutput, "output", "o", "table", "Output format (table, json)")

	// Create command flags
	apiKeysCreateCmd.Flags().StringVarP(&apiKeyName, "name", "n", "", "Name for the API key (required)")
	apiKeysCreateCmd.Flags().StringVar(&apiKeyExpiresIn, "expires-in", "", "Expiration time (e.g., 30d, 1h, 7d)")
	apiKeysCreateCmd.Flags().StringVar(&apiKeyNotes, "notes", "", "Optional notes/description")
	_ = apiKeysCreateCmd.MarkFlagRequired("name")

	// Update command flags
	apiKeysUpdateCmd.Flags().StringVarP(&apiKeyName, "name", "n", "", "Update the key name")
	apiKeysUpdateCmd.Flags().StringVar(&apiKeyNotes, "notes", "", "Update notes/description")
	apiKeysUpdateCmd.Flags().StringVar(&apiKeyExpiresIn, "expires-in", "", "Update expiration time (e.g., 30d, 1h, 7d)")

	// Revoke command flags
	apiKeysRevokeCmd.Flags().BoolVarP(&apiKeyForce, "force", "f", false, "Force revocation without confirmation")
}
