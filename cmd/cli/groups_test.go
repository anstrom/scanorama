package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── displayGroupsTable ───────────────────────────────────────────────────────

func TestDisplayGroupsTable(t *testing.T) {
	tests := []struct {
		name            string
		groups          []hostGroup
		expectedHeaders []string
		expectedRows    int
		expectNoRows    bool
	}{
		{
			name:            "empty_list",
			groups:          []hostGroup{},
			expectedHeaders: []string{},
			expectedRows:    0,
			expectNoRows:    true,
		},
		{
			name: "single_group",
			groups: []hostGroup{
				{
					ID:          "aaa-111",
					Name:        "Production",
					Description: "Prod servers",
					Color:       "#FF0000",
					MemberCount: 5,
					CreatedAt:   "2024-01-01T00:00:00Z",
					UpdatedAt:   "2024-01-02T00:00:00Z",
				},
			},
			expectedHeaders: []string{"ID", "NAME", "DESCRIPTION", "COLOR", "MEMBERS", "CREATED"},
			expectedRows:    1,
			expectNoRows:    false,
		},
		{
			name: "multiple_groups",
			groups: []hostGroup{
				{
					ID:          "aaa-111",
					Name:        "Production",
					Description: "Prod servers",
					Color:       "#FF0000",
					MemberCount: 10,
					CreatedAt:   "2024-01-01T00:00:00Z",
					UpdatedAt:   "2024-01-01T00:00:00Z",
				},
				{
					ID:          "bbb-222",
					Name:        "Staging",
					Description: "Staging environment",
					Color:       "#00FF00",
					MemberCount: 3,
					CreatedAt:   "2024-02-01T00:00:00Z",
					UpdatedAt:   "2024-02-02T00:00:00Z",
				},
				{
					ID:          "ccc-333",
					Name:        "Lab",
					Description: "",
					Color:       "",
					MemberCount: 0,
					CreatedAt:   "2024-03-01T00:00:00Z",
					UpdatedAt:   "2024-03-01T00:00:00Z",
				},
			},
			expectedHeaders: []string{"ID", "NAME", "DESCRIPTION", "COLOR", "MEMBERS", "CREATED"},
			expectedRows:    3,
			expectNoRows:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			displayGroupsTable(tt.groups)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			output := buf.String()

			if tt.expectNoRows {
				assert.Contains(t, output, groupsNoGroupsFound)
				return
			}

			for _, header := range tt.expectedHeaders {
				assert.Contains(t, output, header, "Table should contain header: %s", header)
			}

			// Count data rows: lines with pipe separators that are not borders,
			// separators, or the header row (which contains "NAME" in all caps).
			lines := strings.Split(output, "\n")
			dataRows := 0
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				isBorder := strings.ContainsAny(trimmed[:1], "┌└├")
				isSeparator := strings.Contains(line, "─")
				isHeader := strings.Contains(line, "NAME") && strings.Contains(line, "MEMBERS")
				if !isBorder && !isSeparator && !isHeader && strings.Count(line, "│") >= 2 {
					dataRows++
				}
			}
			assert.Equal(t, tt.expectedRows, dataRows,
				"expected %d data rows, got %d\n%s", tt.expectedRows, dataRows, output)

			for _, g := range tt.groups {
				assert.Contains(t, output, g.ID)
				assert.Contains(t, output, g.Name)
			}
		})
	}
}

// ─── displayGroupsJSON ────────────────────────────────────────────────────────

func TestDisplayGroupsJSON(t *testing.T) {
	tests := []struct {
		name   string
		groups []hostGroup
		count  int
	}{
		{
			name:   "empty_list",
			groups: []hostGroup{},
			count:  0,
		},
		{
			name: "single_group",
			groups: []hostGroup{
				{
					ID:          "aaa-111",
					Name:        "Production",
					Description: "Prod servers",
					Color:       "#FF0000",
					MemberCount: 5,
					CreatedAt:   "2024-01-01T00:00:00Z",
					UpdatedAt:   "2024-01-02T00:00:00Z",
				},
			},
			count: 1,
		},
		{
			name: "multiple_groups",
			groups: []hostGroup{
				{
					ID:   "aaa-111",
					Name: "Production",
				},
				{
					ID:   "bbb-222",
					Name: "Staging",
				},
			},
			count: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			displayGroupsJSON(tt.groups)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			output := buf.String()
			require.NotEmpty(t, output)

			// Must be valid JSON.
			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(output), &raw),
				"output must be valid JSON: %s", output)

			// Top-level keys must be present.
			assert.Contains(t, raw, "groups", "JSON should have 'groups' key")
			assert.Contains(t, raw, "count", "JSON should have 'count' key")

			count, ok := raw["count"].(float64)
			require.True(t, ok, "'count' should be a number")
			assert.Equal(t, float64(tt.count), count)

			groups, ok := raw["groups"].([]interface{})
			require.True(t, ok, "'groups' should be an array")
			assert.Len(t, groups, tt.count)

			// Verify indentation is present (pretty-printed).
			assert.Contains(t, output, "  ")
		})
	}
}

func TestDisplayGroupsJSON_NilSlice(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	displayGroupsJSON(nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	require.NotEmpty(t, output)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &raw))

	groups, ok := raw["groups"].([]interface{})
	require.True(t, ok, "'groups' should be an array even when nil was passed")
	assert.Len(t, groups, 0)

	count, ok := raw["count"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(0), count)
}

// ─── displayGroupMembersJSON ──────────────────────────────────────────────────

func TestDisplayGroupMembersJSON(t *testing.T) {
	tests := []struct {
		name    string
		members []groupMember
		count   int
	}{
		{
			name:    "empty_list",
			members: []groupMember{},
			count:   0,
		},
		{
			name: "single_member",
			members: []groupMember{
				{
					ID:        "host-aaa",
					IPAddress: "10.0.0.1",
					Hostname:  "web-01",
					Status:    "up",
					LastSeen:  "2024-06-01T12:00:00Z",
				},
			},
			count: 1,
		},
		{
			name: "multiple_members",
			members: []groupMember{
				{
					ID:        "host-aaa",
					IPAddress: "10.0.0.1",
					Status:    "up",
					LastSeen:  "2024-06-01T12:00:00Z",
				},
				{
					ID:        "host-bbb",
					IPAddress: "10.0.0.2",
					Hostname:  "db-01",
					Status:    "down",
					LastSeen:  "2024-05-31T00:00:00Z",
				},
			},
			count: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			displayGroupMembersJSON(tt.members)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			output := buf.String()
			require.NotEmpty(t, output)

			// Must be valid JSON.
			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(output), &raw),
				"output must be valid JSON: %s", output)

			// Top-level keys must be present.
			assert.Contains(t, raw, "members", "JSON should have 'members' key")
			assert.Contains(t, raw, "count", "JSON should have 'count' key")

			count, ok := raw["count"].(float64)
			require.True(t, ok, "'count' should be a number")
			assert.Equal(t, float64(tt.count), count)

			members, ok := raw["members"].([]interface{})
			require.True(t, ok, "'members' should be an array")
			assert.Len(t, members, tt.count)

			// Verify indentation is present (pretty-printed).
			assert.Contains(t, output, "  ")
		})
	}
}

func TestDisplayGroupMembersJSON_NilSlice(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	displayGroupMembersJSON(nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	require.NotEmpty(t, output)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &raw))

	members, ok := raw["members"].([]interface{})
	require.True(t, ok, "'members' should be an array even when nil was passed")
	assert.Len(t, members, 0)

	count, ok := raw["count"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(0), count)
}

// ─── Command structure ────────────────────────────────────────────────────────

func TestGroupsCommandStructure(t *testing.T) {
	t.Run("groups help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("groups", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "list", "groups help should list the list subcommand")
		assert.Contains(t, all, "show", "groups help should list the show subcommand")
		assert.Contains(t, all, "create", "groups help should list the create subcommand")
		assert.Contains(t, all, "update", "groups help should list the update subcommand")
		assert.Contains(t, all, "delete", "groups help should list the delete subcommand")
		assert.Contains(t, all, "members", "groups help should list the members subcommand")
		assert.Contains(t, all, "add-member", "groups help should list the add-member subcommand")
		assert.Contains(t, all, "remove-member", "groups help should list the remove-member subcommand")
	})

	t.Run("groups show requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("groups", "show")
		assert.Error(t, err, "groups show with no args should fail")
	})

	t.Run("groups delete requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("groups", "delete")
		assert.Error(t, err, "groups delete with no args should fail")
	})

	t.Run("groups members requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("groups", "members")
		assert.Error(t, err, "groups members with no args should fail")
	})

	t.Run("groups add-member requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("groups", "add-member")
		assert.Error(t, err, "groups add-member with no args should fail")
	})

	t.Run("groups create requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("groups", "create")
		assert.Error(t, err, "groups create with no args should fail")
	})

	t.Run("groups create help shows flags", func(t *testing.T) {
		out, errOut, err := executeCommand("groups", "create", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--description", "groups create help should show --description flag")
		assert.Contains(t, all, "--color", "groups create help should show --color flag")
		assert.Contains(t, all, "--output", "groups create help should show --output flag")
	})

	t.Run("groups update help shows flags", func(t *testing.T) {
		out, errOut, err := executeCommand("groups", "update", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--name", "groups update help should show --name flag")
		assert.Contains(t, all, "--description", "groups update help should show --description flag")
		assert.Contains(t, all, "--color", "groups update help should show --color flag")
	})

	t.Run("groups add-member help shows hosts flag", func(t *testing.T) {
		out, errOut, err := executeCommand("groups", "add-member", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--hosts", "groups add-member help should show --hosts flag")
	})

	t.Run("groups remove-member help shows hosts flag", func(t *testing.T) {
		out, errOut, err := executeCommand("groups", "remove-member", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--hosts", "groups remove-member help should show --hosts flag")
	})
}

// ─── splitHosts ───────────────────────────────────────────────────────────────

func TestSplitHosts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single host",
			input:    "host-aaa",
			expected: []string{"host-aaa"},
		},
		{
			name:     "multiple hosts",
			input:    "host-aaa,host-bbb,host-ccc",
			expected: []string{"host-aaa", "host-bbb", "host-ccc"},
		},
		{
			name:     "hosts with spaces",
			input:    " host-aaa , host-bbb ",
			expected: []string{"host-aaa", "host-bbb"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only commas",
			input:    ",,,",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitHosts(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ─── decodeMembersList ────────────────────────────────────────────────────────

func TestDecodeMembersList(t *testing.T) {
	t.Run("non-empty array returns members", func(t *testing.T) {
		// Simulate apiResp.Data holding the decoded value of the "data" key from
		// a paginated envelope: a []interface{} containing the member objects.
		data := []interface{}{
			map[string]interface{}{
				"id":         "host-1",
				"ip_address": "192.168.1.1",
				"hostname":   "web01",
				"status":     "up",
				"last_seen":  "2024-01-15T10:00:00Z",
			},
			map[string]interface{}{
				"id":         "host-2",
				"ip_address": "192.168.1.2",
				"hostname":   "",
				"status":     "down",
				"last_seen":  "2024-01-14T09:00:00Z",
			},
		}

		members, err := decodeMembersList(data)
		require.NoError(t, err)
		require.Len(t, members, 2)
		assert.Equal(t, "host-1", members[0].ID)
		assert.Equal(t, "192.168.1.1", members[0].IPAddress)
		assert.Equal(t, "web01", members[0].Hostname)
		assert.Equal(t, "host-2", members[1].ID)
	})

	t.Run("empty array returns empty non-nil slice", func(t *testing.T) {
		members, err := decodeMembersList([]interface{}{})
		require.NoError(t, err)
		assert.NotNil(t, members)
		assert.Empty(t, members)
	})

	t.Run("nil data returns empty non-nil slice", func(t *testing.T) {
		members, err := decodeMembersList(nil)
		require.NoError(t, err)
		assert.NotNil(t, members)
		assert.Empty(t, members)
	})
}
