// Package cli provides command-line interface commands for the Scanorama network scanner.
// This file implements the groups subcommand with full CRUD and member management.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// Groups command flag variables.
const (
	groupsEndpoint        = "/groups"
	groupsMembersPath     = "/hosts"
	groupsNoGroupsFound   = "No groups found."
	groupsNoMembersFound  = "No members found."
	groupsOutputFlagUsage = "Output format (table, json)"
)

var (
	groupsOutput      string
	groupsName        string
	groupsDescription string
	groupsColor       string
	groupsHosts       string
)

// hostGroup mirrors the server-side db.HostGroup for JSON decoding.
type hostGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color,omitempty"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// groupMember mirrors the server-side groupMemberResponse for JSON decoding.
type groupMember struct {
	ID        string `json:"id"`
	IPAddress string `json:"ip_address"`
	Hostname  string `json:"hostname,omitempty"`
	Status    string `json:"status"`
	LastSeen  string `json:"last_seen"`
}

// groupsListResponse is the wire format for GET /groups.
type groupsListResponse struct {
	Groups []hostGroup `json:"groups"`
	Total  int         `json:"total"`
}

// groupsMemberRequest is the request body for add/remove member operations.
type groupsMemberRequest struct {
	HostIDs []string `json:"host_ids"`
}

// groupsCreateRequest is the request body for POST /groups.
type groupsCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// groupsUpdateRequest is the request body for PUT /groups/{id}.
type groupsUpdateRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// ─── Command definitions ──────────────────────────────────────────────────────

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Manage host groups",
	Long: `View and manage host groups. Groups allow you to organize discovered hosts
into logical collections for targeted scanning, dashboards, and reporting.

Examples:
  scanorama groups list
  scanorama groups create "Production" --description "Prod servers" --color "#FF0000"
  scanorama groups show <id>
  scanorama groups update <id> --name "Staging"
  scanorama groups delete <id>
  scanorama groups members <id>
  scanorama groups add-member <group-id> --hosts h1,h2
  scanorama groups remove-member <group-id> --hosts h1`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var groupsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all host groups",
	Long: `List all host groups with member counts and metadata.

Examples:
  scanorama groups list
  scanorama groups list --output json`,
	Run: runGroupsList,
}

var groupsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a host group",
	Long: `Show detailed information about a specific host group.

Examples:
  scanorama groups show <id>
  scanorama groups show <id> --output json`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsShow,
}

var groupsCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a host group",
	Long: `Create a new host group.

Examples:
  scanorama groups create "Production"
  scanorama groups create "Staging" --description "Staging servers" --color "#00FF00"
  scanorama groups create "Lab" --output json`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsCreate,
}

var groupsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a host group",
	Long: `Update an existing host group. At least one of --name, --description,
or --color must be provided.

Examples:
  scanorama groups update <id> --name "New Name"
  scanorama groups update <id> --description "Updated description" --color "#0000FF"`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsUpdate,
}

var groupsDeleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"remove", "rm"},
	Short:   "Delete a host group",
	Long: `Delete a host group by ID.

Examples:
  scanorama groups delete <id>`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsDelete,
}

var groupsMembersCmd = &cobra.Command{
	Use:   "members <id>",
	Short: "List members of a host group",
	Long: `List all hosts that are members of the given group.

Examples:
  scanorama groups members <id>
  scanorama groups members <id> --output json`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsMembers,
}

var groupsAddMemberCmd = &cobra.Command{
	Use:   "add-member <group-id>",
	Short: "Add hosts to a group",
	Long: `Add one or more hosts to a host group.

Examples:
  scanorama groups add-member <group-id> --hosts <host-id1>,<host-id2>`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsAddMember,
}

var groupsRemoveMemberCmd = &cobra.Command{
	Use:   "remove-member <group-id>",
	Short: "Remove hosts from a group",
	Long: `Remove one or more hosts from a host group.

Examples:
  scanorama groups remove-member <group-id> --hosts <host-id1>,<host-id2>`,
	Args: cobra.ExactArgs(1),
	Run:  runGroupsRemoveMember,
}

// ─── Run handlers ─────────────────────────────────────────────────────────────

func runGroupsList(cmd *cobra.Command, args []string) {
	if err := executeGroupsList(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsShow(cmd *cobra.Command, args []string) {
	if err := executeGroupsShow(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsCreate(cmd *cobra.Command, args []string) {
	if err := executeGroupsCreate(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsUpdate(cmd *cobra.Command, args []string) {
	if err := executeGroupsUpdate(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsDelete(cmd *cobra.Command, args []string) {
	if err := executeGroupsDelete(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsMembers(cmd *cobra.Command, args []string) {
	if err := executeGroupsMembers(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsAddMember(cmd *cobra.Command, args []string) {
	if err := executeGroupsMemberChange(args[0], true); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runGroupsRemoveMember(cmd *cobra.Command, args []string) {
	if err := executeGroupsMemberChange(args[0], false); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ─── Execute helpers ──────────────────────────────────────────────────────────

func executeGroupsList() error {
	return WithAPIClient("list groups", func(client *APIClient) error {
		resp, err := client.Get(groupsEndpoint)
		if err != nil {
			return err
		}

		groups, err := decodeGroupsList(resp.Data)
		if err != nil {
			return err
		}

		if groupsOutput == outputFormatJSON {
			displayGroupsJSON(groups)
		} else {
			displayGroupsTable(groups)
		}
		return nil
	})
}

func executeGroupsShow(id string) error {
	return WithAPIClient("show group", func(client *APIClient) error {
		resp, err := client.Get(groupsEndpoint + "/" + id)
		if err != nil {
			return err
		}

		group, err := decodeGroup(resp.Data)
		if err != nil {
			return err
		}

		if groupsOutput == outputFormatJSON {
			displayGroupsJSON([]hostGroup{group})
		} else {
			displayGroupsTable([]hostGroup{group})
		}
		return nil
	})
}

func executeGroupsCreate(name string) error {
	return WithAPIClient("create group", func(client *APIClient) error {
		payload := groupsCreateRequest{
			Name:        name,
			Description: groupsDescription,
			Color:       groupsColor,
		}

		resp, err := client.Post(groupsEndpoint, payload)
		if err != nil {
			return err
		}

		group, err := decodeGroup(resp.Data)
		if err != nil {
			return err
		}

		if groupsOutput == outputFormatJSON {
			displayGroupsJSON([]hostGroup{group})
		} else {
			fmt.Printf("Group created: %s (%s)\n", group.Name, group.ID)
		}
		return nil
	})
}

func executeGroupsUpdate(id string) error {
	return WithAPIClient("update group", func(client *APIClient) error {
		payload := groupsUpdateRequest{
			Name:        groupsName,
			Description: groupsDescription,
			Color:       groupsColor,
		}

		if payload.Name == "" && payload.Description == "" && payload.Color == "" {
			return fmt.Errorf("at least one of --name, --description, or --color must be provided")
		}

		resp, err := client.Put(groupsEndpoint+"/"+id, payload)
		if err != nil {
			return err
		}

		group, err := decodeGroup(resp.Data)
		if err != nil {
			return err
		}

		fmt.Printf("Group updated: %s (%s)\n", group.Name, group.ID)
		return nil
	})
}

func executeGroupsDelete(id string) error {
	return WithAPIClient("delete group", func(client *APIClient) error {
		_, err := client.Delete(groupsEndpoint + "/" + id)
		if err != nil {
			return err
		}
		fmt.Printf("Group %s deleted successfully.\n", id)
		return nil
	})
}

func executeGroupsMembers(id string) error {
	return WithAPIClient("list group members", func(client *APIClient) error {
		resp, err := client.Get(groupsEndpoint + "/" + id + groupsMembersPath)
		if err != nil {
			return err
		}

		members, err := decodeMembersList(resp.Data)
		if err != nil {
			return err
		}

		if groupsOutput == outputFormatJSON {
			displayGroupMembersJSON(members)
		} else {
			displayGroupMembersTable(members)
		}
		return nil
	})
}

func executeGroupsMemberChange(groupID string, add bool) error {
	if groupsHosts == "" {
		return fmt.Errorf("--hosts flag is required")
	}

	hostIDs := splitHosts(groupsHosts)

	operation := "remove hosts from group"
	if add {
		operation = "add hosts to group"
	}

	return WithAPIClient(operation, func(client *APIClient) error {
		endpoint := groupsEndpoint + "/" + groupID + groupsMembersPath
		payload := groupsMemberRequest{HostIDs: hostIDs}

		var err error
		if add {
			_, err = client.Post(endpoint, payload)
		} else {
			_, err = client.DeleteWithBody(endpoint, payload)
		}
		if err != nil {
			return err
		}

		verb := "removed from"
		if add {
			verb = "added to"
		}
		fmt.Printf("%d host(s) %s group %s.\n", len(hostIDs), verb, groupID)
		return nil
	})
}

// splitHosts parses a comma-separated list of host IDs.
func splitHosts(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ─── Decode helpers ───────────────────────────────────────────────────────────

func decodeGroupsList(data interface{}) ([]hostGroup, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to encode response: %w", err)
	}

	var resp groupsListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("failed to decode groups list: %w", err)
	}

	if resp.Groups == nil {
		return make([]hostGroup, 0), nil
	}
	return resp.Groups, nil
}

func decodeGroup(data interface{}) (hostGroup, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return hostGroup{}, fmt.Errorf("failed to encode response: %w", err)
	}

	var g hostGroup
	if err := json.Unmarshal(raw, &g); err != nil {
		return hostGroup{}, fmt.Errorf("failed to decode group: %w", err)
	}
	return g, nil
}

func decodeMembersList(data interface{}) ([]groupMember, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to encode response: %w", err)
	}

	// apiResp.Data holds the value of the "data" key from the paginated envelope
	// — an array, not the envelope struct. Unmarshal directly into []groupMember.
	var members []groupMember
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("failed to decode members list: %w", err)
	}

	if members == nil {
		return make([]groupMember, 0), nil
	}
	return members, nil
}

// ─── Display helpers ──────────────────────────────────────────────────────────

// displayGroupsTable renders groups in a tabular format.
func displayGroupsTable(groups []hostGroup) {
	if len(groups) == 0 {
		fmt.Println(groupsNoGroupsFound)
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("ID", "NAME", "DESCRIPTION", "COLOR", "MEMBERS", "CREATED")

	for i := range groups {
		g := &groups[i]
		desc := g.Description
		if len(desc) > maxDescriptionLength {
			desc = desc[:maxDescriptionLength] + "..."
		}
		_ = table.Append([]string{
			g.ID,
			g.Name,
			desc,
			g.Color,
			fmt.Sprintf("%d", g.MemberCount),
			g.CreatedAt,
		})
	}

	_ = table.Render()
}

// displayGroupsJSON renders groups as a JSON object with a count field.
func displayGroupsJSON(groups []hostGroup) {
	if groups == nil {
		groups = make([]hostGroup, 0)
	}

	output := struct {
		Groups []hostGroup `json:"groups"`
		Count  int         `json:"count"`
	}{
		Groups: groups,
		Count:  len(groups),
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

// displayGroupMembersTable renders group members in a tabular format.
func displayGroupMembersTable(members []groupMember) {
	if len(members) == 0 {
		fmt.Println(groupsNoMembersFound)
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("ID", "IP ADDRESS", "HOSTNAME", "STATUS", "LAST SEEN")

	for i := range members {
		m := &members[i]
		_ = table.Append([]string{
			m.ID,
			m.IPAddress,
			m.Hostname,
			m.Status,
			m.LastSeen,
		})
	}

	_ = table.Render()
}

// displayGroupMembersJSON renders group members as a JSON object with a count field.
func displayGroupMembersJSON(members []groupMember) {
	if members == nil {
		members = make([]groupMember, 0)
	}

	output := struct {
		Members []groupMember `json:"members"`
		Count   int           `json:"count"`
	}{
		Members: members,
		Count:   len(members),
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

// ─── Init ─────────────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(groupsCmd)

	groupsCmd.AddCommand(groupsListCmd)
	groupsCmd.AddCommand(groupsShowCmd)
	groupsCmd.AddCommand(groupsCreateCmd)
	groupsCmd.AddCommand(groupsUpdateCmd)
	groupsCmd.AddCommand(groupsDeleteCmd)
	groupsCmd.AddCommand(groupsMembersCmd)
	groupsCmd.AddCommand(groupsAddMemberCmd)
	groupsCmd.AddCommand(groupsRemoveMemberCmd)

	// list flags
	groupsListCmd.Flags().StringVarP(&groupsOutput, "output", "o", "table", groupsOutputFlagUsage)

	// show flags
	groupsShowCmd.Flags().StringVarP(&groupsOutput, "output", "o", "table", groupsOutputFlagUsage)

	// create flags
	groupsCreateCmd.Flags().StringVar(&groupsDescription, "description", "", "Group description")
	groupsCreateCmd.Flags().StringVar(&groupsColor, "color", "", "Group color as a hex value (e.g. #FF0000)")
	groupsCreateCmd.Flags().StringVarP(&groupsOutput, "output", "o", "table", groupsOutputFlagUsage)

	// update flags
	groupsUpdateCmd.Flags().StringVar(&groupsName, "name", "", "New group name")
	groupsUpdateCmd.Flags().StringVar(&groupsDescription, "description", "", "New group description")
	groupsUpdateCmd.Flags().StringVar(&groupsColor, "color", "", "New group color as a hex value")

	// members flags
	groupsMembersCmd.Flags().StringVarP(&groupsOutput, "output", "o", "table", groupsOutputFlagUsage)

	// add-member / remove-member flags
	groupsAddMemberCmd.Flags().StringVar(&groupsHosts, "hosts", "", "Comma-separated list of host IDs (required)")
	groupsRemoveMemberCmd.Flags().StringVar(&groupsHosts, "hosts", "", "Comma-separated list of host IDs (required)")

	if err := groupsAddMemberCmd.MarkFlagRequired("hosts"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark hosts flag as required: %v\n", err)
	}
	if err := groupsRemoveMemberCmd.MarkFlagRequired("hosts"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to mark hosts flag as required: %v\n", err)
	}
}
